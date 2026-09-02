import type { PercentMetric } from "../lib/board-metrics";
import { fmtPct } from "../format";
import { cn } from "../lib/utils";

export function PercentMetricGrid({ metrics }: { metrics: PercentMetric[] }) {
  if (metrics.length === 0) return null;
  return (
    <div className={cn("mb-2 grid gap-px overflow-hidden rounded-md border border-[#1f2a44] bg-[#1f2a44] text-center text-sm", metrics.length >= 4 ? "grid-cols-2 sm:grid-cols-4" : "grid-cols-3")}>
      {metrics.map((p) => (
        <div key={p.key} className="bg-[#0f1626] py-1.5">
          <div className="truncate px-1 text-[10px] uppercase tracking-wider text-slate-500">{p.label}</div>
          <div className={cn("font-mono text-sm font-medium", p.severity && `sev-${p.severity}`)}>{fmtPct(p.value)}</div>
        </div>
      ))}
    </div>
  );
}
