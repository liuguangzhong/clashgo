package proxy

// dns.go — 内置 DNS 解析器 + Fake-IP
//
// 参照 mihomo/dns/resolver.go + component/fakeip 实现。
//
// 支持：
//   - 系统 DNS / 自定义上游 DNS（UDP 53）
//   - Fake-IP 模式：为每个域名分配假 IP，tunnel 通过反查还原域名
//   - DoH（DNS over HTTPS）上游（可选）

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

// ── Fake-IP 池 ────────────────────────────────────────────────────────────────
// 参照 mihomo/component/fakeip/pool.go
// 在 198.18.0.0/15 地址段分配假 IP（IANA 保留地址，不会与真实公网 IP 冲突）

type fakeIPPool struct {
	mu       sync.Mutex
	subnet   *net.IPNet
	host2IP  map[string]net.IP // domain → fakeIP
	ip2Host  map[string]string // fakeIP.String() → domain
	next     uint32            // 下一个可用 IP 偏移
	capacity uint32
}

// newFakeIPPool 创建 Fake-IP 地址池
// subnet 默认使用 IANA 保留的 198.18.0.0/15
func newFakeIPPool(subnet string) (*fakeIPPool, error) {
	if subnet == "" {
		subnet = "198.18.0.0/15"
	}
	_, network, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("parse fake-ip subnet %s: %w", subnet, err)
	}

	// 计算地址容量
	ones, bits := network.Mask.Size()
	capacity := uint32(1) << uint(bits-ones)
	if capacity > 2 {
		capacity -= 2 // 去掉网络地址和广播地址
	}

	return &fakeIPPool{
		subnet:   network,
		host2IP:  make(map[string]net.IP),
		ip2Host:  make(map[string]string),
		capacity: capacity,
		next:     1, // 从第一个可用 IP 开始（.1）
	}, nil
}

// Lookup 为域名分配或查找 Fake-IP
func (p *fakeIPPool) Lookup(host string) net.IP {
	host = strings.ToLower(host)
	p.mu.Lock()
	defer p.mu.Unlock()

	if ip, ok := p.host2IP[host]; ok {
		return ip
	}

	// 分配新 Fake-IP
	ip := p.allocate()
	p.host2IP[host] = ip
	p.ip2Host[ip.String()] = host
	return ip
}

// Reverse 通过 Fake-IP 反查域名
func (p *fakeIPPool) Reverse(ip net.IP) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	host, ok := p.ip2Host[ip.String()]
	return host, ok
}

// IsFakeIP 判断 IP 是否属于 Fake-IP 段
func (p *fakeIPPool) IsFakeIP(ip net.IP) bool {
	return p.subnet.Contains(ip)
}

func (p *fakeIPPool) allocate() net.IP {
	// 循环分配（对应 mihomo/component/fakeip 的 next 指针回绕）
	if p.next >= p.capacity {
		p.next = 1
		// 清空旧映射（简化实现）
		p.host2IP = make(map[string]net.IP)
		p.ip2Host = make(map[string]string)
	}
	baseIP := p.subnet.IP
	offset := p.next
	p.next++

	ip := make(net.IP, 4)
	base := ipToUint32(baseIP.To4())
	binary.BigEndian.PutUint32(ip, base+offset)
	return ip
}

func ipToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip)
}

// ── DNS 解析器 ────────────────────────────────────────────────────────────────
// 参照 mihomo/dns/resolver.go

// Resolver 自实现 DNS 解析器
type Resolver struct {
	mu          sync.RWMutex
	nameservers []string    // 上游 DNS 服务器列表（"8.8.8.8:53"）
	fakeIPPool  *fakeIPPool // Fake-IP 池（nil = 禁用）
	cache       map[string]dnsEntry
	cacheTTL    time.Duration

	hosts map[string]net.IP // 静态 hosts 映射
}

type dnsEntry struct {
	ips     []net.IP
	expires time.Time
}

// globalResolver 全局 DNS 解析器单例
var globalResolver *Resolver

