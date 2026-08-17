import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { HealthBadge, SevDot } from "./Severity";

describe("HealthBadge", () => {
  it("shows a Chinese label and icon for online", () => {
    render(<HealthBadge health="online" />);
    expect(screen.getByText("在线")).toBeInTheDocument();
  });

  it("falls back to the raw value for unknown health", () => {
    render(<HealthBadge health="mystery" />);
    expect(screen.getByText("mystery")).toBeInTheDocument();
  });
});

describe("SevDot", () => {
  it("applies the severity background class", () => {
    render(<SevDot severity="error" />);
    expect(screen.getByLabelText("error")).toHaveClass("bg-sev-error");
  });
});
