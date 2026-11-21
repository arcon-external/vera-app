// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  PurchaseOrderReportsPage,
  PurchaseOrderReportsView,
} from "./PurchaseOrderReportsPage";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMaxFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportMonthWithRecordedFutureFixture,
  purchaseOrderReportYearFixture,
  purchaseOrderReportYearWithRecordedFutureFixture,
} from "../poReportFixtures";
import {
  initialReportFilters,
  reportPeriodRange,
  reportPresetRange,
  reportRangeLabel,
  toReportQuery,
} from "../poReportPage";
import type { PurchaseOrderReportData } from "../poReport";
import type { PurchaseOrderReportView } from "../poReportExport";
import { nextMonthStart } from "../poReportComparison";
import type { CurrencyInfo } from "../format";
import { todayWIB } from "../workspaceSettings";
import { ToastProvider } from "../toast";

// Issue #630: the Reports page is driven through its real binding boundary.
// The suite replaces only `GetPurchaseOrderReport`, so every assertion is the
// request the server would receive and the normalized snapshot the screen
// would show. It pins the honesty rules: exact filters, one fetch per change,
// stale responses dropped, null never rendered as 0%, errors without invented
// zeros, and an export that receives exactly the visible snapshot.

const { reportMock } = vi.hoisted(() => ({
  reportMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  GetPurchaseOrderReport: (...args: unknown[]) => reportMock(...args),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const CATALOGUE = [IDR, USD];

type Query = ReturnType<typeof toReportQuery>;

/**
 * The base fixture is a partial "this year" snapshot and predates the
 * server-owned monthSummary. These tests are about the page's older seams, so
 * they attach a truthful current-month readout from the report's own period
 * row: actual-to-date is present while the month forecast is explicitly
 * unavailable (the fixture's history is not a current-year sample). The
 * dedicated month tests below use the coherent year/month fixtures.
 */
function reportFor(query: Query): PurchaseOrderReportData {
  const report = purchaseOrderReportFixture();
  report.filter = { ...query };
  const month = report.asOf.slice(0, 7);
  const period = report.periods.find(
    (row) => row.periodStart === `${month}-01`
  );
  if (period !== undefined && report.monthSummary === undefined) {
    report.monthSummary = {
      month,
      from: `${month}-01`,
      to: nextMonthStart(month),
      asOf: report.asOf,
      mode: "month",
      actual: {
        poCount: period.poCount,
        amountMinor: period.amountMinor,
        costedMinor: period.costedMinor,
        uncostedMinor: period.uncostedMinor,
        capitalMinor: period.capitalMinor,
        marginMinor: period.marginMinor,
        marginBps: period.marginBps,
        coverageBps: period.coverageBps,
      },
      projection: {
        status: "unavailable",
        reason: "insufficient_months",
        method: "No current-year forecast in this fixture.",
      },
    };
  }
  return report;
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  reportMock.mockReset();
  reportMock.mockImplementation(async (query: Query) => reportFor(query));
  Element.prototype.scrollIntoView = () => undefined;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

async function customRange(): Promise<void> {
  const custom = document.querySelector<HTMLButtonElement>(
    'button[data-report-period="custom"]'
  );
  if (custom === null || custom.getAttribute("aria-pressed") === "true") return;
  await act(async () => {
    custom.click();
  });
  await flush();
}

async function choose(control: string, option: string): Promise<void> {
  if (
    (control === "Bucket" || control === "Date range preset") &&
    document.querySelector(`button[aria-label="${control}"]`) === null
  ) {
    await customRange();
  }
  await act(async () => {
    button(control).click();
  });
  const found = Array.from(
    document.querySelectorAll<HTMLElement>('[role="option"]')
  ).find((node) => node.textContent?.trim() === option);
  if (found === undefined) throw new Error(`no option ${option} in ${control}`);
  await act(async () => {
    found.click();
  });
  await flush();
}

async function typeDate(label: string, value: string): Promise<void> {
  if (
    document.querySelector(`input[aria-label="${label}"]`) === null
  ) {
    await customRange();
  }
  const input = document.querySelector<HTMLInputElement>(
    `input[aria-label="${label}"]`
  );
  if (input === null) throw new Error(`no date input ${label}`);
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value"
    )?.set;
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function calls(): Query[] {
  return reportMock.mock.calls.map((call) => call[0] as Query);
}

interface RenderOptions {
  defaultCurrency?: string;
  catalogue?: CurrencyInfo[] | null;
  onExport?: (view: PurchaseOrderReportView) => Promise<string>;
}

async function renderPage(options: RenderOptions = {}): Promise<void> {
  await act(async () => {
    root.render(
      <ToastProvider>
        <PurchaseOrderReportsPage
          defaultCurrency={options.defaultCurrency ?? "IDR"}
          currencyCatalogue={options.catalogue ?? CATALOGUE}
          onExport={options.onExport}
        />
      </ToastProvider>
    );
  });
  await flush();
}

/** The embedded chart's one "Selected month" link. */
function exactValuesLink(): HTMLAnchorElement {
  const found = document.querySelector<HTMLAnchorElement>(
    'a[href="#report-period-table"]'
  );
  if (found === null) throw new Error("no exact-value anchor");
  return found;
}

/** The chart's accessible description, which travels with the anchor. */
function exactValuesDescription(): string {
  return (
    document.querySelector("svg[data-chart-svg] desc")?.textContent ?? ""
  );
}

/**
 * Issue #644 F5: the exact-value anchor must name a real node at every month
 * state. jsdom implements no fragment navigation, so the browser contract is
 * pinned on the target itself — exactly one node, inside this page and the
 * selected-month section, focusable, and carrying the scroll margin that keeps
 * fragment scrolling below the sticky chrome. The focus call proves the
 * tabindex contract is real rather than a string.
 */
function assertExactValuesTarget(
  state: string,
  browser: HTMLElement | null
): HTMLElement {
  const link = exactValuesLink();
  expect(link.textContent, `${state}: anchor label`).toContain(
    "Selected month"
  );

  const targets = document.querySelectorAll("#report-period-table");
  expect(targets, `${state}: exactly one target`).toHaveLength(1);
  const target = targets[0] as HTMLElement;
  expect(container.contains(target), `${state}: target is in this page`).toBe(
    true
  );
  if (browser !== null) {
    expect(
      target === browser ||
        target.contains(browser) ||
        browser.contains(target),
      `${state}: target is the selected-month section`
    ).toBe(true);
  }
  expect(target.getAttribute("tabindex"), `${state}: focusable`).toBe("-1");
  expect(target.className, `${state}: scroll margin`).toContain("scroll-mt-6");

  target.focus();
  expect(document.activeElement, `${state}: focus lands`).toBe(target);
  return target;
}

describe("Reports filters (issue #630)", () => {
  it("fetches the initial current-year filter once", async () => {
    await renderPage();

    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(calls()[0]).toEqual(
      toReportQuery(initialReportFilters(todayWIB(), "IDR"))
    );
  });

  it("does not refetch when the page re-renders for an unrelated prop", async () => {
    await renderPage();
    expect(reportMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.render(
        <ToastProvider>
          <PurchaseOrderReportsPage
            defaultCurrency="IDR"
            currencyCatalogue={[...CATALOGUE]}
          />
        </ToastProvider>
      );
    });
    await flush();

    expect(reportMock).toHaveBeenCalledTimes(1);
  });

  it("sends every filter exactly, one fetch per change, including All and Unassigned", async () => {
    await renderPage();
    expect(reportMock).toHaveBeenCalledTimes(1);

    await choose("Category", "Unassigned");
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1].category).toBe("UNASSIGNED");

    await choose("Category", "All categories");
    expect(reportMock).toHaveBeenCalledTimes(3);
    expect(calls()[2].category).toBe("all");

    await choose("Currency", "USD");
    expect(reportMock).toHaveBeenCalledTimes(4);
    expect(calls()[3].currency).toBe("USD");

    await choose("Bucket", "Year");
    // The year-bucketed overview is a new read, but the month browser reuses
    // the exact annual snapshot the initial Year view already loaded.
    expect(reportMock).toHaveBeenCalledTimes(5);
    expect(calls()[4].bucket).toBe("year");
    expect(calls()[3].bucket).toBe("month");

    await typeDate("From date", "2025-12-01");
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(6);
    expect(calls()[5]).toMatchObject({
      from: "2025-12-01",
      to: "2027-01-01",
      bucket: "year",
      category: "all",
      currency: "USD",
    });
  });

  it("translates the inclusive end date into the API's exclusive to", async () => {
    await renderPage();
    await typeDate("To date", "2026-12-31");
    await flush();

    expect(calls().at(-1)?.to).toBe("2027-01-01");
  });

  it("keeps the previous snapshot while a filter loads and marks it stale", async () => {
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(async (query: Query) => reportFor(query));
    reportMock.mockImplementationOnce(() => pending.promise);
    await renderPage({ onExport: async () => "/tmp/opencode/x.pdf" });
    expect(document.body.textContent).toContain("November 2026");

    await choose("Bucket", "Year");

    // The previous month section stays visible under its own heading while
    // the next overview loads; it never repaints old money as the new period.
    expect(document.body.textContent).toContain("November 2026");
    expect(document.body.textContent).toContain("Updating the report");
    expect(button("Export PDF").disabled).toBe(true);

    act(() => {
      pending.resolve(reportFor({ ...calls()[1] }));
    });
    await flush();

    expect(document.body.textContent).not.toContain("Updating the report");
    expect(button("Export PDF").disabled).toBe(false);
    expect(document.body.textContent).toContain("November 2026");
  });

  it("drops a late response from the previous filter", async () => {
    await renderPage();
    await customRange();
    const first = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => first.promise);
    await typeDate("From date", "2026-01-02");
    reportMock.mockImplementationOnce(async (query: Query) => reportFor(query));

    await choose("Bucket", "Year");
    expect(document.body.textContent).toContain("2026");

    act(() => {
      first.resolve(
        reportFor({
          from: "2026-01-01",
          to: "2027-01-01",
          bucket: "month",
          category: "all",
          currency: "IDR",
        })
      );
    });
    await flush();

    // The slower month request must not repaint the year view. The new
    // Overview scope legitimately prints "2026" in its range, so the chart's
    // own bucket labels are the precise marker: a repaint would relabel them
    // with months, while the year bucket keeps the year vocabulary.
    expect(document.body.textContent).toContain("2026");
    const axisLabels = Array.from(
      document.querySelectorAll('[data-chart-axis="x"] text')
    ).map((node) => node.textContent);
    expect(axisLabels.length).toBeGreaterThan(0);
    expect(axisLabels.every((label) => label === "2026")).toBe(true);
  });

  it("shows a validation alarm and sends no request for an invalid range", async () => {
    await renderPage();
    await typeDate("From date", "2026-12-31");
    await flush();
    const beforeInvalid = reportMock.mock.calls.length;

    await typeDate("To date", "2026-01-01");
    await flush();

    expect(reportMock).toHaveBeenCalledTimes(beforeInvalid);
    expect(document.body.textContent).toContain(
      "The start date must be on or before the end date."
    );
  });
});

