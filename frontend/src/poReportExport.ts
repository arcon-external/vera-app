// Issue #641/#642: the frozen version-1 export envelope built by the desktop.
//
// The binding still accepts one JSON string. This module turns the visible
// overview and the selected month snapshots into the bounded source array and
// section selections the native renderer validates. It is pure: no fetch, no
// mutation and no financial arithmetic. Aggregate references the one annual
// snapshot whose server total the renderer prints; it never builds a row sum.

import type { PurchaseOrderReportData } from "./poReport";
import { reportFilterKey } from "./poReportPage";
import {
  allYearForMonths,
  annualSourceCoversMonth,
  canonicalMonths,
  categorySourceMatchesOverview,
  monthStart,
  sourceScopeMatches,
  type ReportComparisonLayout,
} from "./poReportComparison";

export const REPORT_VIEW_VERSION = 1 as const;

export type PurchaseOrderReportViewLayout = ReportComparisonLayout;

export interface PurchaseOrderReportViewPeriods {
  sources: number[];
  /** Canonical YYYY-MM identifiers, unique and chronological. */
  months: string[];
  layout: PurchaseOrderReportViewLayout;
  currentMonthSource?: number;
}

export interface PurchaseOrderReportViewCategories {
  source: number;
  codes: string[];
  layout: PurchaseOrderReportViewLayout;
}

export interface PurchaseOrderReportView {
  version: 1;
  sources: PurchaseOrderReportData[];
  overview: number;
  periods: PurchaseOrderReportViewPeriods;
  categories: PurchaseOrderReportViewCategories;
}

/** One selected month and the exact snapshot that owns its evidence. */
export interface PurchaseOrderReportViewMonth {
  /** Canonical YYYY-MM. */
  month: string;
  /** The exact snapshot that owns the selected month. */
  source: PurchaseOrderReportData;
  /** The current month came from a full-current-month/day overview. */
  usesOverviewMonthSummary: boolean;
}

export interface PurchaseOrderReportViewSelection {
  /** In any order; the builder canonicalizes it before serializing. */
  months: readonly PurchaseOrderReportViewMonth[];
  layout: PurchaseOrderReportViewLayout;
  /** When absent, the legacy full-overview category section is preserved. */
  categories?: PurchaseOrderReportViewCategorySelection;
}

/** One explicit local category selection and the snapshot that owns it. */
export interface PurchaseOrderReportViewCategorySelection {
  source: PurchaseOrderReportData;
  codes: readonly string[];
  layout: PurchaseOrderReportViewLayout;
}

function queryKey(report: PurchaseOrderReportData): string {
  return reportFilterKey(report.filter);
}

function currentMonthKey(overview: PurchaseOrderReportData): string {
  const asOf = overview.asOf.trim();
  return /^\d{4}-\d{2}-\d{2}$/.test(asOf) ? asOf.slice(0, 7) : "";
}

/**
 * Builds the envelope for the visible selection. The overview is always
 * source 0; each exact-duplicate month source reuses its index. Every selected
 * source must share the overview's as-of, Business category and currency: a
 * differently scoped snapshot is not combined into one current view.
 *
 * An empty selection produces the explicit empty period section with no
 * required month source. Aggregate requires exactly the twelve calendar months
 * of one year and references that year's one annual snapshot. Individual (the
 * shape single and empty use) references each distinct month source once.
 */
