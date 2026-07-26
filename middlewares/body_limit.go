package middlewares

import (
	"mime"
	"net/http"

	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// DefaultMaxAPIRequestBodyBytes 是普通 API 请求体上限。
// 文件上传使用 multipart，并由 ResourceController 设置独立的 100 MiB 上限。
const DefaultMaxAPIRequestBodyBytes int64 = 5 << 20

// MaxRegisterRequestBodyBytes 为 10 MiB 头像转换成 Base64 后预留约三分之一膨胀空间，
// 并留出 data URL 前缀、账号、昵称等 JSON 字段开销。
const MaxRegisterRequestBodyBytes int64 = 14 << 20

// LimitAPIRequestBody 限制 multipart 以外的 API 请求体大小，避免注册头像、设备信息或消息内容被异常放大后占满内存。
// 不能只检查 application/json：ShouldBindJSON 会强制解析 JSON，客户端可以伪造 Content-Type 绕过仅按类型判断的限制。
// Content-Length 可用时会在读取前返回 413；分块请求则由 MaxBytesReader 在读取过程中强制截断。
func LimitAPIRequestBody(maxBytes int64) gin.HandlerFunc {
	return LimitAPIRequestBodyWithPathLimits(maxBytes, nil)
}

// LimitAPIRequestBodyWithPathLimits 允许少数路由覆盖普通 API 的请求体上限。
// 注册接口需要承载 Base64 头像，其余 JSON 接口继续使用较小的默认限制。
func LimitAPIRequestBodyWithPathLimits(maxBytes int64, pathLimits map[string]int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxAPIRequestBodyBytes
	}
	return func(c *gin.Context) {
		if isMultipartContentType(c.GetHeader("Content-Type")) {
			c.Next()
			return
		}
		requestMaxBytes := maxBytes
		if pathMaxBytes := pathLimits[c.Request.URL.Path]; pathMaxBytes > 0 {
			requestMaxBytes = pathMaxBytes
		}
		if c.Request.ContentLength > requestMaxBytes {
			responses.Abort(c, http.StatusRequestEntityTooLarge, "请求体过大")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, requestMaxBytes)
		c.Next()
	}
}

func isMultipartContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	return mediaType == "multipart/form-data"
}
