package controllers

import (
	"errors"
	"net/http"

	"flow-talk/middlewares"
	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// AuthController 处理注册、登录、当前用户信息等认证相关接口。
type AuthController struct {
	// JWT 是从 conf/app.ini 读取出来的 token 配置。
	// controller 签发 token 时需要使用相同的 secret 和 ttl。
	JWT models.JWTConfig
}

// RegisterRequest 是注册接口的 JSON 入参。
type RegisterRequest struct {
	// Username 和 Password 带 binding:"required"，Gin 会在 ShouldBindJSON 时做必填校验。
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	// Nickname 不传时 model 层会默认使用 username。
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// LoginRequest 是登录接口的 JSON 入参。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Register 创建本地用户，并在注册成功后直接签发 token。
func (ctl AuthController) Register(c *gin.Context) {
	var req RegisterRequest
	// ShouldBindJSON 会把请求体 JSON 反序列化到 req，并执行 binding 标签校验。
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	// controller 不直接写 SQL，把用户创建交给 models 层，保持 MVC 分工清楚。
	user, err := models.CreateLocalUser(req.Username, req.Password, req.Nickname, req.AvatarURL)
	if err != nil {
		writeModelError(c, err)
		return
	}

	// 注册成功后直接签发 token，前端可以立即进入登录态。
	token, err := middlewares.GenerateToken(user, ctl.JWT)
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
		return
	}
	responses.Success(c, gin.H{"user": user.ToDTO(), "token": token}, "注册成功")
}

// Login 校验本地账号密码，并签发新的 token。
func (ctl AuthController) Login(c *gin.Context) {
	var req LoginRequest
	// 登录接口只接受 JSON，例如 {"username":"alice","password":"123456"}。
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	// model 层会统一处理用户不存在、密码错误、用户禁用等情况。
	user, err := models.LoginUser(req.Username, req.Password)
	if err != nil {
		writeModelError(c, err)
		return
	}

	// 每次登录成功都签发一个新的 token。
	token, err := middlewares.GenerateToken(user, ctl.JWT)
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
		return
	}
	responses.Success(c, gin.H{"user": user.ToDTO(), "token": token}, "登录成功")
}

// Me 返回当前 token 对应的用户信息。
func (ctl AuthController) Me(c *gin.Context) {
	// /api/me 已经在路由层挂了 AuthRequired 中间件。
	// 这里直接读取中间件写入的当前用户即可。
	user, ok := middlewares.CurrentUser(c)
	if !ok {
		responses.Error(c, http.StatusUnauthorized, "未登录或登录已失效")
		return
	}
	responses.Success(c, user.ToDTO(), "获取当前用户成功")
}

// writeModelError 把 model 层错误转换成稳定的 HTTP 状态码和错误码。
func writeModelError(c *gin.Context, err error) {
	// model 层只返回领域错误，controller 负责把它们翻译成 HTTP 状态码和前端可识别的 error code。
	switch {
	case errors.Is(err, models.ErrUsernameTaken):
		responses.Error(c, http.StatusConflict, "用户名已存在")
	case errors.Is(err, models.ErrValidation):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	case errors.Is(err, models.ErrInvalidCredentials):
		responses.Error(c, http.StatusUnauthorized, "账号或密码错误")
	case errors.Is(err, models.ErrUserDisabled):
		responses.Error(c, http.StatusForbidden, "用户已禁用")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
