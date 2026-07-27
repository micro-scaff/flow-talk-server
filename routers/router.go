package routers

import (
	"log"

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
	resourceController := controllers.ResourceController{}

	// Hub 是当前进程内的 WebSocket 连接管理器。
	// v4-v7 都围绕它实现单机实时投递和本机在线状态查询。
	wsHub := models.NewWSHub()
	realtimeBus := models.RealtimeBus(models.NewMemoryRealtimeBus())
	presenceProvider := models.PresenceProvider(models.NewHubPresenceProvider(wsHub))
	presenceTracker := models.PresenceTracker(models.NoopPresenceTracker{})
	if cfg.Redis.Enabled && models.RedisClient != nil {
		// 启用 Redis 后：
		// 1. 消息投递先发布到 Pub/Sub，再由各实例投递给自己的本机连接；
		// 2. 在线状态写入 Redis TTL key，批量查询能看到跨实例连接。
		realtimeBus = models.NewRedisRealtimeBus(models.RedisClient, cfg.Redis.Channel)
		redisPresence := models.NewRedisPresenceStore(models.RedisClient, cfg.Redis)
		presenceProvider = redisPresence
		presenceTracker = redisPresence
	}
	if err := realtimeBus.SubscribeMessageDeliver(func(event models.MessageDeliverEvent) {
		// 订阅端只负责把事件转成本机 Hub 投递。
		// 这样 HTTP 发送、WebSocket 发送和 Redis Pub/Sub 都复用同一个最终投递入口。
		controllers.DeliverMessageToLocalHub(wsHub, event)
	}); err != nil {
		log.Printf("订阅实时投递事件失败: %v", err)
	}
	if err := realtimeBus.SubscribePresenceChanged(func(event models.PresenceChangedEvent) {
		controllers.DeliverPresenceChangedToLocalHub(wsHub, event)
	}); err != nil {
		log.Printf("订阅在线状态事件失败: %v", err)
	}
	if err := realtimeBus.SubscribeConversationUnreadChanged(func(event models.ConversationUnreadChangedEvent) {
		controllers.DeliverConversationUnreadChangedToLocalHub(wsHub, event)
	}); err != nil {
		log.Printf("订阅会话未读事件失败: %v", err)
	}
	if err := realtimeBus.SubscribeConversationChanged(func(event models.ConversationChangedEvent) {
		controllers.DeliverConversationChangedToLocalHub(wsHub, event)
	}); err != nil {
		log.Printf("订阅会话变化事件失败: %v", err)
	}
	wsController := controllers.WSController{
		JWT:              cfg.JWT,
		Hub:              wsHub,
		Bus:              realtimeBus,
		PresenceTracker:  presenceTracker,
		PresenceProvider: presenceProvider,
	}
	messageController = controllers.MessageController{
		Hub: wsHub,
		Bus: realtimeBus,
	}
	conversationController = controllers.ConversationController{
		Hub: wsHub,
		Bus: realtimeBus,
	}
	groupController = controllers.GroupController{
		Hub: wsHub,
		Bus: realtimeBus,
	}
	presenceController := controllers.PresenceController{PresenceProvider: presenceProvider}

	// /api 放面向客户端的通用接口。
	api := engine.Group("/api")
	// 普通 API 请求体统一限制为 5 MiB；multipart 资源上传有自己的独立上限，不受这里影响。
	api.Use(middlewares.LimitAPIRequestBodyWithPathLimits(
		middlewares.DefaultMaxAPIRequestBodyBytes,
		map[string]int64{
			"/api/auth/register": middlewares.MaxRegisterRequestBodyBytes,
		},
	))
	{
		api.GET("/ws", wsController.Connect)

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
			conversations.POST("/detail", conversationController.Show)
			conversations.POST("/direct", conversationController.CreateDirect)
			conversations.POST("/groups", conversationController.CreateGroup)
			conversations.PATCH("/profile", groupController.UpdateProfile)
			conversations.POST("/members", groupController.AddMembers)
			conversations.DELETE("/members", groupController.RemoveMember)
			conversations.POST("/leave", groupController.Leave)
			conversations.PATCH("/members/role", groupController.UpdateMemberRole)
			conversations.POST("/messages", messageController.Create)
			conversations.POST("/messages/list", messageController.Index)
			conversations.POST("/messages/search", searchController.SearchConversation)
			conversations.POST("/read", messageController.MarkRead)
		}

		devices := api.Group("/devices", middlewares.AuthRequired(cfg.JWT))
		{
			devices.POST("", deviceController.Upsert)
			devices.GET("", deviceController.Index)
			devices.DELETE("", deviceController.Delete)
		}

		messages := api.Group("/messages", middlewares.AuthRequired(cfg.JWT))
		{
			messages.POST("/search", searchController.SearchMine)
			messages.POST("/receipts", receiptController.Index)
			messages.POST("/read", receiptController.Read)
			messages.POST("/unread", receiptController.Unread)
		}

		users := api.Group("/users", middlewares.AuthRequired(cfg.JWT))
		{
			// 正式用户列表接口，前端通讯录初始化通过 all=true 显式读取全量用户。
			users.GET("", userController.Index)
			// 查询单个指定用户的在线状态，请求体传入 user_id。
			users.POST("/presence", presenceController.Show)
			// 查询全部、在线、离线或指定用户，同时返回前端展示所需的用户资料和在线状态。
			users.POST("/presence/batch", presenceController.Batch)
		}

		resources := api.Group("/resources", middlewares.AuthRequired(cfg.JWT))
		{
			resources.POST("/upload", resourceController.Upload)
		}

		// /api/admin 放后台或调试接口。
		// 整个分组统一挂 AuthRequired，组内接口默认都需要登录。
		admin := api.Group("/admin", middlewares.AuthRequired(cfg.JWT))
		{
			// 兼容旧客户端；新客户端请使用 GET /api/users，避免普通业务依赖 admin 路径。
			admin.GET("/users", userController.Index)
		}
	}
}
