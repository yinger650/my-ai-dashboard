import { useState } from "react";
import { Link } from "react-router-dom";
import { ChevronDown, ChevronUp } from "lucide-react";
import type { BoardService, StatusItem } from "../types";
import { compactCardServices } from "../lib/board-card";
import { formatStatusValue } from "../lib/board-metrics";
import { SevDot } from "./Severity";
import { cn } from "../lib/utils";

function NewLogBadge({ count }: { count: number }) {
  if (count <= 0) return null;
  const label = count > 99 ? "99+" : String(count);
  return (
    <span
      className="inline-flex h-4 min-w-4 shrink-0 items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-semibold leading-none text-white"
      title={`${count} 条日志`}
    >
      {label}
    </span>
  );
}

export function StatusList({
  services,
  statuses,
  collapsedCount = 6,
  compact = false,
  newLogCounts = {},
  host,
  onOpenService,
}: {
  services: BoardService[];
  statuses: StatusItem[];
  collapsedCount?: number;
  compact?: boolean;
  newLogCounts?: Record<string, number>;
  host?: { kind?: string | null; machineLastSeenAt?: string | null };
  onOpenService?: (serviceId?: string, serviceKey?: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const items = buildRows(compact ? compactCardServices(services, host) : services, compact ? [] : statuses);
  if (items.length === 0) {
    return <div className="py-1 text-xs text-slate-500">暂无状态</div>;
  }
  const shown = open ? items : items.slice(0, collapsedCount);
  const hidden = items.length - shown.length;

  return (
    <div className="flex flex-col gap-1">
      {shown.map((row) => {
        const count = newLogCounts[row.serviceId ?? ""] ?? newLogCounts[row.serviceKey ?? ""] ?? 0;
        return (
          <div
            key={row.key}
            className={cn("flex min-w-0 gap-2 text-xs", compact ? "items-center" : "items-start")}
          >
            <SevDot severity={row.severity} />
            <div className="min-w-0 flex-1">
              {row.href ? (
                <Link
                  to={row.href}
                  onClick={(e) => {
                    e.stopPropagation();
                    onOpenService?.(row.serviceId, row.serviceKey);
                  }}
                  className="block truncate font-medium text-slate-200 hover:text-indigo-300"
                >
                  {row.title}
                </Link>
              ) : (
                <span className="block truncate text-slate-300">{row.title}</span>
              )}
              {!compact && row.detail && (
                <div className={cn("truncate text-slate-500", `sev-${row.severity}`)}>{row.detail}</div>
              )}
            </div>
            {compact && <NewLogBadge count={count} />}
          </div>
        );
      })}
      {items.length > collapsedCount && (
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
  serviceId?: string;
  serviceKey?: string;
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
        serviceId: svc.id,
        serviceKey: svc.service_key,
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
    serviceId: st.service_id,
    serviceKey: st.service_key,
  }));
}
