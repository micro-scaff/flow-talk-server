package auth

import (
	"context"
	"errors"
	"strings"
)

const (
	// AuthSourceLocal 表示用户来自当前服务内置的注册登录流程。
	AuthSourceLocal = "local"
	// AuthSourceExternal 预留给后续从外部登录系统同步过来的用户。
	AuthSourceExternal = "external"

	UserStatusDisabled = 0
	UserStatusEnabled  = 1
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user disabled")
	ErrUserNotFound       = errors.New("user not found")
	ErrUsernameTaken      = errors.New("username taken")
	ErrValidation         = errors.New("validation failed")
)

// User 是认证模块的用户领域模型，对应 users 表。
// Password 只在服务内部用于登录校验，对外响应必须通过 DTO 脱敏。
type User struct {
	ID         int64
	ExternalID string
	Username   string
	Password   string
	Nickname   string
	AvatarURL  string
	AuthSource string
	Status     int
}

// CreateUserParams 是创建用户时传给存储层的写入模型。
// 空字符串是否映射为数据库 NULL，由具体存储实现决定。
type CreateUserParams struct {
	ExternalID string
	Username   string
	Password   string
	Nickname   string
	AvatarURL  string
	AuthSource string
}

// UserStore 是认证模块依赖的用户存储边界。
// 认证逻辑只依赖这个接口，后续可以替换为外部身份同步实现。
type UserStore interface {
	CreateUser(ctx context.Context, params CreateUserParams) (User, error)
	FindUserByUsername(ctx context.Context, username string) (User, error)
	FindUserByID(ctx context.Context, id int64) (User, error)
}

// RegisterRequest 是本地注册接口需要的参数。
type RegisterRequest struct {
	Username  string
	Password  string
	Nickname  string
	AvatarURL string
}

// LoginRequest 是本地登录接口需要的账号密码。
type LoginRequest struct {
	Username string
	Password string
}

// AuthResult 是注册或登录成功后的返回结果。
// Token 会被客户端用于后续 HTTP 或 WebSocket 请求。
type AuthResult struct {
	User  User
	Token string
}

// Service 负责协调明文密码校验、token 签发和用户存储。
// HTTP 层应该调用这个服务，而不是直接操作数据库。
type Service struct {
	store        UserStore
	tokenManager TokenManager
}

// NewService 创建认证服务。
func NewService(store UserStore, tokenManager TokenManager) *Service {
	return &Service{
		store:        store,
		tokenManager: tokenManager,
	}
}

// Register 创建本地用户，并立即签发 token。
// 当前阶段保留这个能力，确保没有外部登录系统时服务也能独立使用。
func (s *Service) Register(ctx context.Context, req RegisterRequest) (AuthResult, error) {
	username := strings.TrimSpace(req.Username)
	password := req.Password
	nickname := strings.TrimSpace(req.Nickname)
	if username == "" || password == "" {
		return AuthResult{}, ErrValidation
	}
	if nickname == "" {
		nickname = username
	}

	user, err := s.store.CreateUser(ctx, CreateUserParams{
		Username:   username,
		Password:   password,
		Nickname:   nickname,
		AvatarURL:  strings.TrimSpace(req.AvatarURL),
		AuthSource: AuthSourceLocal,
	})
	if err != nil {
		return AuthResult{}, err
	}
	return s.resultForUser(user)
}

// Login 校验本地账号密码，并签发新的 token。
// 用户不存在和密码错误都会返回 ErrInvalidCredentials，避免暴露账号是否存在。
func (s *Service) Login(ctx context.Context, req LoginRequest) (AuthResult, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		return AuthResult{}, ErrInvalidCredentials
	}

	user, err := s.store.FindUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}
	if user.Status != UserStatusEnabled {
		return AuthResult{}, ErrUserDisabled
	}
	if req.Password != user.Password {
		return AuthResult{}, ErrInvalidCredentials
	}
	return s.resultForUser(user)
}

// Authenticate 校验 token，并从存储层重新加载用户。
// 重新加载用户可以保证用户被禁用后，旧 token 不会继续生效。
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	claims, err := s.tokenManager.Verify(token)
	if err != nil {
		return User{}, err
	}
	user, err := s.store.FindUserByID(ctx, claims.UserID)
	if err != nil {
		return User{}, err
	}
	if user.Status != UserStatusEnabled {
		return User{}, ErrUserDisabled
	}
	return user, nil
}

// resultForUser 统一注册和登录成功后的 token 生成逻辑。
func (s *Service) resultForUser(user User) (AuthResult, error) {
	token, err := s.tokenManager.Generate(UserClaims{UserID: user.ID, Username: user.Username})
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: user, Token: token}, nil
}
