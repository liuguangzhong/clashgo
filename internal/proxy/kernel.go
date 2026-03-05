package proxy

// kernel.go — 代理内核启停入口
//
// 参照 mihomo/hub/hub.go + hub/executor/executor.go 实现。

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// KernelConfig 内核所需的配置
type KernelConfig struct {
	MixedPort          int                      `yaml:"mixed-port"`
	Port               int                      `yaml:"port"`
	SocksPort          int                      `yaml:"socks-port"`
	Mode               string                   `yaml:"mode"`
	LogLevel           string                   `yaml:"log-level"`
	AllowLan           bool                     `yaml:"allow-lan"`
	ExternalController string                   `yaml:"external-controller"`
	Secret             string                   `yaml:"secret"`
	GeoIPPath          string                   `yaml:"geoip-mmdb"`
	DNS                kernelDNSConfig          `yaml:"dns"`
	TUN                TUNConfig                `yaml:"tun"`
	Proxies            []map[string]interface{} `yaml:"proxies"`
	ProxyGroups        []map[string]interface{} `yaml:"proxy-groups"`
	Rules              []string                 `yaml:"rules"`
}

type kernelDNSConfig struct {
	Enable       bool     `yaml:"enable"`
	Nameservers  []string `yaml:"nameserver"`
	DoH          []string `yaml:"doh-servers"`
	FakeIPRange  string   `yaml:"fake-ip-range"`
	EnhancedMode string   `yaml:"enhanced-mode"`
}

// ParseConfig 将 YAML 字节解析为 KernelConfig（供 manager.go 调用）
func ParseConfig(data []byte) (*KernelConfig, error) {
	return parseKernelConfig(data)
}

// Kernel 自实现的代理内核实例
type Kernel struct {
	mu         sync.Mutex
	tunnel     *Tunnel
	listeners  []*MixedListener
	apiServer  *APIServer
	tunManager *TUNManager
	running    bool
	log        *zap.Logger
	currentCfg *KernelConfig // 最近一次成功应用的配置
}

// GetConfig 返回当前运行配置（只读）
func (k *Kernel) GetConfig() *KernelConfig {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.currentCfg
}

var (
	globalKernel     *Kernel
	globalKernelOnce sync.Once
)

// GlobalKernel 返回全局内核单例
func GlobalKernel() *Kernel {
	globalKernelOnce.Do(func() {
		globalKernel = &Kernel{
			tunnel: NewTunnel(),
			log:    zap.NewNop(),
		}
	})
	return globalKernel
}

func (k *Kernel) SetLogger(log *zap.Logger) { k.log = log }

// Parse 解析配置字节并启动内核
func Parse(configBytes []byte, log *zap.Logger) error {
	k := GlobalKernel()
	k.SetLogger(log)
	cfg, err := parseKernelConfig(configBytes)
	if err != nil {
		return fmt.Errorf("parse kernel config: %w", err)
	}
	return k.ApplyConfig(cfg)
}

