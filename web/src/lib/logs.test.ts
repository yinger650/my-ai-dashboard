import { describe, expect, it } from "vitest";
import { countLogsByService, countNewLogs, mergeLogPages } from "./logs";
import type { LogEntry } from "../types";

function log(id: string, at: string): LogEntry {
  return { event_id: id, markdown: id, severity: "info", occurred_at: at };
}

describe("mergeLogPages", () => {
  it("dedupes by event_id and sorts newest first", () => {
    const merged = mergeLogPages(
      [log("b", "2026-01-01T00:00:02Z"), log("a", "2026-01-01T00:00:01Z")],
      [log("a", "2026-01-01T00:00:01Z"), log("c", "2026-01-01T00:00:00Z")],
    );
    expect(merged.map((l) => l.event_id)).toEqual(["b", "a", "c"]);
  });
});

describe("countNewLogs", () => {
  it("counts incoming ids not already present", () => {
    expect(countNewLogs([log("n1", "2"), log("old", "1")], [log("old", "1")])).toBe(1);
  });
});

describe("countLogsByService", () => {
  it("counts by service id and key without double-counting the same log", () => {
    const counts = countLogsByService([
      { ...log("a", "1"), service_id: "s-nginx", service_key: "nginx" },
      { ...log("b", "2"), service_id: "s-nginx", service_key: "nginx" },
      { ...log("c", "3"), service_key: "docker" },
    ]);
    expect(counts["s-nginx"]).toBe(2);
    expect(counts.nginx).toBe(2);
    expect(counts.docker).toBe(1);
  });
});
