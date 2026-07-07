# Alibaba Cloud Linux 3 部署指南

本文档面向以下系统：

```text
Alibaba Cloud Linux 3.2104 U13.1 (OpenAnolis Edition)
ID_LIKE="rhel fedora centos anolis"
PLATFORM_ID="platform:al8"
```

`flow-talk-server` 是 Go + Gin 服务，运行时依赖 MySQL；Redis 为可选依赖，用于多实例 WebSocket 实时投递和全局在线状态。

## 目标部署形态

推荐先按单机部署落地：

```text
Nginx(80/443)
  -> flow-talk-server(:8080)
      -> MySQL(flow_talk)
      -> Redis(可选)
      -> static/ 本地上传资源
```

如果暂时没有域名或 HTTPS，也可以先只开放 `8080` 端口做联调。

本文档默认你已经用 `root` 登录服务器，项目直接部署到：

```text
/root/flow-talk-server
```

## 一、服务器准备

### 1. 安装基础工具

Alibaba Cloud Linux 3 兼容 RHEL/CentOS 系工具链，优先使用 `dnf`：

```bash
dnf update -y
dnf install -y git make tar gzip vim curl wget
```

### 2. 安装 Go

项目 `go.mod` 当前要求：

```text
go 1.25.4
```

建议到 Go 官方下载页选择 `linux-amd64` 或 `linux-arm64` 安装包：

```text
https://go.dev/dl/
```

示例安装方式如下，请把文件名替换成实际下载的版本和架构：

```bash
cd /tmp
wget https://go.dev/dl/go1.25.4.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.25.4.linux-amd64.tar.gz
```

配置环境变量：

```bash
tee /etc/profile.d/go.sh >/dev/null <<'EOF'
export PATH=/usr/local/go/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
export GOTOOLCHAIN=local
EOF

source /etc/profile.d/go.sh
go version
```

### 3. 创建部署目录

```bash
mkdir -p /root/flow-talk-server
```

## 二、确认已有 MySQL

本文档不包含 MySQL 安装步骤，默认你已经有可用的 MySQL。

先确认能连上数据库：

```bash
mysql -h 127.0.0.1 -P 3306 -u root -p
```

如果数据库在其他机器或 RDS，把 `127.0.0.1` 换成实际内网地址，并确认安全组或白名单允许当前服务器访问。

创建项目数据库：

```sql
CREATE DATABASE IF NOT EXISTS flow_talk
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;
```

如果 `flow_talk` 已经存在，可以跳过这条 SQL。

## 三、部署代码

### 方式 A：服务器上拉代码并编译

```bash
git clone <你的仓库地址> /root/flow-talk-server
cd /root/flow-talk-server
```

如果 `/root/flow-talk-server` 里已经有旧代码，不要重复 `git clone`，进入目录后执行 `git pull` 即可。

编译前先跑测试：

```bash
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go test ./...
```

编译二进制：

```bash
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go build -o flow-talk-server .
```

### 方式 B：本地交叉编译后上传

如果本地环境已经安装 Go，可以在本地编译 Linux 二进制：

```bash
GOOS=linux GOARCH=amd64 go build -o flow-talk-server .
```

ARM 服务器使用：

```bash
GOOS=linux GOARCH=arm64 go build -o flow-talk-server .
```

上传到服务器：

```bash
scp flow-talk-server root@服务器IP:/tmp/
scp -r conf static root@服务器IP:/tmp/
ssh root@服务器IP
mkdir -p /root/flow-talk-server/conf /root/flow-talk-server/static
cp /tmp/flow-talk-server /root/flow-talk-server/flow-talk-server
cp -R /tmp/conf/. /root/flow-talk-server/conf/
cp -R /tmp/static/. /root/flow-talk-server/static/
chmod 755 /root/flow-talk-server/flow-talk-server
```

注意：运行目录里必须有 `conf/`、`static/` 等目录，因为服务启动时会读取 `conf/app.ini`，并通过 `./static` 保存上传资源。后续修改生产配置时，编辑服务器上的 `/root/flow-talk-server/conf/app.ini`。

## 四、配置服务

编辑生产配置：

```bash
vim /root/flow-talk-server/conf/app.ini
```

推荐生产配置示例：

```ini
[mysql]
host = 127.0.0.1
port = 3306
username = root
password = 请替换成你的数据库密码
database = flow_talk
parse_time = true

[http]
addr = :8080
mode = release

[jwt]
secret = 请替换成至少32位随机字符串
ttl = 24h

[redis]
enabled = false
addr = 127.0.0.1:6379
password =
db = 0
key_prefix = flow-talk
presence_ttl = 90s
channel = flow-talk:message_deliver
instance_id =
```

关键点：

- `http.mode` 线上建议使用 `release`。
- `jwt.secret` 必须固定且足够随机；修改后已登录 token 会全部失效。
- 如果你的 MySQL 账号不是 `root`，把 `username` 和 `password` 改成实际账号。
- `http.addr = :8080` 表示监听所有网卡的 8080 端口。如果只允许本机 Nginx 访问，可以改成 `127.0.0.1:8080`。
- 当前代码默认读取相对路径 `conf/app.ini`，所以 systemd 的 `WorkingDirectory` 必须指向 `/root/flow-talk-server`。

## 五、初始化数据库表

项目不会在启动时自动建表，必须先执行 `docs/数据库` 下的建表 SQL。

