import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PortTable } from "./PortTable";

describe("PortTable", () => {
  it("shows empty state", () => {
    render(<PortTable ports={[]} />);
    expect(screen.getByText("暂无监听端口")).toBeInTheDocument();
  });

  it("renders listening rows", () => {
    render(
      <PortTable
        ports={[
          { protocol: "tcp", address: "127.0.0.1", port: 8090, process: "board-server" },
          { protocol: "tcp", address: "0.0.0.0", port: 443, process: "nginx" },
        ]}
      />,
    );
    expect(screen.getByText("board-server")).toBeInTheDocument();
    expect(screen.getByText("8090")).toBeInTheDocument();
    expect(screen.getByText("nginx")).toBeInTheDocument();
  });
});
