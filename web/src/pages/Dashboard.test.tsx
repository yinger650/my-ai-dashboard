import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BoardMachine } from "../types";
import { SHOW_OFFLINE_STORAGE_KEY } from "../lib/board-filter";
import { DashboardPage } from "./Dashboard";

function machine(partial: Pick<BoardMachine, "id" | "name" | "health"> & Partial<BoardMachine>): BoardMachine {
  return {
    machine_key: partial.machine_key ?? partial.id,
    kind: "vm",
    resource_severity: "normal",
    last_seen_at: null,
    os: null,
    arch: null,
    latest_metric: null,
    service_counts: { normal: 0, info: 0, warning: 0, error: 0, unknown: 0 },
    services: [],
    statuses: [],
    pinned_logs: [],
    recent_logs: [],
    ...partial,
  };
}

function jsonOk(data: unknown): Response {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function stubMatchMedia() {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }),
  });
}

function mockBoard(machines: BoardMachine[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/v1/board")) {
        return jsonOk({
          title: "测试看板",
          machines,
          recent_abnormal: 0,
          server_time: "2026-08-25T12:00:00.000Z",
          poll_interval_seconds: 15,
        });
      }
      if (url.includes("/logs")) {
        return jsonOk({ logs: [], pinned: [] });
      }
      return jsonOk({});
    }),
  );
}

function renderDashboard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const online = machine({ id: "m-online", name: "在线盒子", health: "online" });
const offline = machine({ id: "m-offline", name: "离线盒子", health: "offline" });

describe("DashboardPage offline visibility", () => {
  beforeEach(() => {
    localStorage.clear();
    stubMatchMedia();
    mockBoard([online, offline]);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("shows online and offline machines by default", async () => {
    renderDashboard();
    expect(await screen.findByText("在线盒子")).toBeInTheDocument();
    expect(screen.getByText("离线盒子")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /离线 1/ })).toBeInTheDocument();
  });

  it("hides offline machines when the switch is turned off", async () => {
    renderDashboard();
    await screen.findByText("离线盒子");

    fireEvent.click(screen.getByLabelText("显示离线"));

    await waitFor(() => {
      expect(screen.queryByText("离线盒子")).not.toBeInTheDocument();
    });
    expect(screen.getByText("在线盒子")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /离线 1（已隐藏）/ })).toBeInTheDocument();
    expect(localStorage.getItem(SHOW_OFFLINE_STORAGE_KEY)).toBe("0");
  });

  it("restores hidden machines from the empty-state action", async () => {
    mockBoard([offline]);
    renderDashboard();
    await screen.findByText("离线盒子");

    fireEvent.click(screen.getByLabelText("显示离线"));
    expect(await screen.findByText(/已隐藏 1 台离线机器/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "显示离线" }));
    expect(await screen.findByText("离线盒子")).toBeInTheDocument();
  });

  it("honors a stored hide preference on first render", async () => {
    localStorage.setItem(SHOW_OFFLINE_STORAGE_KEY, "0");
    renderDashboard();
    expect(await screen.findByText("在线盒子")).toBeInTheDocument();
    expect(screen.queryByText("离线盒子")).not.toBeInTheDocument();
  });
});
