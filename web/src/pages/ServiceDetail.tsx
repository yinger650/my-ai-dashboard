import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { useEffect, useState } from "react";
import { apiGet } from "../api";
import type { LogEntry, Machine, PinnedLog, Run, Service, StatusItem } from "../types";
import { SevDot } from "../components/Severity";
import { PercentMetricGrid } from "../components/PercentMetricGrid";
import { StatusLines } from "../components/StatusLines";
import { PinnedLogPanel, ServiceRunsLogs } from "../components/ServiceConsole";
import { collectPercentMetrics } from "../lib/board-metrics";
import { userFacingStatuses } from "../lib/status-filter";
import { markServiceLogsSeen } from "../lib/log-seen";
import { toggleRunKey } from "../lib/run-logs";
import { ServicePathLine } from "../components/ServicePath";

interface ServiceDetail {
  service: Service;
  machine: Machine | null;
  statuses: StatusItem[];
  pinned: PinnedLog | null;
}

export function ServiceDetailPage() {
  const { serviceId } = useParams();
  const [selectedRunKeys, setSelectedRunKeys] = useState<string[]>([]);

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

  useEffect(() => {
    setSelectedRunKeys([]);
  }, [serviceId]);

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

  return (
    <div>
      {machine && (
        <Link to={`/machines/${machine.id}`} className="mb-3 inline-block text-sm text-indigo-300 hover:text-indigo-200">
          ← {machine.name}
        </Link>
      )}
      <div className="mb-5">
        <div className="flex flex-wrap items-center gap-3">
          <SevDot severity={s.severity} />
          <h1 className="text-xl font-semibold tracking-tight">{s.name}</h1>
          <span className="rounded border border-[#1f2a44] px-1.5 py-0.5 font-mono text-[11px] text-slate-400">{s.type}</span>
          <span className="text-sm text-slate-400">{s.state_summary || s.current_state}</span>
        </div>
        <ServicePathLine path={s.path} />
      </div>

      {percentStats.length > 0 && (
        <div className="mb-4">
          <PercentMetricGrid metrics={percentStats} />
        </div>
      )}
      {statusLines.length > 0 && (
        <div className="ab-panel mb-4 px-3 py-2.5">
          <StatusLines statuses={statusLines} />
        </div>
      )}

      <PinnedLogPanel pin={detail.data!.pinned ?? null} />

      <ServiceRunsLogs
        runs={runs.data ?? []}
        logs={logs.data ?? []}
        selectedRunKeys={selectedRunKeys}
        onToggleRun={(key) => setSelectedRunKeys((prev) => toggleRunKey(prev, key))}
        onClearSelection={() => setSelectedRunKeys([])}
      />
    </div>
  );
}
