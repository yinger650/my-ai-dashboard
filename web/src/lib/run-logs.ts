import type { LogEntry } from "../types";

/** Unselected = all logs. Selected = only those run_keys (unbound logs drop). */
export function filterLogsByRuns(logs: LogEntry[], selectedRunKeys: string[]): LogEntry[] {
  if (selectedRunKeys.length === 0) return logs;
  const want = new Set(selectedRunKeys);
  return logs.filter((l) => Boolean(l.run_key) && want.has(l.run_key!));
}

export function toggleRunKey(selected: string[], runKey: string): string[] {
  return selected.includes(runKey) ? selected.filter((k) => k !== runKey) : [...selected, runKey];
}
