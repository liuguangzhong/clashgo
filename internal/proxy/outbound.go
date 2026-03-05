package proxy

// outbound.go — 出口代理实现
//
// 参照 mihomo/adapter/outbound 实现 Direct / REJECT / SOCKS5 / HTTP 出口。

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// Outbound 出口代理接口（对应 mihomo/constant.Proxy）
type Outbound interface {
	Name() string
	DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error)
}

// ── DirectOutbound：直连 ────────────────────────────────────────────────────

type DirectOutbound struct{}

func (d *DirectOutbound) Name() string { return "DIRECT" }
func (d *DirectOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", metadata.RemoteAddress())
}

// ── RejectOutbound：拒绝 ────────────────────────────────────────────────────

type RejectOutbound struct{}

func (r *RejectOutbound) Name() string { return "REJECT" }
func (r *RejectOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	return nil, fmt.Errorf("connection rejected")
}

// ── Socks5Outbound：SOCKS5 上游代理 ──────────────────────────────────────────
//
// 实现 RFC 1928 SOCKS5 握手

type Socks5Outbound struct {
	name     string
	server   string
	username string
	password string
}

func NewSocks5Outbound(name, server, username, password string) *Socks5Outbound {
	return &Socks5Outbound{name: name, server: server, username: username, password: password}
}

func (s *Socks5Outbound) Name() string { return s.name }

func (s *Socks5Outbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.server)
	if err != nil {
		return nil, fmt.Errorf("connect to socks5 server %s: %w", s.server, err)
	}
	if err := s.handshake(conn, metadata); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (s *Socks5Outbound) handshake(conn net.Conn, metadata *Metadata) error {
	// Phase 1: 方法协商
	var methods []byte
	if s.username != "" {
		methods = []byte{0x05, 0x02, 0x00, 0x02}
	} else {
		methods = []byte{0x05, 0x01, 0x00}
	}
	if _, err := conn.Write(methods); err != nil {
		return fmt.Errorf("send methods: %w", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("read method response: %w", err)
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("invalid socks version: %d", resp[0])
	}
	switch resp[1] {
	case 0x00: // no auth
	case 0x02:
		if err := s.authUserPass(conn); err != nil {
			return err
		}
	case 0xFF:
		return fmt.Errorf("no acceptable auth method")
	default:
		return fmt.Errorf("unsupported auth method: %d", resp[1])
	}

	// Phase 2: CONNECT 请求
	req := buildSocks5Request(metadata)
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("send connect request: %w", err)
	}
	return readSocks5Reply(conn)
}

func (s *Socks5Outbound) authUserPass(conn net.Conn) error {
	ul, pl := len(s.username), len(s.password)
	buf := make([]byte, 3+ul+pl)
	buf[0] = 0x01
	buf[1] = byte(ul)
	copy(buf[2:], s.username)
	buf[2+ul] = byte(pl)
	copy(buf[3+ul:], s.password)
	if _, err := conn.Write(buf); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("read auth response: %w", err)
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("authentication failed")
	}
	return nil
}

// buildSocks5Request 构造 SOCKS5 CONNECT 请求字节
func buildSocks5Request(metadata *Metadata) []byte {
	var buf []byte
	buf = append(buf, 0x05, 0x01, 0x00) // VER CMD RSV
	if metadata.DstHost != "" {
		buf = append(buf, 0x03, byte(len(metadata.DstHost)))
		buf = append(buf, []byte(metadata.DstHost)...)
	} else if ip4 := metadata.DstIP.To4(); ip4 != nil {
		buf = append(buf, 0x01)
		buf = append(buf, ip4...)
	} else {
		buf = append(buf, 0x04)
		buf = append(buf, metadata.DstIP.To16()...)
	}
	port := make([]byte, 2)
	binary.BigEndian.PutUint16(port, metadata.DstPort)
	return append(buf, port...)
}

// readSocks5Reply 读取并验证 SOCKS5 響應
func readSocks5Reply(conn net.Conn) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read reply header: %w", err)
	}
	if header[1] != 0x00 {
		return fmt.Errorf("socks5 CONNECT failed, REP=0x%02x", header[1])
	}
	switch header[3] {
	case 0x01:
		tmp := make([]byte, 6)
		_, _ = io.ReadFull(conn, tmp)
	case 0x03:
		lb := make([]byte, 1)
		_, _ = io.ReadFull(conn, lb)
		tmp := make([]byte, int(lb[0])+2)
		_, _ = io.ReadFull(conn, tmp)
	case 0x04:
		tmp := make([]byte, 18)
		_, _ = io.ReadFull(conn, tmp)
	}
	return nil
}

// ── HttpOutbound：HTTP CONNECT 上游代理 ──────────────────────────────────────

type HttpOutbound struct {
	name     string
	server   string
	username string
	password string
}

func NewHttpOutbound(name, server, username, password string) *HttpOutbound {
	return &HttpOutbound{name: name, server: server, username: username, password: password}
}

func (h *HttpOutbound) Name() string { return h.name }

func (h *HttpOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", h.server)
	if err != nil {
		return nil, fmt.Errorf("connect to http proxy %s: %w", h.server, err)
	}
	if err := h.tunnel(conn, metadata); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (h *HttpOutbound) tunnel(conn net.Conn, metadata *Metadata) error {
	target := metadata.RemoteAddress()
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if h.username != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(h.username + ":" + h.password))
		req += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", cred)
	}
	req += "\r\n"
	if _, err := fmt.Fprint(conn, req); err != nil {
		return fmt.Errorf("send CONNECT: %w", err)
	}
	line, err := readLine(conn)
	if err != nil {
		return fmt.Errorf("read CONNECT response: %w", err)
	}
	if len(line) < 12 || line[9:12] != "200" {
		return fmt.Errorf("HTTP CONNECT failed: %s", line)
	}
	for {
		l, err := readLine(conn)
		if err != nil || l == "" {
			break
		}
	}
	return nil
}

// readLine 逐字节读取一行（\r\n 或 \n 结尾）
func readLine(conn net.Conn) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			if len(line) > 0 {
				return string(line), nil
			}
			return "", err
		}
		if buf[0] == '\n' {
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			return string(line), nil
		}
		line = append(line, buf[0])
	}
}

// ── TLS 辅助 ──────────────────────────────────────────────────────────────────

// tlsClientConn 包装 TLS 客户端（供 wrapTLSClient / VMess 使用）
func tlsClientConn(conn net.Conn, sni string, skipCertVerify bool) *tls.Conn {
	return tls.Client(conn, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: skipCertVerify, //nolint:gosec
		MinVersion:         tls.VersionTLS12,
	})
}
