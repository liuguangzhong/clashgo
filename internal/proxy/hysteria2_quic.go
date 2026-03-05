package proxy

// hysteria2_quic.go — Hysteria2 QUIC 协议实现
//
// 协议规范: https://v2.hysteria.network/docs/developers/Protocol/
//
// 认证流程:
//   1. QUIC 连接 (TLS 1.3, ALPN: "h3")
//   2. HTTP/3 POST /auth 请求，Hysteria-Auth 头带密码
//   3. 服务端返回 HTTP 233 = 认证成功
//
// TCP 代理:
//   每个请求打开新 QUIC bidirectional stream，发送:
//     [varint] 0x401 (TCPRequest ID)
//     [varint] Address length
//     [bytes]  Address string (host:port)
//     [varint] Padding length (0)
//     [bytes]  Padding
//   服务端响应:
//     [uint8]  Status (0x00=OK, 0x01=Error)
//     [varint] Message length
//     [bytes]  Message string
//     [varint] Padding length
//     [bytes]  Padding
//   状态 OK 后 stream 变为双向数据中继

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
	"github.com/quic-go/quic-go/http3"
)

func init() {
	SetHysteria2DialFn(hysteria2QuicDial)
}

// hysteria2QuicDial 通过 HTTP/3 + QUIC 建立 Hysteria2 连接并认证
func hysteria2QuicDial(ctx context.Context, server, password, sni string, skipCert bool) (net.Conn, error) {
	if sni == "" {
		host, _, _ := net.SplitHostPort(server)
		sni = host
	}

	tlsConf := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: skipCert, //nolint:gosec
		NextProtos:         []string{http3.NextProtoH3},
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

	// Hysteria2 认证: 通过 HTTP/3 POST /auth
	if err := hy2Authenticate(ctx, qconn, password); err != nil {
		qconn.CloseWithError(0, "auth failed")
		return nil, fmt.Errorf("hysteria2 auth: %w", err)
	}

	log.Printf("[Hysteria2] 认证成功: server=%s", server)

	return &hy2QUICConn{
		qconn:  qconn,
		server: server,
	}, nil
}

// hy2Authenticate 发送 HTTP/3 认证请求
// POST /auth, Hysteria-Auth: <password>, 期望回复 233
func hy2Authenticate(ctx context.Context, qconn *quic.Conn, password string) error {
	// 打开一个 QUIC stream 发送 HTTP/3 请求
	stream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open auth stream: %w", err)
	}

	// 构建 HTTP/3 HEADERS 帧 (手动编码 QPACK)
	// HTTP/3 帧格式: [varint type] [varint length] [payload]
	// HEADERS 帧 type = 0x01
	// QPACK header block: [required insert count (0)] [delta base (0)] [encoded headers]
	headers := encodeQPACKHeaders(map[string]string{
		":method":        "POST",
		":path":          "/auth",
		":authority":     "hysteria",
		"hysteria-auth":  password,
		"hysteria-cc-rx": "0",
	})

	// HTTP/3 HEADERS frame
	frame := encodeH3Frame(0x01, headers)

	if _, err := stream.Write(frame); err != nil {
		stream.Close()
		return fmt.Errorf("write auth request: %w", err)
	}

	// 读取 HTTP/3 响应
	statusCode, err := readH3ResponseStatus(stream)
	if err != nil {
		stream.Close()
		return fmt.Errorf("read auth response: %w", err)
	}

	stream.Close()

	if statusCode != 233 {
		return fmt.Errorf("auth failed, status=%d (expected 233)", statusCode)
	}

	return nil
}

// hy2QUICConn 包装 QUIC 连接
type hy2QUICConn struct {
	qconn  *quic.Conn
	server string
	mu     sync.Mutex
}

