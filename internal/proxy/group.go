package proxy

// group.go — 代理组实现
//
// 参照 mihomo/adapter/outboundgroup 实现三种代理组：
//   SelectGroup  — 手动选择（对应 mihomo Selector）
//   URLTestGroup — 按延迟自动选最快（对应 mihomo URLTest）
//   FallbackGroup— 按顺序 fallback 失败则换下一个（对应 mihomo Fallback）

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ── SelectGroup：手动选择（对应 mihomo/adapter/outboundgroup.Selector）────────

type SelectGroup struct {
	name     string
	proxies  []Outbound
	selected atomic.Int32 // 选中的代理索引
	mu       sync.RWMutex
}

func NewSelectGroup(name string, proxies []Outbound) *SelectGroup {
	g := &SelectGroup{name: name, proxies: proxies}
	// 默认选第一个
	g.selected.Store(0)
	return g
}

func (g *SelectGroup) Name() string { return g.name }

func (g *SelectGroup) Now() string {
	idx := int(g.selected.Load())
	if idx >= 0 && idx < len(g.proxies) {
		return g.proxies[idx].Name()
	}
	return ""
}

// Select 手动切换到的代理（对应 mihomo Selector.Set）
func (g *SelectGroup) Select(name string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for i, p := range g.proxies {
		if p.Name() == name {
			g.selected.Store(int32(i))
			return nil
		}
	}
	return fmt.Errorf("proxy %q not found in group %q", name, g.name)
}

func (g *SelectGroup) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	idx := int(g.selected.Load())
	if idx < 0 || idx >= len(g.proxies) {
		return nil, fmt.Errorf("no proxy selected in group %q", g.name)
	}
	return g.proxies[idx].DialTCP(ctx, metadata)
}

func (g *SelectGroup) Proxies() []Outbound {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.proxies
}

// ── URLTestGroup：延迟最低自动选择（对应 mihomo URLTest）───────────────────────

type proxyDelay struct {
	proxy Outbound
	delay time.Duration
	alive bool
}

type URLTestGroup struct {
	name      string
	proxies   []Outbound
	testURL   string
	interval  time.Duration
	tolerance time.Duration

	mu      sync.RWMutex
	delays  []proxyDelay // 与 proxies 一一对应
	fastIdx int32        // 当前延迟最低的代理索引（atomic）
	stopCh  chan struct{}
	once    sync.Once
}

func NewURLTestGroup(name string, proxies []Outbound, testURL string, interval, tolerance time.Duration) *URLTestGroup {
	g := &URLTestGroup{
		name:      name,
		proxies:   proxies,
		testURL:   testURL,
		interval:  interval,
		tolerance: tolerance,
		delays:    make([]proxyDelay, len(proxies)),
		stopCh:    make(chan struct{}),
	}
	for i, p := range proxies {
		g.delays[i] = proxyDelay{proxy: p, delay: 999 * time.Second, alive: false}
	}
	return g
}

func (g *URLTestGroup) Name() string { return g.name }

func (g *URLTestGroup) Now() string {
	idx := int(atomic.LoadInt32(&g.fastIdx))
	if idx >= 0 && idx < len(g.proxies) {
		return g.proxies[idx].Name()
	}
	return ""
}

func (g *URLTestGroup) Start() {
	g.once.Do(func() {
		// 启动时立即测试一次，然后定期测试
		go g.testAll()
		go g.loop()
	})
}

func (g *URLTestGroup) Stop() {
	select {
	case g.stopCh <- struct{}{}:
	default:
	}
}

func (g *URLTestGroup) loop() {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			go g.testAll()
		case <-g.stopCh:
			return
		}
	}
}