// NewResolver 创建 DNS 解析器
// nameservers: 上游 DNS 服务器，如 ["8.8.8.8:53", "1.1.1.1:53"]
// fakeIPSubnet: Fake-IP 子网，空字符串禁用
func NewResolver(nameservers []string, fakeIPSubnet string) (*Resolver, error) {
	r := &Resolver{
		nameservers: nameservers,
		cache:       make(map[string]dnsEntry),
		cacheTTL:    30 * time.Minute,
		hosts:       make(map[string]net.IP),
	}
	if fakeIPSubnet != "" {
		pool, err := newFakeIPPool(fakeIPSubnet)
		if err != nil {
			return nil, err
		}
		r.fakeIPPool = pool
	}
	globalResolver = r
	return r, nil
}

// SetGlobalResolver 设置全局解析器
func SetGlobalResolver(r *Resolver) {
	globalResolver = r
}

// GlobalResolver 返回全局解析器
func GlobalResolver() *Resolver {
	return globalResolver
}

// ResolveFakeIP 为域名分配 Fake-IP（Fake-IP 模式）
// 对应 mihomo/component/fakeip Pool.Lookup
func (r *Resolver) ResolveFakeIP(host string) net.IP {
	if r.fakeIPPool == nil {
		return nil
	}
	return r.fakeIPPool.Lookup(host)
}

// ReverseFakeIP 反查 Fake-IP 对应域名
func (r *Resolver) ReverseFakeIP(ip net.IP) (string, bool) {
	if r.fakeIPPool == nil {
		return "", false
	}
	return r.fakeIPPool.Reverse(ip)
}

// IsFakeIP 判断是否 Fake-IP
func (r *Resolver) IsFakeIP(ip net.IP) bool {
	if r.fakeIPPool == nil {
		return false
	}
	return r.fakeIPPool.IsFakeIP(ip)
}

// Resolve 解析域名，返回第一个 IPv4 地址
// 优先级：hosts 静态映射 > DNS 缓存 > 上游 DNS
func (r *Resolver) Resolve(ctx context.Context, host string) (net.IP, error) {
	host = strings.ToLower(host)

	// 1. hosts 静态映射
	if ip, ok := r.hosts[host]; ok {
		return ip, nil
	}

	// 2. DNS 缓存
	r.mu.RLock()
	entry, cached := r.cache[host]
	r.mu.RUnlock()
	if cached && time.Now().Before(entry.expires) {
		if len(entry.ips) > 0 {
			return entry.ips[rand.Intn(len(entry.ips))], nil //nolint:gosec
		}
	}

	// 3. 上游 DNS 查询
	ips, ttl, err := r.queryUpstream(ctx, host)
	if err != nil {
		// fallback: 系统 DNS
		fallback, fe := net.DefaultResolver.LookupHost(ctx, host)
		if fe != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
		for _, s := range fallback {
			if ip := net.ParseIP(s); ip != nil {
				if ip4 := ip.To4(); ip4 != nil {
					ips = append(ips, ip4)
				}
			}
		}
		ttl = r.cacheTTL
	}

	// 缓存结果
	if len(ips) > 0 {
		r.mu.Lock()
		r.cache[host] = dnsEntry{ips: ips, expires: time.Now().Add(ttl)}
		r.mu.Unlock()
		return ips[rand.Intn(len(ips))], nil //nolint:gosec
	}
	return nil, fmt.Errorf("no A record for %s", host)
}

// queryUpstream 通过 UDP 查询上游 DNS（对应 mihomo/dns/client.go）
func (r *Resolver) queryUpstream(ctx context.Context, host string) ([]net.IP, time.Duration, error) {
	if len(r.nameservers) == 0 {
		return nil, 0, fmt.Errorf("no nameservers configured")
	}

	// 随机选一个上游服务器（简单负载均衡）
	ns := r.nameservers[rand.Intn(len(r.nameservers))] //nolint:gosec

	// 构造 DNS 查询报文（Type A）
	query := buildDNSQuery(host)

	// UDP 查询
	conn, err := net.DialTimeout("udp", ns, 5*time.Second)
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()

	deadline, ok := ctx.Deadline()
	if ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	if _, err := conn.Write(query); err != nil {
		return nil, 0, err
	}

	resp := make([]byte, 512)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, 0, err
	}

	return parseDNSResponse(resp[:n])
}

