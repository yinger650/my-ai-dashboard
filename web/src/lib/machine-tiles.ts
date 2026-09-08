import type { StatusItem } from "../types";
import { isTelemetryStatusKey } from "./status-filter";
import type { PercentMetric } from "./board-metrics";
import { formatStatusValue, isPercentStatus, parseStatusNumber } from "./board-metrics";

export const MAX_MACHINE_TILES = 12;
export const MACHINE_TILES_STORAGE_KEY = "abp.machine-tiles";

export type DisplayTile = {
  id: string;
  label: string;
  severity?: string;
  removable: boolean;
} & ({ kind: "percent"; value: number } | { kind: "status"; text: string });

function store(storage?: Storage | null): Storage | null {
  try {
    return storage ?? (typeof localStorage === "undefined" ? null : localStorage);
  } catch {
    return null;
  }
}

export function statusTileKey(st: Pick<StatusItem, "status_key" | "service_id" | "service_key">): string {
  return `${st.service_id || st.service_key || "_"}::${st.status_key}`;
}

export function readMachineExtraTiles(machineId: string | undefined, storage?: Storage | null): string[] {
  if (!machineId) return [];
  try {
    const raw = store(storage)?.getItem(MACHINE_TILES_STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return [];
    const row = (parsed as Record<string, unknown>)[machineId];
    if (!Array.isArray(row)) return [];
    return row.filter((k): k is string => typeof k === "string" && k.includes("::"));
  } catch {
    return [];
  }
}

export function writeMachineExtraTiles(
  machineId: string,
  keys: string[],
  storage?: Storage | null,
): void {
  try {
    const s = store(storage);
    if (!s) return;
    let all: Record<string, string[]> = {};
    try {
      const raw = s.getItem(MACHINE_TILES_STORAGE_KEY);
      const parsed = raw ? (JSON.parse(raw) as unknown) : {};
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        all = parsed as Record<string, string[]>;
      }
    } catch {
      all = {};
    }
    all[machineId] = keys;
    s.setItem(MACHINE_TILES_STORAGE_KEY, JSON.stringify(all));
  } catch {
    // private mode / disabled storage
  }
}

export function addExtraTile(existing: string[], id: string, percentCount: number): string[] {
  if (!id || existing.includes(id)) return existing;
  if (percentCount + existing.length >= MAX_MACHINE_TILES) return existing;
  return [...existing, id];
}

export function removeExtraTile(existing: string[], id: string): string[] {
  return existing.filter((k) => k !== id);
}

function statusToTile(st: StatusItem, id: string): DisplayTile {
  if (isPercentStatus(st)) {
    const n = parseStatusNumber(st);
    if (n != null) {
      return {
        kind: "percent",
        id,
        label: st.label || st.status_key,
        value: n,
        severity: st.severity,
        removable: true,
      };
    }
  }
  const unit = st.unit ? ` ${st.unit}` : "";
  return {
    kind: "status",
    id,
    label: st.label || st.status_key,
    text: `${formatStatusValue(st)}${unit}`,
    severity: st.severity,
    removable: true,
  };
}

export function buildMachineTiles(
  percents: PercentMetric[],
  extras: string[],
  statuses: StatusItem[] | null | undefined,
): { tiles: DisplayTile[]; canAdd: boolean } {
  const tiles: DisplayTile[] = [];
  const seen = new Set<string>();
  for (const p of percents) {
    if (tiles.length >= MAX_MACHINE_TILES) break;
    if (seen.has(p.key)) continue;
    seen.add(p.key);
    tiles.push({
      kind: "percent",
      id: p.key,
      label: p.label,
      value: p.value,
      severity: p.severity,
      removable: false,
    });
  }
  const byKey = new Map((statuses ?? []).map((st) => [statusTileKey(st), st]));
  for (const id of extras) {
    if (tiles.length >= MAX_MACHINE_TILES) break;
    if (seen.has(id)) continue;
    seen.add(id);
    const st = byKey.get(id);
    if (st) {
      tiles.push(statusToTile(st, id));
    } else {
      const label = id.split("::")[1] || id;
      tiles.push({ kind: "status", id, label, text: "--", removable: true });
    }
  }
  return { tiles, canAdd: tiles.length < MAX_MACHINE_TILES };
}

/** Status-log rows that can still be pinned as extra tiles. */
export function availableStatusPicks(statuses: StatusItem[], tiles: DisplayTile[]): StatusItem[] {
  const occupiedIds = new Set(tiles.map((t) => t.id));
  const occupiedPercentKeys = new Set(tiles.filter((t) => !t.removable).map((t) => t.id));
  return statuses.filter((st) => {
    if (isTelemetryStatusKey(st.status_key)) return false;
    const id = statusTileKey(st);
    if (occupiedIds.has(id)) return false;
    if (occupiedPercentKeys.has(st.status_key)) return false;
    return true;
  });
}
