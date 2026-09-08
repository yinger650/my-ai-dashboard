import { describe, expect, it } from "vitest";
import type { StatusItem } from "../types";
import {
  MAX_MACHINE_TILES,
  addExtraTile,
  availableStatusPicks,
  buildMachineTiles,
  readMachineExtraTiles,
  removeExtraTile,
  statusTileKey,
  writeMachineExtraTiles,
} from "./machine-tiles";

function st(partial: Partial<StatusItem> & Pick<StatusItem, "status_key" | "label" | "value_json">): StatusItem {
  return {
    value_type: "string",
    unit: null,
    severity: "normal",
    display_format: "text",
    sort_order: 10,
    service_id: "s1",
    service_key: "openclaw",
    service_name: "OpenClaw",
    ...partial,
  };
}

describe("machine extra tiles", () => {
  it("builds a unique composite key", () => {
    expect(statusTileKey(st({ status_key: "model", label: "模型", value_json: '"q"' }))).toBe("s1::model");
  });

  it("caps at 12 and keeps default percent tiles unremovable", () => {
    const percents = Array.from({ length: 3 }, (_, i) => ({
      key: `cpu${i}`,
      label: "CPU",
      value: i,
    }));
    const extras = ["s1::model", "s1::queue"];
    const { tiles, canAdd } = buildMachineTiles(percents, extras, [
      st({ status_key: "model", label: "模型", value_json: '"llama"' }),
      st({ status_key: "queue", label: "队列", value_json: '"blocked"', severity: "warning" }),
    ]);
    expect(tiles).toHaveLength(5);
    expect(tiles[0]).toMatchObject({ id: "cpu0", removable: false, kind: "percent" });
    expect(tiles[3]).toMatchObject({ id: "s1::model", removable: true, kind: "status", text: "llama" });
    expect(tiles[4]).toMatchObject({ text: "blocked", severity: "warning" });
    expect(canAdd).toBe(true);
  });

  it("refuses extra tiles beyond the 12 slot cap", () => {
    const percents = Array.from({ length: 11 }, (_, i) => ({ key: `p${i}`, label: "P", value: i }));
    const next = addExtraTile(["s1::a"], "s1::b", percents.length);
    expect(next).toEqual(["s1::a"]);
    const { tiles, canAdd } = buildMachineTiles(
      percents,
      ["s1::a", "s1::b"],
      [st({ status_key: "model", label: "模型", value_json: '"x"', service_id: "s1" })],
    );
    expect(tiles).toHaveLength(MAX_MACHINE_TILES);
    expect(canAdd).toBe(false);
  });

  it("persists extras per machine", () => {
    const mem = new Map<string, string>();
    const storage = {
      getItem: (k: string) => mem.get(k) ?? null,
      setItem: (k: string, v: string) => {
        mem.set(k, v);
      },
      removeItem: (k: string) => {
        mem.delete(k);
      },
      clear: () => mem.clear(),
      key: () => null,
      length: 0,
    } as Storage;
    writeMachineExtraTiles("m1", ["s1::model"], storage);
    writeMachineExtraTiles("m2", ["s2::q"], storage);
    expect(readMachineExtraTiles("m1", storage)).toEqual(["s1::model"]);
    expect(readMachineExtraTiles("m2", storage)).toEqual(["s2::q"]);
    expect(removeExtraTile(["s1::model", "s1::q"], "s1::model")).toEqual(["s1::q"]);
  });

  it("omits already shown rows from the picker", () => {
    const { tiles } = buildMachineTiles(
      [{ key: "cpu", label: "CPU", value: 1 }],
      ["s1::model"],
      [
        st({ status_key: "model", label: "模型", value_json: '"llama"' }),
        st({ status_key: "queue", label: "队列", value_json: '"ok"' }),
        st({ status_key: "cpu", label: "CPU", value_json: "1", value_type: "number", unit: "%" }),
        st({ status_key: "alive", label: "存活", value_json: "true" }),
      ],
    );
    const picks = availableStatusPicks(
      [
        st({ status_key: "model", label: "模型", value_json: '"llama"' }),
        st({ status_key: "queue", label: "队列", value_json: '"ok"' }),
        st({ status_key: "alive", label: "存活", value_json: "true" }),
      ],
      tiles,
    );
    expect(picks.map((p) => p.status_key)).toEqual(["queue"]);
  });
});
