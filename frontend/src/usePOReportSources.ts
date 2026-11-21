// Issue #641/#642/#643: the reports page's per-mounted shared source registry.
//
// Reports is mounted once per signed-in workspace and unmounted on sign-out, so
// this cache is scoped to the authenticated page instance: there is no client
// organization id to key on and no cross-session persistence. The registry
// stores every ready overview by exact query and as-of, deduplicates identical
// in-flight reads, retries an explicit key, and serves both the independent
// month comparison and the all-category category comparison. It never fetches
// speculatively for a current month the overview's own monthSummary resolves,
// and it never fetches while the picker pages another year.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { GetPurchaseOrderReport } from "@backend/backend/Quotation";
import { errorMessage } from "./errors";
import {
  allYearForMonths,
  allYearSelection,
  annualMonthRequest,
  categorySourceRequest,
  initialPickerYear,
  isAllCategoriesSelected,
  monthKeyFromDate,
  monthReadoutExportable,
  monthStart,
  nextActiveCategory,
  nextActiveMonth,
  overviewCanSupplyCurrentMonth,
  overviewIsAnnualForMonth,
  reportSourceKey,
  resolveCategoryReadout,
  resolveMonthAggregate,
  resolveMonthReadout,
  resolveSelectedCategoryCodes,
  selectAllCategories,
  shiftMonth,
  toggleCategoryInSelection,
  toggleMonthInSelection,
  type AnnualMonthSourceRequest,
  type CategorySourceRequest,
  type MonthSourceStatus,
  type PurchaseOrderCategoryReadout,
  type PurchaseOrderMonthAggregate,
  type PurchaseOrderMonthReadout,
  type ReportComparisonLayout,
  type ReportMonthLayout,
} from "./poReportComparison";
import {
  normalizePurchaseOrderReport,
  type PurchaseOrderReportData,
  type PurchaseOrderReportQuery,
} from "./poReport";
import type {
  PurchaseOrderReportViewCategorySelection,
  PurchaseOrderReportViewSelection,
} from "./poReportExport";

/** One immutable source snapshot cached by exact query and as-of. */
export interface POReportSourceEntry {
  key: string;
  status: MonthSourceStatus;
  query: PurchaseOrderReportQuery;
  report: PurchaseOrderReportData | null;
  error: string;
}

/**
 * The per-mounted page's shared source registry. #645's generation,
 * invalidation and eviction will plug into this seam; #643 deliberately adds
 * only seeding, deduplicated reads and retry.
 */
export interface POReportSourceRegistry {
  get: (key: string) => POReportSourceEntry | null;
  ensure: (
    requests: readonly { key: string; query: PurchaseOrderReportQuery }[]
  ) => void;
  retry: (key: string) => void;
}

/**
 * The month and category browsers' one source cache. Every ready overview is
 * seeded under its exact `query#asOf`, so switching Performance's category on
 * the same range/currency reuses the all-category snapshot instead of
 * refetching it. Identical in-flight keys are deduplicated; a retry supersedes
 * the previous attempt and a late response can only write its own key.
 */
