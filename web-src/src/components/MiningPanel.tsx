import { useEffect, useMemo, useRef, useState } from "react";
import { Badge, Card, Text } from "@radix-ui/themes";
import { useTranslation } from "react-i18next";
import type { MiningMiner, Record as LiveRecord } from "@/types/LiveData";
import { formatBytes } from "@/utils/unitHelper";
import {
  formatHashrate,
  formatMiningDuration,
  miningTempClass,
  shortWallet,
} from "@/utils/mining";

type MiningPanelProps = {
  uuid?: string;
  nodeName?: string;
  record?: LiveRecord;
  className?: string;
};

type SparkPoint = { t: number; v: number };

const Sparkline = ({ data }: { data: SparkPoint[] }) => {
  if (data.length < 2) {
    return (
      <div className="flex h-12 w-full items-center justify-center text-[10px] text-[var(--gray-9)]">
        正在积累算力采样…
      </div>
    );
  }
  const max = Math.max(1, ...data.map((p) => p.v)) * 1.12;
  const points = data
    .map((p, i) => {
      const x = (i / (data.length - 1)) * 100;
      const y = 30 - (p.v / max) * 25;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
  return (
    <svg
      viewBox="0 0 100 32"
      preserveAspectRatio="none"
      className="h-12 w-full"
      role="img"
      aria-label="hashrate trend"
    >
      <defs>
        <linearGradient id="km-mining-spark-fill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#22d3ee" stopOpacity="0.35" />
          <stop offset="100%" stopColor="#22d3ee" stopOpacity="0.02" />
        </linearGradient>
      </defs>
      <polygon
        points={`0,32 ${points} 100,32`}
        fill="url(#km-mining-spark-fill)"
      />
      <polyline
        points={points}
        fill="none"
        stroke="#22d3ee"
        strokeWidth="1.4"
        vectorEffect="non-scaling-stroke"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
};

const Stat = ({
  label,
  value,
  unit,
  foot,
  accent,
}: {
  label: string;
  value: string;
  unit?: string;
  foot?: string;
  accent: string;
}) => (
  <div className="relative overflow-hidden rounded-xl border border-[var(--gray-a4)] bg-[var(--gray-a2)] px-3 py-2.5">
    <div
      className="absolute inset-x-0 top-0 h-[2px] opacity-80"
      style={{ background: `linear-gradient(90deg, ${accent}, transparent 75%)` }}
    />
    <div className="text-[10px] uppercase tracking-wide text-[var(--gray-10)]">
      {label}
    </div>
    <div className="mt-1 truncate font-mono text-xl font-semibold leading-tight">
      {value}
      {unit && (
        <span className="ml-1 text-[10px] font-normal text-[var(--gray-10)]">
          {unit}
        </span>
      )}
    </div>
    {foot && (
      <div className="mt-1 truncate text-[10px] font-mono text-[var(--gray-9)]" title={foot}>
        {foot}
      </div>
    )}
  </div>
);

const GpuChip = ({ miner }: { miner: MiningMiner }) => (
  <span className="inline-flex items-center gap-1 rounded-full border border-violet-500/30 bg-violet-500/10 px-1.5 py-0.5 font-mono text-[9.5px] text-violet-300">
    ⚒ {miner.name}
    <b className="font-semibold text-cyan-300">
      {miner.hashrate_text || formatHashrate(miner.hashrate)}
    </b>
  </span>
);

export default function MiningPanel({
  uuid,
  nodeName,
  record,
  className,
}: MiningPanelProps) {
  const { t } = useTranslation();
  const [history, setHistory] = useState<SparkPoint[]>([]);
  const lastStamp = useRef<string | number | null>(null);

  useEffect(() => {
    setHistory([]);
    lastStamp.current = null;
  }, [uuid]);

  const hashrate = record?.mining?.total_hashrate ?? 0;
  const updatedAt = record?.updated_at;

  useEffect(() => {
    if (!record?.mining) return;
    const stamp = updatedAt ?? Date.now();
    if (lastStamp.current === stamp) return;
    lastStamp.current = stamp;
    setHistory((prev) => [...prev, { t: Date.now(), v: hashrate }].slice(-120));
  }, [hashrate, record?.mining, updatedAt]);

  const gpus = record?.gpu?.detailed_info ?? [];
  const miners = record?.mining?.miners ?? [];

  const summary = useMemo(() => {
    const avgUtil = Number(record?.gpu?.average_usage ?? 0);
    const totalPower =
      record?.mining?.total_power ??
      gpus.reduce((sum, gpu) => sum + Number(gpu.power ?? 0), 0);
    const maxTemp = gpus.reduce(
      (max, gpu) =>
        Math.max(
          max,
          Number(gpu.temperature ?? 0),
          Number(gpu.memory_temperature ?? 0),
        ),
      0,
    );
    const hotCount = gpus.filter(
      (gpu) =>
        Math.max(
          Number(gpu.temperature ?? 0),
          Number(gpu.memory_temperature ?? 0),
        ) >= 80,
    ).length;
    return { avgUtil, totalPower, maxTemp, hotCount };
  }, [gpus, record?.gpu?.average_usage, record?.mining?.total_power]);

  if (!record) {
    return (
      <Card className="w-full overflow-hidden p-4 text-[12px] text-[var(--gray-9)]">
        正在等待探针上报…
      </Card>
    );
  }
  if (!record.mining && gpus.length === 0) {
    return (
      <Card className="w-full overflow-hidden p-4 text-[12px] text-[var(--gray-9)]">
        该节点的 agent 未上报 GPU / 挖矿数据（需要 komari-miner-probe 或带 --gpu 的官方 agent）。
      </Card>
    );
  }

  const minersForGpu = (index: number) =>
    miners.filter((miner) => (miner.gpus ?? []).includes(index));

  const alerts = record?.mining?.alerts ?? [];

  return (
    <Card
      className={`km-mining-panel w-full max-w-[1200px] overflow-hidden p-0 ${className ?? ""}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[var(--gray-a4)] bg-gradient-to-r from-cyan-500/[0.07] via-transparent to-violet-500/[0.06] px-5 py-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-cyan-400">⛏</span>
            <Text size="3" weight="bold">
              {t("mining.title", "矿机监控")}
            </Text>
            <Badge color="cyan" variant="soft">
              {miners.length} {t("mining.miners", "miners")}
            </Badge>
            <Badge color="violet" variant="soft">
              {gpus.length} GPU
            </Badge>
          </div>
          <div className="mt-1 truncate font-mono text-[11px] text-[var(--gray-10)]">
            {nodeName || uuid || "rig"}
            {record?.message ? ` · ${record.message}` : ""}
          </div>
        </div>
        <div className="flex items-center gap-4">
          <div className="hidden sm:block w-[180px]">
            <Sparkline data={history} />
          </div>
          <div className="text-right">
            <div className="font-mono text-2xl font-bold leading-tight text-cyan-400">
              {formatHashrate(hashrate)}
            </div>
            <div className="text-[10px] uppercase tracking-wide text-[var(--gray-10)]">
              {t("mining.totalHashrate", "Total hashrate")}
            </div>
          </div>
        </div>
      </div>

      {alerts.length > 0 && (
        <div className="flex flex-col gap-1.5 border-b border-amber-500/20 bg-amber-500/[0.06] px-5 py-2.5">
          {alerts.map((alert) => (
            <div key={alert} className="flex items-start gap-2 text-[12px] text-amber-300">
              <span className="mt-[1px]">◆</span>
              <span>{alert}</span>
            </div>
          ))}
        </div>
      )}

      <div className="grid grid-cols-2 gap-2 border-b border-[var(--gray-a4)] px-4 py-3 md:grid-cols-3 xl:grid-cols-6">
        <Stat
          label={t("mining.totalHashrate", "总算力")}
          value={formatHashrate(hashrate)}
          foot={`${miners.length} ${t("mining.miners", "miners")}`}
          accent="#22d3ee"
        />
        <Stat
          label={t("mining.miners", "挖矿进程")}
          value={String(miners.length)}
          unit="active"
          foot={miners.map((m) => m.name).join(" · ") || "—"}
          accent="#34d399"
        />
        <Stat
          label={t("mining.gpus", "GPU 数量")}
          value={String(gpus.length)}
          unit="cards"
          foot={`${record?.gpu?.count ?? gpus.length} ${t("mining.detected", "detected")}`}
          accent="#a78bfa"
        />
        <Stat
          label={t("mining.avgUtil", "平均利用率")}
          value={`${summary.avgUtil.toFixed(1)}%`}
          foot={`${t("mining.maxTemp", "最高温度")} ${summary.maxTemp}°C`}
          accent="#fbbf24"
        />
        <Stat
          label={t("mining.power", "总功耗")}
          value={`${Math.round(summary.totalPower)}`}
          unit="W"
          foot={
            gpus.length
              ? `${Math.round(summary.totalPower / gpus.length)} W / GPU`
              : "—"
          }
          accent="#fb7185"
        />
        <Stat
          label={t("mining.maxTemp", "最高温度")}
          value={`${summary.maxTemp}`}
          unit="°C"
          foot={
            summary.hotCount
              ? `${summary.hotCount} GPU ≥ 80°C`
              : t("mining.tempNormal", "温度正常")
          }
          accent={summary.hotCount ? "#fb7185" : "#fbbf24"}
        />
      </div>

      <div className="px-4 py-4">
        <div className="mb-2.5 flex items-center justify-between">
          <div className="flex items-center gap-2 text-[13px] font-semibold">
            <span className="text-cyan-400">▤</span>
            {t("mining.gpuStatus", "GPU 状态")}
          </div>
          <span className="font-mono text-[10px] text-[var(--gray-9)]">
            {t("mining.gpuHint", "利用率 · 显存 · 温度 · 功耗")}
          </span>
        </div>
        {gpus.length === 0 ? (
          <div className="rounded-lg border border-dashed border-[var(--gray-a5)] px-4 py-8 text-center text-[12px] text-[var(--gray-9)]">
            {t("mining.noGpu", "未读取到 GPU 数据（nvidia-smi / rocm-smi 不可用）")}
          </div>
        ) : (
          <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-4">
            {gpus.map((gpu, index) => {
              const gpuIndex = gpu.index ?? index;
              const util = Math.max(0, Math.min(100, Number(gpu.utilization ?? 0)));
              const memPct = gpu.memory_total
                ? Math.max(0, Math.min(100, (gpu.memory_used / gpu.memory_total) * 100))
                : 0;
              const temp = Math.round(Number(gpu.temperature ?? 0));
              const memTemp = Math.round(Number(gpu.memory_temperature ?? 0));
              const assigned = minersForGpu(gpuIndex);
              return (
                <div
                  key={`${gpuIndex}-${index}`}
                  className={`rounded-xl border bg-[var(--gray-a2)] p-3 ${
                    temp >= 80 ? "border-red-500/45" : "border-[var(--gray-a4)]"
                  }`}
                >
                  <div className="mb-2 flex items-center gap-2">
                    <span className="rounded border border-cyan-500/25 bg-cyan-500/10 px-1.5 py-0.5 font-mono text-[10px] text-cyan-300">
                      #{gpuIndex}
                    </span>
                    <span className="truncate text-[12px] font-medium" title={gpu.name}>
                      {gpu.name}
                    </span>
                    <span
                      className={`ml-auto flex-none rounded border px-1.5 py-0.5 font-mono text-[10px] font-semibold ${miningTempClass(temp)}`}
                    >
                      {temp}°C
                    </span>
                  </div>
                  <div className="flex items-baseline justify-between text-[10.5px] text-[var(--gray-10)]">
                    <span>{t("mining.utilization", "GPU 使用率")}</span>
                    <b className="font-mono text-[11px] text-[var(--gray-12)]">
                      {util.toFixed(1)}%
                    </b>
                  </div>
                  <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-[var(--gray-a4)]">
                    <div
                      className="h-full rounded-full bg-gradient-to-r from-cyan-600 to-cyan-300"
                      style={{ width: `${util}%` }}
                    />
                  </div>
                  <div className="mt-2 flex items-baseline justify-between text-[10.5px] text-[var(--gray-10)]">
                    <span>{t("mining.vram", "显存")}</span>
                    <b className="font-mono text-[10.5px] text-[var(--gray-12)]">
                      {formatBytes(gpu.memory_used)} / {formatBytes(gpu.memory_total)}
                    </b>
                  </div>
                  <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-[var(--gray-a4)]">
                    <div
                      className="h-full rounded-full bg-gradient-to-r from-violet-600 to-violet-300"
                      style={{ width: `${memPct}%` }}
                    />
                  </div>
                  <div className="mt-2.5 grid grid-cols-4 gap-1 border-t border-dashed border-[var(--gray-a4)] pt-2 text-center">
                    {[
                      [t("mining.powerShort", "功耗"), `${Math.round(Number(gpu.power ?? 0))} W`],
                      [t("mining.fan", "风扇"), gpu.fan_speed ? `${Math.round(gpu.fan_speed)}%` : "—"],
                      [t("mining.clock", "频率"), gpu.clock_mhz ? `${(gpu.clock_mhz / 1000).toFixed(2)} GHz` : "—"],
                      [t("mining.memTemp", "显存温"), memTemp ? `${memTemp}°C` : "—"],
                    ].map(([label, value]) => (
                      <div key={label} className="min-w-0">
                        <div className="truncate text-[9px] text-[var(--gray-9)]">{label}</div>
                        <div className="truncate font-mono text-[10.5px] font-medium">{value}</div>
                      </div>
                    ))}
                  </div>
                  <div className="mt-2.5 flex flex-wrap gap-1">
                    {assigned.length > 0 ? (
                      assigned.map((miner) => <GpuChip key={miner.pid} miner={miner} />)
                    ) : (
                      <span className="rounded-full border border-[var(--gray-a4)] px-1.5 py-0.5 font-mono text-[9.5px] text-[var(--gray-9)]">
                        {t("mining.idle", "空闲")}
                      </span>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <div className="border-t border-[var(--gray-a4)] px-4 py-4">
        <div className="mb-2.5 flex items-center justify-between">
          <div className="flex items-center gap-2 text-[13px] font-semibold">
            <span className="text-cyan-400">⚒</span>
            {t("mining.minerProcesses", "挖矿进程")}
          </div>
          <span className="font-mono text-[10px] text-[var(--gray-9)]">
            {t("mining.minerHint", "算力 / 份额来自本地 miner API")}
          </span>
        </div>
        {miners.length === 0 ? (
          <div className="rounded-lg border border-dashed border-[var(--gray-a5)] px-4 py-8 text-center text-[12px] text-[var(--gray-9)]">
            {t("mining.noMiner", "未检测到挖矿进程")}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[880px] border-collapse text-[12px]">
              <thead>
                <tr className="border-b border-[var(--gray-a4)] text-left text-[9.5px] uppercase tracking-wider text-[var(--gray-9)]">
                  <th className="px-2 py-2 font-semibold">{t("mining.process", "进程")}</th>
                  <th className="px-2 py-2 font-semibold">{t("mining.hashAlgo", "算力 / 算法")}</th>
                  <th className="px-2 py-2 font-semibold">{t("mining.poolWallet", "矿池 / 钱包")}</th>
                  <th className="px-2 py-2 font-semibold">GPU</th>
                  <th className="px-2 py-2 font-semibold">{t("mining.shares", "份额")}</th>
                  <th className="px-2 py-2 font-semibold">{t("mining.uptime", "运行时间")}</th>
                  <th className="px-2 py-2 font-semibold">CPU / {t("mining.memory", "内存")}</th>
                </tr>
              </thead>
              <tbody>
                {miners.map((miner) => (
                  <tr
                    key={`${miner.pid}-${miner.name}`}
                    className="border-b border-[var(--gray-a3)] last:border-0 hover:bg-cyan-500/[0.035]"
                  >
                    <td className="px-2 py-2.5">
                      <div className="flex items-center gap-1.5 whitespace-nowrap text-[12.5px] font-semibold">
                        <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 shadow-[0_0_7px_rgba(52,211,153,0.7)]" />
                        {miner.name}
                        <span className="font-mono text-[9.5px] font-normal text-[var(--gray-9)]">
                          PID {miner.pid}
                        </span>
                      </div>
                      <div
                        className="mt-0.5 max-w-[320px] truncate font-mono text-[9.5px] text-[var(--gray-9)]"
                        title={miner.command || miner.exe}
                      >
                        {miner.command || miner.exe}
                      </div>
                    </td>
                    <td className="px-2 py-2.5 whitespace-nowrap">
                      <div className="font-mono text-[12.5px] font-semibold text-cyan-400">
                        {miner.hashrate_text || formatHashrate(miner.hashrate)}
                      </div>
                      <div className="mt-0.5 font-mono text-[9.5px] text-[var(--gray-9)]">
                        {miner.algorithm || "unknown"}
                      </div>
                    </td>
                    <td className="px-2 py-2.5">
                      <div
                        className="max-w-[280px] truncate font-mono text-[10.5px]"
                        title={miner.pool}
                      >
                        {miner.pool || "—"}
                      </div>
                      <div
                        className="mt-0.5 max-w-[280px] truncate font-mono text-[9.5px] text-[var(--gray-9)]"
                        title={miner.wallet}
                      >
                        {shortWallet(miner.wallet)}
                      </div>
                    </td>
                    <td className="px-2 py-2.5">
                      <div className="flex max-w-[130px] flex-wrap gap-1">
                        {(miner.gpus ?? []).length > 0 ? (
                          (miner.gpus ?? []).map((index) => (
                            <span
                              key={index}
                              className="rounded-full border border-emerald-500/25 bg-emerald-500/10 px-1.5 py-0.5 font-mono text-[9px] text-emerald-300"
                            >
                              GPU {index}
                            </span>
                          ))
                        ) : (
                          <span className="rounded-full border border-[var(--gray-a4)] px-1.5 py-0.5 font-mono text-[9px] text-[var(--gray-9)]">
                            CPU
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-2 py-2.5 whitespace-nowrap font-mono text-[11px]">
                      <div className="text-emerald-400">✓ {miner.accepted_shares ?? 0}</div>
                      <div className="text-red-400">✗ {miner.rejected_shares ?? 0}</div>
                    </td>
                    <td className="px-2 py-2.5 whitespace-nowrap font-mono text-[10.5px] text-[var(--gray-10)]">
                      {formatMiningDuration(miner.uptime)}
                    </td>
                    <td className="px-2 py-2.5 whitespace-nowrap font-mono text-[10.5px]">
                      <div>{Number(miner.cpu_percent ?? 0).toFixed(1)}%</div>
                      <div className="text-[9.5px] text-[var(--gray-9)]">
                        {formatBytes(miner.memory_bytes ?? 0)}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Card>
  );
}
