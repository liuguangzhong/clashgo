package proxy

// hysteria2_quic.go — Hysteria2 QUIC 协议实现
//
// 协议规范: https://v2.hysteria.network/docs/developers/Protocol/
//
// 流程：
//   1. UDP 连接 → Salamander 混淆包装 → QUIC 传输（TLS 1.3, ALPN: "h3"）
//   2. HTTP/3 POST /auth，Hysteria-Auth 头带密码，期望回复 233
//   3. 后续每个 TCP 代理请求打开新 QUIC stream，发送 TCPRequest 二进制消息
//   4. stream 变为双向数据中继

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

// hysteria2QuicDial 通过 QUIC + Salamander 建立 Hysteria2 连接
func hysteria2QuicDial(ctx context.Context, server, password, sni string, skipCert bool) (net.Conn, error) {
	if sni == "" {
		host, _, _ := net.SplitHostPort(server)
		sni = host
	}

	log.Printf("[Hysteria2] QUIC dial: server=%s sni=%s", server, sni)

	// 解析服务器地址
	udpAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", server, err)
	}

	// 创建 UDP 连接
	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}

	// 获取 outbound 的 obfs 配置（通过 context 传递）
	obfsPassword := getObfsPassword(ctx)

	// 包装 Salamander 混淆
	var pconn net.PacketConn = udpConn
	if obfsPassword != "" {
		log.Printf("[Hysteria2] 启用 Salamander 混淆: server=%s", server)
		pconn = NewSalamanderConn(udpConn, obfsPassword)
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

	// 使用 Transport 指定自定义 PacketConn（带混淆）
	tr := &quic.Transport{
		Conn: pconn,
	}

	qconn, err := tr.Dial(ctx, udpAddr, tlsConf, quicConf)
	if err != nil {
		pconn.Close()
		return nil, fmt.Errorf("QUIC dial %s: %w", server, err)
	}

	// HTTP/3 认证
	if err := hy2Authenticate(ctx, qconn, password); err != nil {
		qconn.CloseWithError(0, "auth failed")
		return nil, fmt.Errorf("hysteria2 auth: %w", err)
	}

	log.Printf("[Hysteria2] 认证成功: server=%s", server)

	return &hy2QUICConn{
		qconn:  qconn,
		server: server,
		pconn:  pconn,
	}, nil
}

// ── context 传递 obfs-password ────────────────────────────────────────────────

type obfsPasswordKey struct{}

// WithObfsPassword 将 obfs-password 存入 context
func WithObfsPassword(ctx context.Context, password string) context.Context {
	return context.WithValue(ctx, obfsPasswordKey{}, password)
}

func getObfsPassword(ctx context.Context) string {
	if v, ok := ctx.Value(obfsPasswordKey{}).(string); ok {
		return v
	}
	return ""
}

// ── HTTP/3 认证 ──────────────────────────────────────────────────────────────

func hy2Authenticate(ctx context.Context, qconn *quic.Conn, password string) error {
	stream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open auth stream: %w", err)
	}

	// HTTP/3 HEADERS 帧
	headers := encodeQPACKHeaders(map[string]string{
		":method":        "POST",
		":path":          "/auth",
		":authority":     "hysteria",
		"hysteria-auth":  password,
		"hysteria-cc-rx": "0",
	})
	frame := encodeH3Frame(0x01, headers)

	if _, err := stream.Write(frame); err != nil {
		stream.Close()
		return fmt.Errorf("write auth: %w", err)
	}

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

// ── QUIC 连接包装 ────────────────────────────────────────────────────────────

type hy2QUICConn struct {
	qconn  *quic.Conn
	server string
	pconn  net.PacketConn // 底层 UDP（可能带 Salamander）
	mu     sync.Mutex
}

