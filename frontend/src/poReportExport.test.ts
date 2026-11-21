import { describe, expect, it } from "vitest";
import {
  buildPurchaseOrderReportView,
  REPORT_VIEW_VERSION,
  type PurchaseOrderReportViewMonth,
} from "./poReportExport";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
} from "./poReportFixtures";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
} from "./poReport";
import { allMonthsOfYear } from "./poReportComparison";

function clone(report: PurchaseOrderReportData): PurchaseOrderReportData {
  return structuredClone(report);
}

function zeroMetrics(): PurchaseOrderReportMetrics {
  return {
    poCount: 0,
    amountMinor: 0,
    costedMinor: 0,
    uncostedMinor: 0,
    capitalMinor: 0,
    marginMinor: 0,
    marginBps: null,
    coverageBps: null,
  };
}

function annualSourceFor(
  year: number,
  template: PurchaseOrderReportData
): PurchaseOrderReportData {
  const source = clone(template);
  source.filter = {
    from: `${year}-01-01`,
    to: `${year + 1}-01-01`,
    bucket: "month",
    category: "all",
    currency: "IDR",
  };
  source.periods = Array.from({ length: 12 }, (_, index) => ({
    periodStart: `${year}-${String(index + 1).padStart(2, "0")}-01`,
    partial: false,
    inProgress: false,
    ...zeroMetrics(),
  }));
  source.totals = zeroMetrics();
  return source;
}

function monthEntry(
  month: string,
  source: PurchaseOrderReportData,
  usesOverviewMonthSummary = false
): PurchaseOrderReportViewMonth {
  return { month, source, usesOverviewMonthSummary };
}

