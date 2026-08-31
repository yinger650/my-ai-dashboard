import { useEffect, useState, type ReactNode } from "react";
import { Pin, X } from "lucide-react";
import type { PinnedLog } from "../types";
import { Markdown } from "./Markdown";
import { localTime } from "../format";
import { cn } from "../lib/utils";

export function PinnedLogBar({ pins, children }: { pins: PinnedLog[]; children: ReactNode }) {
  const [open, setOpen] = useState<PinnedLog | null>(null);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(null);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      {pins.length > 0 && (
        <div className="mb-1 flex h-7 min-h-7 shrink-0 items-center gap-1 overflow-x-auto whitespace-nowrap [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {pins.map((p, i) => {
            const id = p.event_id ?? `${p.occurred_at}-${i}`;
            const active = open?.event_id ? open.event_id === p.event_id : open === p;
            return (
              <button
                key={id}
                type="button"
                onClick={() => setOpen((cur) => (samePin(cur, p) ? null : p))}
                className={cn(
                  "inline-flex h-6 max-w-[10rem] shrink-0 items-center gap-1 rounded-full px-2 text-[11px] transition",
                  active
                    ? "bg-indigo-500/30 text-indigo-100"
                    : "bg-indigo-500/10 text-indigo-300 hover:bg-indigo-500/20",
                )}
                title={p.service_name ? `${p.service_name} · 点击查看置顶` : "点击查看置顶"}
              >
                <Pin className="h-3 w-3 shrink-0" />
                <span className="truncate">{p.service_name || "置顶"}</span>
              </button>
            );
          })}
        </div>
      )}
      <div className="relative h-0 min-h-0 flex-1">
        {children}
        {open && (
          <>
            <button
              type="button"
              className="absolute inset-0 z-10 bg-black/45"
              aria-label="关闭置顶"
              onClick={() => setOpen(null)}
            />
            <div
              role="dialog"
              aria-modal="true"
              aria-label={open.service_name ? `${open.service_name} 置顶日志` : "置顶日志"}
              className="absolute inset-x-1 top-1 bottom-1 z-20 flex min-h-0 flex-col overflow-hidden rounded-lg border border-indigo-500/40 bg-[#0f1626] shadow-2xl"
            >
              <div className="flex shrink-0 items-center gap-2 border-b border-slate-800 px-3 py-1.5 text-xs">
                <Pin className="h-3.5 w-3.5 text-indigo-300" />
                <span className="truncate font-medium text-indigo-100">{open.service_name || "置顶日志"}</span>
                <span className="ml-auto shrink-0 text-slate-500">{localTime(open.occurred_at)}</span>
                <button
                  type="button"
                  onClick={() => setOpen(null)}
                  className="rounded p-0.5 text-slate-500 hover:bg-slate-800 hover:text-white"
                  aria-label="关闭"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
              <div className="log-pane min-h-0 flex-1 overflow-y-scroll px-3 py-2">
                <Markdown>{open.markdown}</Markdown>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function samePin(a: PinnedLog | null, b: PinnedLog): boolean {
  if (!a) return false;
  if (a.event_id && b.event_id) return a.event_id === b.event_id;
  return a === b;
}
