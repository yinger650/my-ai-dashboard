import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { StatusList } from "./StatusList";
import type { BoardService } from "../types";

function svc(id: string, name: string): BoardService {
  return {
    id,
    service_key: id,
    name,
    type: "daemon",
    current_state: "running",
    state_summary: "active/running",
    severity: "normal",
    last_seen_at: null,
  };
}

describe("StatusList", () => {
  it("shows all services when collapsedCount is all", () => {
    const services = Array.from({ length: 8 }, (_, i) => svc(`s${i}`, `svc-${i}`));
    render(
      <MemoryRouter>
        <StatusList services={services} statuses={[]} collapsedCount="all" />
      </MemoryRouter>,
    );
    expect(screen.getByText("svc-0")).toBeInTheDocument();
    expect(screen.getByText("svc-7")).toBeInTheDocument();
    expect(screen.queryByText(/还有/)).not.toBeInTheDocument();
  });

  it("hides extra rows until expanded", () => {
    const services = Array.from({ length: 6 }, (_, i) => svc(`s${i}`, `svc-${i}`));
    render(
      <MemoryRouter>
        <StatusList services={services} statuses={[]} collapsedCount={2} />
      </MemoryRouter>,
    );
    expect(screen.getByText("svc-0")).toBeInTheDocument();
    expect(screen.queryByText("svc-5")).not.toBeInTheDocument();
    expect(screen.getByText("还有 4 项")).toBeInTheDocument();
  });
});