describe("version-1 export envelope (issue #641/#642)", () => {
  it("reuses the daily overview as the current month source", () => {
    const overview = purchaseOrderReportMonthFixture();
    const view = buildPurchaseOrderReportView(overview, {
      months: [monthEntry("2026-11", overview, true)],
      layout: "individual",
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
      months: [monthEntry("2026-10", annual)],
      layout: "individual",
    });
    expect(view.sources).toEqual([overview, annual]);
    expect(view.periods).toEqual({
      sources: [1],
      months: ["2026-10"],
      layout: "individual",
    });
    expect("currentMonthSource" in view.periods).toBe(false);

    const second = buildPurchaseOrderReportView(overview, {
      months: [monthEntry("2026-09", annual)],
      layout: "individual",
    });
    expect(second.sources).toHaveLength(2);
    expect(second.periods.sources).toEqual([1]);
    expect(second.periods.months).toEqual(["2026-09"]);
  });

  it("uses an exact annual overview without duplicating it", () => {
    const overview = purchaseOrderReportYearFixture();
    const view = buildPurchaseOrderReportView(overview, {
      months: [monthEntry("2026-10", overview)],
      layout: "individual",
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
        months: [monthEntry("2026-10", annual)],
        layout: "individual",
      })
    ).toThrow(/does not share/);

    const otherCurrency = purchaseOrderReportYearFixture();
    otherCurrency.filter.currency = "USD";
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [monthEntry("2026-10", otherCurrency)],
        layout: "individual",
      })
    ).toThrow(/does not share/);
  });

  it("refuses a non-canonical month and a current-month claim from another source", () => {
    const overview = purchaseOrderReportMonthFixture();
    const annual = purchaseOrderReportYearFixture();
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [monthEntry("2026-1", overview)],
        layout: "individual",
      })
    ).toThrow(/canonical/);
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [monthEntry("2026-11", annual, true)],
        layout: "individual",
      })
    ).toThrow(/overview's own monthSummary/);
  });

  it("serializes exactly the frozen shape, omitting an absent currentMonthSource", () => {
    const overview = purchaseOrderReportFixture();
    const annual = purchaseOrderReportYearFixture();
    const view = buildPurchaseOrderReportView(overview, {
      months: [monthEntry("2026-10", annual)],
      layout: "individual",
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

  it("builds an explicit empty period section that keeps the overview categories", () => {
    const overview = purchaseOrderReportYearFixture();
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
    });
    expect(view.sources).toEqual([overview]);
    expect(view.periods).toEqual({
      sources: [],
      months: [],
      layout: "individual",
    });
    expect(view.categories).toEqual({
      source: 0,
      codes: overview.categories.map((row) => row.businessCategory),
      layout: "aggregate",
    });
  });

  it("canonicalizes a cross-year subset and reuses each annual source once", () => {
    const overview = purchaseOrderReportMonthFixture();
    const annual2025 = annualSourceFor(2025, overview);
    const view = buildPurchaseOrderReportView(overview, {
      months: [
        monthEntry("2026-11", overview, true),
        monthEntry("2025-12", annual2025),
      ],
      layout: "individual",
    });
    expect(view.periods.months).toEqual(["2025-12", "2026-11"]);
    expect(view.periods.sources).toEqual([1, 0]);
    expect(view.periods.currentMonthSource).toBe(0);
    expect(view.sources).toEqual([overview, annual2025]);
    // The override appears once even though the envelope has two sources.
    expect(
      JSON.parse(JSON.stringify(view)).periods.currentMonthSource
    ).toBe(0);
  });

  it("uses the annual server snapshot for a one-year Aggregate without a row sum", () => {
    const overview = purchaseOrderReportYearFixture();
    overview.totals = { ...overview.totals, amountMinor: 777_777_777 };
    const view = buildPurchaseOrderReportView(overview, {
      months: allMonthsOfYear("2026").map((month) =>
        monthEntry(month, overview)
      ),
      layout: "aggregate",
    });
    expect(view.periods.layout).toBe("aggregate");
    expect(view.periods.months).toEqual(allMonthsOfYear("2026"));
    expect(view.periods.sources).toEqual([0]);
    expect("currentMonthSource" in view.periods).toBe(false);
    expect(view.sources[0].totals.amountMinor).toBe(777_777_777);
  });

  it("keeps All-Individual chronological with one current-month override at most", () => {
    const overview = purchaseOrderReportMonthFixture();
    const annual = purchaseOrderReportYearFixture();
    const months = allMonthsOfYear("2026").map((month) =>
      month === "2026-11"
        ? monthEntry(month, overview, true)
        : monthEntry(month, annual)
    );
    const view = buildPurchaseOrderReportView(overview, {
      months,
      layout: "individual",
    });
    expect(view.periods.months).toEqual(allMonthsOfYear("2026"));
    expect(view.periods.sources).toEqual([1, 0]);
    expect(view.periods.currentMonthSource).toBe(0);
    expect(view.sources).toEqual([overview, annual]);
  });

  it("refuses Aggregate months that are not one calendar year's twelve", () => {
    const overview = purchaseOrderReportYearFixture();
    const eleven = allMonthsOfYear("2026").slice(0, 11);
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: eleven.map((month) => monthEntry(month, overview)),
        layout: "aggregate",
      })
    ).toThrow(/twelve/);

    const crossYear = [
      ...allMonthsOfYear("2026").slice(1),
      "2027-01",
    ];
    const annual2027 = annualSourceFor(2027, overview);
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: crossYear.map((month) =>
          monthEntry(
            month,
            month.startsWith("2027") ? annual2027 : overview
          )
        ),
        layout: "aggregate",
      })
    ).toThrow(/calendar year/);

    const zeroYear = annualSourceFor(2025, overview);
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: allMonthsOfYear("2026").map((month) =>
          monthEntry(month, zeroYear)
        ),
        layout: "aggregate",
      })
    ).toThrow(/annual/);
  });

  it("refuses a duplicated month", () => {
    const overview = purchaseOrderReportYearFixture();
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [
          monthEntry("2026-10", overview),
          monthEntry("2026-10", overview),
        ],
        layout: "individual",
      })
    ).toThrow(/twice/);
  });

  it("refuses a source that does not cover the selected month's year", () => {
    const overview = purchaseOrderReportYearFixture();
    const otherYear = annualSourceFor(2025, overview);
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [monthEntry("2026-01", otherYear)],
        layout: "individual",
      })
    ).toThrow(/exact annual/);
  });
});
