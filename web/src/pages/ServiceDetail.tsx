import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { apiGet } from "../api";
import type { LogEntry, Machine, PinnedLog, Run, Service, StatusItem } from "../types";
import { SevDot } from "../components/Severity";
import { Markdown } from "../components/Markdown";
import { CollapsibleText } from "../components/CollapsibleText";
import { localTime } from "../format";

interface ServiceDetail {
  service: Service;
  machine: Machine | null;
  statuses: StatusItem[];
  pinned: PinnedLog | null;
}

export function ServiceDetailPage() {
  const { serviceId } = useParams();

  const detail = useQuery({
    queryKey: ["service", serviceId],
    queryFn: () => apiGet<ServiceDetail>(`/api/v1/services/${serviceId}`),
    refetchInterval: 15000,
  });
  const logs = useQuery({
    queryKey: ["service-logs", serviceId],
    queryFn: () => apiGet<LogEntry[]>(`/api/v1/services/${serviceId}/logs`),
    refetchInterval: 10000,
  });
  const runs = useQuery({
    queryKey: ["service-runs", serviceId],
    queryFn: () => apiGet<Run[]>(`/api/v1/services/${serviceId}/runs`),
    refetchInterval: 15000,
  });

  if (detail.isLoading) return <div className="text-slate-400">加载中…</div>;
  if (detail.error) return <div className="sev-error">加载失败：{(detail.error as Error).message}</div>;

  const s = detail.data!.service;
  const machine = detail.data!.machine;
  const statuses = detail.data!.statuses ?? [];
  const pinned = detail.data!.pinned;
  const logList = logs.data ?? [];

  return (
    <div>
      {machine && (
        <Link to={`/machines/${machine.id}`} className="mb-3 inline-block text-sm text-indigo-400">
          ← {machine.name}
        </Link>
      )}
      <div className="mb-5 flex items-center gap-3">
        <SevDot severity={s.severity} />
        <h1 className="text-2xl font-semibold">{s.name}</h1>
        <span className="rounded bg-slate-800 px-2 py-0.5 text-xs text-slate-400">{s.type}</span>
        <span className="text-sm text-slate-400">{s.state_summary || s.current_state}</span>
      </div>

      {statuses.length > 0 && (
        <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {statuses.map((st) => (
            <div key={st.status_key} className="card p-3">
              <div className="flex items-center gap-2 text-xs text-slate-500">
                <SevDot severity={st.severity} /> {st.label}
              </div>
              <div className="text-lg font-semibold">
                {formatStatusValue(st)} <span className="text-xs text-slate-500">{st.unit}</span>
              </div>
            </div>
          ))}
        </div>
      )}

      {pinned && (
        <div className="card mb-6 border-l-4 p-4" style={{ borderLeftColor: "#60a5fa" }}>
          <div className="mb-1 text-xs text-slate-500">置顶日志 · {localTime(pinned.occurred_at)}</div>
          <Markdown>{pinned.markdown}</Markdown>
        </div>
      )}

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="card p-4">
          <h2 className="mb-3 font-medium">日志时间线</h2>
          <div className="flex flex-col gap-2">
            {logList.length === 0 && <div className="text-slate-500">暂无日志</div>}
            {[...logList].reverse().map((l) => (
              <div key={l.event_id} className="rounded-md bg-slate-900/60 p-2">
                <div className="mb-1 flex items-center gap-2 text-xs text-slate-500">
                  <SevDot severity={l.severity} />
                  {localTime(l.occurred_at)}
                  {l.source && <span className="rounded bg-slate-800 px-1">{l.source}</span>}
                </div>
                <CollapsibleText text={l.markdown} />
              </div>
            ))}
          </div>
        </div>

        <div className="card p-4">
          <h2 className="mb-3 font-medium">运行记录 (Runs)</h2>
          <div className="divide-y divide-slate-800">
            {(runs.data ?? []).length === 0 && <div className="text-slate-500">暂无运行记录</div>}
            {(runs.data ?? []).map((r) => (
              <div key={r.id} className="flex items-center gap-3 py-2 text-sm">
                <RunStatus status={r.status} />
                <span className="text-slate-300">{r.summary || r.run_key.slice(0, 8)}</span>
                <span className="ml-auto text-xs text-slate-500">{localTime(r.created_at)}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function formatStatusValue(st: StatusItem): string {
  try {
    const v = JSON.parse(st.value_json);
    return String(v);
  } catch {
    return st.value_json;
  }
}

const RUN_SEV: Record<string, string> = {
  succeeded: "normal",
  running: "info",
  queued: "info",
  waiting_input: "info",
  blocked: "warning",
  failed: "error",
  timed_out: "error",
  cancelled: "unknown",
};

function RunStatus({ status }: { status: string }) {
  return (
    <span className={`flex items-center gap-1 sev-${RUN_SEV[status] ?? "unknown"}`}>
      <SevDot severity={RUN_SEV[status] ?? "unknown"} /> {status}
    </span>
  );
}
