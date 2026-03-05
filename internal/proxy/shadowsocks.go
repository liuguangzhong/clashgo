package proxy

// shadowsocks.go — Shadowsocks AEAD 出站实现
//
// 参照 mihomo/transport/shadowsocks 和 sing-shadowsocks2 实现。
// 支持当前主流 AEAD 加密方式：
//   aes-128-gcm / aes-256-gcm / chacha20-ietf-poly1305
//
// Shadowsocks AEAD 协议规范：
//   https://shadowsocks.org/doc/aead.html
//
// 数据帧格式（TCP）：
//   [salt(keySize)] [encrypted chunks...]
//   每个 chunk = [encrypted length(2B) + tag(16B)] [encrypted payload + tag(16B)]

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec
	"crypto/rand"
	"crypto/sha1" //nolint:gosec
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	ssMaxChunkSize = 0x3FFF // 16383 bytes per chunk（规范上限）
	ssTagSize      = 16     // AEAD tag 长度（GCM/Poly1305）
)

// ShadowsocksOutbound Shadowsocks AEAD 出站代理
// 对应 mihomo/adapter/outbound.ShadowSocks
type ShadowsocksOutbound struct {
	name     string
	server   string // "host:port"
	password string
	cipher   string // "aes-128-gcm" / "aes-256-gcm" / "chacha20-ietf-poly1305"
}

func NewShadowsocksOutbound(name, server, password, cipherName string) *ShadowsocksOutbound {
	return &ShadowsocksOutbound{
		name:     name,
		server:   server,
		password: password,
		cipher:   strings.ToLower(cipherName),
	}
}

func (s *ShadowsocksOutbound) Name() string { return s.name }

func (s *ShadowsocksOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	// 1. 连接 Shadowsocks 服务器
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.server)
	if err != nil {
		return nil, fmt.Errorf("connect to ss server %s: %w", s.server, err)
	}

	// 2. 获取 key 和 AEAD 构造器
	keySize, newAEAD, err := ssPickAEAD(s.cipher, s.password)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// 3. 生成随机 salt 并发送（对应规范第一步）
	salt := make([]byte, keySize)
	if _, err := rand.Read(salt); err != nil {
		conn.Close()
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	if _, err := conn.Write(salt); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write salt: %w", err)
	}

	// 4. 用 HKDF 从 password+salt 派生子密钥，构造 AEAD
	subKey := ssHKDF(ssKDF(s.password, keySize), salt, keySize)
	aead, err := newAEAD(subKey)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create aead: %w", err)
	}

	// 5. 包装为 AEAD 加密连接
	encConn := &ssConn{
		Conn:  conn,
		aead:  aead,
		nonce: make([]byte, aead.NonceSize()),
	}

	// 6. 写入目标地址（SOCKS5 addr 格式）
	if err := encConn.writeSocksAddr(metadata); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write ss addr: %w", err)
	}

	return encConn, nil
}

// ── ssConn: AEAD 加密连接 ─────────────────────────────────────────────────────

type ssConn struct {
	net.Conn
	aead     cipher.AEAD
	nonce    []byte // 递增 nonce（little-endian counter）
	readBuf  []byte // 已解密待读缓冲
	chunkBuf []byte // 读取 chunk 用 buffer
}

func (c *ssConn) incrementNonce() {
	for i := range c.nonce {
		c.nonce[i]++
		if c.nonce[i] != 0 {
			break
		}
	}
}

// Write 加密写入（封装成 AEAD chunks）
func (c *ssConn) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		chunk := b
		if len(chunk) > ssMaxChunkSize {
			chunk = b[:ssMaxChunkSize]
		}
		if err := c.writeChunk(chunk); err != nil {
			return total, err
		}
		total += len(chunk)
		b = b[len(chunk):]
	}
	return total, nil
}

func (c *ssConn) writeChunk(payload []byte) error {
	// 写长度 chunk：[u16be length 加密 + tag]
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(payload)))
	encLen := c.aead.Seal(nil, c.nonce, lenBuf, nil)
	c.incrementNonce()

	// 写数据 chunk：[payload 加密 + tag]
	encPayload := c.aead.Seal(nil, c.nonce, payload, nil)
	c.incrementNonce()

	// 一次写入（减少系统调用）
	out := append(encLen, encPayload...)
	_, err := c.Conn.Write(out)
	return err
}

