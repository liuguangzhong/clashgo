package proxy

// hysteria2_quic.go — Hysteria2 QUIC 协议实现
//
// 协议规范: https://v2.hysteria.network/docs/developers/Protocol/
//
// 流程：
//   1. UDP → Salamander 混淆 → QUIC (TLS1.3, ALPN:"h3")
//   2. http3.Transport.NewClientConn → HTTP/3 POST /auth (自动控制流+QPACK)
//   3. 检查响应 status=233
//   4. 每个 TCP 代理：新 QUIC stream + TCPRequest 消息

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func init() {
	SetHysteria2DialFn(hysteria2QuicDial)
}

// hysteria2QuicDial 建立 Hysteria2 QUIC 连接并 HTTP/3 认证
func hysteria2QuicDial(ctx context.Context, server, password, sni string, skipCert bool) (net.Conn, error) {
	if sni == "" {
		host, _, _ := net.SplitHostPort(server)
		sni = host
	}

	log.Printf("[hy2] ① QUIC dial: server=%s sni=%s", server, sni)

	udpAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, fmt.Errorf("[hy2] 解析地址失败 %s: %w", server, err)
	}

	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, fmt.Errorf("[hy2] 创建 UDP socket 失败: %w", err)
	}
	log.Printf("[hy2] ② UDP socket 创建成功: local=%s", udpConn.LocalAddr())

	obfsPassword := getObfsPassword(ctx)
	var pconn net.PacketConn = udpConn
	if obfsPassword != "" {
		log.Printf("[hy2] ③ 启用 Salamander 混淆: server=%s", server)
		pconn = NewSalamanderConn(udpConn, obfsPassword)
	} else {
		log.Printf("[hy2] ③ 无混淆（obfs-password 未设置）: server=%s", server)
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

	tr := &quic.Transport{Conn: pconn}

	dialCtx, cancelDial := context.WithTimeout(ctx, 8*time.Second)
	defer cancelDial()

	qconn, err := tr.Dial(dialCtx, udpAddr, tlsConf, quicConf)
	if err != nil {
		pconn.Close()
		return nil, fmt.Errorf("[hy2] ④ QUIC 握手失败 %s: %w", server, err)
	}
	log.Printf("[hy2] ④ QUIC 握手成功: server=%s proto=%s",
		server, qconn.ConnectionState().TLS.NegotiatedProtocol)

	// HTTP/3 认证
	if err := hy2AuthViaH3(ctx, qconn, sni, password, skipCert); err != nil {
		qconn.CloseWithError(0, "auth failed")
		pconn.Close()
		return nil, err
	}

	log.Printf("[hy2] ⑥ 认证成功，连接就绪: server=%s", server)

	return &hy2QUICConn{
		qconn:  qconn,
		server: server,
		pconn:  pconn,
	}, nil
}

