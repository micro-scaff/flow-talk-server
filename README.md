# flow-talk-server

流言：一个基于 Go + MySQL 的即时通讯服务。

## 第一版目标

第一版实现一个群聊基础版 IM 服务，覆盖完整的即时通讯主流程，同时为后续扩展预留接口。

核心能力：

- 当前阶段提供内置用户注册、登录、JWT 鉴权
- 预留后续接入外部登录系统的身份适配能力
- WebSocket 长连接实时收发消息
- HTTP 接口管理登录注册、会话、历史消息、群聊
- 单聊与群聊统一建模
- MySQL 持久化消息、会话、成员关系
- 支持离线消息、会话级未读数
- 预留移动端离线推送设备信息

## 设计选择

### 用户体系

当前阶段保留内置用户系统，由 IM 服务提供注册、登录和 JWT 鉴权。这样服务可以独立运行，前端或测试客户端不依赖外部登录系统也能完成完整 IM 流程。

同时，用户体系需要预留后续接入外部登录系统的能力。业务层不直接依赖账号密码登录细节，而是通过鉴权抽象获取当前用户：

- `AuthProvider`：负责本地登录、外部登录态校验、用户资料同步等能力
- `TokenVerifier`：负责校验当前请求携带的 token，并解析当前用户身份
- `UserSyncer`：负责把外部用户资料同步到 IM 的 `users` 表

后续接入外部登录系统时，只需要替换或新增 `AuthProvider` 实现，并通过 `external_id` 绑定外部用户，不影响 IM 消息、会话、群聊主流程。

### 实时通信

第一版采用 WebSocket 为主：

- WebSocket：实时收发消息、在线通知、心跳
- HTTP：登录注册、会话列表、历史消息、群管理、设备信息上报

同时预留推送通道，后续可接入 APNs、FCM 或厂商推送。第一版在线用户通过 WebSocket 投递，离线用户只保存消息和设备信息。

### 数据模型

采用统一会话模型：

- 单聊和群聊都属于 `conversations`
- 消息统一存储在 `messages`
- 成员、未读数、已读游标通过 `conversation_members` 管理

这种方式避免单聊和群聊写两套逻辑，后续扩展会话列表、多端同步、历史消息和消息搜索更简单。

## MySQL 表设计

### users

用户表。当前阶段支持本地注册登录，同时预留外部登录系统用户映射。

```sql
CREATE TABLE users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  external_id VARCHAR(128) NULL,
  username VARCHAR(64) NOT NULL,
  password_hash VARCHAR(255) NULL,
  nickname VARCHAR(64) NOT NULL,
  avatar_url VARCHAR(255) NULL,
  auth_source ENUM('local', 'external') NOT NULL DEFAULT 'local',
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_users_username (username),
  UNIQUE KEY uk_users_external_id (external_id)
);
```

字段说明：

- `external_id`：外部登录系统中的用户 ID，本地用户可为空
- `username`：本地登录账号；外部用户接入后可使用外部账号名或生成稳定用户名
- `password_hash`：本地用户密码哈希；外部用户可为空
- `nickname`：IM 展示昵称，可本地设置或由外部系统同步
- `auth_source`：用户来源，`local` 表示本地注册，`external` 表示外部系统同步
- `status`：用户状态，第一版可用 `1` 表示正常，`0` 表示禁用

### conversations

会话表。单聊和群聊都存在这里。

```sql
CREATE TABLE conversations (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  type ENUM('direct', 'group') NOT NULL,
  title VARCHAR(128) NULL,
  avatar_url VARCHAR(255) NULL,
  owner_id BIGINT NULL,
  last_message_id BIGINT NULL,
  last_message_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_conversations_last_message_at (last_message_at),
  KEY idx_conversations_owner_id (owner_id)
);
```

字段说明：

- `type`：`direct` 表示单聊，`group` 表示群聊
- `title`：群聊名称；单聊可为空，由客户端根据对方用户展示
- `owner_id`：群主用户 ID；单聊可为空
- `last_message_id` / `last_message_at`：用于会话列表排序

### conversation_members

会话成员表。负责单聊成员、群成员、群角色、已读游标和成员状态。

```sql
CREATE TABLE conversation_members (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  conversation_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  role ENUM('owner', 'admin', 'member') NOT NULL DEFAULT 'member',
  joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  muted_until DATETIME NULL,
  last_read_message_id BIGINT NULL,
  last_read_at DATETIME NULL,
  status ENUM('active', 'left', 'removed') NOT NULL DEFAULT 'active',
  UNIQUE KEY uk_conversation_members_conversation_user (conversation_id, user_id),
  KEY idx_conversation_members_user_status (user_id, status),
  KEY idx_conversation_members_conversation_status (conversation_id, status)
);
```

字段说明：

- `role`：群角色；单聊成员统一使用 `member`
- `muted_until`：预留免打扰或禁言时间
- `last_read_message_id`：会话级已读游标，用于计算未读数
- `status`：成员状态，支持退出群聊、被移除等场景

### messages

消息表。所有单聊和群聊消息统一存储。

