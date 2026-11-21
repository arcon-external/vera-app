// Issue #641/#642/#643: the reports page's per-mounted shared source registry.
//
// Reports is mounted once per signed-in workspace and unmounted on sign-out, so
// this cache is scoped to the authenticated page instance: there is no client
// organization id to key on and no cross-session persistence. #645 adds the
// lifecycle: a freshness generation, bounded LRU eviction with visible/export
// pinning, a pending cap, and late-response rejection by token, generation,
// mount and filter key. The registry never fetches speculatively for a current
// month the overview's own monthSummary resolves, and it never fetches while
// the picker pages another year.

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
import { reportFilterKey, reportMatchesFilter } from "./poReportPage";
import type {
  PurchaseOrderReportViewCategorySelection,
  PurchaseOrderReportViewSelection,
} from "./poReportExport";

/** The cache bound: enough for the 120-month span, its years and overrides. */
export const MAX_REPORT_CACHE_ENTRIES = 24;
/** The in-flight bound: above the 14 sources one maximum export can name. */
export const MAX_PENDING_REPORT_REQUESTS = 16;

/** One immutable source snapshot cached by exact filter identity. */
export interface POReportSourceEntry {
  /** Exact canonical `reportFilterKey(query)` (`from|to|bucket|category|currency`). */
  key: string;
  status: MonthSourceStatus;
  query: PurchaseOrderReportQuery;
  /** Most recent snapshot for this exact `key` only; never reused across scopes. */
  report: PurchaseOrderReportData | null;
  error: string;
  /** Registry generation that produced `report` (0 when `report === null`). */
  reportGeneration: number;
  /** Registry generation of the latest started/completed attempt for `key`. */
  attemptGeneration: number;
  /** Monotonic access tick used for LRU eviction of unpinned entries. */
  lastUsed: number;
}

export type POReportPinOwner = "overview" | "months" | "categories";

/** The per-mounted page's shared, bounded source registry. */
export interface POReportSourceRegistry {
  /** Current workspace freshness generation (starts at 1; increments on invalidateAll). */
  generation: number;
  /** Reads an entry by exact `reportFilterKey(query)` and touches its LRU order. */
  get: (key: string) => POReportSourceEntry | null;
  /** True when `entry` has a snapshot older than `generation`. */
  isStale: (entry: POReportSourceEntry | null) => boolean;
  /**
   * Ensures every requested key has a fresh snapshot or an in-flight attempt at
   * `generation`. No-op when the workspace is hidden. Deduplicates simultaneous
   * identical reads across the overview, months and categories.
   */
  ensure: (
    requests: readonly { key: string; query: PurchaseOrderReportQuery }[]
  ) => void;
  /** Forces a fresh attempt for `key` at the current generation (preserves the last-known `report` on the same key). */
  retry: (key: string) => void;
  /**
   * Marks all cached entries stale by incrementing `generation` and detaching
   * superseded in-flight tokens. Does NOT fetch; active surfaces revalidate on
   * their next `ensure()` effect.
   */
  invalidateAll: () => void;
  /** Registers one surface's currently visible required keys (cannot be evicted). */
  pinVisible: (owner: POReportPinOwner, keys: readonly string[]) => void;
  /** Pins every source key in an active export until the returned release runs. */
  pinExport: (keys: readonly string[]) => () => void;
}

export interface POReportSourceRegistryOptions {
  /** False while Reports is hidden: `ensure` fetches nothing. Default true. */
  active?: boolean;
}

interface InFlightToken {
  token: number;
  generation: number;
}

function isReportData(value: unknown): value is PurchaseOrderReportData {
  return (
    typeof value === "object" &&
    value !== null &&
    "filter" in value &&
    "asOf" in value
  );
}

/**
 * The month and category browsers' one source cache. Every ready overview is
 * seeded under its exact filter identity, so switching Performance's category
 * on the same range/currency reuses the all-category snapshot instead of
 * refetching it. Identical in-flight keys are deduplicated; a retry supersedes
 * the previous attempt and a late response can only write its own key.
 *
 * `options.active === false` (the Reports route is hidden) makes `ensure` a
 * no-op: invalidation marks evidence stale, it never wakes a hidden read.
 *
 * The accepted #643 page still passes the mounted overview as the argument so
 * its snapshot seeds the shared key; the #645 page integration passes
 * `{ active }` instead. The report shape is detected structurally so both
 * callers stay source-compatible during that transition.
 */
