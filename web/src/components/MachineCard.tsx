import { Link } from "react-router-dom";
import { useCallback, useMemo, useState } from "react";
import type { BoardMachine, LogEntry } from "../types";
import { HealthBadge, SevDot } from "./Severity";
import { StatusList } from "./StatusList";
import { StatusLines } from "./StatusLines";
import { PercentMetricGrid } from "./PercentMetricGrid";
import { MachineLogStream } from "./MachineLogStream";
import { ActiveRunsList } from "./ActiveRunsList";
import { fmtBps, relativeTime } from "../format";
import { cn } from "../lib/utils";
import { countUnseenLogsByService } from "../lib/logs";
import { readLogSeen, markServiceLogsSeen } from "../lib/log-seen";
import { collectPercentMetrics, hasNetworkSample } from "../lib/board-metrics";
import { userFacingStatuses } from "../lib/status-filter";
import { GripVertical } from "lucide-react";

export function MachineCard({
  m,
  autoRefresh,
  pollMs,
  editMode,
}: {
  m: BoardMachine;
  autoRefresh: boolean;
  pollMs: number;
  editMode?: boolean;
}) {
  const offline = m.health === "offline";
  const lm = m.latest_metric;
  const percents = useMemo(
    () =>
      collectPercentMetrics({
        latest_metric: lm,
        heartbeat_metrics: m.heartbeat_metrics,
        statuses: m.statuses,
      }),
    [lm, m.heartbeat_metrics, m.statuses],
  );
  const lines = useMemo(() => userFacingStatuses(m.statuses, "card"), [m.statuses]);
  const showNet = hasNetworkSample(lm);
  const c = m.service_counts;
  const [liveLogs, setLiveLogs] = useState<LogEntry[]>(() => m.recent_logs ?? []);
  const [seenUntil, setSeenUntil] = useState<Record<string, string>>(readLogSeen);
  const onLogsChange = useCallback((logs: LogEntry[]) => setLiveLogs(logs), []);
  const newLogCounts = useMemo(() => countUnseenLogsByService(liveLogs, seenUntil), [liveLogs, seenUntil]);
  const onOpenService = useCallback((serviceId?: string, serviceKey?: string) => {
    setSeenUntil(markServiceLogsSeen([serviceId, serviceKey]));
  }, []);

  return (
    <article
      className={cn(
        "flex h-full min-h-0 flex-col rounded-xl border border-slate-800 bg-[#0f1626] p-4 shadow-sm transition",
        offline && "opacity-70",
        editMode && "ring-1 ring-indigo-500/30",
      )}
    >
      <header className="mb-3 flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            {editMode && (
              <span className="drag-handle cursor-grab text-slate-600 active:cursor-grabbing">
                <GripVertical className="h-4 w-4" />
              </span>
            )}
            <Link to={`/machines/${m.id}`} className="truncate font-medium hover:text-indigo-300">
              {m.name}
            </Link>
          </div>
          <div className="truncate text-xs text-slate-500">
            {m.kind} · {m.machine_key}
          </div>
        </div>
        <HealthBadge health={m.health} />
      </header>

      <PercentMetricGrid metrics={percents} />
      {showNet && (
        <div className="mb-2 text-xs text-slate-400">
          ↓ {fmtBps(lm?.network_rx_bps ?? null)} · ↑ {fmtBps(lm?.network_tx_bps ?? null)}
        </div>
      )}

      <StatusLines statuses={lines} />

      <ActiveRunsList runs={m.active_runs ?? []} compact />

      <div className="log-pane mb-2 max-h-24 overflow-y-auto pr-1">
        <StatusList
          services={m.services ?? []}
          statuses={[]}
          collapsedCount={8}
          compact
          newLogCounts={newLogCounts}
          onOpenService={onOpenService}
          host={{ kind: m.kind, machineLastSeenAt: m.last_seen_at }}
        />
      </div>

      <MachineLogStream
        machineId={m.id}
        autoRefresh={autoRefresh}
        pollMs={pollMs}
        initialLogs={m.recent_logs ?? []}
        initialPinned={m.pinned_logs ?? []}
        compact
        onLogsChange={onLogsChange}
      />

      <footer className="mt-2 flex items-center gap-3 text-xs">
        <span className="flex items-center gap-1">
          <SevDot severity="normal" /> {c.normal}
        </span>
        <span className="flex items-center gap-1">
          <SevDot severity="warning" /> {c.warning}
        </span>
        <span className="flex items-center gap-1">
          <SevDot severity="error" /> {c.error}
        </span>
        <span className="ml-auto text-slate-500">
          {offline ? "最后数据 · " : ""}
          {relativeTime(m.last_seen_at)}
        </span>
      </footer>
    </article>
  );
}
