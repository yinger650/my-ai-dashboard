import type { ActiveRun } from "../types";

const ACTIVE_STATUSES = new Set(["queued", "running", "waiting_input", "blocked"]);

export function isActiveRunStatus(status: string): boolean {
  return ACTIVE_STATUSES.has(status);
}

export function formatActiveRunLine(r: Pick<ActiveRun, "service_name" | "summary" | "status">): string {
  const detail = r.summary.trim() || r.status;
  return `${r.service_name} · ${detail}`;
}
