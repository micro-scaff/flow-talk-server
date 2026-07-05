# 项目总览

`flow-talk-server` 是“流言”的后端服务，当前定位是一个可以本地独立运行的即时通讯服务。服务基于 Gin + GORM + MySQL + Redis，实现用户注册登录、会话、消息、资源上传、WebSocket 实时投递、在线状态和消息回执等基础能力。

运行、目录和本地环境配置请看 [README.md](../README.md)。本文档只说明项目目标、核心设计和主要业务流程。

## 当前能力分层

### v1：用户体系

- 本地用户注册、登录和 JWT 签发
- `Authorization: Bearer <token>` 鉴权
- 根据 token 获取当前用户
- `/api/users?all=true` 读取全部用户列表
- `users.external_id` 和 `users.auth_source` 为外部身份预留字段

### v2：会话基础

- 创建或获取单聊会话
- 创建群聊会话
- 查询当前用户会话列表
- 查询单个会话详情

### v3：消息与历史

- HTTP 发送消息并持久化
- 使用 `client_msg_id` 保证发送幂等
- 分页拉取历史消息
- 更新会话最后消息
- 标记会话已读
- 基于已读游标计算未读数

### v4：实时通信

- `/api/ws?token={jwt}&device_id={device_id}` WebSocket 建连鉴权
- `ping` / `pong` 应用层心跳
- WebSocket `message.send` 复用 v3 消息写入逻辑
- `message.ack` 确认入库
- `message.deliver` 向本机在线成员投递

### v5：设备与离线

- 当前用户设备上报、查询和删除
- WebSocket 建连或收到事件时刷新设备最近活跃时间
- 离线用户不依赖实时投递，上线后通过会话列表、未读数和历史消息补齐
- 设备详情保存为客户端上报的 JSON，服务端不限制内部字段

### v6：群管理

- 群聊添加成员、移除成员、退出群聊
- 群主设置
- 群主修改群资料

### v7：扩展能力

- demo 外部身份登录，换取 IM JWT
- 外部用户按 `external_id` 自动同步到 `users`
- 单实例在线状态查询和批量查询
- 单条消息已读/未读回执
- 会话内文本消息搜索
- 当前用户全部 active 会话文本消息搜索

## 文档导航

- [README.md](../README.md)：本地运行、目录结构、常用命令
- [docs/openapi.json](./openapi.json)：Apifox 可导入的 OpenAPI 文件
- [接口文档生成](./接口文档生成.md)：接口文档维护方式
- [前端对接文档](./前端对接文档.md)：前端项目生成和接口接入说明
- [用户管理服务接入](./用户管理服务接入.md)：外部用户管理服务登录态换取 IM token 的接入方案
- [数据库落地执行指南](./数据库/数据库落地执行指南.md)：本地 MySQL 建库建表步骤
- [数据库表关系](./数据库/数据库表关系.md)：表关系和落地顺序
- [图片、视频等资源存放](./图片、视频等资源存放.md)：上传资源约定

## 系统分层

```text
HTTP / WebSocket
  -> routers         路由注册和中间件挂载
  -> controllers    参数绑定、当前用户读取、响应转换
  -> models         业务规则、事务、数据库访问、WebSocket Hub
  -> MySQL / static 持久化数据和上传资源
```

当前项目默认保持轻量：不启用 Redis、MQ 或独立对象存储。WebSocket 在线连接和本机在线状态保存在单进程内存中，上传资源先落到本地 `static` 目录，MySQL 负责核心业务数据持久化。

项目已支持可选 Redis。`conf/app.ini` 中 `redis.enabled=false` 时继续使用单进程内存 Hub；改为 `true` 后，Redis Pub/Sub 用于跨节点实时投递，Redis TTL key/ZSET 用于全局在线状态。真实 WebSocket 连接仍保存在各进程自己的 `WSHub` 中。

## 核心设计

### 用户体系

当前阶段保留内置用户系统，服务可以不依赖外部登录平台独立完成注册、登录和鉴权流程。

同时项目已经预留外部身份接入边界：

- `AuthProvider`：按 provider 选择外部 token 校验器
- `TokenVerifier`：校验外部 access token，返回外部用户资料
- `external_id`：把外部用户稳定映射到 IM 内部 `users.id`

