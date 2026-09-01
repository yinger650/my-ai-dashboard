const STORAGE_KEY = "abp.log-seen";

export { STORAGE_KEY as LOG_SEEN_STORAGE_KEY };

function store(storage?: Storage | null): Storage | null {
  try {
    return storage ?? (typeof localStorage === "undefined" ? null : localStorage);
  } catch {
    return null;
  }
}

/** Map of service_id / service_key -> ISO time; logs at or before this are read. */
export function readLogSeen(storage?: Storage | null): Record<string, string> {
  try {
    const raw = store(storage)?.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
      if (typeof v === "string" && v) out[k] = v;
    }
    return out;
  } catch {
    return {};
  }
}

export function writeLogSeen(seen: Record<string, string>, storage?: Storage | null): void {
  try {
    store(storage)?.setItem(STORAGE_KEY, JSON.stringify(seen));
  } catch {
    // private mode / disabled storage
  }
}

export function markServiceLogsSeen(
  keys: Array<string | null | undefined>,
  at: string = new Date().toISOString(),
  storage?: Storage | null,
): Record<string, string> {
  const seen = readLogSeen(storage);
  for (const key of keys) {
    if (key) seen[key] = at;
  }
  writeLogSeen(seen, storage);
  return seen;
}

export function logSeenWatermark(
  seen: Record<string, string>,
  serviceId?: string | null,
  serviceKey?: string | null,
): string | undefined {
  const a = serviceId ? seen[serviceId] : undefined;
  const b = serviceKey ? seen[serviceKey] : undefined;
  if (a && b) return a >= b ? a : b;
  return a ?? b;
}

export function isLogUnseen(
  occurredAt: string,
  seen: Record<string, string>,
  serviceId?: string | null,
  serviceKey?: string | null,
): boolean {
  const wm = logSeenWatermark(seen, serviceId, serviceKey);
  if (!wm) return true;
  const logMs = Date.parse(occurredAt);
  const seenMs = Date.parse(wm);
  if (Number.isFinite(logMs) && Number.isFinite(seenMs)) return logMs > seenMs;
  return occurredAt > wm;
}
