import { useMemo } from "react";
import GridLayout, { noCompactor, useContainerWidth, type Layout } from "react-grid-layout";
import { GridBackground } from "react-grid-layout/extras";
import "react-grid-layout/css/styles.css";
import type { BoardMachine } from "../types";
import { MachineCard } from "./MachineCard";
import {
  GRID_COLS,
  GRID_MARGIN,
  GRID_ROW_HEIGHT,
  MOBILE_BREAKPOINT,
  reconcileLayout,
} from "../lib/layout";
import { useMediaQuery } from "../hooks/useMediaQuery";
import { cn } from "../lib/utils";

export function BoardGrid({
  machines,
  savedLayout,
  editMode,
  autoRefresh,
  pollMs,
  onLayoutChange,
}: {
  machines: BoardMachine[];
  savedLayout: Layout | null;
  editMode: boolean;
  autoRefresh: boolean;
  pollMs: number;
  onLayoutChange: (layout: Layout) => void;
}) {
  const isMobile = useMediaQuery(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`);
  const { width, containerRef, mounted } = useContainerWidth();
  const ids = machines.map((m) => m.id);
  const layout = useMemo(() => reconcileLayout(savedLayout, ids), [savedLayout, ids.join(",")]);

  function persist(next: Layout) {
    if (!editMode) return;
    const visible = new Set(ids);
    const rest = (savedLayout ?? []).filter((item) => !visible.has(item.i));
    onLayoutChange([...next, ...rest]);
  }

  if (isMobile) {
    return (
      <div className="flex flex-col gap-4">
        {machines.map((m) => (
          <div key={m.id} className="h-[540px]">
            <MachineCard m={m} autoRefresh={autoRefresh} pollMs={pollMs} />
          </div>
        ))}
      </div>
    );
  }

  return (
    <div ref={containerRef} className={cn("relative min-h-[200px]", editMode && "rounded-xl bg-slate-950/40")}>
      {mounted && width > 0 && (
        <>
          {editMode && (
            <GridBackground
              width={width}
              cols={GRID_COLS}
              rowHeight={GRID_ROW_HEIGHT}
              margin={GRID_MARGIN}
              rows="auto"
              height={Math.max(400, layout.reduce((h, i) => Math.max(h, (i.y + i.h) * (GRID_ROW_HEIGHT + GRID_MARGIN[1])), 0))}
              color="rgba(99,102,241,0.07)"
              borderRadius={8}
              className="pointer-events-none absolute inset-0"
            />
          )}
          <GridLayout
            width={width}
            layout={layout}
            gridConfig={{ cols: GRID_COLS, rowHeight: GRID_ROW_HEIGHT, margin: GRID_MARGIN }}
            dragConfig={{ enabled: editMode, handle: ".drag-handle", bounded: true }}
            resizeConfig={{ enabled: editMode, handles: ["se", "e", "s"] }}
            compactor={{ ...noCompactor, preventCollision: true }}
            onDragStop={(next) => persist(next)}
            onResizeStop={(next) => persist(next)}
            className="relative"
          >
            {machines.map((m) => (
              <div key={m.id} className="h-full">
                <MachineCard m={m} autoRefresh={autoRefresh} pollMs={pollMs} editMode={editMode} />
              </div>
            ))}
          </GridLayout>
        </>
      )}
    </div>
  );
}
