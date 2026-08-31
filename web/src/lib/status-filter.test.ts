import { describe, expect, it } from "vitest";
import type { StatusItem } from "../types";
import { isUserFacingStatus, userFacingStatuses } from "./status-filter";

function st(partial: Partial<StatusItem> & Pick<StatusItem, "status_key" | "label" | "value_json">): StatusItem {
  return {
    value_type: "string",
    unit: null,
    severity: "normal",
    display_format: "text",
    sort_order: 10,
    ...partial,
  };
}

describe("isUserFacingStatus", () => {
  it("never shows heartbeat telemetry", () => {
    for (const key of ["alive", "provider", "last_heartbeat"] as const) {
      const row = st({
        status_key: key,
        label: key,
        value_json: '"x"',
        severity: "error",
      });
      expect(isUserFacingStatus(row, "card")).toBe(false);
      expect(isUserFacingStatus(row, "machine")).toBe(false);
      expect(isUserFacingStatus(row, "service")).toBe(false);
    }
  });

  it("card keeps only warning/error operational rows", () => {
    expect(
      isUserFacingStatus(st({ status_key: "probe", label: "探测", value_json: '"up"' }), "card"),
    ).toBe(false);
    expect(
      isUserFacingStatus(
        st({ status_key: "probe", label: "探测", value_json: '"down"', severity: "error" }),
        "card",
      ),
    ).toBe(true);
    expect(
      isUserFacingStatus(st({ status_key: "queue", label: "队列", value_json: "4" }), "card"),
    ).toBe(false);
    expect(
      isUserFacingStatus(
        st({ status_key: "queue", label: "队列", value_json: "4", severity: "warning" }),
        "card",
      ),
    ).toBe(true);
  });

  it("machine hides listen ports and duplicate summaries unless abnormal", () => {
    expect(
      isUserFacingStatus(
        st({ status_key: "listen_80", label: "监听 80", value_json: '"0.0.0.0:80/tcp"' }),
        "machine",
      ),
    ).toBe(false);
    expect(
      isUserFacingStatus(st({ status_key: "running", label: "运行中", value_json: "3" }), "machine"),
    ).toBe(false);
    expect(
      isUserFacingStatus(
        st({ status_key: "spool_queue", label: "队列", value_json: "12", severity: "warning" }),
        "machine",
      ),
    ).toBe(true);
    expect(
      isUserFacingStatus(st({ status_key: "model", label: "模型", value_json: '"llama"' }), "machine"),
    ).toBe(true);
  });

  it("service keeps workspace and HTTP probe numbers", () => {
    expect(
      isUserFacingStatus(
        st({ status_key: "workspace", label: "目录", value_json: '"/tmp/proj"' }),
        "service",
      ),
    ).toBe(true);
    expect(
      isUserFacingStatus(
        st({ status_key: "latency_ms", label: "响应时间", value_json: "45", value_type: "number" }),
        "service",
      ),
    ).toBe(true);
  });
});

describe("userFacingStatuses", () => {
  it("drops percent tiles and telemetry from a mixed list", () => {
    const rows = userFacingStatuses(
      [
        st({ status_key: "alive", label: "存活", value_json: "true", value_type: "boolean" }),
        st({ status_key: "gpu_mem", label: "显存", value_json: "72", value_type: "number", unit: "%" }),
        st({ status_key: "ssl_days", label: "证书", value_json: "5", severity: "error", value_type: "number" }),
      ],
      "card",
    );
    expect(rows.map((s) => s.status_key)).toEqual(["ssl_days"]);
  });
});