export function usePOReportSourceRegistry(
  options?: POReportSourceRegistryOptions | PurchaseOrderReportData | null
): POReportSourceRegistry {
  const legacyOverview = isReportData(options) ? options : null;
  const active = isReportData(options) ? true : options?.active ?? true;
  const activeRef = useRef(active);
  activeRef.current = active;

  const [entries, setEntries] = useState<Record<string, POReportSourceEntry>>(
    {}
  );
  const entriesRef = useRef<Record<string, POReportSourceEntry>>(entries);
  const [generation, setGeneration] = useState(1);
  const generationRef = useRef(generation);
  const inFlight = useRef<Map<string, InFlightToken>>(new Map());
  const attemptSeq = useRef(0);
  const accessSeq = useRef(0);
  const pinSeq = useRef(0);
  const mounted = useRef(true);
  const visiblePins = useRef<Record<POReportPinOwner, Set<string>>>({
    overview: new Set(),
    months: new Set(),
    categories: new Set(),
  });
  const exportPins = useRef<Map<number, Set<string>>>(new Map());

  useEffect(() => {
    mounted.current = true;
    entriesRef.current = entries;
    return () => {
      mounted.current = false;
    };
  }, []);

  const pinnedKeys = useCallback((): Set<string> => {
    const pinned = new Set<string>();
    for (const keys of Object.values(visiblePins.current)) {
      for (const key of keys) pinned.add(key);
    }
    for (const keys of exportPins.current.values()) {
      for (const key of keys) pinned.add(key);
    }
    return pinned;
  }, []);

  /**
   * The bounded-eviction pass: while over the cap, sacrifice the least recently
   * used unpinned entry, preferring non-ready work. Pinned entries are never
   * removed; an all-pinned overflow waits for an export pin release.
   */
  const evictOverflow = useCallback(
    (
      next: Record<string, POReportSourceEntry>,
      keepKey?: string
    ): Record<string, POReportSourceEntry> => {
      let current = next;
      while (Object.keys(current).length > MAX_REPORT_CACHE_ENTRIES) {
        const pinned = pinnedKeys();
        const candidates = Object.values(current).filter(
          (entry) => !pinned.has(entry.key) && entry.key !== keepKey
        );
        if (candidates.length === 0) break;
        const notReady = candidates.filter(
          (entry) => entry.status !== "ready"
        );
        const pool = notReady.length > 0 ? notReady : candidates;
        const victim = pool.reduce((oldest, entry) =>
          entry.lastUsed < oldest.lastUsed ? entry : oldest
        );
        inFlight.current.delete(victim.key);
        const trimmed: Record<string, POReportSourceEntry> = {};
        for (const [key, entry] of Object.entries(current)) {
          if (key !== victim.key) trimmed[key] = entry;
        }
        current = trimmed;
      }
      return current;
    },
    [pinnedKeys]
  );

  const commit = useCallback(
    (entry: POReportSourceEntry) => {
      const next = evictOverflow(
        {
          ...entriesRef.current,
          [entry.key]: entry,
        },
        entry.key
      );
      entriesRef.current = next;
      setEntries(next);
    },
    [evictOverflow]
  );

  const start = useCallback(
    (key: string, query: PurchaseOrderReportQuery) => {
      const attemptGeneration = generationRef.current;
      // Slow abandoned replies must not accumulate. Sacrifice the oldest
      // unpinned in-flight token; the 14-source export bound keeps an
      // all-pinned overflow unreachable in real use.
      while (inFlight.current.size >= MAX_PENDING_REPORT_REQUESTS) {
        const pinned = pinnedKeys();
        let victim: string | null = null;
        for (const pendingKey of inFlight.current.keys()) {
          if (!pinned.has(pendingKey)) {
            victim = pendingKey;
            break;
          }
        }
        if (victim === null) break;
        inFlight.current.delete(victim);
      }

      const token = ++attemptSeq.current;
      inFlight.current.set(key, { token, generation: attemptGeneration });
      const previous = entriesRef.current[key];
      commit({
        key,
        status: "loading",
        query,
        report: previous?.report ?? null,
        error: "",
        reportGeneration:
          previous === undefined || previous.report === null
            ? 0
            : previous.reportGeneration,
        attemptGeneration,
        lastUsed: ++accessSeq.current,
      });

      void (async () => {
        try {
          const raw = await GetPurchaseOrderReport(query);
          if (!mounted.current) return;
          const live = inFlight.current.get(key);
          if (
            live === undefined ||
            live.token !== token ||
            live.generation !== generationRef.current
          ) {
            return;
          }
          const report = normalizePurchaseOrderReport(raw);
          if (reportFilterKey(report.filter) !== reportFilterKey(query)) {
            inFlight.current.delete(key);
            commit({
              key,
              status: "error",
              query,
              report: null,
              error:
                query.category === "all"
                  ? "The loaded snapshot does not match this report's range, bucket, currency or all-category scope."
                  : "The loaded snapshot does not match this report's range, bucket, category or currency.",
              reportGeneration: 0,
              attemptGeneration,
              lastUsed: ++accessSeq.current,
            });
            return;
          }
          inFlight.current.delete(key);
          commit({
            key,
            status: "ready",
            query,
            report,
            error: "",
            reportGeneration: generationRef.current,
            attemptGeneration,
            lastUsed: ++accessSeq.current,
          });
        } catch (caught) {
          if (!mounted.current) return;
          const live = inFlight.current.get(key);
          if (
            live === undefined ||
            live.token !== token ||
            live.generation !== generationRef.current
          ) {
            return;
          }
          inFlight.current.delete(key);
          const previous = entriesRef.current[key];
          commit({
            key,
            status: "error",
            query,
            report: previous?.report ?? null,
            error: errorMessage(
              caught,
              "The report could not be loaded. Check the API connection and try again."
            ),
            reportGeneration:
              previous === undefined || previous.report === null
                ? 0
                : previous.reportGeneration,
            attemptGeneration,
            lastUsed: ++accessSeq.current,
          });
        }
      })();
    },
    [commit, pinnedKeys]
  );

  const ensure = useCallback(
    (
      requests: readonly { key: string; query: PurchaseOrderReportQuery }[]
    ) => {
      if (!activeRef.current) return;
      for (const request of requests) {
        const pending = inFlight.current.get(request.key);
        if (pending !== undefined) {
          if (pending.generation === generationRef.current) continue;
          inFlight.current.delete(request.key);
        }
        const existing = entriesRef.current[request.key];
        if (existing !== undefined) {
          if (
            existing.status === "ready" &&
            existing.reportGeneration === generationRef.current
          ) {
            existing.lastUsed = ++accessSeq.current;
            continue;
          }
          if (
            existing.status === "error" &&
            existing.attemptGeneration === generationRef.current
          ) {
            continue;
          }
        }
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

  const invalidateAll = useCallback(() => {
    generationRef.current += 1;
    inFlight.current.clear();
    setGeneration(generationRef.current);
  }, []);

  const get = useCallback(
    (key: string): POReportSourceEntry | null => {
      const entry = entries[key] ?? null;
      if (entry !== null) entry.lastUsed = ++accessSeq.current;
      return entry;
    },
    [entries]
  );

  const isStale = useCallback(
    (entry: POReportSourceEntry | null): boolean =>
      entry !== null &&
      entry.report !== null &&
      entry.reportGeneration < generation,
    [generation]
  );

  const pinVisible = useCallback(
    (owner: POReportPinOwner, keys: readonly string[]) => {
      visiblePins.current[owner] = new Set(keys);
    },
    []
  );

  const pinExport = useCallback(
    (keys: readonly string[]): (() => void) => {
      const id = ++pinSeq.current;
      exportPins.current.set(id, new Set(keys));
      return () => {
        if (!exportPins.current.delete(id)) return;
        const trimmed = evictOverflow(entriesRef.current);
        if (trimmed !== entriesRef.current) {
          entriesRef.current = trimmed;
          setEntries(trimmed);
        }
      };
    },
    [evictOverflow]
  );

  // The accepted #643 page's live overview is authoritative for its own key: it
  // seeds the shared snapshot and supersedes any identical read still in flight.
  // The #645 page integration seeds through `usePOReportOverviewSource` instead.
  useEffect(() => {
    if (legacyOverview === null) return;
    const key = reportFilterKey(legacyOverview.filter);
    const existing = entriesRef.current[key];
    if (
      existing !== undefined &&
      existing.status === "ready" &&
      existing.report === legacyOverview
    ) {
      return;
    }
    inFlight.current.delete(key);
    commit({
      key,
      status: "ready",
      query: { ...legacyOverview.filter },
      report: legacyOverview,
      error: "",
      reportGeneration:
        existing !== undefined && existing.report === legacyOverview
          ? existing.reportGeneration
          : generationRef.current,
      attemptGeneration: generationRef.current,
      lastUsed: ++accessSeq.current,
    });
  }, [legacyOverview, commit]);

  return useMemo(
    () => ({
      generation,
      get,
      isStale,
      ensure,
      retry,
      invalidateAll,
      pinVisible,
      pinExport,
    }),
    [
      ensure,
      generation,
      get,
      invalidateAll,
      isStale,
      pinExport,
      pinVisible,
      retry,
    ]
  );
}

interface RetainedSourceState {
  status: MonthSourceStatus;
  error: string;
  stale: boolean;
  refreshing: boolean;
}

/**
 * What the last-known snapshot for one entry may honestly show right now. A
 * retained report during a re-read stays visible, but the surface is stale and
 * must never export. A same-scope failure keeps the snapshot and its error.
 */
function retainedSourceState(
  entry: POReportSourceEntry | null,
  staleByGeneration: boolean
): RetainedSourceState {
  if (entry === null) {
    return { status: "loading", error: "", stale: false, refreshing: false };
  }
  if (entry.report === null) {
    return {
      status: entry.status,
      error: entry.error,
      stale: false,
      refreshing: false,
    };
  }
  if (entry.status === "error") {
    return {
      status: "ready",
      error: entry.error,
      stale: true,
      refreshing: false,
    };
  }
  if (entry.status === "loading") {
    return {
      status: "ready",
      error: "",
      stale: true,
      refreshing: true,
    };
  }
  return {
    status: "ready",
    error: "",
    stale: staleByGeneration,
    refreshing: false,
  };
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
  registry?: POReportSourceRegistry,
  options?: { active?: boolean }
): POReportMonthComparison {
  const active = options?.active ?? true;
  const ownRegistry = usePOReportSourceRegistry(
    registry === undefined ? overview : { active }
  );
  const sourceRegistry = registry ?? ownRegistry;
  const { ensure, get, retry, isStale, generation } = sourceRegistry;
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
    if (!active) return;
    ensure(
      annualRequests.map((request) => ({
        key: reportFilterKey(request.query),
        query: request.query,
      }))
    );
  }, [active, annualRequests, ensure, generation]);

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
      return get(reportFilterKey(request.query));
    },
    [overview, get]
  );

  const readouts = useMemo(() => {
    if (overview === null) return [];
    return months.map((month) => {
      const request = annualMonthRequest(
        month,
        overview.filter.category,
        overview.filter.currency,
        overview.asOf
      );
      // The overview's own monthSummary (or an exact annual overview) owns this
      // month; no separate annual entry's freshness may relabel that evidence.
      const usesAnnual =
        request !== null &&
        !overviewCanSupplyCurrentMonth(overview, month) &&
        !overviewIsAnnualForMonth(overview, month);
      const entry = annualEntryFor(month);
      const retained = retainedSourceState(
        entry,
        entry !== null && isStale(entry)
      );
      const resolved = resolveMonthReadout({
        overview,
        month,
        annualSource: usesAnnual ? entry?.report ?? null : null,
        annualStatus: usesAnnual ? retained.status : "loading",
        annualError: usesAnnual ? retained.error : "",
        stale: usesAnnual && retained.stale,
        refreshing: usesAnnual && retained.refreshing,
      });
      return usesAnnual &&
        resolved.status === "ready" &&
        retained.error !== ""
        ? { ...resolved, error: retained.error }
        : resolved;
    });
  }, [overview, months, annualEntryFor, isStale]);

  const aggregate = useMemo(() => {
    if (overview === null || allYear === null) return null;
    const entry = annualEntryFor(`${allYear}-01`);
    const usesAnnual = !overviewIsAnnualForMonth(overview, `${allYear}-01`);
    const retained = retainedSourceState(
      entry,
      entry !== null && isStale(entry)
    );
    const resolved = resolveMonthAggregate({
      overview,
      year: allYear,
      annualSource: usesAnnual ? entry?.report ?? null : null,
      annualStatus: usesAnnual ? retained.status : "loading",
      annualError: usesAnnual ? retained.error : "",
      stale: usesAnnual && retained.stale,
      refreshing: usesAnnual && retained.refreshing,
    });
    return usesAnnual &&
      resolved.status === "ready" &&
      retained.error !== ""
      ? { ...resolved, error: retained.error }
      : resolved;
  }, [overview, allYear, annualEntryFor, isStale]);

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
      retry(reportFilterKey(request.query));
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
          aggregate.scopeMatchesOverview &&
          aggregate.stale !== true &&
          aggregate.refreshing !== true
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
    stale: false,
    refreshing: false,
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
  registry?: POReportSourceRegistry,
  options?: { active?: boolean }
): POReportCategoryComparison {
  const active = options?.active ?? true;
  const ownRegistry = usePOReportSourceRegistry(
    registry === undefined ? overview : { active }
  );
  const sourceRegistry = registry ?? ownRegistry;
  const { ensure, get, retry, isStale, generation } = sourceRegistry;
  const [explicitCodes, setExplicitCodes] = useState<string[] | null>(null);
  const [activeCode, setActiveCode] = useState<string | null>(null);
  const [layout, setLayout] = useState<ReportComparisonLayout>("aggregate");

  const request = useMemo<CategorySourceRequest | null>(
    () => (overview === null ? null : categorySourceRequest(overview)),
    [overview]
  );
  const requestKey =
    request === null ? null : reportFilterKey(request.query);

  useEffect(() => {
    if (!active || request === null || requestKey === null) return;
    ensure([{ key: requestKey, query: request.query }]);
  }, [active, ensure, generation, request, requestKey]);

  const entry = requestKey === null ? null : get(requestKey);
  const readout = useMemo(() => {
    if (overview === null) return emptyCategoryReadout();
    const retained = retainedSourceState(
      entry,
      entry !== null && isStale(entry)
    );
    const resolved = resolveCategoryReadout({
      overview,
      source: entry?.report ?? null,
      status: retained.status,
      error: retained.error,
      explicitCodes,
      stale: retained.stale,
      refreshing: retained.refreshing,
    });
    return retained.status === "ready" && retained.error !== ""
      ? { ...resolved, error: retained.error }
      : resolved;
  }, [entry, explicitCodes, overview, isStale]);

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
    if (requestKey !== null) retry(requestKey);
  }, [requestKey, retry]);

  const emptySelected = explicitCodes !== null && explicitCodes.length === 0;
  const canExport =
    overview !== null &&
    (emptySelected ||
      (source !== null &&
        readout.status === "ready" &&
        readout.scopeMatchesOverview &&
        readout.stale !== true &&
        readout.refreshing !== true &&
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

/**
 * Performance's own source, served by the same registry as both comparison
 * browsers. A filter change keeps the last successful snapshot visible (marked
 * stale) until its replacement is ready, matching the overview's existing
 * "previous figures while loading" behaviour; the comparison surfaces keep
 * their stricter no-old-money rule. A hidden workspace fetches nothing.
 */
export interface POReportOverviewSource {
  /** The last successful overview snapshot (retained while switching filters or refreshing). */
  report: PurchaseOrderReportData | null;
  loading: boolean;
  /** True when `report` does not match `query` OR belongs to an older generation. */
  stale: boolean;
  error: string;
  retry: () => void;
}

export function usePOReportOverviewSource(
  query: PurchaseOrderReportQuery | null,
  registry: POReportSourceRegistry,
  active = true
): POReportOverviewSource {
  const key = query === null ? null : reportFilterKey(query);
  const entry = key === null ? null : registry.get(key);
  const [lastReport, setLastReport] = useState<PurchaseOrderReportData | null>(
    null
  );

  useEffect(() => {
    if (entry === null || entry.report === null) return;
    setLastReport((current) =>
      current === entry.report ? current : entry.report
    );
  }, [entry]);

  useEffect(() => {
    if (!active || query === null || key === null) return;
    registry.ensure([{ key, query }]);
  }, [active, key, query, registry.ensure, registry.generation]);

  const report = entry?.report ?? lastReport;
  const refreshing =
    entry !== null && entry.report !== null && entry.status === "loading";
  const failed =
    entry !== null && entry.report !== null && entry.status === "error";
  const loading =
    active &&
    query !== null &&
    (entry === null || entry.status === "loading");
  const stale =
    report !== null &&
    (query === null ||
      !reportMatchesFilter(report, query) ||
      entry === null ||
      entry.report !== report ||
      registry.isStale(entry) ||
      refreshing ||
      failed);
  const error = entry?.error ?? "";

  const retry = useCallback(() => {
    if (key !== null) registry.retry(key);
  }, [key, registry.retry]);

  return { report, loading, stale, error, retry };
}
