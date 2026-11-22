package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	claims := &Claims{
		Uid:         7,
		Username:    "deputy_1",
		Role:        "deputy",
		DisplayName: "副裁判甲",
	}
	token, err := GenerateToken(claims)
	if err != nil {
		t.Fatalf("GenerateToken error: %v", err)
	}
	// 格式：base64url(payload) + "." + hex(签名)
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("token should have 2 parts, got %d", len(parts))
	}
	got, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken error: %v", err)
	}
	if got.Uid != claims.Uid || got.Username != claims.Username ||
		got.Role != claims.Role || got.DisplayName != claims.DisplayName {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, claims)
	}
	if got.Exp <= time.Now().Unix() {
		t.Error("token exp should be in the future")
	}
}

func TestParseTokenInvalid(t *testing.T) {
	for _, token := range []string{"", "abc", "a.b", "x.y.z", "e30.zz"} {
		if _, err := ParseToken(token); !errors.Is(err, ErrTokenInvalid) {
			t.Errorf("ParseToken(%q) should fail with ErrTokenInvalid, got %v", token, err)
		}
	}
}

func TestTokenTampered(t *testing.T) {
	claims := &Claims{Uid: 1, Username: "u1", Role: "deputy", DisplayName: "x"}
	token, err := GenerateToken(claims)
	if err != nil {
		t.Fatalf("GenerateToken error: %v", err)
	}
	parts := strings.SplitN(token, ".", 2)
	// 篡改载荷第一个字符（保持合法 base64url 字符）
	tampered := "A" + parts[0][1:] + "." + parts[1]
	if tampered == token {
		t.Fatal("sanity check failed: tampered token equals original")
	}
	if _, err := ParseToken(tampered); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("tampered token should be rejected, got %v", err)
	}
	// 篡改签名
	tamperedSig := parts[0] + "." + strings.TrimLeft(parts[1], "0")
	if tamperedSig == token {
		t.Skip("signature happens to start without 0, skip")
	}
	if _, err := ParseToken(tamperedSig); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("token with tampered signature should be rejected, got %v", err)
	}
}

func TestTokenExpired(t *testing.T) {
	claims := &Claims{Uid: 2, Username: "u2", Role: "chief", DisplayName: "主裁判", Exp: time.Now().Add(-time.Hour).Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	part := base64.RawURLEncoding.EncodeToString(payload)
	token := part + "." + signToken(part)
	if _, err := ParseToken(token); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("expired token should be rejected, got %v", err)
	}
}
