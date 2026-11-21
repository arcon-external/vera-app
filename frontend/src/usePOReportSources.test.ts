// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  MAX_PENDING_REPORT_REQUESTS,
  MAX_REPORT_CACHE_ENTRIES,
  usePOReportCategoryComparison,
  usePOReportMonthComparison,
  usePOReportOverviewSource,
  usePOReportSourceRegistry,
  type POReportOverviewSource,
  type POReportSourceRegistry,
} from "./usePOReportSources";
import { reportFilterKey } from "./poReportPage";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportYearFixture,
} from "./poReportFixtures";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportQuery,
} from "./poReport";

// Issue #645: the reports source registry's bounded lifecycle. Every test drives
// the public registry/hook surface (ensure, get, retry, invalidateAll, pins) and
// observes the real binding calls, never the private cache internals.

const { reportMock } = vi.hoisted(() => ({
  reportMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  GetPurchaseOrderReport: (...args: unknown[]) => reportMock(...args),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function query(
  overrides: Partial<PurchaseOrderReportQuery> = {}
): PurchaseOrderReportQuery {
  return {
    from: "2026-01-01",
    to: "2027-01-01",
    bucket: "month",
    category: "all",
    currency: "IDR",
    ...overrides,
  };
}

function keyOf(value: PurchaseOrderReportQuery): string {
  return reportFilterKey(value);
}

function reportFor(
  value: PurchaseOrderReportQuery,
  asOf = "2026-11-19"
): PurchaseOrderReportData {
  const report = structuredClone(purchaseOrderReportYearFixture());
  report.filter = { ...value };
  report.asOf = asOf;
  return report;
}

function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason: unknown) => void;
} {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, resolve, reject };
}

let host: HTMLElement;
let root: Root;
let registry: POReportSourceRegistry | undefined;
let overviewSource: POReportOverviewSource | undefined;

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

beforeEach(() => {
  reportMock.mockReset();
  reportMock.mockImplementation(async (value: PurchaseOrderReportQuery) =>
    reportFor(value)
  );
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  registry = undefined;
  overviewSource = undefined;
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
  });
  host.remove();
  document.body.innerHTML = "";
});

function RegistryHarness({ active }: { active: boolean }) {
  registry = usePOReportSourceRegistry({ active });
  return null;
}

async function mountRegistry(active = true): Promise<void> {
  await act(async () => {
    root.render(createElement(RegistryHarness, { active }));
  });
}

async function setRegistryActive(active: boolean): Promise<void> {
  await act(async () => {
    root.render(createElement(RegistryHarness, { active }));
  });
}

function ensure(
  requests: readonly { key: string; query: PurchaseOrderReportQuery }[]
): void {
  act(() => {
    registry!.ensure(requests);
  });
}

function calls(): PurchaseOrderReportQuery[] {
  return reportMock.mock.calls.map(
    (call) => call[0] as PurchaseOrderReportQuery
  );
}