建表顺序如下：

1. `docs/数据库/数据库表创建-人员.md`
2. `docs/数据库/数据库表创建-会话.md`
3. `docs/数据库/数据库表创建-消息.md`
4. `docs/数据库/数据库表创建-设备.md`
5. `docs/数据库/数据库表创建-消息回执.md`

登录数据库：

```bash
mysql -h 127.0.0.1 -P 3306 -u root -p flow_talk
```

按顺序复制每个文档中的 `CREATE TABLE` SQL 执行。执行完成后检查：

```sql
SHOW TABLES;
```

预期至少包含：

```text
users
conversations
conversation_members
messages
user_devices
message_receipts
```

更完整的数据库落地说明见 [数据库落地执行指南](./数据库/数据库落地执行指南.md)。

## 六、使用 systemd 托管

创建服务文件：

```bash
vim /etc/systemd/system/flow-talk-server.service
```

写入：

```ini
[Unit]
Description=flow-talk-server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/root/flow-talk-server
ExecStart=/root/flow-talk-server/flow-talk-server
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
systemctl daemon-reload
systemctl enable --now flow-talk-server
systemctl status flow-talk-server
```

查看日志：

```bash
journalctl -u flow-talk-server -f
```

本机验证：

```bash
curl -i http://127.0.0.1:8080/api/users
```

如果接口需要登录，返回 `401` 也说明 HTTP 服务已经起来了。

## 七、开放端口和安全组

### 只用 8080 联调

服务器防火墙：

```bash
dnf install -y firewalld
systemctl enable --now firewalld
firewall-cmd --add-port=8080/tcp --permanent
firewall-cmd --reload
```

阿里云 ECS 安全组也需要放行 `8080/tcp`。

### 使用 Nginx 反向代理

推荐线上只开放 `80/443`，不直接暴露 `8080`。

安装 Nginx：

```bash
dnf install -y nginx
systemctl enable --now nginx
```

如果本机启用了 firewalld，放行 HTTP/HTTPS：

```bash
firewall-cmd --add-service=http --permanent
firewall-cmd --add-service=https --permanent
firewall-cmd --reload
```

创建配置：

```bash
vim /etc/nginx/conf.d/flow-talk-server.conf
```

写入：

```nginx
server {
    listen 80;
    server_name example.com;

    client_max_body_size 100m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

检查并重载：

```bash
nginx -t
systemctl reload nginx
```

如果使用 HTTPS，可以用阿里云证书、Let's Encrypt 或 SLB/ALB 终止 TLS。前端 WebSocket 地址对应为：

```text
wss://example.com/api/ws
```

## 八、可选：启用 Redis

单实例部署可以不启用 Redis。多实例部署时建议启用 Redis，否则 WebSocket 在线连接和在线状态只在单个进程内有效。

安装 Redis：

```bash
dnf install -y redis
systemctl enable --now redis
systemctl status redis
```

修改 `conf/app.ini`：

```ini
[redis]
enabled = true
addr = 127.0.0.1:6379
password =
db = 0
key_prefix = flow-talk
presence_ttl = 90s
channel = flow-talk:message_deliver
instance_id =
```

重启服务：

```bash
systemctl restart flow-talk-server
journalctl -u flow-talk-server -n 100 --no-pager
```

如果使用阿里云 Redis，建议走专有网络内网地址，并在 Redis 白名单中只放行服务所在 ECS 或容器网段。

## 九、发布更新

服务器上拉代码发布：

```bash
cd /root/flow-talk-server
git pull
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go test ./...
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go build -o flow-talk-server.new .
mv flow-talk-server flow-talk-server.bak.$(date +%Y%m%d%H%M%S)
mv flow-talk-server.new flow-talk-server
chmod 755 flow-talk-server
systemctl restart flow-talk-server
systemctl status flow-talk-server
```

如果启动失败，可以回滚到上一版：

```bash
cd /root/flow-talk-server
systemctl stop flow-talk-server
cp flow-talk-server.bak.上一次时间戳 flow-talk-server
chmod 755 flow-talk-server
systemctl start flow-talk-server
```

## 十、常见问题

### 1. 启动时报 `读取配置文件失败`

确认 systemd 配置中有：

```ini
WorkingDirectory=/root/flow-talk-server
```

并确认文件存在：

```bash
ls -l /root/flow-talk-server/conf/app.ini
```

### 2. 启动时报 `Ping MySQL 失败`

检查配置、账号、数据库和网络：

```bash
mysql -h 127.0.0.1 -P 3306 -u root -p flow_talk
```

如果数据库不在本机，确认 ECS 和数据库在同一 VPC，数据库白名单或安全组允许 ECS 内网 IP。

### 3. 上传文件后访问 404

确认服务运行目录正确，且 `static` 目录存在：

```bash
mkdir -p /root/flow-talk-server/static
```

服务通过 `/api/static/...` 暴露本地 `static/` 目录。

### 4. WebSocket 连接失败

如果走 Nginx，必须配置：

```nginx
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
proxy_read_timeout 3600s;
```

同时确认前端地址使用：

```text
ws://服务器IP:8080/api/ws
```

或：

```text
wss://example.com/api/ws
```

### 5. `go test` 或 `go build` 下载依赖慢

使用国内代理：

```bash
GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go mod download
```

如果服务器不能访问公网，建议在本地编译 Linux 二进制后上传，或在内网搭建 Go module 代理。
