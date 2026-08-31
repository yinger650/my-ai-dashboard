import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PinnedLogBar } from "./PinnedLogBar";
import type { PinnedLog } from "../types";

const pins: PinnedLog[] = [
  {
    markdown: "| server | listen |\n| --- | --- |\n| bt.yinger650.com | 8080 |",
    severity: "info",
    occurred_at: "2026-08-27T07:00:00.000Z",
    service_key: "nginx",
    service_name: "Nginx",
    event_id: "pin-nginx",
  },
  {
    markdown: "cron table",
    severity: "info",
    occurred_at: "2026-08-27T07:01:00.000Z",
    service_key: "cron",
    service_name: "Cron",
    event_id: "pin-cron",
  },
];

describe("PinnedLogBar", () => {
  it("keeps pins to one row of chips and does not inline the table", () => {
    render(
      <PinnedLogBar pins={pins}>
        <div>滚动日志</div>
      </PinnedLogBar>,
    );
    expect(screen.getByRole("button", { name: /Nginx/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Cron/ })).toBeInTheDocument();
    expect(screen.queryByText("bt.yinger650.com")).not.toBeInTheDocument();
    expect(screen.getByText("滚动日志")).toBeInTheDocument();
  });

  it("opens a floating dialog on click", () => {
    render(
      <PinnedLogBar pins={pins}>
        <div>滚动日志</div>
      </PinnedLogBar>,
    );
    fireEvent.click(screen.getByRole("button", { name: /Nginx/ }));
    expect(screen.getByRole("dialog", { name: "Nginx 置顶日志" })).toBeInTheDocument();
    expect(screen.getByText("bt.yinger650.com")).toBeInTheDocument();
  });
});
