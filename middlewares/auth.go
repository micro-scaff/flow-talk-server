package middlewares

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

const currentUserKey = "current_user"

var (
	// ErrInvalidToken 表示 token 结构、签名或声明不合法。
	ErrInvalidToken = errors.New("无效 token")
	// ErrTokenExpired 表示 token 已超过 exp 时间。
	ErrTokenExpired = errors.New("token 已过期")
)

// AuthRequired 是 Gin JWT 鉴权中间件。
// 它负责解析 Authorization: Bearer <token>，校验 token 后把当前用户写入 gin.Context。
func AuthRequired(cfg models.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从请求头中读取 Bearer token。
		// 约定格式：Authorization: Bearer <token>
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			responses.Abort(c, http.StatusUnauthorized, "未登录或登录已失效")
			return
		}

		// 2. 校验 token 签名和过期时间，并解析出用户 ID。
		claims, err := VerifyToken(token, cfg.Secret)
		if err != nil {
			responses.Abort(c, http.StatusUnauthorized, "未登录或登录已失效")
			return
		}

		// 3. token 校验通过后，再从数据库读取用户。
		// 这样即使用户已经拿到旧 token，只要后台禁用了该用户，下一次请求也会失效。
		user, err := models.FindUserByID(claims.UserID)
		if err != nil || user.Status != models.UserStatusEnabled {
			responses.Abort(c, http.StatusUnauthorized, "未登录或登录已失效")
			return
		}

		// 4. 把当前用户放入 Gin 上下文，后续 controller 可以通过 CurrentUser 读取。
		c.Set(currentUserKey, user)
		c.Next()
	}
}

// CurrentUser 从 Gin 上下文读取鉴权中间件保存的当前用户。
func CurrentUser(c *gin.Context) (models.User, bool) {
	// Gin Context 是一次请求内共享数据的地方。
	// 中间件写入 current_user，controller 再从这里取，避免重复解析 token。
	value, exists := c.Get(currentUserKey)
	if !exists {
		return models.User{}, false
	}
	user, ok := value.(models.User)
	return user, ok
}

// GenerateToken 生成当前服务使用的 HS256 JWT。
func GenerateToken(user models.User, cfg models.JWTConfig) (string, error) {
	now := time.Now()
	// payload 只放最小身份信息：用户 ID、用户名、签发时间和过期时间。
	// 完整用户资料仍然以数据库为准，避免 token 内信息过期。
	payload := jwtPayload{
		Subject:  strconv.FormatInt(user.ID, 10),
		Username: user.Username,
		IssuedAt: now.Unix(),
		Expires:  now.Add(cfg.TTL).Unix(),
	}
	// header 明确指定 HS256，VerifyToken 也只接受 HS256，避免算法混淆风险。
	header := jwtHeader{Algorithm: "HS256", Type: "JWT"}

	// JWT 的 header 和 payload 都是 JSON，再进行 base64url 编码。
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// unsigned 是签名前的内容：base64url(header).base64url(payload)。
	// 最终 token 是 unsigned.signature。
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	return unsigned + "." + sign(unsigned, cfg.Secret), nil
}

// VerifyToken 校验 token 的结构、签名、算法和过期时间。
func VerifyToken(token string, secret string) (jwtClaims, error) {
	// JWT 必须刚好由三段组成：header.payload.signature。
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return jwtClaims{}, ErrInvalidToken
	}

	// 先校验签名，确认 token 确实由当前服务密钥签发。
	unsigned := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(sign(unsigned, secret)), []byte(parts[2])) {
		return jwtClaims{}, ErrInvalidToken
	}

	// 解码并校验 header，只接受 HS256/JWT。
	var header jwtHeader
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headerJSON, &header) != nil {
		return jwtClaims{}, ErrInvalidToken
	}
	if header.Algorithm != "HS256" || header.Type != "JWT" {
		return jwtClaims{}, ErrInvalidToken
	}

	// 解码 payload，并校验过期时间。
	var payload jwtPayload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payloadJSON, &payload) != nil {
		return jwtClaims{}, ErrInvalidToken
	}
	if payload.Expires < time.Now().Unix() {
		return jwtClaims{}, ErrTokenExpired
	}

	// sub 是用户 ID。必须是正整数，否则不能作为有效身份。
	userID, err := strconv.ParseInt(payload.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return jwtClaims{}, ErrInvalidToken
	}
	return jwtClaims{UserID: userID, Username: payload.Username}, nil
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	// 严格要求 Bearer 前缀，避免误把其它认证头当成 JWT。
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func sign(unsigned string, secret string) string {
	// HMAC-SHA256 使用同一个 secret 进行签名和验签。
	// base64.RawURLEncoding 不带 padding，符合 JWT 常见编码方式。
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type jwtClaims struct {
	// UserID 是服务端真正信任的身份标识。
	UserID   int64
	Username string
}

type jwtHeader struct {
	// Algorithm 对应 JWT header 的 alg 字段。
	Algorithm string `json:"alg"`
	// Type 对应 JWT header 的 typ 字段，固定为 JWT。
	Type string `json:"typ"`
}

type jwtPayload struct {
	// Subject 对应 JWT sub 字段，这里存内部用户 ID。
	Subject  string `json:"sub"`
	Username string `json:"username"`
	// IssuedAt / Expires 分别对应 iat / exp，使用 Unix 秒级时间戳。
	IssuedAt int64 `json:"iat"`
	Expires  int64 `json:"exp"`
}
