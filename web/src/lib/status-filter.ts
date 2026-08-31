import type { StatusItem } from "../types";
import { textStatuses } from "./board-metrics";

/** Where a status line is rendered. */
export type StatusSurface = "card" | "machine" | "service";

/** Heartbeat metadata already covered by service.state / last_seen_at. */
const TELEMETRY_KEYS = new Set(["alive", "provider", "last_heartbeat"]);

/**
 * Keys that restate another surface (service summary, port table, client internals).
 * Hidden on machine overview unless severity is warning/error.
 */
const MACHINE_DUP_KEYS = new Set([
  "workspace",
  "uptime",
  "spool_queue",
  "running",
  "stopped",
  "images",
  "jobs",
  "proxies",
  "probe",
  "http_status",
  "latency_ms",
  "ssl_days",
]);

function isListenKey(key: string): boolean {
  return key.startsWith("listen_");
}

export function isAbnormalStatus(st: Pick<StatusItem, "severity">): boolean {
  return st.severity === "warning" || st.severity === "error";
}

export function isTelemetryStatusKey(key: string): boolean {
  return TELEMETRY_KEYS.has(key);
}

/**
 * Keep only statuses a person would act on.
 *
 * - card / text board: warning or error (never telemetry)
 * - machine overview: drop telemetry, listen_*, and keys already shown as
 *   service summary / port table / CPU tiles; keep custom keys and alerts
 * - service detail: drop telemetry only (HTTP probe numbers belong here)
 */
export function isUserFacingStatus(st: StatusItem, surface: StatusSurface): boolean {
  if (isTelemetryStatusKey(st.status_key)) return false;
  if (surface === "service") return true;
  if (surface === "card") return isAbnormalStatus(st);
  if (isListenKey(st.status_key)) return false;
  if (MACHINE_DUP_KEYS.has(st.status_key)) return isAbnormalStatus(st);
  return true;
}

export function userFacingStatuses(
  statuses: StatusItem[] | null | undefined,
  surface: StatusSurface,
): StatusItem[] {
  return textStatuses(statuses).filter((st) => isUserFacingStatus(st, surface));
}
