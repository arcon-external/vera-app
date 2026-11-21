import { describe, expect, it } from "vitest";
import {
  buildPurchaseOrderReportView,
  REPORT_VIEW_VERSION,
} from "./poReportExport";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
} from "./poReportFixtures";

describe("version-1 export envelope (issue #641)", () => {
  it("reuses the daily overview as the current month source", () => {
    const overview = purchaseOrderReportMonthFixture();
    const view = buildPurchaseOrderReportView(overview, {
      month: "2026-11",
      source: overview,
      usesOverviewMonthSummary: true,
    });
    expect(view.version).toBe(REPORT_VIEW_VERSION);
    expect(view.sources).toEqual([overview]);
    expect(view.overview).toBe(0);
    expect(view.periods).toEqual({
      sources: [0],
      months: ["2026-11"],
      layout: "individual",
      currentMonthSource: 0,
    });
    expect(view.categories.layout).toBe("aggregate");
    expect(view.categories.source).toBe(0);
    expect(view.categories.codes).toEqual(
      overview.categories.map((row) => row.businessCategory)
    );
  });

  it("adds one annual source for a selected past month and reuses it by index", () => {
    const overview = purchaseOrderReportFixture();
    const annual = purchaseOrderReportYearFixture();
    const view = buildPurchaseOrderReportView(overview, {
      month: "2026-10",
      source: annual,
      usesOverviewMonthSummary: false,
    });
    expect(view.sources).toEqual([overview, annual]);
    expect(view.periods).toEqual({
      sources: [1],
      months: ["2026-10"],
      layout: "individual",
    });
    expect("currentMonthSource" in view.periods).toBe(false);

    const second = buildPurchaseOrderReportView(overview, {
      month: "2026-09",
      source: annual,
      usesOverviewMonthSummary: false,
    });
    expect(second.sources).toHaveLength(2);
    expect(second.periods.sources).toEqual([1]);
    expect(second.periods.months).toEqual(["2026-09"]);
  });

  it("uses an exact annual overview without duplicating it", () => {
    const overview = purchaseOrderReportYearFixture();
    const view = buildPurchaseOrderReportView(overview, {
      month: "2026-10",
      source: overview,
      usesOverviewMonthSummary: false,
    });
    expect(view.sources).toEqual([overview]);
    expect(view.periods.sources).toEqual([0]);
    expect("currentMonthSource" in view.periods).toBe(false);
  });

  it("refuses a snapshot whose as-of, category or currency differs", () => {
    const overview = purchaseOrderReportFixture();
    const annual = purchaseOrderReportYearFixture();
    annual.asOf = "2026-11-20";
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        month: "2026-10",
        source: annual,
        usesOverviewMonthSummary: false,
      })
    ).toThrow(/does not share/);

    const otherCurrency = purchaseOrderReportYearFixture();
    otherCurrency.filter.currency = "USD";
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        month: "2026-10",
        source: otherCurrency,
        usesOverviewMonthSummary: false,
      })
    ).toThrow(/does not share/);
  });

  it("refuses a non-canonical month and a current-month claim from another source", () => {
    const overview = purchaseOrderReportMonthFixture();
    const annual = purchaseOrderReportYearFixture();
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        month: "2026-1",
        source: overview,
        usesOverviewMonthSummary: false,
      })
    ).toThrow(/canonical/);
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        month: "2026-11",
        source: annual,
        usesOverviewMonthSummary: true,
      })
    ).toThrow(/overview's own monthSummary/);
  });

  it("serializes exactly the frozen shape, omitting an absent currentMonthSource", () => {
    const overview = purchaseOrderReportFixture();
    const annual = purchaseOrderReportYearFixture();
    const view = buildPurchaseOrderReportView(overview, {
      month: "2026-10",
      source: annual,
      usesOverviewMonthSummary: false,
    });
    const parsed = JSON.parse(JSON.stringify(view)) as Record<string, unknown>;
    expect(parsed.version).toBe(1);
    expect(parsed).not.toHaveProperty("selection");
    expect(parsed.periods).not.toHaveProperty("currentMonthSource");
    expect(Object.keys(parsed).sort()).toEqual([
      "categories",
      "overview",
      "periods",
      "sources",
      "version",
    ]);
  });
});
