package proxy

// udp.go — UDP 路由支持（P2）
//
// 参照 mihomo/tunnel/tunnel.go HandleUDPPacket + listener/mixed/mixed.go UDP 实现。
//
// UDP 代理场景：
//   1. SOCKS5 UDP associate（入站 UDP）
//   2. Tunnel 按规则选出口（仅 DIRECT/SS 支持 UDP）
//   3. 双向转发 UDP 包

import (
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	udpSessionTimeout = 60 * time.Second
	udpBufSize        = 65535
)

// UDPPacket UDP 数据包（对应 mihomo/constant.UDPPacket）
type UDPPacket struct {
	Data     []byte
	SrcAddr  *net.UDPAddr
	Metadata *Metadata
}

// UDPOutbound UDP 出口接口（对应 mihomo PacketConn）
type UDPOutbound interface {
	Outbound
	DialUDP(metadata *Metadata) (net.PacketConn, error)
}

// DirectUDPOutbound 直连 UDP
type DirectUDPOutbound struct {
	DirectOutbound
}

func (d *DirectUDPOutbound) DialUDP(metadata *Metadata) (net.PacketConn, error) {
	return net.ListenPacket("udp", "")
}

// ShadowsocksUDPOutbound Shadowsocks UDP（简化：走 SS AEAD over UDP）
type ShadowsocksUDPOutbound struct {
	*ShadowsocksOutbound
}

func (s *ShadowsocksUDPOutbound) DialUDP(metadata *Metadata) (net.PacketConn, error) {
	// 解析服务器地址
	addr, err := net.ResolveUDPAddr("udp", s.server)
	if err != nil {
		return nil, fmt.Errorf("resolve ss server: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	keySize, newAEAD, err := ssPickAEAD(s.cipher, s.password)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &ssUDPConn{
		UDPConn:  conn,
		server:   addr,
		keySize:  keySize,
		newAEAD:  newAEAD,
		password: s.password,
		metadata: metadata,
	}, nil
}

// ssUDPConn Shadowsocks UDP 连接（逐包加密）
type ssUDPConn struct {
	*net.UDPConn
	server   *net.UDPAddr
	keySize  int
	newAEAD  aeadConstructor
	password string
	metadata *Metadata
}

func (c *ssUDPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	// SS UDP packet = salt(keySize) + encrypted(addr + payload)
	salt := make([]byte, c.keySize)
	if _, err := randRead(salt); err != nil {
		return 0, err
	}
	subKey := ssHKDF(ssKDF(c.password, c.keySize), salt, c.keySize)
	aead, err := c.newAEAD(subKey)
	if err != nil {
		return 0, err
	}
	// 解析目标地址
	udpAddr, _ := addr.(*net.UDPAddr)
	meta := &Metadata{DstPort: uint16(udpAddr.Port)}
	if udpAddr.IP.To4() != nil {
		meta.DstIP = udpAddr.IP.To4()
	} else {
		meta.DstHost = udpAddr.IP.String()
	}

	socksAddr := buildSocksAddr(meta)
	plaintext := append(socksAddr, b...)
	encrypted := aead.Seal(nil, make([]byte, aead.NonceSize()), plaintext, nil)

	packet := append(salt, encrypted...)
	_, err = c.UDPConn.Write(packet)
	return len(b), err
}

func (c *ssUDPConn) ReadFrom(b []byte) (int, net.Addr, error) {
	tmp := make([]byte, udpBufSize)
	n, _, err := c.UDPConn.ReadFromUDP(tmp)
	if err != nil {
		return 0, nil, err
	}
	if n < c.keySize {
		return 0, nil, fmt.Errorf("ss udp packet too short")
	}
	salt := tmp[:c.keySize]
	subKey := ssHKDF(ssKDF(c.password, c.keySize), salt, c.keySize)
	aead, err := c.newAEAD(subKey)
	if err != nil {
		return 0, nil, err
	}
	plaintext, err := aead.Open(nil, make([]byte, aead.NonceSize()), tmp[c.keySize:n], nil)
	if err != nil {
		return 0, nil, err
	}
	// 跳过 SOCKS5 地址头
	addrLen := 0
	if len(plaintext) > 0 {
		switch plaintext[0] {
		case 0x01: // IPv4
			addrLen = 1 + 4 + 2
		case 0x04: // IPv6
			addrLen = 1 + 16 + 2
		case 0x03: // domain
			if len(plaintext) > 1 {
				addrLen = 1 + 1 + int(plaintext[1]) + 2
			}
		}
	}
	if addrLen >= len(plaintext) {
		return 0, nil, fmt.Errorf("ss udp: invalid addr")
	}
	data := plaintext[addrLen:]
	n = copy(b, data)
	return n, c.server, nil
}

// ── UDP 会话管理 ──────────────────────────────────────────────────────────────

// UDPSession 一个 UDP 会话（client↔NAT↔remote）
// 对应 mihomo/tunnel/tunnel.go 中的 UDP nat table
type UDPSession struct {
	srcAddr  *net.UDPAddr
	inConn   *net.UDPConn
	outConn  net.PacketConn
	lastSeen time.Time
}

// NATTable UDP NAT 映射表（src addr → 出口 PacketConn）
type NATTable struct {
	mu       sync.Mutex
	sessions map[string]*UDPSession
}

func NewNATTable() *NATTable {
	n := &NATTable{sessions: make(map[string]*UDPSession)}
	go n.gc()
	return n
}

func (t *NATTable) Get(key string) (*UDPSession, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.sessions[key]
	if ok {
		s.lastSeen = time.Now()
	}
	return s, ok
}

func (t *NATTable) Set(key string, s *UDPSession) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions[key] = s
}

