package proxy

// hysteria2_quic.go — 真正的 Hysteria2 QUIC 拨号实现
//
// 使用 quic-go 库实现 Hysteria2 协议的 QUIC 连接。

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
	SetHysteria2DialFn(hysteria2QuicDial)
}

// hysteria2QuicDial 通过 QUIC 建立 Hysteria2 连接并认证
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

	// 打开认证 stream
	authStream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		qconn.CloseWithError(0, "open auth stream failed")
		return nil, fmt.Errorf("open auth stream: %w", err)
	}

	// 发送认证
	if err := sendHy2Auth(authStream, password); err != nil {
		qconn.CloseWithError(0, "auth failed")
		return nil, fmt.Errorf("send auth: %w", err)
	}

	// 读取认证响应
	if err := readHy2AuthResp(authStream); err != nil {
		qconn.CloseWithError(0, "auth response failed")
		return nil, fmt.Errorf("read auth response: %w", err)
	}

	log.Printf("[Hysteria2] 认证成功: server=%s", server)

	return &hy2QUICConn{
		qconn:  qconn,
		server: server,
	}, nil
}

// hy2QUICConn 包装 QUIC 连接
type hy2QUICConn struct {
	qconn  *quic.Conn
	server string
	mu     sync.Mutex
}

// DialStream 在已有 QUIC 连接上打开新 stream 进行代理
func (c *hy2QUICConn) DialStream(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	stream, err := c.qconn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open proxy stream: %w", err)
	}

	if err := sendHy2Connect(stream, metadata); err != nil {
		stream.Close()
		return nil, fmt.Errorf("send connect: %w", err)
	}

	if err := readHy2ConnectResp(stream); err != nil {
		stream.Close()
		return nil, fmt.Errorf("connect response: %w", err)
	}

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

// ── Hysteria2 协议消息 ────────────────────────────────────────────────────────

func sendHy2Auth(w io.Writer, password string) error {
	buf := make([]byte, 0, 1+binary.MaxVarintLen64+len(password))
	buf = append(buf, 0x04) // AUTH type
	lenBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(lenBuf, uint64(len(password)))
	buf = append(buf, lenBuf[:n]...)
	buf = append(buf, []byte(password)...)
	_, err := w.Write(buf)
	return err
}

func readHy2AuthResp(r io.Reader) error {
	status := make([]byte, 1)
	if _, err := io.ReadFull(r, status); err != nil {
		return fmt.Errorf("read auth status: %w", err)
	}
	if status[0] != 0x00 {
		msgLen, err := binary.ReadUvarint(newByteReaderAdapter(r))
		if err != nil {
			return fmt.Errorf("auth rejected (status=%d)", status[0])
		}
		msg := make([]byte, msgLen)
		_, _ = io.ReadFull(r, msg)
		return fmt.Errorf("auth rejected: %s", string(msg))
	}
	return nil
}

func sendHy2Connect(w io.Writer, metadata *Metadata) error {
	var buf []byte

	reqID := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(reqID, 0x401)
	buf = append(buf, reqID[:n]...)

	host := metadata.DstHost
	port := metadata.DstPort
	if host != "" {
		buf = append(buf, 0x03)
		buf = append(buf, byte(len(host)))
		buf = append(buf, []byte(host)...)
	} else if metadata.DstIP != nil {
		if ip4 := metadata.DstIP.To4(); ip4 != nil {
			buf = append(buf, 0x01)
			buf = append(buf, ip4...)
		} else {
			buf = append(buf, 0x04)
			buf = append(buf, metadata.DstIP...)
		}
	}

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, port)
	buf = append(buf, portBytes...)

	padLen := make([]byte, binary.MaxVarintLen64)
	pn := binary.PutUvarint(padLen, 0)
	buf = append(buf, padLen[:pn]...)

	_, err := w.Write(buf)
	return err
}

func readHy2ConnectResp(r io.Reader) error {
	status := make([]byte, 1)
	if _, err := io.ReadFull(r, status); err != nil {
		return fmt.Errorf("read connect status: %w", err)
	}
	if status[0] != 0x00 {
		msgLen, err := binary.ReadUvarint(newByteReaderAdapter(r))
		if err != nil {
			return fmt.Errorf("connect rejected (status=%d)", status[0])
		}
		msg := make([]byte, msgLen)
		_, _ = io.ReadFull(r, msg)
		return fmt.Errorf("connect rejected: %s", string(msg))
	}
	return nil
}

type byteReaderAdapter struct{ r io.Reader }

func newByteReaderAdapter(r io.Reader) *byteReaderAdapter { return &byteReaderAdapter{r: r} }
func (br *byteReaderAdapter) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := br.r.Read(buf)
	return buf[0], err
}
