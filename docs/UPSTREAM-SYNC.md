# 上游同步清单

本项目是 Komari 的改造版。`server/` 与 `web-src/` 分别对应
[komari-monitor/komari](https://github.com/komari-monitor/komari) 和
[komari-monitor/komari-web](https://github.com/komari-monitor/komari-web)。

合并上游更新时，下面这些文件的本地改动需要重新应用。`agent/` 与
`server/tools/packdist/` 是纯新增目录，不会产生冲突。

## server/

| 文件 | 改动 |
| --- | --- |
| `protocol/v2/jsonrpc.go` | `Report.Mining *MiningReport`；`GPUDeviceInfo` 增加 `Index`/`UUID`/`MemoryTemperature`/`Power`/`PowerLimit`/`FanSpeed`/`ClockMHz`/`MemoryClockMHz`；新增 `MiningReport`、`MinerProcess` 类型 |
| `web/rpc/jsonrpc/common.go` | `recordLike` 增加 `Mining *v2.MiningReport`，并在构造最新状态时赋值 |
| `internal/metricstore/metrics.go` | 新增 5 个挖矿指标常量 |
| `internal/metricstore/definitions.go` | 追加对应的指标定义 |
| `internal/metricstore/report_mapping.go` | 上报数据 → 指标点的映射（含单进程 tag） |
| `internal/metricstore/metricstore_test.go` | 指标定义总数断言 21 → 26 |
| `internal/metricstore/mining_test.go` | 新增：验证挖矿指标写入 |
| `tools/packdist/main.go` | 新增：把前端 dist 打成 `dist.tar.zst` |

## web-src/

| 文件 | 改动 |
| --- | --- |
| `src/types/LiveData.tsx` | `MiningMiner`/`MiningReport` 类型，`Record.mining?`，GPU 字段扩展 |
| `src/contexts/LiveDataContext.tsx` | `mapLiveGpu` 保留 `gpu_detailed_info` 全字段；透传 `mining`；`sameLiveRecord` 比较 GPU/mining 变化 |
| `src/utils/mining.ts` | 新增：算力/时长格式化、钱包缩写、温度分级 |
| `src/components/MiningPanel.tsx` | 新增：矿机监控面板（概览、GPU 网格、矿工表、告警） |
| `src/pages/instance/index.tsx` | 在实例页渲染 `<MiningPanel />` |
| `src/components/Node.tsx` | 节点卡片增加 GPU 利用率条与总算力行 |
| `src/components/DetailsGrid.tsx` | GPU 型号后显示卡数 `(xN)` |
| `src/i18n/locales/zh_CN.json`、`en.json` | 新增 `mining.*` 文案 |

## 改动前端后的必做步骤

`server/web/public/defaultTheme/dist.tar.zst` 是生成物。上游把它 gitignore，本仓库
**特意提交**它，以便直接 `go build` 出完整服务端。任何前端改动都要重新生成：

```bash
bash scripts/build.sh
```

或手动：

```bash
cd web-src && npm ci && npm run build
cd ../server && go run ./tools/packdist -src ../web-src/dist -dst web/public/defaultTheme/dist.tar.zst
cp ../web-src/komari-theme.json web/public/defaultTheme/
go build -o komari-server .
```

否则二进制里内嵌的仍是旧界面——改了代码却看不到效果，通常就是这个原因。
