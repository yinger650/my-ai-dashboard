import { describe, expect, it } from "vitest";
import {
  LOG_SEEN_STORAGE_KEY,
  isLogUnseen,
  markServiceLogsSeen,
  readLogSeen,
} from "./log-seen";
import { countUnseenLogsByService } from "./logs";
import type { LogEntry } from "../types";

function memStore(): Storage {
  const data = new Map<string, string>();
  return {
    get length() {
      return data.size;
    },
    clear: () => data.clear(),
    getItem: (k) => data.get(k) ?? null,
    key: (i) => [...data.keys()][i] ?? null,
    removeItem: (k) => {
      data.delete(k);
    },
    setItem: (k, v) => {
      data.set(k, v);
    },
  };
}

function log(id: string, at: string, svc: { service_id?: string; service_key?: string }): LogEntry {
  return { event_id: id, markdown: id, severity: "info", occurred_at: at, ...svc };
}

describe("log seen watermarks", () => {
  it("persists marks and treats older logs as read", () => {
    const storage = memStore();
    markServiceLogsSeen(["s-nginx", "nginx"], "2026-08-28T12:00:00.000Z", storage);
    expect(readLogSeen(storage)).toEqual({ "s-nginx": "2026-08-28T12:00:00.000Z", nginx: "2026-08-28T12:00:00.000Z" });
    expect(storage.getItem(LOG_SEEN_STORAGE_KEY)).toContain("s-nginx");
    expect(isLogUnseen("2026-08-28T11:59:00.000Z", readLogSeen(storage), "s-nginx", "nginx")).toBe(false);
    expect(isLogUnseen("2026-08-28T12:00:01.000Z", readLogSeen(storage), "s-nginx", "nginx")).toBe(true);
    expect(isLogUnseen("2026-08-28T12:00:00Z", { nginx: "2026-08-28T12:00:00.500Z" }, undefined, "nginx")).toBe(false);
  });

  it("keeps unseen when the service was never opened", () => {
    expect(isLogUnseen("2026-08-28T12:00:00.000Z", {}, "s-nginx", "nginx")).toBe(true);
  });
});

describe("countUnseenLogsByService", () => {
  it("drops logs at or before the watermark after opening a service", () => {
    const logs = [
      log("old", "2026-08-28T11:00:00.000Z", { service_id: "s-nginx", service_key: "nginx" }),
      log("mid", "2026-08-28T12:00:00.000Z", { service_id: "s-nginx", service_key: "nginx" }),
      log("new", "2026-08-28T12:01:00.000Z", { service_id: "s-nginx", service_key: "nginx" }),
      log("other", "2026-08-28T12:01:00.000Z", { service_key: "docker" }),
    ];
    const seen = { "s-nginx": "2026-08-28T12:00:00.000Z" };
    const counts = countUnseenLogsByService(logs, seen);
    expect(counts["s-nginx"]).toBe(1);
    expect(counts.nginx).toBe(1);
    expect(counts.docker).toBe(1);
  });
});
