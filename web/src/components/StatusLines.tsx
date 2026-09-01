import type { StatusItem } from "../types";
import { formatStatusValue } from "../lib/board-metrics";
import { SevDot } from "./Severity";

export function StatusLines({ statuses }: { statuses: StatusItem[] }) {
  if (statuses.length === 0) return null;
  return (
    <div className="mb-2 flex flex-col gap-0.5 text-xs">
      {statuses.map((st) => (
        <div key={`${st.service_id ?? ""}-${st.status_key}`} className="flex min-w-0 items-center gap-2">
          <SevDot severity={st.severity} />
          <span className="truncate text-slate-400">{st.label}</span>
          <span className="ml-auto truncate text-slate-200">
            {formatStatusValue(st)}
            {st.unit ? ` ${st.unit}` : ""}
          </span>
        </div>
      ))}
    </div>
  );
}
