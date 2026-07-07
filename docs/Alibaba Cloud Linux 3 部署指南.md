# Alibaba Cloud Linux 3 部署指南

适用系统：Alibaba Cloud Linux 3 / OpenAnolis。

约定：

- 使用 `root` 部署。
- 部署目录：`/root/flow-talk-server`。
- MySQL 已安装好，本文档不写 MySQL 安装。
- 服务端口默认：`8080`。
- 不需要 Redis 时保持 `redis.enabled=false`。

## 1. 准备服务器

```bash
dnf update -y
dnf install -y git make tar gzip vim curl wget
mkdir -p /root/flow-talk-server
```

安装 Go。项目当前要求 Go `1.25.4`：

```bash
cd /tmp
wget https://go.dev/dl/go1.25.4.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.25.4.linux-amd64.tar.gz
```

配置 Go 环境：

```bash
tee /etc/profile.d/go.sh >/dev/null <<'EOF'
export PATH=/usr/local/go/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
export GOTOOLCHAIN=local
EOF

source /etc/profile.d/go.sh
go version
```

## 2. 准备数据库

确认能连接已有 MySQL：

```bash
mysql -h 127.0.0.1 -P 3306 -u root -p
```

创建数据库：

```sql
CREATE DATABASE IF NOT EXISTS flow_talk
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;
```

项目不会自动建表，需要按顺序执行 `docs/数据库` 下的建表 SQL：

1. `数据库表创建-人员.md`
2. `数据库表创建-会话.md`
3. `数据库表创建-消息.md`
4. `数据库表创建-设备.md`
5. `数据库表创建-消息回执.md`

建表后检查：

```sql
USE flow_talk;
SHOW TABLES;
```

## 3. 部署代码

服务器上直接拉代码并编译：

```bash
git clone <你的仓库地址> /root/flow-talk-server
cd /root/flow-talk-server
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go test ./...
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go build -o flow-talk-server .
```

如果目录里已经有代码：

```bash
cd /root/flow-talk-server
git pull
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go test ./...
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go build -o flow-talk-server .
```

## 4. 本地打包

只需要 amd64 包时执行：

```bash
mkdir -p dist/flow-talk-server-linux-amd64/conf dist/flow-talk-server-linux-amd64/static
env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOCACHE="$PWD/.gocache" GOMODCACHE="$PWD/.gomodcache" GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go test ./...
env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOCACHE="$PWD/.gocache" GOMODCACHE="$PWD/.gomodcache" GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go build -o dist/flow-talk-server-linux-amd64/flow-talk-server .
cp -R conf/. dist/flow-talk-server-linux-amd64/conf/
cp -R static/. dist/flow-talk-server-linux-amd64/static/
cp README.md dist/flow-talk-server-linux-amd64/README.md
tar -czf dist/flow-talk-server-linux-amd64.tar.gz -C dist flow-talk-server-linux-amd64
```

ARM 服务器改用：

```bash
mkdir -p dist/flow-talk-server-linux-arm64/conf dist/flow-talk-server-linux-arm64/static
env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GOCACHE="$PWD/.gocache" GOMODCACHE="$PWD/.gomodcache" GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go test ./...
env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GOCACHE="$PWD/.gocache" GOMODCACHE="$PWD/.gomodcache" GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go build -o dist/flow-talk-server-linux-arm64/flow-talk-server .
cp -R conf/. dist/flow-talk-server-linux-arm64/conf/
cp -R static/. dist/flow-talk-server-linux-arm64/static/
cp README.md dist/flow-talk-server-linux-arm64/README.md
tar -czf dist/flow-talk-server-linux-arm64.tar.gz -C dist flow-talk-server-linux-arm64
```

产物：

```text
dist/flow-talk-server-linux-amd64.tar.gz
dist/flow-talk-server-linux-arm64.tar.gz
```

## 5. 修改配置

编辑：

```bash
vim /root/flow-talk-server/conf/app.ini
```

参考配置：

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

如果 MySQL 不在本机，修改 `host`、`port`、`username`、`password`。

## 6. systemd 启动（让 Go 服务变成 Linux 系统服务，由系统统一管理）

创建服务文件：

```bash
vim /etc/systemd/system/flow-talk-server.service
```

内容：

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

启动：

```bash
systemctl daemon-reload
systemctl enable --now flow-talk-server
systemctl status flow-talk-server
```

查看日志：

```bash
journalctl -u flow-talk-server -f
```

验证：

```bash
curl -i http://127.0.0.1:8080/api/users
```

返回 `401` 也说明服务已经启动，因为该接口需要登录。

## 7. Nginx 反向代理

如果直接用 `8080` 联调，只需要在 ECS 安全组放行 `8080/tcp`。

线上建议用 Nginx：

```bash
dnf install -y nginx
systemctl enable --now nginx
vim /etc/nginx/conf.d/flow-talk-server.conf
```

配置：

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

重载：

```bash
nginx -t
systemctl reload nginx
```

## 8. 常用维护

更新代码：

```bash
cd /root/flow-talk-server
git pull
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go test ./...
env PATH=/usr/local/go/bin:$PATH GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local go build -o flow-talk-server.new .
mv flow-talk-server flow-talk-server.bak.$(date +%Y%m%d%H%M%S)
mv flow-talk-server.new flow-talk-server
chmod 755 flow-talk-server
systemctl restart flow-talk-server
```

常见排查：

```bash
journalctl -u flow-talk-server -n 100 --no-pager
ls -l /root/flow-talk-server/conf/app.ini
mysql -h 127.0.0.1 -P 3306 -u root -p flow_talk
mkdir -p /root/flow-talk-server/static
```
