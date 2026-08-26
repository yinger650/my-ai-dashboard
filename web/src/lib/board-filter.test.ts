import { describe, expect, it } from "vitest";
import {
  countMachineHealth,
  filterBoardMachines,
  isOfflineHealth,
  readShowOfflinePreference,
  SHOW_OFFLINE_STORAGE_KEY,
  writeShowOfflinePreference,
} from "./board-filter";

const machines = [
  { name: "家庭 NAS", machine_key: "home-nas", health: "online" },
  { name: "旧笔记本", machine_key: "old-laptop", health: "offline" },
  { name: "构建机", machine_key: "builder", health: "stale" },
  { name: "新机器", machine_key: "new-box", health: "unknown" },
];

describe("isOfflineHealth", () => {
  it("only treats offline as offline", () => {
    expect(isOfflineHealth("offline")).toBe(true);
    expect(isOfflineHealth("stale")).toBe(false);
    expect(isOfflineHealth("unknown")).toBe(false);
    expect(isOfflineHealth("online")).toBe(false);
  });
});

describe("filterBoardMachines", () => {
  it("shows every machine by default", () => {
    expect(filterBoardMachines(machines).map((m) => m.machine_key)).toEqual([
      "home-nas",
      "old-laptop",
      "builder",
      "new-box",
    ]);
  });

  it("hides offline machines when showOffline is false", () => {
    expect(filterBoardMachines(machines, { showOffline: false }).map((m) => m.machine_key)).toEqual([
      "home-nas",
      "builder",
      "new-box",
    ]);
  });

  it("applies search after the offline filter", () => {
    expect(
      filterBoardMachines(machines, { search: "nas", showOffline: false }).map((m) => m.machine_key),
    ).toEqual(["home-nas"]);
    expect(filterBoardMachines(machines, { search: "laptop", showOffline: false })).toEqual([]);
    expect(
      filterBoardMachines(machines, { search: "laptop", showOffline: true }).map((m) => m.machine_key),
    ).toEqual(["old-laptop"]);
  });

  it("matches machine_key case-insensitively", () => {
    expect(filterBoardMachines(machines, { search: "HOME-NAS" }).map((m) => m.machine_key)).toEqual([
      "home-nas",
    ]);
  });
});

describe("countMachineHealth", () => {
  it("counts from the full list even when some cards are hidden", () => {
    expect(countMachineHealth(machines)).toEqual({ online: 1, degraded: 1, offline: 1 });
    expect(countMachineHealth([{ health: "degraded" }, { health: "stale" }])).toEqual({
      online: 0,
      degraded: 2,
      offline: 0,
    });
  });
});

describe("show-offline preference", () => {
  it("defaults to showing offline machines", () => {
    const mem = memoryStorage();
    expect(readShowOfflinePreference(mem)).toBe(true);
  });

  it("round-trips through storage", () => {
    const mem = memoryStorage();
    writeShowOfflinePreference(false, mem);
    expect(mem.getItem(SHOW_OFFLINE_STORAGE_KEY)).toBe("0");
    expect(readShowOfflinePreference(mem)).toBe(false);
    writeShowOfflinePreference(true, mem);
    expect(readShowOfflinePreference(mem)).toBe(true);
  });
});

function memoryStorage(): Storage {
  const data = new Map<string, string>();
  return {
    get length() {
      return data.size;
    },
    clear: () => data.clear(),
    getItem: (k) => (data.has(k) ? data.get(k)! : null),
    key: (i) => [...data.keys()][i] ?? null,
    removeItem: (k) => {
      data.delete(k);
    },
    setItem: (k, v) => {
      data.set(k, String(v));
    },
  };
}
