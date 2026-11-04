# 客户端（矿机）部署指南

本文说明如何在一台矿机上安装 `komari-miner-probe`，并把 GPU 与挖矿进程状态上报到
`https://prl.yjf123.top`。

- 探针是一个**单文件静态二进制**，无运行时依赖（不需要 Go、Python、Node、Docker）。
- 只用 Go 标准库，唯一的外部命令是 `nvidia-smi`（NVIDIA 驱动自带）或 `rocm-smi`（AMD）。
- 采集间隔默认 5 秒，单次开销通常 < 50 ms，对挖矿性能基本无影响。

---

## 1. 在服务端创建节点，拿到 token

打开 <https://prl.yjf123.top>，登录管理后台 →「添加节点」，填一个名字（如 `RIG-01`）。

创建后该客户端会有一对标识：

| 字段 | 示例 | 用途 |
| --- | --- | --- |
| `uuid` | `e0545d5d-0980-44f7-b2d2-ae39286786ce` | 节点唯一 ID，仅用于展示/API |
| `token` | `xAhJeZps0JLmgV5kDVVHxI` | **探针上报凭据，用这个** |

> ⚠️ 探针的 `--token` 必须填 **token**，不是 uuid。填 uuid 会返回 `401 Unauthorized.`。

---

## 2. 准备二进制

### 方式 A：直接从服务端下载（推荐）

服务端上已经放了一份 Linux amd64 版本：

```bash
# 在矿机上执行
curl -fL -o /usr/local/bin/komari-miner-probe \
  https://prl.yjf123.top/probe/komari-miner-probe
chmod +x /usr/local/bin/komari-miner-probe
komari-miner-probe --version
```

当前服务端已开放该路径（二进制为 Linux amd64 静态编译，可直接下载运行）。

### 方式 B：在任意一台有 Go 的机器上交叉编译

仓库内的 `komari/probe` 是独立 Go module：

```bash
cd komari/probe

# Linux amd64（大多数矿机）
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o komari-miner-probe .

# 其他常见组合
# GOOS=linux GOARCH=arm64   # ARM 版 HiveOS / 树莓派
# GOOS=windows GOARCH=amd64 # Windows 矿机
```

产物是 6~7 MB 的静态二进制，`scp` 到矿机的 `/usr/local/bin/` 即可。

### 方式 C：在矿机上直接构建

```bash
cd komari/probe && go build -o /usr/local/bin/komari-miner-probe .
```

---

## 3. 首次运行（先验证，再常驻）

先用 `--once` 打印一次采集结果，确认能读到 GPU 和矿工进程：

```bash
komari-miner-probe --once --no-dashboard | head -60
```

预期能看到 `gpus` 数组（含 `name`、`temperature`、`utilization`、`power`、`memory_used`）
和 `miners` 数组（含 `name`、`algorithm`、`pool`、`hashrate`）。

如果 `gpus` 为空：

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| `nvidia-smi: command not found` | 驱动未装或不在 PATH | 确认 `nvidia-smi` 能直接执行；无驱动则探针只能看到 CPU 矿工 |
| 有 GPU 但字段为空 | 驱动较老，缺 13 字段查询 | 探针会自动降级到 7 字段查询，属正常 |
| AMD 卡无数据 | `rocm-smi` 缺失 | 安装 ROCm 或忽略（NVIDIA 优先） |

然后做一次真实上报（前台跑，观察日志）：

```bash
komari-miner-probe \
  --name RIG-01 \
  --komari https://prl.yjf123.top \
  --token xAhJeZps0JLmgV5kDVVHxI \
  --interval 5
```

看到 `[miner-probe] probe 0.1.0 starting: ... report=true` 且没有 `report failed`，
刷新网页就能看到该节点在线，并出现「矿机监控」面板。

`Ctrl+C` 退出后进入下一步做常驻。

---

## 4. 配置常驻（systemd，Linux）

### 4.1 专用配置目录

```bash
sudo mkdir -p /etc/komari-probe
sudo tee /etc/komari-probe/config.json >/dev/null <<'JSON'
{
  "name": "RIG-01",
  "listen": "0.0.0.0:9090",
  "interval": 5,
  "komari_url": "https://prl.yjf123.top",
  "token": "xAhJeZps0JLmgV5kDVVHxI",
  "demo": false,
  "disable_dashboard": false,
  "miner_patterns": []
}
JSON
sudo chmod 600 /etc/komari-probe/config.json   # 含 token，收紧权限
```

`miner_patterns` 用于补充识别自定义矿工：填进程名的片段即可，例如
`["my-miner", "xmrig-custom"]`。内置已覆盖 30+ 常见矿工（t-rex、lolMiner、xmrig、
phoenixminer、nbminer、gminer、teamredminer、ethminer、srbminer 等）。

### 4.2 systemd 单元

```bash
sudo tee /etc/systemd/system/komari-miner-probe.service >/dev/null <<'UNIT'
[Unit]
Description=Komari Miner Probe
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/komari-miner-probe --config /etc/komari-probe/config.json
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable --now komari-miner-probe
sudo systemctl status komari-miner-probe --no-pager
```