// hy2AuthViaH3 通过 http3.Transport 发送认证请求（自动处理控制流和 QPACK）
func hy2AuthViaH3(ctx context.Context, qconn *quic.Conn, host, password string, skipCert bool) error {
	log.Printf("[hy2] ⑤ 开始 HTTP/3 认证: host=%s", host)

	// 用已有的 *quic.Conn 创建 http3.ClientConn
	h3tr := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipCert, //nolint:gosec
		},
	}
	clientConn := h3tr.NewClientConn(qconn)

	padding := randomPadding(32, 64)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+host+"/auth", http.NoBody)
	if err != nil {
		return fmt.Errorf("[hy2] 构建认证请求失败: %w", err)
	}
	req.Header.Set("Hysteria-Auth", password)
	req.Header.Set("Hysteria-CC-RX", "0")
	req.Header.Set("Hysteria-Padding", padding)

	log.Printf("[hy2] ⑤ 发送认证请求: POST https://%s/auth password=%s...",
		host, password[:min(6, len(password))])

	authCtx, cancelAuth := context.WithTimeout(ctx, 10*time.Second)
	defer cancelAuth()

	resp, err := clientConn.RoundTrip(req.WithContext(authCtx))
	if err != nil {
		return fmt.Errorf("[hy2] ⑤ 认证请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	log.Printf("[hy2] ⑤ 认证响应: status=%d body=%q", resp.StatusCode, string(body))

	if resp.StatusCode != 233 {
		return fmt.Errorf("[hy2] ⑤ 认证拒绝: status=%d (期望 233)", resp.StatusCode)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func randomPadding(minLen, maxLen int) string {
	n := minLen + rand.Intn(maxLen-minLen+1)
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + rand.Intn(26))
	}
	return string(b)
}

// ── context 传递 obfs-password ─────────────────────────────────────────────────

type obfsPasswordKey struct{}

func WithObfsPassword(ctx context.Context, password string) context.Context {
	return context.WithValue(ctx, obfsPasswordKey{}, password)
}

func getObfsPassword(ctx context.Context) string {
	if v, ok := ctx.Value(obfsPasswordKey{}).(string); ok {
		return v
	}
	return ""
}

// ── QUIC 连接包装 ──────────────────────────────────────────────────────────────

type hy2QUICConn struct {
	qconn  *quic.Conn
	server string
	pconn  net.PacketConn
	mu     sync.Mutex
}

func (c *hy2QUICConn) DialStream(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	addr := metadata.DstHost + ":" + fmt.Sprint(metadata.DstPort)
	if metadata.DstHost == "" && metadata.DstIP != nil {
		addr = metadata.DstIP.String() + ":" + fmt.Sprint(metadata.DstPort)
	}
	log.Printf("[hy2] 打开 TCP 代理流: %s via %s", addr, c.server)

	stream, err := c.qconn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("[hy2] open stream 失败: %w", err)
	}

	// TCPRequest: [varint 0x401][varint addr_len][addr][varint pad_len][pad]
	var buf []byte
	buf = appendUvarint(buf, 0x401)
	buf = appendUvarint(buf, uint64(len(addr)))
	buf = append(buf, addr...)
	buf = appendUvarint(buf, 0)

	if _, err := stream.Write(buf); err != nil {
		stream.Close()
		return nil, fmt.Errorf("[hy2] 写 TCPRequest 失败: %w", err)
	}
	log.Printf("[hy2] TCPRequest 已发送: addr=%s", addr)

	// TCPResponse: [uint8 status][varint msg_len][msg][varint pad_len][pad]
	sb := make([]byte, 1)
	if _, err := io.ReadFull(stream, sb); err != nil {
		stream.Close()
		return nil, fmt.Errorf("[hy2] 读 TCPResponse status 失败: %w", err)
	}

	msgLen, err := binary.ReadUvarint(ioByteReader(stream))
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("[hy2] 读 msg_len 失败: %w", err)
	}
	var msg string
	if msgLen > 0 {
		mb := make([]byte, msgLen)
		io.ReadFull(stream, mb)
		msg = string(mb)
	}

	padLen, _ := binary.ReadUvarint(ioByteReader(stream))
	if padLen > 0 {
		io.ReadFull(stream, make([]byte, padLen))
	}

	log.Printf("[hy2] TCPResponse: status=0x%02x msg=%q", sb[0], msg)

	if sb[0] != 0x00 {
		stream.Close()
		return nil, fmt.Errorf("[hy2] 服务端拒绝 TCP 连接: status=0x%02x msg=%s addr=%s", sb[0], msg, addr)
	}

	return &hy2StreamConn{
		stream: stream,
		laddr:  c.qconn.LocalAddr(),
		raddr:  c.qconn.RemoteAddr(),
	}, nil
}

func (c *hy2QUICConn) Read(b []byte) (int, error)  { return 0, io.EOF }
func (c *hy2QUICConn) Write(b []byte) (int, error) { return 0, io.ErrClosedPipe }
func (c *hy2QUICConn) Close() error {
	log.Printf("[hy2] 关闭 QUIC 连接: server=%s", c.server)
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

// ── QUIC stream → net.Conn ────────────────────────────────────────────────────

type hy2StreamConn struct {
	stream *quic.Stream
	laddr  net.Addr
	raddr  net.Addr
}

func (c *hy2StreamConn) Read(b []byte) (int, error)  { return c.stream.Read(b) }
func (c *hy2StreamConn) Write(b []byte) (int, error) { return c.stream.Write(b) }
func (c *hy2StreamConn) Close() error {
	c.stream.CancelRead(0)
	return c.stream.Close()
}
func (c *hy2StreamConn) LocalAddr() net.Addr                { return c.laddr }
func (c *hy2StreamConn) RemoteAddr() net.Addr               { return c.raddr }
func (c *hy2StreamConn) SetDeadline(t time.Time) error      { return nil }
func (c *hy2StreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *hy2StreamConn) SetWriteDeadline(t time.Time) error { return nil }

// ── 辅助 ──────────────────────────────────────────────────────────────────────

func appendUvarint(buf []byte, v uint64) []byte {
	tmp := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(tmp, v)
	return append(buf, tmp[:n]...)
}

type ioByteReaderImpl struct{ r io.Reader }

func ioByteReader(r io.Reader) io.ByteReader { return &ioByteReaderImpl{r: r} }
func (b *ioByteReaderImpl) ReadByte() (byte, error) {
	buf := [1]byte{}
	_, err := b.r.Read(buf[:])
	return buf[0], err
}
