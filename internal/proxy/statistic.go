package proxy

// statistic.go — 流量统计 + 连接追踪 + Sniffing（P2/P3）
//
// 参照 mihomo/tunnel/statistic/ + component/sniffer/ 实现。
//
// 功能：
//   - 全局上传/下载字节计数
//   - 活跃连接列表（供 /connections API 使用）
//   - 带宽速率计算
//   - 协议嗅探（HTTP Host / TLS SNI）

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ── 全局统计 ──────────────────────────────────────────────────────────────────

var globalStats = &Statistics{}

// Statistics 全局流量统计（对应 mihomo/tunnel/statistic.Manager）
type Statistics struct {
	UploadTotal   atomic.Int64
	DownloadTotal atomic.Int64

	// 速率（每秒刷新一次）
	UploadRate   atomic.Int64
	DownloadRate atomic.Int64

	prevUp   int64
	prevDown int64
	mu       sync.Mutex
}

func init() {
	go globalStats.rateLoop()
}

func GlobalStats() *Statistics {
	return globalStats
}

func (s *Statistics) rateLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		up := s.UploadTotal.Load()
		down := s.DownloadTotal.Load()
		s.UploadRate.Store(up - s.prevUp)
		s.DownloadRate.Store(down - s.prevDown)
		s.prevUp = up
		s.prevDown = down
		s.mu.Unlock()
	}
}

// ── 连接追踪 ──────────────────────────────────────────────────────────────────

// ConnTracker 活跃连接追踪器（对应 mihomo/tunnel/statistic.Manager connections）
type ConnTracker struct {
	mu    sync.RWMutex
	conns map[string]*TrackedConnection
	seq   atomic.Uint64
}

var globalConnTracker = &ConnTracker{
	conns: make(map[string]*TrackedConnection),
}

func GlobalConnTracker() *ConnTracker { return globalConnTracker }

