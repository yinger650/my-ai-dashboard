import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatusLines } from "./StatusLines";
import type { StatusItem } from "../types";

function st(partial: Partial<StatusItem> & Pick<StatusItem, "status_key" | "label" | "value_json">): StatusItem {
  return {
    value_type: "string",
    unit: null,
    severity: "normal",
    display_format: "text",
    sort_order: 10,
    ...partial,
  };
}

describe("StatusLines", () => {
  it("groups rows under service titles when grouped", () => {
    render(
      <StatusLines
        grouped
        statuses={[
          st({
            status_key: "ssl_days",
            label: "证书",
            value_json: "5",
            service_id: "s-nginx",
            service_name: "Nginx",
          }),
          st({
            status_key: "queue",
            label: "队列",
            value_json: '"blocked"',
            service_id: "s-cursor",
            service_name: "Cursor Agent",
          }),
        ]}
      />,
    );
    expect(screen.getByRole("heading", { name: "Nginx" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Cursor Agent" })).toBeInTheDocument();
    expect(screen.getByText("证书")).toBeInTheDocument();
    expect(screen.getByText("队列")).toBeInTheDocument();
  });

  it("does not add service headings when ungrouped", () => {
    render(
      <StatusLines
        statuses={[
          st({
            status_key: "ssl_days",
            label: "证书",
            value_json: "5",
            service_name: "Nginx",
          }),
        ]}
      />,
    );
    expect(screen.queryByRole("heading", { name: "Nginx" })).not.toBeInTheDocument();
    expect(screen.getByText("证书")).toBeInTheDocument();
  });
});