export function usePOReportSourceRegistry(
  overview: PurchaseOrderReportData | null
): POReportSourceRegistry {
  const [entries, setEntries] = useState<Record<string, POReportSourceEntry>>(
    {}
  );
  const entriesRef = useRef<Record<string, POReportSourceEntry>>({});
  const inFlight = useRef<Map<string, number>>(new Map());
  const attemptSeq = useRef(0);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const commit = useCallback((entry: POReportSourceEntry) => {
    entriesRef.current = { ...entriesRef.current, [entry.key]: entry };
    setEntries(entriesRef.current);
  }, []);

  const start = useCallback(
    (key: string, query: PurchaseOrderReportQuery) => {
      const attempt = ++attemptSeq.current;
      inFlight.current.set(key, attempt);
      commit({ key, status: "loading", query, report: null, error: "" });
      void (async () => {
        try {
          const raw = await GetPurchaseOrderReport(query);
          const report = normalizePurchaseOrderReport(raw);
          if (!mounted.current || inFlight.current.get(key) !== attempt) return;
          commit({ key, status: "ready", query, report, error: "" });
        } catch (caught) {
          if (!mounted.current || inFlight.current.get(key) !== attempt) return;
          commit({
            key,
            status: "error",
            query,
            report: null,
            error: errorMessage(
              caught,
              "The monthly snapshot could not be loaded. Check the API connection and try again."
            ),
          });
        } finally {
          if (inFlight.current.get(key) === attempt) inFlight.current.delete(key);
        }
      })();
    },
    [commit]
  );

  const ensure = useCallback(
    (
      requests: readonly { key: string; query: PurchaseOrderReportQuery }[]
    ) => {
      for (const request of requests) {
        const existing = entriesRef.current[request.key];
        if (
          existing !== undefined &&
          (existing.status === "ready" || existing.status === "loading")
        ) {
          continue;
        }
        if (inFlight.current.has(request.key)) continue;
        start(request.key, request.query);
      }
    },
    [start]
  );

  const retry = useCallback(
    (key: string) => {
      const existing = entriesRef.current[key];
      if (existing === undefined) return;
      start(key, existing.query);
    },
    [start]
  );

  useEffect(() => {
    if (overview === null) return;
    const key = reportSourceKey(overview.filter, overview.asOf);
    const existing = entriesRef.current[key];
    if (existing !== undefined && existing.status === "ready") return;
    // The mounted overview is authoritative for its own key: supersede any
    // identical read still in flight so a late failure cannot replace it.
    inFlight.current.delete(key);
    commit({
      key,
      status: "ready",
      query: { ...overview.filter },
      report: overview,
      error: "",
    });
  }, [overview, commit]);

  const get = useCallback(
    (key: string) => entries[key] ?? null,
    [entries]
  );

  return useMemo(() => ({ get, ensure, retry }), [get, ensure, retry]);
}

export interface POReportMonthComparison {
  /** Canonical chronological selected months; the explicit empty state is []. */
  months: string[];
  /** The focused card. Arrows move this in subset/All-Individual mode. */
  activeMonth: string | null;
  /** The remembered All display preference (Aggregate by default). */
  layout: ReportMonthLayout;
  /** The one calendar year an All selection names, or null. */
  allYear: string | null;
  /** A refused selection action's inline explanation; "" when none. */
  notice: string;
  /** One resolved readout per selected month, in the same order. */
  readouts: PurchaseOrderMonthReadout[];
  /** The All/Aggregate one-year readout when All is selected. */
  aggregate: PurchaseOrderMonthAggregate | null;
  /** Every visible selected source is ready and scope-consistent. */
  canExport: boolean;
  /** The frozen envelope selection; null while export is not allowed. */
  exportSelection: PurchaseOrderReportViewSelection | null;
  selectAll: (year: string) => void;
  toggleMonth: (month: string) => void;
  setLayout: (layout: ReportMonthLayout) => void;
  selectPrevious: () => void;
  selectNext: () => void;
  retryMonth: (month: string) => void;
  clearNotice: () => void;
  /** The page opens the picker at this year (server year when empty). */
  pickerStartYear: () => string;
}

/**
 * Owns the independent month selection and its annual source registry. The
 * overview is the displayed Performance snapshot; the selected months inherit
 * its category, currency and as-of while their own dates stay independent.
 * Passing a registry shares it with the category browser; omitting one keeps
 * the #642 signature source-compatible.
 */
