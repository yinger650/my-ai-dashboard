const SHOW_OFFLINE_STORAGE_KEY = "abp.show-offline";

export { SHOW_OFFLINE_STORAGE_KEY };

export interface BoardFilterable {
  name: string;
  machine_key: string;
  health: string;
}

export function isOfflineHealth(health: string): boolean {
  return health === "offline";
}

export function filterBoardMachines<T extends BoardFilterable>(
  machines: T[],
  opts: { search?: string; showOffline?: boolean } = {},
): T[] {
  const q = (opts.search ?? "").trim().toLowerCase();
  const showOffline = opts.showOffline !== false;
  return machines.filter((m) => {
    if (!showOffline && isOfflineHealth(m.health)) return false;
    if (!q) return true;
    return m.name.toLowerCase().includes(q) || m.machine_key.toLowerCase().includes(q);
  });
}

export function countMachineHealth(machines: { health: string }[]): {
  online: number;
  degraded: number;
  offline: number;
} {
  const counts = { online: 0, degraded: 0, offline: 0 };
  for (const m of machines) {
    if (m.health === "online") counts.online++;
    else if (m.health === "degraded" || m.health === "stale") counts.degraded++;
    else if (m.health === "offline") counts.offline++;
  }
  return counts;
}

export function readShowOfflinePreference(storage?: Storage | null): boolean {
  try {
    const store = storage ?? (typeof localStorage === "undefined" ? null : localStorage);
    const raw = store?.getItem(SHOW_OFFLINE_STORAGE_KEY);
    if (raw == null) return true;
    return raw === "1" || raw === "true";
  } catch {
    return true;
  }
}

export function writeShowOfflinePreference(show: boolean, storage?: Storage | null): void {
  try {
    const store = storage ?? (typeof localStorage === "undefined" ? null : localStorage);
    store?.setItem(SHOW_OFFLINE_STORAGE_KEY, show ? "1" : "0");
  } catch {
    // private mode / disabled storage
  }
}
