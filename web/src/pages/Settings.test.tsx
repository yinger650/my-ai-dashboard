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

describe("SettingsPage log cleanup", () => {
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
            runs_closed: 4,
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

  it("explains stale-run close and reports closed count after cleanup", async () => {
    renderSettings();
    expect(await screen.findByText(/超过 1\s*天没有新日志的进行中 Run 会直接关闭/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "立即清理" }));

    expect(await screen.findByText(/已关闭过期 Run 4/)).toBeInTheDocument();
    await waitFor(() => {
      const calls = vi.mocked(fetch).mock.calls;
      expect(calls.some(([input, init]) => String(input).includes("/admin/maintenance/run") && init?.method === "POST")).toBe(true);
    });
  });
});
