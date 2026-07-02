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
type ExternalAuthController struct {
	JWT models.JWTConfig
}

type ExternalLoginRequest struct {
	Provider    string `json:"provider" binding:"required"`
	AccessToken string `json:"access_token" binding:"required"`
}

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

	token, err := middlewares.GenerateToken(user, ctl.JWT)
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
		return
	}
	responses.Success(c, gin.H{"user": user.ToDTO(), "token": token}, "登录成功")
}

func writeExternalAuthError(c *gin.Context, err error) {
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
