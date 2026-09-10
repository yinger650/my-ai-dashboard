import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { InstallPage } from "./Install";
import { MACHINE_KEY_PLACEHOLDER } from "../lib/install-prompt";

function jsonOk(data: unknown, status = 200): Response {
  return new Response(JSON.stringify({ data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderInstall(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/install/*" element={<InstallPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("InstallPage", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/auth/session")) return jsonOk({ authenticated: true, csrf_token: "t", edition: "personal" });
        if (url.includes("/api/v1/admin/settings")) {
          return jsonOk({
            public_url: "https://board.example",
            ingest_origin: "https://board.example",
            edition: "personal",
          });
        }
        if (url.includes("/api/v1/admin/machines") && init?.method === "POST") {
          return jsonOk(
            {
              machine: {
                id: "m-new",
                machine_key: "my-app",
                name: "my-app",
                kind: "virtual",
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
              token: { id: "tok", token: "abp_m_secret", prefix: "abp_m_secr", scope: "machine_ingest" },
            },
            201,
          );
        }
        if (url.includes("/api/v1/admin/machines")) return jsonOk([]);
        return jsonOk({});
      }),
    );
    Object.assign(navigator, { clipboard: { writeText: vi.fn(async () => undefined) } });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("splits install into agent vs host", () => {
    renderInstall("/install");
    expect(screen.getByRole("link", { name: /AI Agent/ })).toHaveAttribute("href", "/install/agent");
    expect(screen.getByRole("link", { name: /实体运行环境/ })).toHaveAttribute("href", "/install/host");
  });

  it("shows a speak-to-agent spell with {Machine Key} until a token exists", async () => {
    renderInstall("/install/agent");
    expect(await screen.findByText(/对你的 AI 说这句话/)).toBeInTheDocument();
    const spell = screen.getByRole("blockquote");
    expect(spell.textContent).toContain(MACHINE_KEY_PLACEHOLDER);
    expect(screen.getByRole("button", { name: "复制给 Agent" })).toBeInTheDocument();
    expect(spell.textContent).toContain("AGENTBOARD_PROVIDER 不要写进 .env");
    expect(spell.textContent).toContain("cursor / codex / claude / openclaw / hermes / pi");
    expect(screen.getByText(/手动安装/)).toBeInTheDocument();
  });

  it("fills the spell after creating a machine token", async () => {
    renderInstall("/install/agent");
    fireEvent.click(await screen.findByRole("button", { name: "创建机器并生成 Token" }));
    await waitFor(() => {
      expect(screen.getByRole("blockquote").textContent).toContain("abp_m_secret");
    });
    expect(screen.getByRole("blockquote").textContent).not.toContain(MACHINE_KEY_PLACEHOLDER);
  });

  it("offers linux downloads and marks macos/windows as in development", async () => {
    renderInstall("/install/host");
    expect(await screen.findByText("Linux x86_64")).toBeInTheDocument();
    expect(screen.getByText("Linux ARM64")).toBeInTheDocument();
    const linux = screen.getAllByRole("link", { name: /Linux x86_64/ });
    expect(linux[0]).toHaveAttribute("href", "https://board.example/client-updates/board-client-linux-amd64");
    expect(linux[1]).toHaveAttribute(
      "href",
      "https://github.com/yinger650/my-ai-dashboard/releases/latest/download/board-client-linux-amd64",
    );
    expect(screen.getAllByText("开发中").length).toBe(2);
    expect(screen.getByText("macOS")).toBeInTheDocument();
    expect(screen.getByText("Windows")).toBeInTheDocument();
    expect(screen.getByText(/手动安装/)).toBeInTheDocument();
  });
});
