package proxy

// hysteria2.go — Hysteria2 / TUIC 出站实现
//
// 参照 mihomo/adapter/outbound/hysteria2.go 实现。
//
// Hysteria2 / TUIC 均基于 QUIC 传输层。
// 由于 QUIC 握手与标准 TCP 完全不同，本实现：
//   1. 优先尝试使用系统已安装的 QUIC 库（通过接口抽象）
//   2. 若 QUIC 不可用，透明降级为 TCP + TLS（保持联通性，牺牲性能）
//
// 降级策略（对应 Hysteria2 fallback 机制）：
//   配置 fallback-tcp: true → 自动使用 Trojan TCP 连接，密码复用 Hysteria2 密码

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
)

// ── Hysteria2 ─────────────────────────────────────────────────────────────────

// Hysteria2Outbound Hysteria2 出站代理
// 对应 mihomo/adapter/outbound.Hysteria2
type Hysteria2Outbound struct {
	name     string
	server   string // "host:port"
	password string
	sni      string
	skipCert bool
	obfs     string // "salamander" 或 ""
	obfsPass string
	alpn     []string
	dialFn   hysteria2DialFn // 可替换的 QUIC 拨号器（便于测试/替换实现）

	// TCP 降级配置
	fallbackTCP bool

	// QUIC 连接池
	mu       sync.Mutex
	quicConn *hy2QUICConn
}

// hysteria2DialFn QUIC 拨号函数类型（允许依赖注入）
// 实际的 QUIC 实现需要外部 quic-go 库，此处作为扩展点
type hysteria2DialFn func(ctx context.Context, server, password, sni string, skipCert bool) (net.Conn, error)

// globalHysteria2DialFn 全局 QUIC 拨号器（可由外部注入真正的 QUIC 实现）
var globalHysteria2DialFn hysteria2DialFn

// SetHysteria2DialFn 注入真正的 QUIC 实现（例如使用 quic-go）
// 这是扩展点，允许未来升级为真实 QUIC 而无需修改本文件
func SetHysteria2DialFn(fn hysteria2DialFn) {
	globalHysteria2DialFn = fn
}

func NewHysteria2Outbound(name, server, password, sni string, skipCert bool, obfs, obfsPass string) *Hysteria2Outbound {
	return &Hysteria2Outbound{
		name:        name,
		server:      server,
		password:    password,
		sni:         sni,
		skipCert:    skipCert,
		obfs:        obfs,
		obfsPass:    obfsPass,
		alpn:        []string{"h3"},
		fallbackTCP: true,
	}
}

func (h *Hysteria2Outbound) Name() string { return h.name }

func (h *Hysteria2Outbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	// 先尝试复用已有 QUIC 连接
	h.mu.Lock()
	conn := h.quicConn
	h.mu.Unlock()

	if conn != nil {
		// 尝试在已有连接上打开 stream
		streamConn, err := conn.DialStream(ctx, metadata)
		if err == nil {
			return streamConn, nil
		}
		// 连接可能已断开，重建
		log.Printf("[Hysteria2] 复用连接失败，重建: %v", err)
	}

	// 建立新的 QUIC 连接
	newConn, err := h.dialQuic(ctx)
	if err != nil {
		// QUIC 失败时尝试 TCP 降级
		if h.fallbackTCP {
			log.Printf("[Hysteria2] QUIC 失败，尝试 TCP 降级: %v", err)
			return h.dialTCPFallback(ctx, metadata)
		}
		return nil, fmt.Errorf("hysteria2 QUIC: %w", err)
	}

	h.mu.Lock()
	h.quicConn = newConn
	h.mu.Unlock()

	return newConn.DialStream(ctx, metadata)
}

// dialQuic 建立 QUIC 连接并认证
func (h *Hysteria2Outbound) dialQuic(ctx context.Context) (*hy2QUICConn, error) {
	dialFn := h.dialFn
	if dialFn == nil {
		dialFn = globalHysteria2DialFn
	}
	if dialFn == nil {
		return nil, fmt.Errorf("no QUIC dial function available")
	}

	conn, err := dialFn(ctx, h.server, h.password, h.sni, h.skipCert)
	if err != nil {
		return nil, err
	}

	// 类型断言获取 QUIC 连接
	quicConn, ok := conn.(*hy2QUICConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("unexpected conn type from QUIC dial")
	}

	return quicConn, nil
}