// ApplyConfig 热加载配置
//
// 顺序：DNS → GeoIP → Proxies+Groups → Rules → Tunnel → Listeners（端口变化时） → API Server（端口变化时）
func (k *Kernel) ApplyConfig(cfg *KernelConfig) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.log.Info("Applying kernel config",
		zap.String("mode", cfg.Mode),
		zap.Int("mixed-port", cfg.MixedPort),
		zap.Int("rules", len(cfg.Rules)),
		zap.Int("proxies", len(cfg.Proxies)),
	)

	// ── 1. DNS ───────────────────────────────────────────────────────────────
	if cfg.DNS.Enable {
		ns := cfg.DNS.Nameservers
		if len(ns) == 0 {
			ns = []string{"8.8.8.8:53", "1.1.1.1:53"}
		}
		fakeIPSubnet := ""
		if cfg.DNS.EnhancedMode == "fake-ip" {
			fakeIPSubnet = cfg.DNS.FakeIPRange
		}
		var r *Resolver
		var err error
		if len(cfg.DNS.DoH) > 0 {
			rdoh, e := NewResolverWithDoH(ns, cfg.DNS.DoH, fakeIPSubnet)
			if e == nil {
				SetGlobalResolver(rdoh.Resolver)
			}
			err = e
		} else {
			r, err = NewResolver(ns, fakeIPSubnet)
			if err == nil {
				SetGlobalResolver(r)
			}
		}
		if err != nil {
			k.log.Warn("DNS resolver init failed", zap.Error(err))
		} else {
			k.log.Info("DNS resolver initialized",
				zap.Strings("nameservers", ns),
				zap.Bool("fake-ip", fakeIPSubnet != ""),
			)
		}
	}

	// ── 2. GeoIP DB ───────────────────────────────────────────────────────────
	if cfg.GeoIPPath != "" {
		if err := LoadGeoIPDB(cfg.GeoIPPath); err != nil {
			k.log.Warn("GeoIP DB load failed", zap.Error(err))
		} else {
			k.log.Info("GeoIP DB loaded", zap.String("path", cfg.GeoIPPath))
		}
	}

	// ── 3. 代理映射（个体代理 + 代理组）─────────────────────────────────────
	proxies, err := buildProxies(cfg.Proxies, cfg.ProxyGroups)
	if err != nil {
		return fmt.Errorf("build proxies: %w", err)
	}
	proxies["DIRECT"] = &DirectOutbound{}
	proxies["REJECT"] = &RejectOutbound{}

	// ── 4. 规则列表 ──────────────────────────────────────────────────────────
	rules, err := buildRules(cfg.Rules)
	if err != nil {
		return fmt.Errorf("build rules: %w", err)
	}

	// ── 5. 更新 Tunnel ───────────────────────────────────────────────────────
	k.tunnel.SetMode(ParseMode(cfg.Mode))
	k.tunnel.UpdateProxies(proxies)
	k.tunnel.UpdateRules(rules)

	// ── 6. 重建监听器（仅端口/bind 变化时）────────────────────────────────
	listenerChanged := k.currentCfg == nil ||
		k.currentCfg.MixedPort != cfg.MixedPort ||
		k.currentCfg.Port != cfg.Port ||
		k.currentCfg.SocksPort != cfg.SocksPort ||
		k.currentCfg.AllowLan != cfg.AllowLan
	if listenerChanged {
		if err := k.updateListeners(cfg); err != nil {
			return fmt.Errorf("update listeners: %w", err)
		}
	} else {
		k.log.Info("Listeners unchanged, skipping restart")
	}

	// ── 7. API Server（仅首次启动或地址变化时）───────────────────────────────
	apiChanged := k.currentCfg == nil ||
		k.currentCfg.ExternalController != cfg.ExternalController ||
		k.currentCfg.Secret != cfg.Secret
	if cfg.ExternalController != "" && apiChanged {
		if k.apiServer != nil {
			k.apiServer.Stop()
		}
		k.apiServer = NewAPIServer(k, cfg.Secret)
		if err := k.apiServer.Start(cfg.ExternalController); err != nil {
			k.log.Warn("API server start failed", zap.Error(err))
		} else {
			k.log.Info("API server started", zap.String("addr", cfg.ExternalController))
		}
	} else if cfg.ExternalController != "" && k.apiServer == nil {
		// 首次启动但标记未变
		k.apiServer = NewAPIServer(k, cfg.Secret)
		if err := k.apiServer.Start(cfg.ExternalController); err != nil {
			k.log.Warn("API server start failed", zap.Error(err))
		} else {
			k.log.Info("API server started", zap.String("addr", cfg.ExternalController))
		}
	} else {
		k.log.Info("API server unchanged, skipping restart")
	}

	// ── 8. TUN 透明代理 ───────────────────────────────────────────────────────
	if cfg.TUN.Enable {
		if k.tunManager != nil {
			k.tunManager.Stop()
		}
		k.tunManager = NewTUNManager(&cfg.TUN, k.tunnel)
		if err := k.tunManager.Start(); err != nil {
			k.log.Warn("TUN start failed", zap.Error(err))
		} else {
			k.log.Info("TUN started")
		}
	}

	k.running = true
	k.currentCfg = cfg
	k.log.Info("Kernel config applied successfully")
	return nil
}

// Stop 关闭内核
func (k *Kernel) Stop() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.closeListeners()
	if k.tunManager != nil {
		k.tunManager.Stop()
		k.tunManager = nil
	}
	if k.apiServer != nil {
		k.apiServer.Stop()
		k.apiServer = nil
	}
	k.running = false
	k.log.Info("Kernel stopped")
}

func (k *Kernel) IsRunning() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.running
}

func (k *Kernel) Tunnel() *Tunnel { return k.tunnel }

// ── 内部实现 ──────────────────────────────────────────────────────────────────

