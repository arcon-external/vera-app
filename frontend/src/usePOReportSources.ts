// Issue #641: the month browser's per-mounted source registry.
//
// Reports is mounted once per signed-in workspace and unmounted on sign-out, so
// this cache is scoped to the authenticated page instance: there is no client
// organization id to key on and no cross-session persistence. The registry
// stores frozen overview/year snapshots, deduplicates identical in-flight
// reads, and reuses a year after its first load. It never fetches speculatively
// for a current month the overview's own monthSummary already resolves.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { GetPurchaseOrderReport } from "@backend/backend/Quotation";
import { errorMessage } from "./errors";
import {
  annualMonthRequest,
  annualSourceKey,
  monthKeyFromDate,
  monthReadoutExportable,
  monthStart,
  overviewCanSupplyCurrentMonth,
  overviewIsAnnualForMonth,
  resolveMonthReadout,
  resolveMonthSource,
  shiftMonth,
  type MonthSourceStatus,
  type PurchaseOrderMonthReadout,
} from "./poReportComparison";
import {
  normalizePurchaseOrderReport,
  type PurchaseOrderReportData,
  type PurchaseOrderReportQuery,
} from "./poReport";

interface AnnualEntry {
  key: string;
  status: MonthSourceStatus;
  query: PurchaseOrderReportQuery;
  report: PurchaseOrderReportData | null;
  error: string;
}

export interface POReportMonthBrowser {
  /** Canonical selected month, or "" before the first overview response. */
  month: string;
  readout: PurchaseOrderMonthReadout | null;
  /** The exact selected-month source, when it is ready. */
  source: PurchaseOrderReportData | null;
  usesOverviewMonthSummary: boolean;
  /** The selected evidence is ready and shares the overview's scope. */
  canExport: boolean;
  selectPrevious: () => void;
  selectNext: () => void;
  selectMonth: (month: string) => void;
  retryMonth: () => void;
}

/**
 * Owns the independent month selection and its annual source registry. The
 * overview is the displayed report, so a stale filter still shows the old
 * month's own scope until the new overview arrives; the chart keeps receiving
 * the same object.
 */
export function usePOReportMonthBrowser(
  overview: PurchaseOrderReportData | null
): POReportMonthBrowser {
  const [explicitMonth, setExplicitMonth] = useState<string | null>(null);
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

  const monthly = overview === null ? "" : monthKeyFromDate(overview.asOf);
  const month = explicitMonth ?? monthly;

  // A displayed overview that is itself an exact year/monthly snapshot is also
  // a valid annual source. Keep it in the registry so a later Performance
  // bucket change reuses the already-loaded monthly evidence instead of
  // refetching the identical query. A year-bucketed overview is never inserted
  // and never substitutes for a monthly source.
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

  const annual = useMemo(() => {
    if (overview === null || month === "") return null;
    if (overviewCanSupplyCurrentMonth(overview, month)) return null;
    if (overviewIsAnnualForMonth(overview, month)) return null;
    return annualMonthRequest(
      month,
      overview.filter.category,
      overview.filter.currency,
      overview.asOf
    );
  }, [overview, month]);

  useEffect(() => {
    if (annual === null) return;
    const key = annual.key;
    const existing = entriesRef.current[key];
    if (
      existing !== undefined &&
      (existing.status === "ready" || existing.status === "loading")
    ) {
      return;
    }
    if (inFlight.current.has(key)) return;
    inFlight.current.add(key);
    const entry: AnnualEntry = {
      key,
      status: "loading",
      query: annual.query,
      report: null,
      error: "",
    };
    entriesRef.current = { ...entriesRef.current, [key]: entry };
    setEntries(entriesRef.current);
    void (async () => {
      try {
        const raw = await GetPurchaseOrderReport(annual.query);
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
  }, [annual, retryTick]);

  const resolution = useMemo(() => {
    if (overview === null || month === "") return null;
    const entry = annual === null ? null : entries[annual.key];
    return resolveMonthSource({
      overview,
      month,
      annualSource: entry?.report ?? null,
      annualStatus: entry?.status ?? "loading",
      annualError: entry?.error ?? "",
    });
  }, [overview, month, annual, entries]);

  const readout = useMemo(() => {
    if (overview === null || month === "") return null;
    const entry = annual === null ? null : entries[annual.key];
    return resolveMonthReadout({
      overview,
      month,
      annualSource: entry?.report ?? null,
      annualStatus: entry?.status ?? "loading",
      annualError: entry?.error ?? "",
    });
  }, [overview, month, annual, entries]);

  const selectMonth = useCallback((next: string) => {
    if (monthStart(next) === "") return;
    setExplicitMonth(next);
  }, []);

  const selectPrevious = useCallback(() => {
    if (month === "") return;
    const previous = shiftMonth(month, -1);
    if (monthStart(previous) === "") return;
    setExplicitMonth(previous);
  }, [month]);

  const selectNext = useCallback(() => {
    if (month === "") return;
    const next = shiftMonth(month, 1);
    if (monthStart(next) === "") return;
    setExplicitMonth(next);
  }, [month]);

  const retryMonth = useCallback(() => {
    if (annual === null) return;
    const next = { ...entriesRef.current };
    delete next[annual.key];
    entriesRef.current = next;
    setEntries(next);
    setRetryTick((current) => current + 1);
  }, [annual]);

  return {
    month,
    readout,
    source: resolution?.source ?? null,
    usesOverviewMonthSummary:
      resolution?.usesOverviewMonthSummary ?? false,
    canExport:
      readout !== null &&
      overview !== null &&
      monthReadoutExportable(readout, overview),
    selectPrevious,
    selectNext,
    selectMonth,
    retryMonth,
  };
}
