package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// deriveKey 从配置文件路径派生固定密钥（同机器同用户始终相同）
func deriveKey() []byte {
	home, _ := os.UserHomeDir()
	seed := filepath.Join(home, ".config", "ccnew-vdl", "config.json")
	h := sha256.Sum256([]byte("doubi-cookie-encrypt-v1:" + seed))
	return h[:] // 32字节 AES-256
}

// Encrypt 明文 → base64( nonce | ciphertext | tag )
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key := deriveKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes.NewCipher: %w", err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher.NewGCM: %w", err)
	}
	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	ciphertext := aesgcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt base64( nonce | ciphertext | tag ) → 明文
// 兼容：非enc:前缀的旧明文直接返回
func Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if len(encoded) < 4 || encoded[:4] != "enc:" {
		return encoded, nil // 兼容旧明文
	}
	raw, err := base64.StdEncoding.DecodeString(encoded[4:])
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	key := deriveKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes.NewCipher: %w", err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher.NewGCM: %w", err)
	}
	nonceSize := aesgcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
