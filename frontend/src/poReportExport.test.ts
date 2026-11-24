import { describe, expect, it } from "vitest";
import {
  buildPurchaseOrderReportView,
  REPORT_VIEW_VERSION,
  type PurchaseOrderReportViewCategorySelection,
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
import { allMonthsOfYear, categorySourceQuery } from "./poReportComparison";

function clone(report: PurchaseOrderReportData): PurchaseOrderReportData {
  return structuredClone(report);
}

function zeroMetrics(): PurchaseOrderReportMetrics {
  return {
    poCount: 0,
    amountMinor: 0,
    costedMinor: 0,
    uncostedMinor: 0,
    extraExpenseMinor: 0,
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

// Issue #643: the explicit category selection inside the same version-1
// envelope. An absent selection preserves the #641/#642 bytes exactly.
function restrictedCategoryOverview(category: string): {
  overview: PurchaseOrderReportData;
  universe: PurchaseOrderReportData;
} {
  const base = purchaseOrderReportFixture();
  const overview = clone(base);
  overview.filter = { ...base.filter, category };
  overview.categories = base.categories.filter(
    (row) => row.businessCategory === category
  );
  const universe = clone(base);
  universe.filter = categorySourceQuery(overview.filter);
  return { overview, universe };
}

function categorySelection(
  source: PurchaseOrderReportData,
  codes: readonly string[],
  layout: "aggregate" | "individual"
): PurchaseOrderReportViewCategorySelection {
  return { source, codes, layout };
}

const ALL_CODES = ["FM", "FAB", "SUPPLY_GEN", "OTHERS", "UNASSIGNED"];

describe("category comparison export envelope (issue #643)", () => {
  it("keeps the legacy full-overview category section when categories is absent", () => {
    const overview = purchaseOrderReportFixture();
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
    });
    expect(view.categories).toEqual({
      source: 0,
      codes: overview.categories.map((row) => row.businessCategory),
      layout: "aggregate",
    });
    expect(Object.keys(view.categories).sort()).toEqual([
      "codes",
      "layout",
      "source",
    ]);
  });

  it("builds an explicit empty category section without a financial source", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");
    const wrongCurrency = clone(universe);
    wrongCurrency.filter.currency = "USD";
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
      categories: categorySelection(wrongCurrency, [], "aggregate"),
    });
    expect(view.categories).toEqual({
      source: 0,
      codes: [],
      layout: "individual",
    });
    expect(view.sources).toEqual([overview]);
    expect(view.periods).toEqual({
      sources: [],
      months: [],
      layout: "individual",
    });
  });

  it("adds the all-category source once for a single restricted selection", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
      categories: categorySelection(universe, ["FAB"], "individual"),
    });
    expect(view.sources).toEqual([overview, universe]);
    expect(view.categories).toEqual({
      source: 1,
      codes: ["FAB"],
      layout: "individual",
    });
  });

  it("canonicalizes an unsorted subset into supplied source order", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");
    const input = ["UNASSIGNED", "FM", "FAB"];
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
      categories: categorySelection(universe, input, "individual"),
    });
    expect(view.categories).toEqual({
      source: 1,
      codes: ["FM", "FAB", "UNASSIGNED"],
      layout: "individual",
    });
    // The caller's array is never mutated.
    expect(input).toEqual(["UNASSIGNED", "FM", "FAB"]);
  });

  it("accepts the full supplied set in Individual layout", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");
    const reversed = [...ALL_CODES].reverse();
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
      categories: categorySelection(universe, reversed, "individual"),
    });
    expect(view.categories).toEqual({
      source: 1,
      codes: ALL_CODES,
      layout: "individual",
    });
  });

  it("accepts the full supplied set in Aggregate layout", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");
    const shuffled = ["OTHERS", "FM", "UNASSIGNED", "FAB", "SUPPLY_GEN"];
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
      categories: categorySelection(universe, shuffled, "aggregate"),
    });
    expect(view.categories).toEqual({
      source: 1,
      codes: ALL_CODES,
      layout: "aggregate",
    });
    expect(view.sources).toHaveLength(2);
  });

  it("reuses source 0 when the all-category source is the overview itself", () => {
    const overview = purchaseOrderReportFixture();
    const duplicate = clone(overview);
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
      categories: categorySelection(duplicate, ["FAB"], "individual"),
    });
    expect(view.sources).toEqual([overview]);
    expect(view.categories).toEqual({
      source: 0,
      codes: ["FAB"],
      layout: "individual",
    });
  });

  it("accepts a #641 restricted overview as its own full-row category source", () => {
    const { overview } = restrictedCategoryOverview("FM");
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
      categories: categorySelection(overview, ["FM"], "aggregate"),
    });
    expect(view.sources).toEqual([overview]);
    expect(view.categories).toEqual({
      source: 0,
      codes: ["FM"],
      layout: "aggregate",
    });
  });

  it("refuses a subset in Aggregate layout", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(universe, ["FM", "FAB"], "aggregate"),
      })
    ).toThrow(/Aggregate requires every category/);
  });

  it("refuses duplicate, unknown and non-canonical codes", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(universe, ["FM", "FM"], "individual"),
      })
    ).toThrow(/selected twice/);
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(universe, ["FM", "NOPE"], "individual"),
      })
    ).toThrow(/not present/);
    // Non-canonical order is canonicalized, never silently dropped; an
    // Aggregate subset remains refused after canonicalization.
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(
          universe,
          ["FAB", "FM"],
          "aggregate"
        ),
      })
    ).toThrow(/Aggregate requires every category/);
  });

  it("refuses a foreign scope or as-of", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");

    const stale = clone(universe);
    stale.asOf = "2026-11-18";
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(stale, ["FAB"], "individual"),
      })
    ).toThrow(/as-of/);

    const wrongCurrency = clone(universe);
    wrongCurrency.filter.currency = "USD";
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(wrongCurrency, ["FAB"], "individual"),
      })
    ).toThrow(/does not match/);

    const wrongRange = clone(universe);
    wrongRange.filter.from = "2026-01-02";
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(wrongRange, ["FAB"], "individual"),
      })
    ).toThrow(/does not match/);

    const foreign = clone(universe);
    foreign.filter.category = "FAB";
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(foreign, ["FAB"], "individual"),
      })
    ).toThrow(/does not match/);

    const restrictedSource = restrictedCategoryOverview("FAB").overview;
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months: [],
        layout: "aggregate",
        categories: categorySelection(
          restrictedSource,
          ["FAB"],
          "individual"
        ),
      })
    ).toThrow(/does not match/);
  });

  it("refuses more than sixteen sources", () => {
    const overview = purchaseOrderReportFixture();
    const years = Array.from({ length: 16 }, (_, index) => 2000 + index);
    const months = years.map((year) => {
      const source = annualSourceFor(year, overview);
      return monthEntry(`${year}-06`, source);
    });
    expect(() =>
      buildPurchaseOrderReportView(overview, {
        months,
        layout: "individual",
      })
    ).toThrow(/16 sources/);
  });

  it("serializes exactly the frozen category keys with no version bump", () => {
    const { overview, universe } = restrictedCategoryOverview("FM");
    const view = buildPurchaseOrderReportView(overview, {
      months: [],
      layout: "aggregate",
      categories: categorySelection(universe, ["FAB"], "individual"),
    });
    const parsed = JSON.parse(JSON.stringify(view)) as Record<string, unknown>;
    expect(parsed.version).toBe(1);
    expect(parsed.categories).toEqual({
      source: 1,
      codes: ["FAB"],
      layout: "individual",
    });
    expect(Object.keys(parsed.categories as object).sort()).toEqual([
      "codes",
      "layout",
      "source",
    ]);
    expect(Object.keys(parsed).sort()).toEqual([
      "categories",
      "overview",
      "periods",
      "sources",
      "version",
    ]);
  });
});
