package controllers

import (
	"errors"
	"net/http"

	"flow-talk/middlewares"
	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// ExternalAuthController 处理外部登录态换取 IM token。
// 它不直接信任客户端传来的外部用户信息，而是把 access_token 交给 model 层 verifier 校验。
type ExternalAuthController struct {
	JWT models.JWTConfig
}

// ExternalLoginRequest 是外部登录请求。
// provider 决定使用哪一个外部身份校验器；当前落地的是 demo provider。
type ExternalLoginRequest struct {
	Provider    string `json:"provider" binding:"required"`
	AccessToken string `json:"access_token" binding:"required"`
}

// Login 校验外部登录态，并签发本服务自己的 JWT。
// 这样后续 IM 接口仍复用统一 AuthRequired 中间件，不需要每个接口都理解外部系统。
func (ctl ExternalAuthController) Login(c *gin.Context) {
	var req ExternalLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	user, err := models.LoginExternalUser(req.Provider, req.AccessToken)
	if err != nil {
		writeExternalAuthError(c, err)
		return
	}

	// 外部账号同步成内部 users 记录后，仍签发本服务 JWT。
	// token 中只保存内部 user_id，后续权限判断统一以内库数据为准。
	token, err := middlewares.GenerateToken(user, ctl.JWT)
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
		return
	}
	responses.Success(c, gin.H{"user": user.ToDTO(), "token": token}, "登录成功")
}

func writeExternalAuthError(c *gin.Context, err error) {
	// 不支持的 provider、空 token 等都归为参数错误。
	// 外部系统真实错误后续接入时可以扩展成 502/503，但当前 demo provider 不需要。
	switch {
	case errors.Is(err, models.ErrValidation),
		errors.Is(err, models.ErrUnsupportedAuthProvider):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	case errors.Is(err, models.ErrUserDisabled):
		responses.Error(c, http.StatusForbidden, "用户已禁用")
	case errors.Is(err, models.ErrUsernameTaken):
		responses.Error(c, http.StatusConflict, "用户名已存在")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
