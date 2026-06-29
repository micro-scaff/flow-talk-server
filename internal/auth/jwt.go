package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

// UserClaims 是 token 中携带的最小身份信息。
// 认证层只保留用户 ID 和用户名，完整用户资料在 token 校验后从存储层读取。
type UserClaims struct {
	UserID   int64
	Username string
}

// TokenManager 负责生成和校验当前阶段使用的 JWT。
// 目前只支持 HS256，避免误接受其他签名算法生成的 token。
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenManager 创建 token 管理器。
// secret 应来自配置，并在服务重启后保持稳定，否则旧 token 会全部失效。
func NewTokenManager(secret []byte, ttl time.Duration) TokenManager {
	return TokenManager{
		secret: secret,
		ttl:    ttl,
		now:    time.Now,
	}
}

// Generate 签发 JWT，内容包含内部用户 ID、用户名、签发时间和过期时间。
func (m TokenManager) Generate(claims UserClaims) (string, error) {
	if len(m.secret) == 0 {
		return "", errors.New("token secret is required")
	}

	now := m.now()
	payload := jwtPayload{
		Subject:  strconv.FormatInt(claims.UserID, 10),
		Username: claims.Username,
		IssuedAt: now.Unix(),
		Expires:  now.Add(m.ttl).Unix(),
	}
	header := jwtHeader{Algorithm: "HS256", Type: "JWT"}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal jwt payload: %w", err)
	}

	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	return unsigned + "." + m.sign(unsigned), nil
}

// Verify 校验 token 的结构、签名、算法、过期时间和用户 ID。
// 只有签名确认来自当前服务配置的 secret 后，才会返回 claims。
func (m TokenManager) Verify(token string) (UserClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return UserClaims{}, ErrInvalidToken
	}

	unsigned := parts[0] + "." + parts[1]
	expectedSignature := m.sign(unsigned)
	// hmac.Equal 使用常量时间比较，适合校验消息认证码。
	if !hmac.Equal([]byte(expectedSignature), []byte(parts[2])) {
		return UserClaims{}, ErrInvalidToken
	}

	var header jwtHeader
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return UserClaims{}, ErrInvalidToken
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return UserClaims{}, ErrInvalidToken
	}
	if header.Algorithm != "HS256" || header.Type != "JWT" {
		return UserClaims{}, ErrInvalidToken
	}

	var payload jwtPayload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return UserClaims{}, ErrInvalidToken
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return UserClaims{}, ErrInvalidToken
	}
	if payload.Expires < m.now().Unix() {
		return UserClaims{}, ErrTokenExpired
	}

	userID, err := strconv.ParseInt(payload.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return UserClaims{}, ErrInvalidToken
	}
	return UserClaims{UserID: userID, Username: payload.Username}, nil
}

// sign 对 JWT 的 header 和 payload 计算 HMAC，并返回 base64url 编码结果。
func (m TokenManager) sign(unsigned string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// jwtHeader 是私有结构，避免外部调用方影响服务接受的签名算法。
type jwtHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

// jwtPayload 保存服务当前需要的 JWT 字段。
// 用户 ID 是身份判断的准绳，用户名只是便于调试和展示的附加信息。
type jwtPayload struct {
	Subject  string `json:"sub"`
	Username string `json:"username"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}
