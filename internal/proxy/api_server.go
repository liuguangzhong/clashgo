package proxy

// api_server.go — HTTP RESTful API 控制器
//
// 参照 mihomo/hub/route/ 实现完整的 REST API：
//   GET  /version                   — 版本信息
//   GET  /proxies                   — 所有代理列表
//   PUT  /proxies/{name}            — 切换 Selector 代理
//   GET  /proxies/{name}/delay      — 测速
//   GET  /rules                     — 所有规则
//   PUT  /configs                   — 热加载配置
//   PATCH /configs                  — 修改模式/允许LAN等
//   GET  /connections               — 活跃连接（简化）
//   GET  /logs                      — WebSocket 实时日志
//   GET  /traffic                   — WebSocket 实时流量

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// APIServer mihomo RESTful API 服务（对应 mihomo/hub/route）
type APIServer struct {
	kernel *Kernel
	secret string // API 鉴权 Token（对应 mihomo secret）
	mux    *http.ServeMux
	server *http.Server

	// 活跃连接跟踪（对应 mihomo/tunnel/statistic）
	connMu  sync.RWMutex
	conns   map[string]*trackedConn
	connSeq atomic.Uint64

	// 实时日志订阅（对应 mihomo/log Subscribe）
	logMu   sync.Mutex
	logSubs map[uint64]chan string
	logSeq  atomic.Uint64
}

