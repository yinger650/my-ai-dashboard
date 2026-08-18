import type { LogEntry } from "../types";

/** Merge log pages by event_id, newest first. */
export function mergeLogPages(primary: LogEntry[], extra: LogEntry[]): LogEntry[] {
  const seen = new Set<string>();
  const out: LogEntry[] = [];
  for (const entry of [...primary, ...extra]) {
    if (!entry?.event_id || seen.has(entry.event_id)) continue;
    seen.add(entry.event_id);
    out.push(entry);
  }
  out.sort((a, b) => {
    const cmp = b.occurred_at.localeCompare(a.occurred_at);
    if (cmp !== 0) return cmp;
    return b.event_id.localeCompare(a.event_id);
  });
  return out;
}

export function countNewLogs(incoming: LogEntry[], existing: LogEntry[]): number {
  const have = new Set(existing.map((l) => l.event_id));
  return incoming.filter((l) => l.event_id && !have.has(l.event_id)).length;
}
