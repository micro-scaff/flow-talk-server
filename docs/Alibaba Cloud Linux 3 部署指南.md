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

## 2. 部署代码

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

## 3. 本地打包

默认打 amd64 包：

```bash
export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
export GOCACHE="$PWD/.gocache" GOMODCACHE="$PWD/.gomodcache"
export GOPROXY=https://goproxy.cn,direct GOTOOLCHAIN=local

OUT="dist/flow-talk-server-linux-$GOARCH"
mkdir -p "$OUT/conf" "$OUT/static"

go test ./...
go build -o "$OUT/flow-talk-server" .
cp -R conf/. "$OUT/conf/"
cp -R static/. "$OUT/static/"
tar -czf "$OUT.tar.gz" -C dist "$(basename "$OUT")"
```

说明：

- `GOOS=linux GOARCH=amd64` 表示生成 Linux amd64 服务器可运行的二进制；ARM 服务器把 `GOARCH=amd64` 改成 `GOARCH=arm64`。
- `CGO_ENABLED=0` 尽量生成不依赖本机 C 动态库的二进制，上传到服务器更省心。
- `GOCACHE` 和 `GOMODCACHE` 把 Go 缓存放在当前项目目录，避免污染系统目录，也方便清理。
- `go test ./...` 先跑测试，测试通过后再 `go build`。
- `cp -R conf/.` 和 `cp -R static/.` 把配置文件和静态目录一起放进部署包。
- `tar -czf` 会生成可上传的压缩包，例如 `dist/flow-talk-server-linux-amd64.tar.gz`。

打包后产物：

```text
dist/flow-talk-server-linux-amd64/
dist/flow-talk-server-linux-amd64.tar.gz
dist/flow-talk-server-linux-arm64/
dist/flow-talk-server-linux-arm64.tar.gz
```

## 4. 修改配置

如果当前目录已经是部署产物目录，例如：

```text
flow-talk-server-linux-amd64/
  conf/
  flow-talk-server
  static/
```

先进入该目录：

```bash
cd flow-talk-server-linux-amd64
```

编辑配置：

```bash
vim conf/app.ini
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

## 5. 启动服务

进入部署目录后，先给二进制加执行权限：

```bash
chmod +x ./flow-talk-server
```

前台启动，适合临时验证：

```bash
./flow-talk-server
```

看到类似下面日志，说明服务已经启动：

```text
flow-talk server listening on :8080
```

后台启动，适合不用 systemd 的简单部署：

```bash
nohup ./flow-talk-server > app.log 2>&1 &

# 成功的标识
[1] 20596
```

查看日志：

```bash
tail -f app.log
```

验证服务：

```bash
curl -i http://127.0.0.1:8080/api/users
```

返回 `401` 也说明服务已经启动，因为该接口需要登录。

```text
HTTP/1.1 401 Unauthorized
Access-Control-Allow-Headers: Origin, Content-Type, Accept, Authorization
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
Access-Control-Allow-Origin: *
Access-Control-Max-Age: 86400
Content-Type: application/json; charset=utf-8
Content-Length: 64

{"code":401,"data":null,"message":"未登录或登录已失效"}
```

如果服务没启动，会是之前那种：

```text
curl: (7) Failed to connect
```

停止服务：

```bash
pkill -f 'flow-talk-server'
```

确认是否已停止：

```bash
pgrep -af 'flow-talk-server'
```

如果启动时看到了进程号，例如 `[1] 20596`，也可以按 PID 停止：

```bash
kill 20596
```

如果还没停，再强制停止：

```bash
kill -9 20596
```

注意：`tail -f app.log` 时按 `Ctrl+C` 只是退出日志查看，不会停止后端服务。

## 6. systemd 启动（可选）

systemd 的作用是让服务开机自启、崩溃自动重启、统一查看日志。临时部署可以跳过这一节，直接用上一节的 `nohup`。

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

如果你的实际目录不是 `/root/flow-talk-server`，把 `WorkingDirectory` 和 `ExecStart` 改成真实路径。例如当前包目录在 `/root/server-go/flow-talk-server-linux-amd64`，则改成：

```ini
WorkingDirectory=/root/server-go/flow-talk-server-linux-amd64
ExecStart=/root/server-go/flow-talk-server-linux-amd64/flow-talk-server
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
