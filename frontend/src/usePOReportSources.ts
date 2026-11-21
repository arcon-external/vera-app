// Issue #641/#642: the month comparison's per-mounted source registry.
//
// Reports is mounted once per signed-in workspace and unmounted on sign-out, so
// this cache is scoped to the authenticated page instance: there is no client
// organization id to key on and no cross-session persistence. The registry
// stores frozen annual snapshots, deduplicates identical in-flight reads, and
// loads exactly one source per required calendar year. It never fetches
// speculatively for a current month the overview's own monthSummary resolves,
// and it never fetches while the picker pages another year.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { GetPurchaseOrderReport } from "@backend/backend/Quotation";
import { errorMessage } from "./errors";
import {
  allYearForMonths,
  allYearSelection,
  annualMonthRequest,
  annualSourceKey,
  initialPickerYear,
  monthKeyFromDate,
  monthReadoutExportable,
  monthStart,
  nextActiveMonth,
  overviewCanSupplyCurrentMonth,
  overviewIsAnnualForMonth,
  resolveMonthAggregate,
  resolveMonthReadout,
  shiftMonth,
  toggleMonthInSelection,
  type AnnualMonthSourceRequest,
  type MonthSourceStatus,
  type PurchaseOrderMonthAggregate,
  type PurchaseOrderMonthReadout,
  type ReportMonthLayout,
} from "./poReportComparison";
import {
  normalizePurchaseOrderReport,
  type PurchaseOrderReportData,
  type PurchaseOrderReportQuery,
} from "./poReport";
import type { PurchaseOrderReportViewSelection } from "./poReportExport";

interface AnnualEntry {
  key: string;
  status: MonthSourceStatus;
  query: PurchaseOrderReportQuery;
  report: PurchaseOrderReportData | null;
  error: string;
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
 */
export function usePOReportMonthComparison(
  overview: PurchaseOrderReportData | null
): POReportMonthComparison {
  const [explicitMonths, setExplicitMonths] = useState<string[] | null>(null);
  const [activeMonth, setActiveMonth] = useState<string | null>(null);
  const [layout, setLayout] = useState<ReportMonthLayout>("aggregate");
  const [notice, setNotice] = useState("");
  const [entries, setEntries] = useState<Record<string, AnnualEntry>>({});
  const [retryTick, setRetryTick] = useState(0);
  const entriesRef = useRef<Record<string, AnnualEntry>>({});
  const inFlight = useRef<Set<string>>(new Set());
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

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

  // A displayed overview that is itself an exact annual snapshot is a valid
  // annual source. Keep it so a later Performance bucket change reuses the
  // already-loaded monthly evidence instead of refetching the identical query.
  // A year-bucketed overview is never inserted and never substitutes.
  const overviewAnnual = useMemo(() => {
    if (overview === null) return null;
    const filter = overview.filter;
    if (filter.bucket !== "month") return null;
    const year = filter.from.slice(0, 4);
    if (!/^\d{4}$/.test(year)) return null;
    if (
      filter.from !== `${year}-01-01` ||
      filter.to !== `${Number(year) + 1}-01-01`
    ) {
      return null;
    }
    return { key: annualSourceKey(filter, overview.asOf), query: { ...filter } };
  }, [overview]);

  useEffect(() => {
    if (overview === null || overviewAnnual === null) return;
    const existing = entriesRef.current[overviewAnnual.key];
    if (existing !== undefined && existing.status === "ready") return;
    const entry: AnnualEntry = {
      key: overviewAnnual.key,
      status: "ready",
      query: overviewAnnual.query,
      report: overview,
      error: "",
    };
    entriesRef.current = { ...entriesRef.current, [overviewAnnual.key]: entry };
    setEntries(entriesRef.current);
  }, [overview, overviewAnnual]);

  // One annual read per required calendar year, never per card. A current month
  // resolved by the overview's monthSummary needs no annual read at all.
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
    for (const request of annualRequests) {
      const key = request.key;
      const existing = entriesRef.current[key];
      if (
        existing !== undefined &&
        (existing.status === "ready" || existing.status === "loading")
      ) {
        continue;
      }
      if (inFlight.current.has(key)) continue;
      inFlight.current.add(key);
      const entry: AnnualEntry = {
        key,
        status: "loading",
        query: request.query,
        report: null,
        error: "",
      };
      entriesRef.current = { ...entriesRef.current, [key]: entry };
      setEntries(entriesRef.current);
      void (async () => {
        try {
          const raw = await GetPurchaseOrderReport(request.query);
          const report = normalizePurchaseOrderReport(raw);
          if (!mounted.current) return;
          entriesRef.current = {
            ...entriesRef.current,
            [key]: { ...entry, status: "ready", report },
          };
          setEntries(entriesRef.current);
        } catch (caught) {
          if (!mounted.current) return;
          entriesRef.current = {
            ...entriesRef.current,
            [key]: {
              ...entry,
              status: "error",
              error: errorMessage(
                caught,
                "The monthly snapshot could not be loaded. Check the API connection and try again."
              ),
            },
          };
          setEntries(entriesRef.current);
        } finally {
          inFlight.current.delete(key);
        }
      })();
    }
  }, [annualRequests, retryTick]);

  const annualEntryFor = useCallback(
    (month: string): AnnualEntry | null => {
      if (overview === null) return null;
      const request = annualMonthRequest(
        month,
        overview.filter.category,
        overview.filter.currency,
        overview.asOf
      );
      if (request === null) return null;
      return entries[request.key] ?? null;
    },
    [overview, entries]
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
      const next = { ...entriesRef.current };
      delete next[request.key];
      entriesRef.current = next;
      setEntries(next);
      setRetryTick((current) => current + 1);
    },
    [overview]
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
