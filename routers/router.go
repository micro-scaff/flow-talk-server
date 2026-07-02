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
	externalAuthController := controllers.ExternalAuthController{JWT: cfg.JWT}
	userController := controllers.UserController{}
	conversationController := controllers.ConversationController{}
	messageController := controllers.MessageController{}
	groupController := controllers.GroupController{}
	deviceController := controllers.DeviceController{}
	receiptController := controllers.MessageReceiptController{}
	searchController := controllers.MessageSearchController{}

	// Hub 是当前进程内的 WebSocket 连接管理器。
	// v4-v7 都围绕它实现单机实时投递和本机在线状态查询。
	wsHub := models.NewWSHub()
	wsController := controllers.WSController{JWT: cfg.JWT, Hub: wsHub}
	presenceController := controllers.PresenceController{Hub: wsHub}

	engine.GET("/ws", wsController.Connect)

	// /api 放面向客户端的通用接口。
	api := engine.Group("/api")
	{
		// /api/auth 下放认证相关接口。
		// 注册和登录不需要 token，否则新用户无法进入系统。
		auth := api.Group("/auth")
		{
			auth.POST("/register", authController.Register)
			auth.POST("/login", authController.Login)
			auth.POST("/external", externalAuthController.Login)
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
			conversations.PATCH("/:conversation_id", groupController.UpdateProfile)
			conversations.POST("/:conversation_id/members", groupController.AddMembers)
			conversations.DELETE("/:conversation_id/members/:user_id", groupController.RemoveMember)
			conversations.POST("/:conversation_id/leave", groupController.Leave)
			conversations.PATCH("/:conversation_id/members/:user_id/role", groupController.UpdateMemberRole)
			conversations.POST("/:conversation_id/messages", messageController.Create)
			conversations.GET("/:conversation_id/messages", messageController.Index)
			conversations.GET("/:conversation_id/messages/search", searchController.SearchConversation)
			conversations.POST("/:conversation_id/read", messageController.MarkRead)
		}

		devices := api.Group("/devices", middlewares.AuthRequired(cfg.JWT))
		{
			devices.POST("", deviceController.Upsert)
			devices.GET("", deviceController.Index)
			devices.DELETE("/:device_id", deviceController.Delete)
		}

		messages := api.Group("/messages", middlewares.AuthRequired(cfg.JWT))
		{
			messages.GET("/search", searchController.SearchMine)
			messages.PATCH("/:message_id/recall", messageController.Recall)
			messages.PATCH("/:message_id/delete", messageController.Delete)
			messages.GET("/:message_id/receipts", receiptController.Index)
			messages.POST("/:message_id/delivered", receiptController.Delivered)
			messages.POST("/:message_id/read", receiptController.Read)
		}

		users := api.Group("/users", middlewares.AuthRequired(cfg.JWT))
		{
			users.GET("/:user_id/presence", presenceController.Show)
			users.POST("/presence/batch", presenceController.Batch)
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
