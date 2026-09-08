import type { StatusItem } from "../types";
import { formatStatusValue } from "../lib/board-metrics";
import { groupStatusesByService } from "../lib/status-filter";
import { SevDot } from "./Severity";

function StatusRow({ st }: { st: StatusItem }) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      <SevDot severity={st.severity} />
      <span className="truncate text-slate-400">{st.label}</span>
      <span className="ml-auto truncate text-slate-200">
        {formatStatusValue(st)}
        {st.unit ? ` ${st.unit}` : ""}
      </span>
    </div>
  );
}

export function StatusLines({
  statuses,
  grouped = false,
}: {
  statuses: StatusItem[];
  grouped?: boolean;
}) {
  if (statuses.length === 0) return null;
  if (!grouped) {
    return (
      <div className="mb-2 flex flex-col gap-0.5 text-xs">
        {statuses.map((st) => (
          <StatusRow key={`${st.service_id ?? ""}-${st.status_key}`} st={st} />
        ))}
      </div>
    );
  }

  const groups = groupStatusesByService(statuses);
  return (
    <div className="mb-2 flex flex-col gap-2 text-xs">
      {groups.map((g) => (
        <section key={g.key} className="min-w-0">
          <h3 className="mb-0.5 truncate font-medium text-slate-200">{g.title}</h3>
          <div className="flex flex-col gap-0.5">
            {g.items.map((st) => (
              <StatusRow key={`${st.service_id ?? ""}-${st.status_key}`} st={st} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