describe("Reports figures (issue #630)", () => {
  it("renders the KPI strip and coverage from the server's figures", async () => {
    await renderPage();

    for (const label of [
      "PO count",
      "PO amount",
      "Capital",
      "PO margin",
      "PO margin %",
    ]) {
      expect(document.body.textContent).toContain(label);
    }
    expect(document.body.textContent).toContain("12");
    expect(document.body.textContent).toContain("Rp480.000.000");
    expect(document.body.textContent).toContain("Rp390.000.000");
    expect(document.body.textContent).toContain("Rp42.000.000");
    expect(document.body.textContent).toContain("10.77%");
    expect(document.body.textContent).toContain("Coverage");
    expect(document.body.textContent).toContain("90.00%");
    expect(document.body.textContent).toContain("Rp48.000.000");
  });

  it("renders the whole maximum exact amount with its derived percentage", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = purchaseOrderReportMaxFixture();
      report.filter = { ...query };
      return report;
    });
    await renderPage();

    // The complete strings, not a rounded or clipped stand-in. The separate
    // width guard measures the synthetic widest percentage; this assembled
    // report carries the honest 0.00% of a 1000-minor margin against capital
    // near MAX_SAFE_INTEGER.
    expect(document.body.textContent).toContain("Rp9.007.199.254.740.991");
    expect(document.body.textContent).toContain("0.00%");
    expect(document.body.textContent).toContain("9,007,199,254,740,991");
  });

  it("formats the snapshot in the selected currency's own convention", async () => {
    await renderPage();
    await choose("Currency", "USD");

    expect(calls().at(-1)?.currency).toBe("USD");
    expect(document.body.textContent).toContain("$4,800,000.00");
    expect(document.body.textContent).toContain("$3,900,000.00");
    expect(document.body.textContent).not.toContain("Rp480.000.000");
  });

  it("keeps a previous snapshot visible when a refresh fails, never zeroing it", async () => {
    await renderPage();
    expect(document.body.textContent).toContain("Rp480.000.000");

    reportMock.mockRejectedValueOnce(new Error("Network is unreachable."));
    await choose("Bucket", "Year");

    expect(document.body.textContent).toContain("Network is unreachable.");
    expect(document.body.textContent).toContain("Showing the previous");
    expect(document.body.textContent).toContain("Rp480.000.000");
    expect(document.body.textContent).toContain("PO margin %");
  });

  it("renders signed losses and null percentages without inventing a zero", async () => {
    await renderPage();

    const rows = Array.from(document.querySelectorAll("tbody tr"));
    const fabrication = rows.find((row) =>
      row.textContent?.includes("Fabrication")
    );
    const unassigned = rows.find((row) =>
      row.textContent?.includes("Unassigned")
    );
    expect(fabrication?.textContent).toContain("Rp-30.000.000");
    expect(fabrication?.textContent).toContain("-20.00%");
    expect(unassigned?.textContent).toContain("-");
    expect(unassigned?.textContent).not.toContain("0.00%");
  });

  it("foots the category table and shows the selected month's six metrics", async () => {
    await renderPage();

    expect(document.body.textContent).toContain("Actual to date");
    const category = document.querySelector(
      'table[aria-label="Category totals"]'
    );
    expect(category).not.toBeNull();
    expect(category?.querySelector("tfoot")?.textContent).toContain(
      "Rp480.000.000"
    );
    const month = document.querySelector('table[aria-label="Period totals"]');
    expect(month).not.toBeNull();
    expect(month?.querySelectorAll("tbody tr")).toHaveLength(6);
    const labels = Array.from(month?.querySelectorAll("tbody th") ?? []).map(
      (node) => node.textContent
    );
    expect(labels).toEqual([
      "POs",
      "PO amount",
      "Costed amount",
      "Capital",
      "PO margin",
      "PO margin %",
    ]);
  });

  it("keeps the last column's body values inside the same right inset as its header", async () => {
    await renderPage();

    for (const label of ["Category totals", "Period totals"]) {
      const table = document.querySelector(`table[aria-label="${label}"]`);
      if (table === null) throw new Error(`expected the ${label} table`);
      const header = table.querySelector("thead th:last-child");
      const cell = table.querySelector("tbody td:last-child");
      expect(header?.className).toContain("text-right");
      expect(cell?.className).toContain("text-right");
    }
  });

  it("replaces the old projection tables with one current-period summary", async () => {
    await renderPage();

    expect(document.body.textContent).toContain("Current-period projection");
    expect(document.body.textContent).toContain(
      "Historical range, no current-period projection"
    );
    expect(document.body.textContent).toContain(
      "not the current month or year"
    );
    expect(document.body.textContent).not.toContain("Monthly projection");
    expect(document.body.textContent).not.toContain("Yearly projection");
  });

  it("never reports a projection as an actual total", async () => {
    await renderPage();

    const kpi = document.querySelector('[aria-label="Report KPIs"]');
    expect(kpi?.textContent).not.toContain("Apr 2026");
    expect(kpi?.textContent).toContain("Rp480.000.000");
  });

  it("names the selected range in the empty state and shows no zero KPI strip", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = reportFor(query);
      report.totals = {
        poCount: 0,
        amountMinor: 0,
        costedMinor: 0,
        uncostedMinor: 0,
        capitalMinor: 0,
        marginMinor: 0,
        marginBps: null,
        coverageBps: null,
      };
      report.periods = [];
      report.categories = [];
      return report;
    });
    await renderPage();

    expect(document.body.textContent).toContain("No purchase orders");
    expect(document.body.textContent).toContain(reportRangeLabel(calls()[0]));
    expect(document.querySelector('[aria-label="Report KPIs"]')).toBeNull();
    expect(
      document.querySelectorAll("[data-report-period-toggle]")
    ).toHaveLength(1);
    // The independent month browser is not hidden by the empty overview.
    expect(
      document.querySelector("[data-month-browser]")
    ).not.toBeNull();
  });

  it("shows a real error state with retry, never fake zeros", async () => {
    reportMock.mockRejectedValueOnce(new Error("The API is unreachable."));
    await renderPage();

    expect(document.body.textContent).toContain("Report unavailable");
    expect(document.body.textContent).toContain("The API is unreachable.");
    expect(document.body.textContent).not.toContain("PO amount");
    expect(document.body.textContent).not.toContain("Rp0");
    expect(
      document.querySelectorAll("[data-report-period-toggle]")
    ).toHaveLength(1);

    await act(async () => {
      button("Retry report").click();
    });
    await flush();

    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("PO amount");
    expect(
      document.querySelectorAll("[data-report-period-toggle]")
    ).toHaveLength(1);
  });

  it("shows a skeleton for the first load", async () => {
    reportMock.mockImplementationOnce(() => new Promise(() => {}));
    await act(async () => {
      root.render(
        <ToastProvider>
          <PurchaseOrderReportsPage
            defaultCurrency="IDR"
            currencyCatalogue={CATALOGUE}
          />
        </ToastProvider>
      );
    });
    await flush();

    expect(document.querySelector('[aria-label="Loading report"]')).not.toBeNull();
    expect(
      document.querySelectorAll("[data-report-period-toggle]")
    ).toHaveLength(1);
  });

  it("renders - rather than a fake IDR figure when the currency is unknown", async () => {
    await renderPage({ catalogue: [USD] });

    // The catalogue has no IDR entry, so every money figure is unknown.
    expect(document.body.textContent).not.toContain("Rp480.000.000");
    expect(document.body.textContent).toContain(
      "IDR is not in this app's currency catalogue"
    );
    expect(document.body.textContent).toContain("-");
  });
});

describe("Reports performance chart placement (issue #631)", () => {
  it("places the chart after the KPI strip and before the category tables", async () => {
    await renderPage();

    const kpis = document.querySelector('[aria-label="Report KPIs"]');
    const chart = document.querySelector("[data-report-chart]");
    const categories = document.querySelector(
      'table[aria-label="Category totals"]'
    );
    expect(kpis).not.toBeNull();
    expect(chart).not.toBeNull();
    expect(categories).not.toBeNull();
    expect(
      kpis!.compareDocumentPosition(chart!) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
    expect(
      chart!.compareDocumentPosition(categories!) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });

  it("draws no chart when the range has no purchase orders", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = reportFor(query);
      report.totals = {
        poCount: 0,
        amountMinor: 0,
        costedMinor: 0,
        uncostedMinor: 0,
        capitalMinor: 0,
        marginMinor: 0,
        marginBps: null,
        coverageBps: null,
      };
      report.periods = [];
      report.categories = [];
      return report;
    });
    await renderPage();

    expect(document.body.textContent).toContain("No purchase orders");
    expect(document.querySelector("[data-report-chart]")).toBeNull();
    expect(
      document.querySelectorAll("[data-report-period-toggle]")
    ).toHaveLength(1);
  });

  it("keeps the previous chart on the previous snapshot while a refresh fails", async () => {
    await renderPage();
    const before = document.querySelector("[data-report-chart]");
    expect(before?.textContent).toContain("11 actual monthly periods");

    reportMock.mockRejectedValueOnce(new Error("Network is unreachable."));
    await choose("Bucket", "Year");

    expect(document.body.textContent).toContain("Network is unreachable.");
    expect(document.body.textContent).toContain("Showing the previous");
    const after = document.querySelector("[data-report-chart]");
    expect(after).toBe(before);
    expect(after?.textContent).toContain("11 actual monthly periods");
  });

  it("anchors the chart's table link to the period table", async () => {
    await renderPage();
    const link = document.querySelector<HTMLAnchorElement>(
      '[data-report-chart] a[href="#report-period-table"]'
    );
    const table = document.querySelector("#report-period-table");
    expect(link).not.toBeNull();
    expect(table).not.toBeNull();
    expect(table?.getAttribute("aria-label")).toBe("Period totals");
    expect(table?.getAttribute("tabindex")).toBe("-1");
  });
});

