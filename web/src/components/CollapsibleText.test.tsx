import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CollapsibleText } from "./CollapsibleText";

describe("CollapsibleText", () => {
  it("renders short text without a toggle", () => {
    render(<CollapsibleText text="hello world" />);
    expect(screen.getByText("hello world")).toBeInTheDocument();
    expect(screen.queryByText("展开")).not.toBeInTheDocument();
  });

  it("collapses long text and expands on click", () => {
    const text = "很长的状态说明，".repeat(40);
    render(<CollapsibleText text={text} maxChars={40} />);
    expect(screen.getByText("展开")).toBeInTheDocument();
    fireEvent.click(screen.getByText("展开"));
    expect(screen.getByText("收起")).toBeInTheDocument();
  });
});
