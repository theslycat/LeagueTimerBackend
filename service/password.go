package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// 密码存储参数：PBKDF2-SHA256，10000 轮迭代、16 字节随机盐、派生 32 字节密钥
const (
	pbkdf2Iterations = 10000
	pbkdf2SaltSize   = 16
	pbkdf2KeySize    = 32
)

// pbkdf2SHA256 手写 PBKDF2-HMAC-SHA256（RFC 8018），仅使用标准库，
// 不引入 golang.org/x/crypto。当前只需派生 32 字节（单块）。
func pbkdf2SHA256(password string, salt []byte, iterations, keyLen int) ([]byte, error) {
	if keyLen <= 0 || keyLen > sha256.Size {
		return nil, errors.New("invalid key length")
	}
	if iterations < 1 {
		return nil, errors.New("invalid iterations")
	}
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1}) // INT(1)：大端 4 字节块序号
	u := mac.Sum(nil)
	block := make([]byte, len(u))
	copy(block, u)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range block {
			block[j] ^= u[j]
		}
	}
	return block[:keyLen], nil
}

// HashPassword 生成密码摘要，存储格式：hex(salt) + ":" + hex(hash)
func HashPassword(password string) (string, error) {
	salt := make([]byte, pbkdf2SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2SHA256(password, salt, pbkdf2Iterations, pbkdf2KeySize)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(key), nil
}

// VerifyPassword 校验密码，常数时间比较
func VerifyPassword(password, stored string) bool {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	got, err := pbkdf2SHA256(password, salt, pbkdf2Iterations, len(want))
	if err != nil {
		return false
	}
	return hmac.Equal(got, want)
}