export function usePOReportMonthComparison(
  overview: PurchaseOrderReportData | null,
  registry?: POReportSourceRegistry
): POReportMonthComparison {
  const ownRegistry = usePOReportSourceRegistry(
    registry === undefined ? overview : null
  );
  const sourceRegistry = registry ?? ownRegistry;
  const { ensure, get, retry } = sourceRegistry;
  const [explicitMonths, setExplicitMonths] = useState<string[] | null>(null);
  const [activeMonth, setActiveMonth] = useState<string | null>(null);
  const [layout, setLayout] = useState<ReportMonthLayout>("aggregate");
  const [notice, setNotice] = useState("");

  const currentMonth = overview === null ? "" : monthKeyFromDate(overview.asOf);
  const months = useMemo(() => {
    if (explicitMonths !== null) return explicitMonths;
    return currentMonth === "" ? [] : [currentMonth];
  }, [explicitMonths, currentMonth]);
  const allYear = useMemo(() => allYearForMonths(months), [months]);
  const effectiveActive =
    activeMonth !== null && months.includes(activeMonth)
      ? activeMonth
      : months[0] ?? null;

  // One annual read per required calendar year, never per card. A current month
  // resolved by the overview's monthSummary needs no annual read at all. The
  // shared registry already seeded an exact annual overview under its key.
  const annualRequests = useMemo(() => {
    if (overview === null) return [];
    const byYear = new Map<number, AnnualMonthSourceRequest>();
    for (const month of months) {
      if (overviewCanSupplyCurrentMonth(overview, month)) continue;
      if (overviewIsAnnualForMonth(overview, month)) continue;
      const request = annualMonthRequest(
        month,
        overview.filter.category,
        overview.filter.currency,
        overview.asOf
      );
      if (request === null) continue;
      byYear.set(request.year, request);
    }
    return Array.from(byYear.values()).sort(
      (left, right) => left.year - right.year
    );
  }, [overview, months]);

  useEffect(() => {
    ensure(annualRequests);
  }, [annualRequests, ensure]);

  const annualEntryFor = useCallback(
    (month: string): POReportSourceEntry | null => {
      if (overview === null) return null;
      const request = annualMonthRequest(
        month,
        overview.filter.category,
        overview.filter.currency,
        overview.asOf
      );
      if (request === null) return null;
      return get(request.key);
    },
    [overview, get]
  );

  const readouts = useMemo(() => {
    if (overview === null) return [];
    return months.map((month) => {
      const entry = annualEntryFor(month);
      return resolveMonthReadout({
        overview,
        month,
        annualSource: entry?.report ?? null,
        annualStatus: entry?.status ?? "loading",
        annualError: entry?.error ?? "",
      });
    });
  }, [overview, months, annualEntryFor]);

  const aggregate = useMemo(() => {
    if (overview === null || allYear === null) return null;
    const entry = annualEntryFor(`${allYear}-01`);
    return resolveMonthAggregate({
      overview,
      year: allYear,
      annualSource: entry?.report ?? null,
      annualStatus: entry?.status ?? "loading",
      annualError: entry?.error ?? "",
    });
  }, [overview, allYear, annualEntryFor]);

  const changeSelection = useCallback(
    (next: string[], active: string | null) => {
      setExplicitMonths(next);
      setActiveMonth(active);
      setNotice("");
    },
    []
  );

  const selectAll = useCallback(
    (year: string) => {
      const result = allYearSelection(year);
      if (result.rejected) {
        setNotice(result.message);
        return;
      }
      changeSelection(result.months, `${year}-01`);
    },
    [changeSelection]
  );

  const toggleMonth = useCallback(
    (month: string) => {
      const result = toggleMonthInSelection(months, month);
      if (result.rejected) {
        setNotice(result.message);
        return;
      }
      const removed = months.includes(month) && !result.months.includes(month);
      if (!removed) {
        changeSelection(result.months, month);
        return;
      }
      const previousIndex = months.indexOf(month);
      const nextActive =
        result.months[Math.min(previousIndex, result.months.length - 1)] ?? null;
      changeSelection(result.months, nextActive);
    },
    [months, changeSelection]
  );

  const changeLayout = useCallback((next: ReportMonthLayout) => {
    setLayout(next);
    setNotice("");
  }, []);

  const moveActive = useCallback(
    (delta: number) => {
      if (months.length === 0) return;
      if (months.length === 1) {
        const next = shiftMonth(months[0], delta);
        if (monthStart(next) === "") return;
        changeSelection([next], next);
        return;
      }
      if (allYear !== null && layout === "aggregate") return;
      const next = nextActiveMonth(months, effectiveActive, delta);
      if (next !== null) setActiveMonth(next);
    },
    [allYear, changeSelection, effectiveActive, layout, months]
  );

  const selectPrevious = useCallback(() => moveActive(-1), [moveActive]);
  const selectNext = useCallback(() => moveActive(1), [moveActive]);

  const retryMonth = useCallback(
    (month: string) => {
      if (overview === null) return;
      const request = annualMonthRequest(
        month,
        overview.filter.category,
        overview.filter.currency,
        overview.asOf
      );
      if (request === null) return;
      retry(request.key);
    },
    [overview, retry]
  );

  const clearNotice = useCallback(() => setNotice(""), []);

  const aggregateMode = allYear !== null && layout === "aggregate";
  const exportReady =
    overview !== null &&
    (months.length === 0
      ? true
      : aggregateMode
        ? aggregate !== null &&
          aggregate.status === "ready" &&
          aggregate.complete &&
          aggregate.scopeMatchesOverview
        : readouts.length === months.length &&
          readouts.every(
            (readout) =>
              readout.status === "ready" &&
              monthReadoutExportable(readout, overview)
          ));

  const exportSelection = useMemo<PurchaseOrderReportViewSelection | null>(() => {
    if (!exportReady || overview === null) return null;
    if (months.length === 0) {
      return { months: [], layout: "individual" };
    }
    if (aggregateMode && allYear !== null) {
      const entry = annualEntryFor(`${allYear}-01`);
      const source = overviewIsAnnualForMonth(overview, `${allYear}-01`)
        ? overview
        : entry?.status === "ready"
          ? entry.report
          : null;
      if (source === null) return null;
      return {
        months: months.map((month) => ({
          month,
          source,
          usesOverviewMonthSummary: false,
        })),
        layout: "aggregate",
      };
    }
    const selection = months.map((month, index) => {
      const readout = readouts[index];
      const source = overviewCanSupplyCurrentMonth(overview, month)
        ? overview
        : overviewIsAnnualForMonth(overview, month)
          ? overview
          : annualEntryFor(month)?.report ?? null;
      if (readout === undefined || source === null) return null;
      return {
        month,
        source,
        usesOverviewMonthSummary: readout.usesOverviewMonthSummary,
      };
    });
    if (selection.some((entry) => entry === null)) return null;
    return {
      months: selection.filter(
        (entry): entry is NonNullable<typeof entry> => entry !== null
      ),
      layout: "individual",
    };
  }, [
    aggregateMode,
    allYear,
    annualEntryFor,
    exportReady,
    months,
    overview,
    readouts,
  ]);

  const pickerStartYear = useCallback(() => {
    const serverYear =
      overview === null ? "" : monthKeyFromDate(overview.asOf).slice(0, 4);
    return initialPickerYear(months, effectiveActive, serverYear);
  }, [effectiveActive, months, overview]);

  return {
    months,
    activeMonth: effectiveActive,
    layout,
    allYear,
    notice,
    readouts,
    aggregate,
    canExport: exportReady,
    exportSelection,
    selectAll,
    toggleMonth,
    setLayout: changeLayout,
    selectPrevious,
    selectNext,
    retryMonth,
    clearNotice,
    pickerStartYear,
  };
}