describe("registry freshness generation (issue #645)", () => {
  it("starts at generation 1 and never fetches while hidden", async () => {
    await mountRegistry(false);
    expect(registry!.generation).toBe(1);

    ensure([
      { key: keyOf(query()), query: query() },
      {
        key: keyOf(query({ currency: "USD" })),
        query: query({ currency: "USD" }),
      },
    ]);
    await flush();
    expect(reportMock).not.toHaveBeenCalled();

    act(() => {
      registry!.invalidateAll();
    });
    await flush();
    expect(registry!.generation).toBe(2);
    expect(reportMock).not.toHaveBeenCalled();

    await setRegistryActive(true);
    ensure([{ key: keyOf(query()), query: query() }]);
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(calls()[0]).toEqual(query());
  });

  it("marks ready evidence stale on invalidation and re-reads only on ensure", async () => {
    await mountRegistry(true);
    ensure([{ key: keyOf(query()), query: query() }]);
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
    const ready = registry!.get(keyOf(query()));
    expect(ready?.status).toBe("ready");
    expect(registry!.isStale(ready)).toBe(false);

    act(() => {
      registry!.invalidateAll();
    });
    await flush();
    expect(registry!.generation).toBe(2);
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(registry!.get(keyOf(query()))?.status).toBe("ready");
    expect(registry!.isStale(registry!.get(keyOf(query())))).toBe(true);

    ensure([{ key: keyOf(query()), query: query() }]);
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    const fresh = registry!.get(keyOf(query()));
    expect(fresh?.status).toBe("ready");
    expect(registry!.isStale(fresh)).toBe(false);
  });

  it("marks cached evidence stale while hidden without fetching", async () => {
    await mountRegistry(true);
    const request = { key: keyOf(query()), query: query() };
    ensure([request]);
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);

    await setRegistryActive(false);
    act(() => {
      registry!.invalidateAll();
    });
    await flush();
    expect(registry!.generation).toBe(2);
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(registry!.get(request.key)?.status).toBe("ready");
    expect(registry!.isStale(registry!.get(request.key))).toBe(true);

    // A hidden ensure is still a no-op; the evidence stays stale, not refetched.
    ensure([request]);
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);

    await setRegistryActive(true);
    ensure([request]);
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(registry!.isStale(registry!.get(request.key))).toBe(false);
  });

  it("drops a late response from a superseded generation", async () => {
    await mountRegistry(true);
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    ensure([{ key: keyOf(query()), query: query() }]);
    expect(reportMock).toHaveBeenCalledTimes(1);

    act(() => {
      registry!.invalidateAll();
    });
    await act(async () => {
      pending.resolve(reportFor(query(), "2026-11-18"));
    });
    await flush();
    const late = registry!.get(keyOf(query()));
    expect(late?.status).not.toBe("ready");
    expect(late?.report ?? null).toBeNull();

    ensure([{ key: keyOf(query()), query: query() }]);
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(registry!.get(keyOf(query()))?.status).toBe("ready");
  });

  it("drops a response that resolves after the page unmounted", async () => {
    await mountRegistry(true);
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    ensure([{ key: keyOf(query()), query: query() }]);
    const captured = registry!;
    await act(async () => {
      root.unmount();
    });
    await act(async () => {
      pending.resolve(reportFor(query()));
    });
    await flush();
    expect(captured.get(keyOf(query()))?.status).toBe("loading");
    expect(captured.get(keyOf(query()))?.report ?? null).toBeNull();
  });

  it("turns a mismatched-filter response into an error without storing it", async () => {
    await mountRegistry(true);
    reportMock.mockImplementationOnce(async () =>
      reportFor(query({ currency: "USD" }))
    );
    ensure([{ key: keyOf(query()), query: query() }]);
    await flush();
    const entry = registry!.get(keyOf(query()));
    expect(entry?.status).toBe("error");
    expect(entry?.report ?? null).toBeNull();
    expect(entry?.error).toContain("does not match");
  });
});

describe("registry retry and deduplication (issue #645)", () => {
  it("deduplicates simultaneous identical reads from every caller", async () => {
    await mountRegistry(true);
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    const request = { key: keyOf(query()), query: query() };
    ensure([request, request, request]);
    expect(reportMock).toHaveBeenCalledTimes(1);

    ensure([request]);
    expect(reportMock).toHaveBeenCalledTimes(1);
    await act(async () => {
      pending.resolve(reportFor(query()));
    });
    await flush();
    expect(registry!.get(request.key)?.status).toBe("ready");
  });

  it("retries a key while preserving its last-known report", async () => {
    await mountRegistry(true);
    ensure([{ key: keyOf(query()), query: query() }]);
    await flush();
    const first = registry!.get(keyOf(query()))?.report;
    expect(first).not.toBeNull();

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    act(() => {
      registry!.retry(keyOf(query()));
    });
    const refreshing = registry!.get(keyOf(query()));
    expect(refreshing?.status).toBe("loading");
    expect(refreshing?.report).toBe(first);
    expect(refreshing?.report).not.toBeNull();

    await act(async () => {
      pending.resolve(reportFor(query(), "2026-11-20"));
    });
    await flush();
    const refreshed = registry!.get(keyOf(query()));
    expect(refreshed?.status).toBe("ready");
    expect(refreshed?.report?.asOf).toBe("2026-11-20");
    expect(refreshed?.report).not.toBe(first);
  });

  it("keeps the last-known report on a same-key error and replaces it on retry", async () => {
    await mountRegistry(true);
    ensure([{ key: keyOf(query()), query: query() }]);
    await flush();
    const first = registry!.get(keyOf(query()))?.report;

    act(() => {
      registry!.invalidateAll();
    });
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    ensure([{ key: keyOf(query()), query: query() }]);
    await act(async () => {
      pending.reject(new Error("The report API is unreachable."));
    });
    await flush();
    const failed = registry!.get(keyOf(query()));
    expect(failed?.status).toBe("error");
    expect(failed?.error).toBe("The report API is unreachable.");
    expect(failed?.report).toBe(first);
    expect(registry!.isStale(failed)).toBe(true);

    reportMock.mockImplementation(async (value: PurchaseOrderReportQuery) =>
      reportFor(value, "2026-11-20")
    );
    act(() => {
      registry!.retry(keyOf(query()));
    });
    await flush();
    expect(registry!.get(keyOf(query()))?.status).toBe("ready");
    expect(registry!.get(keyOf(query()))?.report?.asOf).toBe("2026-11-20");
  });
});

