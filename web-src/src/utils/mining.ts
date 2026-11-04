/** Format a normalized hash rate (H/s) using SI units. */
export function formatHashrate(value: number | null | undefined): string {
  const v0 = Number(value ?? 0);
  if (!Number.isFinite(v0) || v0 <= 0) return "0 H/s";
  const units = ["H/s", "kH/s", "MH/s", "GH/s", "TH/s", "PH/s"];
  let v = v0;
  let i = 0;
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000;
    i += 1;
  }
  const digits = v >= 100 ? 0 : v >= 10 ? 1 : 2;
  return `${v.toFixed(digits)} ${units[i]}`;
}

export function formatMiningDuration(seconds: number | null | undefined): string {
  const total = Math.max(0, Math.floor(Number(seconds ?? 0)));
  if (!total) return "-";
  const d = Math.floor(total / 86400);
  const h = Math.floor((total % 86400) / 3600);
  const m = Math.floor((total % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${total % 60}s`;
  return `${total}s`;
}

export function shortWallet(wallet: string | null | undefined): string {
  const w = String(wallet ?? "");
  if (!w) return "—";
  if (w.length <= 20) return w;
  return `${w.slice(0, 10)}…${w.slice(-6)}`;
}

export function miningTempClass(temp: number): string {
  if (temp >= 80) return "text-red-400 border-red-500/40 bg-red-500/10";
  if (temp >= 72) return "text-amber-400 border-amber-500/40 bg-amber-500/10";
  return "text-emerald-400 border-emerald-500/30 bg-emerald-500/10";
}
