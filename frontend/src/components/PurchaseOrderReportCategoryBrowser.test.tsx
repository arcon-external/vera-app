// @vitest-environment jsdom
// Issue #643: the independent By category browser's presentational contract.
// These are structural DOM assertions on the same renderToString output the
// snapshot harness consumes. Live interaction (picker focus, request counts,
// export gating) belongs to PurchaseOrderReportsPage.test.tsx.

import { createRef } from "react";
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import {
  PurchaseOrderReportCategoryAggregate,
  PurchaseOrderReportCategoryBrowser,
  PurchaseOrderReportCategoryCard,
  type PurchaseOrderReportCategoryBrowserProps,
} from "./PurchaseOrderReportCategoryBrowser";
import { resolveCategoryReadout } from "../poReportComparison";
import type {
  PurchaseOrderCategoryReadout,
  ReportComparisonLayout,
} from "../poReportComparison";
import { purchaseOrderReportFixture } from "../poReportFixtures";
import type { PurchaseOrderReportData } from "../poReport";
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

const FIVE_CODES = ["FM", "FAB", "SUPPLY_GEN", "OTHERS", "UNASSIGNED"];
const THREE_CODES = ["FM", "FAB", "SUPPLY_GEN"];
const FOUR_CODES = ["FM", "FAB", "SUPPLY_GEN", "OTHERS"];

function readoutFor(
  explicitCodes: readonly string[] | null,
  source: PurchaseOrderReportData = purchaseOrderReportFixture()
): PurchaseOrderCategoryReadout {
  return resolveCategoryReadout({
    overview: source,
    source,
    status: "ready",
    error: "",
    explicitCodes,
  });
}

function codesOf(readout: PurchaseOrderCategoryReadout): string[] {
  return readout.selected.map((item) => item.code);
}

interface RenderOptions {
  readout?: PurchaseOrderCategoryReadout;
  codes?: readonly string[];
  allSelected?: boolean;
  layout?: ReportComparisonLayout;
  entry?: CurrencyInfo | null;
}

function renderBrowser(options: RenderOptions = {}): HTMLElement {
  const readout = options.readout ?? readoutFor(null);
  const codes = options.codes ?? codesOf(readout);
  const supplied = readout.supplied.map((item) => item.code);
  const allSelected =
    options.allSelected ??
    (supplied.length > 0 &&
      codes.length === supplied.length &&
      supplied.every((code) => codes.includes(code)));
  const props: PurchaseOrderReportCategoryBrowserProps = {
    codes,
    activeCode: codes[0] ?? null,
    allSelected,
    layout: options.layout ?? "aggregate",
    readout,
    entry: options.entry === undefined ? IDR : options.entry,
    triggerRef: createRef<HTMLButtonElement>(),
    onOpenPicker: () => undefined,
    onLayoutChange: () => undefined,
    onPrevious: () => undefined,
    onNext: () => undefined,
    onRetry: () => undefined,
  };
  document.body.innerHTML = renderToString(
    <PurchaseOrderReportCategoryBrowser {...props} />
  );
  return document.body;
}

