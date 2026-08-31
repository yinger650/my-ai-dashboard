/** Host machines run board-client and collect OS daemons. Virtual machines are agent-only. */
const HOST_KINDS = new Set(["physical", "vm", "container_host"]);

/** Max lag of a service last_seen behind the machine last_seen (covers cursor-agent at 5m). */
export const HOST_SERVICE_STALE_MS = 15 * 60 * 1000;

const LIVE_STATES = new Set(["running", "failed", "starting", "stopping", "stale", "alive"]);

export interface HostServiceRow {
  service_key: string;
  current_state: string;
  last_seen_at: string | null;
}

export function isHostMachineKind(kind: string | null | undefined): boolean {
  return !!kind && HOST_KINDS.has(kind);
}

export function dropServiceSuffixDupes<T extends { service_key: string }>(services: T[]): T[] {
  const byKey = new Map(services.map((s) => [s.service_key, s]));
  return services.filter((s) => {
    if (!s.service_key.endsWith(".service")) return true;
    const bare = s.service_key.slice(0, -".service".length);
    return !byKey.has(bare);
  });
}

function parseTime(iso: string | null | undefined): number | null {
  if (!iso) return null;
  const ms = Date.parse(iso);
  return Number.isFinite(ms) ? ms : null;
}

function isFreshlyCollected(lastSeenAt: string | null, machineLastSeenAt: string | null | undefined): boolean {
  const serviceAt = parseTime(lastSeenAt);
  if (serviceAt == null) return false;
  const machineAt = parseTime(machineLastSeenAt);
  if (machineAt == null) return true;
  return machineAt - serviceAt <= HOST_SERVICE_STALE_MS;
}

/**
 * For host machines, keep services that are still being collected and look live.
 * Virtual / unknown kinds are returned unchanged.
 */
export function visibleHostServices<T extends HostServiceRow>(
  services: T[],
  opts: { kind?: string | null; machineLastSeenAt?: string | null },
): T[] {
  if (!isHostMachineKind(opts.kind)) return services;
  const live = services.filter((s) => {
    if (!isFreshlyCollected(s.last_seen_at, opts.machineLastSeenAt)) return false;
    return LIVE_STATES.has(s.current_state);
  });
  return dropServiceSuffixDupes(live);
}
