import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Layout } from "./Layout";

function jsonOk(data: unknown): Response {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("Layout nav", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input).includes("/auth/session")) {
          return jsonOk({ authenticated: true, csrf_token: "t", edition: "personal" });
        }
        return jsonOk({});
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("puts 安装 next to 看板 / 访问记录 / 设置", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <Layout>
            <div />
          </Layout>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    const labels = screen.getAllByRole("link").map((el) => el.textContent);
    expect(labels).toEqual(expect.arrayContaining(["看板", "安装", "访问记录", "设置"]));
    expect(screen.getByRole("link", { name: "安装" })).toHaveAttribute("href", "/install");
  });
});
