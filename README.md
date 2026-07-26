# flow-talk-server

“流言”即时通讯后端，基于 Gin、GORM、MySQL 和 WebSocket；Redis 为可选的多实例实时通信组件。

业务能力、架构和接口说明见 [项目总览](./docs/OVERVIEW.md)。

## 环境要求

- Go 1.25+
- MySQL 8+
- Redis（可选，默认关闭）

## 快速启动

1. 修改本地配置 [conf/app.ini](./conf/app.ini)，至少确认 MySQL 连接和 JWT 密钥。
2. 创建数据库并按[数据库落地执行指南](./docs/数据库/数据库落地执行指南.md)完成建表。
3. 整理依赖并启动服务：

```bash
make tidy
make run
```

默认地址：

```text
HTTP      http://localhost:8080
WebSocket ws://localhost:8080/api/ws?token={jwt}&device_id={device_id}
```

生产环境不要直接使用仓库中的数据库密码和 `dev-secret`。

## 常用命令

```text
make run             启动服务
make fresh           热重载启动
make install-fresh   安装 fresh
make tidy            整理依赖
make test            全包编译检查
make vet             Go 静态检查
make build           Go 打包
```

如需更换 Go module 代理：

```bash
make run GO_PROXY=https://proxy.golang.org,direct
```

## 文档

- [项目总览](./docs/OVERVIEW.md)：能力、架构、数据模型和业务流程
- [OpenAPI](./docs/openapi.json)：接口定义
- [前端对接文档](./docs/前端对接文档.md)：客户端接入说明
- [数据库文档](./docs/数据库)：建表与数据关系
- [部署指南](./docs/Alibaba%20Cloud%20Linux%203%20部署指南.md)：生产部署参考

运行时路由以 [routers/router.go](./routers/router.go) 为准。
