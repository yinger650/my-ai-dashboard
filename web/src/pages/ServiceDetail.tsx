import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { useEffect } from "react";
import { apiDelete, apiGet, apiUpload } from "../api";
import type { Artifact, LogEntry, Machine, PinnedLog, Run, Service, StatusItem } from "../types";
import { SevDot } from "../components/Severity";
import { CollapsibleText } from "../components/CollapsibleText";
import { PercentMetricGrid } from "../components/PercentMetricGrid";
import { StatusLines } from "../components/StatusLines";
import { collectPercentMetrics } from "../lib/board-metrics";
import { userFacingStatuses } from "../lib/status-filter";
import { isActiveRunStatus } from "../lib/active-runs";
import { markServiceLogsSeen } from "../lib/log-seen";
import { fmtBytes, localTime } from "../format";
import { ServicePathLine } from "../components/ServicePath";

interface ServiceDetail {
  service: Service;
  machine: Machine | null;
  statuses: StatusItem[];
  pinned: PinnedLog | null;
}

export function ServiceDetailPage() {
  const { serviceId } = useParams();
  const qc = useQueryClient();

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
  const artifacts = useQuery({
    queryKey: ["service-artifacts", serviceId],
    queryFn: () => apiGet<{ artifacts: Artifact[]; bytes_used: number; quota_bytes: number }>(`/api/v1/services/${serviceId}/artifacts`),
    refetchInterval: 15000,
  });

  const upload = useMutation({
    mutationFn: (file: File) => {
      const fd = new FormData();
      fd.append("file", file);
      return apiUpload(`/api/v1/services/${serviceId}/artifacts`, fd);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["service-artifacts", serviceId] }),
  });
  const removeArt = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/artifacts/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["service-artifacts", serviceId] }),
  });

  useEffect(() => {
    if (serviceId) markServiceLogsSeen([serviceId]);
  }, [serviceId]);

  useEffect(() => {
    const svc = detail.data?.service;
    if (!svc) return;
    markServiceLogsSeen([svc.id, svc.service_key]);
    return () => {
      markServiceLogsSeen([svc.id, svc.service_key]);
    };
  }, [detail.data?.service.id, detail.data?.service.service_key]);

  if (detail.isLoading) return <div className="text-slate-400">加载中…</div>;
  if (detail.error) return <div className="sev-error">加载失败：{(detail.error as Error).message}</div>;

  const s = detail.data!.service;
  const machine = detail.data!.machine;
  const statuses = detail.data!.statuses ?? [];
  const percentStats = collectPercentMetrics({ statuses });
  const statusLines = userFacingStatuses(statuses, "service");
  const logList = logs.data ?? [];
  const runList = runs.data ?? [];
  const activeRuns = runList.filter((r) => isActiveRunStatus(r.status));

  return (
    <div>
      {machine && (
        <Link to={`/machines/${machine.id}`} className="mb-3 inline-block text-sm text-indigo-400">
          ← {machine.name}
        </Link>
      )}
      <div className="mb-5">
        <div className="flex flex-wrap items-center gap-3">
          <SevDot severity={s.severity} />
          <h1 className="text-2xl font-semibold">{s.name}</h1>
          <span className="rounded bg-slate-800 px-2 py-0.5 text-xs text-slate-400">{s.type}</span>
          <span className="text-sm text-slate-400">{s.state_summary || s.current_state}</span>
        </div>
        <ServicePathLine path={s.path} />
      </div>

      {percentStats.length > 0 && (
        <div className="mb-6">
          <PercentMetricGrid metrics={percentStats} />
        </div>
      )}
      {statusLines.length > 0 && (
        <div className="card mb-6 p-4">
          <StatusLines statuses={statusLines} />
        </div>
      )}

      <section className="card mb-6 p-4">
        <div className="mb-3 flex flex-wrap items-center gap-3">
          <h2 className="font-medium">附件</h2>
          <span className="text-xs text-slate-500">
            已用 {fmtBytes(artifacts.data?.bytes_used ?? 0)} / {fmtBytes(artifacts.data?.quota_bytes ?? 0)}
          </span>
          <label className="ml-auto cursor-pointer rounded-md border border-slate-700 px-3 py-1.5 text-sm hover:bg-slate-800">
            上传
            <input
              type="file"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) upload.mutate(f);
                e.target.value = "";
              }}
            />
          </label>
        </div>
        {upload.error && <div className="mb-2 text-sm sev-error">{(upload.error as Error).message}</div>}
        <div className="flex flex-col gap-3">
          {(artifacts.data?.artifacts ?? []).length === 0 && <div className="text-slate-500">暂无附件</div>}
          {(artifacts.data?.artifacts ?? []).map((a) => (
            <div key={a.id} className="rounded-md bg-slate-900/60 p-3">
              <div className="mb-1 flex flex-wrap items-center gap-2 text-sm">
                <span className="font-medium">{a.original_name}</span>
                <span className="text-xs text-slate-500">{fmtBytes(a.size_bytes)} · {a.mime_type}</span>
                <span className="text-xs text-slate-500">{localTime(a.created_at)}</span>
                <a
                  href={`/api/v1/artifacts/${a.id}/content`}
                  className="ml-auto text-xs text-indigo-400 hover:underline"
                >
                  下载
                </a>
                <button onClick={() => removeArt.mutate(a.id)} className="text-xs sev-error hover:underline">
                  删除
                </button>
              </div>
              {a.mime_type.startsWith("image/") && a.mime_type !== "image/svg+xml" && (
                <img
                  src={`/api/v1/artifacts/${a.id}/content?inline=1`}
                  alt={a.original_name}
                  className="mt-2 max-h-64 rounded-md"
                />
              )}
            </div>
          ))}
        </div>
      </section>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="card p-4">
          <h2 className="mb-3 font-medium">日志时间线</h2>
          <div className="flex h-[28rem] flex-col gap-2 overflow-y-auto pr-1">
            {logList.length === 0 && <div className="text-slate-500">暂无日志</div>}
            {logList.map((l) => (
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
          {activeRuns.length > 0 && (
            <div className="mb-3 rounded-md border border-indigo-500/30 bg-indigo-500/10 px-2 py-2 text-xs text-indigo-200">
              进行中 {activeRuns.length}
              <div className="mt-1 flex flex-col gap-1">
                {activeRuns.map((r) => (
                  <div key={`active-${r.id}`} className="flex items-center gap-2">
                    <RunStatus status={r.status} />
                    <span className="truncate">{r.summary || r.run_key.slice(0, 8)}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
          <div className="h-[28rem] divide-y divide-slate-800 overflow-y-auto pr-1">
            {runList.length === 0 && <div className="text-slate-500">暂无运行记录</div>}
            {runList.map((r) => (
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
