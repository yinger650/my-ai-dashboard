import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useState } from "react";
import { apiGet } from "../api";
import type { Board, BoardMachine } from "../types";
import { HealthBadge, SevDot } from "../components/Severity";
import { Sparkline } from "../components/Sparkline";
import { fmtBps, fmtPct, relativeTime, usagePct } from "../format";

export function DashboardPage() {
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [search, setSearch] = useState("");
  const { data, isLoading, error, dataUpdatedAt } = useQuery({
    queryKey: ["board"],
    queryFn: () => apiGet<Board>("/api/v1/board"),
    refetchInterval: autoRefresh ? 15000 : false,
  });

  const machines = (data?.machines ?? []).filter((m) =>
    search ? m.name.toLowerCase().includes(search.toLowerCase()) || m.machine_key.includes(search) : true,
  );

  const counts = { online: 0, degraded: 0, offline: 0 };
  for (const m of data?.machines ?? []) {
    if (m.health === "online") counts.online++;
    else if (m.health === "degraded" || m.health === "stale") counts.degraded++;
    else if (m.health === "offline") counts.offline++;
  }

  return (
    <div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">{data?.title ?? "AgentBoard Personal"}</h1>
          <p className="text-sm text-slate-400">
            <span className="sev-normal">在线 {counts.online}</span> ·{" "}
            <span className="sev-warning">降级 {counts.degraded}</span> ·{" "}
            <span className="sev-offline">离线 {counts.offline}</span>
            {data && <> · 异常访问 {data.recent_abnormal}</>}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="搜索机器…"
            className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-sm outline-none focus:border-indigo-500"
          />
          <label className="flex items-center gap-2 text-sm text-slate-400">
            <input type="checkbox" checked={autoRefresh} onChange={(e) => setAutoRefresh(e.target.checked)} />
            自动刷新
          </label>
          <span className="text-xs text-slate-500">
            {dataUpdatedAt ? `更新于 ${relativeTime(new Date(dataUpdatedAt).toISOString())}` : ""}
          </span>
        </div>
      </div>

      {error && (
        <div className="mb-4 rounded-md bg-amber-500/10 px-3 py-2 text-sm sev-warning">
          数据可能已过期：{(error as Error).message}
        </div>
      )}

      {isLoading && <div className="text-slate-400">加载中…</div>}

      {!isLoading && machines.length === 0 && (
        <div className="card p-8 text-center text-slate-400">
          还没有机器。前往 <Link className="text-indigo-400 underline" to="/settings">设置</Link> 创建一台并生成 Token。
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {machines.map((m) => (
          <MachineCard key={m.id} m={m} />
        ))}
      </div>
    </div>
  );
}

function MachineCard({ m }: { m: BoardMachine }) {
  const offline = m.health === "offline";
  const lm = m.latest_metric;
  const mem = usagePct(lm?.memory_used_bytes ?? null, lm?.memory_total_bytes ?? null);
  const disk = usagePct(lm?.root_disk_used_bytes ?? null, lm?.root_disk_total_bytes ?? null);
  const c = m.service_counts;
  const recent = m.recent_logs?.[0];

  return (
    <Link
      to={`/machines/${m.id}`}
      className={`card block p-4 transition hover:border-indigo-500 ${offline ? "opacity-60" : ""}`}
    >
      <div className="mb-3 flex items-start justify-between">
        <div>
          <div className="font-medium">{m.name}</div>
          <div className="text-xs text-slate-500">{m.kind} · {m.machine_key}</div>
        </div>
        <HealthBadge health={m.health} />
      </div>

      <div className="mb-3 grid grid-cols-3 gap-2 text-center text-sm">
        <Metric label="CPU" value={fmtPct(lm?.cpu_percent ?? null)} />
        <Metric label="内存" value={fmtPct(mem)} />
        <Metric label="磁盘" value={fmtPct(disk)} />
      </div>

      <div className="mb-2">
        <Sparkline data={m.sparkline ?? []} />
      </div>

      <div className="mb-2 text-xs text-slate-400">
        ↓ {fmtBps(lm?.network_rx_bps ?? null)} · ↑ {fmtBps(lm?.network_tx_bps ?? null)}
      </div>

      <div className="flex items-center gap-3 text-xs">
        <span className="flex items-center gap-1"><SevDot severity="normal" /> {c.normal}</span>
        <span className="flex items-center gap-1"><SevDot severity="warning" /> {c.warning}</span>
        <span className="flex items-center gap-1"><SevDot severity="error" /> {c.error}</span>
        <span className="ml-auto text-slate-500">{offline ? "最后数据 · " : ""}{relativeTime(m.last_seen_at)}</span>
      </div>

      {recent && (
        <div className={`mt-2 truncate text-xs sev-${recent.severity}`} title={recent.markdown}>
          {recent.severity.toUpperCase()}: {recent.markdown}
        </div>
      )}
    </Link>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-slate-900/60 py-2">
      <div className="text-xs text-slate-500">{label}</div>
      <div className="font-semibold">{value}</div>
    </div>
  );
}