// DialStream 在已有 QUIC 连接上打开新 stream 代理 TCP
func (c *hy2QUICConn) DialStream(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	stream, err := c.qconn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open proxy stream: %w", err)
	}

	// 发送 TCPRequest: [varint 0x401] [varint addr_len] [addr] [varint 0 padding]
	addr := fmt.Sprintf("%s:%d", metadata.DstHost, metadata.DstPort)
	if metadata.DstHost == "" && metadata.DstIP != nil {
		addr = fmt.Sprintf("%s:%d", metadata.DstIP.String(), metadata.DstPort)
	}

	var buf []byte
	// TCPRequest ID = 0x401
	buf = appendVarint(buf, 0x401)
	// Address length + address
	buf = appendVarint(buf, uint64(len(addr)))
	buf = append(buf, []byte(addr)...)
	// Padding length = 0
	buf = appendVarint(buf, 0)

	if _, err := stream.Write(buf); err != nil {
		stream.Close()
		return nil, fmt.Errorf("write tcp request: %w", err)
	}

	// 读取 TCPResponse: [uint8 status] [varint msg_len] [msg] [varint pad_len] [pad]
	status := make([]byte, 1)
	if _, err := io.ReadFull(stream, status); err != nil {
		stream.Close()
		return nil, fmt.Errorf("read tcp response: %w", err)
	}

	// 读 message
	msgLen, err := binary.ReadUvarint(newByteReaderAdapter(stream))
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("read msg len: %w", err)
	}
	if msgLen > 0 {
		msg := make([]byte, msgLen)
		io.ReadFull(stream, msg)
		if status[0] != 0x00 {
			stream.Close()
			return nil, fmt.Errorf("tcp connect rejected: %s", string(msg))
		}
	}

	// 读 padding
	padLen, _ := binary.ReadUvarint(newByteReaderAdapter(stream))
	if padLen > 0 {
		padding := make([]byte, padLen)
		io.ReadFull(stream, padding)
	}

	if status[0] != 0x00 {
		stream.Close()
		return nil, fmt.Errorf("tcp connect failed (status=%d)", status[0])
	}

	log.Printf("[Hysteria2] TCP stream 建立: %s", addr)

	return &quicStreamConn{
		stream: stream,
		laddr:  c.qconn.LocalAddr(),
		raddr:  c.qconn.RemoteAddr(),
	}, nil
}

func (c *hy2QUICConn) Read(b []byte) (int, error)         { return 0, io.EOF }
func (c *hy2QUICConn) Write(b []byte) (int, error)        { return 0, io.ErrClosedPipe }
func (c *hy2QUICConn) Close() error                       { c.qconn.CloseWithError(0, ""); return nil }
func (c *hy2QUICConn) LocalAddr() net.Addr                { return c.qconn.LocalAddr() }
func (c *hy2QUICConn) RemoteAddr() net.Addr               { return c.qconn.RemoteAddr() }
func (c *hy2QUICConn) SetDeadline(t time.Time) error      { return nil }
func (c *hy2QUICConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *hy2QUICConn) SetWriteDeadline(t time.Time) error { return nil }

// quicStreamConn 将 *quic.Stream 包装为 net.Conn
type quicStreamConn struct {
	stream *quic.Stream
	laddr  net.Addr
	raddr  net.Addr
}

func (c *quicStreamConn) Read(b []byte) (int, error)  { return c.stream.Read(b) }
func (c *quicStreamConn) Write(b []byte) (int, error) { return c.stream.Write(b) }
func (c *quicStreamConn) Close() error {
	c.stream.CancelRead(0)
	return c.stream.Close()
}
func (c *quicStreamConn) LocalAddr() net.Addr                { return c.laddr }
func (c *quicStreamConn) RemoteAddr() net.Addr               { return c.raddr }
func (c *quicStreamConn) SetDeadline(t time.Time) error      { return nil }
func (c *quicStreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *quicStreamConn) SetWriteDeadline(t time.Time) error { return nil }

// ── 辅助函数 ─────────────────────────────────────────────────────────────────

// appendVarint 追加 QUIC varint 编码 (RFC 9000)
func appendVarint(buf []byte, v uint64) []byte {
	tmp := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(tmp, v)
	return append(buf, tmp[:n]...)
}

// encodeQPACKHeaders 简单的 QPACK 编码（仅使用 literal header field without name reference）
// 参考 RFC 9204 Section 4.5.6
func encodeQPACKHeaders(headers map[string]string) []byte {
	var buf []byte
	// Required Insert Count = 0, Delta Base = 0
	buf = append(buf, 0x00, 0x00)

	for name, value := range headers {
		// Literal header field without name reference (0x20 prefix, N bit = 0)
		// Format: 0010_NNNN [name_len] [name] [value_len] [value]
		buf = append(buf, 0x20|0x08) // 0x28 = literal + never indexed
		// Name length (7-bit prefix encoding for small values)
		buf = append(buf, byte(len(name)))
		buf = append(buf, []byte(name)...)
		// Value length (7-bit prefix encoding)
		buf = append(buf, byte(len(value)))
		buf = append(buf, []byte(value)...)
	}
	return buf
}

