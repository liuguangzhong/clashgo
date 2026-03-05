package proxy

// trojan.go — Trojan 出站实现
//
// 参照 mihomo/transport/trojan/trojan.go 实现。
// 协议：TLS + SHA224 hex 密码 + SOCKS5 地址头

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
)

// TrojanOutbound Trojan 出站代理
type TrojanOutbound struct {
	name           string
	server         string
	password       string
	sni            string
	skipCertVerify bool
	alpn           []string
}

func NewTrojanOutbound(name, server, password, sni string, skipCertVerify bool) *TrojanOutbound {
	return &TrojanOutbound{
		name:           name,
		server:         server,
		password:       password,
		sni:            sni,
		skipCertVerify: skipCertVerify,
		alpn:           []string{"h2", "http/1.1"},
	}
}

func (t *TrojanOutbound) Name() string { return t.name }

func (t *TrojanOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	// 1. TCP 连接
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", t.server)
	if err != nil {
		return nil, fmt.Errorf("connect to trojan server %s: %w", t.server, err)
	}

	// 2. TLS 握手
	sni := t.sni
	if sni == "" {
		sni, _, _ = net.SplitHostPort(t.server)
	}
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: t.skipCertVerify, //nolint:gosec
		NextProtos:         t.alpn,
		MinVersion:         tls.VersionTLS12,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("trojan TLS handshake: %w", err)
	}

	// 3. 写 Trojan 请求头
	if err := writeTrojanHeader(tlsConn, t.password, metadata); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("write trojan header: %w", err)
	}

	return tlsConn, nil
}

// writeTrojanHeader 写入 Trojan 协议头
//
// 格式（参照 mihomo/transport/trojan.WriteHeader）：
//
//	HexPassword[56] \r\n
//	CMD[1] SOCKS5-ADDR \r\n
func writeTrojanHeader(conn net.Conn, password string, m *Metadata) error {
	hexKey := trojanKey(password)
	addr := buildSocksAddr(m)

	buf := make([]byte, 0, 56+2+1+len(addr)+2)
	buf = append(buf, hexKey[:]...)
	buf = append(buf, '\r', '\n')
	buf = append(buf, 0x01) // CommandTCP
	buf = append(buf, addr...)
	buf = append(buf, '\r', '\n')

	_, err := conn.Write(buf)
	return err
}

// trojanKey SHA224(password) → hex 编码 → [56]byte
func trojanKey(password string) [56]byte {
	hash := sha256.Sum224([]byte(password))
	var key [56]byte
	hex.Encode(key[:], hash[:])
	return key
}