export function buildPurchaseOrderReportView(
  overview: PurchaseOrderReportData,
  selection: PurchaseOrderReportViewSelection
): PurchaseOrderReportView {
  const overviewKey = queryKey(overview);
  const currentKey = currentMonthKey(overview);
  const entries = [...selection.months].sort((left, right) =>
    left.month < right.month ? -1 : left.month > right.month ? 1 : 0
  );
  const canonical = canonicalMonths(entries.map((entry) => entry.month));

  const seenMonths = new Set<string>();
  for (const entry of entries) {
    if (monthStart(entry.month) === "") {
      throw new Error(
        `select a canonical YYYY-MM month, received ${entry.month}`
      );
    }
    if (seenMonths.has(entry.month)) {
      throw new Error(`the month ${entry.month} is selected twice`);
    }
    seenMonths.add(entry.month);
    if (!sourceScopeMatches(overview, entry.source)) {
      throw new Error(
        "the selected month snapshot does not share the overview's as-of, category and currency"
      );
    }
    if (entry.usesOverviewMonthSummary) {
      if (entry.month !== currentKey) {
        throw new Error(
          "only the current month can use the overview's own monthSummary"
        );
      }
      if (
        queryKey(entry.source) !== overviewKey ||
        overview.filter.bucket !== "day"
      ) {
        throw new Error(
          "the current month can only use the overview's own monthSummary"
        );
      }
    } else if (!annualSourceCoversMonth(entry.source, entry.month)) {
      throw new Error(
        `month ${entry.month} has no exact annual/monthly source`
      );
    }
  }
  if (canonical.length !== entries.length) {
    throw new Error("the selected months are not canonical");
  }

  const sources: PurchaseOrderReportData[] = [overview];
  const sourceIndexByKey = new Map<string, number>([[overviewKey, 0]]);
  const sourceIndex = (source: PurchaseOrderReportData): number => {
    const key = queryKey(source);
    const existing = sourceIndexByKey.get(key);
    if (existing !== undefined) return existing;
    const index = sources.length;
    sources.push(source);
    sourceIndexByKey.set(key, index);
    return index;
  };

  const assertSourceCap = (): void => {
    if (sources.length > 16) {
      throw new Error("the report view can reference at most 16 sources");
    }
  };

  const legacyCategories: PurchaseOrderReportViewCategories = {
    source: 0,
    codes: overview.categories.map((row) => row.businessCategory),
    layout: "aggregate",
  };

  /**
   * The explicit category section, canonicalized to the source's supplied
   * order. An empty selection needs no financial source and prints the
   * explicit empty section. A non-empty selection must name a snapshot with the
   * overview's as-of and either the all-category scope or the #641 exact
   * overview filter; subset Aggregate is refused rather than invented.
   */
  const resolveCategories = (): PurchaseOrderReportViewCategories => {
    const categorySelection = selection.categories;
    if (categorySelection === undefined) return legacyCategories;
    const requested = [...categorySelection.codes];
    if (requested.length === 0) {
      return { source: 0, codes: [], layout: "individual" };
    }
    if (
      categorySelection.layout !== "aggregate" &&
      categorySelection.layout !== "individual"
    ) {
      throw new Error("the category layout must be aggregate or individual");
    }
    const source = categorySelection.source;
    if (source.asOf !== overview.asOf) {
      throw new Error(
        "the category snapshot does not share the overview's as-of"
      );
    }
    if (
      !categorySourceMatchesOverview(overview, source) &&
      queryKey(source) !== overviewKey
    ) {
      throw new Error(
        "the category snapshot does not match the overview's range, bucket, currency or all-category scope"
      );
    }
    const unique = new Set(requested);
    if (unique.size !== requested.length) {
      throw new Error("a category is selected twice");
    }
    const supplied = source.categories.map((row) => row.businessCategory);
    const suppliedSet = new Set(supplied);
    for (const code of requested) {
      if (!suppliedSet.has(code)) {
        throw new Error(`the category ${code} is not present in the snapshot`);
      }
    }
    const ordered = supplied.filter((code) => unique.has(code));
    if (
      categorySelection.layout === "aggregate" &&
      ordered.length !== supplied.length
    ) {
      throw new Error(
        "Aggregate requires every category the snapshot supplies"
      );
    }
    return {
      source: sourceIndex(source),
      codes: ordered,
      layout: categorySelection.layout,
    };
  };

  const categories = resolveCategories();

  if (entries.length === 0) {
    // An empty selection has no month display mode; Individual is the honest
    // section shape and the native renderer accepts it with no month source.
    assertSourceCap();
    return {
      version: REPORT_VIEW_VERSION,
      sources,
      overview: 0,
      periods: { sources: [], months: [], layout: "individual" },
      categories,
    };
  }

  if (selection.layout === "aggregate") {
    const year = allYearForMonths(canonical);
    if (year === null || canonical.length !== 12) {
      throw new Error(
        "Aggregate requires the twelve calendar months of exactly one calendar year"
      );
    }
    let annualIndex = -1;
    for (const entry of entries) {
      if (!annualSourceCoversMonth(entry.source, `${year}-01`)) continue;
      const index = sourceIndex(entry.source);
      if (annualIndex >= 0 && annualIndex !== index) {
        throw new Error(`the aggregate year ${year} is covered twice`);
      }
      annualIndex = index;
    }
    if (annualIndex < 0) {
      throw new Error(
        `the aggregate year ${year} has no referenced annual/monthly source`
      );
    }
    assertSourceCap();
    return {
      version: REPORT_VIEW_VERSION,
      sources,
      overview: 0,
      periods: {
        sources: [annualIndex],
        months: canonical,
        layout: "aggregate",
      },
      categories,
    };
  }

  const refs: number[] = [];
  let currentMonthSource: number | undefined;
  for (const entry of entries) {
    const index = sourceIndex(entry.source);
    if (!refs.includes(index)) refs.push(index);
    if (entry.usesOverviewMonthSummary) currentMonthSource = index;
  }

  assertSourceCap();
  return {
    version: REPORT_VIEW_VERSION,
    sources,
    overview: 0,
    periods: {
      sources: refs,
      months: canonical,
      layout: "individual",
      ...(currentMonthSource === undefined ? {} : { currentMonthSource }),
    },
    categories,
  };
}
