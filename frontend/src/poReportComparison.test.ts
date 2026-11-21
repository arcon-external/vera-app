import { describe, expect, it } from "vitest";
import {
  MAX_COMPARISON_MONTHS,
  MAX_COMPARISON_MONTH_SPAN,
  allMonthsOfYear,
  allYearForMonths,
  allYearSelection,
  annualMonthQuery,
  annualSourceKey,
  canonicalMonths,
  categoryDisplayLabels,
  categorySelectionSummary,
  categorySourceMatchesOverview,
  categorySourceQuery,
  categorySourceRequest,
  initialPickerYear,
  isAllCategoriesSelected,
  monthKeyFromDate,
  monthLabel,
  monthSelectionSummary,
  monthSpanCount,
  monthStart,
  monthUnavailableLabel,
  nextActiveCategory,
  nextActiveMonth,
  nextMonthStart,
  overviewSuppliesAllCategories,
  reportSourceKey,
  resolveCategoryReadout,
  resolveMonthAggregate,
  resolveMonthReadout,
  resolveMonthSource,
  resolveSelectedCategoryCodes,
  selectAllCategories,
  selectionLimitViolation,
  shiftMonth,
  sourceScopeMatches,
  toggleCategoryInSelection,
  toggleMonthInSelection,
} from "./poReportComparison";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
  purchaseOrderReportYearWithRecordedFutureFixture,
} from "./poReportFixtures";
import type {
  PurchaseOrderReportCategory,
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
  PurchaseOrderReportQuery,
} from "./poReport";
import { reportFilterKey, reportRangeLabel } from "./poReportPage";

