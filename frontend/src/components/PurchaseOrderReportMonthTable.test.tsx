// @vitest-environment jsdom
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import {
  PurchaseOrderReportMonthCard,
  PurchaseOrderReportMonthTable,
} from "./PurchaseOrderReportMonthTable";
import { resolveMonthReadout } from "../poReportComparison";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportMonthWithRecordedFutureFixture,
  purchaseOrderReportYearFixture,
} from "../poReportFixtures";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderMonthReadout } from "../poReportComparison";

// Issue #641 correction F1: the estimate's base + low/high tuple was one
// non-wrapping string inside a two-column grid. At a real 360px viewport that
// cell is ~118px wide while the tuple needs 328px, so the card's overflow hid
// the low/high evidence. These are structural guards, not a jsdom layout
// claim: each exact amount stays tabular and nowrap, while the tuple gets its
// own full-width cell and explicit Low/High boundaries that may wrap.

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

function currentReadout(): PurchaseOrderMonthReadout {
  return resolveMonthReadout({
    overview: purchaseOrderReportMonthFixture(),
    month: "2026-11",
    annualSource: null,
    annualStatus: "loading",
    annualError: "",
  });
}

function render(readout: PurchaseOrderMonthReadout): HTMLElement {
  document.body.innerHTML = renderToString(
    <PurchaseOrderReportMonthTable
      readout={readout}
      entry={IDR}
      onPrevious={() => undefined}
      onNext={() => undefined}
      onRetry={() => undefined}
    />
  );
  return document.body;
}

function estimateBlock(): HTMLElement {
  const found = document.querySelector<HTMLElement>(
    '[data-month-evidence="estimated"]'
  );
  if (found === null) throw new Error("no estimated block");
  return found;
}