describe("current period toggle (issue #638)", () => {
  const PINNED_TODAY = "2026-11-19";

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(`${PINNED_TODAY}T03:00:00Z`));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  function respond(query: Query): PurchaseOrderReportData {
    let report: PurchaseOrderReportData;
    if (
      query.bucket === "day" &&
      query.from === "2026-11-01" &&
      query.to === "2026-12-01"
    ) {
      report = purchaseOrderReportMonthFixture();
    } else if (
      query.bucket === "month" &&
      query.from === "2026-01-01" &&
      query.to === "2027-01-01"
    ) {
      report = purchaseOrderReportYearFixture();
    } else {
      report = reportFor(query);
    }
    report.filter = { ...query };
    return report;
  }

  function usePeriodReport(): void {
    reportMock.mockImplementation(async (query: Query) => respond(query));
  }

  function summary(): HTMLElement | null {
    return document.querySelector<HTMLElement>(
      "[data-report-projection-summary]"
    );
  }

  it("starts in Year mode and keeps the stacked remainder out of the KPI totals", async () => {
    usePeriodReport();
    await renderPage();

    expect(calls()[0]).toEqual(
      toReportQuery(initialReportFilters(PINNED_TODAY, "IDR"))
    );
    expect(calls()[0].bucket).toBe("month");
    expect(calls()[0].from).toBe("2026-01-01");
    expect(calls()[0].to).toBe("2027-01-01");

    const kpi = document.querySelector('[aria-label="Report KPIs"]');
    expect(
      document.querySelector("[data-report-period-context]")?.textContent
    ).toBe("Current year to date");
    expect(kpi?.textContent).toContain("Rp641.200.000");
    // The ghost December base is Rp50m and the November remainder is Rp110m;
    // neither may appear in the actual total.
    expect(kpi?.textContent).not.toContain("Rp691.200.000");
    expect(kpi?.textContent).not.toContain("Rp751.200.000");

    expect(summary()?.textContent).toContain("Rest of this month");
    expect(summary()?.textContent).toContain("Per remaining month");
    expect(
      document.querySelector("[data-chart-remainder]")
    ).not.toBeNull();
    expect(
      document.querySelectorAll('[data-chart-bucket][data-projected="true"]')
    ).toHaveLength(1);
    expect(document.querySelectorAll("[data-chart-band]")).toHaveLength(1);
  });

  it("drives the whole report from Month to Year and back", async () => {
    usePeriodReport();
    await renderPage();

    await act(async () => {
      button("Month view").click();
    });
    await flush();

    expect(calls().at(-1)).toEqual(
      toReportQuery({
        preset: "custom",
        period: "month",
        ...reportPeriodRange("month", PINNED_TODAY),
        bucket: "day",
        category: "all",
        currency: "IDR",
      })
    );
    expect(button("Month view").getAttribute("aria-pressed")).toBe("true");
    expect(button("Year view").getAttribute("aria-pressed")).toBe("false");

    const kpi = document.querySelector('[aria-label="Report KPIs"]');
    expect(
      document.querySelector("[data-report-period-context]")?.textContent
    ).toBe("Current month to date");
    expect(kpi?.textContent).toContain("Rp190.000.000");
    const periodRows = document.querySelectorAll(
      'table[aria-label="Period totals"] tbody tr'
    );
    expect(periodRows).toHaveLength(6);
    expect(document.body.textContent).toContain("Actual to date");
    expect(document.querySelector("[data-chart-remainder]")).toBeNull();
    expect(
      document.querySelectorAll('[data-chart-bucket][data-projected="true"]')
    ).toHaveLength(11);
    expect(summary()?.textContent).toContain("Month view, daily buckets");
    expect(summary()?.textContent).toContain("Per remaining day");
    expect(summary()?.textContent).toContain("19 elapsed days");
    expect(summary()?.textContent).toContain("12 POs observed");
    expect(summary()?.textContent).toContain("Updates as data arrives");

    await act(async () => {
      button("Year view").click();
    });
    await flush();

    expect(calls().at(-1)).toEqual(
      toReportQuery({
        preset: "custom",
        period: "year",
        ...reportPeriodRange("year", PINNED_TODAY),
        bucket: "month",
        category: "all",
        currency: "IDR",
      })
    );
    expect(button("Year view").getAttribute("aria-pressed")).toBe("true");
    expect(
      document.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).toContain("Rp641.200.000");
    expect(document.querySelector("[data-chart-remainder]")).not.toBeNull();
    expect(
      document.querySelectorAll(
        'table[aria-label="Period totals"] tbody tr'
      )
    ).toHaveLength(6);
  });

  it("clears the toggle and draws no ghost for a historical preset", async () => {
    usePeriodReport();
    await renderPage();

    await choose("Date range preset", "Last year");

    expect(
      calls().find(
        (call) =>
          call.from === "2025-01-01" &&
          call.to === "2026-01-01" &&
          call.bucket === "month"
      )
    ).toBeDefined();
    // The selected current month is still 2026-11, and the initial Year view
    // already loaded that exact monthly snapshot. No new annual read is needed
    // when Performance becomes historical.
    expect(
      calls()
        .slice(1)
        .filter(
          (call) =>
            call.from === "2026-01-01" &&
            call.to === "2027-01-01" &&
            call.bucket === "month"
        )
    ).toHaveLength(0);
    expect(button("Month view").getAttribute("aria-pressed")).toBe("false");
    expect(button("Year view").getAttribute("aria-pressed")).toBe("false");
    expect(summary()?.getAttribute("data-projection-mode")).toBe("none");
    expect(summary()?.textContent).toContain(
      "not the current month or year"
    );
    expect(document.querySelector("[data-chart-remainder]")).toBeNull();
    expect(
      document.querySelectorAll('[data-chart-bucket][data-projected="true"]')
    ).toHaveLength(0);
    expect(document.querySelector("[data-chart-divider]")).toBeNull();
  });

  it("states the reason and draws no range below the projection threshold", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = purchaseOrderReportMonthFixture();
      report.filter = { ...query };
      report.currentProjection.status = "unavailable";
      report.currentProjection.reason = "insufficient_days";
      report.currentProjection.buckets = [];
      return report;
    });
    await renderPage();

    await act(async () => {
      button("Month view").click();
    });
    await flush();

    expect(summary()?.getAttribute("data-projection-status")).toBe(
      "unavailable"
    );
    expect(summary()?.getAttribute("data-projection-reason")).toBe(
      "insufficient_days"
    );
    expect(summary()?.textContent).toContain("Not enough elapsed days");
    expect(summary()?.textContent).toContain("The chart draws actual periods only");
    expect(summary()?.textContent).not.toContain("Per remaining day");
    expect(
      document.querySelectorAll('[data-chart-bucket][data-projected="true"]')
    ).toHaveLength(0);
    expect(document.querySelector("[data-chart-band]")).toBeNull();
    expect(
      document.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).toContain("Rp190.000.000");
  });

  it("exports the same currentProjection the screen is showing", async () => {
    usePeriodReport();
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/report.pdf"
    );
    await renderPage({ onExport });

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    const yearView = onExport.mock.calls[0][0] as PurchaseOrderReportView;
    expect(yearView.version).toBe(1);
    expect(yearView.sources).toHaveLength(1);
    expect(yearView.sources[0].currentProjection.mode).toBe("month");
    expect(yearView.sources[0].currentProjection.status).toBe("available");
    expect(yearView.sources[0].currentProjection.buckets).toHaveLength(1);
    expect(
      yearView.sources[0].currentProjection.currentMonth?.remainder.base
    ).toBe(110_000_000);
    expect(yearView.periods.months).toEqual(["2026-11"]);
    expect("currentMonthSource" in yearView.periods).toBe(false);

    await act(async () => {
      button("Month view").click();
    });
    await flush();
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    const monthView = onExport.mock.calls[1][0] as PurchaseOrderReportView;
    expect(monthView.sources[0].currentProjection.mode).toBe("day");
    expect(monthView.sources[0].currentProjection.buckets).toHaveLength(11);
    expect(monthView.sources[0].filter).toEqual(calls().at(-1));
    expect(monthView.periods.currentMonthSource).toBe(0);
    expect(monthView.periods.sources).toEqual([0]);
  });
});

describe("Reports export seam (issue #630)", () => {
  it("disables Export PDF when no callback is wired", async () => {
    await renderPage();
    expect(button("Export PDF").disabled).toBe(true);
  });

  it("sends the visible normalized snapshot to the callback", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/po-report.pdf"
    );
    await renderPage({ onExport });

    expect(button("Export PDF").disabled).toBe(false);
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(onExport).toHaveBeenCalledTimes(1);
    const sent = onExport.mock.calls[0][0] as PurchaseOrderReportView;
    expect(sent.version).toBe(1);
    expect(sent.overview).toBe(0);
    expect(sent.sources[0].filter).toEqual(calls()[0]);
    expect(sent.sources[0].totals).toEqual(purchaseOrderReportFixture().totals);
    expect(document.body.textContent).toContain(
      "Report saved to /tmp/opencode/po-report.pdf"
    );
  });

  it("treats an empty export result as cancelled, with no success toast", async () => {
    const onExport = vi.fn(async () => "");
    await renderPage({ onExport });

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(onExport).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toContain("Report saved");
    expect(document.body.textContent).not.toContain("could not be saved");
  });

  it("reports an export failure actionably and re-enables the button", async () => {
    const onExport = vi.fn(async () => {
      throw new Error("The save dialog was refused.");
    });
    await renderPage({ onExport });

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(document.body.textContent).toContain("The save dialog was refused.");
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("disables export while the visible snapshot is stale", async () => {
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(async (query: Query) => reportFor(query));
    reportMock.mockImplementationOnce(() => pending.promise);
    await renderPage({ onExport: async () => "/tmp/opencode/x.pdf" });

    expect(button("Export PDF").disabled).toBe(false);
    await choose("Bucket", "Year");
    expect(button("Export PDF").disabled).toBe(true);

    act(() => {
      pending.resolve(reportFor({ ...calls()[1] }));
    });
    await flush();
    expect(button("Export PDF").disabled).toBe(false);
  });
});

