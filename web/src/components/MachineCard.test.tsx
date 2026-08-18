import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { MachineCard } from "./MachineCard";
import type { BoardMachine } from "../types";

vi.mock("./MachineLogStream", () => ({
  MachineLogStream: () => <div data-testid="logs" />,
}));

const machine: BoardMachine = {
  id: "m1",
  machine_key: "box",
  name: "Box",
  kind: "vm",
  health: "online",
  resource_severity: "normal",
  last_seen_at: "2026-08-18T12:00:00.000Z",
  os: "linux",
  arch: "amd64",
  latest_metric: null,
  service_counts: { normal: 8, info: 0, warning: 0, error: 0, unknown: 0 },
  services: Array.from({ length: 8 }, (_, i) => ({
    id: `s${i}`,
    service_key: `svc-${i}`,
    name: `service-${i}`,
    type: "daemon",
    current_state: "running",
    state_summary: "active/running",
    severity: "normal",
    last_seen_at: null,
  })),
  statuses: [],
  pinned_logs: [],
  recent_logs: [],
};

describe("MachineCard status pane", () => {
  it("puts the full service list in a scrollable region", () => {
    render(
      <MemoryRouter>
        <MachineCard m={machine} autoRefresh={false} pollMs={15000} />
      </MemoryRouter>,
    );
    const pane = screen.getByLabelText("服务状态");
    expect(pane.querySelector(".overflow-y-auto")).toBeTruthy();
    expect(screen.getByText("service-0")).toBeInTheDocument();
    expect(screen.getByText("service-7")).toBeInTheDocument();
  });
});
