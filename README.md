# flow-talk-server

流言后端服务，基于 Gin + GORM + MySQL 实现即时通讯能力。

项目的业务目标、设计选择、数据模型和版本规划已经迁移到 [docs/OVERVIEW.md](./docs/OVERVIEW.md)。README 只保留本地开发最常用的信息。

## 目录结构

```text
flow-talk-server
├─ conf
│  └─ app.ini                  本地开发配置，包含 MySQL、HTTP、JWT
├─ controllers                 HTTP/WebSocket 接口控制层
├─ middlewares                 Gin 中间件，例如鉴权、CORS
├─ models                      配置、数据库模型、业务模型、WebSocket Hub
├─ responses                   统一响应结构
├─ routers
│  └─ router.go                路由注册
├─ static                      上传资源和静态资源目录
├─ docs                        项目说明、接口文档、数据库文档
│  ├─ OVERVIEW.md              项目介绍和整体设计
│  ├─ openapi.json             OpenAPI 接口描述
│  └─ 数据库                   建表和数据库落地文档
├─ runner.conf                 fresh 热重载配置
├─ Makefile                    本地开发常用命令
├─ go.mod
└─ main.go                     服务入口
```

## 环境要求

- Go：1.25+
- MySQL：本地默认连接 `127.0.0.1:3306`
- fresh：可选，仅热重载开发时需要

本项目通过 `Makefile` 将 Go 缓存、module 缓存和 fresh 安装目录放在项目内，不修改全局 Go 配置。

默认项目级代理为 `https://goproxy.cn,direct`。如果需要临时换代理，可以只对当前命令生效：

```bash
make run GO_PROXY=https://proxy.golang.org,direct
```

## 本地配置

服务启动时默认读取 `conf/app.ini`：

```ini
[mysql]
host = 127.0.0.1
port = 3306
username = root
password = admin
database = flow_talk
parse_time = true

[http]
addr = :8080
mode = debug

[jwt]
secret = dev-secret
ttl = 24h
```

启动前需要先创建数据库：

```sql
CREATE DATABASE flow_talk CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

建表文档在 [docs/数据库](./docs/数据库)。

## 安装依赖

```bash
make tidy
```

热重载开发安装 fresh：

```bash
make install-fresh
```

如果系统 PATH 中已经存在 `fresh`，`make fresh` 和 `make install-fresh` 会直接复用，不会重复下载。

## 启动项目

普通启动：

```bash
make run
```

热重载启动：

```bash
make fresh
```

服务默认监听：

```text
http://localhost:8080
```

WebSocket 入口：

```text
ws://localhost:8080/api/ws
```

## 常用命令

```text
make run        普通方式启动服务
make tidy       整理 Go module 依赖
make fresh      使用 fresh 热重载启动服务
make install-fresh 安装 fresh 热重载工具
make test       运行测试和编译检查
make vet        运行 Go 静态检查
```

## 接口入口

常用接口：

```text
POST /api/auth/register
POST /api/auth/login
POST /api/auth/external
GET  /api/me
GET  /api/conversations
POST /api/conversations/direct
POST /api/conversations/groups
GET  /api/conversations/:conversation_id/messages
POST /api/conversations/:conversation_id/messages  HTTP 降级发送
POST /api/devices
GET  /api/devices
DELETE /api/devices
POST /api/resources/upload
GET  /api/users?all=true
GET  /api/ws
```

聊天页面发送消息首选 WebSocket `/api/ws` 的 `message.send` 事件；HTTP 发送接口主要用于调试、脚本和 WebSocket 不可用时的降级。

完整接口以路由文件 [routers/router.go](./routers/router.go)、[docs/openapi.json](./docs/openapi.json) 和 [前端对接文档](./docs/前端对接文档.md) 为准。

## 常见问题

### 代理下载失败

```bash
make tidy GO_PROXY=https://proxy.golang.org,direct
make fresh GO_PROXY=https://proxy.golang.org,direct
```

### fresh 不存在

```bash
make install-fresh
make fresh
```
