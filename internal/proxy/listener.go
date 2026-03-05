package proxy

// listener.go — Mixed HTTP/SOCKS5 监听器
//
// 参照 mihomo/listener/mixed/mixed.go 实现。
// Mixed Listener 在同一端口上同时支持 HTTP 代理和 SOCKS5 代理，
// 通过读取第一个字节（0x04/0x05 = SOCKS，其他 = HTTP）来分发。

import (
	"bufio"
	"net"
	"sync"
	"sync/atomic"
)

// MixedListener HTTP/SOCKS5 混合监听器（对应 mihomo/listener/mixed.Listener）
type MixedListener struct {
	listener net.Listener
	tunnel   *Tunnel
	addr     string

	closed atomic.Bool
	wg     sync.WaitGroup
}

// NewMixedListener 在 addr 上启动混合监听（对应 mihomo/listener/mixed.New）
func NewMixedListener(addr string, tunnel *Tunnel) (*MixedListener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	ml := &MixedListener{
		listener: ln,
		tunnel:   tunnel,
		addr:     addr,
	}
	go ml.acceptLoop()
	return ml, nil
}

// Address 返回实际监听地址
func (m *MixedListener) Address() string {
	return m.listener.Addr().String()
}

// Close 关闭监听器，等待所有活跃连接处理完
func (m *MixedListener) Close() error {
	m.closed.Store(true)
	err := m.listener.Close()
	m.wg.Wait()
	return err
}

// acceptLoop 主循环：接受连接并分发（对应 mihomo mixed.go 的 goroutine）
func (m *MixedListener) acceptLoop() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			if m.closed.Load() {
				return
			}
			continue
		}
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.dispatch(conn)
		}()
	}
}

// dispatch 读取第一个字节判断协议类型，分发给 HTTP 或 SOCKS 处理器
//
// 对应 mihomo/listener/mixed.handleConn:
//
//	case socks4.Version(0x04): → handleSocks4
//	case socks5.Version(0x05): → handleSocks5
//	default:                   → handleHTTP
func (m *MixedListener) dispatch(conn net.Conn) {
	// 使用 bufio.Reader 实现 Peek：读取首字节不消耗
	br := bufio.NewReader(conn)
	header, err := br.Peek(1)
	if err != nil {
		conn.Close()
		return
	}

	// 包装为带 buffered reader 的连接
	bc := &bufferedConn{Conn: conn, reader: br}

	switch header[0] {
	case 0x04: // SOCKS4
		handleSocks4(bc, m.tunnel)
	case 0x05: // SOCKS5
		handleSocks5(bc, m.tunnel)
	default: // HTTP
		handleHTTP(bc, m.tunnel)
	}
}

// bufferedConn 带缓冲读取的 net.Conn（保留 Peek 过的字节）
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}
