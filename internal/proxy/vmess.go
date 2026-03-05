package proxy

// vmess.go — VMess AEAD 出站实现
//
// 参照 mihomo/transport/vmess/conn.go + vmess.go 实现。
// 协议规范：https://xtls.github.io/development/protocols/vmess.html

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5" //nolint:gosec
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

// VMess 协议常量
const (
	vmessVersion        byte = 1
	vmessOptChunkStream byte = 0x01
	vmessCmdTCP         byte = 0x01
	vmessAtypDomain     byte = 0x02
	vmessAtypIPv4       byte = 0x01
	vmessAtypIPv6       byte = 0x03
	vmessSecAES128GCM   byte = 0x03
	vmessSecChaCha20    byte = 0x04
	vmessSecNone        byte = 0x05
)

// vmessKDF salt 常量
var (
	kdfSaltAEADKeys             = []byte("VMess AEAD KDF")
	kdfSaltAEADHeaderPayKey     = []byte("VMess Header AEAD Key")
	kdfSaltAEADHeaderPayIV      = []byte("VMess Header AEAD Nonce")
	kdfSaltAEADHeaderLenKey     = []byte("VMess Header AEAD Key_Length")
	kdfSaltAEADHeaderLenIV      = []byte("VMess Header AEAD Nonce_Length")
	kdfSaltAEADRespHeaderLenKey = []byte("AEAD Resp Header Len Key")
	kdfSaltAEADRespHeaderLenIV  = []byte("AEAD Resp Header Len IV")
	kdfSaltAEADRespHeaderKey    = []byte("AEAD Resp Header Key")
	kdfSaltAEADRespHeaderIV     = []byte("AEAD Resp Header IV")
)

// VMessOutbound VMess 出站代理
type VMessOutbound struct {
	name     string
	server   string
	uuid     string
	alterID  int
	security string
	tls      bool
	sni      string
	skipCert bool
}

func NewVMessOutbound(name, server, uuid, security string, alterID int, useTLS bool, sni string, skipCert bool) *VMessOutbound {
	return &VMessOutbound{
		name: name, server: server, uuid: uuid,
		alterID: alterID, security: security,
		tls: useTLS, sni: sni, skipCert: skipCert,
	}
}

func (v *VMessOutbound) Name() string { return v.name }

func (v *VMessOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", v.server)
	if err != nil {
		return nil, fmt.Errorf("connect to vmess server %s: %w", v.server, err)
	}

	if v.tls {
		conn, err = wrapTLSClient(conn, ctx, v.sni, v.server, v.skipCert)
		if err != nil {
			conn.Close()
			return nil, err
		}
	}

	id, err := parseVMessID(v.uuid)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("parse vmess uuid: %w", err)
	}

	sec := vmessPickSecurity(v.security)
	vc, err := newVMessConn(conn, id, metadata, sec)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("vmess handshake: %w", err)
	}
	return vc, nil
}

// ── VMess ID ─────────────────────────────────────────────────────────────────

type vmessID struct {
	UUID   [16]byte
	CmdKey [16]byte
}

func parseVMessID(uuidStr string) (*vmessID, error) {
	raw, err := parseUUID(uuidStr)
	if err != nil {
		return nil, err
	}
	id := &vmessID{UUID: raw}
	h := md5.New() //nolint:gosec
	h.Write(raw[:])
	h.Write([]byte("c48619fe-8f02-49e0-b9e9-edf763e17e21"))
	copy(id.CmdKey[:], h.Sum(nil))
	return id, nil
}

func parseUUID(s string) ([16]byte, error) {
	var raw [16]byte
	if len(s) != 36 {
		return raw, fmt.Errorf("invalid uuid length: %d", len(s))
	}
	h := s[:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
	if len(h) != 32 {
		return raw, fmt.Errorf("invalid uuid format")
	}
	for i := 0; i < 16; i++ {
		b, err := hexByte(h[i*2], h[i*2+1])
		if err != nil {
			return raw, err
		}
		raw[i] = b
	}
	return raw, nil
}

func hexByte(hi, lo byte) (byte, error) {
	h, err := hexNibble(hi)
	if err != nil {
		return 0, err
	}
	l, err := hexNibble(lo)
	if err != nil {
		return 0, err
	}
	return h<<4 | l, nil
}

func hexNibble(c byte) (byte, error) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', nil
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, nil
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, nil
	}
	return 0, fmt.Errorf("invalid hex char: %c", c)
}

