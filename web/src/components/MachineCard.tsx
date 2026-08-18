import { Link } from "react-router-dom";
import type { BoardMachine } from "../types";
import { HealthBadge, SevDot } from "./Severity";
import { StatusList } from "./StatusList";
import { MachineLogStream } from "./MachineLogStream";
import { fmtBps, fmtPct, relativeTime, usagePct } from "../format";
import { cn } from "../lib/utils";
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
  const mem = usagePct(lm?.memory_used_bytes ?? null, lm?.memory_total_bytes ?? null);
  const disk = usagePct(lm?.root_disk_used_bytes ?? null, lm?.root_disk_total_bytes ?? null);
  const c = m.service_counts;

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

      <div className="mb-2 grid grid-cols-3 gap-2 text-center text-sm">
        <Metric label="CPU" value={fmtPct(lm?.cpu_percent ?? null)} />
        <Metric label="内存" value={fmtPct(mem)} />
        <Metric label="磁盘" value={fmtPct(disk)} />
      </div>
      <div className="mb-2 text-xs text-slate-400">
        ↓ {fmtBps(lm?.network_rx_bps ?? null)} · ↑ {fmtBps(lm?.network_tx_bps ?? null)}
      </div>

      <div className="mb-2 max-h-28 overflow-hidden">
        <StatusList services={m.services ?? []} statuses={m.statuses ?? []} collapsedCount={5} />
      </div>

      <MachineLogStream
        machineId={m.id}
        autoRefresh={autoRefresh}
        pollMs={pollMs}
        initialLogs={m.recent_logs ?? []}
        initialPinned={m.pinned_logs ?? []}
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

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-slate-900/60 py-2">
      <div className="text-xs text-slate-500">{label}</div>
      <div className="font-semibold">{value}</div>
    </div>
  );
}