// Read 解密读取
func (c *ssConn) Read(b []byte) (int, error) {
	for len(c.readBuf) == 0 {
		if err := c.readChunk(); err != nil {
			return 0, err
		}
	}
	n := copy(b, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n, nil
}

func (c *ssConn) readChunk() error {
	tagSize := c.aead.Overhead()

	// 读加密长度（2B + tag）
	encLen := make([]byte, 2+tagSize)
	if _, err := io.ReadFull(c.Conn, encLen); err != nil {
		return err
	}
	lenBuf, err := c.aead.Open(encLen[:0], c.nonce, encLen, nil)
	if err != nil {
		return fmt.Errorf("decrypt length: %w", err)
	}
	c.incrementNonce()

	dataLen := int(binary.BigEndian.Uint16(lenBuf))

	// 读加密数据（dataLen + tag）
	encData := make([]byte, dataLen+tagSize)
	if _, err := io.ReadFull(c.Conn, encData); err != nil {
		return err
	}
	data, err := c.aead.Open(encData[:0], c.nonce, encData, nil)
	if err != nil {
		return fmt.Errorf("decrypt data: %w", err)
	}
	c.incrementNonce()

	c.readBuf = data
	return nil
}

// writeSocksAddr 写入 SOCKS5 格式的目标地址
func (c *ssConn) writeSocksAddr(m *Metadata) error {
	addr := buildSocksAddr(m)
	_, err := c.Write(addr)
	return err
}

// buildSocksAddr 构造 SOCKS5 地址字节（ATYP + addr + port）
func buildSocksAddr(m *Metadata) []byte {
	var buf []byte
	if m.DstHost != "" {
		buf = append(buf, 0x03) // domain
		buf = append(buf, byte(len(m.DstHost)))
		buf = append(buf, []byte(m.DstHost)...)
	} else if ip4 := m.DstIP.To4(); ip4 != nil {
		buf = append(buf, 0x01)
		buf = append(buf, ip4...)
	} else {
		buf = append(buf, 0x04)
		buf = append(buf, m.DstIP.To16()...)
	}
	port := make([]byte, 2)
	binary.BigEndian.PutUint16(port, m.DstPort)
	return append(buf, port...)
}

// ── 加密套件选择 ───────────────────────────────────────────────────────────────

type aeadConstructor func(key []byte) (cipher.AEAD, error)

// ssPickAEAD 根据 cipher 名返回 keySize 和 AEAD 构造函数
func ssPickAEAD(name, password string) (keySize int, newAEAD aeadConstructor, err error) {
	switch strings.ToLower(name) {
	case "aes-128-gcm":
		return 16, newAESGCM, nil
	case "aes-192-gcm":
		return 24, newAESGCM, nil
	case "aes-256-gcm":
		return 32, newAESGCM, nil
	case "chacha20-ietf-poly1305", "chacha20-poly1305":
		return 32, func(key []byte) (cipher.AEAD, error) {
			return chacha20poly1305.New(key)
		}, nil
	case "xchacha20-ietf-poly1305", "xchacha20-poly1305":
		return 32, func(key []byte) (cipher.AEAD, error) {
			return chacha20poly1305.NewX(key)
		}, nil
	default:
		return 0, nil, fmt.Errorf("unsupported shadowsocks cipher: %s", name)
	}
}

func newAESGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ssKDF 从密码派生固定长度的主密钥（Shadowsocks 原始 KDF，使用 MD5 链）
// 对应 core/cipher.go Kdf 函数
func ssKDF(password string, keySize int) []byte {
	var buf []byte
	var prev []byte
	for len(buf) < keySize {
		h := md5.New() //nolint:gosec
		h.Write(prev)
		h.Write([]byte(password))
		prev = h.Sum(nil)
		buf = append(buf, prev...)
	}
	return buf[:keySize]
}

// ssHKDF 用 HKDF-SHA1 从主密钥+salt 派生子密钥
// 对应 shadowaead 中的 HKDF 派生
func ssHKDF(key, salt []byte, keySize int) []byte {
	r := hkdf.New(sha1.New, key, salt, []byte("ss-subkey")) //nolint:gosec
	subKey := make([]byte, keySize)
	_, _ = io.ReadFull(r, subKey)
	return subKey
}

// contextIface 兼容 context.Context 的最小接口（避免 import cycle）
type contextIface interface {
	Done() <-chan struct{}
	Err() error
	Value(key any) any
	Deadline() (interface{ String() string }, bool)
}
