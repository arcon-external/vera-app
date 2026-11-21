// Issue #641: the frozen version-1 export envelope built by the desktop.
//
// The binding still accepts one JSON string. This module turns the visible
// overview and the resolved selected-month snapshot into the bounded source
// array and section selections the native renderer validates. It is pure: no
// fetch, no mutation and no financial arithmetic.

import type { PurchaseOrderReportData } from "./poReport";
import { reportFilterKey } from "./poReportPage";
import { monthStart, sourceScopeMatches } from "./poReportComparison";

export const REPORT_VIEW_VERSION = 1 as const;

export type PurchaseOrderReportViewLayout = "aggregate" | "individual";

export interface PurchaseOrderReportViewPeriods {
  sources: number[];
  /** Canonical YYYY-MM identifiers, unique and chronological. */
  months: string[];
  layout: "individual";
  currentMonthSource?: number;
}

export interface PurchaseOrderReportViewCategories {
  source: number;
  codes: string[];
  layout: "aggregate";
}

export interface PurchaseOrderReportView {
  version: 1;
  sources: PurchaseOrderReportData[];
  overview: number;
  periods: PurchaseOrderReportViewPeriods;
  categories: PurchaseOrderReportViewCategories;
}

export interface PurchaseOrderReportMonthSelection {
  /** Canonical YYYY-MM. */
  month: string;
  /** The exact snapshot that owns the selected month. */
  source: PurchaseOrderReportData;
  /** The current month came from a full-current-month/day overview. */
  usesOverviewMonthSummary: boolean;
}

function queryKey(report: PurchaseOrderReportData): string {
  return reportFilterKey(report.filter);
}

/**
 * Builds the envelope for the visible selection. The overview is always
 * source 0; an exact-duplicate month source reuses that index. The selected
 * month must be canonical, and the source must share the overview's as-of,
 * Business category and currency: a differently scoped snapshot is not
 * combined into one current view.
 */
export function buildPurchaseOrderReportView(
  overview: PurchaseOrderReportData,
  selection: PurchaseOrderReportMonthSelection
): PurchaseOrderReportView {
  if (monthStart(selection.month) === "") {
    throw new Error(`select a canonical YYYY-MM month, received ${selection.month}`);
  }
  if (!sourceScopeMatches(overview, selection.source)) {
    throw new Error(
      "the selected month snapshot does not share the overview's as-of, category and currency"
    );
  }
  if (
    selection.usesOverviewMonthSummary &&
    queryKey(selection.source) !== queryKey(overview)
  ) {
    throw new Error(
      "the current month can only use the overview's own monthSummary"
    );
  }

  const sources = [overview];
  let sourceIndex = 0;
  if (queryKey(selection.source) !== queryKey(overview)) {
    sources.push(selection.source);
    sourceIndex = 1;
  }

  return {
    version: REPORT_VIEW_VERSION,
    sources,
    overview: 0,
    periods: {
      sources: [sourceIndex],
      months: [selection.month],
      layout: "individual",
      ...(selection.usesOverviewMonthSummary
        ? { currentMonthSource: sourceIndex }
        : {}),
    },
    categories: {
      source: 0,
      codes: overview.categories.map((row) => row.businessCategory),
      layout: "aggregate",
    },
  };
}