function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
} {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("independent month browser (issue #641)", () => {
  const PINNED_TODAY = "2026-11-19";

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(`${PINNED_TODAY}T03:00:00Z`));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  function isCurrentDayQuery(query: Query): boolean {
    return (
      query.bucket === "day" &&
      query.from === "2026-11-01" &&
      query.to === "2026-12-01"
    );
  }

  function isAnnual2026Query(query: Query): boolean {
    return (
      query.bucket === "month" &&
      query.from === "2026-01-01" &&
      query.to === "2027-01-01"
    );
  }

  function respond(query: Query): PurchaseOrderReportData {
    let report: PurchaseOrderReportData;
    if (isCurrentDayQuery(query)) {
      report = purchaseOrderReportMonthFixture();
    } else if (isAnnual2026Query(query)) {
      report = purchaseOrderReportYearFixture();
    } else if (
      query.bucket === "month" &&
      query.from === "2027-01-01" &&
      query.to === "2028-01-01"
    ) {
      report = purchaseOrderReportYearFixture();
      report.filter = { ...query };
      report.periods = report.periods.map((period, index) => ({
        ...period,
        periodStart: `2027-${String(index + 1).padStart(2, "0")}-01`,
        ...{
          poCount: 0,
          amountMinor: 0,
          costedMinor: 0,
          uncostedMinor: 0,
          capitalMinor: 0,
          marginMinor: 0,
          marginBps: null,
          coverageBps: null,
        },
      }));
      report.currentProjection = {
        ...report.currentProjection,
        mode: "none",
        status: "unavailable",
        reason: "not_current_period",
        buckets: [],
        currentMonth: undefined,
      };
      report.monthSummary = undefined;
      return report;
    } else {
      report = purchaseOrderReportFixture();
      report.filter = { ...query };
      const month = report.asOf.slice(0, 7);
      const period = report.periods.find(
        (row) => row.periodStart === `${month}-01`
      );
      if (period !== undefined) {
        report.monthSummary = {
          month,
          from: `${month}-01`,
          to: nextMonthStart(month),
          asOf: report.asOf,
          mode: "month",
          actual: {
            poCount: period.poCount,
            amountMinor: period.amountMinor,
            costedMinor: period.costedMinor,
            uncostedMinor: period.uncostedMinor,
            capitalMinor: period.capitalMinor,
            marginMinor: period.marginMinor,
            marginBps: period.marginBps,
            coverageBps: period.coverageBps,
          },
          projection: {
            status: "unavailable",
            reason: "insufficient_months",
            method: "No current-year forecast in this fixture.",
          },
        };
      }
    }
    report.filter = { ...query };
    return report;
  }

  function zeroYearReport(): PurchaseOrderReportData {
    const report = purchaseOrderReportYearFixture();
    const zero = {
      poCount: 0,
      amountMinor: 0,
      costedMinor: 0,
      uncostedMinor: 0,
      capitalMinor: 0,
      marginMinor: 0,
      marginBps: null,
      coverageBps: null,
    } as const;
    report.totals = { ...zero };
    report.periods = report.periods.map((period) => ({ ...period, ...zero }));
    report.categories = report.categories.map((category) => ({
      ...category,
      ...zero,
    }));
    report.currentProjection = {
      ...report.currentProjection,
      status: "unavailable",
      reason: "no_data",
      buckets: [],
      currentMonth: undefined,
    };
    report.monthSummary = {
      ...report.monthSummary!,
      actual: { ...zero },
      projection: {
        status: "unavailable",
        reason: "no_data",
        method: report.monthSummary!.projection.method,
      },
    };
    return report;
  }

  function monthBrowser(): HTMLElement {
    const found = document.querySelector<HTMLElement>("[data-month-browser]");
    if (found === null) throw new Error("no month browser");
    return found;
  }

  function chartMarks(): Element | null {
    return document.querySelector("[data-chart-marks]");
  }

  it("seeds the month from the server's WIB as-of, not the client clock", async () => {
    vi.setSystemTime(new Date("2026-12-02T03:00:00Z"));
    reportMock.mockImplementation(async (query: Query) => respond(query));

    await renderPage();

    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(monthBrowser().textContent).toContain("November 2026");
    expect(monthBrowser().textContent).not.toContain("December 2026");
  });

  it("opens the default Year view with one query and browses months without another", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();

    expect(reportMock).toHaveBeenCalledTimes(1);
    const marks = chartMarks();
    const kpiBefore = document.querySelector('[aria-label="Report KPIs"]')
      ?.textContent;
    expect(monthBrowser().textContent).toContain("November 2026");
    expect(monthBrowser().textContent).toContain("Actual to date");

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(monthBrowser().textContent).toContain("October 2026");
    expect(monthBrowser().textContent).toContain("Recorded");
    expect(document.querySelector('[aria-label="Report KPIs"]')?.textContent).toBe(
      kpiBefore
    );
    expect(chartMarks()).toBe(marks);

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(monthBrowser().textContent).toContain("November 2026");

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(monthBrowser().textContent).toContain("December 2026");
    const estimated = monthBrowser().querySelector(
      '[data-month-evidence="estimated"]'
    );
    expect(estimated?.textContent).toContain("Estimated forecast (not actual)");
    expect(estimated?.textContent).toContain("Rp50.000.000");
    expect(estimated?.getAttribute("aria-label")).toContain(
      "Estimates are not actual evidence"
    );
    expect(chartMarks()).toBe(marks);
  });

  it("preserves an explicit month across a Performance filter change", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    expect(monthBrowser().textContent).toContain("October 2026");

    await choose("Category", "Unassigned");
    await flush();

    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1].category).toBe("UNASSIGNED");
    expect(monthBrowser().textContent).toContain("October 2026");
    expect(monthBrowser().textContent).not.toContain("November 2026");
  });

  it("does not treat a This year partial preset as the annual monthly source", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    const initial = calls()[0];
    expect(initial).toEqual(
      toReportQuery(initialReportFilters(PINNED_TODAY, "IDR"))
    );

    await choose("Date range preset", "This year");

    const partial = toReportQuery({
      ...initialReportFilters(PINNED_TODAY, "IDR"),
      preset: "this-year",
      period: null,
      ...reportPresetRange("this-year", PINNED_TODAY),
    });
    expect(calls()).toEqual([initial, partial]);
    expect(partial.bucket).toBe("month");
    expect(partial.to).not.toBe(initial.to);
    // The partial preset did not become an annual source; the browser reuses
    // the exact annual snapshot from the initial Year view.
    expect(monthBrowser().textContent).toContain("Monthly snapshot");
    expect(monthBrowser().textContent).toContain("November 2026");
  });

  it("reuses the cached annual snapshot for a day view's previous month", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await act(async () => {
      button("Month view").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    const marks = chartMarks();

    await act(async () => {
      button("Previous month").click();
    });
    await flush();

    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(monthBrowser().textContent).toContain("October 2026");
    expect(monthBrowser().textContent).toContain("Monthly snapshot");
    expect(chartMarks()).toBe(marks);

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(monthBrowser().textContent).toContain("November 2026");
    expect(monthBrowser().textContent).toContain("Rp190.000.000");
    expect(chartMarks()).toBe(marks);

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(monthBrowser().textContent).toContain("December 2026");
    expect(
      monthBrowser().querySelector('[data-month-evidence="estimated"]')
        ?.textContent
    ).toContain("Rp50.000.000");
    expect(chartMarks()).toBe(marks);
  });

  it("fetches a monthly source when the year overview has no cached match", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    const initial = calls()[0];

    await choose("Bucket", "Year");
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1].bucket).toBe("year");

    // A year-bucketed overview with a new category is not a monthly source. The
    // browser must load the exact annual monthly snapshot for that category.
    await choose("Category", "Unassigned");
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(4);
    expect(calls()[2]).toMatchObject({
      from: initial.from,
      to: initial.to,
      bucket: "year",
      category: "UNASSIGNED",
    });
    expect(calls()[3]).toEqual({
      ...initial,
      category: "UNASSIGNED",
      bucket: "month",
    });
    expect(monthBrowser().textContent).toContain("Monthly snapshot");
    expect(monthBrowser().textContent).toContain("November 2026");
  });

  it("loads a new year once and reuses it when the selection returns", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    expect(reportMock).toHaveBeenCalledTimes(1);
    const marks = chartMarks();

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(monthBrowser().textContent).toContain("December 2026");
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1]).toMatchObject({
      from: "2027-01-01",
      to: "2028-01-01",
      bucket: "month",
    });
    expect(monthBrowser().textContent).toContain("January 2027");
    expect(monthBrowser().textContent).toContain("beyond the current year");
    expect(
      monthBrowser().querySelector("[data-month-scope]")?.textContent
    ).toContain("All categories");
    expect(
      monthBrowser().querySelector("[data-month-scope]")?.textContent
    ).toContain("IDR");
    expect(chartMarks()).toBe(marks);
    expect(chartMarks()?.getAttribute("data-chart-motion")).toBe(
      marks?.getAttribute("data-chart-motion")
    );

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(monthBrowser().textContent).toContain("December 2026");

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(monthBrowser().textContent).toContain("January 2027");
    expect(chartMarks()).toBe(marks);
  });

  it("deduplicates an in-flight annual read when the selection returns", async () => {
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await act(async () => {
      button("Month view").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    reportMock.mockImplementationOnce(() => pending.promise);

    // December 2026 is covered by the cached year snapshot; January 2027 is a
    // new year and starts the one in-flight annual read.
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(3);
    expect(monthBrowser().textContent).toContain("Loading January 2027");

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(3);

    await act(async () => {
      pending.resolve(
        respond({
          from: "2027-01-01",
          to: "2028-01-01",
          bucket: "month",
          category: "all",
          currency: "IDR",
        })
      );
    });
    await flush();
    expect(monthBrowser().textContent).toContain("January 2027");
    expect(monthBrowser().textContent).toContain("beyond the current year");
  });

  it("prints a maximum exact month amount without rounding or clipping", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = purchaseOrderReportMaxFixture();
      report.filter = { ...query };
      report.monthSummary = {
        month: "2026-11",
        from: "2026-11-01",
        to: "2026-12-01",
        asOf: report.asOf,
        mode: "month",
        actual: report.totals,
        projection: {
          status: "unavailable",
          reason: "insufficient_months",
          method: "No current-year forecast in this fixture.",
        },
      };
      return report;
    });
    await renderPage();

    expect(
      monthBrowser().querySelector('[data-month-metric="PO amount"]')
        ?.textContent
    ).toBe("Rp9.007.199.254.740.991");
    expect(
      monthBrowser().querySelector('[data-month-metric="POs"]')?.textContent
    ).toBe("9,007,199,254,740,991");
  });

  it("keeps a negative month margin signed and danger-toned", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = purchaseOrderReportMonthFixture();
      report.filter = { ...query };
      report.monthSummary!.actual.marginMinor = -1_000_000;
      report.monthSummary!.actual.marginBps = -10000;
      return report;
    });
    await renderPage();

    const margin = monthBrowser().querySelector(
      '[data-month-metric="PO margin"]'
    );
    expect(margin?.textContent).toContain("Rp-1.000.000");
    expect(margin?.className).toContain("text-danger-700");
    expect(
      monthBrowser().querySelector('[data-month-metric="PO margin %"]')
        ?.textContent
    ).toBe("-100.00%");
  });

  it("never paints old money under a loading or failed new month", async () => {
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage({ onExport: async () => "/tmp/opencode/loading.pdf" });
    await act(async () => {
      button("Month view").click();
    });
    await flush();
    expect(monthBrowser().textContent).toContain("Rp190.000.000");
    reportMock.mockImplementationOnce(() => pending.promise);

    // Move through the cached December into an uncached January 2027, whose
    // annual source is still in flight.
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    await act(async () => {
      button("Next month").click();
    });
    await flush();

    expect(monthBrowser().textContent).toContain("January 2027");
    expect(monthBrowser().textContent).toContain("Loading January 2027");
    expect(monthBrowser().textContent).not.toContain("Rp190.000.000");
    expect(button("Export PDF").disabled).toBe(true);

    await act(async () => {
      pending.resolve(
        respond({
          from: "2027-01-01",
          to: "2028-01-01",
          bucket: "month",
          category: "all",
          currency: "IDR",
        })
      );
    });
    await flush();
    expect(monthBrowser().textContent).toContain("beyond the current year");
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("reports a failed selected source without old money and blocks export", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await act(async () => {
      button("Month view").click();
    });
    await flush();
    reportMock.mockRejectedValueOnce(
      new Error("The monthly API is unreachable.")
    );

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    await act(async () => {
      button("Next month").click();
    });
    await flush();

    expect(monthBrowser().textContent).toContain("January 2027");
    expect(monthBrowser().textContent).toContain(
      "The monthly API is unreachable."
    );
    expect(monthBrowser().textContent).not.toContain("Rp190.000.000");
    expect(button("Export PDF").disabled).toBe(true);
    expect(monthBrowser().textContent).toContain("Retry");

    reportMock.mockImplementationOnce(async (query: Query) => {
      const report = respond({
        from: "2027-01-01",
        to: "2028-01-01",
        bucket: "month",
        category: "all",
        currency: "IDR",
      });
      report.filter = { ...query };
      return report;
    });
    await act(async () => {
      button("Retry month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(4);
    expect(monthBrowser().textContent).not.toContain(
      "The monthly API is unreachable."
    );
    expect(monthBrowser().textContent).toContain("beyond the current year");
  });

  it("resolves the exact-value anchor to the ready selected-month table", async () => {
    await renderPage();

    expect(monthBrowser().textContent).toContain("November 2026");
    expect(monthBrowser().textContent).toContain("Actual to date");
    const target = assertExactValuesTarget("ready", monthBrowser());
    expect(target.tagName).toBe("TABLE");
    expect(target.getAttribute("aria-label")).toBe("Period totals");
  });

  it("keeps the exact-value anchor target present while the selected month loads", async () => {
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();

    // December 2026 is already in the cached annual source; January 2027 is a
    // new year whose annual read stays in flight.
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    reportMock.mockImplementationOnce(() => pending.promise);
    await act(async () => {
      button("Next month").click();
    });
    await flush();

    expect(monthBrowser().textContent).toContain("Loading January 2027");
    expect(monthBrowser().textContent).toContain("January 2027");
    assertExactValuesTarget("loading", monthBrowser());
    // The description no longer promises a ready table while the section is
    // still fetching.
    expect(exactValuesDescription()).toContain(
      "the selected month section after this chart lists every exact value once it loads"
    );
    expect(exactValuesDescription()).not.toContain("selected month table");
  });

  it("keeps the exact-value anchor target present when the selected month fails", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    reportMock.mockRejectedValueOnce(
      new Error("The monthly API is unreachable.")
    );
    await act(async () => {
      button("Next month").click();
    });
    await flush();

    expect(monthBrowser().textContent).toContain(
      "The monthly API is unreachable."
    );
    expect(monthBrowser().textContent).toContain("Retry");
    assertExactValuesTarget("error", monthBrowser());
    expect(exactValuesDescription()).toContain(
      "the selected month section after this chart lists every exact value once it loads"
    );
    expect(exactValuesDescription()).not.toContain("selected month table");
  });

  it("points a direct view caller's anchor at a table that exists without a month readout", async () => {
    const filters = initialReportFilters(PINNED_TODAY, "IDR");
    const report = purchaseOrderReportFixture();
    report.filter = toReportQuery(filters);
    const noop = () => undefined;
    const view = (monthReadout: undefined | null) => (
      <PurchaseOrderReportsView
        filters={filters}
        report={report}
        loading={false}
        stale={false}
        error=""
        validation=""
        catalogue={CATALOGUE}
        exporting={false}
        canExport={false}
        onPresetChange={noop}
        onPeriodChange={noop}
        onFromChange={noop}
        onToChange={noop}
        onBucketChange={noop}
        onCategoryChange={noop}
        onCurrencyChange={noop}
        onRetry={noop}
        onExport={noop}
        monthReadout={monthReadout}
      />
    );

    await act(async () => {
      root.render(view(undefined));
    });
    await flush();

    // No independent month browser in this legacy shape: the embedded anchor
    // falls back to the legacy period table rather than dangling.
    expect(document.querySelector("[data-month-browser]")).toBeNull();
    const target = assertExactValuesTarget("no month readout", null);
    expect(target.tagName).toBe("TABLE");
    expect(target.getAttribute("aria-label")).toBe("Period totals");

    // An explicit null readout takes the same legacy path instead of dropping
    // the table and leaving the anchor with nothing to name.
    await act(async () => {
      root.render(view(null));
    });
    await flush();
    expect(document.querySelector("[data-month-browser]")).toBeNull();
    expect(assertExactValuesTarget("null month readout", null)).toBe(target);
  });

  it("keeps the zero month browser visible under an empty overview", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = zeroYearReport();
      report.filter = { ...query };
      return report;
    });
    await renderPage({ onExport: async () => "/tmp/opencode/zero.pdf" });

    expect(document.body.textContent).toContain("No purchase orders");
    expect(document.querySelector('[aria-label="Report KPIs"]')).toBeNull();
    expect(document.querySelector("[data-chart-marks]")).toBeNull();
    expect(
      document.querySelectorAll("[data-report-period-toggle]")
    ).toHaveLength(1);
    expect(monthBrowser().textContent).toContain("November 2026");
    expect(monthBrowser().textContent).toContain("Actual to date");
    expect(
      monthBrowser().querySelector('[data-month-metric="POs"]')?.textContent
    ).toBe("0");
    expect(
      monthBrowser().querySelector('[data-month-metric="PO margin %"]')
        ?.textContent
    ).toBe("-");
    expect(button("Export PDF").disabled).toBe(false);

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    expect(monthBrowser().textContent).toContain("October 2026");
    expect(monthBrowser().textContent).toContain("Recorded");
    expect(
      monthBrowser().querySelector('[data-month-metric="PO amount"]')
        ?.textContent
    ).toContain("Rp0");
  });

  it("blocks export and labels the section when the annual as-of differs", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = respond(query);
      if (query.from === "2027-01-01" && query.to === "2028-01-01") {
        report.asOf = "2026-11-20";
      }
      report.filter = { ...query };
      return report;
    });
    await renderPage();
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    await act(async () => {
      button("Next month").click();
    });
    await flush();

    expect(monthBrowser().textContent).toContain("January 2027");
    expect(monthBrowser().textContent).toContain(
      "different business date from the overview"
    );
    expect(monthBrowser().textContent).toContain("As of 20 Nov 2026");
    expect(button("Export PDF").disabled).toBe(true);
  });

  it("exports the frozen selected month with no extra request", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/month.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage({ onExport });
    await act(async () => {
      button("Month view").click();
    });
    await flush();

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(reportMock).toHaveBeenCalledTimes(2);
    const view = onExport.mock.calls[0][0] as PurchaseOrderReportView;
    expect(view.version).toBe(1);
    expect(view.sources).toHaveLength(2);
    expect(view.sources[0].filter.bucket).toBe("day");
    expect(view.sources[1].filter).toEqual(calls()[0]);
    expect(view.periods.months).toEqual(["2026-10"]);
    expect(view.periods.sources).toEqual([1]);
    expect("currentMonthSource" in view.periods).toBe(false);

    // A later selection cannot alter the envelope already handed over.
    const frozen = JSON.stringify(view);
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(JSON.stringify(view)).toBe(frozen);
  });

  it("shows recorded future evidence separately from the estimated remainder", async () => {
    const annualQuery = toReportQuery(initialReportFilters(PINNED_TODAY, "IDR"));
    const dayQuery = toReportQuery({
      ...initialReportFilters(PINNED_TODAY, "IDR"),
      preset: "custom",
      period: "month",
      ...reportPeriodRange("month", PINNED_TODAY),
      bucket: "day",
    });
    const sameQuery = (left: Query, right: Query): boolean =>
      left.from === right.from &&
      left.to === right.to &&
      left.bucket === right.bucket &&
      left.category === right.category &&
      left.currency === right.currency;
    const mocked: {
      query: Query;
      mode: "year" | "day";
      report: PurchaseOrderReportData;
    }[] = [];

    reportMock.mockImplementation(async (query: Query) => {
      // The mocked source must match the requested scope. A default
      // Year/monthly request is answered by the coherent year fixture; only an
      // actual Month/day request may receive the day fixture. Any other query
      // is an unsupported relabelling and must fail loudly.
      if (sameQuery(query, annualQuery)) {
        const report = purchaseOrderReportYearWithRecordedFutureFixture();
        report.filter = { ...query };
        mocked.push({ query, mode: "year", report });
        return report;
      }
      if (sameQuery(query, dayQuery)) {
        const report = purchaseOrderReportMonthWithRecordedFutureFixture();
        report.filter = { ...query };
        mocked.push({ query, mode: "day", report });
        return report;
      }
      throw new Error(`unexpected source query ${JSON.stringify(query)}`);
    });

    await renderPage();
    expect(mocked.map(({ query, mode }) => ({ query, mode }))).toEqual([
      { query: annualQuery, mode: "year" },
    ]);
    expect(mocked[0].report.filter).toEqual(annualQuery);
    expect(mocked[0].report.currentProjection.mode).toBe("month");
    expect(
      mocked[0].report.periods.every((period) => period.periodStart.endsWith("-01"))
    ).toBe(true);

    await act(async () => {
      button("Month view").click();
    });
    await flush();

    expect(calls()).toEqual([annualQuery, dayQuery]);
    expect(mocked.map(({ query, mode }) => ({ query, mode }))).toEqual([
      { query: annualQuery, mode: "year" },
      { query: dayQuery, mode: "day" },
    ]);
    expect(mocked[1].report.filter).toEqual(dayQuery);
    expect(mocked[1].report.currentProjection.mode).toBe("day");
    expect(mocked[1].report.totals.amountMinor).toBe(190_777_000);
    expect(
      mocked[1].report.periods.find(
        (period) => period.periodStart === "2026-11-25"
      )?.amountMinor
    ).toBe(777_000);

    const recorded = monthBrowser().querySelector(
      '[data-month-evidence="recorded-future"]'
    );
    const estimated = monthBrowser().querySelector(
      '[data-month-evidence="estimated"]'
    );
    expect(recorded?.textContent).toContain("Rp777.000");
    expect(recorded?.textContent).toContain("Rp600.000");
    expect(estimated?.textContent).toContain("Estimated remainder (not actual)");
    expect(estimated?.textContent).toContain("Rp110.000.000");
    expect(estimated?.getAttribute("data-estimated")).toBe("true");
    expect(
      monthBrowser().querySelector("table")?.getAttribute("data-estimated")
    ).toBeNull();
    // The future order is in the raw totals/period/category rows (coherent
    // day fixture), but the month actual remains month-to-date and the
    // estimate stays a separate alternative. No cell renders recorded+forecast.
    expect(
      document.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).toContain("Rp190.777.000");
    expect(
      monthBrowser().querySelector('[data-month-metric="PO amount"]')
        ?.textContent
    ).toBe("Rp190.000.000");
    expect(monthBrowser().textContent).not.toContain("Rp110.777.000");
  });
});

