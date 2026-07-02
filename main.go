package main

import (
	"encoding/json"
	"log"

	"flow-talk/middlewares"
	"flow-talk/models"
	"flow-talk/routers"

	"github.com/gin-gonic/gin"
)

// main 是服务入口。
// 启动顺序保持清晰：读取配置 -> 初始化数据库 -> 创建 Gin 引擎 -> 注册路由 -> 启动 HTTP 服务。
func main() {
	// 1. 读取 conf/app.ini。
	// 配置读取集中放在 models.LoadConfig，main 只关心最终的结构化配置。
	cfg, err := models.LoadConfig(models.DefaultConfigPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	cfgJSON, err := json.MarshalIndent(cfg.LogFields(), "", "  ")
	if err != nil {
		log.Fatalf("格式化配置日志失败: %v", err)
	}
	log.Printf("当前配置:\n%s", cfgJSON)

	// 2. 设置 Gin 运行模式。
	// debug 模式会打印详细路由和请求日志，release 模式更适合线上环境。
	gin.SetMode(cfg.HTTP.Mode)

	// 3. 初始化数据库连接。
	// InitDB 内部会创建 GORM 连接并 Ping 数据库，保证服务启动时就能暴露数据库配置问题。
	if err := models.InitDB(cfg.MySQL); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	// 4. main 退出时关闭数据库连接池。
	// 正常运行时这里一般只会在进程停止时触发。
	defer func() {
		if err := models.CloseDB(); err != nil {
			log.Printf("关闭数据库连接失败: %v", err)
		}
	}()

	// 5. 创建 Gin 引擎。
	// gin.Default() 默认带 Logger 和 Recovery 中间件：前者打印请求日志，后者兜底 panic。
	engine := gin.Default()
	// 当前服务没有放在反向代理之后，关闭 trusted proxies 可以消除 Gin 的安全警告。
	if err := engine.SetTrustedProxies(nil); err != nil {
		log.Fatalf("设置 Gin trusted proxies 失败: %v", err)
	}
	// 必须在业务路由之前注册 CORS 中间件，才能处理浏览器发出的 OPTIONS 预检请求。
	engine.Use(middlewares.CORS())

	// 暴露 static 目录，后续上传文件、图片、前端静态资源都可以先放这里。
	engine.Static("/static", "./static")

	// 6. 注册业务路由。
	// 路由文件只负责把 URL 绑定到 controller，具体业务逻辑放在 controllers/models。
	routers.InitRouter(engine, cfg)

	// 7. 启动 HTTP 服务。
	// engine.Run 会阻塞当前 goroutine，直到服务启动失败或进程退出。
	log.Printf("flow-talk server listening on %s", cfg.HTTP.Addr)
	if err := engine.Run(cfg.HTTP.Addr); err != nil {
		log.Fatalf("启动 HTTP 服务失败: %v", err)
	}
}