// testAll 并发测试所有代理的延迟（对应 mihomo GroupBase.healthCheck）
func (g *URLTestGroup) testAll() {
	var wg sync.WaitGroup
	results := make([]proxyDelay, len(g.proxies))
	for i, p := range g.proxies {
		results[i] = proxyDelay{proxy: p}
		wg.Add(1)
		go func(idx int, proxy Outbound) {
			defer wg.Done()
			d, err := g.testOne(proxy)
			if err != nil {
				results[idx].alive = false
				results[idx].delay = 999 * time.Second
			} else {
				results[idx].alive = true
				results[idx].delay = d
			}
		}(i, p)
	}
	wg.Wait()

	g.mu.Lock()
	g.delays = results
	g.mu.Unlock()

	g.selectFast()
}

// testOne 测试单个代理的延迟（对应 mihomo URLTest.URLTest）
func (g *URLTestGroup) testOne(proxy Outbound) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用 HTTP（非 HTTPS）URL 来测试延迟，避免端口不匹配导致
	// "Unsolicited response received on idle HTTP channel" 错误。
	// testURL 可能是 https 的，但我们强制用 http + port 80 测试连通性。
	testURL := "http://www.gstatic.com/generate_204"

	start := time.Now()
	meta := &Metadata{Network: "tcp", Type: "HTTP", DstHost: "www.gstatic.com", DstPort: 80}
	conn, err := proxy.DialTCP(ctx, meta)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	// 发送 HTTP GET（注意：使用 http:// URL 与端口 80 一致）
	req, _ := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	req.Header.Set("User-Agent", "ClashGo/1.0")
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return conn, nil
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return time.Since(start), nil
}

// selectFast 从测试结果中选出延迟最低的存活代理
func (g *URLTestGroup) selectFast() {
	g.mu.RLock()
	delays := g.delays
	g.mu.RUnlock()

	bestIdx := 0
	bestDelay := time.Duration(999 * time.Second)
	for i, d := range delays {
		if d.alive && d.delay < bestDelay {
			bestDelay = d.delay
			bestIdx = i
		}
	}

	// 容忍度：当前 fast 代理与最快代理的差在 tolerance 内则不切换
	current := int(atomic.LoadInt32(&g.fastIdx))
	if current >= 0 && current < len(delays) && delays[current].alive {
		if delays[current].delay-bestDelay <= g.tolerance {
			return
		}
	}
	atomic.StoreInt32(&g.fastIdx, int32(bestIdx))
}

func (g *URLTestGroup) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	// 确保测试已启动
	g.Start()

	idx := int(atomic.LoadInt32(&g.fastIdx))
	if idx >= 0 && idx < len(g.proxies) {
		conn, err := g.proxies[idx].DialTCP(ctx, metadata)
		if err == nil {
			return conn, nil
		}
	}
	// fast 失败则遍历找一个可用的
	for _, p := range g.proxies {
		conn, err := p.DialTCP(ctx, metadata)
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("all proxies failed in group %q", g.name)
}

// ── FallbackGroup：顺序故障转移（对应 mihomo Fallback）───────────────────────

type FallbackGroup struct {
	name    string
	proxies []Outbound
	testURL string
	mu      sync.RWMutex
	alive   []bool // 对应 proxies 中每个的存活状态
	stopCh  chan struct{}
	once    sync.Once
}

func NewFallbackGroup(name string, proxies []Outbound, testURL string, interval time.Duration) *FallbackGroup {
	g := &FallbackGroup{
		name:    name,
		proxies: proxies,
		testURL: testURL,
		alive:   make([]bool, len(proxies)),
		stopCh:  make(chan struct{}),
	}
	// 初始全部标记为存活
	for i := range g.alive {
		g.alive[i] = true
	}
	go g.loop(interval)
	return g
}

func (g *FallbackGroup) Name() string { return g.name }

func (g *FallbackGroup) Now() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for i, alive := range g.alive {
		if alive && i < len(g.proxies) {
			return g.proxies[i].Name()
		}
	}
	return ""
}

func (g *FallbackGroup) loop(interval time.Duration) {
	go g.checkAll()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			go g.checkAll()
		case <-g.stopCh:
			return
		}
	}
}

