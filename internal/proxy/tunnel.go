package proxy

// tunnel.go — 核心路由 Tunnel
//
// 参照 mihomo/tunnel/tunnel.go 实现。
// Tunnel 是整个代理内核的核心：
//   - 所有 inbound（HTTP/SOCKS5/Mixed）入站连接交给 Tunnel
//   - Tunnel 按规则匹配选出 outbound（Direct/Proxy）
//   - 建立隧道并双向转发数据

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TunnelMode 代理模式（对应 mihomo tunnel.TunnelMode）
type TunnelMode int32

const (
	ModeRule   TunnelMode = iota // 按规则路由
	ModeGlobal                   // 全部走代理
	ModeDirect                   // 全部直连
)

func (m TunnelMode) String() string {
	switch m {
	case ModeGlobal:
		return "global"
	case ModeDirect:
		return "direct"
	default:
		return "rule"
	}
}

func ParseMode(s string) TunnelMode {
	switch s {
	case "global":
		return ModeGlobal
	case "direct":
		return ModeDirect
	default:
		return ModeRule
	}
}

// Metadata 连接元数据（对应 mihomo/constant.Metadata）
type Metadata struct {
	Network string // "tcp" | "udp"
	SrcIP   net.IP
	SrcPort uint16
	DstHost string // 域名（优先）
	DstIP   net.IP
	DstPort uint16
	Type    string // "HTTP" | "HTTPS" | "SOCKS5" | "SOCKS4"
	Process string // 发起连接的进程名（由 process sniffer 填入）
}

func (m *Metadata) RemoteAddress() string {
	if m.DstHost != "" {
		return fmt.Sprintf("%s:%d", m.DstHost, m.DstPort)
	}
	return fmt.Sprintf("%s:%d", m.DstIP.String(), m.DstPort)
}

// Tunnel 核心路由器（对应 mihomo tunnel.tunnel）
//
// 所有入站连接通过 HandleTCPConn 交入，
// Tunnel 匹配规则选出出口，建立连接后双向 relay 数据。
type Tunnel struct {
	mode    atomic.Int32
	rules   atomic.Value // []Rule
	proxies atomic.Value // map[string]Outbound

	mu     sync.Mutex
	closed bool
}

// NewTunnel 创建 Tunnel 实例
func NewTunnel() *Tunnel {
	t := &Tunnel{}
	t.mode.Store(int32(ModeRule))
	t.rules.Store([]Rule{})
	t.proxies.Store(map[string]Outbound{})
	return t
}

// SetMode 切换路由模式
func (t *Tunnel) SetMode(m TunnelMode) {
	t.mode.Store(int32(m))
}

// Mode 返回当前路由模式
func (t *Tunnel) Mode() TunnelMode {
	return TunnelMode(t.mode.Load())
}

// UpdateRules 原子替换规则列表（对应 mihomo tunnel.UpdateRules）
func (t *Tunnel) UpdateRules(rules []Rule) {
	t.rules.Store(rules)
}

// UpdateProxies 原子替换代理映射（对应 mihomo tunnel.UpdateProxies）
func (t *Tunnel) UpdateProxies(proxies map[string]Outbound) {
	t.proxies.Store(proxies)
}

// HandleTCPConn 处理一条 TCP 入站连接（对应 mihomo tunnel.HandleTCPConn）
//
// 调用路径：Inbound.Accept → HandleTCPConn → match → dial → relay
func (t *Tunnel) HandleTCPConn(conn net.Conn, metadata *Metadata) {
	defer conn.Close()

	// 选出出口代理
	outbound, err := t.pickOutbound(metadata)
	if err != nil {
		log.Printf("[Tunnel] pickOutbound 失败: dst=%s:%d err=%v", metadata.DstHost, metadata.DstPort, err)
		return
	}

	log.Printf("[Tunnel] HandleTCPConn: %s:%d → outbound=%s", metadata.DstHost, metadata.DstPort, outbound.Name())

	// 通过出口代理拨号目标地址
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remote, err := outbound.DialTCP(ctx, metadata)
	if err != nil {
		log.Printf("[Tunnel] DialTCP 失败: dst=%s:%d outbound=%s err=%v", metadata.DstHost, metadata.DstPort, outbound.Name(), err)
		return
	}
	defer remote.Close()

	// 双向中继（对应 mihomo relay/pipe）
	relay(conn, remote)
}

// pickOutbound 根据规则选出出口（对应 mihomo tunnel.match）
func (t *Tunnel) pickOutbound(metadata *Metadata) (Outbound, error) {
	proxies := t.proxies.Load().(map[string]Outbound)

	// 模式覆盖（Global / Direct）
	switch t.Mode() {
	case ModeGlobal:
		if ob, ok := proxies["GLOBAL"]; ok {
			return ob, nil
		}
		// fallthrough to DIRECT
	case ModeDirect:
		if ob, ok := proxies["DIRECT"]; ok {
			return ob, nil
		}
		return &DirectOutbound{}, nil
	}

	// Rule 模式：逐条匹配规则（对应 mihomo tunnel.match 的规则循环）
	rules := t.rules.Load().([]Rule)
	for _, rule := range rules {
		if rule.Match(metadata) {
			target := rule.Adapter()
			if ob, ok := proxies[target]; ok {
				return ob, nil
			}
			// 特殊内置策略
			switch target {
			case "DIRECT":
				return &DirectOutbound{}, nil
			case "REJECT":
				return nil, fmt.Errorf("rejected by rule")
			}
		}
	}

	// 无匹配规则：默认直连
	return &DirectOutbound{}, nil
}

// relay 在两个连接之间双向转发数据，直到任一端关闭
func relay(left, right net.Conn) {
	done := make(chan struct{}, 2)
	copy := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		_ = dst.SetDeadline(time.Now())
		done <- struct{}{}
	}
	go copy(right, left)
	go copy(left, right)
	<-done
}
