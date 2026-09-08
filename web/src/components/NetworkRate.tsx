import { fmtBps } from "../format";
import { cn } from "../lib/utils";

export function NetworkRate({
  rx,
  tx,
  className,
}: {
  rx: number | null | undefined;
  tx: number | null | undefined;
  className?: string;
}) {
  if (rx == null && tx == null) return null;
  return (
    <span className={cn("shrink-0 font-mono text-xs text-slate-400", className)} title="网速">
      ↓ {fmtBps(rx ?? null)} · ↑ {fmtBps(tx ?? null)}
    </span>
  );
}