function amountPart(block: HTMLElement, part: string): HTMLElement {
  const found = block.querySelector<HTMLElement>(
    `[data-amount-part="${part}"]`
  );
  if (found === null) throw new Error(`no amount part ${part}`);
  return found;
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("PurchaseOrderReportMonthTable estimate readability (issue #641 F1)", () => {
  it("keeps each exact amount tabular and nowrap but lets the tuple reflow with labels", () => {
    const readout = currentReadout();
    render(readout);
    const estimate = estimateBlock();

    const grid = estimate.querySelector<HTMLElement>(
      "[data-month-estimate-grid]"
    );
    expect(grid).not.toBeNull();
    expect(grid?.className).toContain("grid-cols-1");
    expect(grid?.className.split(" ")).not.toContain("grid-cols-2");

    const amountCell = estimate.querySelector<HTMLElement>(
      '[data-month-estimate-metric="PO amount"]'
    );
    expect(amountCell).not.toBeNull();
    expect(amountCell?.className).toContain("sm:col-span-2");
    expect(amountCell?.className).toContain("lg:col-span-3");

    expect(
      document
        .querySelector<HTMLElement>("[data-month-browser]")
        ?.className.split(" ")
    ).not.toContain("overflow-hidden");
    const amount = estimate.querySelector<HTMLElement>(
      "[data-month-estimate-amount]"
    );
    expect(amount).not.toBeNull();
    expect(amount?.className).toContain("flex-wrap");
    expect(amount?.textContent).not.toContain(
      "Rp110.000.000 (Rp88.000.000 – Rp132.000.000)"
    );
    expect(amount?.textContent).toContain("Low");
    expect(amount?.textContent).toContain("High");

    expect(amountPart(estimate, "base").textContent).toBe("Rp110.000.000");
    expect(amountPart(estimate, "low").textContent).toBe("Rp88.000.000");
    expect(amountPart(estimate, "high").textContent).toBe("Rp132.000.000");
    for (const part of ["base", "low", "high"]) {
      const node = amountPart(estimate, part);
      expect(node.className).toContain("tnum");
      expect(node.className).toContain("whitespace-nowrap");
    }

    const cells = estimate.querySelectorAll<HTMLElement>(
      "[data-month-estimate-metric]"
    );
    expect(cells.length).toBe(6);
    for (const cell of Array.from(cells)) {
      expect(cell.className).toContain("min-w-0");
    }
    for (const value of Array.from(
      estimate.querySelectorAll<HTMLElement>(
        '[data-month-estimate-metric]:not([data-month-estimate-metric="PO amount"]) dd'
      )
    )) {
      expect(value.className).toContain("tnum");
      expect(value.className).toContain("whitespace-nowrap");
    }
  });

  it("keeps recorded-future secondary metrics single-column and nowrap at narrow widths", () => {
    const readout = resolveMonthReadout({
      overview: purchaseOrderReportMonthWithRecordedFutureFixture(),
      month: "2026-11",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    render(readout);
    const recorded = document.querySelector<HTMLElement>(
      '[data-month-evidence="recorded-future"]'
    );
    expect(recorded).not.toBeNull();
    expect(recorded?.className).toContain("grid-cols-1");
    const cells = recorded!.querySelectorAll<HTMLElement>(
      "[data-month-recorded-metric]"
    );
    expect(cells.length).toBe(6);
    for (const cell of Array.from(cells)) {
      expect(cell.className).toContain("min-w-0");
      const value = cell.querySelector<HTMLElement>("dd");
      expect(value?.className).toContain("tnum");
      expect(value?.className).toContain("whitespace-nowrap");
    }
  });

  it("keeps all secondary metrics readable at the maximum exact value", () => {
    const readout = currentReadout();
    const max = Number.MAX_SAFE_INTEGER;
    readout.estimate = {
      ...readout.estimate!,
      low: max - 2_000_000,
      base: max - 1_000_000,
      high: max,
      capital: max - 3_000_000,
      margin: -1_000_000,
      marginBps: -3333,
    };
    render(readout);
    const estimate = estimateBlock();

    const grid = estimate.querySelector<HTMLElement>(
      "[data-month-estimate-grid]"
    );
    expect(grid?.className).toContain("grid-cols-1");
    expect(estimate.querySelector("[class*='overflow-hidden']")).toBeNull();

    for (const part of ["base", "low", "high"]) {
      const node = amountPart(estimate, part);
      expect(node.textContent?.startsWith("Rp")).toBe(true);
      expect(node.className).toContain("whitespace-nowrap");
    }
    expect(amountPart(estimate, "base").textContent?.length).toBeGreaterThan(15);

    for (const value of Array.from(
      estimate.querySelectorAll<HTMLElement>(
        '[data-month-estimate-metric]:not([data-month-estimate-metric="PO amount"]) dd'
      )
    )) {
      expect(value.className).toContain("tnum");
    }
  });
});

describe("comparison card reuses the same vertical table (issue #642)", () => {
  function renderCard(
    readout: PurchaseOrderMonthReadout,
    active = false,
    onRetry = () => undefined
  ): HTMLElement {
    document.body.innerHTML = renderToString(
      <PurchaseOrderReportMonthCard
        readout={readout}
        entry={IDR}
        active={active}
        onRetry={onRetry}
      />
    );
    return document.body;
  }

  it("prints the same six metric rows as the standalone month table", () => {
    const readout = currentReadout();
    const standalone = render(readout);
    const standaloneRows = Array.from(
      standalone.querySelectorAll<HTMLElement>("[data-month-metric]")
    ).map((node) => [node.getAttribute("data-month-metric"), node.textContent]);
    const card = renderCard(readout, true);
    const cardRows = Array.from(
      card.querySelectorAll<HTMLElement>("[data-month-metric]")
    ).map((node) => [node.getAttribute("data-month-metric"), node.textContent]);

    expect(cardRows).toEqual(standaloneRows);
    expect(cardRows.map(([label]) => label)).toEqual([
      "POs",
      "PO amount",
      "Costed amount",
      "Capital",
      "PO margin",
      "PO margin %",
    ]);
    expect(document.querySelector("[data-month-card]")).not.toBeNull();
    expect(
      document.querySelector("[data-month-card]")?.getAttribute("data-month-key")
    ).toBe("2026-11");
    expect(
      document.querySelector("[data-month-card]")?.getAttribute("data-month-active")
    ).toBe("true");
    // The card is a piece of a comparison, not a second browser chrome.
    expect(document.querySelector("[data-month-browser]")).toBeNull();
    expect(document.querySelector('[data-month-nav="previous"]')).toBeNull();
    expect(document.querySelector("[data-month-evidence='estimated']")).not.toBeNull();
  });

  it("shows a per-card retry and no money when its year source failed", () => {
    const readout = resolveMonthReadout({
      overview: purchaseOrderReportMonthFixture(),
      month: "2025-12",
      annualSource: null,
      annualStatus: "error",
      annualError: "The monthly API is unreachable.",
    });
    renderCard(readout);
    expect(document.body.textContent).toContain("The monthly API is unreachable.");
    expect(document.body.textContent).toContain("Retry");
    expect(document.querySelector("[data-month-metric]")).toBeNull();
    expect(document.body.textContent).not.toContain("Rp");
  });

  it("renders a loaded zero month as six real zero cells", () => {
    const annual = purchaseOrderReportYearFixture();
    const january = annual.periods[0];
    january.poCount = 0;
    january.amountMinor = 0;
    january.costedMinor = 0;
    january.uncostedMinor = 0;
    january.capitalMinor = 0;
    january.marginMinor = 0;
    january.marginBps = null;
    january.coverageBps = null;
    const readout = resolveMonthReadout({
      overview: purchaseOrderReportFixture(),
      month: "2026-01",
      annualSource: annual,
      annualStatus: "ready",
      annualError: "",
    });
    renderCard(readout);
    expect(
      document.querySelector('[data-month-metric="POs"]')?.textContent
    ).toBe("0");
    expect(
      document.querySelector('[data-month-metric="PO amount"]')?.textContent
    ).toBe("Rp0");
    expect(
      document.querySelector('[data-month-metric="PO margin %"]')?.textContent
    ).toBe("-");
  });
});