后续接企业登录、OAuth 或网关登录态时，消息、会话、群聊等业务仍只依赖内部 `users.id`。

接入外部用户管理服务时，请看 [用户管理服务接入](./用户管理服务接入.md)。

注意：当前本地用户密码仍用于打通开发流程，代码里也标记了后续应替换为 `password_hash`。

### 统一会话

单聊和群聊都存储在 `conversations`：

- 单聊：`type = direct`，使用稳定 `direct_key` 防止重复创建
- 群聊：`type = group`，通过 `owner_id`、`title`、`avatar_url` 管理群信息
- 成员关系：统一由 `conversation_members` 管理

统一模型可以让会话列表、消息分页、未读数、成员权限和搜索逻辑复用同一套实现。

### 消息写入

所有消息都写入 `messages`。客户端发送消息时必须提供 `client_msg_id`，服务端用 `(sender_id, client_msg_id)` 做幂等，避免断线重试产生重复消息。

发送消息时，服务端在同一个事务内完成：

1. 校验发送者是会话 active 成员
2. 写入或复用已有消息
3. 更新 `conversations.last_message_id`
4. 更新 `conversations.last_message_at`

### 实时投递

WebSocket 连接由单进程 `WSHub` 管理。一个用户可以同时有多条连接，例如 Web、移动端、桌面端。

当前实时投递策略：

- 在线成员：通过本进程 WebSocket 连接实时投递
- 慢连接：发送队列满时放弃本次实时投递
- 离线成员：不实时投递，依靠历史消息、会话列表和未读数补齐
- 多实例：启用 Redis 后通过 `RealtimeBus` 发布 `message.deliver`，每个节点订阅后只投递自己的本机连接

Redis 用于跨节点协同，但不替代本机 WebSocket 连接管理：

- 本机 `WSHub`：继续保存真实 WebSocket 连接、发送队列和连接生命周期
- Redis Pub/Sub：广播 `message.deliver` 等跨节点实时事件
- Redis TTL key 或 ZSET：记录用户在线连接快照，支撑全局在线状态查询
- MySQL：继续保存消息、会话、成员、回执等最终业务状态

## 数据模型

详细 SQL 请看 [docs/数据库](./数据库)。总览如下：

```text
users
  用户基础资料，本地登录账号，外部身份映射

conversations
  单聊和群聊统一会话，保存会话类型、群信息、最后消息

conversation_members
  会话成员、群角色、成员状态、已读游标

messages
  文本、图片、文件、系统消息，支持幂等发送

user_devices
  用户设备 JSON 数据、最近活跃时间

message_receipts
  单条消息的 已读/未读 回执
```

表关系：

```text
users 1 - N conversation_members
conversations 1 - N conversation_members
conversations 1 - N messages
users 1 - N messages
users 1 - 1 user_devices
messages 1 - N message_receipts
```

当前 SQL 暂不声明数据库外键，关系正确性由 model 层显式校验。这样本地开发和表结构迭代更轻，但所有写入入口都必须先确认上游记录和权限有效。

## 接口分组

本节描述目标接口设计；代码路由同步完成前，[routers/router.go](../routers/router.go) 仍代表当前实现。

### 认证

```text
POST /api/auth/register
POST /api/auth/login
POST /api/auth/external
GET  /api/me
```

### 会话和群聊

```text
GET    /api/conversations
POST   /api/conversations/detail
POST   /api/conversations/direct
POST   /api/conversations/groups
PATCH  /api/conversations/profile
POST   /api/conversations/members
DELETE /api/conversations/members
POST   /api/conversations/leave
PATCH  /api/conversations/members/role
```

会话和群聊接口中，除列表、创建单聊、创建群聊外，业务 ID 统一放到请求 body，不放在 URL 路径中。

查询会话详情：

```json
{
  "conversation_id": 1
}
```

修改群资料：

```json
{
  "conversation_id": 1,
  "title": "新群名",
  "avatar_url": ""
}
```

添加群成员：

```json
{
  "conversation_id": 1,
  "user_ids": [2, 3]
}
```

移除群成员：

```json
{
  "conversation_id": 1,
  "user_id": 2
}
```

退出群聊：

```json
{
  "conversation_id": 1
}
```

修改成员角色：