describe("registry bounds and pinning (issue #645)", () => {
  function distinctQueries(count: number): PurchaseOrderReportQuery[] {
    return Array.from({ length: count }, (_, index) =>
      query({
        from: `2025-01-${String(index + 1).padStart(2, "0")}`,
        to: `2025-02-${String(index + 1).padStart(2, "0")}`,
      })
    );
  }

  function request(value: PurchaseOrderReportQuery): {
    key: string;
    query: PurchaseOrderReportQuery;
  } {
    return { key: keyOf(value), query: value };
  }

  /** Fill in batches so a burst never trips the pending cap by design. */
  async function fill(values: readonly PurchaseOrderReportQuery[]): Promise<void> {
    for (let index = 0; index < values.length; index += 1) {
      ensure([request(values[index])]);
      if ((index + 1) % MAX_PENDING_REPORT_REQUESTS === 0) await flush();
    }
    await flush();
  }

  it("caps unpinned entries at 24 and evicts the least recently used", async () => {
    await mountRegistry(true);
    const values = distinctQueries(MAX_REPORT_CACHE_ENTRIES + 1);
    await fill(values.slice(0, MAX_REPORT_CACHE_ENTRIES));
    // Touch the oldest entry so the next-oldest becomes the eviction victim.
    expect(registry!.get(keyOf(values[0]))?.status).toBe("ready");
    ensure([request(values[MAX_REPORT_CACHE_ENTRIES])]);
    await flush();

    const present = values.filter((value) => registry!.get(keyOf(value)) !== null);
    expect(present).toHaveLength(MAX_REPORT_CACHE_ENTRIES);
    expect(registry!.get(keyOf(values[0]))).not.toBeNull();
    expect(registry!.get(keyOf(values[1]))).toBeNull();
    expect(
      registry!.get(keyOf(values[MAX_REPORT_CACHE_ENTRIES]))
    ).not.toBeNull();

    // The evicted scope is fetched again when it becomes visible again.
    const before = reportMock.mock.calls.length;
    ensure([{ key: keyOf(values[1]), query: values[1] }]);
    await flush();
    expect(reportMock.mock.calls.length).toBe(before + 1);
  });

  it("never evicts a visible or exporting key", async () => {
    await mountRegistry(true);
    const values = distinctQueries(MAX_REPORT_CACHE_ENTRIES + 4);
    const visible = values[0];
    const exporting = values[1];
    act(() => {
      registry!.pinVisible("months", [keyOf(visible)]);
    });
    const release = (() => {
      let fn: () => void = () => undefined;
      act(() => {
        fn = registry!.pinExport([keyOf(exporting)]);
      });
      return fn;
    })();

    await fill(values.slice(0, MAX_REPORT_CACHE_ENTRIES));
    await fill(values.slice(MAX_REPORT_CACHE_ENTRIES));

    expect(registry!.get(keyOf(visible))).not.toBeNull();
    expect(registry!.get(keyOf(exporting))).not.toBeNull();
    expect(registry!.get(keyOf(values[2]))).toBeNull();

    act(() => {
      registry!.pinVisible("months", []);
    });
    act(() => {
      release();
    });
    for (const value of values.slice(0, 2)) {
      ensure([{ key: keyOf(value), query: value }]);
    }
    await flush();
    const afterRelease = values.filter(
      (value) => registry!.get(keyOf(value)) !== null
    ).length;
    expect(afterRelease).toBeLessThanOrEqual(MAX_REPORT_CACHE_ENTRIES);
  });

  it("holds more than the cap only while every entry is pinned, then evicts on release", async () => {
    await mountRegistry(true);
    const values = distinctQueries(MAX_REPORT_CACHE_ENTRIES + 1);
    let release: () => void = () => undefined;
    act(() => {
      release = registry!.pinExport(values.map(keyOf));
    });
    for (const value of values) {
      ensure([{ key: keyOf(value), query: value }]);
    }
    await flush();
    // No unpinned entry can be sacrificed, so the pin wins over the cap.
    expect(
      values.filter((value) => registry!.get(keyOf(value)) !== null)
    ).toHaveLength(MAX_REPORT_CACHE_ENTRIES + 1);

    act(() => {
      release();
    });
    await flush();
    expect(
      values.filter((value) => registry!.get(keyOf(value)) !== null)
    ).toHaveLength(MAX_REPORT_CACHE_ENTRIES);
  });

  it("detaches the oldest unpinned pending read at the pending cap", async () => {
    await mountRegistry(true);
    const values = distinctQueries(MAX_PENDING_REPORT_REQUESTS + 1);
    const pending = values.map(() => deferred<PurchaseOrderReportData>());
    let index = 0;
    reportMock.mockImplementation(() => {
      const current = pending[index];
      index += 1;
      return current.promise;
    });

    for (const value of values.slice(0, MAX_PENDING_REPORT_REQUESTS)) {
      ensure([{ key: keyOf(value), query: value }]);
    }
    expect(reportMock).toHaveBeenCalledTimes(MAX_PENDING_REPORT_REQUESTS);
    // The 17th read starts; the oldest unpinned token is detached to make room.
    ensure([{ key: keyOf(values[16]), query: values[16] }]);
    expect(reportMock).toHaveBeenCalledTimes(MAX_PENDING_REPORT_REQUESTS + 1);

    await act(async () => {
      pending[0].resolve(reportFor(values[0]));
    });
    await flush();
    expect(registry!.get(keyOf(values[0]))?.status).not.toBe("ready");
    expect(registry!.get(keyOf(values[0]))?.report ?? null).toBeNull();

    // A detached key is fetchable again.
    const retry = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => retry.promise);
    ensure([{ key: keyOf(values[0]), query: values[0] }]);
    expect(reportMock).toHaveBeenCalledTimes(MAX_PENDING_REPORT_REQUESTS + 2);
    await act(async () => {
      retry.resolve(reportFor(values[0]));
    });
    await flush();
    expect(registry!.get(keyOf(values[0]))?.status).toBe("ready");
  });
});

