# 服务端部署指南

Komari Miner 服务端是一个 Go 二进制，内嵌了构建好的前端（`web/public/defaultTheme/dist.tar.zst`），
所以**只需要一个二进制 + 一个数据目录**就能跑起来。

- 默认监听 `0.0.0.0:25774`
- 数据库默认 SQLite（`./data/komari.db`），也支持 MySQL / PostgreSQL
- 首次访问会引导创建管理员账号

---

## 1. 编译

```bash
# 在仓库根目录
cd server
go build -o komari-server .

# 交叉编译到 Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o komari-server .
```

> 仓库里已经带了 `web/public/defaultTheme/dist.tar.zst`，所以 clone 下来直接 `go build`
> 就能得到含界面的二进制。**如果改过前端**，请先跑 `scripts/build.sh` 重新生成该文件。

### 改了前端怎么重建

```bash
# 需要 Node 22+ / npm
bash scripts/build.sh
```

脚本做三件事：`web-src` → `npm run build` → `tools/packdist` 打成 `dist.tar.zst` →
`go build` 出 `server/komari-server`。

---

## 2. 运行

```bash
./komari-server server --listen 0.0.0.0:25774
```

| 参数 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--listen` | `KOMARI_LISTEN` | `0.0.0.0:25774` | 监听地址 |
| `--database` | — | `./data/komari.db` | SQLite 文件路径 |
| `--db-type` | — | `sqlite` | `sqlite` / `mysql` / `postgres` |

首次启动后打开 `http://<主机>:25774`，按引导创建管理员。

---

## 3. systemd

```bash
sudo mkdir -p /opt/komari-miner/data
sudo cp komari-server /opt/komari-miner/

sudo tee /etc/systemd/system/komari-miner.service >/dev/null <<'UNIT'
[Unit]
Description=Komari Miner Monitor Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/komari-miner
ExecStart=/opt/komari-miner/komari-server server --listen 127.0.0.1:25774
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable --now komari-miner
journalctl -u komari-miner -f
```

监听 `127.0.0.1` 是配合下面的 Nginx 反代；如果直接对外暴露就用 `0.0.0.0:25774`。

---

## 4. Nginx 反向代理 + HTTPS

探针通过 HTTP/WebSocket 上报，反代需要透传 Upgrade 头、放开超时和请求体大小。

```nginx
upstream komari_miner_backend {
    server 127.0.0.1:25774;
    keepalive 32;
}

server {
    listen 80;
    listen [::]:80;
    server_name monitor.example.com;

    location ^~ /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
        default_type text/plain;
    }
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name monitor.example.com;

    ssl_certificate     /etc/nginx/certs/monitor.example.com_cert.pem;
    ssl_certificate_key /etc/nginx/certs/monitor.example.com_key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    client_max_body_size 1024m;

    location / {
        proxy_pass http://komari_miner_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;   # map 见下
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
        proxy_buffering off;
    }
}
```

`$connection_upgrade` 需要先在 `http {}` 里定义：

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      "";
}
```

### 证书

```bash
sudo certbot certonly --webroot -w /var/www/letsencrypt -d monitor.example.com
```

续期后需要把证书同步到 Nginx 并 reload，可放在
`/etc/letsencrypt/renewal-hooks/deploy/` 里写个脚本。

> 注意：如果默认站点对未知域名 `return 444`，ACME 的 HTTP 校验会失败。
> 记得给 `/.well-known/acme-challenge/` 放行。

---

## 5. 接入矿机

**在服务端「添加节点」创建客户端，拿到 `token`**（不是 uuid），然后在矿机执行：

```bash
komari-miner-probe \
  --name RIG-01 \
  --komari https://monitor.example.com \
  --token <客户端 token>
```

完整客户端说明见 [DEPLOY-CLIENT.md](DEPLOY-CLIENT.md)。

---

## 6. 验证链路

```bash
curl -s -X POST https://monitor.example.com/api/rpc2 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"common:getNodesLatestStatus","params":{}}' \
  | jq '.result[] | {name, online, gpu: (.gpu_detailed_info|length), miners: (.mining.miners|length)}'
```

期望 `online=true`，且 `gpu` / `miners` 数量与矿机实际一致。

---

## 7. 协议扩展说明

上报 payload（`agent.report`）新增可选字段 `mining`：

```jsonc
{
  "mining": {
    "miner_count": 3,
    "total_hashrate": 310089335,
    "total_power": 2612,
    "miners": [
      {
        "pid": 4012,
        "name": "t-rex",
        "algorithm": "kawpow",
        "hashrate": 123456789,
        "hashrate_text": "123.46 MH/s",
        "accepted_shares": 1024,
        "rejected_shares": 3,
        "pool": "stratum+tcp://pool.example:3333",
        "wallet": "0xabc...",
        "gpus": [0, 1, 2, 3],
        "cpu_percent": 12.5,
        "memory_bytes": 2147483648,
        "uptime": 86400,
        "command": "t-rex -a kawpow ...",
        "exe": "/opt/miner/t-rex",
        "status": "running"
      }
    ],
    "alerts": ["GPU #2 显存温度 97°C"]
  }
}
```

`gpu_detailed_info` 扩展字段：`index`、`uuid`、`memory_temperature`、`power`、
`power_limit`、`fan_speed`、`clock_mhz`、`memory_clock_mhz`。

字段全部为可选，旧 agent 上报不受影响；旧服务端收到新 agent 数据也只是忽略多余字段。

### 历史指标

新增 5 个指标，走与原有指标相同的保留策略与 rollup：

| 指标名 | 类型 | 说明 |
| --- | --- | --- |
| `miner.count` | gauge | 运行中的矿工数量 |
| `miner.hashrate` | gauge | 总算力（H/s） |
| `miner.power` | gauge | 矿工相关总功耗（W） |
| `miner.process.hashrate` | gauge（带 tag） | 单进程算力，tag: `pid` `miner` `algorithm` `pool` |
| `miner.process.shares` | counter（带 tag） | 单进程份额，tag 同上 |

---

## 8. 上游同步

本项目基于 [komari-monitor/komari](https://github.com/komari-monitor/komari)
和 [komari-monitor/komari-web](https://github.com/komari-monitor/komari-web)，遵循 MIT 许可证。

同步上游时需要关注的改动文件见 [UPSTREAM-SYNC.md](UPSTREAM-SYNC.md)。