```json
{
  "conversation_id": 1,
  "user_id": 2,
  "role": "admin"
}
```

### 消息

```text
POST  /api/conversations/messages
POST  /api/conversations/messages/list
POST  /api/conversations/messages/search
POST  /api/conversations/read
POST  /api/messages/search
```

消息接口中，`conversation_id`、`message_id` 等业务 ID 统一放到请求 body，不放在 URL 路径中。

发送消息：

```json
{
  "conversation_id": 1,
  "client_msg_id": "client-001",
  "message_type": "text",
  "content": {
    "text": "hello"
  }
}
```

分页拉取历史消息：

```json
{
  "conversation_id": 1,
  "before_id": 1001,
  "limit": 20
}
```

搜索会话内消息：

```json
{
  "conversation_id": 1,
  "keyword": "hello",
  "limit": 20
}
```

搜索当前用户全部消息：

```json
{
  "keyword": "hello",
  "limit": 20
}
```

标记会话已读：

```json
{
  "conversation_id": 1,
  "last_read_message_id": 1001
}
```

### 设备、在线状态和回执

```text
POST   /api/devices
GET    /api/devices
DELETE /api/devices
POST   /api/users/presence
POST   /api/users/presence/batch
POST   /api/messages/receipts
POST   /api/messages/read
POST   /api/messages/unread
```

查询单个用户在线状态时，`user_id` 不放在 URL 路径中，统一放到请求 body：

```json
{
  "user_id": 2
}
```

消息回执接口中，`message_id` 不放在 URL 路径中，统一放到请求 body。当前只保留已读和未读两种状态。

```json
{
  "message_id": 1001
}
```

### 资源和调试

```text
POST /api/resources/upload
GET  /api/users?all=true
GET  /api/ws
```

用户列表支持查询参数：`all=true` 表示读取全部用户；不传 `all` 时可用 `limit`、`offset` 分页；`keyword` 可搜索用户名、昵称和外部 ID；`status`、`auth_source` 可按状态和来源过滤。

除注册、登录和 WebSocket 建连前鉴权外，业务接口默认需要 JWT。

## WebSocket 协议

建连地址：

```text
GET /api/ws?token={jwt}&device_id={device_id}
```

`token` 必填。`device_id` 可选，仅用于连接标识；服务端会按当前用户刷新设备最近活跃时间。

统一事件信封：

```json
{
  "type": "message.send",
  "request_id": "req-001",
  "payload": {}
}
```

### 心跳

客户端发送：

```json
{
  "type": "ping",
  "request_id": "ping-001"
}
```

服务端返回：

```json
{
  "type": "pong",
  "request_id": "ping-001",
  "payload": {
    "server_time": "2026-07-02T12:00:00+08:00"
  }
}
```

### 发送消息

聊天页面发送消息首选 WebSocket `message.send`，这样发送确认和对端实时投递都走同一条长连接协议。HTTP `POST /api/conversations/messages` 保留给调试、脚本、弱网降级和 WebSocket 不可用时的补偿发送。

客户端发送：

```json
{
  "type": "message.send",
  "request_id": "req-001",
  "payload": {
    "conversation_id": 1,
    "client_msg_id": "client-001",
    "message_type": "text",
    "content": {
      "text": "hello"
    }
  }
}
```

服务端先向当前连接返回 `message.ack`，表示消息已经完成入库：

```json
{
  "type": "message.ack",
  "request_id": "req-001",
  "payload": {
    "id": 1001,
    "conversation_id": 1,
    "sender_id": 2,
    "client_msg_id": "client-001",
    "message_type": "text",
    "content": {
      "text": "hello"
    },
    "status": "normal",
    "sent_at": "2026-07-02T12:00:00+08:00"
  }
}
```

随后服务端向会话 active 成员的在线连接广播 `message.deliver`：

```json
{
  "type": "message.deliver",
  "payload": {
    "id": 1001,
    "conversation_id": 1,
    "sender_id": 2,
    "message_type": "text",
    "content": {
      "text": "hello"
    },
    "status": "normal",
    "sent_at": "2026-07-02T12:00:00+08:00"
  }
}
```

前端处理建议：

