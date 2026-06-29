package config

import (
	"fmt"
	"net"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

const (
	defaultConfigPath = "config.ini"
	defaultHTTPAddr   = ":8080"
	defaultJWTTTL     = 24 * time.Hour
)

// Config 保存服务启动所需的配置。
type Config struct {
	MySQLDSN  string
	HTTPAddr  string
	JWTSecret string
	JWTTTL    time.Duration
}

// DefaultPath 返回默认配置文件路径。
func DefaultPath() string {
	return defaultConfigPath
}

// Load 从 ini 配置文件加载服务配置。
func Load(path string) (Config, error) {
	if path == "" {
		path = defaultConfigPath
	}

	file, err := ini.Load(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config file %q: %w", path, err)
	}

	cfg := Config{
		MySQLDSN:  strings.TrimSpace(file.Section("mysql").Key("dsn").String()),
		HTTPAddr:  strings.TrimSpace(file.Section("http").Key("addr").MustString(defaultHTTPAddr)),
		JWTSecret: strings.TrimSpace(file.Section("jwt").Key("secret").String()),
	}

	if cfg.MySQLDSN == "" {
		cfg.MySQLDSN, err = mysqlDSN(file.Section("mysql"))
		if err != nil {
			return Config{}, err
		}
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("jwt.secret is required")
	}

	ttl, err := time.ParseDuration(strings.TrimSpace(file.Section("jwt").Key("ttl").MustString(defaultJWTTTL.String())))
	if err != nil || ttl <= 0 {
		return Config{}, fmt.Errorf("jwt.ttl must be a positive duration")
	}
	cfg.JWTTTL = ttl

	return cfg, nil
}

// ApplyEnv 用环境变量覆盖 ini 配置，便于部署时注入敏感信息或临时参数。
func ApplyEnv(cfg Config, lookup func(string) string) (Config, error) {
	if value := strings.TrimSpace(lookup("MYSQL_DSN")); value != "" {
		cfg.MySQLDSN = value
	}
	if value := strings.TrimSpace(lookup("HTTP_ADDR")); value != "" {
		cfg.HTTPAddr = value
	}
	if value := strings.TrimSpace(lookup("JWT_SECRET")); value != "" {
		cfg.JWTSecret = value
	}
	if value := strings.TrimSpace(lookup("JWT_TTL")); value != "" {
		ttl, err := time.ParseDuration(value)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("JWT_TTL must be a positive duration")
		}
		cfg.JWTTTL = ttl
	}
	return cfg, nil
}

func mysqlDSN(section *ini.Section) (string, error) {
	user := strings.TrimSpace(section.Key("username").String())
	password := strings.TrimSpace(section.Key("password").String())
	host := strings.TrimSpace(section.Key("host").MustString("127.0.0.1"))
	port := strings.TrimSpace(section.Key("port").MustString("3306"))
	database := strings.TrimSpace(section.Key("database").String())
	parseTime := section.Key("parse_time").MustBool(true)

	if user == "" {
		return "", fmt.Errorf("mysql.username is required")
	}
	if database == "" {
		return "", fmt.Errorf("mysql.database is required")
	}

	return fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=%t", user, password, net.JoinHostPort(host, port), database, parseTime), nil
}
