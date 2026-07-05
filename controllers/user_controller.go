package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// UserController 处理用户列表相关接口。
type UserController struct{}

// Index 获取数据库中的 user 信息。
func (ctl UserController) Index(c *gin.Context) {
	// 该接口在 routers/router.go 中挂了 AuthRequired 中间件，只有携带有效 token 的请求能访问。
	// all=true 时返回全部用户；否则支持 limit/offset 分页读取。
	// 参数只影响查询范围，不参与权限放大；当前调用方仍只能看到 DTO 中允许暴露的字段。
	options := models.UserListOptions{
		Keyword:    c.Query("keyword"),
		AuthSource: c.Query("auth_source"),
		Limit:      parseNonNegativeInt(c.Query("limit")),
		Offset:     parseNonNegativeInt(c.Query("offset")),
		All:        parseBool(c.Query("all")),
	}
	if status, ok := parseOptionalInt(c.Query("status")); ok {
		options.Status = &status
	}

	users, err := models.ListUsers(options)
	if err != nil {
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
		return
	}
	responses.Success(c, users, "获取用户列表成功")
}

func parseNonNegativeInt(value string) int {
	// 查询参数来自 URL，非法数字和负数统一回落到 0，由 model 层继续套默认值。
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseOptionalInt(value string) (int, bool) {
	// status 是可选筛选项：空值表示不筛选；非法值不生效，避免把请求误判成服务端错误。
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseBool(value string) bool {
	// 兼容前端、curl 和部分网关常见的布尔写法。
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}