func (c *hy2QUICConn) DialStream(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	stream, err := c.qconn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open proxy stream: %w", err)
	}

	// TCPRequest: [varint 0x401] [varint addr_len] [addr "host:port"] [varint 0]
	addr := fmt.Sprintf("%s:%d", metadata.DstHost, metadata.DstPort)
	if metadata.DstHost == "" && metadata.DstIP != nil {
		addr = fmt.Sprintf("%s:%d", metadata.DstIP.String(), metadata.DstPort)
	}

	var buf []byte
	buf = appendVarint(buf, 0x401)
	buf = appendVarint(buf, uint64(len(addr)))
	buf = append(buf, []byte(addr)...)
	buf = appendVarint(buf, 0) // padding = 0

	if _, err := stream.Write(buf); err != nil {
		stream.Close()
		return nil, fmt.Errorf("write tcp request: %w", err)
	}

	// TCPResponse: [uint8 status] [varint msg_len] [msg] [varint pad_len] [pad]
	status := make([]byte, 1)
	if _, err := io.ReadFull(stream, status); err != nil {
		stream.Close()
		return nil, fmt.Errorf("read tcp response: %w", err)
	}

	msgLen, err := binary.ReadUvarint(newByteReaderAdapter(stream))
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("read msg len: %w", err)
	}
	var msg []byte
	if msgLen > 0 {
		msg = make([]byte, msgLen)
		io.ReadFull(stream, msg)
	}

	padLen, _ := binary.ReadUvarint(newByteReaderAdapter(stream))
	if padLen > 0 {
		pad := make([]byte, padLen)
		io.ReadFull(stream, pad)
	}

	if status[0] != 0x00 {
		stream.Close()
		return nil, fmt.Errorf("tcp connect rejected: status=%d msg=%s", status[0], string(msg))
	}

	log.Printf("[Hysteria2] TCP stream: %s", addr)

	return &quicStreamConn{
		stream: stream,
		laddr:  c.qconn.LocalAddr(),
		raddr:  c.qconn.RemoteAddr(),
	}, nil
}

func (c *hy2QUICConn) Read(b []byte) (int, error)  { return 0, io.EOF }
func (c *hy2QUICConn) Write(b []byte) (int, error) { return 0, io.ErrClosedPipe }
func (c *hy2QUICConn) Close() error {
	c.qconn.CloseWithError(0, "")
	if c.pconn != nil {
		c.pconn.Close()
	}
	return nil
}
func (c *hy2QUICConn) LocalAddr() net.Addr                { return c.qconn.LocalAddr() }
func (c *hy2QUICConn) RemoteAddr() net.Addr               { return c.qconn.RemoteAddr() }
func (c *hy2QUICConn) SetDeadline(t time.Time) error      { return nil }
func (c *hy2QUICConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *hy2QUICConn) SetWriteDeadline(t time.Time) error { return nil }

// ── QUIC Stream → net.Conn ──────────────────────────────────────────────────

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

func appendVarint(buf []byte, v uint64) []byte {
	tmp := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(tmp, v)
	return append(buf, tmp[:n]...)
}

func encodeQPACKHeaders(headers map[string]string) []byte {
	var buf []byte
	buf = append(buf, 0x00, 0x00) // Required Insert Count=0, Delta Base=0
	for name, value := range headers {
		buf = append(buf, 0x20|0x08) // literal without name ref, never indexed
		buf = append(buf, byte(len(name)))
		buf = append(buf, []byte(name)...)
		buf = append(buf, byte(len(value)))
		buf = append(buf, []byte(value)...)
	}
	return buf
}

func encodeH3Frame(frameType uint64, payload []byte) []byte {
	var buf []byte
	buf = appendVarint(buf, frameType)
	buf = appendVarint(buf, uint64(len(payload)))
	buf = append(buf, payload...)
	return buf
}

func readH3ResponseStatus(r io.Reader) (int, error) {
	frameType, err := readVarint(r)
	if err != nil {
		return 0, fmt.Errorf("read frame type: %w", err)
	}
	for frameType != 0x01 {
		frameLen, err := readVarint(r)
		if err != nil {
			return 0, fmt.Errorf("read frame len: %w", err)
		}
		if frameLen > 0 {
			skip := make([]byte, frameLen)
			io.ReadFull(r, skip)
		}
		frameType, err = readVarint(r)
		if err != nil {
			return 0, fmt.Errorf("read next frame type: %w", err)
		}
	}
	frameLen, err := readVarint(r)
	if err != nil {
		return 0, fmt.Errorf("read headers frame len: %w", err)
	}
	headerBlock := make([]byte, frameLen)
	if _, err := io.ReadFull(r, headerBlock); err != nil {
		return 0, fmt.Errorf("read headers block: %w", err)
	}
	status := parseStatusFromQPACK(headerBlock)
	if status == 0 {
		return 0, fmt.Errorf("could not parse :status from response")
	}
	return status, nil
}

func parseStatusFromQPACK(block []byte) int {
	if len(block) < 2 {
		return 0
	}
	pos := 2 // skip Required Insert Count + Delta Base
	for pos < len(block) {
		b := block[pos]
		if b&0x80 != 0 {
			idx := int(b & 0x3f)
			pos++
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