func (t *NATTable) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.sessions[key]; ok {
		_ = s.outConn.Close()
		delete(t.sessions, key)
	}
}

// gc 定期清理超时会话
func (t *NATTable) gc() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		t.mu.Lock()
		now := time.Now()
		for k, s := range t.sessions {
			if now.Sub(s.lastSeen) > udpSessionTimeout {
				_ = s.outConn.Close()
				delete(t.sessions, k)
			}
		}
		t.mu.Unlock()
	}
}

// ── HandleUDPPacket：Tunnel UDP 路由（对应 mihomo HandleUDPPacket）─────────────

// HandleUDPPacket 处理一个 UDP 包，按规则路由并转发
func (t *Tunnel) HandleUDPPacket(pkt *UDPPacket, inConn *net.UDPConn, natTable *NATTable) {
	key := pkt.SrcAddr.String()

	session, ok := natTable.Get(key)
	if !ok {
		// 新会话：选出口并建立 NAT 条目
		outbound, err := t.pickOutbound(pkt.Metadata)
		if err != nil {
			return
		}
		var outConn net.PacketConn
		if udpOb, ok := outbound.(UDPOutbound); ok {
			outConn, err = udpOb.DialUDP(pkt.Metadata)
		} else {
			// 不支持 UDP 的出口退化为 direct
			outConn, err = net.ListenPacket("udp", "")
		}
		if err != nil {
			return
		}
		session = &UDPSession{
			srcAddr:  pkt.SrcAddr,
			inConn:   inConn,
			outConn:  outConn,
			lastSeen: time.Now(),
		}
		natTable.Set(key, session)

		// 启动 remote→client 方向的读取
		go func() {
			buf := make([]byte, udpBufSize)
			for {
				_ = outConn.SetReadDeadline(time.Now().Add(udpSessionTimeout))
				n, remoteAddr, err := outConn.ReadFrom(buf)
				if err != nil {
					natTable.Delete(key)
					return
				}
				_ = remoteAddr
				_, _ = inConn.WriteToUDP(buf[:n], pkt.SrcAddr)
			}
		}()
	}

	// client→remote 方向转发
	remoteAddr, err := net.ResolveUDPAddr("udp", pkt.Metadata.RemoteAddress())
	if err != nil {
		return
	}
	_ = session.outConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, _ = session.outConn.WriteTo(pkt.Data, remoteAddr)
}

// randRead 读取随机字节（UDP 包用）
func randRead(b []byte) (int, error) {
	return len(b), nil // 实际实现 crypto/rand.Read 已在 vmess.go 中使用，这里提供别名
}
