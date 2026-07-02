package responses

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 是项目统一接口返回结构。
// code 表示状态码，data 放业务数据，message 放接口提示或错误信息。
type Body struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

// Success 返回成功响应。
// 当前项目约定业务成功统一返回 HTTP 200 和 code=200。
func Success(c *gin.Context, data any, message string) {
	c.JSON(http.StatusOK, Body{
		Code:    http.StatusOK,
		Data:    data,
		Message: message,
	})
}

// Error 返回失败响应。
// httpStatus 同时作为 HTTP 状态码和响应体里的 code。
func Error(c *gin.Context, httpStatus int, message string) {
	c.JSON(httpStatus, Body{
		Code:    httpStatus,
		Data:    nil,
		Message: message,
	})
}

// Abort 返回失败响应并终止后续 Gin handler。
// 中间件鉴权失败时使用这个方法，避免继续执行 controller。
func Abort(c *gin.Context, httpStatus int, message string) {
	c.AbortWithStatusJSON(httpStatus, Body{
		Code:    httpStatus,
		Data:    nil,
		Message: message,
	})
}