// encodeH3Frame 编码 HTTP/3 帧: [varint type] [varint length] [payload]
func encodeH3Frame(frameType uint64, payload []byte) []byte {
	var buf []byte
	buf = appendVarint(buf, frameType)
	buf = appendVarint(buf, uint64(len(payload)))
	buf = append(buf, payload...)
	return buf
}

// readH3ResponseStatus 读取 HTTP/3 HEADERS 帧并提取状态码
func readH3ResponseStatus(r io.Reader) (int, error) {
	// 读帧类型
	frameType, err := readVarint(r)
	if err != nil {
		return 0, fmt.Errorf("read frame type: %w", err)
	}

	// 跳过非 HEADERS 帧（如 SETTINGS）
	for frameType != 0x01 {
		frameLen, err := readVarint(r)
		if err != nil {
			return 0, fmt.Errorf("read frame len: %w", err)
		}
		// 跳过帧体
		if frameLen > 0 {
			skip := make([]byte, frameLen)
			io.ReadFull(r, skip)
		}
		frameType, err = readVarint(r)
		if err != nil {
			return 0, fmt.Errorf("read next frame type: %w", err)
		}
	}

	// 读 HEADERS 帧长度
	frameLen, err := readVarint(r)
	if err != nil {
		return 0, fmt.Errorf("read headers frame len: %w", err)
	}

	// 读 HEADERS 帧体
	headerBlock := make([]byte, frameLen)
	if _, err := io.ReadFull(r, headerBlock); err != nil {
		return 0, fmt.Errorf("read headers block: %w", err)
	}

	// 解析状态码（在 QPACK 编码中，:status 通常是第一个 header）
	// 简化解析：搜索状态码
	status := parseStatusFromQPACK(headerBlock)
	if status == 0 {
		return 0, fmt.Errorf("could not parse :status from response")
	}

	return status, nil
}

// parseStatusFromQPACK 从 QPACK 编码的 header block 中提取 :status
// QPACK indexed header field 0xd9 = :status 200, 等等
// 对于自定义状态码如 233，会以 literal 形式出现
func parseStatusFromQPACK(block []byte) int {
	if len(block) < 2 {
		return 0
	}
	// 跳过 Required Insert Count 和 Delta Base (前2字节)
	pos := 2

	for pos < len(block) {
		b := block[pos]

		if b&0x80 != 0 {
			// Indexed header field (Section 4.5.2)
			// 6-bit index
			idx := int(b & 0x3f)
			pos++
			// 常见的 QPACK 静态表索引:
			// 24: :status 200, 25: :status 204, 26: :status 206
			// 27: :status 304, 28: :status 400, 29: :status 403
			// 30: :status 404, 31: :status 500
			switch idx {
			case 24:
				return 200
			case 25:
				return 204
			case 26:
				return 206
			case 27:
				return 304
			case 28:
				return 400
			case 29:
				return 403
			case 30:
				return 404
			case 31:
				return 500
			}
			continue
		}

		if b&0xf0 == 0x20 || b&0xf0 == 0x30 {
			// Literal header field without name reference
			pos++
			if pos >= len(block) {
				break
			}
			nameLen := int(block[pos] & 0x7f)
			pos++
			if pos+nameLen > len(block) {
				break
			}
			name := string(block[pos : pos+nameLen])
			pos += nameLen
			if pos >= len(block) {
				break
			}
			valueLen := int(block[pos] & 0x7f)
			pos++
			if pos+valueLen > len(block) {
				break
			}
			value := string(block[pos : pos+valueLen])
			pos += valueLen

			if name == ":status" {
				var status int
				fmt.Sscanf(value, "%d", &status)
				return status
			}
			continue
		}

		// 其他编码格式，尝试跳过
		pos++
	}

	return 0
}

func readVarint(r io.Reader) (uint64, error) {
	return binary.ReadUvarint(newByteReaderAdapter(r))
}

type byteReaderAdapter struct{ r io.Reader }

func newByteReaderAdapter(r io.Reader) *byteReaderAdapter { return &byteReaderAdapter{r: r} }
func (br *byteReaderAdapter) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := br.r.Read(buf)
	return buf[0], err
}
