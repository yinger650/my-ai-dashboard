import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { NetworkRate } from "./NetworkRate";

describe("NetworkRate", () => {
  it("renders down and up rates", () => {
    render(<NetworkRate rx={1024} tx={2048} />);
    expect(screen.getByTitle("网速")).toHaveTextContent("↓");
    expect(screen.getByTitle("网速")).toHaveTextContent("↑");
  });

  it("renders nothing when both sides are missing", () => {
    const { container } = render(<NetworkRate rx={null} tx={null} />);
    expect(container).toBeEmptyDOMElement();
  });
});
