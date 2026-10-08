package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// appKey 是 AES-256 加密密钥，硬编码在二进制中。
// 只防磁盘文件扫描，不防反编译。对小众桌面应用足够。
var appKey = [32]byte{
	0x7a, 0x3f, 0x71, 0xe2, 0x5c, 0x9d, 0x0b, 0x46,
	0x1a, 0x5f, 0x33, 0xc8, 0x6e, 0x22, 0x4d, 0x0f,
	0x85, 0xce, 0x1c, 0x29, 0x3f, 0xa7, 0x80, 0xf4,
	0x2e, 0x9c, 0x17, 0xd5, 0x4a, 0x8e, 0xd2, 0x06,
}

// Encrypt 使用应用固定密钥，返回与既有 LLM 配置兼容的 nonce + 密文格式。
func Encrypt(plain []byte) ([]byte, error) {
	gcm, err := configCipher()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func Decrypt(data []byte) ([]byte, error) {
	gcm, err := configCipher()
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
}

func configCipher() (cipher.AEAD, error) {
	block, err := aes.NewCipher(appKey[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