function metric(card: HTMLElement, label: string): HTMLElement | null {
  return card.querySelector<HTMLElement>(
    `[data-category-metric="${label}"]`
  );
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("category browser — All/Aggregate (issue #643)", () => {
  it("opens with All categories in Aggregate and prints the server total", () => {
    const source = purchaseOrderReportFixture();
    // A total that deliberately differs from the row sum: the footer must be
    // the snapshot's own total, never a client sum.
    source.totals = { ...source.totals, amountMinor: 999_000_000 };
    renderBrowser({ readout: readoutFor(null, source) });

    const browser = document.querySelector("[data-category-browser]");
    expect(browser).not.toBeNull();
    expect(browser?.getAttribute("aria-label")).toBe("Category comparison");
    const trigger = document.querySelector<HTMLButtonElement>(
      "[data-category-picker-trigger]"
    );
    expect(trigger?.getAttribute("data-category-selection-summary")).toBe(
      "All categories"
    );
    expect(trigger?.disabled).toBe(false);

    const scope = document.querySelector("[data-category-scope]");
    expect(scope?.textContent).toContain("All categories");
    expect(scope?.textContent).toContain("1 Jan 2026 – 19 Nov 2026");
    expect(scope?.textContent).toContain("IDR");
    expect(scope?.textContent).toContain("As of 19 Nov 2026");
    expect(scope?.textContent).toContain("snapshot 2026-11-19T05:00:00Z");
    expect(scope?.textContent).not.toContain("Fabrication");

    const group = document.querySelector(
      '[role="group"][aria-label="All categories display"]'
    );
    expect(group).not.toBeNull();
    expect(
      document
        .querySelector('[data-category-layout="aggregate"]')
        ?.getAttribute("aria-pressed")
    ).toBe("true");
    expect(
      document
        .querySelector('[data-category-layout="individual"]')
        ?.getAttribute("aria-pressed")
    ).toBe("false");

    const aggregate = document.querySelector("[data-category-aggregate]");
    expect(aggregate).not.toBeNull();
    const table = aggregate?.querySelector(
      'table[aria-label="Category totals"]'
    );
    expect(table).not.toBeNull();
    const rows = table?.querySelectorAll("[data-category-row]") ?? [];
    expect(rows).toHaveLength(5);
    expect(
      Array.from(rows).map((row) => row.getAttribute("data-category-row"))
    ).toEqual(FIVE_CODES);
    const total = table?.querySelector("[data-category-total]");
    expect(total?.textContent).toContain("Total");
    expect(total?.textContent).toContain("Rp999.000.000");
    expect(total?.textContent).not.toContain("Rp480.000.000");

    // Aggregate has no card target, so it offers no Previous/Next.
    expect(document.querySelector("[data-category-nav]")).toBeNull();
    expect(document.querySelector("[data-category-card]")).toBeNull();
  });

  it("carries no ghost or forecast treatment on actual category evidence", () => {
    renderBrowser();
    expect(document.querySelector("[data-estimated]")).toBeNull();
    expect(document.querySelector("[data-category-estimate]")).toBeNull();
    const text = document.body.textContent ?? "";
    expect(text).not.toContain("Estimated");
    expect(text).not.toContain("Forecast");
    expect(text).not.toContain("forecast");
  });

  it("renders zero rows as real zeros and Unassigned's null ratio as -", () => {
    const source = purchaseOrderReportFixture();
    source.categories = source.categories.map((row) =>
      row.businessCategory === "OTHERS"
        ? {
            ...row,
            poCount: 0,
            amountMinor: 0,
            costedMinor: 0,
            uncostedMinor: 0,
            capitalMinor: 0,
            marginMinor: 0,
            marginBps: null,
            coverageBps: null,
          }
        : row
    );
    renderBrowser({ readout: readoutFor(null, source) });
    const zero = document.querySelector('[data-category-row="OTHERS"]');
    expect(zero?.textContent).toContain("0");
    expect(zero?.textContent).toContain("Rp0");
    expect(zero?.textContent).toContain("-");
    expect(zero?.textContent).not.toContain("0.00%");
    const unassigned = document.querySelector(
      '[data-category-row="UNASSIGNED"]'
    );
    expect(unassigned?.textContent).toContain("-");
    expect(unassigned?.textContent).not.toContain("0.00%");
  });

  it("discloses uncosted amount in the aggregate without calling it margin", () => {
    renderBrowser();
    const note = document.querySelector(
      "[data-category-aggregate] [data-category-uncosted]"
    );
    expect(note?.textContent).toContain("not costed");
    expect(note?.textContent).toContain("never counted as margin");
  });
});

describe("category browser — explicit empty (issue #643)", () => {
  it("shows a choose-categories state and no invented zero metrics", () => {
    renderBrowser({ readout: readoutFor([]), codes: [] });
    const empty = document.querySelector("[data-category-empty]");
    expect(empty).not.toBeNull();
    expect(empty?.textContent).toContain("Choose categories");
    expect(empty?.textContent).toContain("No categories are selected");
    expect(
      document.querySelector('[data-category-picker-trigger]')
        ?.getAttribute("data-category-selection-summary")
    ).toBe("No categories selected");
    expect(document.querySelector("[data-category-metric]")).toBeNull();
    expect(document.querySelector("table")).toBeNull();
    expect(document.querySelector("[data-category-nav]")).toBeNull();
    expect(
      document.querySelector('[aria-label="All categories display"]')
    ).toBeNull();
  });
});

describe("category browser — single card (issue #643)", () => {
  it("renders one full-width card with the six vertical metric rows", () => {
    const readout = readoutFor(["FAB"]);
    renderBrowser({ readout, codes: ["FAB"] });
    const card = document.querySelector("[data-category-card]");
    expect(card).not.toBeNull();
    expect(card?.getAttribute("data-category-key")).toBe("FAB");
    expect(card?.getAttribute("data-category-active")).toBe("true");
    expect(
      card?.querySelector("[data-category-card-heading]")?.textContent
    ).toBe("Fabrication");
    const rows = Array.from(
      card?.querySelectorAll("[data-category-metric]") ?? []
    ).map((cell) => cell.getAttribute("data-category-metric"));
    expect(rows).toEqual([
      "POs",
      "PO amount",
      "Costed amount",
      "Capital",
      "PO margin",
      "PO margin %",
    ]);
    expect(metric(card as HTMLElement, "POs")?.textContent).toBe("3");
    expect(metric(card as HTMLElement, "PO amount")?.textContent).toBe(
      "Rp120.000.000"
    );
    expect(metric(card as HTMLElement, "Costed amount")?.textContent).toBe(
      "Rp120.000.000"
    );
    expect(metric(card as HTMLElement, "Capital")?.textContent).toBe(
      "Rp150.000.000"
    );
    const margin = metric(card as HTMLElement, "PO margin");
    expect(margin?.textContent).toBe("Rp-30.000.000");
    expect(margin?.className).toContain("text-danger-700");
    expect(metric(card as HTMLElement, "PO margin %")?.textContent).toBe(
      "-20.00%"
    );
    // Single presentation has no grid and no aggregate table.
    expect(document.querySelector("[data-comparison-grid]")).toBeNull();
    expect(document.querySelector("[data-category-aggregate]")).toBeNull();
    expect(document.querySelector("[data-category-nav]")).not.toBeNull();
  });

  it("clamps single navigation at the first and last supplied row", () => {
    renderBrowser({ readout: readoutFor(["FM"]), codes: ["FM"] });
    expect(
      document
        .querySelector('[data-category-nav="previous"]')
        ?.hasAttribute("disabled")
    ).toBe(true);
    expect(
      document
        .querySelector('[data-category-nav="next"]')
        ?.hasAttribute("disabled")
    ).toBe(false);

    renderBrowser({ readout: readoutFor(["UNASSIGNED"]), codes: ["UNASSIGNED"] });
    expect(
      document
        .querySelector('[data-category-nav="previous"]')
        ?.hasAttribute("disabled")
    ).toBe(false);
    expect(
      document
        .querySelector('[data-category-nav="next"]')
        ?.hasAttribute("disabled")
    ).toBe(true);
  });

  it("discloses uncosted only when the category holds uncosted amount", () => {
    renderBrowser({ readout: readoutFor(["FM"]), codes: ["FM"] });
    const note = document.querySelector("[data-category-uncosted]");
    expect(note?.textContent).toContain("Rp18.000.000");
    expect(note?.textContent).toContain("never counted as margin");

    renderBrowser({ readout: readoutFor(["FAB"]), codes: ["FAB"] });
    expect(document.querySelector("[data-category-uncosted]")).toBeNull();
  });

  it("renders - for every money cell when the currency is unresolved", () => {
    renderBrowser({
      readout: readoutFor(["FAB"]),
      codes: ["FAB"],
      entry: null,
    });
    const card = document.querySelector<HTMLElement>("[data-category-card]");
    expect(card).not.toBeNull();
    expect(metric(card!, "POs")?.textContent).toBe("3");
    for (const label of [
      "PO amount",
      "Costed amount",
      "Capital",
      "PO margin",
    ]) {
      expect(metric(card!, label)?.textContent).toBe("-");
    }
    expect(metric(card!, "PO margin %")?.textContent).toBe("-20.00%");
  });
});

describe("category browser — subset grids (issue #643)", () => {
  it("renders three categories as a pair plus a full-width final card", () => {
    const readout = readoutFor(THREE_CODES);
    renderBrowser({ readout, codes: THREE_CODES });
    const cards = document.querySelectorAll("[data-comparison-card]");
    expect(cards).toHaveLength(3);
    expect(
      document.querySelectorAll("[data-comparison-last-odd]")
    ).toHaveLength(1);
    expect(cards[2].hasAttribute("data-comparison-last-odd")).toBe(true);
    expect(cards[2].className).toContain("lg:col-span-2");
    expect(document.querySelector("[data-category-aggregate]")).toBeNull();
    expect(
      document.querySelector('[aria-label="All categories display"]')
    ).toBeNull();
  });

  it("renders four categories as two pairs with no odd final card", () => {
    const readout = readoutFor(FOUR_CODES);
    renderBrowser({ readout, codes: FOUR_CODES });
    expect(document.querySelectorAll("[data-comparison-card]")).toHaveLength(4);
    expect(document.querySelector("[data-comparison-last-odd]")).toBeNull();
  });

  it("renders All/Individual as two pairs plus a full-width fifth card", () => {
    const readout = readoutFor(null);
    renderBrowser({ readout, codes: FIVE_CODES, layout: "individual" });
    const cards = document.querySelectorAll("[data-comparison-card]");
    expect(cards).toHaveLength(5);
    expect(cards[4].hasAttribute("data-comparison-last-odd")).toBe(true);
    expect(
      document
        .querySelector('[data-category-layout="individual"]')
        ?.getAttribute("aria-pressed")
    ).toBe("true");
    expect(document.querySelector("[data-category-nav]")).not.toBeNull();
    expect(document.querySelector("[data-category-aggregate]")).toBeNull();
    expect(document.querySelectorAll("[data-category-card]")).toHaveLength(5);
  });

  it("keeps every selected card in supplied order even when selection is unsorted", () => {
    renderBrowser({
      readout: readoutFor(["FAB", "FM"]),
      codes: ["FM", "FAB"],
    });
    const keys = Array.from(
      document.querySelectorAll("[data-category-card]")
    ).map((card) => card.getAttribute("data-category-key"));
    expect(keys).toEqual(["FM", "FAB"]);
  });
});

describe("category browser — labels (issue #643)", () => {
  it("falls back to the code for an empty label and disambiguates duplicates", () => {
    const source = purchaseOrderReportFixture();
    source.categories = source.categories.map((row) => {
      if (row.businessCategory === "FM") return { ...row, label: "   " };
      if (row.businessCategory === "FAB") return { ...row, label: "Shared" };
      if (row.businessCategory === "SUPPLY_GEN")
        return { ...row, label: "Shared" };
      return row;
    });
    renderBrowser({ readout: readoutFor(null, source) });
    expect(
      document.querySelector('[data-category-row="FM"]')?.textContent
    ).toContain("FM");
    expect(
      document.querySelector('[data-category-row="FAB"]')?.textContent
    ).toContain("Shared (FAB)");
    expect(
      document.querySelector('[data-category-row="SUPPLY_GEN"]')?.textContent
    ).toContain("Shared (SUPPLY_GEN)");
  });
});

describe("category browser — source states (issue #643)", () => {
  it("renders a loading state with no money and a disabled trigger", () => {
    const readout = resolveCategoryReadout({
      overview: purchaseOrderReportFixture(),
      source: null,
      status: "loading",
      error: "",
      explicitCodes: null,
    });
    renderBrowser({ readout, codes: [] });
    expect(document.body.textContent).toContain("Loading categories");
    expect(
      document.querySelector<HTMLButtonElement>("[data-category-picker-trigger]")
        ?.disabled
    ).toBe(true);
    expect(document.querySelector("[data-category-metric]")).toBeNull();
    expect(document.querySelector("[data-category-aggregate]")).toBeNull();
    expect(document.body.textContent).not.toContain("Rp");
  });

  it("renders an error with Retry and never a zero", () => {
    const readout = resolveCategoryReadout({
      overview: purchaseOrderReportFixture(),
      source: null,
      status: "error",
      error: "The all-category API is unreachable.",
      explicitCodes: ["FAB"],
    });
    renderBrowser({ readout, codes: [] });
    expect(document.body.textContent).toContain(
      "The all-category API is unreachable."
    );
    expect(
      document.querySelector('button[aria-label="Retry categories"]')
    ).not.toBeNull();
    expect(
      document.querySelector<HTMLButtonElement>("[data-category-picker-trigger]")
        ?.disabled
    ).toBe(true);
    expect(document.querySelector("[data-category-metric]")).toBeNull();
    expect(document.body.textContent).not.toContain("Rp");
  });

  it("surfaces a foreign currency or range source as an error, never zero", () => {
    const overview = purchaseOrderReportFixture();
    const foreign = purchaseOrderReportFixture();
    foreign.filter = { ...foreign.filter, currency: "USD" };
    const readout = resolveCategoryReadout({
      overview,
      source: foreign,
      status: "ready",
      error: "",
      explicitCodes: null,
    });
    expect(readout.status).toBe("error");
    renderBrowser({ readout, codes: [] });
    expect(document.body.textContent).toContain(
      "does not match this report's range, bucket, currency or all-category scope"
    );
    expect(document.querySelector("[data-category-metric]")).toBeNull();
    expect(document.querySelector("[data-category-row]")).toBeNull();
    expect(document.body.textContent).not.toContain("Rp");
  });

  it("shows a business-date notice while keeping the loaded evidence", () => {
    const overview = purchaseOrderReportFixture();
    const stale = purchaseOrderReportFixture();
    stale.asOf = "2026-11-20";
    const readout = resolveCategoryReadout({
      overview,
      source: stale,
      status: "ready",
      error: "",
      explicitCodes: null,
    });
    expect(readout.status).toBe("ready");
    expect(readout.scopeMatchesOverview).toBe(false);
    renderBrowser({ readout });
    expect(document.body.textContent).toContain(
      "different business date from the overview"
    );
    expect(document.querySelector("[data-category-aggregate]")).not.toBeNull();
    expect(
      document.querySelector('[data-category-row="FAB"]')?.textContent
    ).toContain("Rp120.000.000");
  });
});

describe("category card and aggregate direct exports (issue #643)", () => {
  it("renders the card and aggregate as standalone callable pieces", () => {
    const readout = readoutFor(null);
    const fab = readout.supplied.find((item) => item.code === "FAB");
    if (fab === undefined) throw new Error("no FAB item");
    document.body.innerHTML = renderToString(
      <PurchaseOrderReportCategoryCard
        item={fab}
        readout={readout}
        entry={IDR}
        active
      />
    );
    expect(document.querySelector("[data-category-card]")).not.toBeNull();
    expect(document.querySelector("[data-category-metric]")).not.toBeNull();

    document.body.innerHTML = renderToString(
      <PurchaseOrderReportCategoryAggregate
        readout={readout}
        entry={IDR}
        onRetry={() => undefined}
      />
    );
    expect(document.querySelector("[data-category-aggregate]")).not.toBeNull();
    expect(
      document.querySelector('table[aria-label="Category totals"]')
    ).not.toBeNull();
  });
});

// Issue #645: the category surface must say when its retained snapshot is no
// longer current, keep the same-scope rows while a re-read is in flight or has
// failed, and never show old money under a new scope.
describe("category freshness states (issue #645)", () => {
  it("labels a refreshing snapshot and keeps the same-scope rows", () => {
    const source = purchaseOrderReportFixture();
    const readout = resolveCategoryReadout({
      overview: source,
      source,
      status: "ready",
      error: "",
      explicitCodes: null,
      stale: true,
      refreshing: true,
    });
    renderBrowser({ readout });

    const badge = document.querySelector("[data-category-stale]");
    expect(badge).not.toBeNull();
    expect(badge?.getAttribute("data-category-refreshing")).toBe("true");
    expect(badge?.textContent).toContain("Updating…");
    expect(
      document.querySelector('[data-category-row="FAB"]')?.textContent
    ).toContain("Rp120.000.000");
    expect(
      document.querySelector<HTMLButtonElement>("[data-category-picker-trigger]")
        ?.disabled
    ).toBe(false);
  });

  it("keeps the previous rows with an inline refresh error and Retry", () => {
    const source = purchaseOrderReportFixture();
    const readout = resolveCategoryReadout({
      overview: source,
      source,
      status: "ready",
      error: "The all-category API is unreachable.",
      explicitCodes: null,
      stale: true,
    });
    renderBrowser({ readout });

    const badge = document.querySelector("[data-category-stale]");
    expect(badge?.getAttribute("data-category-refreshing")).toBe("false");
    expect(badge?.textContent).toContain("Stale");
    expect(document.body.textContent).toContain(
      "The categories could not be refreshed: The all-category API is unreachable."
    );
    expect(document.body.textContent).toContain("Showing the previous figures.");
    expect(
      document.querySelector('button[aria-label="Retry categories"]')
    ).not.toBeNull();
    expect(
      document.querySelector('[data-category-row="FAB"]')?.textContent
    ).toContain("Rp120.000.000");
    expect(document.body.textContent).toContain("Rp120.000.000");
  });

  it("shows pure loading with no old money for a different scope", () => {
    const readout = resolveCategoryReadout({
      overview: purchaseOrderReportFixture(),
      source: null,
      status: "loading",
      error: "",
      explicitCodes: ["FAB"],
    });
    renderBrowser({ readout, codes: ["FAB"] });

    expect(document.body.textContent).toContain("Loading categories…");
    expect(document.querySelector("[data-category-stale]")).toBeNull();
    expect(document.querySelector("[data-category-row]")).toBeNull();
    expect(document.querySelector("[data-category-card]")).toBeNull();
    expect(document.body.textContent).not.toContain("Rp");
    expect(
      document.querySelector<HTMLButtonElement>("[data-category-picker-trigger]")
        ?.disabled
    ).toBe(true);
  });
});