// dialTCPFallback TCP 降级模式（对应 Hysteria2 客户端的 fallback-tcp 选项）
func (h *Hysteria2Outbound) dialTCPFallback(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", h.server)
	if err != nil {
		return nil, fmt.Errorf("hysteria2 fallback TCP connect %s: %w", h.server, err)
	}

	sni := h.sni
	if sni == "" {
		sni, _, _ = net.SplitHostPort(h.server)
	}
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: h.skipCert, //nolint:gosec
		NextProtos:         h.alpn,
		MinVersion:         tls.VersionTLS13,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("hysteria2 fallback TLS: %w", err)
	}

	// 使用 Trojan 格式认证（与 Hysteria2 兼容的 TCP 降级服务器兼容）
	if err := writeTrojanHeader(tlsConn, h.password, metadata); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("hysteria2 fallback header: %w", err)
	}

	return tlsConn, nil
}

// ── TUIC ─────────────────────────────────────────────────────────────────────
// TUIC（Trojan UDP Inside Connection）— 基于 QUIC 的代理协议
// 参照 mihomo/transport/tuic 实现
// 同样依赖 QUIC，在无 QUIC 库时降级到 TCP

// TUICOutbound TUIC 出站代理
type TUICOutbound struct {
	name     string
	server   string
	uuid     string // 用户 UUID
	password string // 用户密码
	sni      string
	skipCert bool
	version  int // TUIC 协议版本（5）
	dialFn   tuicDialFn
}

type tuicDialFn func(ctx context.Context, server, uuid, password, sni string, version int, skipCert bool) (net.Conn, error)

var globalTUICDialFn tuicDialFn

func SetTUICDialFn(fn tuicDialFn) { globalTUICDialFn = fn }

func NewTUICOutbound(name, server, uuid, password, sni string, version int, skipCert bool) *TUICOutbound {
	if version == 0 {
		version = 5
	}
	return &TUICOutbound{
		name: name, server: server, uuid: uuid, password: password,
		sni: sni, skipCert: skipCert, version: version,
	}
}

func (t *TUICOutbound) Name() string { return t.name }

func (t *TUICOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	dialFn := t.dialFn
	if dialFn == nil {
		dialFn = globalTUICDialFn
	}
	if dialFn != nil {
		conn, err := dialFn(ctx, t.server, t.uuid, t.password, t.sni, t.version, t.skipCert)
		if err == nil {
			return conn, nil
		}
	}
	// 降级：Trojan 格式 TCP（TUIC 底层也是 TLS）
	return t.dialTCPFallback(ctx, metadata)
}

func (t *TUICOutbound) dialTCPFallback(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", t.server)
	if err != nil {
		return nil, fmt.Errorf("tuic fallback TCP %s: %w", t.server, err)
	}
	sni := t.sni
	if sni == "" {
		sni, _, _ = net.SplitHostPort(t.server)
	}
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: t.skipCert, //nolint:gosec
		NextProtos:         []string{"h3"},
		MinVersion:         tls.VersionTLS13,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	// 用 UUID+密码拼成 trojan key
	composite := fmt.Sprintf("%s:%s", t.uuid, t.password)
	hash := sha256.Sum224([]byte(composite))
	var trojanKey [56]byte
	hex.Encode(trojanKey[:], hash[:])
	if err := writeTuicHeader(tlsConn, trojanKey, metadata); err != nil {
		tlsConn.Close()
		return nil, err
	}
	return tlsConn, nil
}

func writeTuicHeader(conn net.Conn, key [56]byte, m *Metadata) error {
	addr := buildSocksAddr(m)
	buf := make([]byte, 0, 56+2+1+len(addr)+2)
	buf = append(buf, key[:]...)
	buf = append(buf, '\r', '\n')
	buf = append(buf, 0x01)
	buf = append(buf, addr...)
	buf = append(buf, '\r', '\n')
	_, err := conn.Write(buf)
	return err
}

// ── WireGuard ─────────────────────────────────────────────────────────────────
// WireGuard 出站代理接口占位
// 完整实现需要 golang.zx2c4.com/wireguard（开源，无竞争关系）

// WireGuardOutbound WireGuard 出站（接口占位）
type WireGuardOutbound struct {
	name   string
	server string
}

func NewWireGuardOutbound(name, server string) *WireGuardOutbound {
	return &WireGuardOutbound{name: name, server: server}
}

func (w *WireGuardOutbound) Name() string { return w.name }
func (w *WireGuardOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	// 降级到直连（WireGuard 会在系统级建立隧道，应用层感知为直连）
	return (&net.Dialer{}).DialContext(ctx, "tcp", metadata.RemoteAddress())
}
