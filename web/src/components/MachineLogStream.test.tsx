import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MachineLogStream } from "./MachineLogStream";
import type { LogEntry, PinnedLog } from "../types";
import { apiGetPage } from "../api";

vi.mock("../api", () => ({
  apiGetPage: vi.fn(),
}));

const pinned: PinnedLog[] = [
  {
    markdown: "置顶摘要：Cursor Agent 日志总结",
    severity: "info",
    occurred_at: "2026-08-18T12:00:00.000Z",
    service_name: "Cursor Agent",
    event_id: "pin-1",
  },
];

const logs: LogEntry[] = [
  {
    event_id: "log-1",
    markdown: "普通日志条目 hello",
    severity: "info",
    occurred_at: "2026-08-18T12:01:00.000Z",
    service_name: "sshd.service",
  },
];

describe("MachineLogStream", () => {
  beforeEach(() => {
    vi.mocked(apiGetPage).mockResolvedValue({
      data: { logs, pinned },
      nextCursor: null,
    });
  });

  it("renders pinned logs outside the scrolling timeline", async () => {
    render(
      <MachineLogStream
        machineId="m1"
        autoRefresh={false}
        pollMs={15000}
        initialLogs={logs}
        initialPinned={pinned}
      />,
    );

    const pinPane = await screen.findByLabelText("置顶日志");
    const timeline = screen.getByLabelText("日志时间线");
    await waitFor(() => {
      expect(pinPane).toContainElement(screen.getByText(/置顶摘要/));
      expect(timeline).toContainElement(screen.getByText(/普通日志条目 hello/));
    });
    expect(timeline).not.toContainElement(pinPane);
    expect(pinPane.className).not.toMatch(/sticky/);
  });
});
