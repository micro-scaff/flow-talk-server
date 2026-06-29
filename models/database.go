package models

import (
	"fmt"
	"net"
	"strings"
	"time"

	"gopkg.in/ini.v1"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	// DefaultConfigPath 是项目默认配置文件位置，和 demo-gin-mvc 的 conf/app.ini 结构保持一致。
	DefaultConfigPath = "conf/app.ini"
	// defaultHTTPAddr 是 http.addr 缺省时使用的监听地址。
	defaultHTTPAddr = ":8080"
	// defaultGinMode 是 http.mode 缺省时使用的 Gin 模式。
	defaultGinMode = "debug"
	// defaultJWTTTL 是 jwt.ttl 缺省时使用的 token 有效期。
	defaultJWTTTL = 24 * time.Hour
)

// DB 是全局 GORM 数据库连接。
// 当前项目规模较小，使用全局连接可以让 controller/model 写法更贴近传统 Gin MVC 示例。
var DB *gorm.DB

// AppConfig 是服务启动时会用到的完整配置。
// 配置文件只在 main 里读取一次，之后作为参数传给路由和中间件。
type AppConfig struct {
	// MySQL 保存数据库相关配置，最终会转换成 GORM 可用的 DSN。
	MySQL MySQLConfig
	// HTTP 保存 Gin 服务的监听地址和运行模式。
	HTTP HTTPConfig
	// JWT 保存 token 签名密钥和有效期。
	JWT JWTConfig
}

// MySQLConfig 保存数据库连接配置。
type MySQLConfig struct {
	// Host / Port 对应 MySQL 连接地址，例如 127.0.0.1:3306。
	Host string
	Port string
	// Username / Password 对应 MySQL 账号密码。
	Username string
	Password string
	// Database 是当前服务使用的数据库名。
	Database string
	// ParseTime 控制 MySQL 时间类型是否解析为 Go 的 time.Time。
	ParseTime bool
	// DSN 是最终传给 GORM 的完整连接串；如果 app.ini 里直接配置了 dsn，会优先使用它。
	DSN string
}

// HTTPConfig 保存 Gin 服务监听配置。
type HTTPConfig struct {
	// Addr 是 HTTP 监听地址，例如 :8080。
	Addr string
	// Mode 是 Gin 模式：debug / release / test。
	Mode string
}

// JWTConfig 保存本地登录 token 的签名密钥和有效期。
type JWTConfig struct {
	// Secret 是 HS256 签名密钥，必须保持稳定，否则已签发 token 会失效。
	Secret string
	// TTL 是 token 有效期，例如 24h。
	TTL time.Duration
}

