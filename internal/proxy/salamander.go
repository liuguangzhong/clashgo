package proxy

// salamander.go — Hysteria2 Salamander 混淆实现
//
// 参照 Hysteria2 协议规范:
// https://v2.hysteria.network/docs/developers/Protocol/#salamander-obfuscation
//
// 算法：
//   发送: salt = random(8), hash = BLAKE2b-256(key + salt), payload ^= hash
//         packet = [8 bytes salt] + [xor'd payload]
//   接收: salt = packet[:8], hash = BLAKE2b-256(key + salt), payload = packet[8:] ^ hash

import (
	"crypto/rand"
	"net"

	"golang.org/x/crypto/blake2b"
)

const salamanderSaltSize = 8

// SalamanderConn 包装 net.PacketConn，对每个 UDP 包做 Salamander 混淆
type SalamanderConn struct {
	net.PacketConn
	key []byte // obfs-password
}

// NewSalamanderConn 创建混淆连接
func NewSalamanderConn(conn net.PacketConn, password string) *SalamanderConn {
	return &SalamanderConn{
		PacketConn: conn,
		key:        []byte(password),
	}
}

// WriteTo 发送混淆后的数据包
func (c *SalamanderConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	// 1. 生成 8 字节随机 salt
	salt := make([]byte, salamanderSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return 0, err
	}

	// 2. hash = BLAKE2b-256(key + salt)
	hash := calcSalamanderHash(c.key, salt)

	// 3. XOR payload
	obfuscated := make([]byte, salamanderSaltSize+len(p))
	copy(obfuscated[:salamanderSaltSize], salt)
	for i := 0; i < len(p); i++ {
		obfuscated[salamanderSaltSize+i] = p[i] ^ hash[i%32]
	}

	// 4. 发送 [salt][obfuscated payload]
	_, err := c.PacketConn.WriteTo(obfuscated, addr)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// ReadFrom 接收并解混淆数据包
func (c *SalamanderConn) ReadFrom(p []byte) (int, net.Addr, error) {
	buf := make([]byte, 65535)
	n, addr, err := c.PacketConn.ReadFrom(buf)
	if err != nil {
		return 0, nil, err
	}

	if n < salamanderSaltSize {
		// 包太短，丢弃
		return 0, addr, nil
	}

	// 1. 提取 salt
	salt := buf[:salamanderSaltSize]
	payload := buf[salamanderSaltSize:n]

	// 2. hash = BLAKE2b-256(key + salt)
	hash := calcSalamanderHash(c.key, salt)

	// 3. XOR 解混淆
	for i := 0; i < len(payload); i++ {
		payload[i] ^= hash[i%32]
	}

	// 4. 复制到输出缓冲区
	copy(p, payload)
	return len(payload), addr, nil
}

// calcSalamanderHash 计算 BLAKE2b-256(key + salt)
func calcSalamanderHash(key, salt []byte) [32]byte {
	data := make([]byte, len(key)+len(salt))
	copy(data, key)
	copy(data[len(key):], salt)
	return blake2b.Sum256(data)
}
