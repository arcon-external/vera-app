import { describe, expect, it } from "vitest";
import {
  annualMonthQuery,
  annualSourceKey,
  monthKeyFromDate,
  monthLabel,
  monthStart,
  monthUnavailableLabel,
  nextMonthStart,
  resolveMonthReadout,
  resolveMonthSource,
  shiftMonth,
  sourceScopeMatches,
} from "./poReportComparison";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
} from "./poReportFixtures";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
  PurchaseOrderReportQuery,
} from "./poReport";
import { reportFilterKey } from "./poReportPage";

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
