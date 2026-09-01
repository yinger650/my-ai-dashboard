import { describe, expect, it } from "vitest";
import { formatActiveRunLine, isActiveRunStatus } from "./active-runs";

describe("active runs", () => {
  it("treats queued/running/waiting/blocked as active", () => {
    expect(isActiveRunStatus("running")).toBe(true);
    expect(isActiveRunStatus("queued")).toBe(true);
    expect(isActiveRunStatus("waiting_input")).toBe(true);
    expect(isActiveRunStatus("blocked")).toBe(true);
    expect(isActiveRunStatus("succeeded")).toBe(false);
    expect(isActiveRunStatus("failed")).toBe(false);
  });

  it("formats service plus summary", () => {
    expect(
      formatActiveRunLine({ service_name: "Cursor Agent", summary: "实现 M7", status: "running" }),
    ).toBe("Cursor Agent · 实现 M7");
    expect(formatActiveRunLine({ service_name: "demo", summary: "  ", status: "queued" })).toBe("demo · queued");
  });
});