// TrackedConnection 被追踪的连接（对应 statistic.tracker）
type TrackedConnection struct {
	ID       string    `json:"id"`
	Network  string    `json:"network"`
	DstAddr  string    `json:"dst"`
	Rule     string    `json:"rule"`
	Proxy    string    `json:"proxy"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Start    time.Time `json:"start"`
	conn     net.Conn
}

func (t *ConnTracker) Track(conn net.Conn, metadata *Metadata, rule, proxy string) *TrackedConnection {
	id := fmt.Sprintf("%016x", t.seq.Add(1))
	tc := &TrackedConnection{
		ID:      id,
		Network: metadata.Network,
		DstAddr: metadata.RemoteAddress(),
		Rule:    rule,
		Proxy:   proxy,
		Start:   time.Now(),
		conn:    conn,
	}
	t.mu.Lock()
	t.conns[id] = tc
	t.mu.Unlock()
	return tc
}

func (t *ConnTracker) Remove(id string) {
	t.mu.Lock()
	delete(t.conns, id)
	t.mu.Unlock()
}

func (t *ConnTracker) List() []*TrackedConnection {
	t.mu.RLock()
	defer t.mu.RUnlock()
	list := make([]*TrackedConnection, 0, len(t.conns))
	for _, c := range t.conns {
		list = append(list, c)
	}
	return list
}

func (t *ConnTracker) CloseAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.conns {
		_ = c.conn.Close()
	}
	t.conns = make(map[string]*TrackedConnection)
}

// ── 统计连接包装器 ────────────────────────────────────────────────────────────

// statisticConn 在 net.Conn 上叠加流量计数（对应 statistic.tcpTracker）
type statisticConn struct {
	net.Conn
	tc *TrackedConnection
}

func NewStatisticConn(conn net.Conn, tc *TrackedConnection) net.Conn {
	return &statisticConn{Conn: conn, tc: tc}
}

func (c *statisticConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.tc.Download += int64(n)
		globalStats.DownloadTotal.Add(int64(n))
	}
	return n, err
}

func (c *statisticConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.tc.Upload += int64(n)
		globalStats.UploadTotal.Add(int64(n))
	}
	return n, err
}

func (c *statisticConn) Close() error {
	globalConnTracker.Remove(c.tc.ID)
	return c.Conn.Close()
}

// ── Protocol Sniffer（嗅探）──────────────────────────────────────────────────
// 参照 mihomo/component/sniffer
// 通过读取连接最初的几个字节来识别协议，提取目标 Host/SNI

// Sniffer 协议嗅探器接口
type Sniffer interface {
	SniffTCP(b []byte) (host string, err error)
}

// HTTPSniffer HTTP 明文嗅探器（提取 Host 头）
// 对应 mihomo/component/sniffer/http.go
type HTTPSniffer struct{}

func (s *HTTPSniffer) SniffTCP(b []byte) (string, error) {
	if len(b) < 4 {
		return "", fmt.Errorf("too short")
	}
	// 检查 HTTP 方法
	methods := []string{"GET ", "POST", "PUT ", "HEAD", "DELE", "OPTI", "CONN", "PATC", "TRAC"}
	matched := false
	prefix := string(b[:4])
	for _, m := range methods {
		if prefix == m {
			matched = true
			break
		}
	}
	if !matched {
		return "", fmt.Errorf("not HTTP")
	}
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(b)))
	if err != nil {
		return "", err
	}
	host := req.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host, nil
}

// TLSSniffer TLS SNI 嗅探器（从 ClientHello 提取 SNI）
// 对应 mihomo/component/sniffer/tls.go
type TLSSniffer struct{}

func (s *TLSSniffer) SniffTCP(b []byte) (string, error) {
	// TLS ClientHello：
	//   0: ContentType = 22 (Handshake)
	//   1-2: Version (0x03 0x01/02/03)
	//   3-4: Length
	//   5: HandshakeType = 1 (ClientHello)
	//   ... SNI extension
	if len(b) < 5 || b[0] != 0x16 || b[1] != 0x03 {
		return "", fmt.Errorf("not TLS")
	}
	if len(b) < 9 || b[5] != 0x01 {
		return "", fmt.Errorf("not ClientHello")
	}
	return extractTLSSNI(b[5:])
}

// extractTLSSNI 从 TLS Handshake 消息中提取 SNI
func extractTLSSNI(data []byte) (string, error) {
	if len(data) < 38 {
		return "", fmt.Errorf("too short for ClientHello")
	}
	// HandshakeType(1) + Length(3) + Version(2) + Random(32) = 38
	pos := 4 + 2 + 32 // skip HandshakeType(1) + Length(3) + Version(2) + Random(32)
	if pos >= len(data) {
		return "", fmt.Errorf("parse error at session id")
	}
	// Session ID
	sessionIDLen := int(data[pos])
	pos += 1 + sessionIDLen
	if pos+2 > len(data) {
		return "", fmt.Errorf("parse error at cipher suites")
	}
	// Cipher Suites
	csLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2 + csLen
	if pos+1 > len(data) {
		return "", fmt.Errorf("parse error at compression")
	}
	// Compression Methods
	cmLen := int(data[pos])
	pos += 1 + cmLen
	if pos+2 > len(data) {
		return "", fmt.Errorf("no extensions")
	}
	// Extensions
	extLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2
	extEnd := pos + extLen
	for pos+4 <= extEnd && pos+4 <= len(data) {
		extType := binary.BigEndian.Uint16(data[pos : pos+2])
		extDataLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4
		if extType == 0x00 && pos+extDataLen <= len(data) {
			// SNI extension (type 0)
			sniData := data[pos : pos+extDataLen]
			if len(sniData) < 5 {
				break
			}
			listLen := int(binary.BigEndian.Uint16(sniData[0:2]))
			if listLen+2 > len(sniData) {
				break
			}
			nameType := sniData[2]
			nameLen := int(binary.BigEndian.Uint16(sniData[3:5]))
			if nameType == 0 && 5+nameLen <= len(sniData) {
				return string(sniData[5 : 5+nameLen]), nil
			}
		}
		pos += extDataLen
	}
	return "", fmt.Errorf("SNI not found")
}

// SniffConn 对连接进行协议嗅探，更新 Metadata.DstHost
// 对应 mihomo/component/sniffer/dispatcher.go
type sniffConn struct {
	net.Conn
	buf    []byte // 已 peek 的字节
	offset int
}

func NewSniffConn(conn net.Conn) *sniffConn {
	return &sniffConn{Conn: conn}
}

// Peek 读取最多 n 字节但不消耗（用于嗅探）
func (c *sniffConn) Peek(n int) ([]byte, error) {
	if len(c.buf) >= n {
		return c.buf[:n], nil
	}
	tmp := make([]byte, n-len(c.buf))
	m, err := c.Conn.Read(tmp)
	c.buf = append(c.buf, tmp[:m]...)
	if err != nil && len(c.buf) == 0 {
		return nil, err
	}
	return c.buf, nil
}

func (c *sniffConn) Read(b []byte) (int, error) {
	if c.offset < len(c.buf) {
		n := copy(b, c.buf[c.offset:])
		c.offset += n
		return n, nil
	}
	return c.Conn.Read(b)
}

// SniffAndUpdate 嗅探协议并更新 Metadata.DstHost（如果原来为空）
func SniffAndUpdate(conn *sniffConn, metadata *Metadata) {
	if metadata.DstHost != "" {
		return // 已有域名，不需要嗅探
	}
	peek, err := conn.Peek(300)
	if err != nil || len(peek) == 0 {
		return
	}
	sniffers := []Sniffer{&TLSSniffer{}, &HTTPSniffer{}}
	for _, s := range sniffers {
		host, err := s.SniffTCP(peek)
		if err == nil && host != "" && !strings.Contains(host, ":") {
			metadata.DstHost = host
			return
		}
	}
}