func parseKernelConfig(data []byte) (*KernelConfig, error) {
	var cfg KernelConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Mode == "" {
		cfg.Mode = "rule"
	}
	return &cfg, nil
}

// buildProxies 构造个体代理 + 代理组
func buildProxies(rawProxies []map[string]interface{}, rawGroups []map[string]interface{}) (map[string]Outbound, error) {
	result := make(map[string]Outbound)

	// 第一轮：个体代理
	for _, p := range rawProxies {
		name, _ := p["name"].(string)
		typ, _ := p["type"].(string)
		if name == "" || typ == "" {
			continue
		}
		var ob Outbound
		switch strings.ToLower(typ) {
		case "socks5":
			server, _ := p["server"].(string)
			port, user, pass := toInt(p["port"]), strVal(p, "username"), strVal(p, "password")
			ob = NewSocks5Outbound(name, fmt.Sprintf("%s:%d", server, port), user, pass)
		case "http":
			server, _ := p["server"].(string)
			port, user, pass := toInt(p["port"]), strVal(p, "username"), strVal(p, "password")
			ob = NewHttpOutbound(name, fmt.Sprintf("%s:%d", server, port), user, pass)
		case "ss", "shadowsocks":
			server, _ := p["server"].(string)
			port := toInt(p["port"])
			password := strVal(p, "password")
			cipher := strVal(p, "cipher")
			if cipher == "" {
				cipher = "aes-256-gcm"
			}
			ob = NewShadowsocksOutbound(name, fmt.Sprintf("%s:%d", server, port), password, cipher)
		case "trojan":
			server, _ := p["server"].(string)
			port := toInt(p["port"])
			password := strVal(p, "password")
			sni := strVal(p, "sni")
			skipCert, _ := p["skip-cert-verify"].(bool)
			ob = NewTrojanOutbound(name, fmt.Sprintf("%s:%d", server, port), password, sni, skipCert)
		case "vmess":
			server, _ := p["server"].(string)
			port := toInt(p["port"])
			uuid := strVal(p, "uuid")
			altID := toInt(p["alterId"])
			cipher := strVal(p, "cipher")
			if cipher == "" {
				cipher = "auto"
			}
			useTLS, _ := p["tls"].(bool)
			sni := strVal(p, "servername")
			skipCert, _ := p["skip-cert-verify"].(bool)
			ob = NewVMessOutbound(name, fmt.Sprintf("%s:%d", server, port), uuid, cipher, altID, useTLS, sni, skipCert)

		case "vless":
			server, _ := p["server"].(string)
			port := toInt(p["port"])
			uuid := strVal(p, "uuid")
			flow := strVal(p, "flow")
			useTLS, _ := p["tls"].(bool)
			sni := strVal(p, "servername")
			skipCert, _ := p["skip-cert-verify"].(bool)
			ob = NewVLESSOutbound(name, fmt.Sprintf("%s:%d", server, port), uuid, flow, useTLS, sni, skipCert)

		case "hysteria2", "hy2":
			server, _ := p["server"].(string)
			port := toInt(p["port"])
			password := strVal(p, "password")
			sni := strVal(p, "sni")
			skipCert, _ := p["skip-cert-verify"].(bool)
			obfs := strVal(p, "obfs")
			obfsPass := strVal(p, "obfs-password")
			ob = NewHysteria2Outbound(name, fmt.Sprintf("%s:%d", server, port), password, sni, skipCert, obfs, obfsPass)

		case "tuic":
			server, _ := p["server"].(string)
			port := toInt(p["port"])
			uuid := strVal(p, "uuid")
			password := strVal(p, "password")
			sni := strVal(p, "sni")
			skipCert, _ := p["skip-cert-verify"].(bool)
			version := toInt(p["version"])
			ob = NewTUICOutbound(name, fmt.Sprintf("%s:%d", server, port), uuid, password, sni, version, skipCert)

		case "ssh":
			server, _ := p["server"].(string)
			port := toInt(p["port"])
			username := strVal(p, "username")
			password := strVal(p, "password")
			privKey := strVal(p, "private-key")
			ob = NewSSHOutbound(name, fmt.Sprintf("%s:%d", server, port), username, password, privKey)

		case "wireguard", "wg":
			server, _ := p["server"].(string)
			port := toInt(p["port"])
			ob = NewWireGuardOutbound(name, fmt.Sprintf("%s:%d", server, port))

		default:
			ob = &DirectOutbound{}
		}
		result[name] = ob
	}

	// 第二轮：代理组（可以引用第一轮结果）
	for _, g := range rawGroups {
		name, _ := g["name"].(string)
		typ, _ := g["type"].(string)
		if name == "" || typ == "" {
			continue
		}
		proxiesRaw, _ := g["proxies"].([]interface{})
		var members []Outbound
		for _, pn := range proxiesRaw {
			pname, _ := pn.(string)
			if ob, ok := result[pname]; ok {
				members = append(members, ob)
			}
		}
		if len(members) == 0 {
			continue
		}
		testURL := strVal(g, "url")
		if testURL == "" {
			testURL = "http://www.gstatic.com/generate_204"
		}
		intervalSec := toInt(g["interval"])
		if intervalSec == 0 {
			intervalSec = 300
		}
		interval := time.Duration(intervalSec) * time.Second

		switch strings.ToLower(typ) {
		case "select", "selector":
			result[name] = NewSelectGroup(name, members)
		case "url-test":
			tol := time.Duration(toInt(g["tolerance"])) * time.Millisecond
			if tol == 0 {
				tol = 150 * time.Millisecond
			}
			grp := NewURLTestGroup(name, members, testURL, interval, tol)
			grp.Start()
			result[name] = grp
		case "fallback":
			result[name] = NewFallbackGroup(name, members, testURL, interval)
		case "load-balance":
			result[name] = NewLoadBalanceGroup(name, members)
		default:
			result[name] = NewSelectGroup(name, members)
		}
	}

	return result, nil
}

