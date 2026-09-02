import { describe, expect, it } from "vitest";
import {
  collectPercentMetrics,
  formatStatusValue,
  isPercentStatus,
  textStatuses,
} from "./board-metrics";
import type { StatusItem } from "../types";

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

describe("isPercentStatus", () => {
  it("treats % / progress as percent and strings as text", () => {
    expect(
      isPercentStatus(st({ status_key: "gpu_mem", label: "显存", value_json: "72.5", value_type: "number", unit: "%" })),
    ).toBe(true);
    expect(
      isPercentStatus(st({ status_key: "gpu_util", label: "GPU 利用率", value_json: "40", value_type: "number" })),
    ).toBe(true);
    expect(
      isPercentStatus(st({ status_key: "alive", label: "存活", value_json: "true", value_type: "boolean" })),
    ).toBe(false);
    expect(
      isPercentStatus(st({ status_key: "latency_ms", label: "响应时间", value_json: "45", value_type: "number", unit: "ms", display_format: "number" })),
    ).toBe(false);
  });
});

describe("collectPercentMetrics", () => {
  it("skips empty host placeholders and keeps extra percents", () => {
    const rows = collectPercentMetrics({
      latest_metric: null,
      heartbeat_metrics: { gpu_mem: 64, gpu_util: 81 },
      statuses: [
        st({ status_key: "alive", label: "存活", value_json: "true", value_type: "boolean" }),
        st({ status_key: "vram", label: "显存利用率", value_json: "50", value_type: "number", unit: "%" }),
      ],
    });
    expect(rows.map((r) => r.key)).toEqual(["gpu_mem", "gpu_util", "vram"]);
    expect(textStatuses([
      st({ status_key: "alive", label: "存活", value_json: "true", value_type: "boolean" }),
      st({ status_key: "vram", label: "显存利用率", value_json: "50", value_type: "number", unit: "%" }),
    ]).map((s) => s.status_key)).toEqual(["alive"]);
  });

  it("labels data_dir as 目录占用", () => {
    const rows = collectPercentMetrics({ heartbeat_metrics: { data_dir: 41 } });
    expect(rows).toEqual([{ key: "data_dir", label: "目录占用", value: 41 }]);
  });

  it("includes host cpu/mem/disk only when present", () => {
    const rows = collectPercentMetrics({
      latest_metric: {
        occurred_at: "",
        cpu_percent: 3.6,
        load1: null,
        memory_used_bytes: 347,
        memory_total_bytes: 1000,
        swap_used_bytes: null,
        swap_total_bytes: null,
        disk_read_bps: null,
        disk_write_bps: null,
        network_rx_bps: 1,
        network_tx_bps: 2,
        root_disk_used_bytes: null,
        root_disk_total_bytes: null,
      },
    });
    expect(rows.map((r) => r.label)).toEqual(["CPU", "内存"]);
  });
});

describe("formatStatusValue", () => {
  it("renders booleans in Chinese", () => {
    expect(formatStatusValue(st({ status_key: "alive", label: "存活", value_json: "true" }))).toBe("是");
    expect(formatStatusValue(st({ status_key: "alive", label: "存活", value_json: "false" }))).toBe("否");
  });
});
