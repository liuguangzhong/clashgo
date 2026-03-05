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

	// HTTP/3 HEADERS 帧 (QPACK 编码)
	headers := encodeQPACKAuthRequest(password)
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

// encodeQPACKAuthRequest 按 RFC 9204 正确编码 Hysteria2 认证请求头
// QPACK 静态表: :method POST=20, :path=1, :authority=0
func encodeQPACKAuthRequest(password string) []byte {
	var buf []byte

	// Required Insert Count = 0, Delta Base = 0
	buf = append(buf, 0x00, 0x00)

	// :method POST — Indexed Field Line (static table index 20)
	// Format: 1_T_Index(6+), T=1(static), Index=20
	// Byte: 1_1_010100 = 0xd4
	buf = append(buf, 0xd4)

	// :path /auth — Literal with Name Reference (static table index 1)
	// Format: 0_1_N_T_Index(4+), N=0, T=1(static), Index=1
	// Byte: 0_1_0_1_0001 = 0x51
	buf = append(buf, 0x51)
	// Value: H=0, Length(7+)=5, then "/auth"
	buf = append(buf, 0x05)
	buf = append(buf, "/auth"...)

	// :authority hysteria — Literal with Name Reference (static index 0)
	// Byte: 0_1_0_1_0000 = 0x50
	buf = append(buf, 0x50)
	buf = append(buf, 0x08) // len("hysteria")=8
	buf = append(buf, "hysteria"...)

	// hysteria-auth: <password> — Literal Without Name Reference
	// Format: 0_0_1_N_H_NameLen(3+), N=0, H=0
	// "hysteria-auth" = 13 chars, 3-bit prefix max=7, need overflow: 7 + 6
	// Byte: 0_0_1_0_0_111 = 0x27, then 6
	buf = append(buf, 0x27, 0x06)
	buf = append(buf, "hysteria-auth"...)
	// Value: H=0, len with 7-bit prefix
	buf = appendPrefixInt(buf, 7, uint64(len(password)))
	buf = append(buf, password...)

	// hysteria-cc-rx: 0 — Literal Without Name Reference
	// "hysteria-cc-rx" = 14 chars, overflow: 7 + 7
	buf = append(buf, 0x27, 0x07)
	buf = append(buf, "hysteria-cc-rx"...)
	buf = append(buf, 0x01) // len("0")=1
	buf = append(buf, '0')

	return buf
}

// appendPrefixInt 编码 QPACK/HPACK 前缀整数 (RFC 7541 Section 5.1)
func appendPrefixInt(buf []byte, prefixBits int, value uint64) []byte {
	maxVal := uint64((1 << prefixBits) - 1)
	if value < maxVal {
		return append(buf, byte(value))
	}
	buf = append(buf, byte(maxVal))
	value -= maxVal
	for value >= 128 {
		buf = append(buf, byte(value%128+128))
		value /= 128
	}
	buf = append(buf, byte(value))
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