func vmessPickSecurity(s string) byte {
	switch s {
	case "aes-128-gcm":
		return vmessSecAES128GCM
	case "chacha20-poly1305":
		return vmessSecChaCha20
	case "none":
		return vmessSecNone
	default:
		return vmessSecAES128GCM
	}
}

// ── VMess 连接 ────────────────────────────────────────────────────────────────

type vmessConn struct {
	net.Conn
	reader     io.Reader
	writer     io.Writer
	received   bool
	reqBodyIV  []byte
	reqBodyKey []byte
	respV      byte
	security   byte
}

func newVMessConn(conn net.Conn, id *vmessID, metadata *Metadata, security byte) (*vmessConn, error) {
	reqBodyIV := make([]byte, 16)
	reqBodyKey := make([]byte, 16)
	_, _ = rand.Read(reqBodyIV)
	_, _ = rand.Read(reqBodyKey)
	respV := randByte()

	vc := &vmessConn{
		Conn:       conn,
		reqBodyIV:  reqBodyIV,
		reqBodyKey: reqBodyKey,
		respV:      respV,
		security:   security,
	}

	respBodyIV := md5Sum(reqBodyIV)
	respBodyKey := md5Sum(reqBodyKey)

	if err := vc.sendAEADRequest(id, metadata); err != nil {
		return nil, err
	}

	vc.writer = vc.newBodyWriter(security, reqBodyKey, reqBodyIV)
	vc.reader = vc.newBodyReader(security, respBodyKey, respBodyIV)
	return vc, nil
}

func (vc *vmessConn) Write(b []byte) (int, error) { return vc.writer.Write(b) }
func (vc *vmessConn) Read(b []byte) (int, error) {
	if !vc.received {
		if err := vc.recvAEADResponse(); err != nil {
			return 0, err
		}
		vc.received = true
	}
	return vc.reader.Read(b)
}

// sendAEADRequest 发送 VMess AEAD 请求头
func (vc *vmessConn) sendAEADRequest(id *vmessID, metadata *Metadata) error {
	timestamp := time.Now().Unix()

	buf := &bytes.Buffer{}
	buf.WriteByte(vmessVersion)
	buf.Write(vc.reqBodyIV)
	buf.Write(vc.reqBodyKey)
	buf.WriteByte(vc.respV)
	buf.WriteByte(vmessOptChunkStream)

	p := randByte() & 0x0F
	buf.WriteByte(byte(p<<4) | vc.security)
	buf.WriteByte(0x00)
	buf.WriteByte(vmessCmdTCP)

	binary.Write(buf, binary.BigEndian, metadata.DstPort) //nolint:errcheck

	if metadata.DstHost != "" {
		buf.WriteByte(vmessAtypDomain)
		buf.WriteByte(byte(len(metadata.DstHost)))
		buf.WriteString(metadata.DstHost)
	} else if ip4 := metadata.DstIP.To4(); ip4 != nil {
		buf.WriteByte(vmessAtypIPv4)
		buf.Write(ip4)
	} else {
		buf.WriteByte(vmessAtypIPv6)
		buf.Write(metadata.DstIP.To16())
	}

	paddingBytes := make([]byte, p)
	_, _ = rand.Read(paddingBytes)
	buf.Write(paddingBytes)

	fnv1a := fnv.New32a()
	fnv1a.Write(buf.Bytes())
	buf.Write(fnv1a.Sum(nil))

	var cmdKey [16]byte
	copy(cmdKey[:], id.CmdKey[:])
	sealed := sealVMessAEADHeader(cmdKey, buf.Bytes(), timestamp)
	_, err := vc.Conn.Write(sealed)
	return err
}

