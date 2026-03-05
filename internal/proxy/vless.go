package proxy

// vless.go — VLESS 出站实现
//
// 参照 mihomo/transport/vless + xtls-rprx/Xray-core VLESS 协议规范实现。
// VLESS 协议规范：https://xtls.github.io/development/protocols/vless.html
//
// VLESS 与 VMess 的核心区别：
//   1. 没有请求加密（依赖底层 TLS）
//   2. 没有时间戳认证（更轻量）
//   3. 请求头格式更简单：
//      Version(1B) UUID(16B) AddonLen(1B) [Addon] Command(1B) Port(2B) AddrType(1B) Addr Payload
//
// VLESS 请求头（TCP）：
//   +----------+------+---------------+------+---------+------+
//   | Version  | UUID | Addon-Length  |  Cmd |  Port   | Addr |
//   | 1 byte   | 16 B |    1 byte     | 1 B  |  2 B    |  var |
//   +----------+------+---------------+------+---------+------+
//
// 响应头（服务器 → 客户端）：
//   Version(1B) AddonLen(1B) [Addon] Payload

import (
	"context"
	"fmt"
	"net"
)

// VLESS 协议常量
const (
	vlessVersion byte = 0
	vlessCmdTCP  byte = 1
	vlessCmdUDP  byte = 2

	vlessAtypDomain byte = 2
	vlessAtypIPv4   byte = 1
	vlessAtypIPv6   byte = 3
)

// VLESSOutbound VLESS 出站代理
// 对应 mihomo/adapter/outbound.Vless
type VLESSOutbound struct {
	name     string
	server   string // "host:port"
	uuid     string
	flow     string // "xtls-rprx-vision" 等（高级特性，暂仅占位）
	tls      bool
	sni      string
	skipCert bool
	alpn     []string
}

func NewVLESSOutbound(name, server, uuid, flow string, useTLS bool, sni string, skipCert bool) *VLESSOutbound {
	return &VLESSOutbound{
		name:     name,
		server:   server,
		uuid:     uuid,
		flow:     flow,
		tls:      useTLS,
		sni:      sni,
		skipCert: skipCert,
		alpn:     []string{"h2", "http/1.1"},
	}
}

func (v *VLESSOutbound) Name() string { return v.name }

func (v *VLESSOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	// 1. TCP 连接
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", v.server)
	if err != nil {
		return nil, fmt.Errorf("connect to vless server %s: %w", v.server, err)
	}

	// 2. TLS（VLESS 强烈推荐 TLS，但协议本身不强制）
	if v.tls {
		conn, err = wrapTLSClient(conn, nil, v.sni, v.server, v.skipCert)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("vless TLS: %w", err)
		}
	}

	// 3. 解析 UUID
	uuidBytes, err := parseUUID(v.uuid)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("parse vless uuid: %w", err)
	}

	// 4. 发送 VLESS 请求头
	if err := writeVLESSHeader(conn, uuidBytes, metadata); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write vless header: %w", err)
	}

	// 5. 读取响应头（服务器确认头，对应 resp.go VlessServerResponseProcess）
	return &vlessConn{Conn: conn, headerRead: false}, nil
}

// writeVLESSHeader 写入 VLESS 协议请求头
//
//	Version(1) UUID(16) AddonLen(1)=0 Cmd(1) Port(2BE) AddrType(1) Addr
func writeVLESSHeader(conn net.Conn, uuidBytes [16]byte, metadata *Metadata) error {
	var buf []byte

	// Version
	buf = append(buf, vlessVersion)

	// UUID (16 bytes)
	buf = append(buf, uuidBytes[:]...)

	// Addon Length = 0（不附加扩展）
	buf = append(buf, 0x00)

	// Command = TCP
	buf = append(buf, vlessCmdTCP)

	// Port (big-endian uint16)
	buf = append(buf, byte(metadata.DstPort>>8), byte(metadata.DstPort))

	// Address
	if metadata.DstHost != "" {
		buf = append(buf, vlessAtypDomain)
		buf = append(buf, byte(len(metadata.DstHost)))
		buf = append(buf, []byte(metadata.DstHost)...)
	} else if ip4 := metadata.DstIP.To4(); ip4 != nil {
		buf = append(buf, vlessAtypIPv4)
		buf = append(buf, ip4...)
	} else {
		buf = append(buf, vlessAtypIPv6)
		buf = append(buf, metadata.DstIP.To16()...)
	}

	_, err := conn.Write(buf)
	return err
}

// vlessConn 包装 VLESS 连接，读取时先跳过服务器响应头
type vlessConn struct {
	net.Conn
	headerRead bool
}

func (c *vlessConn) Read(b []byte) (int, error) {
	if !c.headerRead {
		// 读响应头：Version(1) AddonLen(1) [Addon]
		header := make([]byte, 2)
		if _, err := readFull(c.Conn, header); err != nil {
			return 0, fmt.Errorf("read vless response header: %w", err)
		}
		addonLen := int(header[1])
		if addonLen > 0 {
			addon := make([]byte, addonLen)
			if _, err := readFull(c.Conn, addon); err != nil {
				return 0, fmt.Errorf("read vless addon: %w", err)
			}
		}
		c.headerRead = true
	}
	return c.Conn.Read(b)
}

// readFull 确保读满 n 字节
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
