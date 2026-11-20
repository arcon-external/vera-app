// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PurchaseOrderReportsPage } from "./PurchaseOrderReportsPage";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMaxFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
} from "../poReportFixtures";
import {
  initialReportFilters,
  reportPeriodRange,
  reportRangeLabel,
  toReportQuery,
} from "../poReportPage";
import type { PurchaseOrderReportData } from "../poReport";
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

function reportFor(query: Query): PurchaseOrderReportData {
  const report = purchaseOrderReportFixture();
  report.filter = { ...query };
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
  onExport?: (report: PurchaseOrderReportData) => Promise<string>;
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
    expect(reportMock).toHaveBeenCalledTimes(5);
    expect(calls()[4].bucket).toBe("year");

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
    expect(document.body.textContent).toContain("Jan 2026");

    await choose("Bucket", "Year");

    // The previous monthly table is still visible, labelled as updating.
    expect(document.body.textContent).toContain("Jan 2026");
    expect(document.body.textContent).toContain("Updating the report");
    expect(button("Export PDF").disabled).toBe(true);

    act(() => {
      pending.resolve(reportFor({ ...calls()[1] }));
    });
    await flush();

    expect(document.body.textContent).toContain("2026");
    expect(document.body.textContent).not.toContain("Jan 2026");
    expect(document.body.textContent).not.toContain("Updating the report");
    expect(button("Export PDF").disabled).toBe(false);
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

    // The slower month request must not repaint the year table.
    expect(document.body.textContent).toContain("2026");
    expect(document.body.textContent).not.toContain("Jan 2026");
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

  it("marks a partial period and foots both tables", async () => {
    await renderPage();

    expect(document.body.textContent).toContain("Partial");
    const tables = [
      document.querySelector('table[aria-label="Category totals"]'),
      document.querySelector('table[aria-label="Period totals"]'),
    ];
    for (const table of tables) {
      if (table === null) throw new Error("expected both actual tables");
      const foot = table.querySelector("tfoot");
      expect(foot?.textContent).toContain("Total");
      expect(foot?.textContent).toContain("Rp480.000.000");
      expect(foot?.textContent).toContain("Rp42.000.000");
      expect(foot?.textContent).toContain("10.77%");
    }
  });

  it("keeps the last column's body values inside the same right inset as its header", async () => {
    await renderPage();

    for (const label of ["Category totals", "Period totals"]) {
      const table = document.querySelector(`table[aria-label="${label}"]`);
      if (table === null) throw new Error(`expected the ${label} table`);
      const header = table.querySelector("thead th:last-child");
      const cell = table.querySelector("tbody td:last-child");
      expect(header?.className).toContain("pr-5");
      expect(cell?.className).toContain("pr-5");
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
    expect(document.body.textContent).not.toContain("PO amount");
  });

  it("shows a real error state with retry, never fake zeros", async () => {
    reportMock.mockRejectedValueOnce(new Error("The API is unreachable."));
    await renderPage();

    expect(document.body.textContent).toContain("Report unavailable");
    expect(document.body.textContent).toContain("The API is unreachable.");
    expect(document.body.textContent).not.toContain("PO amount");
    expect(document.body.textContent).not.toContain("Rp0");

    await act(async () => {
      button("Retry report").click();
    });
    await flush();

    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("PO amount");
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
    expect(kpi?.textContent).toContain("Rp480.000.000");
    // The ghost December base is Rp50m and the November remainder is Rp20m;
    // neither may appear in the actual total.
    expect(kpi?.textContent).not.toContain("Rp530.000.000");
    expect(kpi?.textContent).not.toContain("Rp500.000.000");

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
    expect(periodRows).toHaveLength(19);
    expect(
      document.querySelector('table[aria-label="Period totals"]')?.textContent
    ).not.toContain("30 Nov");
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
    ).toContain("Rp480.000.000");
    expect(document.querySelector("[data-chart-remainder]")).not.toBeNull();
    expect(
      document.querySelectorAll(
        'table[aria-label="Period totals"] tbody tr'
      )
    ).toHaveLength(11);
  });

  it("clears the toggle and draws no ghost for a historical preset", async () => {
    usePeriodReport();
    await renderPage();

    await choose("Date range preset", "Last year");

    expect(calls().at(-1)).toMatchObject({
      from: "2025-01-01",
      to: "2026-01-01",
      bucket: "month",
    });
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
      async (_report: PurchaseOrderReportData) => "/tmp/opencode/report.pdf"
    );
    await renderPage({ onExport });

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    const yearSnapshot = onExport.mock.calls[0][0] as PurchaseOrderReportData;
    expect(yearSnapshot.currentProjection.mode).toBe("month");
    expect(yearSnapshot.currentProjection.status).toBe("available");
    expect(yearSnapshot.currentProjection.buckets).toHaveLength(1);
    expect(
      yearSnapshot.currentProjection.currentMonth?.remainder.base
    ).toBe(20_000_000);

    await act(async () => {
      button("Month view").click();
    });
    await flush();
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    const monthSnapshot = onExport.mock.calls[1][0] as PurchaseOrderReportData;
    expect(monthSnapshot.currentProjection.mode).toBe("day");
    expect(monthSnapshot.currentProjection.buckets).toHaveLength(11);
    expect(monthSnapshot.filter).toEqual(calls().at(-1));
  });
});

describe("Reports export seam (issue #630)", () => {
  it("disables Export PDF when no callback is wired", async () => {
    await renderPage();
    expect(button("Export PDF").disabled).toBe(true);
  });

  it("sends the visible normalized snapshot to the callback", async () => {
    const onExport = vi.fn(
      async (_report: PurchaseOrderReportData) => "/tmp/opencode/po-report.pdf"
    );
    await renderPage({ onExport });

    expect(button("Export PDF").disabled).toBe(false);
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(onExport).toHaveBeenCalledTimes(1);
    const sent = onExport.mock.calls[0][0] as PurchaseOrderReportData;
    expect(sent.filter).toEqual(calls()[0]);
    expect(sent.totals).toEqual(purchaseOrderReportFixture().totals);
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