// recvAEADResponse 解密服务器响应头
func (vc *vmessConn) recvAEADResponse() error {
	lenKey := vmessKDF(vc.reqBodyKey, kdfSaltAEADRespHeaderLenKey)[:16]
	lenIV := vmessKDF(vc.reqBodyIV, kdfSaltAEADRespHeaderLenIV)[:12]
	lenBlock, _ := aes.NewCipher(lenKey)
	lenAEAD, _ := cipher.NewGCM(lenBlock)

	encLen := make([]byte, 18)
	if _, err := io.ReadFull(vc.Conn, encLen); err != nil {
		return fmt.Errorf("read response length: %w", err)
	}
	decLen, err := lenAEAD.Open(nil, lenIV, encLen, nil)
	if err != nil {
		return fmt.Errorf("decrypt response length: %w", err)
	}
	headerLen := binary.BigEndian.Uint16(decLen)

	payKey := vmessKDF(vc.reqBodyKey, kdfSaltAEADRespHeaderKey)[:16]
	payIV := vmessKDF(vc.reqBodyIV, kdfSaltAEADRespHeaderIV)[:12]
	payBlock, _ := aes.NewCipher(payKey)
	payAEAD, _ := cipher.NewGCM(payBlock)

	encHeader := make([]byte, int(headerLen)+16)
	if _, err := io.ReadFull(vc.Conn, encHeader); err != nil {
		return fmt.Errorf("read response header: %w", err)
	}
	header, err := payAEAD.Open(nil, payIV, encHeader, nil)
	if err != nil {
		return fmt.Errorf("decrypt response header: %w", err)
	}
	if len(header) < 4 || header[0] != vc.respV {
		return fmt.Errorf("response V mismatch")
	}
	return nil
}

// ── Body 加密层 ───────────────────────────────────────────────────────────────

func (vc *vmessConn) newBodyWriter(security byte, key, iv []byte) io.Writer {
	switch security {
	case vmessSecAES128GCM:
		return newVMessAEADWriter(vc.Conn, key, iv, newAESGCM)
	case vmessSecChaCha20:
		return newVMessAEADWriter(vc.Conn, key, iv, func(k []byte) (cipher.AEAD, error) {
			return chacha20poly1305.New(k)
		})
	default:
		return vc.Conn
	}
}

func (vc *vmessConn) newBodyReader(security byte, key, iv []byte) io.Reader {
	switch security {
	case vmessSecAES128GCM:
		return newVMessAEADReader(vc.Conn, key, iv, newAESGCM)
	case vmessSecChaCha20:
		return newVMessAEADReader(vc.Conn, key, iv, func(k []byte) (cipher.AEAD, error) {
			return chacha20poly1305.New(k)
		})
	default:
		return vc.Conn
	}
}

type vmessAEADWriter struct {
	conn  net.Conn
	aead  cipher.AEAD
	nonce []byte
}

func newVMessAEADWriter(conn net.Conn, key, iv []byte, newAEAD aeadConstructor) *vmessAEADWriter {
	aead, _ := newAEAD(key[:16])
	nonce := make([]byte, aead.NonceSize())
	copy(nonce, iv[:aead.NonceSize()])
	return &vmessAEADWriter{conn: conn, aead: aead, nonce: nonce}
}

func (w *vmessAEADWriter) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		chunk := b
		if len(chunk) > ssMaxChunkSize {
			chunk = b[:ssMaxChunkSize]
		}
		lenBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBuf, uint16(len(chunk)+ssTagSize))
		encLen := w.aead.Seal(nil, w.nonce, lenBuf, nil)
		incrementNonce(w.nonce)
		encData := w.aead.Seal(nil, w.nonce, chunk, nil)
		incrementNonce(w.nonce)
		if _, err := w.conn.Write(append(encLen, encData...)); err != nil {
			return total, err
		}
		total += len(chunk)
		b = b[len(chunk):]
	}
	return total, nil
}

type vmessAEADReader struct {
	conn    net.Conn
	aead    cipher.AEAD
	nonce   []byte
	readBuf []byte
}

func newVMessAEADReader(conn net.Conn, key, iv []byte, newAEAD aeadConstructor) *vmessAEADReader {
	aead, _ := newAEAD(key[:16])
	nonce := make([]byte, aead.NonceSize())
	copy(nonce, iv[:aead.NonceSize()])
	return &vmessAEADReader{conn: conn, aead: aead, nonce: nonce}
}

