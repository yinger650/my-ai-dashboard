import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LogEntry, Machine, PinnedLog, Run, Service } from "../types";
import { ServiceDetailPage } from "./ServiceDetail";

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
  last_seen_at: "2026-09-02T12:00:00.000Z",
  enabled: true,
};

const service: Service = {
  id: "svc-1",
  machine_id: "m1",
  service_key: "cursor",
  name: "Cursor Agent",
  type: "agent",
  description: "",
  current_state: "running",
  state_summary: "2 进行中",
  severity: "info",
  last_seen_at: "2026-09-02T12:00:00.000Z",
  last_run_at: "2026-09-02T12:00:00.000Z",
  enabled: true,
};

const pinned: PinnedLog = {
  markdown: "当前任务清单",
  severity: "info",
  occurred_at: "2026-09-02T11:00:00.000Z",
  event_id: "pin-1",
};

const runs: Run[] = [
  {
    id: "r1",
    run_key: "run-a",
    status: "running",
    summary: "实现筛选",
    started_at: "2026-09-02T12:00:00.000Z",
    finished_at: null,
    provider: "cursor",
    duration_ms: null,
    created_at: "2026-09-02T12:00:00.000Z",
  },
  {
    id: "r2",
    run_key: "run-b",
    status: "succeeded",
    summary: "修 TTL",
    started_at: "2026-09-02T11:00:00.000Z",
    finished_at: "2026-09-02T11:10:00.000Z",
    provider: "cursor",
    duration_ms: 600000,
    created_at: "2026-09-02T11:00:00.000Z",
  },
];

const logs: LogEntry[] = [
  { event_id: "l1", markdown: "筛选开始", severity: "info", occurred_at: "2026-09-02T12:01:00.000Z", run_key: "run-a" },
  { event_id: "l2", markdown: "TTL 完成", severity: "info", occurred_at: "2026-09-02T11:10:00.000Z", run_key: "run-b" },
  { event_id: "l3", markdown: "服务心跳", severity: "info", occurred_at: "2026-09-02T12:02:00.000Z" },
];

function jsonOk(data: unknown): Response {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/logs")) return jsonOk(logs);
      if (url.includes("/runs")) return jsonOk(runs);
      if (url.includes("/artifacts")) throw new Error("artifacts should not be requested");
      if (url.includes("/services/svc-1")) {
        return jsonOk({ service, machine, statuses: [], pinned });
      }
      return jsonOk({});
    }),
  );
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/services/svc-1"]}>
        <Routes>
          <Route path="/services/:serviceId" element={<ServiceDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ServiceDetailPage", () => {
  beforeEach(mockApi);
  afterEach(() => vi.unstubAllGlobals());

  it("shows pinned logs, hides attachments, and filters the log column by selected runs", async () => {
    renderPage();
    expect(await screen.findByText("当前任务清单")).toBeInTheDocument();
    expect(screen.queryByText("附件")).not.toBeInTheDocument();
    expect(screen.getByText("筛选开始")).toBeInTheDocument();
    expect(screen.getByText("TTL 完成")).toBeInTheDocument();
    expect(screen.getByText("服务心跳")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "选择 实现筛选" }));
    expect(screen.getByText("筛选开始")).toBeInTheDocument();
    expect(screen.queryByText("TTL 完成")).not.toBeInTheDocument();
    expect(screen.queryByText("服务心跳")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "选择 修 TTL" }));
    expect(screen.getByText("筛选开始")).toBeInTheDocument();
    expect(screen.getByText("TTL 完成")).toBeInTheDocument();
    expect(screen.queryByText("服务心跳")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "显示全部" }));
    expect(screen.getByText("服务心跳")).toBeInTheDocument();
  });
});