describe("month comparison (issue #642)", () => {
  const PINNED_TODAY = "2026-11-19";
  const ZERO = {
    poCount: 0,
    amountMinor: 0,
    costedMinor: 0,
    uncostedMinor: 0,
    capitalMinor: 0,
    marginMinor: 0,
    marginBps: null,
    coverageBps: null,
  } as const;

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(`${PINNED_TODAY}T03:00:00Z`));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  function annualQueryFor(query: Query): number | null {
    if (query.bucket !== "month") return null;
    const match = /^(\d{4})-01-01$/.exec(query.from);
    if (match === null) return null;
    const year = Number(match[1]);
    return query.to === `${year + 1}-01-01` ? year : null;
  }

  function zeroAnnual(year: number, query?: Query): PurchaseOrderReportData {
    const source = purchaseOrderReportYearFixture();
    source.filter = {
      from: `${year}-01-01`,
      to: `${year + 1}-01-01`,
      bucket: "month",
      category: query?.category ?? "all",
      currency: query?.currency ?? "IDR",
    };
    source.periods = Array.from({ length: 12 }, (_, index) => ({
      periodStart: `${year}-${String(index + 1).padStart(2, "0")}-01`,
      partial: false,
      inProgress: false,
      ...ZERO,
    }));
    source.totals = { ...ZERO };
    source.categories = source.categories.map((category) => ({
      ...category,
      ...ZERO,
    }));
    source.currentProjection = {
      ...source.currentProjection,
      mode: "none",
      status: "unavailable",
      reason: "not_current_period",
      buckets: [],
      currentMonth: undefined,
    };
    source.monthSummary = undefined;
    return source;
  }

  function respond(query: Query): PurchaseOrderReportData {
    if (
      query.bucket === "day" &&
      query.from === "2026-11-01" &&
      query.to === "2026-12-01"
    ) {
      const report = purchaseOrderReportMonthFixture();
      report.filter = { ...query };
      return report;
    }
    const annualYear = annualQueryFor(query);
    if (annualYear === 2026) {
      const report = purchaseOrderReportYearFixture();
      report.filter = { ...query };
      return report;
    }
    if (annualYear !== null) {
      return zeroAnnual(annualYear, query);
    }
    const report = purchaseOrderReportFixture();
    report.filter = { ...query };
    return report;
  }

  function pickerDialog(): HTMLElement | null {
    return document.querySelector<HTMLElement>("[data-report-picker]");
  }

  function pickerItem(month: string): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>(
      `[data-picker-key="${month}"]`
    );
    if (found === null) throw new Error(`no picker item ${month}`);
    return found;
  }

  function pickerAll(): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>("[data-picker-all]");
    if (found === null) throw new Error("no All months control");
    return found;
  }

  function pickerClose(): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>("[data-picker-close]");
    if (found === null) throw new Error("no picker close");
    return found;
  }

  function trigger(): HTMLButtonElement {
    return button("Choose months");
  }

  function cards(): HTMLElement[] {
    return Array.from(document.querySelectorAll<HTMLElement>("[data-month-card]"));
  }

  function card(month: string): HTMLElement {
    const found = document.querySelector<HTMLElement>(
      `[data-month-card][data-month-key="${month}"]`
    );
    if (found === null) throw new Error(`no month card ${month}`);
    return found;
  }

  function activeCard(): HTMLElement | null {
    return document.querySelector<HTMLElement>('[data-month-active="true"]');
  }

  function aggregate(): HTMLElement {
    const found = document.querySelector<HTMLElement>("[data-month-aggregate]");
    if (found === null) throw new Error("no aggregate");
    return found;
  }

  async function openPicker(): Promise<void> {
    await act(async () => {
      trigger().click();
    });
    await flush();
  }

  async function clickPicker(control: HTMLButtonElement): Promise<void> {
    await act(async () => {
      control.click();
    });
    await flush();
  }

  async function previousPage(times = 1): Promise<void> {
    for (let index = 0; index < times; index += 1) {
      await clickPicker(button("Previous page"));
    }
  }

  async function nextPage(times = 1): Promise<void> {
    for (let index = 0; index < times; index += 1) {
      await clickPicker(button("Next page"));
    }
  }

  function chartMarks(): Element | null {
    return document.querySelector("[data-chart-marks]");
  }

  it("opens on the selected year and commits while the picker stays open", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(trigger().getAttribute("data-month-selection-summary")).toBe(
      "November 2026"
    );

    await openPicker();
    expect(pickerDialog()).not.toBeNull();
    expect(pickerItem("2026-11").getAttribute("aria-pressed")).toBe("true");
    expect(pickerItem("2026-10").getAttribute("aria-pressed")).toBe("false");
    expect(pickerAll().getAttribute("aria-pressed")).toBe("false");
    expect(pickerAll().textContent).toContain("All months in 2026");

    await clickPicker(pickerItem("2026-10"));
    expect(pickerDialog()).not.toBeNull();
    expect(pickerItem("2026-10").getAttribute("aria-pressed")).toBe("true");
    expect(pickerItem("2026-11").getAttribute("aria-pressed")).toBe("true");
    expect(document.querySelector("[data-picker-count]")?.textContent).toContain(
      "2 selected"
    );
    expect(cards()).toHaveLength(2);
    expect(cards().map((node) => node.getAttribute("data-month-key"))).toEqual([
      "2026-10",
      "2026-11",
    ]);
    expect(reportMock).toHaveBeenCalledTimes(1);
  });

  it("does not dismiss on an item, the backdrop or Escape; the X closes and keeps the selection", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await openPicker();

    await clickPicker(pickerItem("2026-10"));
    expect(pickerDialog()).not.toBeNull();

    const backdrop = document.querySelector<HTMLElement>(
      "[data-report-picker-backdrop]"
    );
    if (backdrop === null) throw new Error("no backdrop");
    await act(async () => {
      backdrop.click();
    });
    await flush();
    expect(pickerDialog()).not.toBeNull();

    await act(async () => {
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", bubbles: true })
      );
    });
    await flush();
    expect(pickerDialog()).not.toBeNull();

    await clickPicker(pickerClose());
    expect(pickerDialog()).toBeNull();
    expect(trigger().getAttribute("data-month-selection-summary")).toBe(
      "2 months · 2026"
    );
    expect(cards()).toHaveLength(2);
    expect(cards()[0].getAttribute("data-month-key")).toBe("2026-10");
  });

  it("returns focus to the same trigger node after its text changes", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    const opener = trigger();
    await openPicker();
    await clickPicker(pickerAll());
    expect(trigger().getAttribute("data-month-selection-summary")).toBe(
      "12 months · 2026"
    );
    await clickPicker(pickerClose());
    expect(document.querySelector("[data-report-picker]")).toBeNull();
    expect(trigger()).toBe(opener);
    expect(document.activeElement).toBe(opener);
  });

  it("pages the picker year without changing selection, membership or requests", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await openPicker();
    const openerSummary = trigger().getAttribute("data-month-selection-summary");

    await nextPage();
    expect(pickerItem("2027-01")).not.toBeNull();
    expect(document.querySelector('[data-picker-key="2026-01"]')).toBeNull();
    expect(pickerAll().textContent).toContain("All months in 2027");
    expect(trigger().getAttribute("data-month-selection-summary")).toBe(
      openerSummary
    );
    expect(document.querySelector("[data-picker-count]")?.textContent).toContain(
      "1 selected"
    );

    await nextPage();
    expect(pickerAll().textContent).toContain("All months in 2028");
    expect(reportMock).toHaveBeenCalledTimes(1);
    await clickPicker(pickerClose());

    // Reopening starts at the active/selected month's year, not the page last
    // browsed.
    await openPicker();
    expect(pickerItem("2026-11")).not.toBeNull();
    expect(document.querySelector('[data-picker-key="2028-01"]')).toBeNull();
  });

  it("All replaces the set with one year's twelve and Aggregate reads the server total", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = respond(query);
      if (annualQueryFor(query) === 2026) {
        // A total that deliberately differs from the row sum: the aggregate
        // must print the server total, never add the rows itself.
        report.totals = { ...report.totals, amountMinor: 999_000_000 };
      }
      return report;
    });
    await renderPage();
    const marks = chartMarks();
    const kpiBefore = document.querySelector(
      '[aria-label="Report KPIs"]'
    )?.textContent;
    const categoryTable = document.querySelector(
      '[aria-label="Category totals"]'
    );
    await openPicker();
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());

    expect(cards()).toHaveLength(0);
    const table = aggregate();
    expect(document.querySelector("#report-period-table")).not.toBeNull();
    expect(table.textContent).toContain("Recorded total");
    const total = table.querySelector(
      '[data-aggregate-total-metric="Amount"]'
    );
    expect(total?.textContent).toBe("Rp999.000.000");
    const rowSum = Array.from(
      table.querySelectorAll<HTMLElement>(
        '[data-aggregate-row] [data-aggregate-metric="Amount"]'
      )
    ).reduce(
      (sum, cell) =>
        sum +
        Number((cell.textContent ?? "").replace(/[^0-9-]/g, "")),
      0
    );
    expect(rowSum).not.toBe(999_000_000);
    expect(
      table.querySelectorAll("[data-aggregate-row]")
    ).toHaveLength(12);
    expect(table.textContent).toContain("not a sum of the rows");
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(chartMarks()).toBe(marks);
    expect(
      document.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).toBe(kpiBefore);
    expect(document.querySelector('[aria-label="Category totals"]')).toBe(
      categoryTable
    );

    // All months is exactly the year, and the picker marks it checked.
    await openPicker();
    expect(pickerAll().getAttribute("aria-pressed")).toBe("true");
    expect(document.querySelector("[data-picker-count]")?.textContent).toContain(
      "12 selected"
    );
  });

  it("keeps the daily current-month precedence when All is compared from Month view", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/day-all.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage({ onExport });
    await act(async () => {
      button("Month view").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);

    await openPicker();
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());
    // The initial annual overview is retained, so All adds no request.
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(aggregate().textContent).toContain("2026 monthly comparison");

    await act(async () => {
      button("Individual").click();
    });
    await flush();
    expect(cards()).toHaveLength(12);
    const current = card("2026-11");
    expect(current.textContent).toContain("Actual to date");
    expect(
      current.querySelector('[data-month-evidence="estimated"]')?.textContent
    ).toContain("Estimated remainder (not actual)");
    expect(
      current.querySelector("[data-month-scope]")?.textContent
    ).toContain("Performance snapshot");
    expect(
      card("2026-10").querySelector("[data-month-scope]")?.textContent
    ).toContain("Monthly snapshot");

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(onExport).toHaveBeenCalledTimes(1);
    const view = onExport.mock.calls[0][0];
    expect(view.periods.sources).toEqual([1, 0]);
    expect(view.periods.currentMonthSource).toBe(0);
    expect(view.sources[0].filter.bucket).toBe("day");
    expect(view.sources[1].filter).toEqual(calls()[0]);
    expect(reportMock).toHaveBeenCalledTimes(2);
  });

  it("remembers an explicit Individual display through All, single and All again", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    const marks = chartMarks();
    await openPicker();
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());

    await act(async () => {
      button("Individual").click();
    });
    await flush();
    expect(cards()).toHaveLength(12);
    expect(document.querySelector("[data-month-aggregate]")).toBeNull();
    expect(document.querySelector("[data-comparison-grid]")).not.toBeNull();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(chartMarks()).toBe(marks);

    await openPicker();
    await clickPicker(pickerItem("2026-10"));
    await clickPicker(pickerClose());
    expect(cards()).toHaveLength(1);
    expect(document.querySelector('[data-month-layout="aggregate"]')).toBeNull();

    await openPicker();
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());
    expect(cards()).toHaveLength(12);
    expect(document.querySelector("[data-month-aggregate]")).toBeNull();
    expect(
      document.querySelector('[data-month-layout="individual"]')?.getAttribute(
        "aria-pressed"
      )
    ).toBe("true");
    expect(reportMock).toHaveBeenCalledTimes(1);
  });

  it("uses single arrows to replace the month and subset arrows to move active focus only", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    expect(cards()[0].getAttribute("data-month-key")).toBe("2026-10");
    expect(reportMock).toHaveBeenCalledTimes(1);

    await openPicker();
    await clickPicker(pickerItem("2026-11"));
    await clickPicker(pickerClose());
    expect(cards()).toHaveLength(2);
    expect(cards().map((node) => node.getAttribute("data-month-key"))).toEqual([
      "2026-10",
      "2026-11",
    ]);
    expect(activeCard()?.getAttribute("data-month-key")).toBe("2026-11");

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    expect(cards()).toHaveLength(2);
    expect(activeCard()?.getAttribute("data-month-key")).toBe("2026-10");
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(button("Previous month").disabled).toBe(true);

    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(cards()).toHaveLength(2);
    expect(activeCard()?.getAttribute("data-month-key")).toBe("2026-11");

    // All/Individual arrows move focus among the twelve cards, not membership.
    await openPicker();
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());
    await act(async () => {
      button("Individual").click();
    });
    await flush();
    expect(cards()).toHaveLength(12);
    const scrollCalls: Element[] = [];
    Element.prototype.scrollIntoView = function scrollIntoView(this: Element) {
      scrollCalls.push(this);
    };
    const before = cards().map((node) => node.getAttribute("data-month-key"));
    await act(async () => {
      button("Next month").click();
    });
    await flush();
    expect(cards()).toHaveLength(12);
    expect(cards().map((node) => node.getAttribute("data-month-key"))).toEqual(
      before
    );
    expect(activeCard()?.getAttribute("data-month-key")).toBe("2026-02");
    expect(scrollCalls.at(-1)).toBe(activeCard()?.parentElement);
    expect(reportMock).toHaveBeenCalledTimes(1);

    // Aggregate has no card target, so its navigation is absent.
    await act(async () => {
      button("Aggregate").click();
    });
    await flush();
    expect(document.querySelector('[data-month-nav="previous"]')).toBeNull();
    expect(document.querySelector('[data-month-nav="next"]')).toBeNull();
  });

  it("keeps cross-year months canonical and loads exactly one source per year", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await openPicker();
    await previousPage();
    await clickPicker(pickerItem("2025-01"));
    await clickPicker(pickerClose());

    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1]).toEqual({
      from: "2025-01-01",
      to: "2026-01-01",
      bucket: "month",
      category: "all",
      currency: "IDR",
    });
    expect(cards().map((node) => node.getAttribute("data-month-key"))).toEqual([
      "2025-01",
      "2026-11",
    ]);
    expect(trigger().getAttribute("data-month-selection-summary")).toBe(
      "2 months · 2025–2026"
    );

    await openPicker();
    expect(document.querySelector("[data-picker-count]")?.textContent).toContain(
      "2 selected"
    );
    expect(pickerItem("2025-01")).not.toBeNull();
    await nextPage();
    expect(document.querySelector('[data-picker-key="2025-01"]')).toBeNull();
    expect(document.querySelector("[data-picker-count]")?.textContent).toContain(
      "2 selected"
    );
    await clickPicker(pickerItem("2026-01"));
    await clickPicker(pickerClose());
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(cards().map((node) => node.getAttribute("data-month-key"))).toEqual([
      "2025-01",
      "2026-01",
      "2026-11",
    ]);
    expect(
      document.querySelectorAll("[data-comparison-card]")
    ).toHaveLength(3);
    expect(
      document.querySelectorAll("[data-comparison-last-odd]")
    ).toHaveLength(1);
  });

  it("discloses independently generated source timestamps across a selection", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = respond(query);
      if (annualQueryFor(query) === 2025) {
        report.generatedAt = "2026-11-20T05:00:00Z";
      }
      return report;
    });
    await renderPage();
    await openPicker();
    await previousPage();
    await clickPicker(pickerItem("2025-01"));
    await clickPicker(pickerClose());

    const provenance = document.querySelector("[data-month-provenance]");
    expect(provenance?.textContent).toContain(
      "independently generated snapshots"
    );
    expect(card("2025-01").textContent).toContain("2026-11-20T05:00:00Z");
    expect(card("2026-11").textContent).toContain("2026-11-19T05:00:00Z");
  });

  it("shows no independent-snapshot note above an Aggregate built from one annual source", async () => {
    // Year view → Month view reads a day snapshot; the retained annual
    // snapshot is a different generation. All/Aggregate still reads every row
    // and the Recorded total from that one annual source and renders no cards,
    // so the cards-only note must not appear above it. Individual keeps the
    // note, because its current-month card really is the day snapshot.
    reportMock.mockImplementation(async (query: Query) => {
      const report = respond(query);
      if (query.bucket === "day") {
        report.generatedAt = "2026-11-19T05:00:00Z";
      }
      if (annualQueryFor(query) === 2026) {
        report.generatedAt = "2026-11-20T05:00:00Z";
      }
      return report;
    });
    await renderPage();
    await act(async () => {
      button("Month view").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);

    await openPicker();
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());

    expect(aggregate().textContent).toContain("2026 monthly comparison");
    expect(aggregate().textContent).toContain(
      "annual snapshot 2026-11-20T05:00:00Z"
    );
    expect(cards()).toHaveLength(0);
    expect(document.querySelector("[data-month-provenance]")).toBeNull();

    await act(async () => {
      button("Individual").click();
    });
    await flush();
    expect(cards()).toHaveLength(12);
    expect(
      document.querySelector("[data-month-provenance]")?.textContent
    ).toContain("independently generated snapshots");
    expect(card("2026-11").textContent).toContain("2026-11-19T05:00:00Z");
    expect(card("2026-10").textContent).toContain("2026-11-20T05:00:00Z");
    expect(reportMock).toHaveBeenCalledTimes(2);
  });

  it("begins an explicit empty selection, keeps the picker open and exports the empty section", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/empty.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage({ onExport });
    await openPicker();
    await clickPicker(pickerAll());
    await clickPicker(pickerItem("2026-10"));
    await clickPicker(pickerItem("2026-10"));
    expect(pickerDialog()).not.toBeNull();
    expect(document.querySelector("[data-picker-count]")?.textContent).toContain(
      "0 selected"
    );
    await clickPicker(pickerClose());

    // With no selected month, the picker opens at the server's current year.
    await openPicker();
    expect(pickerItem("2026-01")).not.toBeNull();
    expect(document.querySelector('[data-picker-key="2025-01"]')).toBeNull();
    await clickPicker(pickerClose());

    const empty = document.querySelector("[data-month-empty]");
    expect(empty?.textContent).toContain("Choose months");
    expect(document.querySelector("#report-period-table")).not.toBeNull();
    expect(cards()).toHaveLength(0);
    expect(button("Export PDF").disabled).toBe(false);

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(onExport).toHaveBeenCalledTimes(1);
    const view = onExport.mock.calls[0][0];
    expect(view.periods).toEqual({
      sources: [],
      months: [],
      layout: "individual",
    });
    expect(view.sources).toHaveLength(1);
    expect(view.categories.codes).toEqual(
      purchaseOrderReportYearFixture().categories.map(
        (row) => row.businessCategory
      )
    );
    expect(reportMock).toHaveBeenCalledTimes(1);
  });

  it("refuses a 120-month span, keeps the selection and makes no request", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await openPicker();
    await previousPage(11);
    expect(pickerItem("2015-01")).not.toBeNull();
    await clickPicker(pickerItem("2015-01"));

    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(document.querySelector("[data-month-limit-notice]")?.textContent).toContain(
      "120 months"
    );
    expect(pickerDialog()?.textContent).toContain("120 months");
    expect(trigger().getAttribute("data-month-selection-summary")).toBe(
      "November 2026"
    );
    expect(document.querySelector("[data-picker-count]")?.textContent).toContain(
      "1 selected"
    );

    // An accepted action clears the message.
    await nextPage(11);
    await clickPicker(pickerItem("2026-10"));
    expect(document.querySelector("[data-month-limit-notice]")).toBeNull();
    expect(trigger().getAttribute("data-month-selection-summary")).toBe(
      "2 months · 2026"
    );
    expect(reportMock).toHaveBeenCalledTimes(1);
  });

  it("shows a loaded zero past year as real zeros and a future year as unavailable", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await openPicker();
    await previousPage();
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1]).toEqual({
      from: "2025-01-01",
      to: "2026-01-01",
      bucket: "month",
      category: "all",
      currency: "IDR",
    });
    expect(aggregate().textContent).toContain("2025 monthly comparison");
    const zeroRow = aggregate().querySelector(
      '[data-aggregate-row="2025-01"] [data-aggregate-metric="POs"]'
    );
    expect(zeroRow?.textContent).toBe("0");
    expect(
      aggregate().querySelector(
        '[data-aggregate-row="2025-01"] [data-aggregate-metric="Amount"]'
      )?.textContent
    ).toBe("Rp0");
    expect(
      aggregate().querySelector(
        '[data-aggregate-row="2025-01"] [data-aggregate-metric="Margin %"]'
      )?.textContent
    ).toBe("-");

    await act(async () => {
      button("Individual").click();
    });
    await flush();
    expect(cards()).toHaveLength(12);
    expect(
      card("2025-01").querySelector('[data-month-metric="POs"]')?.textContent
    ).toBe("0");
    expect(
      card("2025-01").querySelector('[data-month-metric="PO amount"]')
        ?.textContent
    ).toBe("Rp0");

    await openPicker();
    await nextPage(3);
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());
    expect(reportMock).toHaveBeenCalledTimes(3);
    await act(async () => {
      button("Aggregate").click();
    });
    await flush();
    expect(aggregate().textContent).toContain("2028 monthly comparison");
    expect(aggregate().textContent).toContain("Forecast unavailable");
    expect(
      aggregate().querySelector(
        '[data-aggregate-row="2028-01"] [data-aggregate-metric="Amount"]'
      )?.textContent
    ).toBe("-");

    await act(async () => {
      button("Individual").click();
    });
    await flush();
    expect(card("2028-01").textContent).toContain("beyond the current year");
    expect(card("2028-01").textContent).not.toContain("Rp0");
  });

  it("loads several selected months of one year with a single read each time", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    await openPicker();
    await clickPicker(pickerItem("2026-03"));
    await clickPicker(pickerItem("2026-05"));
    await clickPicker(pickerItem("2026-09"));
    await previousPage();
    await clickPicker(pickerItem("2025-06"));
    await clickPicker(pickerItem("2025-10"));
    await clickPicker(pickerClose());

    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1]).toEqual({
      from: "2025-01-01",
      to: "2026-01-01",
      bucket: "month",
      category: "all",
      currency: "IDR",
    });
    expect(cards().map((node) => node.getAttribute("data-month-key"))).toEqual([
      "2025-06",
      "2025-10",
      "2026-03",
      "2026-05",
      "2026-09",
      "2026-11",
    ]);

    // Picker paging and re-selecting loaded months make no request.
    await openPicker();
    await nextPage();
    await clickPicker(pickerItem("2026-05"));
    await clickPicker(pickerItem("2026-05"));
    await clickPicker(pickerClose());
    expect(reportMock).toHaveBeenCalledTimes(2);
  });

  it("deduplicates an in-flight read for several months in the same year", async () => {
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    expect(reportMock).toHaveBeenCalledTimes(1);
    reportMock.mockImplementationOnce(() => pending.promise);

    await openPicker();
    await previousPage();
    await clickPicker(pickerItem("2025-01"));
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(card("2025-01").textContent).toContain("Loading January 2025");
    await clickPicker(pickerItem("2025-06"));
    await clickPicker(pickerItem("2025-10"));
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(pickerDialog()).not.toBeNull();

    await act(async () => {
      pending.resolve(zeroAnnual(2025));
    });
    await flush();
    expect(pickerDialog()).not.toBeNull();
    // The open picker's marks updated in place; the background cards became
    // real zero evidence without a remount.
    expect(pickerItem("2025-01").getAttribute("aria-pressed")).toBe("true");
    expect(card("2025-01").textContent).toContain("Recorded");
    expect(
      card("2025-01").querySelector('[data-month-metric="POs"]')?.textContent
    ).toBe("0");
    expect(reportMock).toHaveBeenCalledTimes(2);
  });

  it("never paints old money for a new scope and retries a failed year per browser", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage({ onExport: async () => "/tmp/opencode/retry.pdf" });
    await choose("Date range preset", "This year");
    expect(reportMock).toHaveBeenCalledTimes(2);

    const pendingAnnual = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(async (query: Query) => {
      if (annualQueryFor(query) === 2026 && query.category === "UNASSIGNED") {
        return pendingAnnual.promise;
      }
      return respond(query);
    });
    await choose("Category", "Unassigned");
    await flush();

    // The partial UNASSIGNED overview arrived; its annual source is still in
    // flight. The card must not keep the old ALL money under the new heading.
    expect(calls().at(-1)).toMatchObject({
      from: "2026-01-01",
      to: "2027-01-01",
      bucket: "month",
      category: "UNASSIGNED",
      currency: "IDR",
    });
    expect(cards()[0].textContent).toContain("Loading November 2026");
    expect(cards()[0].textContent).not.toContain("Rp190.000.000");
    expect(button("Export PDF").disabled).toBe(true);

    await act(async () => {
      pendingAnnual.reject(new Error("The monthly API is unreachable."));
    });
    await flush();
    expect(cards()[0].textContent).toContain("The monthly API is unreachable.");
    expect(cards()[0].textContent).not.toContain("Rp190.000.000");
    expect(button("Retry month")).not.toBeNull();
    expect(button("Export PDF").disabled).toBe(true);

    const beforeRetry = reportMock.mock.calls.length;
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await act(async () => {
      button("Retry month").click();
    });
    await flush();
    expect(reportMock.mock.calls.length).toBe(beforeRetry + 1);
    expect(cards()[0].textContent).not.toContain(
      "The monthly API is unreachable."
    );
    expect(cards()[0].textContent).toContain("Actual to date");
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("blocks export until every visible selected source and the overview agree", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/compare.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => {
      const report = respond(query);
      if (annualQueryFor(query) === 2025) {
        report.asOf = "2026-11-20";
      }
      return report;
    });
    await renderPage({ onExport });
    await openPicker();
    await previousPage();
    await clickPicker(pickerItem("2025-01"));
    await clickPicker(pickerClose());

    expect(card("2025-01").textContent).toContain(
      "different business date from the overview"
    );
    expect(button("Export PDF").disabled).toBe(true);
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(onExport).not.toHaveBeenCalled();
  });

  it("exports the canonical selected envelope and freezes it before the binding call", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/compare.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage({ onExport });
    await openPicker();
    await previousPage();
    await clickPicker(pickerItem("2025-01"));
    await clickPicker(pickerClose());
    expect(reportMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(onExport).toHaveBeenCalledTimes(1);
    const view = onExport.mock.calls[0][0];
    expect(view.periods.months).toEqual(["2025-01", "2026-11"]);
    expect(view.periods.sources).toEqual([1, 0]);
    expect(view.periods.layout).toBe("individual");
    expect(view.sources).toHaveLength(2);
    expect(view.sources[1].filter).toEqual(calls()[1]);

    const frozen = JSON.stringify(view);
    await openPicker();
    await nextPage();
    await clickPicker(pickerItem("2026-10"));
    await clickPicker(pickerClose());
    expect(JSON.stringify(view)).toBe(frozen);
    expect(reportMock).toHaveBeenCalledTimes(2);
  });

  it("exports the All aggregate and All individual shapes without a row sum", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/all.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage({ onExport });
    await openPicker();
    await clickPicker(pickerAll());
    await clickPicker(pickerClose());

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    const aggregateView = onExport.mock.calls[0][0];
    expect(aggregateView.periods).toEqual({
      sources: [0],
      months: Array.from({ length: 12 }, (_, index) =>
        `2026-${String(index + 1).padStart(2, "0")}`
      ),
      layout: "aggregate",
    });
    expect(aggregateView.sources[0].totals).toEqual(
      purchaseOrderReportYearFixture().totals
    );

    await act(async () => {
      button("Individual").click();
    });
    await flush();
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    const individualView = onExport.mock.calls[1][0];
    expect(individualView.periods.layout).toBe("individual");
    expect(individualView.periods.months).toHaveLength(12);
    expect(individualView.periods.sources).toEqual([0]);
    expect(reportMock).toHaveBeenCalledTimes(1);
  });
});

