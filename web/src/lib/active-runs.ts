import type { ActiveRun } from "../types";

const ACTIVE_STATUSES = new Set(["queued", "running", "waiting_input", "blocked"]);

export const RUN_STATUS_SEVERITY: Record<string, string> = {
  succeeded: "normal",
  running: "info",
  queued: "info",
  waiting_input: "info",
  blocked: "warning",
  failed: "error",
  timed_out: "error",
  cancelled: "unknown",
};

export function isActiveRunStatus(status: string): boolean {
  return ACTIVE_STATUSES.has(status);
}

export function runStatusSeverity(status: string): string {
  return RUN_STATUS_SEVERITY[status] ?? "unknown";
}

export function formatActiveRunLine(r: Pick<ActiveRun, "service_name" | "summary" | "status">): string {
  const detail = r.summary.trim() || r.status;
  return `${r.service_name} · ${detail}`;
}

export function shortRunKey(runKey: string): string {
  if (runKey.length <= 12) return runKey;
  return `${runKey.slice(0, 8)}…`;
}
