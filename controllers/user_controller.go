package controllers

import (
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// UserController 处理后台用户相关接口。
type UserController struct{}

// Index 获取数据库中的所有 user 信息。
func (ctl UserController) Index(c *gin.Context) {
	// 该接口在 routers/router.go 中挂了 AuthRequired 中间件，只有携带有效 token 的请求能访问。
	// 当前作为管理/调试接口，直接返回用户列表；后续可以加分页、搜索和角色权限。
	users, err := models.ListUsers()
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
		return
	}
	responses.Success(c, users, "获取用户列表成功")
}
