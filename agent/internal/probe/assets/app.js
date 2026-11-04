(() => {
  "use strict";

  const $ = (selector) => document.querySelector(selector);
  const POLL_MS = 2000;

  let state = null;
  let lastSnapshotAt = 0;
  let connected = false;
  let probeUptimeBase = 0;
  let probeUptimeAt = 0;

  const esc = (value) =>
    String(value ?? "").replace(/[&<>"']/g, (ch) => ({
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    }[ch]));

  const num = (value, fallback = 0) => {
    const n = Number(value);
    return Number.isFinite(n) ? n : fallback;
  };

  const clamp = (value, min, max) => Math.min(max, Math.max(min, value));

  function fmtHashrate(value) {
    const v0 = num(value);
    if (v0 <= 0) return "0 H/s";
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

  function fmtBytes(value) {
    const v0 = num(value);
    if (v0 <= 0) return "0 B";
    const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
    let v = v0;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i += 1;
    }
    const digits = v >= 100 ? 0 : v >= 10 ? 1 : 2;
    return `${v.toFixed(digits)} ${units[i]}`;
  }

  function fmtDuration(seconds) {
    let s = Math.max(0, Math.floor(num(seconds)));
    if (s <= 0) return "-";
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (d > 0) return `${d}d ${h}h`;
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${s % 60}s`;
    return `${s}s`;
  }

  function fmtClock(mhz) {
    const v = num(mhz);
    if (!v) return "—";
    return `${(v / 1000).toFixed(2)} GHz`;
  }

  function shortWallet(wallet) {
    const w = String(wallet || "");
    if (!w) return "—";
    if (w.length <= 20) return w;
    return `${w.slice(0, 10)}…${w.slice(-6)}`;
  }

  function pct(part, total) {
    if (!total) return 0;
    return clamp((num(part) / num(total)) * 100, 0, 100);
  }

  function tempClass(t) {
    if (t >= 80) return "hot";
    if (t >= 72) return "warm";
    return "";
  }

  function setConnected(ok) {
    connected = ok;
    const dot = $("#link-dot");
    const text = $("#link-text");
    dot.className = `dot ${ok ? "online" : "offline"}`;
    text.textContent = ok ? "已连接" : "连接中断";
    $("#footer-hint").textContent = ok ? "自动刷新 2s" : "正在重试连接…";
  }

  function renderSummary(s) {
    const gpus = s.gpus || [];
    const miners = s.miners || [];
    const avgUtil = num(s.avg_gpu_usage);
    const power = num(s.total_power);
    const maxTemp = num(s.max_gpu_temp);
    const hotCount = gpus.filter((g) => Math.max(num(g.temperature), num(g.memory_temperature)) >= 80).length;
    const totalHash = num(s.total_hashrate);
    const minerNames = miners.map((m) => m.name).join(" · ") || "无运行中的 miner";

    const cards = [
      {
        cls: "cyan",
        label: "总算力",
        value: fmtHashrate(totalHash),
        foot: `${miners.length} 个挖矿进程`,
      },
      {
        cls: "green",
        label: "挖矿进程",
        value: `${miners.length}`,
        unit: "active",
        foot: minerNames,
      },
      {
        cls: "violet",
        label: "GPU 数量",
        value: `${gpus.length}`,
        unit: "cards",
        foot: `平均利用率 ${avgUtil.toFixed(1)}%`,
      },
      {
        cls: "amber",
        label: "平均利用率",
        value: `${avgUtil.toFixed(1)}%`,
        foot: `最高温度 ${maxTemp}°C`,
      },
      {
        cls: "red",
        label: "总功耗",
        value: `${Math.round(power)}`,
        unit: "W",
        foot: gpus.length ? `${Math.round(power / gpus.length)} W / GPU` : "—",
      },
      {
        cls: hotCount ? "red" : "amber",
        label: "最高温度",
        value: `${maxTemp}`,
        unit: "°C",
        foot: hotCount ? `${hotCount} 张 GPU ≥ 80°C` : "温度正常",
      },
    ];

    $("#summary").innerHTML = cards.map((card) => `
      <article class="stat ${card.cls}">
        <div class="stat-label">${esc(card.label)}</div>
        <div class="stat-value">${esc(card.value)}${card.unit ? `<small>${esc(card.unit)}</small>` : ""}</div>
        <div class="stat-foot" title="${esc(card.foot)}">${esc(card.foot)}</div>
      </article>
    `).join("");
  }

  function renderAlerts(s) {
    const items = [];
    (s.alerts || []).forEach((a) => items.push({ text: a, cls: "" }));
    (s.warnings || []).forEach((w) => items.push({ text: w, cls: "warn" }));
    $("#alerts").innerHTML = items.map((item) => `
      <div class="alert ${item.cls}">
        <span class="a-icon">${item.cls ? "⚠" : "◆"}</span>
        <span>${esc(item.text)}</span>
      </div>
    `).join("");
  }

  function renderGPUs(s) {
    const gpus = s.gpus || [];
    const miners = s.miners || [];
    $("#gpu-count").textContent = `${gpus.length} GPU${gpus.length === 1 ? "" : "s"}`;
    const grid = $("#gpu-grid");
    if (!gpus.length) {
      grid.innerHTML = `
        <div class="empty" style="grid-column: 1 / -1">
          <div class="empty-icon">▤</div>
          <div>未读取到 GPU 数据</div>
          <p>请确认 nvidia-smi 或 rocm-smi 已安装并在 PATH 中</p>
        </div>`;
      return;
    }
    grid.innerHTML = gpus.map((g) => {
      const util = clamp(num(g.utilization), 0, 100);
      const memPct = pct(g.memory_used, g.memory_total);
      const temp = Math.round(num(g.temperature));
      const memoryTemp = Math.round(num(g.memory_temperature));
      const tcls = tempClass(temp);
      const assigned = miners.filter((m) => (m.gpus || []).includes(g.index));
      const chips = assigned.map((m) => {
        const hashrate = m.hashrate_text || fmtHashrate(m.hashrate);
        return `<span class="chip" title="${esc(m.name)} · ${esc(hashrate)}">⚒ ${esc(m.name)} <b>${esc(hashrate)}</b></span>`;
      }).join("");
      const otherProcs = (g.processes || [])
        .filter((p) => !assigned.some((m) => m.pid === p.pid))
        .map((p) => `<span class="chip muted" title="PID ${p.pid} ${esc(p.name)}">PID ${p.pid}</span>`)
        .join("");
      const memBarClass = memPct >= 94 ? "danger" : memPct >= 85 ? "warn" : "";
      const utilBarClass = util >= 80 ? "" : util >= 50 ? "warn" : "danger";
      return `
        <article class="gpu ${tcls}">
          <div class="gpu-head">
            <span class="idx">#${num(g.index)}</span>
            <span class="gpu-name" title="${esc(g.name)}">${esc(g.name)}</span>
            <span class="temp ${tcls}">${temp}°C</span>
          </div>
          <div class="row"><span>GPU 使用率</span><b>${util.toFixed(1)}%</b></div>
          <div class="bar ${utilBarClass}"><i style="width:${util}%"></i></div>
          <div class="row"><span>显存</span><b>${fmtBytes(g.memory_used)} / ${fmtBytes(g.memory_total)}</b></div>
          <div class="bar mem ${memBarClass}"><i style="width:${memPct}%"></i></div>
          <div class="gpu-meta">
            <div><span>功耗</span><b>${Math.round(num(g.power))} W</b></div>
            <div><span>风扇</span><b>${num(g.fan_speed) ? `${Math.round(num(g.fan_speed))}%` : "—"}</b></div>
            <div><span>核心频率</span><b>${fmtClock(g.clock_mhz)}</b></div>
            <div><span>显存温度</span><b>${memoryTemp ? `${memoryTemp}°C` : "—"}</b></div>
          </div>
          <div class="gpu-miners">
            ${chips || `<span class="chip muted">空闲</span>`}
            ${otherProcs}
          </div>
        </article>`;
    }).join("");
  }

  function renderMiners(s) {
    const miners = s.miners || [];
    const table = document.querySelector(".miner-table");
    const body = $("#miner-body");
    const empty = $("#miner-empty");
    $("#miner-count").textContent = `${miners.length} miner${miners.length === 1 ? "" : "s"}`;
    if (!miners.length) {
      table.classList.add("hidden");
      empty.classList.remove("hidden");
      body.innerHTML = "";
      return;
    }
    table.classList.remove("hidden");
    empty.classList.add("hidden");
    body.innerHTML = miners.map((m) => {
      const gpuChips = (m.gpus || []).length
        ? (m.gpus || []).map((idx) => `<span class="chip green">GPU ${idx}</span>`).join("")
        : `<span class="chip muted">CPU</span>`;
      const pool = m.pool || "—";
      const wallet = m.wallet || "";
      const cmd = m.command || m.exe || "";
      return `
        <tr>
          <td>
            <div class="mname"><span class="statusdot"></span>${esc(m.name)}<span class="pid">PID ${num(m.pid)}</span></div>
            <div class="cmd" title="${esc(cmd)}">${esc(cmd)}</div>
          </td>
          <td class="hash-cell">
            <b>${esc(m.hashrate_text || fmtHashrate(m.hashrate))}</b>
            <span>${esc(m.algorithm || "unknown")}</span>
          </td>
          <td>
            <div class="pool" title="${esc(pool)}">${esc(pool)}</div>
            <div class="wallet" title="${esc(wallet)}">${esc(shortWallet(wallet))}</div>
          </td>
          <td><div class="gpu-chips">${gpuChips}</div></td>
          <td class="shares">
            <span class="ok">✓ ${num(m.accepted_shares)}</span>
            <span class="bad">✗ ${num(m.rejected_shares)}</span>
          </td>
          <td><span class="sub">${fmtDuration(m.uptime)}</span></td>
          <td class="resources">
            <span>${num(m.cpu_percent).toFixed(1)}%</span>
            <span class="sub">${fmtBytes(m.memory_bytes)}</span>
          </td>
        </tr>`;
    }).join("");
  }

  function renderSystem(s) {
    const sys = s.system || {};
    const cpuPct = clamp(num(sys.cpu_usage), 0, 100);
    const ramPct = pct(sys.ram_used, sys.ram_total);
    const diskPct = pct(sys.disk_used, sys.disk_total);
    const collect = num(s.collect_ms);
    $("#collect-hint").textContent = collect ? `本次采集 ${collect} ms` : "--";

    const items = [
      {
        label: "CPU",
        value: `${cpuPct.toFixed(1)}%`,
        pct: cpuPct,
        foot: `${sys.cpu_name || "Unknown CPU"} · ${num(sys.cpu_cores)} threads`,
        wide: true,
      },
      {
        label: "内存",
        value: `${ramPct.toFixed(1)}%`,
        pct: ramPct,
        foot: `${fmtBytes(sys.ram_used)} / ${fmtBytes(sys.ram_total)}`,
      },
      {
        label: "磁盘",
        value: `${diskPct.toFixed(1)}%`,
        pct: diskPct,
        foot: `${fmtBytes(sys.disk_used)} / ${fmtBytes(sys.disk_total)}`,
      },
      {
        label: "负载",
        value: `${num(sys.load1).toFixed(2)}`,
        pct: clamp((num(sys.load1) / Math.max(1, num(sys.cpu_cores))) * 100, 0, 100),
        foot: `5m ${num(sys.load5).toFixed(2)} · 15m ${num(sys.load15).toFixed(2)}`,
      },
      {
        label: "网络",
        value: `↑ ${fmtBytes(sys.net_up)}/s`,
        foot: `↓ ${fmtBytes(sys.net_down)}/s · 累计 ↑ ${fmtBytes(sys.net_total_up)}`,
      },
      {
        label: "运行时间",
        value: fmtDuration(sys.uptime),
        foot: `${num(sys.processes)} 进程 · TCP ${num(sys.tcp)} / UDP ${num(sys.udp)}`,
      },
      {
        label: "系统",
        value: `${sys.os || "—"} / ${sys.arch || "—"}`,
        foot: `${sys.hostname || "—"} · kernel ${sys.kernel || "—"}`,
        wide: true,
      },
    ];

    $("#system-grid").innerHTML = items.map((item) => `
      <div class="sys-item ${item.wide ? "sys-wide" : ""}">
        <div class="sys-label"><span>${esc(item.label)}</span><b>${esc(item.value)}</b></div>
        ${item.pct === undefined ? "" : `<div class="bar" style="margin-top:9px"><i style="width:${clamp(item.pct, 0, 100)}%"></i></div>`}
        <div class="sys-foot" title="${esc(item.foot)}">${esc(item.foot)}</div>
      </div>
    `).join("");
  }

  function drawChart(history) {
    const canvas = $("#hashrate-chart");
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(rect.width * dpr);
    canvas.height = Math.round(rect.height * dpr);
    const ctx = canvas.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, rect.width, rect.height);

    const points = (history || []).filter((p) => p && p.time);
    if (points.length < 2) {
      ctx.fillStyle = "rgba(132, 147, 173, 0.65)";
      ctx.font = "12px ui-monospace, SFMono-Regular, Menlo, monospace";
      ctx.textAlign = "center";
      ctx.fillText("正在积累采样数据…", rect.width / 2, rect.height / 2);
      return;
    }

    const padL = 62;
    const padR = 14;
    const padT = 14;
    const padB = 24;
    const w = rect.width - padL - padR;
    const h = rect.height - padT - padB;
    const maxValue = Math.max(1, ...points.map((p) => num(p.total_hashrate) * 1.12));
    const x = (i) => padL + (i / (points.length - 1)) * w;
    const y = (v) => padT + h - (num(v) / maxValue) * h;

    ctx.strokeStyle = "rgba(34, 48, 80, 0.6)";
    ctx.fillStyle = "rgba(132, 147, 173, 0.7)";
    ctx.font = "10px ui-monospace, SFMono-Regular, Menlo, monospace";
    ctx.lineWidth = 1;
    ctx.textAlign = "right";
    for (let i = 0; i <= 4; i += 1) {
      const yy = padT + (h / 4) * i;
      ctx.beginPath();
      ctx.moveTo(padL, yy + 0.5);
      ctx.lineTo(padL + w, yy + 0.5);
      ctx.stroke();
      const label = fmtHashrate(maxValue * (1 - i / 4));
      ctx.fillText(label, padL - 10, yy + 3);
    }

    const gradient = ctx.createLinearGradient(0, padT, 0, padT + h);
    gradient.addColorStop(0, "rgba(34, 211, 238, 0.32)");
    gradient.addColorStop(1, "rgba(34, 211, 238, 0.01)");

    ctx.beginPath();
    points.forEach((p, i) => {
      const px = x(i);
      const py = y(p.total_hashrate);
      if (i === 0) ctx.moveTo(px, py);
      else ctx.lineTo(px, py);
    });
    ctx.lineTo(x(points.length - 1), padT + h);
    ctx.lineTo(x(0), padT + h);
    ctx.closePath();
    ctx.fillStyle = gradient;
    ctx.fill();

    ctx.beginPath();
    points.forEach((p, i) => {
      const px = x(i);
      const py = y(p.total_hashrate);
      if (i === 0) ctx.moveTo(px, py);
      else ctx.lineTo(px, py);
    });
    ctx.strokeStyle = "#22d3ee";
    ctx.lineWidth = 2;
    ctx.shadowColor = "rgba(34, 211, 238, 0.5)";
    ctx.shadowBlur = 10;
    ctx.stroke();
    ctx.shadowBlur = 0;

    const last = points[points.length - 1];
    ctx.beginPath();
    ctx.arc(x(points.length - 1), y(last.total_hashrate), 4, 0, Math.PI * 2);
    ctx.fillStyle = "#22d3ee";
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x(points.length - 1), y(last.total_hashrate), 8, 0, Math.PI * 2);
    ctx.strokeStyle = "rgba(34, 211, 238, 0.35)";
    ctx.lineWidth = 2;
    ctx.stroke();

    ctx.fillStyle = "rgba(132, 147, 173, 0.7)";
    ctx.textAlign = "left";
    const firstTime = new Date(points[0].time);
    ctx.fillText(firstTime.toLocaleTimeString("zh-CN", { hour12: false }), padL, rect.height - 5);
    ctx.textAlign = "right";
    ctx.fillText(new Date(last.time).toLocaleTimeString("zh-CN", { hour12: false }), padL + w, rect.height - 5);
    ctx.textAlign = "center";
    ctx.fillStyle = "rgba(34, 211, 238, 0.9)";
    ctx.font = "11px ui-monospace, SFMono-Regular, Menlo, monospace";
    ctx.fillText(fmtHashrate(last.total_hashrate), padL + w / 2, padT + 12);
  }

  function render(data) {
    const snapshot = data.snapshot;
    $("#version").textContent = `v${data.version || "0.1.0"}`;
    $("#chart-total").textContent = snapshot ? fmtHashrate(snapshot.total_hashrate) : "0 H/s";
    if (!snapshot) {
      $("#rig-name").textContent = "等待探针采集…";
      $("#gpu-hint").textContent = "正在读取 nvidia-smi / rocm-smi";
      return;
    }
    const s = snapshot;
    const sys = s.system || {};
    $("#rig-name").textContent = s.name || s.hostname || "unknown rig";
    $("#hostname").textContent = s.hostname || "—";
    $("#os-name").textContent = `${sys.os || "—"} / ${sys.arch || "—"}`;
    $("#demo-badge").classList.toggle("hidden", !s.demo);
    $("#gpu-hint").textContent = s.gpus && s.gpus.length
      ? "利用率 · 显存 · 温度 · 功耗"
      : "未发现可用 GPU 工具";
    $("#miner-hint").textContent = s.miners && s.miners.length
      ? "算力 / 份额来自本地 miner API"
      : "未匹配到常见 miner 进程";

    lastSnapshotAt = Date.now();
    probeUptimeBase = num(data.probe_uptime);
    probeUptimeAt = Date.now();
    renderAlerts(s);
    renderSummary(s);
    renderGPUs(s);
    renderMiners(s);
    renderSystem(s);
    drawChart(data.history || []);
  }

  async function poll() {
    try {
      const response = await fetch("/api/status", { cache: "no-store" });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      state = data;
      render(data);
      setConnected(true);
    } catch (error) {
      setConnected(false);
      console.warn("status poll failed", error);
    } finally {
      window.setTimeout(poll, POLL_MS);
    }
  }

  function tick() {
    if (lastSnapshotAt) {
      const seconds = Math.max(0, Math.floor((Date.now() - lastSnapshotAt) / 1000));
      $("#last-update").textContent = seconds <= 1 ? "刚刚更新" : `${seconds} 秒前更新`;
    }
    if (probeUptimeAt) {
      const elapsed = Math.floor((Date.now() - probeUptimeAt) / 1000);
      $("#probe-uptime").textContent = `probe up ${fmtDuration(probeUptimeBase + elapsed)}`;
    }
    if (state) drawChart(state.history || []);
  }

  window.addEventListener("resize", () => {
    if (state) drawChart(state.history || []);
  });

  setInterval(tick, 1000);
  poll();
})();