> 注意：**不要**加 `NoNewPrivileges=true` 或 `PrivateDevices=true`，否则读不到
> `/dev/nvidia*`；HiveOS 的 miner 若以别的用户运行，探针用 root 跑才能看到全部进程。

日志与运维：

```bash
journalctl -u komari-miner-probe -f          # 实时日志
sudo systemctl restart komari-miner-probe    # 改完配置后重启
sudo systemctl disable --now komari-miner-probe  # 停用
```

---

## 5. 两种上报模式

### 5.1 仅上报（推荐，省资源）

```json
{ "disable_dashboard": false, "listen": "127.0.0.1:9090" }
```

内置看板打开但只在本机监听，需要时用 SSH 隧道查看：

```bash
ssh -L 9090:127.0.0.1:9090 user@矿机IP
# 浏览器打开 http://127.0.0.1:9090
```

想彻底关掉看板：`"disable_dashboard": true`（此时 `listen` 被忽略）。

### 5.2 看板对外暴露

```json
{ "listen": "0.0.0.0:9090", "disable_dashboard": false }
```

浏览器访问 `http://矿机IP:9090`。看板无鉴权，**不要把 9090 直接暴露到公网**，
在防火墙或内网里用。

---

## 6. 全部参数

优先级：**默认值 < config.json < 环境变量 < 命令行参数**。

| 命令行 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--config` | — | — | JSON 配置文件路径 |
| `--name` | `PROBE_NAME` | 主机名 | 节点显示名 |
| `--listen` | `PROBE_LISTEN` | `:9090` | 内置看板监听地址 |
| `--interval` | `PROBE_INTERVAL` | `5` | 采集/上报间隔（秒） |
| `--komari` | `KOMARI_SERVER` | 空 | Komari 地址，留空则只跑看板 |
| `--token` | `KOMARI_TOKEN` | 空 | 客户端 **token**（非 uuid） |
| `--demo` | `PROBE_DEMO` | `false` | 模拟 8 卡数据，无需 GPU |
| `--no-dashboard` | `PROBE_DISABLE_DASHBOARD` | `false` | 关闭看板，仅上报 |
| `--miners` | `PROBE_MINER_PATTERNS` | 空 | 额外矿工名片段，逗号分隔 |
| `--once` | — | `false` | 采集一次输出 JSON 后退出 |
| `--version` | — | — | 打印版本 |

另有 `PROBE_COMMAND_TIMEOUT` 控制单次 `nvidia-smi` 超时，默认 6 秒。

环境变量方式示例（适合 Docker / 一键脚本）：

```bash
export KOMARI_SERVER=https://prl.yjf123.top
export KOMARI_TOKEN=xAhJeZps0JLmgV5kDVVHxI
export PROBE_NAME=RIG-01
komari-miner-probe --no-dashboard
```

---

## 7. 常见问题

**上报 401 `Unauthorized.`**
`--token` 用了 uuid。改用「添加节点」返回的 `token` 字段。

**节点在线但面板显示「未上报 GPU / 挖矿数据」**
服务端收到的是旧版 agent 的数据（无 `gpu_detailed_info` / `mining` 字段）。
确认上报方是 `komari-miner-probe`，而不是官方 `komari-agent`。

**矿工进程识别不到**
进程名不在内置列表里。用 `--miners 你的进程名片段` 补充；
或看 `--once` 输出的 `miners` 是否为空，确认进程确实在跑。

**算力显示 0 并提示「算力接口不可用」**
非 XMRig 系矿工且未开 HTTP API 时，探针只能从命令行参数解析算力（通常拿不到）。
CPU 占用、显存、所在 GPU 仍然正常显示。开启矿工 API 可拿到真实算力
（XMRig 默认 `--http-enabled`，探针自动读 `/json_rpc`）。

**Windows 矿机**
把 `komari-miner-probe.exe` 放到任意目录，用管理员权限的 PowerShell 运行，
或注册为计划任务：

```powershell
schtasks /create /tn "KomariMinerProbe" /tr "C:\komari\komari-miner-probe.exe --config C:\komari\config.json" /sc onstart /ru SYSTEM /rl highest
```

GPU 依赖 `nvidia-smi` 在 PATH 中（默认已装）。

**HiveOS**
HiveOS 基于 Linux，按第 3、4 节操作即可。若用 HiveOS 的 `hive-*` 脚本管理，
可把探针放进 `/hive/miners/custom/` 用 minergroup 启动；权限同样需要 root。

**想确认上报内容**
在服务端执行：

```bash
curl -s -X POST https://prl.yjf123.top/api/rpc2 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"common:getNodesLatestStatus","params":{}}' \
  | jq '.result[] | {name, online, gpu: (.gpu_detailed_info|length), miners: (.mining.miners|length)}'
```

`online=true` 且 `gpu` / `miners` 数量正确即表示链路通。