func (r *vmessAEADReader) Read(b []byte) (int, error) {
	if len(r.readBuf) > 0 {
		n := copy(b, r.readBuf)
		r.readBuf = r.readBuf[n:]
		return n, nil
	}
	tagSize := r.aead.Overhead()
	encLen := make([]byte, 2+tagSize)
	if _, err := io.ReadFull(r.conn, encLen); err != nil {
		return 0, err
	}
	lenBuf, err := r.aead.Open(encLen[:0], r.nonce, encLen, nil)
	if err != nil {
		return 0, err
	}
	incrementNonce(r.nonce)
	dataLen := int(binary.BigEndian.Uint16(lenBuf)) - tagSize
	encData := make([]byte, dataLen+tagSize)
	if _, err := io.ReadFull(r.conn, encData); err != nil {
		return 0, err
	}
	data, err := r.aead.Open(encData[:0], r.nonce, encData, nil)
	if err != nil {
		return 0, err
	}
	incrementNonce(r.nonce)
	r.readBuf = data
	n := copy(b, r.readBuf)
	r.readBuf = r.readBuf[n:]
	return n, nil
}

// ── AEAD Header 加密 ──────────────────────────────────────────────────────────

func sealVMessAEADHeader(cmdKey [16]byte, header []byte, timestamp int64) []byte {
	connNonce := make([]byte, 8)
	_, _ = rand.Read(connNonce)

	lenKey := vmessKDFWithTimestamp(cmdKey[:], connNonce, timestamp, kdfSaltAEADHeaderLenKey)[:16]
	lenIV := vmessKDFWithTimestamp(cmdKey[:], connNonce, timestamp, kdfSaltAEADHeaderLenIV)[:12]
	lenBlock, _ := aes.NewCipher(lenKey)
	lenAEAD, _ := cipher.NewGCM(lenBlock)
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(header)))
	encLen := lenAEAD.Seal(nil, lenIV, lenBuf, nil)

	payKey := vmessKDFWithTimestamp(cmdKey[:], connNonce, timestamp, kdfSaltAEADHeaderPayKey)[:16]
	payIV := vmessKDFWithTimestamp(cmdKey[:], connNonce, timestamp, kdfSaltAEADHeaderPayIV)[:12]
	payBlock, _ := aes.NewCipher(payKey)
	payAEAD, _ := cipher.NewGCM(payBlock)
	encPayload := payAEAD.Seal(nil, payIV, header, nil)

	authID := vmessAEADAuthID(cmdKey, timestamp)

	out := make([]byte, 0, 16+8+len(encLen)+len(encPayload))
	out = append(out, authID[:]...)
	out = append(out, connNonce...)
	out = append(out, encLen...)
	out = append(out, encPayload...)
	return out
}

func vmessAEADAuthID(cmdKey [16]byte, timestamp int64) [16]byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint64(buf[:8], uint64(timestamp))
	_, _ = rand.Read(buf[8:12])
	h := fnv.New32a()
	h.Write(buf[:12])
	binary.BigEndian.PutUint32(buf[8:], h.Sum32())

	key := vmessKDF(cmdKey[:], []byte("AES Auth ID Encryption"))[:16]
	block, _ := aes.NewCipher(key)
	var out [16]byte
	block.Encrypt(out[:], buf)
	return out
}

// ── KDF ──────────────────────────────────────────────────────────────────────

// vmessKDF 使用 HMAC-SHA256 多层派生（对应 mihomo/transport/vmess/kdf.go）
func vmessKDF(key []byte, salts ...[]byte) []byte {
	// 外层 HMAC key = kdfSaltAEADKeys
	h := hmac.New(sha256.New, kdfSaltAEADKeys)
	h.Write(key)
	derived := h.Sum(nil)
	for _, salt := range salts {
		h2 := hmac.New(sha256.New, derived)
		h2.Write(salt)
		derived = h2.Sum(nil)
	}
	return derived
}

func vmessKDFWithTimestamp(key, connNonce []byte, timestamp int64, salt []byte) []byte {
	tsBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBuf, uint64(timestamp))
	h := hmac.New(sha256.New, key)
	h.Write(salt)
	h.Write(tsBuf)
	h.Write(connNonce)
	return h.Sum(nil)
}

// ── 辅助 ──────────────────────────────────────────────────────────────────────

func md5Sum(b []byte) []byte {
	h := md5.New() //nolint:gosec
	h.Write(b)
	return h.Sum(nil)
}

func incrementNonce(nonce []byte) {
	for i := range nonce {
		nonce[i]++
		if nonce[i] != 0 {
			break
		}
	}
}

func randByte() byte {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return b[0]
}
