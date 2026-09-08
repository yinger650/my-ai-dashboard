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
import { ScrollText } from "lucide-react";
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
import { MachineLogStream } from "../components/MachineLogStream";
import { visibleHostServices } from "../lib/host-services";
import { markServiceLogsSeen } from "../lib/log-seen";
import { ServicePathLine } from "../components/ServicePath";
import { useMediaQuery } from "../hooks/useMediaQuery";
import { cn } from "../lib/utils";

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
const WIDE_QUERY = "(min-width: 1024px)";

export function MachineDetailPage() {
  const { machineId } = useParams();
  const [range, setRange] = useState("1h");
  const [logOpen, setLogOpen] = useState(false);
  const wide = useMediaQuery(WIDE_QUERY);

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
    disk: usagePct(s.root_disk_used_bytes, s.root_disk_total_bytes),
  }));
  const logsVisible = wide || logOpen;

  return (
    <div className={cn("flex min-h-0 flex-col", wide && "h-[calc(100dvh-5.5rem)]")}>
      <div className="shrink-0">
        <Link to="/" className="mb-3 inline-block text-sm text-indigo-400">
          ← 返回看板
        </Link>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight">{m.name}</h1>
              <HealthBadge health={detail.data!.health} />
            </div>
            <p className="text-sm text-slate-400">
              {m.hostname ?? "-"} · {m.os ?? "-"}/{m.arch ?? "-"} · Collector {m.collector_version ?? "-"} · 最后上报{" "}
              {relativeTime(m.last_seen_at)}
            </p>
          </div>
        </div>
      </div>

      <div className={cn("flex min-h-0 flex-1 flex-col gap-4", wide && "flex-row")}>
        <div className={cn("min-w-0", wide && "flex-1 overflow-y-auto pr-1")}>
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
              <StatusLines statuses={lines} grouped />
            </div>
          )}
          {(detail.data!.active_runs ?? []).length > 0 && (
            <div className="card mb-4 p-4">
              <ActiveRunsList runs={detail.data!.active_runs ?? []} />
            </div>
          )}

          {chart.length > 0 && (
            <div className="card mb-4 p-4">
              <div className="mb-3 flex items-center justify-between">
                <h2 className="font-medium">百分比指标趋势</h2>
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
                  <Area type="monotone" dataKey="disk" name="磁盘 %" stroke="#fbbf24" fillOpacity={0} strokeWidth={2} isAnimationActive={false} />
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
                    <ServicePathLine path={s.path} />
                    <p className={`text-xs sev-${s.severity}`}>{describeServiceStatus(s)}</p>
                  </div>
                </Link>
              ))}
              {visibleServices.length === 0 && <div className="py-4 text-slate-500">暂无服务</div>}
            </div>
          </div>
        </div>

        {!wide && (
          <button
            type="button"
            aria-expanded={logOpen}
            aria-controls="machine-logs"
            aria-label={logOpen ? "隐藏日志" : "打开日志"}
            onClick={() => setLogOpen((v) => !v)}
            className={cn(
              "fixed top-[38%] z-40 inline-flex items-center gap-1 rounded-l-md border border-r-0 px-1.5 py-3 text-xs font-medium text-white shadow-lg shadow-indigo-500/40 transition",
              logOpen
                ? "right-[min(100vw-2.75rem,28rem)] border-indigo-300 bg-indigo-500 hover:bg-indigo-400"
                : "right-0 border-indigo-400 bg-indigo-600 hover:bg-indigo-500",
            )}
          >
            <ScrollText className="h-3.5 w-3.5" />
            日志
          </button>
        )}

        <aside
          id="machine-logs"
          aria-label="机器日志"
          aria-hidden={!logsVisible}
          className={cn(
            "flex min-h-0 flex-col",
            wide
              ? "w-[min(42%,34rem)] shrink-0"
              : cn(
                  "fixed inset-y-0 right-0 z-30 w-[min(100vw-2.75rem,28rem)] bg-[#0b1020] pt-14 shadow-2xl transition-transform duration-200",
                  logOpen ? "translate-x-0" : "pointer-events-none translate-x-full",
                ),
          )}
        >
          <MachineLogsPane machineId={m.id} />
        </aside>
      </div>
    </div>
  );
}

function MachineLogsPane({ machineId }: { machineId: string }) {
  return (
    <div className="ab-panel flex h-full min-h-0 flex-1 flex-col p-3">
      <MachineLogStream machineId={machineId} autoRefresh pollMs={15000} initialLogs={[]} initialPinned={[]} />
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
