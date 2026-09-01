import type { PercentMetric } from "../lib/board-metrics";
import { fmtPct } from "../format";
import { cn } from "../lib/utils";

export function PercentMetricGrid({ metrics }: { metrics: PercentMetric[] }) {
  if (metrics.length === 0) return null;
  return (
    <div className="mb-2 grid grid-cols-3 gap-2 text-center text-sm">
      {metrics.map((p) => (
        <div key={p.key} className="rounded-md bg-slate-900/60 py-2">
          <div className="truncate px-1 text-xs text-slate-500">{p.label}</div>
          <div className={cn("font-semibold", p.severity && `sev-${p.severity}`)}>{fmtPct(p.value)}</div>
        </div>
      ))}
    </div>
  );
}
