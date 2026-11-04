# komari-miner

把 [Komari](https://github.com/komari-monitor/komari) 改造成**矿机监控面板**：以 GPU 状态和挖矿进程为主角。

- `server/` — Komari 服务端（Go + Gin + GORM），在协议、指标存储、API 上做了矿机监控扩展
- `agent/` — `komari-miner-probe` 客户端探针（Go，仅标准库），采集 GPU / 挖矿进程，可上报到服务端
- `web-src/` — 前端源码（React + Vite + Tailwind），新增「矿机监控」面板

## 它做了什么

上游 Komari 面向通用服务器监控（CPU / 内存 / 磁盘 / 网络）。本项目在其之上增加了矿机需要的东西：

| 能力 | 说明 |
| --- | --- |
| GPU 明细 | 型号、显存用量、利用率、核心温度、**显存温度**、功耗与功耗上限、风扇、核心/显存频率 |
| 挖矿进程 | 自动识别 30+ 常见矿工，解析算法、矿池、钱包、算力、份额、CPU/内存占用、所在 GPU |
| 真实算力 | 通过矿工本地 JSON-RPC（XMRig `/json_rpc`、`/2/summary`）读取，而非从日志猜 |
| 告警 | 核心温度、显存温度、算力接口不可用时给出提示 |
| 独立看板 | 探针自带嵌入式网页，矿机上不依赖服务端也能看状态 |
| 历史指标 | `miner.count` / `miner.hashrate` / `miner.power` / 单进程算力与份额，纳入原有保留策略 |

前端在实例页顶部新增「矿机监控」面板：概览卡、GPU 卡片网格、矿工表格、算力走势图、告警区。节点卡片上也会显示 GPU 利用率条和总算力。

## 快速开始

### 服务端

```bash
cd server
go build -o komari-server .
./komari-server server --listen 0.0.0.0:25774
```

首次访问会引导创建管理员账号。详细部署（Nginx / HTTPS / systemd）见 [docs/DEPLOY.md](docs/DEPLOY.md)。

### 客户端（矿机）

服务端「添加节点」拿到 **token**（不是 uuid），然后：

```bash
cd agent
go build -o komari-miner-probe .

./komari-miner-probe \
  --name RIG-01 \
  --komari http://<服务端地址>:25774 \
  --token <客户端 token> \
  --listen :9090
```

浏览器打开 `http://矿机IP:9090` 即是探针自带看板。无水无卡的环境可以用 `--demo` 看效果：

```bash
./komari-miner-probe --demo --listen :9090
```

完整说明（systemd、配置项、HiveOS、Windows、排错）见 [docs/DEPLOY-CLIENT.md](docs/DEPLOY-CLIENT.md)。

## 目录结构

```
server/                     Komari 服务端
  protocol/v2/              agent 上报协议（新增 mining 段与 GPU 扩展字段）
  internal/metricstore/     历史指标存储（新增 5 个挖矿指标）
  web/rpc/jsonrpc/          最新状态下发（透传 gpu_detailed_info / mining）
  tools/packdist/           把前端产物打包成服务端内嵌主题
  web/public/defaultTheme/  内嵌前端产物（dist.tar.zst，由脚本生成）
agent/                      komari-miner-probe
  main.go                   参数 / 环境变量 / 配置文件解析
  internal/probe/           采集、解析、识别、看板、上报
    assets/                 嵌入式看板（HTML/CSS/JS，无构建步骤）
web-src/                    前端源码
  src/components/MiningPanel.tsx   矿机监控面板
scripts/build.sh            一键构建服务端（前端 + 主题包 + 二进制）
```

## 构建前端

前端改动后需要重新打包给服务端内嵌：

```bash
scripts/build.sh                  # 完整构建：前端 -> dist.tar.zst -> server 二进制
cd server && go build -o komari-server .   # 只重编服务端（复用已有 dist.tar.zst）
```

`server/web/public/defaultTheme/dist.tar.zst` 是生成物，上游默认 gitignore。本仓库为了
「clone 下来就能 `go build` 出一个完整可用的服务端」而**包含**它。如果你改前端，
记得重跑 `scripts/build.sh` 让内嵌产物跟上。

## 协议扩展

上报 payload 增加一个**可选**字段，旧 agent / 旧服务端互不影响：

```jsonc
{
  // ... 原有字段
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
        "status": "running"
      }
    ],
    "alerts": ["GPU #2 显存温度 97°C"]
  }
}
```

`gpu_detailed_info` 同时扩展了 `index` / `uuid` / `memory_temperature` / `power` /
`power_limit` / `fan_speed` / `clock_mhz` / `memory_clock_mhz`。

## 与原项目的关系

`server/` 和 `web-src/` 源自 [komari-monitor/komari](https://github.com/komari-monitor/komari)
与 [komari-monitor/komari-web](https://github.com/komari-monitor/komari-web)，遵循其
MIT 许可证（见 `server/LICENSE`）。上游同步时需要保留的文件改动列在
[docs/UPSTREAM-SYNC.md](docs/UPSTREAM-SYNC.md)。

`agent/` 是本项目新增的独立程序，通过 `replace` 引用 `server/protocol/v2` 以保证协议一致。

## 已知限制

- **Linux 是生产目标**。macOS 只能读到 GPU 型号（无利用率/功耗），Windows GPU 采集为桩实现。
- 非 XMRig 系矿工若未开 HTTP API，算力可能显示 0（进程、功耗、温度仍正常）。
- 探针看板无鉴权，不要直接暴露到公网。