// buildRules 从字符串列表解析规则
func buildRules(rawRules []string) ([]Rule, error) {
	var rules []Rule
	for _, r := range rawRules {
		rule, err := ParseRule(r, nil)
		if err != nil || rule == nil {
			continue
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (k *Kernel) updateListeners(cfg *KernelConfig) error {
	k.closeListeners()
	bindHost := "127.0.0.1"
	if cfg.AllowLan {
		bindHost = "0.0.0.0"
	}
	if cfg.MixedPort > 0 {
		addr := fmt.Sprintf("%s:%d", bindHost, cfg.MixedPort)
		ln, err := NewMixedListener(addr, k.tunnel)
		if err != nil {
			return fmt.Errorf("start mixed listener on %s: %w", addr, err)
		}
		k.listeners = append(k.listeners, ln)
		k.log.Info("Mixed listener started", zap.String("addr", ln.Address()))
	}
	if cfg.Port > 0 && cfg.Port != cfg.MixedPort {
		addr := fmt.Sprintf("%s:%d", bindHost, cfg.Port)
		ln, err := NewMixedListener(addr, k.tunnel)
		if err != nil {
			return fmt.Errorf("start HTTP listener on %s: %w", addr, err)
		}
		k.listeners = append(k.listeners, ln)
		k.log.Info("HTTP listener started", zap.String("addr", ln.Address()))
	}
	if cfg.SocksPort > 0 && cfg.SocksPort != cfg.MixedPort {
		addr := fmt.Sprintf("%s:%d", bindHost, cfg.SocksPort)
		ln, err := NewMixedListener(addr, k.tunnel)
		if err != nil {
			return fmt.Errorf("start SOCKS listener on %s: %w", addr, err)
		}
		k.listeners = append(k.listeners, ln)
		k.log.Info("SOCKS listener started", zap.String("addr", ln.Address()))
	}
	if len(k.listeners) == 0 {
		k.log.Warn("No listeners configured")
	}
	return nil
}

func (k *Kernel) closeListeners() {
	for _, ln := range k.listeners {
		_ = ln.Close()
	}
	k.listeners = nil
}

// ── 辅助 ──────────────────────────────────────────────────────────────────────

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func strVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func lookupHost(host string) net.IP {
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return nil
	}
	return net.ParseIP(addrs[0])
}

// wrapTLSClient 包装 TLS 连接并完成握手
func wrapTLSClient(conn net.Conn, _ interface{}, sni, server string, skipCert bool) (net.Conn, error) {
	if sni == "" {
		sni, _, _ = net.SplitHostPort(server)
	}
	tlsConn := tlsClientConn(conn, sni, skipCert)
	if err := tlsConn.Handshake(); err != nil {
		return nil, fmt.Errorf("TLS handshake: %w", err)
	}
	return tlsConn, nil
}