```sql
CREATE TABLE messages (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  conversation_id BIGINT NOT NULL,
  sender_id BIGINT NOT NULL,
  client_msg_id VARCHAR(64) NOT NULL,
  message_type ENUM('text', 'image', 'file', 'system') NOT NULL,
  content JSON NOT NULL,
  status ENUM('normal', 'recalled', 'deleted') NOT NULL DEFAULT 'normal',
  sent_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_messages_sender_client_msg (sender_id, client_msg_id),
  KEY idx_messages_conversation_id_id (conversation_id, id),
  KEY idx_messages_sent_at (sent_at)
);
```

字段说明：

- `client_msg_id`：客户端生成的消息 ID，用于断线重试时幂等去重
- `message_type`：消息类型，第一版支持文本、图片、文件、系统消息
- `content`：JSON 内容，不同消息类型使用不同结构
- `status`：支持后续撤回、删除

文本消息示例：

```json
{
  "text": "hello"
}
```

文件消息示例：

```json
{
  "url": "https://example.com/a.png",
  "name": "a.png",
  "size": 12345
}
```

### user_devices

用户设备表。第一版用于记录在线设备和离线推送信息，后续接入移动端推送。

```sql
CREATE TABLE user_devices (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  device_id VARCHAR(128) NOT NULL,
  platform ENUM('web', 'ios', 'android', 'desktop') NOT NULL,
  push_token VARCHAR(255) NULL,
  last_seen_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_user_devices_user_device (user_id, device_id)
);
```

字段说明：

- `device_id`：客户端设备唯一标识
- `platform`：设备平台
- `push_token`：移动端离线推送 token，第一版可为空
- `last_seen_at`：最近在线时间

### message_receipts

消息回执表。第一版可以暂缓实现，后续需要展示“某条消息谁已读”时再启用。

```sql
CREATE TABLE message_receipts (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  message_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  status ENUM('delivered', 'read') NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_message_receipts_message_user (message_id, user_id)
);
```

第一版建议只使用 `conversation_members.last_read_message_id` 做会话级已读和未读数。`message_receipts` 作为扩展表保留在设计中，不进入首批强依赖。

## 表关系

```text
users 1 - N conversation_members
conversations 1 - N conversation_members
conversations 1 - N messages
users 1 - N messages
users 1 - N user_devices
messages 1 - N message_receipts
```

## 第一版必须落地的表

- `users`
- `conversations`
- `conversation_members`
- `messages`
- `user_devices`

## 第一版暂缓的表

- `message_receipts`

暂缓原因：会话级未读数可以通过 `conversation_members.last_read_message_id` 和 `messages.id` 计算。逐条消息已读详情属于更细粒度能力，可以在后续版本加入。

## 典型流程

### 当前阶段登录

1. 客户端调用注册接口创建本地用户，服务端写入 `users`，并保存 `password_hash`。
2. 客户端调用登录接口提交 `username` 和密码。
3. 服务端校验密码后签发 JWT。
4. 客户端访问 IM HTTP 或 WebSocket 接口时携带 JWT。
5. IM 服务通过 `TokenVerifier` 校验 JWT，并解析 `users.id`。

### 后续外部身份接入

1. 客户端先在外部登录系统完成登录。
2. 客户端访问 IM HTTP 或 WebSocket 接口时携带外部登录态。
3. IM 服务通过外部登录适配版 `AuthProvider` 校验登录态，并解析 `external_id`。
4. IM 服务通过 `UserSyncer` 查询或同步用户资料到 `users` 表。
5. 后续 IM 业务仍然只使用 `users.id` 作为内部用户 ID。

### 发送消息

1. 客户端通过 WebSocket 发送消息，携带 `conversation_id`、`client_msg_id`、`message_type` 和 `content`。
2. 服务端校验发送者是否是会话成员。
3. 服务端通过 `(sender_id, client_msg_id)` 做幂等检查。
4. 服务端写入 `messages`。
5. 服务端更新 `conversations.last_message_id` 和 `conversations.last_message_at`。
6. 服务端查找在线成员，通过 WebSocket 投递消息。
7. 离线成员不实时投递，后续上线后通过历史消息和未读数同步。

### 拉取会话列表

1. 根据当前用户查询 `conversation_members`。
2. 关联 `conversations` 获取会话基础信息。
3. 关联 `messages` 获取最后一条消息。
4. 使用 `messages.id > conversation_members.last_read_message_id` 计算未读数。
5. 按 `last_message_at` 倒序返回。

### 标记已读

1. 客户端上报某个会话已读到的 `message_id`。
2. 服务端校验该消息属于当前会话。
3. 更新 `conversation_members.last_read_message_id` 和 `last_read_at`。
4. 后续会话列表未读数基于该游标重新计算。

## 后续扩展方向

- 外部身份适配：按不同外部登录系统实现不同的 `AuthProvider`
- 多实例部署：引入 Redis Pub/Sub 或消息队列做跨节点投递
- 在线状态：使用 Redis 维护用户连接状态
- 离线推送：基于 `user_devices.push_token` 接入 APNs、FCM 或厂商推送
- 消息搜索：为 `messages.content` 建立独立搜索索引
- 消息分表：按 `conversation_id` 或时间范围拆分 `messages`
- 精细已读回执：启用 `message_receipts`
