import { Plus, X } from "lucide-react";
import type { StatusItem } from "../types";
import { fmtPct } from "../format";
import { formatStatusValue } from "../lib/board-metrics";
import { availableStatusPicks, type DisplayTile } from "../lib/machine-tiles";
import { groupStatusesByService } from "../lib/status-filter";
import { cn } from "../lib/utils";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";

export function MachineTileGrid({
  tiles,
  canAdd,
  pickerOpen,
  onPickerOpenChange,
  statuses,
  onPick,
  onRemove,
}: {
  tiles: DisplayTile[];
  canAdd: boolean;
  pickerOpen: boolean;
  onPickerOpenChange: (open: boolean) => void;
  statuses: StatusItem[];
  onPick: (st: StatusItem) => void;
  onRemove: (id: string) => void;
}) {
  const picks = availableStatusPicks(statuses, tiles);
  const groups = groupStatusesByService(picks);

  return (
    <>
      <div className="mb-4 grid grid-cols-3 gap-2" data-testid="machine-tile-grid">
        {tiles.map((t) => (
          <div
            key={t.id}
            className="relative rounded-md border border-[#1f2a44] bg-[#0f1626] py-2 text-center"
          >
            {t.removable && (
              <button
                type="button"
                aria-label={`移除 ${t.label}`}
                onClick={() => onRemove(t.id)}
                className="absolute right-1 top-1 rounded p-0.5 text-slate-600 hover:text-slate-200"
              >
                <X className="h-3 w-3" />
              </button>
            )}
            <div className="truncate px-2 text-[10px] uppercase tracking-wider text-slate-500">{t.label}</div>
            <div className={cn("px-2 font-mono text-sm font-medium", t.severity && `sev-${t.severity}`)}>
              {t.kind === "percent" ? fmtPct(t.value) : t.text}
            </div>
          </div>
        ))}
        {canAdd && (
          <button
            type="button"
            aria-label="添加瓦片"
            onClick={() => onPickerOpenChange(true)}
            className="flex min-h-[3.25rem] items-center justify-center rounded-md border border-dashed border-slate-600 bg-transparent text-slate-500 transition hover:border-indigo-400 hover:text-indigo-300"
          >
            <Plus className="h-5 w-5" />
          </button>
        )}
      </div>

      <Dialog open={pickerOpen} onOpenChange={onPickerOpenChange}>
        <DialogContent className="max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>从状态日志添加瓦片</DialogTitle>
            <DialogDescription>选一条状态，吸附到上方瓦片区（最多 12 个）。</DialogDescription>
          </DialogHeader>
          {groups.length === 0 ? (
            <p className="py-6 text-center text-sm text-slate-500">没有可添加的状态</p>
          ) : (
            <div className="flex flex-col gap-3">
              {groups.map((g) => (
                <section key={g.key}>
                  <h3 className="mb-1 text-xs font-medium text-slate-300">{g.title}</h3>
                  <div className="flex flex-col gap-1">
                    {g.items.map((row) => (
                      <button
                        key={row.service_id ? `${row.service_id}-${row.status_key}` : row.status_key}
                        type="button"
                        onClick={() => onPick(row)}
                        className="flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-slate-800/80"
                      >
                        <span className="truncate text-slate-400">{row.label}</span>
                        <span className="ml-auto truncate font-mono text-xs text-slate-200">
                          {formatStatusValue(row)}
                          {row.unit ? ` ${row.unit}` : ""}
                        </span>
                      </button>
                    ))}
                  </div>
                </section>
              ))}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
