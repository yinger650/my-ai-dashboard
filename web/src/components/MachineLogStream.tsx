import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowUp, Pin } from "lucide-react";
import { apiGetPage } from "../api";
import type { LogEntry, MachineLogsPage, PinnedLog } from "../types";
import { countNewLogs, mergeLogPages } from "../lib/logs";
import { CollapsibleText } from "./CollapsibleText";
import { SevDot } from "./Severity";
import { localTime } from "../format";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";

const PAGE_SIZE = 30;

export function MachineLogStream({
  machineId,
  autoRefresh,
  pollMs,
  initialLogs,
  initialPinned,
}: {
  machineId: string;
  autoRefresh: boolean;
  pollMs: number;
  initialLogs: LogEntry[];
  initialPinned: PinnedLog[];
}) {
  const [logs, setLogs] = useState<LogEntry[]>(initialLogs);
  const [pinned, setPinned] = useState<PinnedLog[]>(initialPinned);
  const [cursor, setCursor] = useState<string | null>(
    initialLogs.length >= PAGE_SIZE ? initialLogs[initialLogs.length - 1]?.occurred_at ?? null : null,
  );
  const [hasMore, setHasMore] = useState(initialLogs.length >= PAGE_SIZE);
  const [loadingMore, setLoadingMore] = useState(false);
  const [newCount, setNewCount] = useState(0);
  const [atTop, setAtTop] = useState(true);

  const scrollRef = useRef<HTMLDivElement | null>(null);
  const atTopRef = useRef(true);
  const logsRef = useRef(logs);
  const olderRef = useRef<LogEntry[]>([]);
  const loadingRef = useRef(false);
  logsRef.current = logs;

  const fetchLatest = useCallback(
    async (opts: { jump?: boolean } = {}) => {
      const page = await apiGetPage<MachineLogsPage>(`/api/v1/machines/${machineId}/logs`);
      const incoming = page.data.logs ?? [];
      if (page.data.pinned) setPinned(page.data.pinned);
      const added = countNewLogs(incoming, logsRef.current);
      const merged = mergeLogPages(incoming, [...olderRef.current, ...logsRef.current]);
      const el = scrollRef.current;
      const prevHeight = el?.scrollHeight ?? 0;
      const wasAtTop = atTopRef.current;
      setLogs(merged);
      if (!olderRef.current.length) {
        setCursor(page.nextCursor);
        setHasMore(Boolean(page.nextCursor));
      }
      if (opts.jump) {
        setNewCount(0);
        requestAnimationFrame(() => {
          if (scrollRef.current) scrollRef.current.scrollTop = 0;
        });
        return;
      }
      if (!wasAtTop && added > 0) {
        setNewCount((n) => n + added);
        requestAnimationFrame(() => {
          if (!scrollRef.current) return;
          const delta = scrollRef.current.scrollHeight - prevHeight;
          if (delta > 0) scrollRef.current.scrollTop += delta;
        });
      } else if (wasAtTop) {
        setNewCount(0);
      }
    },
    [machineId],
  );

  useEffect(() => {
    olderRef.current = [];
    setLogs(initialLogs);
    setPinned(initialPinned);
    setCursor(null);
    setHasMore(true);
    setNewCount(0);
    void fetchLatest().catch(() => undefined);
  }, [machineId, fetchLatest]);

  useEffect(() => {
    if (!autoRefresh) return;
    const id = window.setInterval(() => {
      void fetchLatest().catch(() => undefined);
    }, pollMs);
    return () => window.clearInterval(id);
  }, [autoRefresh, pollMs, fetchLatest]);

  const loadMore = useCallback(async () => {
    if (!hasMore || loadingRef.current || !cursor) return;
    loadingRef.current = true;
    setLoadingMore(true);
    try {
      const page = await apiGetPage<MachineLogsPage>(
        `/api/v1/machines/${machineId}/logs?cursor=${encodeURIComponent(cursor)}`,
      );
      const incoming = page.data.logs ?? [];
      olderRef.current = mergeLogPages(olderRef.current, incoming);
      setLogs((prev) => mergeLogPages(prev, incoming));
      setCursor(page.nextCursor);
      setHasMore(Boolean(page.nextCursor) && incoming.length > 0);
    } finally {
      loadingRef.current = false;
      setLoadingMore(false);
    }
  }, [cursor, hasMore, machineId]);

  function onScroll() {
    const el = scrollRef.current;
    if (!el) return;
    const top = el.scrollTop < 16;
    atTopRef.current = top;
    setAtTop(top);
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 48) {
      void loadMore();
    }
  }

  function jumpToLatest() {
    void fetchLatest({ jump: true });
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {pinned.length > 0 && (
        <section className="mb-2 min-h-0 shrink-0" aria-label="置顶日志">
          <div className="mb-1 flex items-center gap-1 text-[11px] text-indigo-300">
            <Pin className="h-3 w-3" />
            置顶日志
          </div>
          <div className="log-pane max-h-28 overflow-y-auto overscroll-contain rounded-md border border-indigo-500/25 bg-indigo-500/5 px-2 py-1.5 font-mono text-[12px] leading-relaxed">
            {pinned.map((p, i) => (
              <div key={p.event_id ?? `${p.occurred_at}-${i}`} className="border-b border-indigo-500/10 py-1 last:border-0">
                <div className="mb-0.5 flex items-center gap-1 text-[10px] text-indigo-300/80">
                  {p.service_name && <span>{p.service_name}</span>}
                  <span className="ml-auto text-slate-500">{localTime(p.occurred_at)}</span>
                </div>
                <CollapsibleText text={p.markdown} maxChars={180} />
              </div>
            ))}
          </div>
        </section>
      )}

      <div className="mb-1 flex items-center justify-between text-[11px] text-slate-500">
        <span>日志</span>
        <button
          type="button"
          onClick={jumpToLatest}
          className={cn(
            "inline-flex items-center gap-1 rounded px-1.5 py-0.5 transition hover:bg-slate-800 hover:text-slate-200",
            !atTop || newCount > 0 ? "text-indigo-400" : "text-slate-500",
          )}
        >
          <ArrowUp className="h-3 w-3" />
          {newCount > 0 ? `${newCount} 条新日志 · 回到顶部` : "回到顶部刷新"}
        </button>
      </div>
      <div
        ref={scrollRef}
        onScroll={onScroll}
        aria-label="日志时间线"
        className="log-pane min-h-0 flex-1 overflow-y-auto overscroll-contain rounded-md border border-slate-800 bg-slate-950/70 px-2 py-1.5 font-mono text-[12px] leading-relaxed"
      >
        {logs.length === 0 && (
          <div className="py-6 text-center text-slate-600">暂无日志</div>
        )}
        {logs.map((l) => (
          <div key={l.event_id} className="border-b border-slate-900/80 py-1.5 last:border-0">
            <div className="mb-0.5 flex flex-wrap items-center gap-1.5 text-[10px] text-slate-500">
              <SevDot severity={l.severity} />
              <span className={`sev-${l.severity}`}>{l.severity}</span>
              <span>{localTime(l.occurred_at)}</span>
              {l.service_name && <span className="rounded bg-slate-800 px-1">{l.service_name}</span>}
              {l.source && <span>{l.source}</span>}
            </div>
            <CollapsibleText text={l.markdown} />
          </div>
        ))}
        {hasMore && (
          <div className="py-2 text-center">
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={loadingMore}
              onClick={() => void loadMore()}
            >
              {loadingMore ? "加载中…" : "加载更早日志"}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
