import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsPage } from "./Settings";

function jsonOk(data: unknown): Response {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function renderSettings() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <SettingsPage />
    </QueryClientProvider>,
  );
}

describe("SettingsPage log storage", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/api/v1/admin/machines")) return jsonOk([]);
        if (url.includes("/api/v1/admin/tokens")) return jsonOk([]);
        if (url.includes("/api/v1/admin/totp")) return jsonOk({ enabled: false });
        if (url.includes("/api/v1/admin/settings")) {
          return jsonOk({
            board_title: "AgentBoard Personal",
            timezone: "UTC",
            poll_interval_seconds: 15,
            event_retention_days: 30,
            event_quota_bytes: 5 * 1024 * 1024 * 1024,
          });
        }
        if (url.includes("/api/v1/admin/maintenance/run") && init?.method === "POST") {
          return jsonOk({
            expired_sessions_deleted: 0,
            events_deleted: 2,
            access_deleted: 0,
            runs_deleted: 0,
            runs_closed: 0,
            quota_deleted: 0,
            events_bytes: 1024,
          });
        }
        return jsonOk({});
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps storage cleanup separate from automatic stale-run close", async () => {
    renderSettings();
    expect(await screen.findByText(/滚动日志、事件与访问记录最多保留一个月/)).toBeInTheDocument();
    expect(screen.queryByText(/超过 1\s*天没有新日志的进行中 Run 会直接关闭/)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "立即清理" }));

    expect(await screen.findByText(/已删事件 2/)).toBeInTheDocument();
    await waitFor(() => {
      const calls = vi.mocked(fetch).mock.calls;
      expect(calls.some(([input, init]) => String(input).includes("/admin/maintenance/run") && init?.method === "POST")).toBe(true);
    });
  });
});

describe("SettingsPage API keys", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/api/v1/admin/machines")) {
          return jsonOk([
            {
              id: "m1",
              machine_key: "agents",
              name: "Agents",
              kind: "physical",
              description: "",
              os: null,
              arch: null,
              hostname: null,
              collector_version: null,
              boot_id: null,
              heartbeat_interval_seconds: 30,
              last_seen_at: null,
              enabled: true,
            },
          ]);
        }
        if (url.includes("/api/v1/admin/tokens")) {
          return jsonOk([
            {
              id: "tok-m",
              name: "Agents machine token",
              token_prefix: "abp_m_NN6QSo",
              scope: "machine_ingest",
              machine_id: "m1",
              service_id: null,
              last_used_at: null,
              last_used_ip: null,
              enabled: true,
              revoked_at: null,
            },
          ]);
        }
        if (url.includes("/api/v1/admin/totp")) return jsonOk({ enabled: false });
        if (url.includes("/api/v1/admin/settings")) {
          return jsonOk({
            board_title: "AgentBoard Personal",
            timezone: "UTC",
            poll_interval_seconds: 15,
            event_retention_days: 30,
            event_quota_bytes: 5 * 1024 * 1024 * 1024,
          });
        }
        return jsonOk({});
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("does not present machine tokens as viewer keys", async () => {
    renderSettings();
    expect(await screen.findByText("Agents machine token")).toBeInTheDocument();
    expect(screen.getByText("Machine Token")).toBeInTheDocument();
    expect(screen.getByText("Viewer Token")).toBeInTheDocument();
    expect(screen.getByText("还没有 Viewer Token。")).toBeInTheDocument();
    expect(screen.getByText("abp_m_NN6QSo…")).toBeInTheDocument();
    expect(screen.getAllByText("agents").length).toBeGreaterThan(0);
    expect(screen.getByText("上报")).toBeInTheDocument();
    expect(screen.queryByText("machine_ingest")).not.toBeInTheDocument();
  });
});
