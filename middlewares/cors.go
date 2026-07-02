package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 处理浏览器跨域请求。
// 当前开发阶段允许所有来源跨域访问。
// 注意：Access-Control-Allow-Origin 使用 * 时，不能同时允许 credentials。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Max-Age", "86400")

		// 浏览器发送正式请求前，会先发 OPTIONS 预检。
		// 预检只需要确认跨域规则，不需要继续进入后续业务路由。
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
