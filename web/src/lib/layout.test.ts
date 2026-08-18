import { describe, expect, it } from "vitest";
import { defaultLayout, reconcileLayout, DEFAULT_H, DEFAULT_W } from "./layout";

describe("defaultLayout", () => {
  it("places three cards on the first row", () => {
    const layout = defaultLayout(["a", "b", "c"]);
    expect(layout).toHaveLength(3);
    expect(layout[0]).toMatchObject({ i: "a", x: 0, y: 0, w: DEFAULT_W });
    expect(layout[1]).toMatchObject({ i: "b", x: DEFAULT_W, y: 0 });
    expect(layout[2]).toMatchObject({ i: "c", x: DEFAULT_W * 2, y: 0 });
  });

  it("wraps the fourth card to the next row", () => {
    const layout = defaultLayout(["a", "b", "c", "d"]);
    expect(layout[3]).toMatchObject({ i: "d", x: 0, y: DEFAULT_H });
  });
});

describe("reconcileLayout", () => {
  it("keeps saved positions and appends new machines below", () => {
    const saved = [{ i: "a", x: 2, y: 4, w: 6, h: 10 }];
    const next = reconcileLayout(saved, ["a", "b"]);
    expect(next[0]).toMatchObject({ i: "a", x: 2, y: 4, w: 6, h: 10 });
    expect(next[1]).toMatchObject({ i: "b", x: 0, y: 14 });
  });

  it("drops machines that no longer exist", () => {
    const saved = [
      { i: "gone", x: 0, y: 0, w: 4, h: 8 },
      { i: "keep", x: 4, y: 0, w: 4, h: 8 },
    ];
    const next = reconcileLayout(saved, ["keep"]);
    expect(next.map((i) => i.i)).toEqual(["keep"]);
  });
});
