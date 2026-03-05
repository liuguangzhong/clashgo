package proxy

// socks5.go — SOCKS4/SOCKS5 入站代理处理
//
// 参照 mihomo/transport/socks5 和 socks4 实现。
// 处理来自客户端应用的 SOCKS4/5 入站连接，
// 解析握手后把目标地址封装成 Metadata 交给 Tunnel 路由。

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// ── SOCKS5 入站处理（对应 mihomo/transport/socks5 服务端）────────────────────

const socks5Version = 0x05

// handleSocks5 处理 SOCKS5 握手并将连接交给 Tunnel
func handleSocks5(conn net.Conn, tunnel *Tunnel) {
	defer func() {
		// 若 Tunnel.HandleTCPConn 未接管，确保关闭
		// （HandleTCPConn 内部关闭连接；此处 recover 防止 panic）
		_ = recover()
	}()

	if err := socks5ServerHandshake(conn, tunnel); err != nil {
		conn.Close()
	}
}

// socks5ServerHandshake 执行完整 SOCKS5 服务端握手（RFC 1928）
func socks5ServerHandshake(conn net.Conn, tunnel *Tunnel) error {
	// ── 1. 读取客户端问候 ─────────────────────────────────
	// VER NMETHODS METHODS...
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != socks5Version {
		return fmt.Errorf("unsupported SOCKS version: %d", header[0])
	}
	nmethods := int(header[1])
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	// ── 2. 回复：选择无认证（简化实现，不支持 user/pass 入站）────
	// 完整实现需检查 methods 列表并配置认证
	if _, err := conn.Write([]byte{socks5Version, 0x00}); err != nil {
		return err
	}

	// ── 3. 读取 CONNECT 请求 ──────────────────────────────
	// VER CMD RSV ATYP [DST.ADDR] DST.PORT
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, reqHeader); err != nil {
		return err
	}
	if reqHeader[1] != 0x01 { // 仅支持 CONNECT（0x01）
		// 发送失败响应后关闭
		_, _ = conn.Write([]byte{socks5Version, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return fmt.Errorf("only CONNECT supported, got CMD=0x%02x", reqHeader[1])
	}

	metadata, err := parseSocks5Address(conn, reqHeader[3])
	if err != nil {
		return err
	}
	metadata.Type = "SOCKS5"
	metadata.Network = "tcp"

	// ── 4. 回复成功（0x00）───────────────────────────────
	// VER REP RSV ATYP BND.ADDR BND.PORT
	// BND = 0.0.0.0:0（表示不绑定特定地址）
	reply := []byte{socks5Version, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := conn.Write(reply); err != nil {
		return err
	}

	// ── 5. 交给 Tunnel 路由 ──────────────────────────────
	tunnel.HandleTCPConn(conn, metadata)
	return nil
}

// parseSocks5Address 根据 ATYP 读取 DST.ADDR 和 DST.PORT
func parseSocks5Address(conn net.Conn, atyp byte) (*Metadata, error) {
	m := &Metadata{}
	switch atyp {
	case 0x01: // IPv4
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return nil, err
		}
		m.DstIP = net.IP(addr)

	case 0x03: // 域名
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return nil, err
		}
		domainBuf := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(conn, domainBuf); err != nil {
			return nil, err
		}
		m.DstHost = string(domainBuf)

	case 0x04: // IPv6
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return nil, err
		}
		m.DstIP = net.IP(addr)

	default:
		return nil, fmt.Errorf("unsupported ATYP: 0x%02x", atyp)
	}

	// 读端口（2字节大端）
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return nil, err
	}
	m.DstPort = binary.BigEndian.Uint16(portBuf)
	return m, nil
}

// ── SOCKS4 入站处理（对应 mihomo/transport/socks4 服务端）────────────────────

const socks4Version = 0x04

// handleSocks4 处理 SOCKS4/4a 握手
func handleSocks4(conn net.Conn, tunnel *Tunnel) {
	defer func() { _ = recover() }()

	if err := socks4ServerHandshake(conn, tunnel); err != nil {
		conn.Close()
	}
}

// socks4ServerHandshake SOCKS4/4a 服务端握手
//
// 格式（SOCKS4）：VER=0x04 CMD DSTPORT DSTIP [USERID]\0
// 格式（SOCKS4a）：VER=0x04 CMD DSTPORT 0.0.0.x USERID\0 HOSTNAME\0
func socks4ServerHandshake(conn net.Conn, tunnel *Tunnel) error {
	// 已读了首字节(0x04) via Peek；此处重新读完整头部
	header := make([]byte, 7) // CMD(1) + PORT(2) + IP(4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	cmd := header[0]
	if cmd != 0x01 { // 仅支持 CONNECT
		socks4Reply(conn, false)
		return fmt.Errorf("SOCKS4: only CONNECT supported")
	}

	port := binary.BigEndian.Uint16(header[1:3])
	ip := net.IP(header[3:7])

	// 吃掉 USERID（以 \0 结尾）
	if _, err := readUntilNull(conn); err != nil {
		return err
	}

	var host string
	// SOCKS4a：IP = 0.0.0.x（x != 0）→ 后面跟域名
	if ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] != 0 {
		domain, err := readUntilNull(conn)
		if err != nil {
			return err
		}
		host = string(domain)
		ip = nil
	}

	socks4Reply(conn, true)

	metadata := &Metadata{
		Network: "tcp",
		Type:    "SOCKS4",
		DstHost: host,
		DstIP:   ip,
		DstPort: port,
	}
	tunnel.HandleTCPConn(conn, metadata)
	return nil
}

// socks4Reply 发送 SOCKS4 响应
func socks4Reply(conn net.Conn, success bool) {
	rep := byte(0x5A) // granted
	if !success {
		rep = 0x5B // rejected
	}
	_, _ = conn.Write([]byte{0x00, rep, 0, 0, 0, 0, 0, 0})
}

// readUntilNull 从连接中读取到 \0 结束的字节序列
func readUntilNull(conn net.Conn) ([]byte, error) {
	var result []byte
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			return result, err
		}
		if buf[0] == 0x00 {
			return result, nil
		}
		result = append(result, buf[0])
	}
}

// ── 辅助 ──────────────────────────────────────────────────────────────────────

// contextWithTimeout30s 创建 30s 超时 context（HTTP/SOCKS 出站拨号用）
func contextWithTimeout30s() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second) //nolint:govet
	return ctx
}

// hostPortSplit 拆分 host:port，处理无端口情况
func hostPortSplit(addr string, defaultPort uint16) (host string, port uint16) {
	if strings.Contains(addr, ":") {
		h, p, err := net.SplitHostPort(addr)
		if err == nil {
			port64, _ := binary.BigEndian.Uint16([]byte{0, 0}), uint64(0)
			_ = port64
			var pp uint64
			fmt.Sscanf(p, "%d", &pp)
			return h, uint16(pp)
		}
	}
	return addr, defaultPort
}
