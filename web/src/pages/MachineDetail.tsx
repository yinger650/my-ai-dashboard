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
import type { ActiveRun, Machine, MetricSample, Service, StatusItem } from "../types";
import { HealthBadge, SevDot } from "../components/Severity";
import { fmtBps, localTime, relativeTime, usagePct } from "../format";
import { describeServiceFunction, describeServiceStatus } from "../lib/service-brief";
import { collectPercentMetrics, hasNetworkSample } from "../lib/board-metrics";
import { userFacingStatuses } from "../lib/status-filter";
import { PercentMetricGrid } from "../components/PercentMetricGrid";
import { StatusLines } from "../components/StatusLines";
import { ActiveRunsList } from "../components/ActiveRunsList";
import { visibleHostServices } from "../lib/host-services";
import { markServiceLogsSeen } from "../lib/log-seen";

interface MachineDetail {
  machine: Machine;
  latest_metric: MetricSample | null;
  health: string;
  resource_severity: string;
  heartbeat_metrics?: Record<string, number> | null;
  statuses?: StatusItem[] | null;
  active_runs?: ActiveRun[] | null;
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
  const percents = collectPercentMetrics({
    latest_metric: lm,
    heartbeat_metrics: detail.data!.heartbeat_metrics,
    statuses: detail.data!.statuses,
  });
  const lines = userFacingStatuses(detail.data!.statuses, "machine");
  const showNet = hasNetworkSample(lm);
  const visibleServices = visibleHostServices(services.data ?? [], {
    kind: m.kind,
    machineLastSeenAt: m.last_seen_at,
  });
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

      {(percents.length > 0 || showNet) && (
        <div className="mb-4">
          <PercentMetricGrid metrics={percents} />
          {showNet && (
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <Stat label="网络" value={`↓${fmtBps(lm?.network_rx_bps ?? null)}`} sub={`↑${fmtBps(lm?.network_tx_bps ?? null)}`} />
            </div>
          )}
        </div>
      )}
      {lines.length > 0 && (
        <div className="card mb-4 p-4">
          <h2 className="mb-2 text-sm font-medium text-slate-400">状态</h2>
          <StatusLines statuses={lines} />
        </div>
      )}
      {(detail.data!.active_runs ?? []).length > 0 && (
        <div className="card mb-4 p-4">
          <ActiveRunsList runs={detail.data!.active_runs ?? []} />
        </div>
      )}

      {chart.length > 0 && (
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
        </div>
      )}

      <div className="card p-4">
        <h2 className="mb-3 font-medium">服务 ({visibleServices.length})</h2>
        <div className="divide-y divide-slate-800">
          {visibleServices.map((s) => (
            <Link
              key={s.id}
              to={`/services/${s.id}`}
              onClick={() => markServiceLogsSeen([s.id, s.service_key])}
              className="flex items-start gap-3 py-2.5 hover:bg-slate-800/40"
            >
              <span className="mt-1.5 inline-flex">
                <SevDot severity={s.severity} />
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{s.name}</span>
                  <span className="rounded bg-slate-800 px-1.5 py-0.5 text-xs text-slate-400">{s.type}</span>
                  <span className="ml-auto text-xs text-slate-500">{localTime(s.last_seen_at)}</span>
                </div>
                <p className="mt-0.5 text-xs leading-relaxed text-slate-400">{describeServiceFunction(s)}</p>
                <p className={`text-xs sev-${s.severity}`}>{describeServiceStatus(s)}</p>
              </div>
            </Link>
          ))}
          {visibleServices.length === 0 && <div className="py-4 text-slate-500">暂无服务</div>}
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