// checkAll 并发检测所有代理存活状态
func (g *FallbackGroup) checkAll() {
	newAlive := make([]bool, len(g.proxies))
	var wg sync.WaitGroup
	for i, p := range g.proxies {
		wg.Add(1)
		go func(idx int, proxy Outbound) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			meta := &Metadata{Network: "tcp", Type: "HTTP", DstHost: "www.gstatic.com", DstPort: 80}
			conn, err := proxy.DialTCP(ctx, meta)
			if err != nil {
				return
			}
			defer conn.Close()
			// 发一个真正的 HTTP 请求来验证连通性，避免只连接不发请求
			req, _ := http.NewRequestWithContext(ctx, "HEAD", "http://www.gstatic.com/generate_204", nil)
			client := &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
						return conn, nil
					},
				},
			}
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				newAlive[idx] = true
			}
		}(i, p)
	}
	wg.Wait()
	g.mu.Lock()
	g.alive = newAlive
	g.mu.Unlock()
}

func (g *FallbackGroup) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	g.mu.RLock()
	aliveSnapshot := make([]bool, len(g.alive))
	copy(aliveSnapshot, g.alive)
	g.mu.RUnlock()

	// 按顺序找第一个存活代理
	for i, alive := range aliveSnapshot {
		if !alive {
			continue
		}
		conn, err := g.proxies[i].DialTCP(ctx, metadata)
		if err == nil {
			return conn, nil
		}
		// 连接失败：更新存活状态
		g.mu.Lock()
		if i < len(g.alive) {
			g.alive[i] = false
		}
		g.mu.Unlock()
	}
	// 所有代理均失败
	return nil, fmt.Errorf("all proxies dead in fallback group %q", g.name)
}

// ── LoadBalanceGroup：轮询负载均衡（对应 mihomo LoadBalance）─────────────────

type LoadBalanceGroup struct {
	name    string
	proxies []Outbound
	counter atomic.Int32
}

func NewLoadBalanceGroup(name string, proxies []Outbound) *LoadBalanceGroup {
	return &LoadBalanceGroup{name: name, proxies: proxies}
}

func (g *LoadBalanceGroup) Name() string { return g.name }

func (g *LoadBalanceGroup) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	if len(g.proxies) == 0 {
		return nil, fmt.Errorf("no proxies in load-balance group %q", g.name)
	}
	idx := int(g.counter.Add(1)-1) % len(g.proxies)
	return g.proxies[idx].DialTCP(ctx, metadata)
}

// ── ProxyGroupInfo 供 REST API 序列化用 ──────────────────────────────────────

type ProxyGroupInfo struct {
	Name   string           `json:"name"`
	Type   string           `json:"type"`
	Now    string           `json:"now"`
	All    []string         `json:"all"`
	Delays map[string]int64 `json:"delays,omitempty"` // ms
}

func GroupInfo(g Outbound) *ProxyGroupInfo {
	info := &ProxyGroupInfo{Name: g.Name()}
	switch v := g.(type) {
	case *SelectGroup:
		info.Type = "Selector"
		info.Now = v.Now()
		for _, p := range v.proxies {
			info.All = append(info.All, p.Name())
		}
	case *URLTestGroup:
		info.Type = "URLTest"
		info.Now = v.Now()
		info.Delays = map[string]int64{}
		v.mu.RLock()
		for _, d := range v.delays {
			info.All = append(info.All, d.proxy.Name())
			info.Delays[d.proxy.Name()] = d.delay.Milliseconds()
		}
		sort.Slice(info.All, func(i, j int) bool {
			return info.Delays[info.All[i]] < info.Delays[info.All[j]]
		})
		v.mu.RUnlock()
	case *FallbackGroup:
		info.Type = "Fallback"
		info.Now = v.Now()
		for _, p := range v.proxies {
			info.All = append(info.All, p.Name())
		}
	case *LoadBalanceGroup:
		info.Type = "LoadBalance"
		for _, p := range v.proxies {
			info.All = append(info.All, p.Name())
		}
	default:
		info.Type = "Unknown"
	}
	return info
}