describe("report hooks share one registry (issue #645)", () => {
  const overviewReport = (): PurchaseOrderReportData => {
    const value = reportFor(query());
    value.monthSummary = structuredClone(
      purchaseOrderReportYearFixture().monthSummary!
    );
    return value;
  };

  function SharedHarness({ active }: { active: boolean }) {
    const shared = usePOReportSourceRegistry({ active });
    const overview = usePOReportOverviewSource(
      query(),
      shared,
      active
    );
    const months = usePOReportMonthComparison(overviewReport(), shared, {
      active,
    });
    const categories = usePOReportCategoryComparison(overviewReport(), shared, {
      active,
    });
    registry = shared;
    overviewSource = overview;
    (SharedHarness as unknown as { months?: unknown }).months = months;
    (SharedHarness as unknown as { categories?: unknown }).categories =
      categories;
    return null;
  }

  it("fetches the shared default scope once across overview, months and categories", async () => {
    await act(async () => {
      root.render(createElement(SharedHarness, { active: true }));
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(calls()[0]).toEqual(query());
    expect(overviewSource!.report?.filter).toEqual(query());
    expect(registry!.get(keyOf(query()))?.status).toBe("ready");
  });

  it("fetches nothing while hidden and revalidates on activation", async () => {
    await act(async () => {
      root.render(createElement(SharedHarness, { active: false }));
    });
    await flush();
    expect(reportMock).not.toHaveBeenCalled();

    await act(async () => {
      root.render(createElement(SharedHarness, { active: true }));
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
  });

  const activeOverview = structuredClone(purchaseOrderReportFixture());

  function MonthActiveHarness({ active }: { active: boolean }) {
    const shared = usePOReportSourceRegistry({ active });
    const monthComparison = usePOReportMonthComparison(activeOverview, shared, {
      active,
    });
    usePOReportCategoryComparison(activeOverview, shared, { active });
    registry = shared;
    (MonthActiveHarness as unknown as { comparison?: unknown }).comparison =
      monthComparison;
    return null;
  }

  it("gates both comparison hooks on activation and revalidates after invalidation", async () => {
    await act(async () => {
      root.render(createElement(MonthActiveHarness, { active: false }));
    });
    await flush();
    expect(reportMock).not.toHaveBeenCalled();

    // Activating fetches the annual source and the all-category source once each.
    await act(async () => {
      root.render(createElement(MonthActiveHarness, { active: true }));
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);

    const api = () =>
      (MonthActiveHarness as unknown as {
        comparison: ReturnType<typeof usePOReportMonthComparison>;
      }).comparison;
    expect(api().readouts[0].status).toBe("ready");
    expect(api().canExport).toBe(true);

    // While active, an invalidation re-ensures the visible scope automatically.
    act(() => {
      registry!.invalidateAll();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(4);
    expect(api().readouts[0].status).toBe("ready");
    expect(api().readouts[0].stale).toBe(false);
    expect(api().canExport).toBe(true);
  });
});

describe("month and category hooks retain last-known evidence (issue #645)", () => {
  const overview = (): PurchaseOrderReportData =>
    structuredClone(purchaseOrderReportFixture());

  const annualQuery = query();

  function MonthHarness() {
    const shared = usePOReportSourceRegistry({ active: true });
    const comparison = usePOReportMonthComparison(overview(), shared, {
      active: true,
    });
    registry = shared;
    (MonthHarness as unknown as { comparison?: unknown }).comparison =
      comparison;
    return null;
  }

  const monthApi = () =>
    (MonthHarness as unknown as {
      comparison: ReturnType<typeof usePOReportMonthComparison>;
    }).comparison;

  it("keeps the annual readout visible and stale while a same-scope retry is in flight", async () => {
    await act(async () => {
      root.render(createElement(MonthHarness));
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(monthApi().readouts[0].status).toBe("ready");
    expect(monthApi().readouts[0].stale).toBe(false);
    expect(monthApi().canExport).toBe(true);
    const primary = monthApi().readouts[0].primary;
    expect(primary).not.toBeNull();

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    act(() => {
      registry!.retry(keyOf(annualQuery));
    });
    expect(monthApi().readouts[0].status).toBe("ready");
    expect(monthApi().readouts[0].stale).toBe(true);
    expect(monthApi().readouts[0].refreshing).toBe(true);
    expect(monthApi().readouts[0].primary).toEqual(primary);
    expect(monthApi().canExport).toBe(false);

    await act(async () => {
      pending.reject(new Error("The monthly API is unreachable."));
    });
    await flush();
    expect(monthApi().readouts[0].status).toBe("ready");
    expect(monthApi().readouts[0].stale).toBe(true);
    expect(monthApi().readouts[0].refreshing).toBe(false);
    expect(monthApi().readouts[0].error).toBe(
      "The monthly API is unreachable."
    );
    expect(monthApi().readouts[0].primary).toEqual(primary);
    expect(monthApi().canExport).toBe(false);
  });

  it("rejects a stale aggregate from export", async () => {
    function AggregateHarness() {
      const shared = usePOReportSourceRegistry({ active: true });
      const comparison = usePOReportMonthComparison(overview(), shared, {
        active: true,
      });
      registry = shared;
      (AggregateHarness as unknown as { comparison?: unknown }).comparison =
        comparison;
      return null;
    }
    await act(async () => {
      root.render(createElement(AggregateHarness));
    });
    await flush();
    const api = () =>
      (AggregateHarness as unknown as {
        comparison: ReturnType<typeof usePOReportMonthComparison>;
      }).comparison;
    act(() => {
      api().selectAll("2026");
    });
    await flush();
    expect(api().aggregate?.status).toBe("ready");
    expect(api().canExport).toBe(true);

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    act(() => {
      registry!.retry(keyOf(annualQuery));
    });
    expect(api().aggregate?.status).toBe("ready");
    expect(api().aggregate?.stale).toBe(true);
    expect(api().canExport).toBe(false);
    await act(async () => {
      pending.resolve(reportFor(annualQuery));
    });
    await flush();
    expect(api().aggregate?.stale).toBe(false);
    expect(api().canExport).toBe(true);
  });

  function CategoryHarness() {
    const shared = usePOReportSourceRegistry({ active: true });
    const comparison = usePOReportCategoryComparison(overview(), shared, {
      active: true,
    });
    registry = shared;
    (CategoryHarness as unknown as { comparison?: unknown }).comparison =
      comparison;
    return null;
  }

  it("keeps category rows and the error while a retry is in flight or fails", async () => {
    await act(async () => {
      root.render(createElement(CategoryHarness));
    });
    await flush();
    const api = () =>
      (CategoryHarness as unknown as {
        comparison: ReturnType<typeof usePOReportCategoryComparison>;
      }).comparison;
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(api().readout.status).toBe("ready");
    expect(api().readout.stale).toBe(false);
    expect(api().readout.supplied.length).toBeGreaterThan(0);
    expect(api().canExport).toBe(true);

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    act(() => {
      api().retry();
    });
    expect(api().readout.status).toBe("ready");
    expect(api().readout.stale).toBe(true);
    expect(api().readout.refreshing).toBe(true);
    expect(api().readout.supplied.length).toBeGreaterThan(0);
    expect(api().canExport).toBe(false);

    await act(async () => {
      pending.reject(new Error("The all-category API is unreachable."));
    });
    await flush();
    expect(api().readout.status).toBe("ready");
    expect(api().readout.stale).toBe(true);
    expect(api().readout.refreshing).toBe(false);
    expect(api().readout.error).toBe(
      "The all-category API is unreachable."
    );
    expect(api().readout.supplied.length).toBeGreaterThan(0);
    expect(api().canExport).toBe(false);
  });
});

describe("usePOReportOverviewSource (issue #645)", () => {
  function OverviewHarness({
    active,
    value,
  }: {
    active: boolean;
    value: PurchaseOrderReportQuery;
  }) {
    const shared = usePOReportSourceRegistry({ active });
    overviewSource = usePOReportOverviewSource(value, shared, active);
    registry = shared;
    return null;
  }

  async function render(
    value: PurchaseOrderReportQuery,
    active = true
  ): Promise<void> {
    await act(async () => {
      root.render(createElement(OverviewHarness, { active, value }));
    });
  }

  it("keeps the last good overview while a new scope loads, marked stale", async () => {
    await render(query());
    await flush();
    const first = overviewSource!.report;
    expect(first?.filter.currency).toBe("IDR");
    expect(overviewSource!.stale).toBe(false);

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    await render(query({ currency: "USD" }));
    expect(overviewSource!.report).toBe(first);
    expect(overviewSource!.loading).toBe(true);
    expect(overviewSource!.stale).toBe(true);

    await act(async () => {
      pending.resolve(reportFor(query({ currency: "USD" })));
    });
    await flush();
    expect(overviewSource!.report?.filter.currency).toBe("USD");
    expect(overviewSource!.stale).toBe(false);
    expect(overviewSource!.loading).toBe(false);
  });

  it("ignores a late old-currency reply", async () => {
    const idr = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => idr.promise);
    await render(query());
    expect(overviewSource!.loading).toBe(true);

    const usd = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => usd.promise);
    await render(query({ currency: "USD" }));
    await act(async () => {
      usd.resolve(reportFor(query({ currency: "USD" })));
    });
    await flush();
    await act(async () => {
      idr.resolve(reportFor(query()));
    });
    await flush();

    expect(overviewSource!.report?.filter.currency).toBe("USD");
    expect(overviewSource!.stale).toBe(false);
    expect(overviewSource!.loading).toBe(false);
  });

  it("retains the previous figures and exposes the error when a refresh fails", async () => {
    await render(query());
    await flush();
    const first = overviewSource!.report;

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    act(() => {
      overviewSource!.retry();
    });
    expect(overviewSource!.report).toBe(first);
    expect(overviewSource!.stale).toBe(true);
    expect(overviewSource!.loading).toBe(true);

    await act(async () => {
      pending.reject(new Error("The report API is unreachable."));
    });
    await flush();
    expect(overviewSource!.report).toBe(first);
    expect(overviewSource!.error).toBe("The report API is unreachable.");
    expect(overviewSource!.stale).toBe(true);
    expect(overviewSource!.loading).toBe(false);
  });

  it("fetches nothing while hidden and revalidates once on activation", async () => {
    await render(query(), false);
    await flush();
    expect(reportMock).not.toHaveBeenCalled();
    expect(overviewSource!.report).toBeNull();
    expect(overviewSource!.loading).toBe(false);

    await render(query(), true);
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(overviewSource!.report?.filter).toEqual(query());
  });
});
