import { describe, expect, it } from "vitest";
import type { LogEntry } from "../types";
import { filterLogsByRuns, toggleRunKey } from "./run-logs";

function log(partial: Pick<LogEntry, "event_id" | "markdown"> & Partial<LogEntry>): LogEntry {
  return {
    severity: "info",
    occurred_at: "2026-09-02T12:00:00.000Z",
    ...partial,
  };
}

const a = log({ event_id: "1", markdown: "A", run_key: "run-a" });
const b = log({ event_id: "2", markdown: "B", run_key: "run-b" });
const unbound = log({ event_id: "3", markdown: "service" });

describe("filterLogsByRuns", () => {
  it("shows every log when nothing is selected", () => {
    expect(filterLogsByRuns([a, b, unbound], [])).toEqual([a, b, unbound]);
  });

  it("keeps only selected run keys and drops unbound logs", () => {
    expect(filterLogsByRuns([a, b, unbound], ["run-a"]).map((l) => l.event_id)).toEqual(["1"]);
    expect(filterLogsByRuns([a, b, unbound], ["run-a", "run-b"]).map((l) => l.event_id)).toEqual(["1", "2"]);
  });
});

describe("toggleRunKey", () => {
  it("adds then removes a run key", () => {
    expect(toggleRunKey([], "run-a")).toEqual(["run-a"]);
    expect(toggleRunKey(["run-a"], "run-a")).toEqual([]);
    expect(toggleRunKey(["run-a"], "run-b")).toEqual(["run-a", "run-b"]);
  });
});
