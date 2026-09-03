import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ServicePathLine } from "./ServicePath";

describe("ServicePathLine", () => {
  it("renders a probe URL", () => {
    render(<ServicePathLine path="https://www.yinger650.com/" />);
    expect(screen.getByText(/探测 URL/)).toBeInTheDocument();
    expect(screen.getByText(/https:\/\/www\.yinger650\.com\//)).toBeInTheDocument();
  });

  it("renders a main process path", () => {
    render(<ServicePathLine path="/usr/sbin/sshd" />);
    expect(screen.getByText(/主进程/)).toBeInTheDocument();
    expect(screen.getByText(/\/usr\/sbin\/sshd/)).toBeInTheDocument();
  });

  it("hides empty paths", () => {
    const { container } = render(<ServicePathLine path="  " />);
    expect(container).toBeEmptyDOMElement();
  });
});
