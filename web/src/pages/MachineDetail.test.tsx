import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LogEntry, Machine, PinnedLog, Service, StatusItem } from "../types";
import { MachineDetailPage } from "./MachineDetail";

const machine: Machine = {
  id: "m1",
  machine_key: "box",
  name: "测试机",
  kind: "vm",
  description: "",
  os: "linux",
  arch: "amd64",
  hostname: "box",
  collector_version: "dev",
  boot_id: null,
  heartbeat_interval_seconds: 30,
  last_seen_at: "2026-09-08T12:00:00.000Z",
  enabled: true,
};

const statuses: StatusItem[] = [
  {
    status_key: "model",
    label: "模型",
    value_json: '"llama"',
    value_type: "string",
    unit: null,
    severity: "normal",
    display_format: "text",
    sort_order: 10,
    service_id: "s-claw",
    service_key: "openclaw",
    service_name: "OpenClaw",
  },
  {
    status_key: "queue",
    label: "队列",
    value_json: '"blocked"',
    value_type: "string",
    unit: null,
    severity: "warning",
    display_format: "text",
    sort_order: 20,
    service_id: "s-cursor",
    service_key: "cursor",
    service_name: "Cursor Agent",
  },
];

const services: Service[] = [
  {
    id: "s-nginx",
    machine_id: "m1",
    service_key: "nginx",
    name: "Nginx",
    type: "daemon",
    description: "",
    current_state: "running",
    state_summary: "3 条生效反代",
    severity: "normal",
    last_seen_at: "2026-09-08T12:00:00.000Z",
    last_run_at: null,
    enabled: true,
  },
  {
    id: "s-cursor",
    machine_id: "m1",
    service_key: "cursor",
    name: "Cursor Agent",
    type: "agent",
    description: "",
    current_state: "running",
    state_summary: "1 进行中：改布局",
    severity: "info",
    last_seen_at: "2026-09-08T12:00:00.000Z",
    last_run_at: "2026-09-08T12:00:00.000Z",
    enabled: true,
  },
];

const pins: PinnedLog[] = [
  {
    markdown: "nginx 反代表",
    severity: "info",
    occurred_at: "2026-09-08T11:00:00.000Z",
    service_key: "nginx",
    service_name: "Nginx",
    event_id: "pin-nginx",
  },
  {
    markdown: "cursor 任务清单",
    severity: "info",
    occurred_at: "2026-09-08T11:01:00.000Z",
    service_key: "cursor-agent",
    service_name: "Cursor Agent",
    event_id: "pin-cursor",
  },
];

const logs: LogEntry[] = [
  {
    event_id: "l1",
    markdown: "机器滚动日志",
    severity: "info",
    occurred_at: "2026-09-08T12:01:00.000Z",
    service_name: "Nginx",
  },
];

function jsonOk(data: unknown): Response {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function stubMatchMedia(matches: boolean) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches,
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

function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/logs")) return jsonOk({ logs, pinned: pins });
      if (url.includes("/metrics")) return jsonOk({ range: "1h", samples: [] });
      if (url.includes("/services")) return jsonOk(services);
      if (url.includes("/machines/m1")) {
        return jsonOk({
          machine,
          latest_metric: { occurred_at: "2026-09-08T12:00:00.000Z", cpu_percent: 12, load1: 0.2, memory_used_bytes: 1, memory_total_bytes: 4, swap_used_bytes: null, swap_total_bytes: null, disk_read_bps: null, disk_write_bps: null, network_rx_bps: null, network_tx_bps: null, root_disk_used_bytes: 10, root_disk_total_bytes: 100 },
          health: "online",
          resource_severity: "normal",
          heartbeat_metrics: { gpu: 40 },
          statuses,
          active_runs: [],
        });
      }
      return jsonOk({});
    }),
  );
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/machines/m1"]}>
        <Routes>
          <Route path="/machines/:machineId" element={<MachineDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("MachineDetailPage logs layout", () => {
  beforeEach(mockApi);
  afterEach(() => vi.unstubAllGlobals());

  it("shows the log pane and all service pin chips on a wide screen", async () => {
    stubMatchMedia(true);
    renderPage();
    expect(await screen.findByRole("heading", { name: "测试机" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "OpenClaw" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Cursor Agent" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /打开日志|隐藏日志/ })).not.toBeInTheDocument();

    const pane = await screen.findByRole("complementary", { name: "机器日志" });
    expect(pane).toHaveAttribute("aria-hidden", "false");
    expect(await screen.findByRole("button", { name: /Nginx/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Cursor Agent/ })).toBeInTheDocument();
    expect(await screen.findByText("机器滚动日志")).toBeInTheDocument();
    expect(screen.getByText("状态：3 条生效反代（正常）。")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Nginx/ }));
    expect(screen.getByRole("dialog", { name: "Nginx 置顶日志" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "关闭" }));
    expect(screen.queryByRole("dialog", { name: "Nginx 置顶日志" })).not.toBeInTheDocument();
  });

  it("hides the log pane on a narrow screen until the side button is toggled", async () => {
    stubMatchMedia(false);
    renderPage();
    expect(await screen.findByRole("heading", { name: "测试机" })).toBeInTheDocument();

    const toggle = await screen.findByRole("button", { name: "打开日志" });
    const pane = document.getElementById("machine-logs");
    expect(pane).toHaveAttribute("aria-hidden", "true");
    expect(pane).toHaveClass("translate-x-full");

    fireEvent.click(toggle);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "隐藏日志" })).toHaveAttribute("aria-expanded", "true");
    });
    expect(pane).toHaveAttribute("aria-hidden", "false");
    expect(pane).not.toHaveClass("translate-x-full");
    expect(await screen.findByText("机器滚动日志")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "隐藏日志" }));
    expect(screen.getByRole("button", { name: "打开日志" })).toHaveAttribute("aria-expanded", "false");
    expect(pane).toHaveAttribute("aria-hidden", "true");
    expect(pane).toHaveClass("translate-x-full");
  });
});
