import { describe, expect, it } from "vitest";
import { compactCardPins, compactCardServices, isCardNoiseLog } from "./board-card";
import type { BoardService, LogEntry, PinnedLog } from "../types";

function svc(key: string, name = key): BoardService {
  return {
    id: key,
    service_key: key,
    name,
    type: "daemon",
    current_state: "running",
    state_summary: "ok",
    severity: "normal",
    last_seen_at: null,
  };
}

describe("compactCardServices", () => {
  it("drops .service duplicates and orders known keys first", () => {
    const rows = compactCardServices([
      svc("nginx.service", "nginx.service"),
      svc("cursor-agent", "Cursor Agent"),
      svc("nginx", "Nginx"),
      svc("docker", "Docker"),
    ]);
    expect(rows.map((s) => s.service_key)).toEqual(["nginx", "docker", "cursor-agent"]);
  });
});

describe("card log filters", () => {
  it("hides cron rolling logs", () => {
    expect(isCardNoiseLog({ event_id: "1", markdown: "x", severity: "info", occurred_at: "", source: "cron" })).toBe(true);
    expect(isCardNoiseLog({ event_id: "2", markdown: "x", severity: "info", occurred_at: "", service_key: "cron" } as LogEntry)).toBe(true);
    expect(isCardNoiseLog({ event_id: "3", markdown: "x", severity: "info", occurred_at: "", source: "nginx" })).toBe(false);
  });

  it("keeps current-state pins only", () => {
    const pins: PinnedLog[] = [
      { markdown: "listen", severity: "info", occurred_at: "", service_key: "host-listen" },
      { markdown: "agent", severity: "info", occurred_at: "", service_key: "cursor-agent" },
    ];
    expect(compactCardPins(pins).map((p) => p.service_key)).toEqual(["host-listen"]);
  });
});