1. 发送前生成稳定 `client_msg_id`，先在当前会话插入一条本地 `sending` 消息。
2. WebSocket 可用时发送 `message.send`，不要再同时调用 HTTP 发送接口，避免重复请求。
3. 收到 `message.ack` 后，用 `client_msg_id` 或服务端 `id` 把本地 `sending` 消息替换为服务端消息。
4. 收到 `message.deliver` 后，按 `id` 去重；如果属于当前会话则追加到消息列表，否则只刷新会话列表未读数和最后消息。
5. 如果 WebSocket 未连接或发送超时，可以使用同一个 `client_msg_id` 走 HTTP 发送接口降级；服务端会按 `(sender_id, client_msg_id)` 幂等。
6. 没收到 `message.ack` 时，客户端可以使用同一个 `client_msg_id` 重试，不要生成新的 `client_msg_id`。

事件处理失败时返回：

```json
{
  "type": "error",
  "request_id": "req-001",
  "payload": {
    "message": "参数校验失败"
  }
}
```

## 典型流程

### 本地登录

1. 客户端调用 `POST /api/auth/register` 创建用户。
2. 客户端调用 `POST /api/auth/login` 获取 JWT。
3. HTTP 接口通过 Authorization 传入 JWT。
4. WebSocket 通过 `/api/ws?token={jwt}` 建连。
5. 服务端每次鉴权后都会查询用户状态，禁用用户不能继续访问。

### 外部身份登录

1. 客户端先在外部系统完成登录。
2. 客户端调用 `POST /api/auth/external`，传入 provider 和 access token。
3. 服务端通过对应 `TokenVerifier` 校验外部登录态。
4. 服务端按 `external_id` 查找或同步内部用户。
5. 后续 IM 业务继续使用内部 `users.id`。

当前内置 `demo` provider 只用于本地联调，不访问真实第三方服务。

### 创建单聊

1. 客户端调用 `POST /api/conversations/direct`，传入目标用户 ID。
2. 服务端按两个用户 ID 生成稳定 `direct_key`。
3. 若会话已存在，直接返回已有会话。
4. 若不存在，在事务内创建 `conversations` 和双方 `conversation_members`。

### 发送消息

1. 聊天页面优先通过 WebSocket `message.send` 发送消息；HTTP 发送接口只作为调试或降级路径。
2. 服务端校验发送者仍是会话 active 成员。
3. 服务端用 `(sender_id, client_msg_id)` 做幂等。
4. 服务端写入 `messages` 并更新会话最后消息。
5. 服务端向在线 active 成员实时投递 `message.deliver`。
6. 离线成员上线后通过会话列表未读数和历史消息补齐。

### 标记已读

1. 客户端调用 `POST /api/conversations/read`，在 body 中传入 `conversation_id` 和 `last_read_message_id`。
2. 服务端校验当前用户是 active 成员。
3. 服务端校验 `last_read_message_id` 属于当前会话。
4. 只在新游标大于旧游标时更新 `conversation_members.last_read_message_id`。

## 落地注意事项

- 单聊必须通过 `direct_key` 和唯一索引防重复。
- 创建会话、写入成员、发送消息、更新最后消息等复合操作必须使用事务。
- 客户端必须生成稳定 `client_msg_id`，重试时复用同一个值。
- 上传资源只保存到本地 `static`，生产环境应替换为对象存储或独立资源服务。
- Redis 未启用时，在线状态只代表本进程视角，实时投递只覆盖本进程内连接。
- 多实例部署时建议启用 Redis；如果需要可重放投递或更强消费确认，可继续演进到 Redis Streams、MQ 或独立 WebSocket 网关。
- 当前未读数基于已读游标计算，数据量变大后可以再做缓存或冗余计数。
- 当前本地密码保存方式只适合开发阶段，正式环境应迁移到哈希存储。

## 后续扩展方向

- 密码哈希、登录限流、刷新 token、登出和 token 黑名单
- 真实外部身份 provider，例如 OAuth、企业微信、内部统一登录
- Redis Streams、MQ 或独立网关支撑更强的多实例投递确认和消费追踪
- 对象存储、资源鉴权、缩略图和视频转码
- 消息全文搜索索引
- 会话置顶、免打扰、草稿、多端同步
- 群公告、群邀请、群主转让、管理员权限细化
- APNs、FCM 或厂商离线推送