// LoadConfig 使用 go-ini 从 conf/app.ini 读取配置。
// 如果配置了 mysql.dsn，会优先使用完整 DSN；否则用 host/port/username/password/database 组装。
func LoadConfig(path string) (AppConfig, error) {
	// 允许传空路径，方便调用方不关心配置文件具体位置。
	if path == "" {
		path = DefaultConfigPath
	}

	// ini.Load 会读取整个配置文件，并按 section/key 的方式取值。
	file, err := ini.Load(path)
	if err != nil {
		return AppConfig{}, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 每个 section 对应 app.ini 里的一个 [xxx] 块。
	// 这里先取出来，后面组装结构体时更清楚。
	mysqlSection := file.Section("mysql")
	httpSection := file.Section("http")
	jwtSection := file.Section("jwt")

	// MustString / MustBool 用来提供默认值。
	// strings.TrimSpace 可以避免配置里多写空格导致连接失败。
	cfg := AppConfig{
		MySQL: MySQLConfig{
			Host:      strings.TrimSpace(mysqlSection.Key("host").MustString("127.0.0.1")),
			Port:      strings.TrimSpace(mysqlSection.Key("port").MustString("3306")),
			Username:  strings.TrimSpace(mysqlSection.Key("username").String()),
			Password:  strings.TrimSpace(mysqlSection.Key("password").String()),
			Database:  strings.TrimSpace(mysqlSection.Key("database").String()),
			ParseTime: mysqlSection.Key("parse_time").MustBool(true),
			DSN:       strings.TrimSpace(mysqlSection.Key("dsn").String()),
		},
		HTTP: HTTPConfig{
			Addr: strings.TrimSpace(httpSection.Key("addr").MustString(defaultHTTPAddr)),
			Mode: strings.TrimSpace(httpSection.Key("mode").MustString(defaultGinMode)),
		},
		JWT: JWTConfig{
			Secret: strings.TrimSpace(jwtSection.Key("secret").String()),
		},
	}

	// 支持两种 MySQL 配置方式：
	// 1. 直接写 mysql.dsn，适合复杂连接参数；
	// 2. 拆分写 host/port/username/password/database，更适合本地开发阅读。
	if cfg.MySQL.DSN == "" {
		cfg.MySQL.DSN, err = cfg.MySQL.BuildDSN()
		if err != nil {
			return AppConfig{}, err
		}
	}
	// JWT 密钥没有安全默认值，必须显式配置。
	if cfg.JWT.Secret == "" {
		return AppConfig{}, fmt.Errorf("jwt.secret 不能为空")
	}

	// time.ParseDuration 支持 30m、2h、24h 这类 Go duration 字符串。
	cfg.JWT.TTL, err = time.ParseDuration(strings.TrimSpace(jwtSection.Key("ttl").MustString(defaultJWTTTL.String())))
	if err != nil || cfg.JWT.TTL <= 0 {
		return AppConfig{}, fmt.Errorf("jwt.ttl 必须是有效的正数时间，例如 24h")
	}
	// 再兜底一次，避免配置项写成空字符串。
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = defaultHTTPAddr
	}
	if cfg.HTTP.Mode == "" {
		cfg.HTTP.Mode = defaultGinMode
	}

	return cfg, nil
}

// BuildDSN 把拆分的 MySQL 配置组装成 GORM/MySQL 驱动可识别的 DSN。
func (cfg MySQLConfig) BuildDSN() (string, error) {
	// username 和 database 是连接数据库必须的信息。
	// password 可以为空，所以不在这里强制校验。
	if cfg.Username == "" {
		return "", fmt.Errorf("mysql.username 不能为空")
	}
	if cfg.Database == "" {
		return "", fmt.Errorf("mysql.database 不能为空")
	}

	// net.JoinHostPort 会正确处理 IPv4、IPv6 和端口拼接，比手写 host + ":" + port 更稳。
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	// charset=utf8mb4 支持 emoji 等完整 Unicode 字符。
	// loc=Local 让时间按本地时区解析，配合 parseTime 使用。
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=%t&loc=Local", cfg.Username, cfg.Password, addr, cfg.Database, cfg.ParseTime), nil
}

// InitDB 创建并保存 GORM 数据库连接。
// main 启动时调用一次；后续 models 包中的方法都复用这个连接池。
func InitDB(cfg MySQLConfig) error {
	// gorm.Open 不一定会立刻完成真实网络连接，所以后面还会调用 Ping 做启动期检查。
	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("连接 MySQL 失败: %w", err)
	}

	// db.DB() 可以拿到底层 database/sql 连接池。
	// GORM 的高级查询走 *gorm.DB，连接池关闭和 Ping 仍然通过 *sql.DB 完成。
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}
	// 主动 Ping 可以在启动阶段就发现账号、密码、数据库名或网络问题。
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("Ping MySQL 失败: %w", err)
	}

	// 保存到包级变量，models/user_model.go 里的查询方法会复用这个连接。
	DB = db
	return nil
}

// CloseDB 关闭 GORM 底层连接池。
func CloseDB() error {
	// 如果数据库还没初始化就调用 CloseDB，直接返回 nil，方便 defer 安全调用。
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
