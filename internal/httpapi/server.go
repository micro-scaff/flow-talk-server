package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"flow-talk/internal/auth"
)

// Server 是 HTTP 适配层。
// 它负责把 JSON 请求转换为 auth.Service 调用，让认证领域层不关心 HTTP 细节。
type Server struct {
	authService *auth.Service
}

// NewServer 基于认证服务创建 HTTP 服务。
func NewServer(authService *auth.Service) *Server {
	return &Server{authService: authService}
}

// Routes 返回当前阶段的 HTTP 路由。
// 第一阶段只做认证闭环，标准库 ServeMux 已经足够。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleMe)))
	return mux
}

// handleRegister 创建本地用户，并返回首个访问 token。
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	result, err := s.authService.Register(r.Context(), auth.RegisterRequest{
		Username:  req.Username,
		Password:  req.Password,
		Nickname:  req.Nickname,
		AvatarURL: req.AvatarURL,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUsernameTaken):
			writeError(w, http.StatusConflict, "username_taken")
		case errors.Is(err, auth.ErrValidation):
			writeError(w, http.StatusBadRequest, "validation_failed")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, newAuthResponse(result))
}

// handleLogin 校验本地账号密码，并返回新的访问 token。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	result, err := s.authService.Login(r.Context(), auth.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "invalid_credentials")
		case errors.Is(err, auth.ErrUserDisabled):
			writeError(w, http.StatusForbidden, "user_disabled")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}

	writeJSON(w, http.StatusOK, newAuthResponse(result))
}

// handleMe 返回当前登录用户信息。
// 这个接口也可以作为客户端检查 token 是否有效的轻量接口。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, userResponse{User: newUserDTO(user)})
}

// requireAuth 校验 Bearer token，并把当前用户写入请求上下文。
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		user, err := s.authService.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(withCurrentUser(r.Context(), user)))
	})
}

// bearerToken 从标准 Authorization 请求头中解析 token：
//
//	Authorization: Bearer <token>
func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

// readJSON 使用严格模式解析 JSON。
// 请求里出现未知字段时直接报错，避免客户端字段写错却被静默忽略。
func readJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

// writeJSON 写入统一的 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError 写入稳定、可被客户端识别的错误响应。
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, errorResponse{Error: code})
}

type registerRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	User  userDTO `json:"user"`
	Token string  `json:"token"`
}

type userResponse struct {
	User userDTO `json:"user"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// userDTO 是对外返回的用户结构。
// 这里刻意不包含 Password，避免任何接口意外泄露明文密码。
type userDTO struct {
	ID         int64  `json:"id"`
	ExternalID string `json:"external_id,omitempty"`
	Username   string `json:"username"`
	Nickname   string `json:"nickname"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	AuthSource string `json:"auth_source"`
	Status     int    `json:"status"`
}

// newAuthResponse 把领域层认证结果转换为 HTTP 响应结构。
func newAuthResponse(result auth.AuthResult) authResponse {
	return authResponse{
		User:  newUserDTO(result.User),
		Token: result.Token,
	}
}

// newUserDTO 是 auth.User 转换为公开用户对象的唯一入口。
func newUserDTO(user auth.User) userDTO {
	return userDTO{
		ID:         user.ID,
		ExternalID: user.ExternalID,
		Username:   user.Username,
		Nickname:   user.Nickname,
		AvatarURL:  user.AvatarURL,
		AuthSource: user.AuthSource,
		Status:     user.Status,
	}
}

// currentUserKey 是私有上下文 key 类型，避免和其他包写入的 context value 冲突。
type currentUserKey struct{}

// withCurrentUser 在一次 HTTP 请求生命周期内保存当前登录用户。
func withCurrentUser(ctx context.Context, user auth.User) context.Context {
	return context.WithValue(ctx, currentUserKey{}, user)
}

// currentUser 读取 requireAuth 写入上下文的当前用户。
func currentUser(ctx context.Context) (auth.User, bool) {
	user, ok := ctx.Value(currentUserKey{}).(auth.User)
	return user, ok
}