function clone(report: PurchaseOrderReportData): PurchaseOrderReportData {
  return structuredClone(report);
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

function recordedEvidence(): PurchaseOrderReportMetrics {
  return {
    poCount: 1,
    amountMinor: 777_000,
    costedMinor: 700_000,
    uncostedMinor: 77_000,
    capitalMinor: 600_000,
    marginMinor: 100_000,
    marginBps: 1667,
    coverageBps: 9009,
  };
}

describe("month source identity (issue #641)", () => {
  it("uses the exact complete-year monthly query, not a partial preset", () => {
    const annual = annualMonthQuery(2026, "FM", "idr");
    expect(annual).toEqual({
      from: "2026-01-01",
      to: "2027-01-01",
      bucket: "month",
      category: "FM",
      currency: "IDR",
    });
    const partial = { ...annual, to: "2026-11-21" };
    expect(annualSourceKey(annual, "2026-11-19")).not.toBe(
      annualSourceKey(partial, "2026-11-19")
    );
    expect(reportFilterKey(annual)).not.toBe(reportFilterKey(partial));
  });

  it("moves months across December and January", () => {
    expect(shiftMonth("2026-12", 1)).toBe("2027-01");
    expect(shiftMonth("2027-01", -1)).toBe("2026-12");
    expect(shiftMonth("2026-11", 0)).toBe("2026-11");
    expect(monthStart("2026-02")).toBe("2026-02-01");
    expect(nextMonthStart("2026-12")).toBe("2027-01-01");
    expect(monthKeyFromDate("2026-11-19")).toBe("2026-11");
    expect(monthLabel("2026-11")).toBe("November 2026");
  });

  it("does not fetch an unused annual source for a daily current month", () => {
    const overview = purchaseOrderReportMonthFixture();
    const request = annualMonthQuery(2026, "all", "IDR");
    expect(annualSourceKey(request, overview.asOf)).toBe(
      `${reportFilterKey(request)}#${overview.asOf}`
    );
    const resolution = resolveMonthSource({
      overview,
      month: "2026-11",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(resolution.status).toBe("ready");
    expect(resolution.source).toBe(overview);
    expect(resolution.usesOverviewMonthSummary).toBe(true);
    expect(resolution.sourceLabel).toBe("Performance snapshot");
  });

  it("uses an exact annual overview as its own monthly source", () => {
    const year = purchaseOrderReportYearFixture();
    const resolution = resolveMonthSource({
      overview: year,
      month: "2026-10",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(resolution.status).toBe("ready");
    expect(resolution.source).toBe(year);
    expect(resolution.usesOverviewMonthSummary).toBe(false);
  });
});

describe("month readout (issue #641)", () => {
  it("reads the current month from the daily overview and keeps future evidence separate", () => {
    const overview = purchaseOrderReportMonthFixture();
    const recorded = recordedEvidence();
    overview.monthSummary!.recordedFuture = recorded;
    const readout = resolveMonthReadout({
      overview,
      month: "2026-11",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(readout.status).toBe("ready");
    expect(readout.primary).toEqual(overview.monthSummary!.actual);
    expect(readout.primaryLabel).toBe("Actual to date");
    expect(readout.recordedFuture).toEqual(recorded);
    expect(readout.estimate?.kind).toBe("remainder");
    expect(readout.estimate?.base).toBe(110_000_000);
    expect(readout.estimate?.base).not.toBe(recorded.amountMinor);
    expect(readout.usesOverviewMonthSummary).toBe(true);
    expect(readout.scopeMatchesOverview).toBe(true);
  });

  it("keeps the annual summary's actual when the year guard is unavailable", () => {
    const overview = clone(purchaseOrderReportMonthFixture());
    // Force the annual path by removing the day overview's month summary.
    delete overview.monthSummary;
    const annual = purchaseOrderReportYearFixture();
    annual.monthSummary!.projection = {
      status: "unavailable",
      reason: "insufficient_months",
      method: annual.monthSummary!.projection.method,
    };
    const readout = resolveMonthReadout({
      overview,
      month: "2026-11",
      annualSource: annual,
      annualStatus: "ready",
      annualError: "",
    });
    expect(readout.status).toBe("ready");
    expect(readout.sourceLabel).toBe("Monthly snapshot");
    expect(readout.primary).toEqual(annual.monthSummary!.actual);
    expect(readout.estimate).toBeNull();
    expect(readout.unavailableReason).toBe("insufficient_months");
    expect(monthUnavailableLabel(readout.unavailableReason)).toContain(
      "Not enough complete months"
    );
  });

  it("prefers a daily overview when the year source is unavailable", () => {
    const overview = purchaseOrderReportMonthFixture();
    const annual = purchaseOrderReportYearFixture();
    annual.currentProjection = {
      ...annual.currentProjection,
      mode: "month",
      status: "unavailable",
      reason: "insufficient_months",
      buckets: [],
      currentMonth: undefined,
    };
    annual.monthSummary!.projection = {
      status: "unavailable",
      reason: "insufficient_months",
      method: annual.monthSummary!.projection.method,
    };
    const readout = resolveMonthReadout({
      overview,
      month: "2026-11",
      annualSource: annual,
      annualStatus: "ready",
      annualError: "",
    });
    // The day path wins, so the table agrees with the daily chart.
    expect(readout.usesOverviewMonthSummary).toBe(true);
    expect(readout.primary).toEqual(overview.monthSummary!.actual);
    expect(readout.estimate?.kind).toBe("remainder");
    expect(readout.unavailableReason).toBe("");
  });

  it("renders a loaded zero past month as genuine zeros with an unknown margin %", () => {
    // The overview is deliberately the partial "this year" snapshot, so the
    // selected past month needs its own annual source.
    const overview = purchaseOrderReportFixture();
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
      overview,
      month: "2026-01",
      annualSource: annual,
      annualStatus: "ready",
      annualError: "",
    });
    expect(readout.status).toBe("ready");
    expect(readout.primary).toMatchObject(zeroMetrics());
    expect(readout.primaryLabel).toBe("Recorded");
    expect(readout.primary?.marginBps).toBeNull();
    expect(readout.estimate).toBeNull();
  });

  it("uses only the current-year projection bucket for a future month", () => {
    const overview = purchaseOrderReportYearFixture();
    const readout = resolveMonthReadout({
      overview,
      month: "2026-12",
      annualSource: overview,
      annualStatus: "ready",
      annualError: "",
    });
    expect(readout.primary).toBeNull();
    expect(readout.estimate).toMatchObject({
      kind: "forecast",
      low: 40_000_000,
      base: 50_000_000,
      high: 60_000_000,
      capital: 40_000_000,
      margin: 10_000_000,
      sampleSize: 3,
      source: "trailing_complete_months",
    });
    expect(readout.unavailableReason).toBe("");
  });

  it("never falls back to the retired projection for a month beyond the current year", () => {
    const overview = purchaseOrderReportYearFixture();
    const nextYear = annualSourceFor(2027, overview);
    nextYear.periods[1] = {
      periodStart: "2027-02-01",
      partial: false,
      inProgress: false,
      ...recordedEvidence(),
    };
    nextYear.projection.monthly.buckets = [
      {
        periodStart: "2027-02-01",
        amountLowMinor: 777_777_777,
        amountBaseMinor: 777_777_777,
        amountHighMinor: 777_777_777,
        capitalMinor: 0,
        marginMinor: 777_777_777,
        marginBps: null,
        sampleSize: 1,
        source: "same_calendar_month",
      },
    ];
    const readout = resolveMonthReadout({
      overview,
      month: "2027-02",
      annualSource: nextYear,
      annualStatus: "ready",
      annualError: "",
    });
    expect(readout.status).toBe("ready");
    expect(readout.primary).toMatchObject(recordedEvidence());
    expect(readout.primaryLabel).toBe("Recorded");
    expect(readout.estimate).toBeNull();
    expect(readout.unavailableReason).toBe("outside_current_year");
    expect(monthUnavailableLabel(readout.unavailableReason)).toContain(
      "beyond the current year"
    );
  });

  it("keeps a future month's stable current-year reason instead of zeros", () => {
    const overview = purchaseOrderReportFixture();
    const annual = purchaseOrderReportYearFixture();
    annual.currentProjection = {
      ...annual.currentProjection,
      mode: "month",
      status: "unavailable",
      reason: "insufficient_days",
      buckets: [],
      currentMonth: undefined,
    };
    const readout = resolveMonthReadout({
      overview,
      month: "2026-12",
      annualSource: annual,
      annualStatus: "ready",
      annualError: "",
    });
    expect(readout.primary).toBeNull();
    expect(readout.estimate).toBeNull();
    expect(readout.unavailableReason).toBe("insufficient_days");
    expect(monthUnavailableLabel(readout.unavailableReason)).toContain(
      "Not enough elapsed days"
    );
  });

  it("does not expose old money while a new annual source loads or after it fails", () => {
    const overview = purchaseOrderReportYearFixture();
    const loading = resolveMonthReadout({
      overview,
      month: "2025-12",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(loading.status).toBe("loading");
    expect(loading.primary).toBeNull();
    expect(loading.estimate).toBeNull();

    const failed = resolveMonthReadout({
      overview,
      month: "2025-12",
      annualSource: null,
      annualStatus: "error",
      annualError: "The API is unreachable.",
    });
    expect(failed.status).toBe("error");
    expect(failed.error).toContain("API is unreachable");
    expect(failed.primary).toBeNull();
  });

  it("labels a source's own as-of and flags a different business date as not exportable", () => {
    const overview = purchaseOrderReportFixture();
    const annual = purchaseOrderReportYearFixture();
    annual.asOf = "2026-11-20";
    const readout = resolveMonthReadout({
      overview,
      month: "2026-10",
      annualSource: annual,
      annualStatus: "ready",
      annualError: "",
    });
    expect(readout.status).toBe("ready");
    expect(readout.asOf).toBe("2026-11-20");
    expect(readout.scopeMatchesOverview).toBe(false);
    expect(sourceScopeMatches(overview, annual)).toBe(false);
  });

  it("does not read annual sources for a malformed month", () => {
    const overview = purchaseOrderReportYearFixture();
    const readout = resolveMonthReadout({
      overview,
      month: "2026-1",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(readout.status).toBe("error");
    expect(readout.error).toContain("canonical YYYY-MM");
    expect(readout.primary).toBeNull();
  });
});

describe("month export eligibility (issue #641)", () => {
  it("requires actual evidence for current/past and allows an unavailable future forecast", async () => {
    const { monthReadoutExportable } = await import("./poReportComparison");
    const overview = purchaseOrderReportMonthFixture();
    const current = resolveMonthReadout({
      overview,
      month: "2026-11",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(monthReadoutExportable(current, overview)).toBe(true);

    const loading = resolveMonthReadout({
      overview,
      month: "2026-10",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(monthReadoutExportable(loading, overview)).toBe(false);

    const future = resolveMonthReadout({
      overview: purchaseOrderReportYearFixture(),
      month: "2027-02",
      annualSource: annualSourceFor(2027, purchaseOrderReportYearFixture()),
      annualStatus: "ready",
      annualError: "",
    });
    expect(monthReadoutExportable(future, purchaseOrderReportYearFixture())).toBe(
      true
    );

    const legacy = clone(purchaseOrderReportYearFixture());
    delete legacy.monthSummary;
    const fallback = resolveMonthReadout({
      overview: legacy,
      month: "2026-11",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(fallback.primary).not.toBeNull();
    expect(fallback.hasMonthSummary).toBe(false);
    expect(monthReadoutExportable(fallback, legacy)).toBe(false);

    const mismatched = resolveMonthReadout({
      overview,
      month: "2026-11",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    mismatched.scopeMatchesOverview = false;
    expect(monthReadoutExportable(mismatched, overview)).toBe(false);
  });
});

describe("month comparison selection (issue #642)", () => {
  it("canonicalizes a month set and recognises exactly one calendar year", () => {
    expect(canonicalMonths(["2026-03", "2025-12", "2026-03", "bad"])).toEqual([
      "2025-12",
      "2026-03",
    ]);
    expect(allMonthsOfYear("2026")).toHaveLength(12);
    expect(allMonthsOfYear("2026")[0]).toBe("2026-01");
    expect(allMonthsOfYear("2026")[11]).toBe("2026-12");
    expect(allYearForMonths(allMonthsOfYear("2026"))).toBe("2026");
    expect(
      allYearForMonths(allMonthsOfYear("2026").slice(0, 11))
    ).toBeNull();
    expect(
      allYearForMonths([
        ...allMonthsOfYear("2026").slice(0, 11),
        "2027-01",
      ])
    ).toBeNull();
  });

  it("focuses from All, then toggles membership down to an explicit empty set", () => {
    const all = allYearSelection("2026");
    expect(all.rejected).toBe(false);
    expect(all.months).toHaveLength(12);

    // A specific click while All is selected focuses that month even though
    // it was already checked.
    const focused = toggleMonthInSelection(all.months, "2026-03");
    expect(focused.rejected).toBe(false);
    expect(focused.months).toEqual(["2026-03"]);

    const added = toggleMonthInSelection(focused.months, "2026-05");
    expect(added.months).toEqual(["2026-03", "2026-05"]);

    const removed = toggleMonthInSelection(added.months, "2026-03");
    expect(removed.months).toEqual(["2026-05"]);

    const empty = toggleMonthInSelection(removed.months, "2026-05");
    expect(empty.months).toEqual([]);
  });

  it("starts the picker at the active month year, else the selected year, else the server year", () => {
    expect(initialPickerYear(["2025-12", "2026-01"], "2025-12", "2026")).toBe(
      "2025"
    );
    expect(initialPickerYear(["2025-12", "2026-01"], null, "2024")).toBe(
      "2026"
    );
    expect(initialPickerYear([], null, "2026")).toBe("2026");
    expect(initialPickerYear([], null, "")).toBe("");
  });

  it("moves active focus among selected months without changing membership", () => {
    const months = ["2025-12", "2026-01", "2026-07"];
    expect(months).toHaveLength(3);
    expect(nextActiveMonth(months, "2025-12", 1)).toBe("2026-01");
    expect(nextActiveMonth(months, "2026-01", 1)).toBe("2026-07");
    expect(nextActiveMonth(months, "2026-07", 1)).toBe("2026-07");
    expect(nextActiveMonth(months, "2026-07", -1)).toBe("2026-01");
    expect(nextActiveMonth(months, "2025-12", -1)).toBe("2025-12");
    expect(months).toEqual(["2025-12", "2026-01", "2026-07"]);
  });

  it("labels a selection with its count and year span", () => {
    expect(monthSelectionSummary([])).toBe("No months selected");
    expect(monthSelectionSummary(["2026-11"])).toBe("November 2026");
    expect(monthSelectionSummary(allYearSelection("2026").months)).toBe(
      "12 months · 2026"
    );
    expect(monthSelectionSummary(["2025-12", "2026-01"])).toBe(
      "2 months · 2025–2026"
    );
  });

  it("accepts exactly 120 months and a 120-month inclusive span", () => {
    expect(MAX_COMPARISON_MONTHS).toBe(120);
    expect(MAX_COMPARISON_MONTH_SPAN).toBe(120);
    const hundredTwenty = Array.from({ length: 120 }, (_, index) =>
      shiftMonth("2017-01", index)
    );
    expect(hundredTwenty[0]).toBe("2017-01");
    expect(hundredTwenty[119]).toBe("2026-12");
    expect(monthSpanCount(hundredTwenty)).toBe(120);
    expect(selectionLimitViolation(hundredTwenty)).toBe("");
    expect(
      selectionLimitViolation(["2016-12", "2026-11"])
    ).toBe("");
  });

  it("refuses a 121st month or a 121-month span without changing the set", () => {
    const hundredTwenty = Array.from({ length: 120 }, (_, index) =>
      shiftMonth("2017-01", index)
    );
    const tooMany = toggleMonthInSelection(hundredTwenty, "2027-01");
    expect(tooMany.rejected).toBe(true);
    expect(tooMany.message).toContain("120");
    expect(tooMany.months).toEqual(hundredTwenty);

    const tooWide = toggleMonthInSelection(
      ["2016-12", "2026-12"],
      "2020-01"
    );
    expect(tooWide.rejected).toBe(true);
    expect(tooWide.months).toEqual(["2016-12", "2026-12"]);

    expect(selectionLimitViolation(["2016-12", "2026-12"])).toContain("120");
  });
});

describe("month aggregate (issue #642)", () => {
  function pastYearSource(year: number): PurchaseOrderReportData {
    const source = annualSourceFor(year, purchaseOrderReportYearFixture());
    source.periods = source.periods.map((period) => ({
      ...period,
      ...zeroMetrics(),
    }));
    source.totals = zeroMetrics();
    return source;
  }

  it("reads twelve chronological rows and the annual snapshot's own total", () => {
    const overview = purchaseOrderReportYearFixture();
    const aggregate = resolveMonthAggregate({
      overview,
      year: "2026",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(aggregate.status).toBe("ready");
    expect(aggregate.rows).toHaveLength(12);
    expect(aggregate.rows[0].month).toBe("2026-01");
    expect(aggregate.rows[11].month).toBe("2026-12");
    expect(aggregate.rows[10].current).toBe(true);
    expect(aggregate.rows[10].recorded).toBe(true);
    expect(aggregate.rows[11].future).toBe(true);
    expect(aggregate.rows[11].recorded).toBe(false);
    expect(aggregate.rows[11].metrics).toBeNull();
    expect(aggregate.total).toEqual(overview.totals);
    expect(aggregate.complete).toBe(true);
    expect(
      aggregate.forecasts.map((forecast) => forecast.month)
    ).toEqual(["2026-12"]);
    expect(aggregate.forecasts[0].estimate.base).toBe(50_000_000);
    expect(aggregate.includesFutureDated).toBe(false);
  });

  it("never substitutes a row sum for a different server total", () => {
    const overview = purchaseOrderReportYearFixture();
    overview.totals = { ...overview.totals, amountMinor: 999_000_000 };
    const aggregate = resolveMonthAggregate({
      overview,
      year: "2026",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    const rowSum = aggregate.rows.reduce(
      (sum, row) => sum + (row.metrics?.amountMinor ?? 0),
      0
    );
    expect(rowSum).not.toBe(overview.totals.amountMinor);
    expect(aggregate.total?.amountMinor).toBe(999_000_000);
  });

  it("keeps a loaded zero past year as twelve genuine recorded zeros", () => {
    const overview = purchaseOrderReportYearFixture();
    const aggregate = resolveMonthAggregate({
      overview,
      year: "2025",
      annualSource: pastYearSource(2025),
      annualStatus: "ready",
      annualError: "",
    });
    expect(aggregate.status).toBe("ready");
    expect(aggregate.total).toEqual(zeroMetrics());
    for (const row of aggregate.rows) {
      expect(row.recorded).toBe(true);
      expect(row.metrics).toMatchObject(zeroMetrics());
    }
    expect(aggregate.forecasts).toEqual([]);
    expect(aggregate.unavailableReason).toBe("");
  });

  it("marks a future year unavailable without inventing forecast rows", () => {
    const overview = purchaseOrderReportYearFixture();
    const aggregate = resolveMonthAggregate({
      overview,
      year: "2027",
      annualSource: pastYearSource(2027),
      annualStatus: "ready",
      annualError: "",
    });
    expect(aggregate.status).toBe("ready");
    expect(aggregate.unavailableReason).toBe("outside_current_year");
    for (const row of aggregate.rows) {
      expect(row.metrics).toBeNull();
      expect(row.recorded).toBe(false);
    }
    expect(aggregate.forecasts).toEqual([]);
    expect(aggregate.total).toEqual(zeroMetrics());
  });

  it("discloses future-dated recorded evidence without folding in the forecast", () => {
    const overview = purchaseOrderReportYearWithRecordedFutureFixture();
    const aggregate = resolveMonthAggregate({
      overview,
      year: "2026",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(aggregate.includesFutureDated).toBe(true);
    expect(aggregate.rows[10].metrics?.poCount).toBe(
      overview.periods[10].poCount
    );
    expect(aggregate.total).toEqual(overview.totals);
    expect(
      aggregate.forecasts.reduce(
        (sum, forecast) => sum + forecast.estimate.base,
        0
      )
    ).not.toBe(aggregate.total?.amountMinor);
  });

  it("never presents a loading, failed or missing-row aggregate as complete", () => {
    const overview = purchaseOrderReportYearFixture();
    const loading = resolveMonthAggregate({
      overview: purchaseOrderReportFixture(),
      year: "2025",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    expect(loading.status).toBe("loading");
    expect(loading.rows).toEqual([]);
    expect(loading.total).toBeNull();

    const failed = resolveMonthAggregate({
      overview: purchaseOrderReportFixture(),
      year: "2025",
      annualSource: null,
      annualStatus: "error",
      annualError: "The annual API is unreachable.",
    });
    expect(failed.status).toBe("error");
    expect(failed.total).toBeNull();

    const missing = pastYearSource(2025);
    missing.periods = missing.periods.filter(
      (period) => period.periodStart !== "2025-03-01"
    );
    const incomplete = resolveMonthAggregate({
      overview,
      year: "2025",
      annualSource: missing,
      annualStatus: "ready",
      annualError: "",
    });
    expect(incomplete.complete).toBe(false);
    expect(incomplete.rows[2].metrics).toBeNull();
  });
});

// Issue #643: the independent category comparison's pure decisions.
const SUPPLIED_CODES = ["FM", "FAB", "SUPPLY_GEN", "OTHERS", "UNASSIGNED"];

function categoryUniverse(
  overview: PurchaseOrderReportData
): PurchaseOrderReportData {
  const source = clone(overview);
  source.filter = categorySourceQuery(overview.filter);
  return source;
}

function restrictedOverview(category: string): {
  overview: PurchaseOrderReportData;
  universe: PurchaseOrderReportData;
} {
  const base = purchaseOrderReportFixture();
  const overview = clone(base);
  overview.filter = { ...base.filter, category };
  overview.categories = base.categories.filter(
    (row) => row.businessCategory === category
  );
  return { overview, universe: categoryUniverse(base) };
}

/** The all-category snapshot with one genuine zero-activity supplied row. */
function withZeroFab(source: PurchaseOrderReportData): PurchaseOrderReportData {
  const copy = clone(source);
  copy.categories = copy.categories.map((row): PurchaseOrderReportCategory =>
    row.businessCategory === "FAB"
      ? {
          businessCategory: "FAB",
          label: "Fabrication",
          ...zeroMetrics(),
        }
      : row
  );
  return copy;
}

function rowFor(
  source: PurchaseOrderReportData,
  code: string
): PurchaseOrderReportCategory {
  const row = source.categories.find(
    (category) => category.businessCategory === code
  );
  if (row === undefined) throw new Error(`missing category ${code}`);
  return row;
}

describe("category source identity (issue #643)", () => {
  it("builds the exact all-category query at the overview's range and currency", () => {
    const overview = purchaseOrderReportMonthFixture();
    const query = categorySourceQuery({
      ...overview.filter,
      category: "FAB",
      currency: " usd ",
    });
    expect(query).toEqual({
      from: "2026-11-01",
      to: "2026-12-01",
      bucket: "day",
      category: "all",
      currency: "USD",
    });
    expect(query.from).toBe(overview.filter.from);
    expect(query.to).toBe(overview.filter.to);
    expect(query.bucket).toBe(overview.filter.bucket);

    const request = categorySourceRequest(overview);
    expect(request.query).toEqual(categorySourceQuery(overview.filter));
    expect(request.key).toBe(reportSourceKey(request.query, overview.asOf));
    expect(request.key).toBe(
      `${reportFilterKey(request.query)}#${overview.asOf}`
    );
  });

  it("reuses an all overview and adds one read for a restricted scope, never per code", () => {
    const all = purchaseOrderReportFixture();
    expect(overviewSuppliesAllCategories(all)).toBe(true);
    expect(categorySourceRequest(all).key).toBe(
      reportSourceKey(all.filter, all.asOf)
    );
    // The registry seeds the ready overview under that exact key: 0 reads.

    const { overview: restricted, universe } = restrictedOverview("FM");
    expect(overviewSuppliesAllCategories(restricted)).toBe(false);
    const request = categorySourceRequest(restricted);
    expect(request.key).not.toBe(
      reportSourceKey(restricted.filter, restricted.asOf)
    );
    expect(request.key).toBe(reportSourceKey(universe.filter, universe.asOf));

    // FM -> FAB on the same range/currency addresses the same all-category key.
    const fab = restrictedOverview("FAB").overview;
    expect(categorySourceRequest(fab).key).toBe(request.key);

    // Selecting one code or all five never changes the one source identity.
    const one = toggleCategoryInSelection(null, SUPPLIED_CODES, "FAB");
    const many = toggleCategoryInSelection(
      one.explicitCodes,
      SUPPLIED_CODES,
      "UNASSIGNED"
    );
    expect(one.explicitCodes).toEqual(["FAB"]);
    expect(many.explicitCodes).toEqual(["FAB", "UNASSIGNED"]);
    expect(categorySourceRequest(fab).key).toBe(request.key);
  });

  it("matches only the overview's exact range, bucket, currency and all scope", () => {
    const { overview, universe } = restrictedOverview("FM");
    expect(categorySourceMatchesOverview(overview, universe)).toBe(true);
    expect(
      categorySourceMatchesOverview(purchaseOrderReportFixture(), universe)
    ).toBe(true);

    const wrongFrom = clone(universe);
    wrongFrom.filter.from = "2026-01-02";
    expect(categorySourceMatchesOverview(overview, wrongFrom)).toBe(false);

    const wrongTo = clone(universe);
    wrongTo.filter.to = "2026-11-21";
    expect(categorySourceMatchesOverview(overview, wrongTo)).toBe(false);

    const wrongBucket = clone(universe);
    wrongBucket.filter.bucket = "year";
    expect(categorySourceMatchesOverview(overview, wrongBucket)).toBe(false);

    const wrongCurrency = clone(universe);
    wrongCurrency.filter.currency = "USD";
    expect(categorySourceMatchesOverview(overview, wrongCurrency)).toBe(false);

    // A restricted FM snapshot is never a substitute for the universe.
    expect(categorySourceMatchesOverview(overview, overview)).toBe(false);

    const otherAsOf = clone(universe);
    otherAsOf.asOf = "2026-11-18";
    expect(categorySourceMatchesOverview(overview, otherAsOf)).toBe(true);
  });
});

describe("category labels (issue #643)", () => {
  it("falls back to the code and disambiguates only colliding labels", () => {
    const rows: PurchaseOrderReportCategory[] = [
      {
        businessCategory: "FM",
        label: "Facility Management",
        ...zeroMetrics(),
      },
      { businessCategory: "FAB", label: "Fabrication", ...zeroMetrics() },
      { businessCategory: "SUPPLY_GEN", label: "Fabrication", ...zeroMetrics() },
      { businessCategory: "OTHERS", label: "", ...zeroMetrics() },
      { businessCategory: "UNASSIGNED", label: "   ", ...zeroMetrics() },
    ];
    const labels = categoryDisplayLabels(rows);
    expect(labels.get("FM")).toBe("Facility Management");
    expect(labels.get("FAB")).toBe("Fabrication (FAB)");
    expect(labels.get("SUPPLY_GEN")).toBe("Fabrication (SUPPLY_GEN)");
    expect(labels.get("OTHERS")).toBe("OTHERS");
    expect(labels.get("UNASSIGNED")).toBe("UNASSIGNED");
  });

  it("does not suffix a colliding row whose fallback label is already its code", () => {
    const rows: PurchaseOrderReportCategory[] = [
      { businessCategory: "FM", label: "", ...zeroMetrics() },
      { businessCategory: "FAB", label: "FM", ...zeroMetrics() },
    ];
    const labels = categoryDisplayLabels(rows);
    expect(labels.get("FM")).toBe("FM");
    expect(labels.get("FAB")).toBe("FM (FAB)");
  });
});

describe("category selection state machine (issue #643)", () => {
  it("starts from All over every supplied row, including zero and UNASSIGNED", () => {
    expect(resolveSelectedCategoryCodes(null, SUPPLIED_CODES)).toEqual(
      SUPPLIED_CODES
    );
    expect(
      isAllCategoriesSelected(SUPPLIED_CODES, SUPPLIED_CODES)
    ).toBe(true);
    expect(isAllCategoriesSelected([], SUPPLIED_CODES)).toBe(false);
    expect(isAllCategoriesSelected([], [])).toBe(false);
    expect(resolveSelectedCategoryCodes(null, ["FM", "FM", "UNASSIGNED"])).toEqual(
      ["FM", "UNASSIGNED"]
    );
  });

  it("focuses one from All, then toggles membership in supplied order", () => {
    const focused = toggleCategoryInSelection(null, SUPPLIED_CODES, "OTHERS");
    expect(focused).toEqual({
      explicitCodes: ["OTHERS"],
      activeCode: "OTHERS",
    });

    const added = toggleCategoryInSelection(
      focused.explicitCodes,
      SUPPLIED_CODES,
      "FM"
    );
    expect(added.explicitCodes).toEqual(["FM", "OTHERS"]);
    expect(added.activeCode).toBe("FM");

    const removed = toggleCategoryInSelection(
      added.explicitCodes,
      SUPPLIED_CODES,
      "FM"
    );
    expect(removed.explicitCodes).toEqual(["OTHERS"]);
    expect(removed.activeCode).toBe("OTHERS");

    const empty = toggleCategoryInSelection(["OTHERS"], SUPPLIED_CODES, "OTHERS");
    expect(empty).toEqual({ explicitCodes: [], activeCode: null });
    expect(isAllCategoriesSelected(empty.explicitCodes ?? [], SUPPLIED_CODES)).toBe(
      false
    );
  });

  it("derives All when every supplied row is selected and remembers order", () => {
    let change = toggleCategoryInSelection(null, SUPPLIED_CODES, "FAB");
    for (const code of ["UNASSIGNED", "FM", "OTHERS", "SUPPLY_GEN"]) {
      change = toggleCategoryInSelection(
        change.explicitCodes,
        SUPPLIED_CODES,
        code
      );
    }
    expect(change.explicitCodes).toEqual(SUPPLIED_CODES);
    expect(
      isAllCategoriesSelected(change.explicitCodes ?? [], SUPPLIED_CODES)
    ).toBe(true);
    expect(selectAllCategories(SUPPLIED_CODES)).toEqual({
      explicitCodes: null,
      activeCode: "FM",
    });
    expect(selectAllCategories([])).toEqual({
      explicitCodes: null,
      activeCode: null,
    });
  });

  it("ignores a code the active source does not supply", () => {
    const specific = toggleCategoryInSelection(["FM"], SUPPLIED_CODES, "NOPE");
    expect(specific.explicitCodes).toEqual(["FM"]);
    expect(specific.activeCode).toBe("FM");

    const all = toggleCategoryInSelection(null, SUPPLIED_CODES, "NOPE");
    expect(all.explicitCodes).toBeNull();
    expect(all.activeCode).toBe("FM");
  });

  it("moves single navigation along supplied order with the same clamp as subsets", () => {
    // Single: the next supplied row replaces the single code, clamped at ends.
    expect(nextActiveCategory(SUPPLIED_CODES, "FM", -1)).toBe("FM");
    expect(nextActiveCategory(SUPPLIED_CODES, "FM", 1)).toBe("FAB");
    expect(nextActiveCategory(SUPPLIED_CODES, "FAB", 1)).toBe("SUPPLY_GEN");
    expect(nextActiveCategory(SUPPLIED_CODES, "UNASSIGNED", 1)).toBe(
      "UNASSIGNED"
    );
    // Subset focus: moves within the selected codes only.
    const subset = ["FM", "OTHERS", "UNASSIGNED"];
    expect(nextActiveCategory(subset, "FM", 1)).toBe("OTHERS");
    expect(nextActiveCategory(subset, null, 1)).toBe("OTHERS");
    expect(nextActiveCategory(subset, "UNASSIGNED", 1)).toBe("UNASSIGNED");
    expect(nextActiveCategory([], null, 1)).toBeNull();
  });
});

describe("category readout (issue #643)", () => {
  it("shows every supplied row and the server total for the all overview", () => {
    const overview = purchaseOrderReportFixture();
    const source = withZeroFab(categoryUniverse(overview));
    const readout = resolveCategoryReadout({
      overview,
      source,
      status: "ready",
      error: "",
      explicitCodes: null,
    });

    expect(readout.status).toBe("ready");
    expect(readout.error).toBe("");
    expect(readout.rangeLabel).toBe(reportRangeLabel(overview.filter));
    expect(readout.rangeLabel).toBe("1 Jan 2026 – 19 Nov 2026");
    expect(readout.currency).toBe("IDR");
    expect(readout.asOf).toBe(overview.asOf);
    expect(readout.generatedAt).toBe(overview.generatedAt);
    expect(readout.sourceLabel).toBe("Performance snapshot");
    expect(readout.scopeMatchesOverview).toBe(true);
    expect(readout.supplied.map((item) => item.code)).toEqual(SUPPLIED_CODES);
    expect(readout.selected.map((item) => item.code)).toEqual(SUPPLIED_CODES);
    expect(readout.total).toEqual(source.totals);

    const zeroFab = readout.supplied.find((item) => item.code === "FAB");
    expect(zeroFab?.metrics).toEqual(zeroMetrics());
    const unassigned = readout.supplied.find(
      (item) => item.code === "UNASSIGNED"
    );
    expect(unassigned?.metrics.marginBps).toBeNull();
    expect(unassigned?.metrics.poCount).toBe(
      rowFor(source, "UNASSIGNED").poCount
    );
    expect(unassigned?.metrics.capitalMinor).toBe(0);
  });

  it("keeps a restricted overview's range while reading the all-category universe", () => {
    const { overview, universe } = restrictedOverview("FM");
    const source = withZeroFab(universe);
    const readout = resolveCategoryReadout({
      overview,
      source,
      status: "ready",
      error: "",
      explicitCodes: ["UNASSIGNED", "FAB"],
    });

    expect(readout.status).toBe("ready");
    expect(readout.rangeLabel).toBe(reportRangeLabel(overview.filter));
    expect(readout.sourceLabel).toBe("All-category snapshot");
    expect(readout.scopeMatchesOverview).toBe(true);
    expect(readout.supplied.map((item) => item.code)).toEqual(SUPPLIED_CODES);
    // Selected rows follow supplied order, not click order.
    expect(readout.selected.map((item) => item.code)).toEqual([
      "FAB",
      "UNASSIGNED",
    ]);
    expect(readout.total).toEqual(source.totals);

    const empty = resolveCategoryReadout({
      overview,
      source,
      status: "ready",
      error: "",
      explicitCodes: [],
    });
    expect(empty.selected).toEqual([]);
    expect(empty.supplied).toHaveLength(SUPPLIED_CODES.length);
    expect(empty.total).toEqual(source.totals);
  });

  it("surfaces a range, bucket, currency or restricted-source mismatch as an error", () => {
    const { overview, universe } = restrictedOverview("FM");
    const wrongCurrency = clone(universe);
    wrongCurrency.filter.currency = "USD";
    const currencyReadout = resolveCategoryReadout({
      overview,
      source: wrongCurrency,
      status: "ready",
      error: "",
      explicitCodes: null,
    });
    expect(currencyReadout.status).toBe("error");
    expect(currencyReadout.supplied).toEqual([]);
    expect(currencyReadout.selected).toEqual([]);
    expect(currencyReadout.total).toBeNull();
    expect(currencyReadout.error).toMatch(/does not match/);

    const wrongRange = clone(universe);
    wrongRange.filter.from = "2026-01-02";
    expect(
      resolveCategoryReadout({
        overview,
        source: wrongRange,
        status: "ready",
        error: "",
        explicitCodes: null,
      }).status
    ).toBe("error");

    const restrictedSource = resolveCategoryReadout({
      overview,
      source: overview,
      status: "ready",
      error: "",
      explicitCodes: null,
    });
    expect(restrictedSource.status).toBe("error");
    expect(restrictedSource.total).toBeNull();
  });

  it("keeps an as-of mismatch visible but marks the scope stale", () => {
    const { overview, universe } = restrictedOverview("FM");
    const stale = clone(universe);
    stale.asOf = "2026-11-18";
    const readout = resolveCategoryReadout({
      overview,
      source: stale,
      status: "ready",
      error: "",
      explicitCodes: null,
    });
    expect(readout.status).toBe("ready");
    expect(readout.scopeMatchesOverview).toBe(false);
    expect(readout.asOf).toBe("2026-11-18");
    expect(readout.supplied).toHaveLength(SUPPLIED_CODES.length);
    expect(readout.total).toEqual(stale.totals);
    expect(readout.sourceLabel).toBe("All-category snapshot");
  });

  it("never presents loading or failed data as known zero activity", () => {
    const all = purchaseOrderReportFixture();
    const loading = resolveCategoryReadout({
      overview: all,
      source: null,
      status: "loading",
      error: "",
      explicitCodes: null,
    });
    expect(loading).toMatchObject({
      status: "loading",
      supplied: [],
      selected: [],
      total: null,
      scopeMatchesOverview: false,
      sourceLabel: "Performance snapshot",
    });

    const failed = resolveCategoryReadout({
      overview: all,
      source: null,
      status: "error",
      error: "The all-category API is unreachable.",
      explicitCodes: null,
    });
    expect(failed.status).toBe("error");
    expect(failed.error).toBe("The all-category API is unreachable.");
    expect(failed.total).toBeNull();

    const readyWithoutSource = resolveCategoryReadout({
      overview: all,
      source: null,
      status: "ready",
      error: "",
      explicitCodes: null,
    });
    expect(readyWithoutSource.status).toBe("error");
    expect(readyWithoutSource.error).toMatch(/could not be loaded/);
    expect(readyWithoutSource.total).toBeNull();
  });

  it("summarizes the trigger honestly for All, one, subset and empty", () => {
    const overview = purchaseOrderReportFixture();
    const readout = resolveCategoryReadout({
      overview,
      source: categoryUniverse(overview),
      status: "ready",
      error: "",
      explicitCodes: null,
    });
    expect(
      categorySelectionSummary(
        readout.selected,
        readout.supplied.length,
        "ready"
      )
    ).toBe("All categories");
    expect(
      categorySelectionSummary(
        [readout.supplied[0]],
        readout.supplied.length,
        "ready"
      )
    ).toBe("Facility Management");
    expect(
      categorySelectionSummary(
        readout.supplied.slice(0, 3),
        readout.supplied.length,
        "ready"
      )
    ).toBe("3 of 5 categories");
    expect(categorySelectionSummary([], 0, "ready")).toBe(
      "No categories selected"
    );
    expect(categorySelectionSummary([], 0, "loading")).toBe(
      "Loading categories"
    );
    expect(categorySelectionSummary([], 0, "error")).toBe(
      "Categories unavailable"
    );
  });
});
