package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

// 默认 token 签名密钥（开发用），可通过环境变量 TOKEN_SECRET 覆盖。
// 多实例部署时必须保证所有实例 TOKEN_SECRET 一致，否则 token 跨实例校验失败。
const defaultTokenSecret = "league-timer-dev-secret"

// tokenTTL token 有效期：7 天
const tokenTTL = 7 * 24 * time.Hour

// ErrTokenInvalid token 无效或已过期
var ErrTokenInvalid = errors.New("token 无效或已过期")

// Claims 无状态 token 载荷
type Claims struct {
	Uid         int64  `json:"uid"`         // chief=0，deputy=数据库 id
	Username    string `json:"username"`    // 用户名
	Role        string `json:"role"`        // "chief" | "deputy"
	DisplayName string `json:"displayName"` // 展示名
	Exp         int64  `json:"exp"`         // 过期时间（unix 秒）
}

// tokenSecret 读取签名密钥：优先环境变量 TOKEN_SECRET
func tokenSecret() []byte {
	if s := os.Getenv("TOKEN_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte(defaultTokenSecret)
}

// signToken 对 base64url 部分做 HMAC-SHA256 签名，输出小写 hex
func signToken(part string) string {
	mac := hmac.New(sha256.New, tokenSecret())
	mac.Write([]byte(part))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateToken 生成无状态 token：
// base64url(payloadJSON) + "." + hexLower(hmacSHA256(secret, base64url部分))
func GenerateToken(claims *Claims) (string, error) {
	claims.Exp = time.Now().Add(tokenTTL).Unix()
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	part := base64.RawURLEncoding.EncodeToString(payload)
	return part + "." + signToken(part), nil
}

// ParseToken 校验并解析 token，签名或 exp 不合法一律返回 ErrTokenInvalid
func ParseToken(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrTokenInvalid
	}
	// 校验签名为合法 hex 后，与小写 hex 签名做 hmac.Equal 常数时间比较
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return nil, ErrTokenInvalid
	}
	if !hmac.Equal([]byte(parts[1]), []byte(signToken(parts[0]))) {
		return nil, ErrTokenInvalid
	}
	// 兼容带 padding 的 base64url
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[0], "="))
	if err != nil {
		return nil, ErrTokenInvalid
	}
	claims := &Claims{}
	if err := json.Unmarshal(payload, claims); err != nil {
		return nil, ErrTokenInvalid
	}
	if claims.Exp <= 0 || claims.Exp < time.Now().Unix() {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// authMiddleware 鉴权中间件：解析 Authorization: Bearer <token>。
// requiredRole 为空串表示任意已登录用户可访问（对应契约中的 role=null），
// 指定角色时角色不匹配返回 40300，token 缺失/无效/过期返回 40100。
func authMiddleware(requiredRole string, next func(http.ResponseWriter, *http.Request, *Claims)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := parseAuthorization(r)
		if err != nil {
			sendError(w, CodeUnauthorized, "未登录或登录状态已失效")
			return
		}
		if requiredRole != "" && claims.Role != requiredRole {
			sendError(w, CodeForbidden, "无权限执行该操作")
			return
		}
		next(w, r, claims)
	}
}

// parseAuthorization 解析 Authorization 请求头
func parseAuthorization(r *http.Request) (*Claims, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return nil, ErrTokenInvalid
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, ErrTokenInvalid
	}
	return ParseToken(strings.TrimSpace(parts[1]))
}
