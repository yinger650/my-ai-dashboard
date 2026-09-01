import { Link } from "react-router-dom";
import type { ActiveRun } from "../types";
import { formatActiveRunLine } from "../lib/active-runs";
import { SevDot } from "./Severity";

const RUN_SEV: Record<string, string> = {
  running: "info",
  queued: "info",
  waiting_input: "info",
  blocked: "warning",
};

export function ActiveRunsList({
  runs,
  compact = false,
}: {
  runs: ActiveRun[];
  compact?: boolean;
}) {
  if (runs.length === 0) {
    return null;
  }
  return (
    <div className={compact ? "mb-2" : "mb-4"}>
      <div className="mb-1 text-[11px] font-medium text-slate-400">进行中 {runs.length}</div>
      <div className="flex flex-col gap-0.5">
        {runs.map((r) => (
          <Link
            key={r.id}
            to={`/services/${r.service_id}`}
            className="flex min-w-0 items-center gap-1.5 text-xs text-slate-300 hover:text-indigo-300"
            title={formatActiveRunLine(r)}
          >
            <SevDot severity={RUN_SEV[r.status] ?? "info"} />
            <span className="truncate">{formatActiveRunLine(r)}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}
