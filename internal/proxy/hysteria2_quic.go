package proxy

// hysteria2_quic.go — 真正的 Hysteria2 QUIC 拨号实现
//
// 使用 quic-go 库实现 Hysteria2 协议的 QUIC 连接。
// Hysteria2 协议流程：
//   1. UDP → QUIC（TLS 1.3, ALPN: "h3"）
//   2. 打开 QUIC stream，发送 Hysteria2 认证请求（HTTP/3 style）
//   3. 读取认证响应（200 OK → 认证成功）
//   4. 使用该 stream 进行代理数据中继

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

func init() {
	// 注入真正的 QUIC 拨号器
	SetHysteria2DialFn(hysteria2QuicDial)
}

// hysteria2QuicDial 通过 QUIC 建立 Hysteria2 连接
func hysteria2QuicDial(ctx context.Context, server, password, sni string, skipCert bool) (net.Conn, error) {
	if sni == "" {
		host, _, _ := net.SplitHostPort(server)
		sni = host
	}

	tlsConf := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: skipCert, //nolint:gosec
		NextProtos:         []string{"h3"},
		MinVersion:         tls.VersionTLS13,
	}

	quicConf := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 15 * time.Second,
		EnableDatagrams: true,
	}

	log.Printf("[Hysteria2] QUIC dial: server=%s sni=%s", server, sni)

	qconn, err := quic.DialAddr(ctx, server, tlsConf, quicConf)
	if err != nil {
		return nil, fmt.Errorf("QUIC dial %s: %w", server, err)
	}

	// 打开认证 stream（Hysteria2 使用 HTTP/3 风格认证）
	authStream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		qconn.CloseWithError(0, "open auth stream failed")
		return nil, fmt.Errorf("open auth stream: %w", err)
	}

	// 发送 Hysteria2 认证请求
	// Hysteria2 认证协议：
	// - 发送一个类似 HTTP/3 的认证请求
	// - 格式：varint frame type + varint length + headers
	if err := sendHy2AuthRequest(authStream, password); err != nil {
		qconn.CloseWithError(0, "auth failed")
		return nil, fmt.Errorf("send auth: %w", err)
	}

	// 读取认证响应
	if err := readHy2AuthResponse(authStream); err != nil {
		qconn.CloseWithError(0, "auth response failed")
		return nil, fmt.Errorf("read auth response: %w", err)
	}

	log.Printf("[Hysteria2] 认证成功: server=%s", server)

	// 返回一个包装了 QUIC 连接的 net.Conn
	return &hy2QUICConn{
		qconn:  qconn,
		server: server,
	}, nil
}

// hy2QUICConn 包装 QUIC 连接为 net.Conn 接口
// 每次 DialTCP 调用时打开新的 stream 用于数据传输
type hy2QUICConn struct {
	qconn  quic.Connection
	server string
	mu     sync.Mutex
}

// DialStream 在已有 QUIC 连接上打开新 stream 用于代理
func (c *hy2QUICConn) DialStream(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	stream, err := c.qconn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open proxy stream: %w", err)
	}

	// 发送 Hysteria2 代理请求头（目标地址）
	if err := sendHy2ConnectRequest(stream, metadata); err != nil {
		stream.Close()
		return nil, fmt.Errorf("send connect request: %w", err)
	}

	// 读取代理响应
	if err := readHy2ConnectResponse(stream); err != nil {
		stream.Close()
		return nil, fmt.Errorf("read connect response: %w", err)
	}

	return &quicStreamConn{
		Stream: stream,
		qconn:  c.qconn,
	}, nil
}

func (c *hy2QUICConn) Read(b []byte) (n int, err error)   { return 0, io.EOF }
func (c *hy2QUICConn) Write(b []byte) (n int, err error)  { return 0, io.ErrClosedPipe }
func (c *hy2QUICConn) Close() error                       { c.qconn.CloseWithError(0, ""); return nil }
func (c *hy2QUICConn) LocalAddr() net.Addr                { return c.qconn.LocalAddr() }
func (c *hy2QUICConn) RemoteAddr() net.Addr               { return c.qconn.RemoteAddr() }
func (c *hy2QUICConn) SetDeadline(t time.Time) error      { return nil }
func (c *hy2QUICConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *hy2QUICConn) SetWriteDeadline(t time.Time) error { return nil }

// quicStreamConn 将 QUIC stream 包装为 net.Conn
type quicStreamConn struct {
	quic.Stream
	qconn quic.Connection
}

