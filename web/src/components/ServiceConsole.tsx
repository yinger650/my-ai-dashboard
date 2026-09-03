import { Pin } from "lucide-react";
import type { LogEntry, PinnedLog, Run } from "../types";
import { CollapsibleText } from "./CollapsibleText";
import { Markdown } from "./Markdown";
import { SevDot } from "./Severity";
import { fmtDuration, localTime } from "../format";
import { filterLogsByRuns } from "../lib/run-logs";
import { isActiveRunStatus, runStatusSeverity, shortRunKey } from "../lib/active-runs";
import { cn } from "../lib/utils";

export function PinnedLogPanel({ pin }: { pin: PinnedLog | null }) {
  return (
    <section className="ab-panel mb-5 flex min-h-[10rem] flex-col">
      <header className="flex shrink-0 items-center gap-2 border-b border-[#1f2a44] px-3 py-2">
        <Pin className="h-3.5 w-3.5 text-indigo-300" aria-hidden />
        <span className="ab-eyebrow">置顶</span>
        {pin && <span className="ml-auto font-mono text-[11px] text-slate-500">{localTime(pin.occurred_at)}</span>}
      </header>
      {pin ? (
        <div className="log-pane min-h-[8rem] max-h-[20rem] flex-1 overflow-y-auto px-4 py-3">
          <Markdown>{pin.markdown}</Markdown>
        </div>
      ) : (
        <div className="flex min-h-[8rem] flex-1 items-center px-4 text-sm text-slate-600">暂无置顶</div>
      )}
    </section>
  );
}

export function ServiceRunsLogs({
  runs,
  logs,
  selectedRunKeys,
  onToggleRun,
  onClearSelection,
}: {
  runs: Run[];
  logs: LogEntry[];
  selectedRunKeys: string[];
  onToggleRun: (runKey: string) => void;
  onClearSelection: () => void;
}) {
  const selected = new Set(selectedRunKeys);
  const visibleLogs = filterLogsByRuns(logs, selectedRunKeys);
  const filtering = selectedRunKeys.length > 0;
  const activeCount = runs.filter((r) => isActiveRunStatus(r.status)).length;

  return (
    <div className="grid min-h-[28rem] grid-cols-1 gap-4 lg:grid-cols-12">
      <section className="ab-panel flex min-h-[24rem] flex-col lg:col-span-5">
        <header className="flex shrink-0 items-center gap-2 border-b border-[#1f2a44] px-3 py-2">
          <span className="ab-eyebrow">Runs</span>
          <span className="font-mono text-[11px] text-slate-500">{runs.length}</span>
          {activeCount > 0 && <span className="text-[11px] text-indigo-300">进行中 {activeCount}</span>}
          {filtering && (
            <button
              type="button"
              onClick={onClearSelection}
              className="ml-auto text-[11px] text-indigo-300 hover:text-indigo-200"
            >
              显示全部
            </button>
          )}
        </header>
        <div className="log-pane min-h-0 flex-1 overflow-y-auto">
          {runs.length === 0 && <div className="px-3 py-8 text-sm text-slate-600">还没有 Run</div>}
          {runs.map((r) => {
            const on = selected.has(r.run_key);
            const sev = runStatusSeverity(r.status);
            const dur = fmtDuration(r.duration_ms);
            return (
              <button
                key={r.id}
                type="button"
                aria-pressed={on}
                aria-label={`${on ? "取消选择" : "选择"} ${r.summary || shortRunKey(r.run_key)}`}
                onClick={() => onToggleRun(r.run_key)}
                className={cn("run-channel", on && "is-on")}
              >
                <span className="mt-0.5 inline-flex">
                  <SevDot severity={sev} />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-baseline gap-2">
                    <span className="truncate text-sm text-slate-100">{r.summary || shortRunKey(r.run_key)}</span>
                    <span className={cn("shrink-0 font-mono text-[10px]", `sev-${sev}`)}>{r.status}</span>
                  </span>
                  <span className="mt-0.5 flex flex-wrap items-center gap-x-2 font-mono text-[10px] text-slate-500">
                    <span>{shortRunKey(r.run_key)}</span>
                    {dur && <span>{dur}</span>}
                    <span className="ml-auto">{localTime(r.started_at || r.created_at)}</span>
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      </section>

      <section className="ab-panel flex min-h-[24rem] flex-col lg:col-span-7">
        <header className="flex shrink-0 items-center gap-2 border-b border-[#1f2a44] px-3 py-2">
          <span className="ab-eyebrow">日志</span>
          <span className="font-mono text-[11px] text-slate-500">
            {filtering ? `${visibleLogs.length} / ${logs.length}` : logs.length}
          </span>
          {filtering && (
            <span className="truncate text-[11px] text-slate-500">已选 {selectedRunKeys.length} 个 Run</span>
          )}
        </header>
        <div className="log-pane min-h-0 flex-1 overflow-y-auto px-3 py-2">
          {visibleLogs.length === 0 && (
            <div className="py-8 text-center text-sm text-slate-600">
              {logs.length === 0 ? "还没有日志" : "选中的 Run 没有日志"}
            </div>
          )}
          {visibleLogs.map((l) => (
            <article key={l.event_id} className="border-b border-slate-900/80 py-2 last:border-0">
              <div className="mb-1 flex flex-wrap items-center gap-1.5 font-mono text-[10px] text-slate-500">
                <SevDot severity={l.severity} />
                <span className={`sev-${l.severity}`}>{l.severity}</span>
                <span>{localTime(l.occurred_at)}</span>
                {l.run_key && (
                  <span className="rounded bg-slate-800/80 px-1 text-indigo-200/80">{shortRunKey(l.run_key)}</span>
                )}
                {l.source && <span>{l.source}</span>}
              </div>
              <CollapsibleText text={l.markdown} />
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}
