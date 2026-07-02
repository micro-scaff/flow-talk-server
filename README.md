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

- Go：项目 `go.mod` 当前要求 `go 1.25.4`
- MySQL：本地默认连接 `127.0.0.1:3306`
- fresh：可选，仅热重载开发时需要

如果本机 Go 版本低于项目要求，Go 会尝试自动下载对应 toolchain。若遇到 `toolchain not available`，可以直接安装 Go 1.25.4，或切换到支持 toolchain 下载的代理：

```bash
go env -w GOPROXY=https://proxy.golang.org,direct
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
go mod tidy
```

热重载开发需要额外安装 fresh：

```bash
go install github.com/pilu/fresh@v0.0.0-20240621171608-8d1fef547a99
```

安装后确认可执行文件存在：

```bash
test -x "$(go env GOPATH)/bin/fresh"
```

## 启动项目

普通启动：

```bash
make run
```

等价于：

```bash
GOCACHE=$(pwd)/.gocache go run .
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
ws://localhost:8080/ws
```

## 常用命令

```text
make run        普通方式启动服务
make fresh      使用 fresh 热重载启动服务
make test       运行测试和编译检查
make vet        运行 Go 静态检查
make cache-env  将 Go 默认 GOCACHE 设置到当前项目的 .gocache
```

`Makefile` 会把 Go 编译缓存写入项目内的 `.gocache/`，fresh 的临时构建文件写入 `tmp/`。

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
POST /api/conversations/:conversation_id/messages
POST /api/resources/upload
GET  /admin/users
GET  /ws
```

完整接口以路由文件 [routers/router.go](./routers/router.go) 和 [docs/openapi.json](./docs/openapi.json) 为准。

## 常见问题

### go: toolchain not available

项目要求 `go 1.25.4`，本机 Go 版本较低时会自动下载 toolchain。若当前代理不支持下载，会出现该错误。

可选处理方式：

```bash
go env -w GOPROXY=https://proxy.golang.org,direct
```

或者直接安装 Go 1.25.4 后重新执行：

```bash
go version
go mod tidy
```

### make fresh 报 /bin/fresh: No such file or directory

说明 `fresh` 没有安装成功，或者 `go env GOPATH` 因 toolchain 问题没有正常返回。

先解决 Go 版本或 toolchain 下载问题，再安装 fresh：

```bash
go install github.com/pilu/fresh@v0.0.0-20240621171608-8d1fef547a99
make fresh
```