// trackedConn 记录连接信息（供 /connections 使用）
type trackedConn struct {
	ID       string    `json:"id"`
	Network  string    `json:"network"`
	DstHost  string    `json:"dst"`
	DstPort  uint16    `json:"port"`
	Rule     string    `json:"rule"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Start    time.Time `json:"start"`
}

// NewAPIServer 创建 API 服务（对应 mihomo hub.NewRouter）
func NewAPIServer(kernel *Kernel, secret string) *APIServer {
	s := &APIServer{
		kernel:  kernel,
		secret:  secret,
		mux:     http.NewServeMux(),
		conns:   make(map[string]*trackedConn),
		logSubs: make(map[uint64]chan string),
	}
	s.registerRoutes()
	return s
}

// Start 在 addr 上启动 API 服务器（对应 mihomo hub.Start）
func (s *APIServer) Start(addr string) error {
	s.server = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("API server listen %s: %w", addr, err)
	}
	go func() {
		_ = s.server.Serve(ln)
	}()
	return nil
}

// Stop 停止 API 服务器
func (s *APIServer) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
	}
}

// PushLog 推送一条日志到所有订阅者
func (s *APIServer) PushLog(line string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	for _, ch := range s.logSubs {
		select {
		case ch <- line:
		default: // 丢弃堆积
		}
	}
}

// registerRoutes 注册所有路由（对应 mihomo hub/route/route.go）
func (s *APIServer) registerRoutes() {
	m := s.mux

	// CORS + auth 中间件通过包装处理
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// CORS（对应 mihomo CORS 配置）
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET,PUT,PATCH,POST,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			// Bearer Token 鉴权
			if s.secret != "" {
				auth := r.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != s.secret {
					// 也支持 query param ?secret=xxx
					if r.URL.Query().Get("secret") != s.secret {
						writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
						return
					}
				}
			}
			h(w, r)
		}
	}

	// GET /version
	m.HandleFunc("/version", wrap(s.handleVersion))

	// /proxies
	m.HandleFunc("/proxies", wrap(s.handleProxies))
	m.HandleFunc("/proxies/", wrap(s.handleProxyItem))

	// /rules
	m.HandleFunc("/rules", wrap(s.handleRules))

	// /configs
	m.HandleFunc("/configs", wrap(s.handleConfigs))

	// /connections
	m.HandleFunc("/connections", wrap(s.handleConnections))

	// /logs  (WebSocket 或 long-poll)
	m.HandleFunc("/logs", wrap(s.handleLogs))

	// /traffic (WebSocket 流量)
	m.HandleFunc("/traffic", wrap(s.handleTraffic))
}

// ── Route Handlers ────────────────────────────────────────────────────────────

// GET /version
func (s *APIServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version": "ClashGo/1.0.0",
		"meta":    "true",
	})
}

// GET /proxies
func (s *APIServer) handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errMsg("method not allowed"))
		return
	}
	proxies := s.kernel.tunnel.proxies.Load().(map[string]Outbound)
	result := make(map[string]interface{})
	for name, p := range proxies {
		switch v := p.(type) {
		case *SelectGroup:
			result[name] = GroupInfo(v)
		case *URLTestGroup:
			result[name] = GroupInfo(v)
		case *FallbackGroup:
			result[name] = GroupInfo(v)
		case *LoadBalanceGroup:
			result[name] = GroupInfo(v)
		default:
			result[name] = map[string]string{
				"name": name,
				"type": fmt.Sprintf("%T", p),
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proxies": result})
}

// /proxies/{name} 和 /proxies/{name}/delay
func (s *APIServer) handleProxyItem(w http.ResponseWriter, r *http.Request) {
	// 解析路径：/proxies/{name} 或 /proxies/{name}/delay
	path := strings.TrimPrefix(r.URL.Path, "/proxies/")
	parts := strings.SplitN(path, "/", 2)
	name := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	proxies := s.kernel.tunnel.proxies.Load().(map[string]Outbound)
	proxy, ok := proxies[name]
	if !ok {
		writeJSON(w, http.StatusNotFound, errMsg("proxy not found: "+name))
		return
	}

	switch sub {
	case "delay":
		// GET /proxies/{name}/delay?url=...&timeout=2000
		s.handleDelay(w, r, proxy)
	default:
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]string{"name": proxy.Name(), "type": fmt.Sprintf("%T", proxy)})
		case http.MethodPut:
			// 切换 Selector
			var req struct {
				Name string `json:"name"`
			}
			if err := readJSON(r, &req); err != nil {
				writeJSON(w, http.StatusBadRequest, errMsg(err.Error()))
				return
			}
			sel, ok := proxy.(*SelectGroup)
			if !ok {
				writeJSON(w, http.StatusBadRequest, errMsg("not a selector"))
				return
			}
			if err := sel.Select(req.Name); err != nil {
				writeJSON(w, http.StatusBadRequest, errMsg(err.Error()))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, errMsg("method not allowed"))
		}
	}
}

// GET /proxies/{name}/delay?url=...&timeout=ms
func (s *APIServer) handleDelay(w http.ResponseWriter, r *http.Request, proxy Outbound) {
	q := r.URL.Query()
	url := q.Get("url")
	if url == "" {
		url = "http://www.gstatic.com/generate_204"
	}
	timeoutMs := 5000
	fmt.Sscanf(q.Get("timeout"), "%d", &timeoutMs)

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	start := time.Now()
	meta := &Metadata{Network: "tcp", DstHost: "www.gstatic.com", DstPort: 80}
	conn, err := proxy.DialTCP(ctx, meta)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errMsg("dial failed: "+err.Error()))
		return
	}
	defer conn.Close()

	// 发 HTTP HEAD 请求
	req, _ := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return conn, nil
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errMsg("request failed: "+err.Error()))
		return
	}
	_ = resp.Body.Close()
	writeJSON(w, http.StatusOK, map[string]int64{"delay": time.Since(start).Milliseconds()})
}

// GET /rules
func (s *APIServer) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errMsg("method not allowed"))
		return
	}
	rules := s.kernel.tunnel.rules.Load().([]Rule)
	type ruleItem struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
		Proxy   string `json:"proxy"`
	}
	var items []ruleItem
	for _, rule := range rules {
		items = append(items, ruleItem{
			Type:    rule.RuleType(),
			Payload: rule.Payload(),
			Proxy:   rule.Adapter(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rules": items,
		"count": len(items),
	})
}

// PUT /configs — 热加载配置文件（对应 mihomo PUT /configs?force=true）
// PATCH /configs — 修改运行参数（mode/allow-lan）
func (s *APIServer) handleConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		// 读取配置路径，重新解析并应用
		var req struct {
			Path string `json:"path"`
		}
		if err := readJSON(r, &req); err != nil {
			// 尝试直接从请求体读 YAML
			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				cfg, err := ParseConfig(body)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, errMsg("parse config: "+err.Error()))
					return
				}
				if err := s.kernel.ApplyConfig(cfg); err != nil {
					writeJSON(w, http.StatusInternalServerError, errMsg("apply config: "+err.Error()))
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			writeJSON(w, http.StatusBadRequest, errMsg(err.Error()))
			return
		}
		if req.Path != "" {
			data, err := os.ReadFile(req.Path)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errMsg("read config file: "+err.Error()))
				return
			}
			cfg, err := ParseConfig(data)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errMsg("parse config: "+err.Error()))
				return
			}
			if err := s.kernel.ApplyConfig(cfg); err != nil {
				writeJSON(w, http.StatusInternalServerError, errMsg("apply config: "+err.Error()))
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodPatch:
		// 对应 mihomo PATCH /configs 修改模式等
		var req struct {
			Mode     string `json:"mode"`
			AllowLan *bool  `json:"allow-lan"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errMsg(err.Error()))
			return
		}
		if req.Mode != "" {
			s.kernel.tunnel.SetMode(ParseMode(req.Mode))
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodGet:
		cfg := s.kernel.GetConfig()
		mixed := 7897
		if cfg != nil && cfg.MixedPort > 0 {
			mixed = int(cfg.MixedPort)
		}
		mode := "rule"
		if s.kernel.tunnel != nil {
			mode = s.kernel.tunnel.Mode().String()
		}
		allowLan := false
		if cfg != nil {
			allowLan = cfg.AllowLan
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"mode":       mode,
			"allow-lan":  allowLan,
			"mixed-port": mixed,
			"port":       0,
			"socks-port": 0,
			"redir-port": 0,
			"ipv6":       true,
			"log-level":  "info",
			"running":    s.kernel.IsRunning(),
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, errMsg("method not allowed"))
	}
}

// GET /connections
func (s *APIServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		// 关闭所有连接（简化：我们没有持久连接跟踪，直接返回 OK）
		s.connMu.Lock()
		s.conns = make(map[string]*trackedConn)
		s.connMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	var list []*trackedConn
	for _, c := range s.conns {
		list = append(list, c)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connections":   list,
		"uploadTotal":   0,
		"downloadTotal": 0,
	})
}

// GET /logs — 实时日志（Server-Sent Events）
// 对应 mihomo /logs WebSocket
func (s *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	// SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	// 注册订阅
	ch := make(chan string, 100)
	id := s.logSeq.Add(1)
	s.logMu.Lock()
	s.logSubs[id] = ch
	s.logMu.Unlock()
	defer func() {
		s.logMu.Lock()
		delete(s.logSubs, id)
		s.logMu.Unlock()
	}()

	for {
		select {
		case line := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// GET /traffic — 实时流量统计（SSE）
func (s *APIServer) handleTraffic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			data, _ := json.Marshal(map[string]int64{"up": 0, "down": 0})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ── JSON 辅助 ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func errMsg(msg string) map[string]string {
	return map[string]string{"message": msg}
}
