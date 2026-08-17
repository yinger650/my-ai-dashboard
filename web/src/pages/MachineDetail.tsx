import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { useState } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { apiGet } from "../api";
import type { Machine, MetricSample, Service } from "../types";
import { HealthBadge, SevDot } from "../components/Severity";
import { fmtBps, fmtBytes, fmtPct, localTime, relativeTime, usagePct } from "../format";

interface MachineDetail {
  machine: Machine;
  latest_metric: MetricSample | null;
  health: string;
  resource_severity: string;
}

const RANGES = ["1h", "6h", "24h", "7d", "30d"];

export function MachineDetailPage() {
  const { machineId } = useParams();
  const [range, setRange] = useState("1h");

  const detail = useQuery({
    queryKey: ["machine", machineId],
    queryFn: () => apiGet<MachineDetail>(`/api/v1/machines/${machineId}`),
    refetchInterval: 15000,
  });
  const metrics = useQuery({
    queryKey: ["machine-metrics", machineId, range],
    queryFn: () => apiGet<{ range: string; samples: MetricSample[] }>(`/api/v1/machines/${machineId}/metrics?range=${range}`),
    refetchInterval: 15000,
  });
  const services = useQuery({
    queryKey: ["machine-services", machineId],
    queryFn: () => apiGet<Service[]>(`/api/v1/machines/${machineId}/services`),
    refetchInterval: 15000,
  });

  if (detail.isLoading) return <div className="text-slate-400">加载中…</div>;
  if (detail.error) return <div className="sev-error">加载失败：{(detail.error as Error).message}</div>;

  const m = detail.data!.machine;
  const lm = detail.data!.latest_metric;
  const chart = (metrics.data?.samples ?? []).map((s) => ({
    t: new Date(s.occurred_at).toLocaleTimeString(),
    cpu: s.cpu_percent,
    mem: usagePct(s.memory_used_bytes, s.memory_total_bytes),
    net: (s.network_rx_bps ?? 0) + (s.network_tx_bps ?? 0),
  }));

  return (
    <div>
      <Link to="/" className="mb-3 inline-block text-sm text-indigo-400">← 返回看板</Link>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold">{m.name}</h1>
            <HealthBadge health={detail.data!.health} />
          </div>
          <p className="text-sm text-slate-400">
            {m.hostname ?? "-"} · {m.os ?? "-"}/{m.arch ?? "-"} · Collector {m.collector_version ?? "-"} · 最后上报 {relativeTime(m.last_seen_at)}
          </p>
        </div>
      </div>

      <div className="mb-4 grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Stat label="CPU" value={fmtPct(lm?.cpu_percent ?? null)} />
        <Stat label="内存" value={fmtPct(usagePct(lm?.memory_used_bytes ?? null, lm?.memory_total_bytes ?? null))} sub={`${fmtBytes(lm?.memory_used_bytes ?? null)} / ${fmtBytes(lm?.memory_total_bytes ?? null)}`} />
        <Stat label="根磁盘" value={fmtPct(usagePct(lm?.root_disk_used_bytes ?? null, lm?.root_disk_total_bytes ?? null))} sub={`${fmtBytes(lm?.root_disk_used_bytes ?? null)} / ${fmtBytes(lm?.root_disk_total_bytes ?? null)}`} />
        <Stat label="网络" value={`↓${fmtBps(lm?.network_rx_bps ?? null)}`} sub={`↑${fmtBps(lm?.network_tx_bps ?? null)}`} />
      </div>

      <div className="card mb-6 p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-medium">指标趋势</h2>
          <div className="flex gap-1">
            {RANGES.map((rr) => (
              <button
                key={rr}
                onClick={() => setRange(rr)}
                className={`rounded px-2 py-1 text-xs ${range === rr ? "bg-indigo-600 text-white" : "bg-slate-800 text-slate-400"}`}
              >
                {rr}
              </button>
            ))}
          </div>
        </div>
        {chart.length === 0 ? (
          <div className="flex h-52 items-center justify-center text-slate-500">该区间暂无数据</div>
        ) : (
          <ResponsiveContainer width="100%" height={240}>
            <AreaChart data={chart}>
              <defs>
                <linearGradient id="cpuG" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#818cf8" stopOpacity={0.6} />
                  <stop offset="95%" stopColor="#818cf8" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="#1f2a44" />
              <XAxis dataKey="t" stroke="#64748b" fontSize={11} minTickGap={40} />
              <YAxis stroke="#64748b" fontSize={11} domain={[0, 100]} width={36} />
              <Tooltip contentStyle={{ background: "#0f1626", border: "1px solid #1f2a44", borderRadius: 8 }} />
              <Area type="monotone" dataKey="cpu" name="CPU %" stroke="#818cf8" fill="url(#cpuG)" strokeWidth={2} isAnimationActive={false} />
              <Area type="monotone" dataKey="mem" name="内存 %" stroke="#34d399" fillOpacity={0} strokeWidth={2} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </div>

      <div className="card p-4">
        <h2 className="mb-3 font-medium">服务 ({services.data?.length ?? 0})</h2>
        <div className="divide-y divide-slate-800">
          {(services.data ?? []).map((s) => (
            <Link key={s.id} to={`/services/${s.id}`} className="flex items-center gap-3 py-2 hover:bg-slate-800/40">
              <SevDot severity={s.severity} />
              <span className="font-medium">{s.name}</span>
              <span className="rounded bg-slate-800 px-1.5 py-0.5 text-xs text-slate-400">{s.type}</span>
              <span className="text-sm text-slate-400">{s.state_summary || s.current_state}</span>
              <span className="ml-auto text-xs text-slate-500">{localTime(s.last_seen_at)}</span>
            </Link>
          ))}
          {(services.data ?? []).length === 0 && <div className="py-4 text-slate-500">暂无服务</div>}
        </div>
      </div>
    </div>
  );
}

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="card p-4">
      <div className="text-xs text-slate-500">{label}</div>
      <div className="text-xl font-semibold">{value}</div>
      {sub && <div className="text-xs text-slate-500">{sub}</div>}
    </div>
  );
}
