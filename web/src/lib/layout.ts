import type { Layout, LayoutItem } from "react-grid-layout";

export const GRID_COLS = 12;
export const GRID_ROW_HEIGHT = 28;
export const GRID_MARGIN: [number, number] = [12, 12];
export const DEFAULT_W = 4;
export const DEFAULT_H = 16;
export const MIN_W = 3;
export const MIN_H = 11;
export const MOBILE_BREAKPOINT = 768;

export function defaultItem(id: string, index: number, startY = 0): LayoutItem {
  return {
    i: id,
    x: (index % 3) * DEFAULT_W,
    y: startY + Math.floor(index / 3) * DEFAULT_H,
    w: DEFAULT_W,
    h: DEFAULT_H,
    minW: MIN_W,
    minH: MIN_H,
  };
}

export function defaultLayout(ids: string[]): Layout {
  return ids.map((id, i) => defaultItem(id, i));
}

/** Keep saved positions for known machines; append new ones below the current grid. */
export function reconcileLayout(saved: Layout | null | undefined, ids: string[]): Layout {
  const byId = new Map((saved ?? []).map((item) => [item.i, item]));
  const kept: LayoutItem[] = [];
  for (const id of ids) {
    const existing = byId.get(id);
    if (existing) {
      kept.push({
        ...existing,
        i: id,
        minW: MIN_W,
        minH: MIN_H,
      });
    }
  }
  const missing = ids.filter((id) => !byId.has(id));
  if (missing.length === 0) return kept;
  let maxY = 0;
  for (const item of kept) {
    maxY = Math.max(maxY, item.y + item.h);
  }
  return [...kept, ...missing.map((id, i) => defaultItem(id, i, maxY))];
}