describe("Performance overview integration (issue #644)", () => {
  it("renders one Overview, one period toggle and one chart, with root actions and tables outside", async () => {
    await renderPage({ onExport: async () => "/tmp/opencode/overview.pdf" });
    const overview = document.querySelector<HTMLElement>(
      "[data-report-overview]"
    );
    expect(overview).not.toBeNull();
    expect(document.querySelectorAll("[data-report-overview]")).toHaveLength(1);
    expect(
      document.querySelectorAll("[data-report-period-toggle]")
    ).toHaveLength(1);
    expect(document.querySelectorAll("[data-report-chart]")).toHaveLength(1);
    expect(document.querySelector("[data-report-chart-embedded]")).not.toBeNull();
    expect(document.querySelectorAll('[aria-label="Report KPIs"]')).toHaveLength(
      1
    );
    expect(
      document.querySelectorAll('[aria-label="Chart legend"]')
    ).toHaveLength(1);
    expect(
      document.querySelectorAll("[data-report-forecast-status]")
    ).toHaveLength(1);
    expect(
      document.querySelector("[data-report-forecast-status]")?.closest("details")
    ).toBeNull();
    expect(
      document
        .querySelector("[data-report-projection-summary]")
        ?.closest("details")
    ).not.toBeNull();

    const details = document.querySelector<HTMLDetailsElement>("details");
    expect(details).not.toBeNull();
    expect(details?.open).toBe(false);
    expect(details?.querySelector("summary")?.textContent).toContain(
      "Method and chart details"
    );
    expect(
      document.querySelector("[data-report-coverage]")?.closest("details")
    ).toBeNull();
    const bodyText = document.body.textContent ?? "";
    // The default fixture is a historical range, so the current-period
    // basis/method prose does not exist at all — certainly not duplicated in
    // the visible page.
    expect(bodyText.match(/Basis:/g) ?? []).toHaveLength(0);
    expect(bodyText.match(/Method:/g) ?? []).toHaveLength(0);

    const heading = document.getElementById("reports-heading");
    const notes = Array.from(document.querySelectorAll("h2")).find(
      (node) => node.textContent === "Notes"
    );
    const category = document.querySelector(
      'table[aria-label="Category totals"]'
    );
    const month = document.querySelector("[data-month-browser]");
    const periodTable = document.querySelector(
      'table[aria-label="Period totals"]'
    );
    expect(heading).not.toBeNull();
    expect(notes).not.toBeNull();
    expect(category).not.toBeNull();
    expect(month).not.toBeNull();
    expect(periodTable).not.toBeNull();
    for (const outside of [
      heading!,
      notes!,
      category!,
      month!,
      periodTable!,
      button("Export PDF"),
    ] as Element[]) {
      expect(overview?.contains(outside)).toBe(false);
    }
  });

  it("keeps current-year basis and method once, inside the closed Details region", async () => {
    reportMock.mockImplementation(async (query: Query) => {
      const report = purchaseOrderReportYearFixture();
      report.filter = { ...query };
      return report;
    });
    await renderPage();

    const details = document.querySelector<HTMLDetailsElement>("details");
    expect(details).not.toBeNull();
    expect(details?.open).toBe(false);
    expect(details?.textContent).toContain("Basis:");
    expect(details?.textContent).toContain("Method:");
    expect(details?.textContent).toContain("Updates as data arrives");

    const bodyText = document.body.textContent ?? "";
    expect(bodyText.match(/Basis:/g) ?? []).toHaveLength(1);
    expect(bodyText.match(/Method:/g) ?? []).toHaveLength(1);
    expect(bodyText.match(/Updates as data arrives/g) ?? []).toHaveLength(1);
    expect(
      document.querySelector("[data-report-forecast-status]")?.closest("details")
    ).toBeNull();
  });

  it("keeps the chart marks through month browsing, Details and an unrelated parent re-render", async () => {
    await renderPage();
    const chart = document.querySelector("[data-report-chart]");
    const marks = document.querySelector("[data-chart-marks]");
    expect(chart).not.toBeNull();
    expect(marks).not.toBeNull();

    await act(async () => {
      button("Previous month").click();
    });
    await flush();
    expect(document.querySelector("[data-report-chart]")).toBe(chart);
    expect(document.querySelector("[data-chart-marks]")).toBe(marks);

    const details = document.querySelector<HTMLDetailsElement>("details");
    await act(async () => {
      details?.querySelector("summary")?.click();
    });
    await flush();
    expect(details?.open).toBe(true);
    expect(document.querySelector("[data-report-chart]")).toBe(chart);
    expect(document.querySelector("[data-chart-marks]")).toBe(marks);

    const callsBefore = reportMock.mock.calls.length;
    await act(async () => {
      root.render(
        <ToastProvider>
          <PurchaseOrderReportsPage
            defaultCurrency="IDR"
            currencyCatalogue={[...CATALOGUE]}
          />
        </ToastProvider>
      );
    });
    await flush();
    expect(reportMock.mock.calls.length).toBe(callsBefore);
    expect(document.querySelector("[data-report-chart]")).toBe(chart);
    expect(document.querySelector("[data-chart-marks]")).toBe(marks);
  });

  it("points the chart's exact-value anchor at the selected month table", async () => {
    await renderPage();
    const link = document.querySelector<HTMLAnchorElement>(
      '[data-report-overview] a[href="#report-period-table"]'
    );
    expect(link).not.toBeNull();
    expect(link?.textContent).toContain("Selected month");
    const table = assertExactValuesTarget(
      "overview ready",
      document.querySelector("[data-month-browser]")
    );
    expect(table.closest("[data-month-browser]")).not.toBeNull();
    expect(table.getAttribute("aria-label")).toBe("Period totals");
  });
});
