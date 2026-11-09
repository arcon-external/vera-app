// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ReconciliationTotalsStrip, type TotalsStripProps } from "./ReconciliationTotalsStrip";
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

const STAGES = {
  subtotalMinor: 20000,
  discountMinor: 0,
  ppnMinor: 2000,
  adjustmentMinor: 0,
  grandTotalMinor: 22000,
};

async function renderStrip(props: Omit<TotalsStripProps, "currency">) {
  await act(async () => {
    root.render(<ReconciliationTotalsStrip {...props} currency={IDR} />);
  });
}

function button(label: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (el) => (el.textContent ?? "").includes(label) || (el.getAttribute("aria-label") ?? "").includes(label)
  );
  if (found === undefined) throw new Error(`no button ${label}`);
  return found;
}

describe("ReconciliationTotalsStrip (U6)", () => {
  it("reads matched when every stage equals the printed figures", async () => {
    await renderStrip({ calculated: { ...STAGES }, printed: { ...STAGES }, reason: null });
    expect(document.body.textContent).toContain("Matches the printed totals");
    expect(document.body.textContent).toContain("Derived only");
    expect(document.body.textContent?.includes("Explaining input")).toBe(false);
    // Neutral status copy: never a danger pill.
    expect(document.querySelector(".border-danger-200, .bg-danger-50, .text-danger-700")).toBeNull();
  });

  it("reads different when offsetting stage differences net out", async () => {
    // Subtotal +1000, discount +1000: the grand total matches but the stages do not.
    await renderStrip({
      calculated: { ...STAGES, subtotalMinor: 21000, discountMinor: 1000 },
      printed: { ...STAGES },
      reason: "Whole-PO discount 2% vs 0%",
    });
    expect(document.body.textContent).toContain("Different");
    expect(document.body.textContent).toContain("Whole-PO discount 2% vs 0%");
    expect(document.querySelector(".border-danger-200, .bg-danger-50, .text-danger-700")).toBeNull();
  });

  it("expands to per-stage rows on demand", async () => {
    await renderStrip({ calculated: { ...STAGES }, printed: { ...STAGES }, reason: null });
    expect(document.body.textContent?.includes("Subtotal")).toBe(false);
    button("Show stages").click();
    await act(async () => {});
    expect(document.body.textContent).toContain("Subtotal");
    expect(document.body.textContent).toContain("PPN");
    expect(document.body.textContent).toContain("Grand total");
  });

  it("reads pending while any amount is invalid", async () => {
    await renderStrip({ calculated: null, printed: { ...STAGES }, reason: null });
    expect(document.body.textContent).toContain("Pending");
    expect(document.body.textContent).toContain("Fix the highlighted input.");
  });

  it("says unavailable with no linked source", async () => {
    await renderStrip({ calculated: { ...STAGES }, printed: null, reason: null });
    expect(document.body.textContent).toContain("unavailable until a Quotation is linked");
  });

  it("says so when no input difference explains the printed total", async () => {
    await renderStrip({
      calculated: { ...STAGES, grandTotalMinor: 24600 },
      printed: { ...STAGES, grandTotalMinor: 24500 },
      reason: "no input difference explains the printed total",
    });
    expect(document.body.textContent).toContain("no input difference explains the printed total");
  });
});
