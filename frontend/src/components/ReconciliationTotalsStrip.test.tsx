// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ReconciliationTotalsStrip } from "./ReconciliationTotalsStrip";
import type { CurrencyInfo } from "../format";

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

let root: Root;
let host: HTMLDivElement;

beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => {
    root.unmount();
  });
  host.remove();
});

async function renderStrip(props: {
  calculatedMinor: number | null;
  printedMinor: number;
  reason: string | null;
}) {
  await act(async () => {
    root.render(<ReconciliationTotalsStrip {...props} currency={IDR} />);
  });
}

describe("ReconciliationTotalsStrip", () => {
  it("reads matched when the calculation equals the printed total", async () => {
    await renderStrip({ calculatedMinor: 24600, printedMinor: 24600, reason: null });
    expect(document.body.textContent).toContain("Matched");
    expect(document.body.textContent).toContain("Derived only");
    expect(document.body.textContent?.includes("Explaining input")).toBe(false);
  });

  it("names the first differing input when different", async () => {
    await renderStrip({
      calculatedMinor: 24600,
      printedMinor: 24500,
      reason: "Whole-PO discount 2% vs 0%",
    });
    expect(document.body.textContent).toContain("Different");
    expect(document.body.textContent).toContain("Whole-PO discount 2% vs 0%");
  });

  it("reads pending while any amount is invalid", async () => {
    await renderStrip({ calculatedMinor: null, printedMinor: 24500, reason: null });
    expect(document.body.textContent).toContain("Pending");
    expect(document.body.textContent).toContain("Complete every amount");
  });

  it("says so when no input difference explains the printed total", async () => {
    await renderStrip({
      calculatedMinor: 24600,
      printedMinor: 24500,
      reason: "no input difference explains the printed total",
    });
    expect(document.body.textContent).toContain(
      "no input difference explains the printed total"
    );
  });
});
