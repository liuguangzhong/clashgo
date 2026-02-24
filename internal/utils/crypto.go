// Package utils - 加密工具（对应原 utils/help.rs AES-GCM 加密逻辑）
// 用于加密存储 WebDAV 密码等敏感信息

package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// masterKeySource 生成加密主密钥的来源（可以是机器ID或固定字符串）
// 在生产环境中应从 OS keychain 获取
var masterKeySource = "clashgo-aes-gcm-key-v1"

// deriveMasterKey 从固定来源派生 AES-256 密钥
func deriveMasterKey() []byte {
	hash := sha256.Sum256([]byte(masterKeySource))
	return hash[:]
}

// EncryptString 使用 AES-256-GCM 加密字符串，返回 base64 编码的密文
func EncryptString(plaintext string) (string, error) {
	key := deriveMasterKey()

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	// 随机 nonce（12 字节）
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// 加密：nonce || ciphertext
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptString 解密 EncryptString 产生的密文
func DecryptString(encoded string) (string, error) {
	key := deriveMasterKey()

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// MaskSensitive 对敏感字符串做部分遮盖（用于日志）
func MaskSensitive(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}
