// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ToastProvider, useToast } from "./toast";

// Issue #634: a saved report prints an absolute path. An unbroken path is the
// widest toast message the app produces, and the card must contain it at 360
// and 1280 rather than letting the text run past the border.

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const LONG_PATH =
  "/Users/tristan/Downloads/Purchase_Order_Report_2026-01-01_to_2026-11-21.pdf";

function SaveProbe() {
  const { pushToast } = useToast();
  return (
    <button
      type="button"
      onClick={() => pushToast("success", `Report saved to ${LONG_PATH}`)}
    >
      Save
    </button>
  );
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe("toast message containment", () => {
  it("lets a long unbroken path shrink and wrap inside the card", () => {
    act(() => {
      root.render(
        <ToastProvider>
          <SaveProbe />
        </ToastProvider>
      );
    });
    const button = container.querySelector("button");
    if (button === null) throw new Error("expected the save probe");
    act(() => button.click());

    const status = document.querySelector('[role="status"]');
    if (status === null) throw new Error("expected the toast");
    const message = Array.from(status.querySelectorAll("span")).find((node) =>
      node.textContent?.includes(LONG_PATH)
    );
    if (message === undefined) throw new Error("expected the path message");

    // min-w-0 is what lets the flex child shrink below its content width;
    // overflow-wrap:anywhere breaks the path at any character when it must.
    expect(message.className).toContain("min-w-0");
    expect(message.className).toContain("[overflow-wrap:anywhere]");
    expect(message.className).toContain("flex-1");
    // The icon and dismiss button align to a multi-line message's first line.
    expect(status.className).toContain("items-start");
    expect(status.textContent).toContain(LONG_PATH);
  });
});
