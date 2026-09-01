import type { LogEntry } from "../types";
import { isLogUnseen } from "./log-seen";

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

/** Count logs per service id and service_key so either lookup works. */
export function countLogsByService(logs: LogEntry[]): Record<string, number> {
  return countUnseenLogsByService(logs, {});
}

/** Count unread logs per service. A log is read if occurred_at <= the service watermark. */
export function countUnseenLogsByService(
  logs: LogEntry[],
  seenUntil: Record<string, string>,
): Record<string, number> {
  const out: Record<string, number> = {};
  for (const l of logs) {
    if (!isLogUnseen(l.occurred_at, seenUntil, l.service_id, l.service_key)) continue;
    const keys = [l.service_id, l.service_key].filter(Boolean) as string[];
    const unique = [...new Set(keys)];
    for (const k of unique) {
      out[k] = (out[k] ?? 0) + 1;
    }
  }
  return out;
}