export interface POReportCategoryComparison {
  /** Selected codes in supplied source order; [] for an explicit empty state. */
  codes: string[];
  activeCode: string | null;
  allSelected: boolean;
  layout: ReportComparisonLayout;
  readout: PurchaseOrderCategoryReadout;
  canExport: boolean;
  exportSelection: PurchaseOrderReportViewCategorySelection | null;
  selectAll: () => void;
  toggleCategory: (code: string) => void;
  setLayout: (layout: ReportComparisonLayout) => void;
  selectPrevious: () => void;
  selectNext: () => void;
  retry: () => void;
}

function emptyCategoryReadout(): PurchaseOrderCategoryReadout {
  return {
    status: "loading",
    error: "",
    rangeLabel: "",
    currency: "",
    asOf: "",
    generatedAt: "",
    sourceLabel: "",
    supplied: [],
    selected: [],
    total: null,
    scopeMatchesOverview: false,
  };
}

/**
 * Owns the local category selection over the unrestricted all-category source
 * at the overview's exact inherited range, bucket and currency. It never
 * changes Performance's query, and every transition over a loaded source is
 * pure local state. The shared registry makes a previously loaded
 * all-category snapshot reusable across page mounts of this hook.
 */
export function usePOReportCategoryComparison(
  overview: PurchaseOrderReportData | null,
  registry?: POReportSourceRegistry
): POReportCategoryComparison {
  const ownRegistry = usePOReportSourceRegistry(
    registry === undefined ? overview : null
  );
  const sourceRegistry = registry ?? ownRegistry;
  const { ensure, get, retry } = sourceRegistry;
  const [explicitCodes, setExplicitCodes] = useState<string[] | null>(null);
  const [activeCode, setActiveCode] = useState<string | null>(null);
  const [layout, setLayout] = useState<ReportComparisonLayout>("aggregate");

  const request = useMemo<CategorySourceRequest | null>(
    () => (overview === null ? null : categorySourceRequest(overview)),
    [overview]
  );

  useEffect(() => {
    if (request !== null) ensure([request]);
  }, [request, ensure]);

  const entry = request === null ? null : get(request.key);
  const readout = useMemo(
    () =>
      overview === null
        ? emptyCategoryReadout()
        : resolveCategoryReadout({
            overview,
            source: entry?.report ?? null,
            status: entry?.status ?? "loading",
            error: entry?.error ?? "",
            explicitCodes,
          }),
    [entry, explicitCodes, overview]
  );

  const suppliedCodes = useMemo(
    () => readout.supplied.map((item) => item.code),
    [readout]
  );
  const codes = useMemo(
    () => readout.selected.map((item) => item.code),
    [readout]
  );
  const allSelected = isAllCategoriesSelected(codes, suppliedCodes);
  const effectiveActive =
    activeCode !== null && codes.includes(activeCode)
      ? activeCode
      : codes[0] ?? null;
  const source = entry?.report ?? null;

  const selectAll = useCallback(() => {
    // All is select-all, not an implicit clear: a repeat press is a no-op.
    if (allSelected) return;
    const change = selectAllCategories(suppliedCodes);
    setExplicitCodes(change.explicitCodes);
    setActiveCode(change.activeCode);
  }, [allSelected, suppliedCodes]);

  const toggleCategory = useCallback(
    (code: string) => {
      if (!suppliedCodes.includes(code)) return;
      const change = toggleCategoryInSelection(
        explicitCodes,
        suppliedCodes,
        code
      );
      setExplicitCodes(change.explicitCodes);
      setActiveCode(change.activeCode);
    },
    [explicitCodes, suppliedCodes]
  );

  const changeLayout = useCallback((next: ReportComparisonLayout) => {
    setLayout(next);
  }, []);

  const moveActive = useCallback(
    (delta: number) => {
      if (codes.length === 0) return;
      // Aggregate presents no card to focus; the buttons are omitted there.
      if (allSelected && layout === "aggregate") return;
      if (codes.length === 1) {
        // Single replaces the membership with the next supplied row, clamped
        // at the first and last supplied category (same math as the subset
        // focus helper, applied over the whole supplied universe).
        const next = nextActiveCategory(suppliedCodes, codes[0], delta);
        if (next === null) return;
        setExplicitCodes([next]);
        setActiveCode(next);
        return;
      }
      const next = nextActiveCategory(codes, effectiveActive, delta);
      if (next !== null) setActiveCode(next);
    },
    [allSelected, codes, effectiveActive, layout, suppliedCodes]
  );

  const selectPrevious = useCallback(() => moveActive(-1), [moveActive]);
  const selectNext = useCallback(() => moveActive(1), [moveActive]);

  const retrySources = useCallback(() => {
    if (request !== null) retry(request.key);
  }, [request, retry]);

  const emptySelected = explicitCodes !== null && explicitCodes.length === 0;
  const canExport =
    overview !== null &&
    (emptySelected ||
      (source !== null &&
        readout.status === "ready" &&
        readout.scopeMatchesOverview &&
        !(
          allSelected &&
          layout === "aggregate" &&
          readout.supplied.length === 0
        )));

  const exportSelection =
    useMemo<PurchaseOrderReportViewCategorySelection | null>(() => {
      if (!canExport || overview === null) return null;
      if (emptySelected) {
        // An explicit empty section needs no financial source; the builder
        // normalizes this to source 0 with no extra snapshot.
        return { source: overview, codes: [], layout: "individual" };
      }
      if (source === null) return null;
      return {
        source,
        codes: readout.selected.map((item) => item.code),
        layout: allSelected ? layout : "individual",
      };
    }, [
      allSelected,
      canExport,
      emptySelected,
      layout,
      overview,
      readout.selected,
      source,
    ]);

  return {
    codes,
    activeCode: effectiveActive,
    allSelected,
    layout,
    readout,
    canExport,
    exportSelection,
    selectAll,
    toggleCategory,
    setLayout: changeLayout,
    selectPrevious,
    selectNext,
    retry: retrySources,
  };
}
