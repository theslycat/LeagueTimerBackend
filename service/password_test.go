package service

import (
	"encoding/hex"
	"testing"
)

// RFC 7914 第 11 节 PBKDF2-HMAC-SHA256 标准测试向量
func TestPbkdf2SHA256Vectors(t *testing.T) {
	cases := []struct {
		password   string
		salt       string
		iterations int
		want       string
	}{
		{"password", "salt", 1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{"password", "salt", 2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{"password", "salt", 4096, "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
	}
	for _, c := range cases {
		got, err := pbkdf2SHA256(c.password, []byte(c.salt), c.iterations, pbkdf2KeySize)
		if err != nil {
			t.Fatalf("pbkdf2SHA256(%q, %q, %d) error: %v", c.password, c.salt, c.iterations, err)
		}
		if gotHex := hex.EncodeToString(got); gotHex != c.want {
			t.Errorf("pbkdf2SHA256(%q, %q, %d) = %s, want %s",
				c.password, c.salt, c.iterations, gotHex, c.want)
		}
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("league-timer-2026")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if !VerifyPassword("league-timer-2026", hash) {
		t.Error("VerifyPassword with correct password should pass")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Error("VerifyPassword with wrong password should fail")
	}
	if VerifyPassword("league-timer-2026", "not-a-valid-hash") {
		t.Error("VerifyPassword with malformed stored hash should fail")
	}
	// 相同密码两次哈希应使用不同随机盐，摘要不同
	hash2, err := HashPassword("league-timer-2026")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == hash2 {
		t.Error("two hashes of the same password should differ due to random salt")
	}
}
