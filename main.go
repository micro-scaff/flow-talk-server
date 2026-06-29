package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"flow-talk/internal/auth"
	"flow-talk/internal/config"
	"flow-talk/internal/httpapi"
	"flow-talk/internal/storage"

	_ "github.com/go-sql-driver/mysql"
)

// main 组装第一阶段可运行服务：
// MySQL 用户存储、本地认证服务和 HTTP 路由。
//
// 默认读取 config.ini：
//   - mysql：数据库连接信息
//   - jwt：本地访问 token 的 HMAC 签名密钥和有效期
//   - http：监听地址
//
// 可选环境变量：
//   - MYSQL_DSN：覆盖配置文件中的 MySQL 连接字符串
//   - JWT_SECRET：覆盖配置文件中的 JWT 签名密钥
//   - HTTP_ADDR：监听地址，默认 :8080
//   - JWT_TTL：token 有效期，使用 Go duration 格式，默认 24h
func main() {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg, err = config.ApplyEnv(cfg, os.Getenv)
	if err != nil {
		log.Fatalf("apply env config: %v", err)
	}

	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer db.Close()

	// 启动时主动 Ping 数据库，避免账号错误或数据库不可达的问题延迟到首个请求才暴露。
	if err := db.Ping(); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}

	userStore := storage.NewMySQLUserStore(db)
	authService := auth.NewService(
		userStore,
		auth.NewTokenManager([]byte(cfg.JWTSecret), cfg.JWTTTL),
	)
	server := httpapi.NewServer(authService)

	log.Printf("flow-talk server listening on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, server.Routes()); err != nil {
		log.Fatalf("serve http: %v", err)
	}
}