// ── DNS 报文构造/解析（极简实现）─────────────────────────────────────────────
// 完整实现可用 golang.org/x/net/dns/dnsmessage 或 github.com/miekg/dns
// 这里手动实现基本的 A 记录查询，避免引入额外依赖

// buildDNSQuery 构造 DNS Type-A 查询报文
func buildDNSQuery(host string) []byte {
	var buf []byte

	// Header
	buf = append(buf, 0x12, 0x34) // Transaction ID
	buf = append(buf, 0x01, 0x00) // Flags: Standard query, Recursion Desired
	buf = append(buf, 0x00, 0x01) // Questions: 1
	buf = append(buf, 0x00, 0x00) // Answer RRs: 0
	buf = append(buf, 0x00, 0x00) // Authority RRs: 0
	buf = append(buf, 0x00, 0x00) // Additional RRs: 0

	// Question: QNAME
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	for _, label := range labels {
		buf = append(buf, byte(len(label)))
		buf = append(buf, []byte(label)...)
	}
	buf = append(buf, 0x00) // Root label

	buf = append(buf, 0x00, 0x01) // QTYPE: A
	buf = append(buf, 0x00, 0x01) // QCLASS: IN
	return buf
}

// parseDNSResponse 解析 DNS 响应中的 A 记录
func parseDNSResponse(resp []byte) ([]net.IP, time.Duration, error) {
	if len(resp) < 12 {
		return nil, 0, fmt.Errorf("dns response too short")
	}

	// 检查响应码
	flags := binary.BigEndian.Uint16(resp[2:4])
	rcode := flags & 0x000F
	if rcode != 0 {
		return nil, 0, fmt.Errorf("dns rcode: %d", rcode)
	}

	anCount := int(binary.BigEndian.Uint16(resp[6:8]))
	if anCount == 0 {
		return nil, 0, fmt.Errorf("no answers")
	}

	// 跳过 Question 段：从第 12 字节开始跳过 QNAME+QTYPE+QCLASS
	pos := 12
	pos = skipDNSName(resp, pos)
	pos += 4 // QTYPE + QCLASS

	var ips []net.IP
	var minTTL time.Duration = 300 * time.Second

	// 解析 Answer 段
	for i := 0; i < anCount && pos < len(resp); i++ {
		pos = skipDNSName(resp, pos)
		if pos+10 > len(resp) {
			break
		}
		rrType := binary.BigEndian.Uint16(resp[pos : pos+2])
		ttl := time.Duration(binary.BigEndian.Uint32(resp[pos+4:pos+8])) * time.Second
		rdLen := int(binary.BigEndian.Uint16(resp[pos+8 : pos+10]))
		pos += 10

		if rrType == 1 && rdLen == 4 && pos+4 <= len(resp) {
			// Type A
			ip := make(net.IP, 4)
			copy(ip, resp[pos:pos+4])
			ips = append(ips, ip)
			if ttl < minTTL {
				minTTL = ttl
			}
		}
		pos += rdLen
	}

	if len(ips) == 0 {
		return nil, 0, fmt.Errorf("no A records in response")
	}
	return ips, minTTL, nil
}

// skipDNSName 跳过 DNS 报文中的 NAME 字段（处理压缩指针）
func skipDNSName(buf []byte, pos int) int {
	for pos < len(buf) {
		length := int(buf[pos])
		if length == 0 {
			return pos + 1
		}
		if length&0xC0 == 0xC0 {
			// 压缩指针（2字节）
			return pos + 2
		}
		pos += 1 + length
	}
	return pos
}

// AddHost 添加静态 hosts 条目
func (r *Resolver) AddHost(host string, ip net.IP) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hosts[strings.ToLower(host)] = ip
}
