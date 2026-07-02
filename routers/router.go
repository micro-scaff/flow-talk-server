package routers

import (
	"flow-talk/controllers"
	"flow-talk/middlewares"
	"flow-talk/models"

	"github.com/gin-gonic/gin"
)

// InitRouter 注册项目全部 HTTP 路由。
// 路由只负责把 URL 分配给 controller，鉴权等横切逻辑交给 middleware。
func InitRouter(engine *gin.Engine, cfg models.AppConfig) {
	// controller 是路由和业务之间的桥。
	// AuthController 需要 JWT 配置来签发 token，UserController 暂时没有额外依赖。
	authController := controllers.AuthController{JWT: cfg.JWT}
	userController := controllers.UserController{}
	conversationController := controllers.ConversationController{}

	// /api 放面向客户端的通用接口。
	api := engine.Group("/api")
	{
		// /api/auth 下放认证相关接口。
		// 注册和登录不需要 token，否则新用户无法进入系统。
		auth := api.Group("/auth")
		{
			auth.POST("/register", authController.Register)
			auth.POST("/login", authController.Login)
		}

		// /api/me 需要 token，用来检查当前登录态是否有效并返回当前用户信息。
		api.GET("/me", middlewares.AuthRequired(cfg.JWT), authController.Me)

		// /api/conversations 下放当前登录用户的会话相关接口。
		conversations := api.Group("/conversations", middlewares.AuthRequired(cfg.JWT))
		{
			conversations.GET("", conversationController.Index)
			conversations.GET("/:conversation_id", conversationController.Show)
			conversations.POST("/direct", conversationController.CreateDirect)
			conversations.POST("/groups", conversationController.CreateGroup)
		}
	}

	// /admin 放后台或调试接口。
	// 整个分组统一挂 AuthRequired，组内接口默认都需要登录。
	admin := engine.Group("/admin", middlewares.AuthRequired(cfg.JWT))
	{
		// 获取所有用户信息，当前用于验证 GORM 查询和用户表映射。
		admin.GET("/users", userController.Index)
	}
}
