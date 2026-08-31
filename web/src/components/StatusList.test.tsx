import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { StatusList } from "./StatusList";
import type { BoardService, StatusItem } from "../types";

const services: BoardService[] = [
  {
    id: "s-nginx",
    service_key: "nginx",
    name: "Nginx",
    type: "daemon",
    current_state: "running",
    state_summary: "2 条生效反代",
    severity: "normal",
    last_seen_at: null,
  },
  {
    id: "s-old",
    service_key: "nginx.service",
    name: "nginx.service",
    type: "daemon",
    current_state: "inactive",
    state_summary: "dead",
    severity: "unknown",
    last_seen_at: null,
  },
];

const statuses: StatusItem[] = [
  {
    status_key: "listen_80",
    label: "监听 80",
    value_json: "\"0.0.0.0:80/tcp\"",
    value_type: "string",
    unit: null,
    severity: "normal",
    display_format: "text",
    sort_order: 10,
    service_id: "s-nginx",
    service_key: "nginx",
  },
];

describe("StatusList compact", () => {
  it("hides summaries and listen statuses; shows a red badge for log counts", () => {
    render(
      <MemoryRouter>
        <StatusList
          services={services}
          statuses={statuses}
          compact
          newLogCounts={{ "s-nginx": 3 }}
        />
      </MemoryRouter>,
    );
    expect(screen.getByText("Nginx")).toBeInTheDocument();
    expect(screen.queryByText("2 条生效反代")).not.toBeInTheDocument();
    expect(screen.queryByText("nginx.service")).not.toBeInTheDocument();
    expect(screen.queryByText("监听 80")).not.toBeInTheDocument();
    expect(screen.getByTitle("3 条日志")).toHaveTextContent("3");
  });

  it("marks a service as opened when its name is clicked", () => {
    const onOpenService = vi.fn();
    render(
      <MemoryRouter>
        <StatusList services={services} statuses={[]} compact newLogCounts={{ "s-nginx": 3 }} onOpenService={onOpenService} />
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByText("Nginx"));
    expect(onOpenService).toHaveBeenCalledWith("s-nginx", "nginx");
  });
});