func (c *quicStreamConn) LocalAddr() net.Addr  { return c.qconn.LocalAddr() }
func (c *quicStreamConn) RemoteAddr() net.Addr { return c.qconn.RemoteAddr() }
func (c *quicStreamConn) Close() error {
	c.Stream.CancelRead(0)
	return c.Stream.Close()
}

// ── Hysteria2 协议消息 ────────────────────────────────────────────────────────

// Hysteria2 使用简化的 HTTP/3 style 协议：
// 客户端发送 CONNECT 请求（带 auth），服务器回复状态码
//
// 认证请求格式（二进制）：
//   [1 byte: 0x04] AUTH 命令
//   [varint: password length]
//   [password bytes]

func sendHy2AuthRequest(stream quic.Stream, password string) error {
	// Hysteria2 auth frame:
	// Type: 0x04 (client auth)
	// Then varint-encoded password
	buf := make([]byte, 0, 1+binary.MaxVarintLen64+len(password))
	buf = append(buf, 0x04) // AUTH type
	lenBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(lenBuf, uint64(len(password)))
	buf = append(buf, lenBuf[:n]...)
	buf = append(buf, []byte(password)...)
	_, err := stream.Write(buf)
	return err
}

func readHy2AuthResponse(stream quic.Stream) error {
	// Read auth response:
	// [1 byte: status] 0x00 = OK, others = error
	// [varint: message length]
	// [message bytes]
	resp := make([]byte, 1)
	if _, err := io.ReadFull(stream, resp); err != nil {
		return fmt.Errorf("read auth status: %w", err)
	}
	if resp[0] != 0x00 {
		// Read error message
		msgLen, err := binary.ReadUvarint(newByteReader(stream))
		if err != nil {
			return fmt.Errorf("auth rejected (status=%d)", resp[0])
		}
		msg := make([]byte, msgLen)
		io.ReadFull(stream, msg)
		return fmt.Errorf("auth rejected: %s", string(msg))
	}
	return nil
}

// Hysteria2 TCP proxy request:
//   [varint: request ID]
//   [1 byte: address type] (1=IPv4, 3=Domain, 4=IPv6)
//   [address bytes]
//   [2 bytes: port, big-endian]

func sendHy2ConnectRequest(stream quic.Stream, metadata *Metadata) error {
	var buf []byte

	// Request ID (varint, use 1 for TCP)
	reqID := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(reqID, 0x401) // TCP connect command
	buf = append(buf, reqID[:n]...)

	// Address
	host := metadata.DstHost
	port := metadata.DstPort
	if host != "" {
		// Domain name
		buf = append(buf, 0x03)            // ATYP: domain
		buf = append(buf, byte(len(host))) // domain length
		buf = append(buf, []byte(host)...)
	} else if metadata.DstIP != nil {
		ip := metadata.DstIP
		if ip4 := ip.To4(); ip4 != nil {
			buf = append(buf, 0x01) // ATYP: IPv4
			buf = append(buf, ip4...)
		} else {
			buf = append(buf, 0x04) // ATYP: IPv6
			buf = append(buf, ip...)
		}
	}

	// Port (big-endian)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, port)
	buf = append(buf, portBytes...)

	// Padding (0 bytes)
	padLen := make([]byte, binary.MaxVarintLen64)
	pn := binary.PutUvarint(padLen, 0)
	buf = append(buf, padLen[:pn]...)

	_, err := stream.Write(buf)
	return err
}

func readHy2ConnectResponse(stream quic.Stream) error {
	// Response: [1 byte: status] [varint: message length] [message]
	status := make([]byte, 1)
	if _, err := io.ReadFull(stream, status); err != nil {
		return fmt.Errorf("read connect status: %w", err)
	}
	if status[0] != 0x00 {
		// Read error message
		msgLen, err := binary.ReadUvarint(newByteReader(stream))
		if err != nil {
			return fmt.Errorf("connect rejected (status=%d)", status[0])
		}
		msg := make([]byte, msgLen)
		io.ReadFull(stream, msg)
		return fmt.Errorf("connect rejected: %s", string(msg))
	}
	return nil
}

// byteReader adaptor for binary.ReadUvarint
type byteReader struct {
	r io.Reader
}

func newByteReader(r io.Reader) *byteReader { return &byteReader{r: r} }

func (br *byteReader) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := br.r.Read(buf)
	return buf[0], err
}
