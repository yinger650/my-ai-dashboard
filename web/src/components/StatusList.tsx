import { useState } from "react";
import { Link } from "react-router-dom";
import { ChevronDown, ChevronUp } from "lucide-react";
import type { BoardService, StatusItem } from "../types";
import { SevDot } from "./Severity";
import { cn } from "../lib/utils";

function formatStatusValue(st: StatusItem): string {
  try {
    const v = JSON.parse(st.value_json);
    return String(v);
  } catch {
    return st.value_json;
  }
}

export function StatusList({
  services,
  statuses,
  collapsedCount = 6,
}: {
  services: BoardService[];
  statuses: StatusItem[];
  collapsedCount?: number | "all";
}) {
  const [open, setOpen] = useState(false);
  const items = buildRows(services, statuses);
  if (items.length === 0) {
    return <div className="py-1 text-xs text-slate-500">暂无状态</div>;
  }
  const cap = collapsedCount === "all" ? items.length : collapsedCount;
  const shown = open || collapsedCount === "all" ? items : items.slice(0, cap);
  const hidden = items.length - shown.length;

  return (
    <div className="flex flex-col gap-1">
      {shown.map((row) => (
        <div key={row.key} className="flex min-w-0 items-start gap-2 text-xs">
          <SevDot severity={row.severity} />
          <div className="min-w-0 flex-1">
            {row.href ? (
              <Link
                to={row.href}
                onClick={(e) => e.stopPropagation()}
                className="truncate font-medium text-slate-200 hover:text-indigo-300"
              >
                {row.title}
              </Link>
            ) : (
              <span className="truncate text-slate-300">{row.title}</span>
            )}
            {row.detail && (
              <div className={cn("truncate text-slate-500", `sev-${row.severity}`)}>{row.detail}</div>
            )}
          </div>
        </div>
      ))}
      {collapsedCount !== "all" && items.length > collapsedCount && (
        <button
          type="button"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            setOpen((v) => !v);
          }}
          className="inline-flex items-center gap-0.5 text-[11px] text-slate-500 hover:text-slate-300"
        >
          {open ? (
            <>
              收起 <ChevronUp className="h-3 w-3" />
            </>
          ) : (
            <>
              还有 {hidden} 项 <ChevronDown className="h-3 w-3" />
            </>
          )}
        </button>
      )}
    </div>
  );
}

interface Row {
  key: string;
  title: string;
  detail?: string;
  severity: string;
  href?: string;
}

function buildRows(services: BoardService[], statuses: StatusItem[]): Row[] {
  const rows: Row[] = [];
  const statusesByService = new Map<string, StatusItem[]>();
  for (const st of statuses) {
    const id = st.service_id || st.service_key || "_";
    const list = statusesByService.get(id) ?? [];
    list.push(st);
    statusesByService.set(id, list);
  }

  if (services.length > 0) {
    for (const svc of services) {
      rows.push({
        key: `svc-${svc.id}`,
        title: svc.name,
        detail: svc.state_summary || svc.current_state,
        severity: svc.severity,
        href: `/services/${svc.id}`,
      });
      for (const st of statusesByService.get(svc.id) ?? []) {
        rows.push({
          key: `st-${svc.id}-${st.status_key}`,
          title: st.label,
          detail: `${formatStatusValue(st)}${st.unit ? ` ${st.unit}` : ""}`,
          severity: st.severity,
        });
      }
    }
    return rows;
  }

  return statuses.map((st) => ({
    key: `st-${st.service_id ?? ""}-${st.status_key}`,
    title: st.service_name ? `${st.service_name} · ${st.label}` : st.label,
    detail: `${formatStatusValue(st)}${st.unit ? ` ${st.unit}` : ""}`,
    severity: st.severity,
  }));
}
