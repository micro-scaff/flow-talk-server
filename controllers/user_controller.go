package controllers

import (
	"net/http"

	"flow-talk/models"

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}
