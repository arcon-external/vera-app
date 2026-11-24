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

const { reportMock, fulfillmentMock } = vi.hoisted(() => ({
  reportMock: vi.fn(),
  fulfillmentMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  GetPurchaseOrderReport: (...args: unknown[]) => reportMock(...args),
  GetPurchaseOrderFulfillment: (...args: unknown[]) => fulfillmentMock(...args),
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
        extraExpenseMinor: period.extraExpenseMinor,
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

interface FulfillmentQueryInput {
  currency?: string;
  status?: string;
  search?: string;
  page?: number;
  pageSize?: number;
}

function emptyFulfillment(query: FulfillmentQueryInput = {}) {
  const page = query.page ?? 1;
  const pageSize = query.pageSize ?? 25;
  return {
    asOf: "2026-11-19",
    filter: {
      currency: query.currency ?? "IDR",
      status: query.status ?? "all",
      search: query.search ?? "",
      page,
      pageSize,
    },
    quotations: [],
    page,
    pageSize,
    total: 0,
    totalPages: 1,
  };
}

beforeEach(() => {
  reportMock.mockReset();
  reportMock.mockImplementation(async (query: Query) => reportFor(query));
  fulfillmentMock.mockReset();
  fulfillmentMock.mockImplementation(async (query: FulfillmentQueryInput) =>
    emptyFulfillment(query)
  );
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
  onExportXLSX?: (view: PurchaseOrderReportView) => Promise<string>;
  mutationEpoch?: number;
  isAdmin?: boolean;
  onCloseQuotation?: (quotationID: number, reason: string) => Promise<unknown>;
  onReopenQuotation?: (quotationID: number) => Promise<unknown>;
}

async function renderPage(options: RenderOptions = {}): Promise<void> {
  await act(async () => {
    root.render(
      <ToastProvider>
        <PurchaseOrderReportsPage
          defaultCurrency={options.defaultCurrency ?? "IDR"}
          currencyCatalogue={options.catalogue ?? CATALOGUE}
          onExport={options.onExport}
          onExportXLSX={options.onExportXLSX}
          mutationEpoch={options.mutationEpoch ?? 0}
          isAdmin={options.isAdmin}
          onCloseQuotation={options.onCloseQuotation}
          onReopenQuotation={options.onReopenQuotation}
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

  it("mounts the fulfillment drill-down below the overview in the report currency", async () => {
    await renderPage();

    const overview = document.querySelector<HTMLElement>(
      "[data-report-overview]"
    );
    const card = document.querySelector<HTMLElement>(
      "[data-purchase-order-fulfillment]"
    );
    if (overview === null || card === null) {
      throw new Error("the page must mount the fulfillment card");
    }
    expect(
      overview.compareDocumentPosition(card) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
    expect(fulfillmentMock).toHaveBeenCalledTimes(1);
    expect(fulfillmentMock.mock.calls[0][0]).toEqual({
      currency: "IDR",
      status: "all",
      search: "",
      page: 1,
      pageSize: 25,
    });
  });

  it("passes the admin close action through to the fulfillment workspace (#648)", async () => {
    fulfillmentMock.mockReset().mockImplementation(async (query: FulfillmentQueryInput) => ({
      ...emptyFulfillment(query),
      quotations: [
        {
          quotationId: 7,
          quoteNumber: "1502260007",
          customerName: "PT. Infineon Technologies Batam",
          currency: "IDR",
          latestRevision: 1,
          issuedAt: "2026-10-10",
          validUntil: "2026-11-09",
          status: "open",
          items: [],
          extraExpenses: [],
        },
      ],
      total: 1,
    }));
    const closeQuotation = vi.fn(async () => undefined);
    const reopenQuotation = vi.fn(async () => undefined);
    await renderPage({
      isAdmin: true,
      onCloseQuotation: closeQuotation,
      onReopenQuotation: reopenQuotation,
    });

    const close = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Close quotation 1502260007"]'
    );
    if (close === null) throw new Error("an admin must see the close action");
    await act(async () => {
      close.click();
    });
    const reason = document.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="Close reason"]'
    );
    if (reason === null) throw new Error("no close reason field");
    act(() => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        "value"
      )?.set;
      setter?.call(reason, "Customer cancelled the remainder");
      reason.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      button("Confirm close quotation").click();
    });
    await flush();
    expect(closeQuotation).toHaveBeenCalledWith(
      7,
      "Customer cancelled the remainder"
    );

    await renderPage({
      isAdmin: false,
      onCloseQuotation: closeQuotation,
      onReopenQuotation: reopenQuotation,
    });
    expect(
      document.querySelector('button[aria-label="Close quotation 1502260007"]')
    ).toBeNull();
  });

  it("follows the report's selected currency into the drill-down", async () => {
    await renderPage({ defaultCurrency: "USD" });
    expect(fulfillmentMock.mock.calls[0][0]).toMatchObject({ currency: "USD" });
  });

  it("re-reads fulfillment after a PO mutation and never keeps stale figures", async () => {
    const quotation = (realized: number) => ({
      quotationId: 7,
      quoteNumber: "1502260007",
      customerName: "PT. Infineon Technologies Batam",
      currency: "IDR",
      latestRevision: 1,
      issuedAt: "2026-10-10",
      validUntil: "2026-11-09",
      status: "closed",
      items: [
        {
          quotationItemNo: 1,
          quotationSubNo: null,
          label: "1",
          description: "Fulfillment line",
          groupTitle: "",
          removed: false,
          quotedQuantityScaled: 50000,
          boughtQuantityScaled: 30000,
          unsoldQuantityScaled: 0,
          overBoughtQuantityScaled: 0,
          realizedAmountMinor: realized,
          realizedMarginMinor: Math.trunc(realized / 2),
          unsoldAmountMinor: 0,
          unsoldMarginMinor: 0,
          quantityExtraIncomeAmountMinor: 0,
          quantityExtraIncomeMarginMinor: 0,
          priceExtraIncomeMinor: 0,
          priceLossMinor: 0,
          closedLossAmountMinor: 40000,
          closedLossMarginMinor: 20000,
          purchaseOrders: [],
        },
      ],
      extraExpenses: [],
    });
    fulfillmentMock
      .mockReset()
      .mockImplementationOnce(async (query: FulfillmentQueryInput) => ({
        ...emptyFulfillment(query),
        quotations: [quotation(60000)],
        total: 1,
      }))
      .mockImplementationOnce(async (query: FulfillmentQueryInput) => ({
        ...emptyFulfillment(query),
        quotations: [quotation(80000)],
        total: 1,
      }));

    await renderPage({ mutationEpoch: 0 });
    const card = document.querySelector<HTMLElement>(
      "[data-purchase-order-fulfillment]"
    );
    expect(card?.textContent).toContain("Rp60.000");

    await renderPage({ mutationEpoch: 1 });
    expect(fulfillmentMock).toHaveBeenCalledTimes(2);
    expect(card?.textContent).toContain("Rp80.000");
    expect(card?.textContent).not.toContain("Rp60.000");
  });

  it("sends every filter exactly, one fetch per new scope, including All and Unassigned", async () => {
    await renderPage();
    expect(reportMock).toHaveBeenCalledTimes(1);

    await choose("Category", "Unassigned");
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1].category).toBe("UNASSIGNED");

    // Issue #645: returning to an exact scope this mount already loaded is a
    // cache read, not a fetch. The controls still read "All categories".
    await choose("Category", "All categories");
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(button("Category").textContent).toContain("All categories");

    await choose("Currency", "USD");
    expect(reportMock).toHaveBeenCalledTimes(3);
    expect(calls()[2].currency).toBe("USD");

    await choose("Bucket", "Year");
    // The year-bucketed overview is a new read, but the month browser reuses
    // the exact annual snapshot the initial Year view already loaded.
    expect(reportMock).toHaveBeenCalledTimes(4);
    expect(calls()[3].bucket).toBe("year");
    expect(calls()[2].bucket).toBe("month");

    await typeDate("From date", "2025-12-01");
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(5);
    expect(calls()[4]).toMatchObject({
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
        extraExpenseMinor: 0,
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
        extraExpenseMinor: 0,
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

    // Issue #645: the initial Year view already loaded this exact annual
    // scope, so returning to it is a cache read. The last *request* is still
    // the month read; the screen must still show the year snapshot.
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
    expect(reportMock).toHaveBeenCalledTimes(2);
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

// #683: the Excel workbook is a second native save over the same frozen view.
// It mirrors the PDF seam exactly (same snapshot, same failure isolation) and
// the two exports never run at the same time.
describe("Reports Excel export seam (issue #683)", () => {
  it("disables Export Excel when no XLSX callback is wired", async () => {
    await renderPage({ onExport: async () => "/tmp/opencode/report.pdf" });
    expect(button("Export Excel").disabled).toBe(true);
    expect(button("Export Excel").title).toBe("Excel export is not available.");
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("disables Export Excel while no report is loaded", async () => {
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementationOnce(() => pending.promise);
    const onExportXLSX = vi.fn(async () => "/tmp/opencode/report.xlsx");
    await renderPage({
      onExport: async () => "/tmp/opencode/report.pdf",
      onExportXLSX,
    });

    expect(button("Export Excel").disabled).toBe(true);
    expect(button("Export Excel").title).toBe(
      "A valid, current report is required before export."
    );
    pending.resolve(reportFor({ ...calls()[0] }));
    await flush();
    expect(button("Export Excel").disabled).toBe(false);
    expect(button("Export Excel").title).toBe(
      "Save this visible report as an Excel workbook"
    );
  });

  it("sends the same frozen view to Excel and PDF and toasts the saved path", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/report.pdf"
    );
    const onExportXLSX = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/report.xlsx"
    );
    await renderPage({ onExport, onExportXLSX });

    await act(async () => {
      button("Export Excel").click();
    });
    await flush();

    expect(onExportXLSX).toHaveBeenCalledTimes(1);
    expect(onExport).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain(
      "Report saved to /tmp/opencode/report.xlsx"
    );

    const xlsxView = onExportXLSX.mock.calls[0][0] as PurchaseOrderReportView;
    expect(xlsxView.version).toBe(1);
    expect(xlsxView.overview).toBe(0);
    expect(xlsxView.sources[0].filter).toEqual(calls()[0]);
    expect(xlsxView.sources[0].totals).toEqual(
      purchaseOrderReportFixture().totals
    );

    // The workbook save does not disturb the report or the PDF path: the PDF
    // export that follows carries byte-identical frozen evidence.
    expect(button("Export PDF").disabled).toBe(false);
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(onExport).toHaveBeenCalledTimes(1);
    expect(onExport.mock.calls[0][0]).toEqual(xlsxView);
    expect(reportMock).toHaveBeenCalledTimes(1);
  });

  it("reports an Excel failure and keeps the visible report", async () => {
    const onExportXLSX = vi
      .fn()
      .mockRejectedValueOnce(new Error("The save dialog was refused."))
      .mockRejectedValueOnce({});
    await renderPage({
      onExport: async () => "/tmp/opencode/report.pdf",
      onExportXLSX,
    });

    await act(async () => {
      button("Export Excel").click();
    });
    await flush();

    expect(document.body.textContent).toContain("The save dialog was refused.");
    expect(document.body.textContent).not.toContain(
      "Report saved to /tmp/opencode/report.xlsx"
    );
    // No refetch: the same visible snapshot is still on screen and exportable.
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(
      document.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).toContain("Rp");
    expect(button("Export Excel").disabled).toBe(false);
    expect(button("Export PDF").disabled).toBe(false);

    // A shapeless rejection still names what failed, not a generic sentence.
    await act(async () => {
      button("Export Excel").click();
    });
    await flush();
    expect(document.body.textContent).toContain(
      "The Excel workbook could not be saved. Choose another location and try again."
    );
    expect(button("Export Excel").disabled).toBe(false);
  });

  it("treats an empty workbook path as cancelled, with no success toast", async () => {
    const onExportXLSX = vi.fn(async () => "");
    await renderPage({
      onExport: async () => "/tmp/opencode/report.pdf",
      onExportXLSX,
    });

    await act(async () => {
      button("Export Excel").click();
    });
    await flush();

    expect(onExportXLSX).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toContain("Report saved");
    expect(document.body.textContent).not.toContain("Excel workbook could not");
    expect(button("Export Excel").disabled).toBe(false);
  });

  it("never runs the PDF and Excel exports at the same time", async () => {
    const xlsx = deferred<string>();
    const pdf = deferred<string>();
    const onExport = vi.fn(() => pdf.promise);
    const onExportXLSX = vi.fn(() => xlsx.promise);
    await renderPage({ onExport, onExportXLSX });

    await act(async () => {
      button("Export Excel").click();
    });
    await flush();
    expect(onExportXLSX).toHaveBeenCalledTimes(1);
    expect(button("Export Excel").disabled).toBe(true);
    expect(button("Export Excel").title).toBe("The workbook is being saved.");
    expect(button("Export PDF").disabled).toBe(true);

    xlsx.resolve("/tmp/opencode/report.xlsx");
    await flush();
    expect(button("Export Excel").disabled).toBe(false);
    expect(button("Export PDF").disabled).toBe(false);

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(onExport).toHaveBeenCalledTimes(1);
    expect(button("Export PDF").disabled).toBe(true);
    expect(button("Export Excel").disabled).toBe(true);
    expect(button("Export Excel").title).toBe(
      "Wait for the PDF export to finish."
    );

    pdf.resolve("/tmp/opencode/report.pdf");
    await flush();
    expect(button("Export PDF").disabled).toBe(false);
    expect(button("Export Excel").disabled).toBe(false);
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
          extraExpenseMinor: 0,
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
            extraExpenseMinor: period.extraExpenseMinor,
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
      extraExpenseMinor: 0,
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
    extraExpenseMinor: 0,
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

describe("category comparison (issue #643)", () => {
  const PINNED_TODAY = "2026-11-19";
  const ZERO = {
    poCount: 0,
    amountMinor: 0,
    costedMinor: 0,
    uncostedMinor: 0,
    extraExpenseMinor: 0,
    capitalMinor: 0,
    marginMinor: 0,
    marginBps: null,
    coverageBps: null,
  } as const;
  const FIVE_CATEGORY_CODES = [
    "FM",
    "FAB",
    "SUPPLY_GEN",
    "OTHERS",
    "UNASSIGNED",
  ];

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(`${PINNED_TODAY}T03:00:00Z`));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  /**
   * A Performance snapshot restricted to one Business category with no
   * purchase orders. The category comparison must still render genuine
   * all-category evidence below it, outside the overview's empty gate.
   */
  function emptyRestricted(query: Query): PurchaseOrderReportData {
    const report = reportFor(query);
    report.filter = { ...query };
    report.totals = { ...ZERO };
    report.periods = [];
    report.categories = report.categories
      .filter((row) => row.businessCategory === query.category)
      .map((row) => ({ ...row, ...ZERO }));
    report.categorySeries = [];
    report.monthSummary = {
      month: "2026-11",
      from: "2026-11-01",
      to: "2026-12-01",
      asOf: report.asOf,
      mode: "month",
      actual: { ...ZERO },
      projection: {
        status: "unavailable",
        reason: "insufficient_months",
        method: "No current-year forecast in this fixture.",
      },
    };
    report.currentProjection = {
      ...report.currentProjection,
      mode: "none",
      status: "unavailable",
      reason: "not_current_period",
      buckets: [],
      currentMonth: undefined,
    };
    return report;
  }

  function respond(query: Query): PurchaseOrderReportData {
    if (query.category !== "all") return emptyRestricted(query);
    const report = reportFor(query);
    report.filter = { ...query };
    return report;
  }

  function categoryTrigger(): HTMLButtonElement {
    return button("Choose categories");
  }

  function categoryPickerKey(code: string): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>(
      `[data-picker-key="${code}"]`
    );
    if (found === null) throw new Error(`no category picker item ${code}`);
    return found;
  }

  function categoryScope(): HTMLElement {
    const found = document.querySelector<HTMLElement>("[data-category-scope]");
    if (found === null) throw new Error("no category scope");
    return found;
  }

  function categoryClose(): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>("[data-picker-close]");
    if (found === null) throw new Error("no category picker close");
    return found;
  }

  function categoryAll(): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>("[data-picker-all]");
    if (found === null) throw new Error("no All categories control");
    return found;
  }

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

  async function exportPdf(): Promise<void> {
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
  }

  async function openCategoryPicker(): Promise<void> {
    await act(async () => {
      categoryTrigger().click();
    });
    await flush();
  }

  async function clickPickerNode(control: HTMLElement): Promise<void> {
    await act(async () => {
      control.click();
    });
    await flush();
  }

  it("renders the independent category section under an empty restricted Performance with no extra request", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage();
    expect(reportMock).toHaveBeenCalledTimes(1);

    await choose("Category", "Facility Management");
    expect(calls()[1]).toMatchObject({ category: "FM" });
    expect(reportMock).toHaveBeenCalledTimes(2);

    const empty = document.querySelector("[data-report-overview-empty]");
    const browser = document.querySelector<HTMLElement>(
      "[data-category-browser]"
    );
    expect(empty).not.toBeNull();
    expect(browser).not.toBeNull();
    expect(
      empty!.compareDocumentPosition(browser!) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();

    // The section is the all-category universe, not the restricted snapshot.
    expect(categoryScope().textContent).toContain("All categories");
    expect(categoryScope().textContent).toContain(reportRangeLabel(calls()[0]));
    expect(categoryScope().textContent).toContain("IDR");
    expect(categoryScope().textContent).toContain("As of 19 Nov 2026");
    expect(categoryScope().textContent).not.toContain("Facility Management");
    // By period still inherits Performance's own restricted category.
    expect(
      document.querySelector("[data-month-scope]")?.textContent
    ).toContain("Facility Management");

    const aggregate = browser!.querySelector("[data-category-aggregate]");
    expect(aggregate).not.toBeNull();
    const fabrication = browser!.querySelector('[data-category-row="FAB"]');
    expect(fabrication?.textContent).toContain("Rp120.000.000");
    expect(fabrication?.textContent).toContain("Rp-30.000.000");
    expect(
      browser!.querySelector('[data-category-row="UNASSIGNED"]')
    ).not.toBeNull();
    expect(
      categoryTrigger().getAttribute("data-category-selection-summary")
    ).toBe("All categories");
    expect(reportMock).toHaveBeenCalledTimes(2);
  });

  it("keeps export enabled for an explicit empty category selection and freezes the v1 envelope", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) =>
        "/tmp/opencode/category-empty.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderPage({ onExport });

    // Restrict Performance so the overview's own request stays answerable while
    // the independent all-category scope is the one that cannot resolve.
    await choose("Category", "Facility Management");
    expect(reportMock).toHaveBeenCalledTimes(2);

    await openCategoryPicker();
    // A specific click while All is selected focuses that category only.
    await clickPickerNode(categoryPickerKey("FAB"));
    expect(document.querySelector("[data-category-card]")).not.toBeNull();
    // The next click toggles it off, leaving the explicit empty selection.
    await clickPickerNode(categoryPickerKey("FAB"));
    expect(document.querySelector("[data-category-empty]")).not.toBeNull();
    expect(
      document.querySelector("[data-category-metric]")
    ).toBeNull();
    const close = document.querySelector<HTMLButtonElement>(
      "[data-picker-close]"
    );
    if (close === null) throw new Error("no category picker close");
    await clickPickerNode(close);
    expect(document.querySelector("[data-report-picker]")).toBeNull();

    // The empty selection needs no financial category source: a still-loading
    // all-category scope cannot block its explicit empty section.
    reportMock.mockImplementation(async (query: Query) => {
      if (query.category === "all" && query.currency === "USD") {
        return new Promise<PurchaseOrderReportData>(() => undefined);
      }
      return respond(query);
    });
    await choose("Currency", "USD");
    await flush();

    // The explicit empty selection survives the inherited scope change. While
    // the unrestricted snapshot loads it shows no old-scope money, and because
    // no selected category evidence is required the export stays available.
    expect(document.querySelector("[data-category-metric]")).toBeNull();
    expect(document.body.textContent).toContain("Loading categories");
    expect(button("Export PDF").disabled).toBe(false);
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(onExport).toHaveBeenCalledTimes(1);
    const view = onExport.mock.calls[0][0];
    expect(view.version).toBe(1);
    expect(view.categories).toEqual({
      source: 0,
      codes: [],
      layout: "individual",
    });
    // Empty categories add no extra snapshot; the overview remains source 0.
    expect(view.sources).toHaveLength(1);
    expect(view.overview).toBe(0);
  });

  it("places the category section below By period and above Notes, replacing the legacy table", async () => {
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage();

    const month = document.querySelector("[data-month-browser]");
    const category = document.querySelector("[data-category-browser]");
    const notes = Array.from(document.querySelectorAll("h2")).find(
      (node) => node.textContent === "Notes"
    );
    expect(month).not.toBeNull();
    expect(category).not.toBeNull();
    expect(notes).not.toBeUndefined();
    expect(
      month!.compareDocumentPosition(category!) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
    expect(
      category!.compareDocumentPosition(notes!) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
    // One By category heading and one Category totals table: the live browser
    // owns both, and the legacy direct-view table is not duplicated beside it.
    expect(
      Array.from(document.querySelectorAll("h2")).filter(
        (node) => node.textContent === "By category"
      )
    ).toHaveLength(1);
    expect(
      document.querySelectorAll('table[aria-label="Category totals"]')
    ).toHaveLength(1);
    expect(
      document
        .querySelector('table[aria-label="Category totals"]')
        ?.closest("[data-category-aggregate]")
    ).not.toBeNull();
  });

  it("selects a specific category from All without changing Performance or By period", async () => {
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage();
    const marks = document.querySelector("[data-chart-marks]");
    const kpis = document.querySelector('[aria-label="Report KPIs"]')?.textContent;
    const monthsSummary = button("Choose months").getAttribute(
      "data-month-selection-summary"
    );
    expect(reportMock).toHaveBeenCalledTimes(1);

    await openCategoryPicker();
    const dialog = document.querySelector("[data-report-picker]");
    expect(dialog).not.toBeNull();
    await clickPickerNode(categoryPickerKey("FAB"));

    // The modal stays open and updates its mark and count in place.
    expect(document.querySelector("[data-report-picker]")).toBe(dialog);
    expect(categoryPickerKey("FAB").getAttribute("aria-pressed")).toBe("true");
    expect(document.querySelector("[data-picker-count]")?.textContent).toContain(
      "1 selected"
    );
    const card = document.querySelector(
      '[data-category-card][data-category-key="FAB"]'
    );
    expect(card).not.toBeNull();
    expect(
      card?.querySelector("[data-category-card-heading]")?.textContent
    ).toBe("Fabrication");
    expect(
      card?.querySelectorAll("[data-category-metric]")
    ).toHaveLength(6);
    expect(
      card?.querySelector('[data-category-metric="PO margin"]')?.className
    ).toContain("text-danger-700");

    // Performance, its chart node and By period are untouched.
    expect(document.querySelector("[data-chart-marks]")).toBe(marks);
    expect(document.querySelector('[aria-label="Report KPIs"]')?.textContent).toBe(
      kpis
    );
    expect(
      button("Choose months").getAttribute("data-month-selection-summary")
    ).toBe(monthsSummary);
    expect(reportMock).toHaveBeenCalledTimes(1);

    await clickPickerNode(categoryClose());
    expect(document.querySelector("[data-report-picker]")).toBeNull();
    expect(document.activeElement).toBe(categoryTrigger());
    expect(
      categoryTrigger().getAttribute("data-category-selection-summary")
    ).toBe("Fabrication");
  });

  it("does not dismiss the category picker on item, backdrop or Escape and restores the same trigger", async () => {
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage();
    const opener = categoryTrigger();
    await openCategoryPicker();
    await clickPickerNode(categoryPickerKey("FAB"));
    expect(document.querySelector("[data-report-picker]")).not.toBeNull();

    const backdrop = document.querySelector<HTMLElement>(
      "[data-report-picker-backdrop]"
    );
    if (backdrop === null) throw new Error("no category picker backdrop");
    await act(async () => {
      backdrop.click();
    });
    await flush();
    expect(document.querySelector("[data-report-picker]")).not.toBeNull();

    await act(async () => {
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", bubbles: true })
      );
    });
    await flush();
    expect(document.querySelector("[data-report-picker]")).not.toBeNull();

    await clickPickerNode(categoryClose());
    expect(document.querySelector("[data-report-picker]")).toBeNull();
    expect(document.activeElement).toBe(opener);

    // An inherited category change keeps the trigger node and the local
    // selection, and asks the shared registry for no new all-category read.
    await choose("Category", "Facility Management");
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(categoryTrigger()).toBe(opener);
    expect(
      categoryTrigger().getAttribute("data-category-selection-summary")
    ).toBe("Fabrication");
    expect(
      document.querySelector('[data-category-card][data-category-key="FAB"]')
    ).not.toBeNull();

    await openCategoryPicker();
    await clickPickerNode(categoryClose());
    expect(document.activeElement).toBe(opener);
    expect(categoryTrigger()).toBe(opener);
  });

  it("fetches one all-category snapshot for an uncached restricted scope, never one per code", async () => {
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage();
    expect(reportMock).toHaveBeenCalledTimes(1);
    expect(calls()[0]).toMatchObject({ category: "all", currency: "IDR" });

    await choose("Category", "Facility Management");
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(calls()[1]).toMatchObject({ category: "FM", currency: "IDR" });
    // The all-category snapshot at the same range was already cached.
    expect(calls().filter((call) => call.category === "all")).toHaveLength(1);

    await choose("Category", "Fabrication");
    expect(reportMock).toHaveBeenCalledTimes(3);
    expect(calls()[2]).toMatchObject({ category: "FAB", currency: "IDR" });
    expect(calls().filter((call) => call.category === "all")).toHaveLength(1);

    await choose("Currency", "USD");
    expect(reportMock).toHaveBeenCalledTimes(5);
    expect(calls()[3]).toMatchObject({ category: "FAB", currency: "USD" });
    expect(calls()[4]).toEqual({
      from: "2026-01-01",
      to: "2027-01-01",
      bucket: "month",
      category: "all",
      currency: "USD",
    });
    // Exactly two all-category reads over the whole journey: the initial
    // overview and the one new inherited scope. No query names the selected
    // category for the local comparison.
    expect(calls().filter((call) => call.category === "all")).toHaveLength(2);
    expect(
      calls()
        .filter((call) => call.category === "FAB")
        .map((call) => call.currency)
    ).toEqual(["IDR", "USD"]);
    expect(
      document.querySelector('[data-category-row="FAB"]')?.textContent
    ).toContain("$1,200,000.00");
  });

  it("never paints old money while a new all-category scope loads and retries only that source", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) =>
        "/tmp/opencode/category-retry.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage({ onExport });
    await choose("Category", "Facility Management");
    expect(reportMock).toHaveBeenCalledTimes(2);
    await openCategoryPicker();
    await clickPickerNode(categoryPickerKey("FAB"));
    await clickPickerNode(categoryClose());
    expect(reportMock).toHaveBeenCalledTimes(2);

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(async (query: Query) => {
      if (query.category === "all" && query.currency === "USD") {
        return pending.promise;
      }
      return reportFor(query);
    });
    await choose("Currency", "USD");
    expect(reportMock).toHaveBeenCalledTimes(4);

    expect(categoryTrigger().disabled).toBe(true);
    expect(categoryScope().textContent).toContain("USD");
    expect(document.body.textContent).toContain("Loading categories");
    expect(document.querySelector("[data-category-metric]")).toBeNull();
    expect(document.querySelector("[data-category-aggregate]")).toBeNull();
    expect(document.body.textContent).not.toContain("Rp120.000.000");
    expect(button("Export PDF").disabled).toBe(true);

    await act(async () => {
      pending.reject(new Error("The all-category API is unreachable."));
    });
    await flush();
    expect(document.body.textContent).toContain(
      "The all-category API is unreachable."
    );
    expect(button("Retry categories")).not.toBeNull();
    expect(document.querySelector("[data-category-metric]")).toBeNull();
    expect(document.body.textContent).not.toContain("Rp120.000.000");
    expect(button("Export PDF").disabled).toBe(true);

    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await act(async () => {
      button("Retry categories").click();
    });
    await flush();
    expect(reportMock).toHaveBeenCalledTimes(5);
    expect(calls()[4]).toMatchObject({ category: "all", currency: "USD" });
    expect(
      document.querySelector(
        '[data-category-card][data-category-key="FAB"] [data-category-metric="PO amount"]'
      )?.textContent
    ).toBe("$1,200,000.00");
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("surfaces a currency-mismatched category snapshot as an error, never zero", async () => {
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage();
    await choose("Category", "Facility Management");
    expect(reportMock).toHaveBeenCalledTimes(2);

    reportMock.mockImplementation(async (query: Query) => {
      if (query.category === "all" && query.currency === "USD") {
        // A stale IDR snapshot returned for the USD request: it must be an
        // error, never rendered as zero under the USD caption.
        return reportFor({ ...query, currency: "IDR" });
      }
      return reportFor(query);
    });
    await choose("Currency", "USD");
    expect(reportMock).toHaveBeenCalledTimes(4);
    expect(document.body.textContent).toContain(
      "does not match this report's range, bucket, currency or all-category scope"
    );
    expect(document.querySelector("[data-category-metric]")).toBeNull();
    expect(document.querySelector("[data-category-row]")).toBeNull();
    expect(button("Export PDF").disabled).toBe(true);
    expect(button("Retry categories")).not.toBeNull();
  });

  it("shows a business-date notice and refuses export for a stale category snapshot", async () => {
    const onExport = vi.fn(async () => "/tmp/opencode/stale-category.pdf");
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage({ onExport });
    await choose("Category", "Facility Management");
    expect(reportMock).toHaveBeenCalledTimes(2);

    reportMock.mockImplementation(async (query: Query) => {
      if (query.category === "all" && query.currency === "USD") {
        const report = reportFor(query);
        report.asOf = "2026-11-20";
        return report;
      }
      return reportFor(query);
    });
    await choose("Currency", "USD");
    expect(reportMock).toHaveBeenCalledTimes(4);
    expect(document.body.textContent).toContain(
      "different business date from the overview"
    );
    expect(document.querySelector("[data-category-aggregate]")).not.toBeNull();
    expect(button("Export PDF").disabled).toBe(true);
    await exportPdf();
    expect(onExport).not.toHaveBeenCalled();
  });

  it("keeps the local category selection and makes zero requests on cached navigation", async () => {
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage();
    await openCategoryPicker();
    await clickPickerNode(categoryPickerKey("FAB"));
    await clickPickerNode(categoryClose());
    expect(
      categoryTrigger().getAttribute("data-category-selection-summary")
    ).toBe("Fabrication");

    await choose("Category", "Facility Management");
    expect(reportMock).toHaveBeenCalledTimes(2);
    expect(
      categoryTrigger().getAttribute("data-category-selection-summary")
    ).toBe("Fabrication");

    await choose("Currency", "USD");
    expect(reportMock).toHaveBeenCalledTimes(4);
    expect(
      categoryTrigger().getAttribute("data-category-selection-summary")
    ).toBe("Fabrication");
    expect(
      document.querySelector(
        '[data-category-card][data-category-key="FAB"] [data-category-metric="PO amount"]'
      )?.textContent
    ).toBe("$1,200,000.00");

    const before = reportMock.mock.calls.length;
    await act(async () => {
      button("Next category").click();
    });
    await flush();
    expect(reportMock.mock.calls.length).toBe(before);
    expect(
      document.querySelector(
        '[data-category-card][data-category-key="SUPPLY_GEN"]'
      )
    ).not.toBeNull();
    expect(
      document.querySelector('[data-category-card][data-category-key="FAB"]')
    ).toBeNull();

    // All remembers the display preference: Individual then Aggregate are pure
    // presentation over the same loaded snapshot.
    await openCategoryPicker();
    await clickPickerNode(categoryAll());
    await clickPickerNode(categoryClose());
    await act(async () => {
      button("Individual").click();
    });
    await flush();
    expect(reportMock.mock.calls.length).toBe(before);
    expect(document.querySelectorAll("[data-category-card]")).toHaveLength(5);
    await act(async () => {
      button("Aggregate").click();
    });
    await flush();
    expect(reportMock.mock.calls.length).toBe(before);
    expect(
      document.querySelector('[data-category-aggregate]')
    ).not.toBeNull();
    expect(document.querySelector("[data-category-card]")).toBeNull();
  });

  it("exports Aggregate, a subset, All/Individual and never a row sum", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/category.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => reportFor(query));
    await renderPage({ onExport });

    await exportPdf();
    expect(onExport).toHaveBeenCalledTimes(1);
    let view = onExport.mock.calls[0][0];
    expect(view.version).toBe(1);
    expect(view.categories).toEqual({
      source: 0,
      codes: FIVE_CATEGORY_CODES,
      layout: "aggregate",
    });
    expect(view.sources).toHaveLength(1);

    await openCategoryPicker();
    await clickPickerNode(categoryPickerKey("FAB"));
    await clickPickerNode(categoryClose());
    await exportPdf();
    view = onExport.mock.calls[1][0];
    expect(view.categories).toEqual({
      source: 0,
      codes: ["FAB"],
      layout: "individual",
    });

    await openCategoryPicker();
    await clickPickerNode(categoryPickerKey("FM"));
    await clickPickerNode(categoryClose());
    await exportPdf();
    view = onExport.mock.calls[2][0];
    expect(view.categories).toEqual({
      source: 0,
      codes: ["FM", "FAB"],
      layout: "individual",
    });

    await openCategoryPicker();
    await clickPickerNode(categoryAll());
    await clickPickerNode(categoryClose());
    await act(async () => {
      button("Individual").click();
    });
    await flush();
    const beforeNavigation = reportMock.mock.calls.length;
    await act(async () => {
      button("Next category").click();
    });
    await flush();
    expect(reportMock.mock.calls.length).toBe(beforeNavigation);
    expect(document.querySelectorAll("[data-category-card]")).toHaveLength(5);

    await exportPdf();
    view = onExport.mock.calls[3][0];
    expect(view.categories).toEqual({
      source: 0,
      codes: FIVE_CATEGORY_CODES,
      layout: "individual",
    });
    // The whole journey is the initial overview only: every category action is
    // local state over the already loaded all-category snapshot.
    expect(reportMock).toHaveBeenCalledTimes(1);
  });
});

// Issue #645: report freshness, bounded lifecycle and safe export.
//
// One shared source registry serves Performance and both comparison browsers.
// These tests pin the user-visible contract: a hidden page fetches nothing, a
// return revalidates only the scopes the screen needs, Refresh keeps every
// local selection, same-scope failures retain honestly-stale figures, a late
// old-scope reply never repaints a newer one, a WIB midnight (or a focus after
// sleep) revalidates without a polling interval, and an export freezes its
// envelope before the save starts.
describe("report freshness lifecycle (issue #645)", () => {
  const PINNED_TODAY = "2026-11-19";
  const ZERO = {
    poCount: 0,
    amountMinor: 0,
    costedMinor: 0,
    uncostedMinor: 0,
    extraExpenseMinor: 0,
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

  function annualYear(query: Query): number | null {
    if (query.bucket !== "month") return null;
    const match = /^(\d{4})-01-01$/.exec(query.from);
    if (match === null) return null;
    const year = Number(match[1]);
    return query.to === `${year + 1}-01-01` ? year : null;
  }

  function zeroAnnual(year: number, query: Query): PurchaseOrderReportData {
    const source = purchaseOrderReportYearFixture();
    source.filter = {
      from: `${year}-01-01`,
      to: `${year + 1}-01-01`,
      bucket: "month",
      category: query.category,
      currency: query.currency,
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
    const year = annualYear(query);
    if (year === 2026) {
      const report = purchaseOrderReportYearFixture();
      report.filter = { ...query };
      return report;
    }
    if (year !== null) return zeroAnnual(year, query);
    return reportFor(query);
  }

  const initial = toReportQuery(initialReportFilters(PINNED_TODAY, "IDR"));
  const thisYear = toReportQuery({
    ...initialReportFilters(PINNED_TODAY, "IDR"),
    preset: "this-year",
    period: null,
    ...reportPresetRange("this-year", PINNED_TODAY),
  });
  const thisYearFM: Query = { ...thisYear, category: "FM" };
  const annual2026FM: Query = {
    from: "2026-01-01",
    to: "2027-01-01",
    bucket: "month",
    category: "FM",
    currency: "IDR",
  };
  const annual2025All: Query = {
    from: "2025-01-01",
    to: "2026-01-01",
    bucket: "month",
    category: "all",
    currency: "IDR",
  };

  interface LifecycleOptions {
    active?: boolean;
    mutationEpoch?: number;
    onExport?: (view: PurchaseOrderReportView) => Promise<string>;
  }

  async function renderLifecycle(options: LifecycleOptions = {}): Promise<void> {
    await act(async () => {
      root.render(
        <ToastProvider>
          <PurchaseOrderReportsPage
            defaultCurrency="IDR"
            currencyCatalogue={CATALOGUE}
            onExport={options.onExport}
            active={options.active}
            mutationEpoch={options.mutationEpoch}
          />
        </ToastProvider>
      );
    });
    await flush();
  }

  function monthTrigger(): HTMLButtonElement {
    return button("Choose months");
  }

  function categoryTrigger(): HTMLButtonElement {
    return button("Choose categories");
  }

  function pickerNode(selector: string): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>(selector);
    if (found === null) throw new Error(`no picker node ${selector}`);
    return found;
  }

  async function clickNode(node: HTMLElement): Promise<void> {
    await act(async () => {
      node.click();
    });
    await flush();
  }

  async function selectMonth(
    month: string,
    previousPages = 0
  ): Promise<void> {
    await clickNode(monthTrigger());
    for (let index = 0; index < previousPages; index += 1) {
      await clickNode(button("Previous page"));
    }
    await clickNode(pickerNode(`[data-picker-key="${month}"]`));
    await clickNode(pickerNode("[data-picker-close]"));
  }

  async function selectCategory(code: string): Promise<void> {
    await clickNode(categoryTrigger());
    await clickNode(pickerNode(`[data-picker-key="${code}"]`));
    await clickNode(pickerNode("[data-picker-close]"));
  }

  function monthCard(month: string): HTMLElement {
    const found = document.querySelector<HTMLElement>(
      `[data-month-card][data-month-key="${month}"]`
    );
    if (found === null) throw new Error(`no month card ${month}`);
    return found;
  }

  function chartNodes(): Element[] {
    return [
      ...document.querySelectorAll("[data-chart-svg]"),
      ...document.querySelectorAll("[data-chart-svg] rect"),
      ...document.querySelectorAll("[data-chart-svg] path"),
      ...document.querySelectorAll("[data-chart-svg] text"),
    ];
  }

  it("makes one request on mount and none for cached selections, layout or pickers", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle();
    expect(calls()).toEqual([initial]);

    // An ordinary parent re-render is not a scope change.
    await renderLifecycle();
    expect(calls()).toEqual([initial]);

    const nodesBefore = chartNodes();
    const svgBefore = document.querySelector("[data-chart-svg]");
    expect(nodesBefore.length).toBeGreaterThan(0);

    await selectMonth("2026-10");
    await selectCategory("FAB");

    expect(calls()).toEqual([initial]);
    // Both local selections took effect without a read.
    expect(monthTrigger().getAttribute("data-month-selection-summary")).toBe(
      "2 months · 2026"
    );
    expect(
      categoryTrigger().getAttribute("data-category-selection-summary")
    ).toBe("Fabrication");
    // The chart is the same SVG and the same geometry nodes.
    expect(document.querySelector("[data-chart-svg]")).toBe(svgBefore);
    const nodesAfter = chartNodes();
    expect(nodesAfter).toHaveLength(nodesBefore.length);
    nodesAfter.forEach((node, index) => {
      expect(node).toBe(nodesBefore[index]);
    });
  });

  it("Refresh revalidates exactly the visible required scopes once and keeps both selections", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport: async () => "/tmp/opencode/645.pdf" });
    await choose("Date range preset", "This year");
    await choose("Category", "Facility Management");
    await selectMonth("2026-10");
    await selectCategory("FAB");

    // Four distinct scopes were already read: the initial annual overview,
    // the this-year overview, the restricted overview and its annual source.
    expect(calls()).toEqual([initial, thisYear, thisYearFM, annual2026FM]);
    const beforeRefresh = calls().length;

    await act(async () => {
      button("Refresh report").click();
    });
    await flush();

    // Exactly the three scopes this screen needs: Performance, the annual
    // source By period still shows, and the unrestricted all-category source.
    // Each appears once even though two cards and both surfaces share them.
    expect(calls().slice(beforeRefresh)).toEqual([
      thisYearFM,
      annual2026FM,
      thisYear,
    ]);
    expect(monthTrigger().getAttribute("data-month-selection-summary")).toBe(
      "2 months · 2026"
    );
    expect(
      categoryTrigger().getAttribute("data-category-selection-summary")
    ).toBe("Fabrication");
    expect(button("Refresh report").disabled).toBe(false);
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("a hidden page fetches nothing and a return revalidates once, deduplicated with a hidden mutation", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));

    // active=false at mount: no request at all.
    await renderLifecycle({ active: false });
    expect(reportMock).not.toHaveBeenCalled();

    await renderLifecycle({ active: true });
    expect(calls()).toEqual([initial]);

    // Give the screen a second required scope: a month outside the overview's
    // calendar year needs its own annual source.
    await selectMonth("2025-01", 1);
    expect(calls()).toEqual([initial, annual2025All]);
    const monthSummary = monthTrigger().getAttribute(
      "data-month-selection-summary"
    );

    // Hide and return with no mutation: one revalidation pass, no more.
    await renderLifecycle({ active: false });
    expect(calls()).toHaveLength(2);
    await renderLifecycle({ active: true });
    expect(calls().slice(2)).toEqual([initial, annual2025All]);
    expect(calls()).toHaveLength(4);

    // Hide, mutate, return: still one pass per required scope, never two.
    await renderLifecycle({ active: false });
    await renderLifecycle({ active: false, mutationEpoch: 1 });
    expect(calls()).toHaveLength(4);
    await renderLifecycle({ active: true, mutationEpoch: 1 });
    expect(calls().slice(4)).toEqual([initial, annual2025All]);
    expect(calls()).toHaveLength(6);
    expect(monthTrigger().getAttribute("data-month-selection-summary")).toBe(
      monthSummary
    );
  });

  it("marks a month card served by the shared overview as stale while it refreshes", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport: async () => "/tmp/opencode/645.pdf" });
    const card = monthCard("2026-11");
    expect(card.querySelector("[data-month-stale]")).toBeNull();

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(() => pending.promise);
    await act(async () => {
      button("Refresh report").click();
    });
    await flush();

    const badge = card.querySelector("[data-month-stale]");
    expect(badge).not.toBeNull();
    expect(badge?.textContent).toContain("Updating…");
    expect(
      document
        .querySelector("[data-month-browser]")
        ?.getAttribute("data-month-browser-stale")
    ).toBe("true");
    expect(button("Export PDF").disabled).toBe(true);

    act(() => {
      pending.resolve(respond(initial));
    });
    await flush();
    expect(card.querySelector("[data-month-stale]")).toBeNull();
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("a same-scope revalidation keeps each surface's figures labelled stale and offers its own Retry", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport: async () => "/tmp/opencode/645.pdf" });
    await choose("Date range preset", "This year");
    await choose("Category", "Facility Management");
    expect(document.body.textContent).toContain("Rp480.000.000");

    type Pending = {
      promise: Promise<PurchaseOrderReportData>;
      resolve: (value: PurchaseOrderReportData) => void;
      reject: (error: unknown) => void;
    };
    const scopeKey = (query: Query): string =>
      [query.from, query.to, query.bucket, query.category, query.currency].join(
        "|"
      );
    const replies = new Map<string, Pending>();
    reportMock.mockImplementation((query: Query) => {
      const pending = deferred<PurchaseOrderReportData>();
      replies.set(scopeKey(query), pending);
      return pending.promise;
    });
    await act(async () => {
      button("Refresh report").click();
    });
    await flush();

    // Three distinct sources are re-reading, and every surface says so while
    // keeping the figures it already had.
    expect(replies.size).toBe(3);
    expect(monthCard("2026-11").querySelector("[data-month-stale]")).not.toBeNull();
    expect(document.querySelector("[data-month-browser-stale]")).not.toBeNull();
    expect(
      document.querySelector("[data-month-browser]")?.getAttribute(
        "data-month-browser-refreshing"
      )
    ).toBe("true");
    expect(document.querySelector("[data-category-stale]")).not.toBeNull();
    expect(document.body.textContent).toContain("Rp480.000.000");
    expect(document.body.textContent).toContain("Rp120.000.000");
    expect(button("Export PDF").disabled).toBe(true);

    await act(async () => {
      for (const pending of replies.values()) {
        pending.reject(new Error("Network is unreachable."));
      }
    });
    await flush();

    expect(document.body.textContent).toContain("Network is unreachable.");
    expect(monthCard("2026-11").textContent).toContain("Rp190.000.000");
    expect(document.querySelector("[data-category-aggregate]")).not.toBeNull();
    expect(button("Retry month")).not.toBeNull();
    expect(button("Retry report")).not.toBeNull();
    expect(button("Retry categories")).not.toBeNull();
    expect(button("Export PDF").disabled).toBe(true);

    // Retry month re-reads only that month's annual source.
    reportMock.mockImplementation(async (query: Query) => respond(query));
    const beforeMonthRetry = calls().length;
    await act(async () => {
      button("Retry month").click();
    });
    await flush();
    expect(calls()).toHaveLength(beforeMonthRetry + 1);
    expect(calls().at(-1)).toEqual(annual2026FM);
    expect(monthCard("2026-11").querySelector("[data-month-stale]")).toBeNull();
    // Performance and the category browser are still honestly failed.
    expect(button("Export PDF").disabled).toBe(true);

    const beforeReportRetry = calls().length;
    await act(async () => {
      button("Retry report").click();
    });
    await flush();
    expect(calls()).toHaveLength(beforeReportRetry + 1);
    expect(calls().at(-1)).toEqual(thisYearFM);
    expect(button("Export PDF").disabled).toBe(true);

    const beforeCategoryRetry = calls().length;
    await act(async () => {
      button("Retry categories").click();
    });
    await flush();
    expect(calls()).toHaveLength(beforeCategoryRetry + 1);
    expect(calls().at(-1)).toEqual(thisYear);
    expect(document.body.textContent).not.toContain("Network is unreachable.");
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("labels the annual aggregate stale and keeps its recorded total on failure", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport: async () => "/tmp/opencode/645.pdf" });
    // A relative preset makes the overview non-annual, so the Aggregate has a
    // distinct annual source of its own.
    await choose("Date range preset", "This year");
    await choose("Category", "Facility Management");
    // All twelve 2026 months in Aggregate read the restricted annual source.
    await clickNode(monthTrigger());
    await clickNode(pickerNode("[data-picker-all]"));
    await clickNode(pickerNode("[data-picker-close]"));
    const aggregate = document.querySelector<HTMLElement>(
      "[data-month-aggregate]"
    );
    if (aggregate === null) throw new Error("no annual aggregate");
    expect(aggregate.textContent).toContain("Recorded total");

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(() => pending.promise);
    await act(async () => {
      button("Refresh report").click();
    });
    await flush();

    expect(
      aggregate.querySelector("[data-month-aggregate-stale]")
    ).not.toBeNull();
    expect(
      aggregate
        .querySelector("[data-month-aggregate-stale]")
        ?.getAttribute("data-month-aggregate-refreshing")
    ).toBe("true");
    expect(button("Export PDF").disabled).toBe(true);

    await act(async () => {
      pending.reject(new Error("The annual API is unreachable."));
    });
    await flush();

    expect(aggregate.textContent).toContain(
      "This annual snapshot could not be refreshed"
    );
    expect(aggregate.textContent).toContain("Recorded total");
    expect(button("Retry month")).not.toBeNull();
    expect(button("Export PDF").disabled).toBe(true);
  });

  it("does not stale-mark an independent annual aggregate from a current-month overview (NB1)", async () => {
    // January is the only month where the annual aggregate's first month is
    // also the current month. Performance on Current month (bucket "day") then
    // looks like the aggregate's owner, but the aggregate is backed by its own
    // annual registry entry. Only that entry's freshness may label it.
    vi.setSystemTime(new Date("2027-01-15T03:00:00Z"));
    function januaryDay(query: Query): PurchaseOrderReportData {
      const report = purchaseOrderReportMonthFixture();
      report.filter = { ...query };
      report.asOf = "2027-01-15";
      const summary = report.monthSummary;
      if (summary !== undefined) {
        report.monthSummary = {
          ...summary,
          month: "2027-01",
          from: "2027-01-01",
          to: "2027-02-01",
          asOf: "2027-01-15",
        };
      }
      return report;
    }
    reportMock.mockImplementation(async (query: Query) => {
      if (query.bucket === "day") return januaryDay(query);
      const annual = zeroAnnual(2027, query);
      annual.asOf = "2027-01-15";
      return annual;
    });
    await renderLifecycle({ onExport: async () => "/tmp/opencode/645.pdf" });

    // All twelve 2027 months in Aggregate, then Current month makes the
    // overview a day report so the aggregate needs its own annual source.
    await clickNode(monthTrigger());
    await clickNode(pickerNode("[data-picker-all]"));
    await clickNode(pickerNode("[data-picker-close]"));
    await clickNode(
      document.querySelector<HTMLElement>('button[data-report-period="month"]')!
    );
    const aggregate = document.querySelector<HTMLElement>(
      "[data-month-aggregate]"
    );
    if (aggregate === null) throw new Error("no annual aggregate");
    expect(
      aggregate.querySelector("[data-month-aggregate-stale]")
    ).toBeNull();

    // Edit the Performance range only. The annual source stays cached and
    // fresh while the overview re-reads; the aggregate must not inherit that
    // overview staleness.
    await clickNode(
      document.querySelector<HTMLElement>('button[data-report-period="custom"]')!
    );
    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(() => pending.promise);
    await typeDate("From date", "2027-01-10");
    await flush();

    expect(document.querySelector("[data-report-overview-stale]")).not.toBeNull();
    expect(
      aggregate.querySelector("[data-month-aggregate-stale]")
    ).toBeNull();

    const last = calls().at(-1);
    if (last === undefined) throw new Error("no pending overview query");
    await act(async () => {
      pending.resolve(
        last.bucket === "day"
          ? januaryDay(last)
          : zeroAnnual(2027, last)
      );
    });
    await flush();
    expect(
      aggregate.querySelector("[data-month-aggregate-stale]")
    ).toBeNull();
  });

  it("isolates a failed annual month source from Performance and the categories", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport: async () => "/tmp/opencode/645.pdf" });
    await selectMonth("2025-01", 1);
    expect(calls()).toEqual([initial, annual2025All]);

    reportMock.mockImplementation(async (query: Query) => {
      if (query.from === annual2025All.from && query.to === annual2025All.to) {
        throw new Error("The 2025 annual API is unreachable.");
      }
      return respond(query);
    });
    await act(async () => {
      button("Refresh report").click();
    });
    await flush();

    const failed = monthCard("2025-01");
    expect(failed.textContent).toContain("The 2025 annual API is unreachable.");
    expect(failed.querySelector("[data-month-stale]")).not.toBeNull();
    expect(failed.textContent).toContain("Rp0");
    // Sibling surfaces are untouched and still show their evidence.
    expect(
      document.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).toContain("Rp641.200.000");
    expect(document.querySelector("[data-category-aggregate]")).not.toBeNull();
    expect(
      document.querySelector('[data-category-row="FAB"]')?.textContent
    ).toContain("Rp128.240.000");
    expect(monthCard("2026-11").querySelector("[data-month-stale]")).toBeNull();
    expect(button("Export PDF").disabled).toBe(true);

    const beforeRetry = calls().length;
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await act(async () => {
      button("Retry month").click();
    });
    await flush();
    expect(calls()).toHaveLength(beforeRetry + 1);
    expect(calls().at(-1)).toEqual(annual2025All);
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("never lets a late IDR reply repaint a USD view", async () => {
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport: async () => "/tmp/opencode/usd.pdf" });
    expect(calls()).toEqual([initial]);

    // A second IDR scope is in flight when the user switches to USD: the
    // yearly IDR read still belongs to the previous view.
    const pendingIDR = deferred<PurchaseOrderReportData>();
    const yearlyIDR: Query = { ...initial, bucket: "year" };
    reportMock.mockImplementation((query: Query) =>
      query.bucket === "year" && query.currency === "IDR"
        ? pendingIDR.promise
        : Promise.resolve(respond(query))
    );
    await choose("Bucket", "Year");
    expect(calls().at(-1)).toEqual(yearlyIDR);

    await choose("Currency", "USD");
    expect(calls()).toContainEqual({ ...yearlyIDR, currency: "USD" });
    expect(document.body.textContent).toContain("$4,800,000.00");
    expect(button("Export PDF").disabled).toBe(false);
    const beforeLate = calls().length;

    act(() => {
      pendingIDR.resolve(respond(yearlyIDR));
    });
    await flush();

    expect(document.body.textContent).toContain("$4,800,000.00");
    expect(document.body.textContent).not.toContain("Rp480.000.000");
    expect(button("Export PDF").disabled).toBe(false);
    expect(calls()).toHaveLength(beforeLate);
  });

  it("keeps explicit empty sections through Refresh and exports them as empty", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/empty.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport });

    // Remove the only selected month and every category.
    await selectMonth("2026-11");
    await selectCategory("FAB");
    await selectCategory("FAB");
    expect(document.querySelector("[data-month-empty]")).not.toBeNull();
    expect(document.querySelector("[data-category-empty]")).not.toBeNull();

    const beforeRefresh = calls().length;
    await act(async () => {
      button("Refresh report").click();
    });
    await flush();
    // No financial source is required for empty sections; only Performance
    // revalidates.
    expect(calls().slice(beforeRefresh)).toEqual([initial]);
    expect(document.querySelector("[data-month-empty]")).not.toBeNull();
    expect(document.querySelector("[data-category-empty]")).not.toBeNull();

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(onExport).toHaveBeenCalledTimes(1);
    const view = onExport.mock.calls[0][0] as PurchaseOrderReportView;
    expect(view.periods).toEqual({
      sources: [],
      months: [],
      layout: "individual",
    });
    expect(view.categories).toEqual({
      source: 0,
      codes: [],
      layout: "individual",
    });
    expect(view.sources).toHaveLength(1);
  });

  it("freezes the export envelope against a concurrent Refresh and selection change", async () => {
    const save = deferred<string>();
    const onExport = vi.fn((_view: PurchaseOrderReportView) => save.promise);
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport });
    const original = respond(initial);
    expect(document.body.textContent).toContain("Rp641.200.000");

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(onExport).toHaveBeenCalledTimes(1);
    const frozen = onExport.mock.calls[0][0];
    expect(frozen.sources[0].totals.amountMinor).toBe(
      original.totals.amountMinor
    );
    expect(frozen.periods.months).toEqual(["2026-11"]);

    // A Refresh and local selection changes start while the save is pending.
    const refreshReplies = new Map<
      string,
      {
        promise: Promise<PurchaseOrderReportData>;
        resolve: (value: PurchaseOrderReportData) => void;
        reject: (error: unknown) => void;
      }
    >();
    const refreshQueries = new Map<string, Query>();
    reportMock.mockImplementation((query: Query) => {
      const pending = deferred<PurchaseOrderReportData>();
      const key = [
        query.from,
        query.to,
        query.bucket,
        query.category,
        query.currency,
      ].join("|");
      refreshReplies.set(key, pending);
      refreshQueries.set(key, query);
      return pending.promise;
    });
    await act(async () => {
      button("Refresh report").click();
    });
    await flush();
    await selectMonth("2026-10");
    await selectCategory("FAB");

    expect(button("Refresh report").disabled).toBe(true);
    expect(button("Export PDF").disabled).toBe(true);

    // The refresh answers with decisively different numbers.
    act(() => {
      for (const [key, pending] of refreshReplies) {
        const query = refreshQueries.get(key);
        if (query === undefined) throw new Error("no refresh query");
        const report = respond(query);
        report.totals = { ...report.totals, amountMinor: 999_000_000 };
        pending.resolve(report);
      }
    });
    await flush();
    expect(document.body.textContent).toContain("Rp999.000.000");

    act(() => {
      save.resolve("/tmp/opencode/frozen.pdf");
    });
    await flush();

    // The document being saved is still the envelope captured at click time.
    expect(frozen.sources[0].totals.amountMinor).toBe(
      original.totals.amountMinor
    );
    expect(frozen.periods.months).toEqual(["2026-11"]);
    expect(frozen.categories.codes).toEqual(
      original.categories.map((row) => row.businessCategory)
    );
    expect(document.body.textContent).toContain(
      "Report saved to /tmp/opencode/frozen.pdf"
    );
  });

  it("refuses a direct export call while the visible evidence is stale (NB3)", async () => {
    const onExport = vi.fn(
      async (_view: PurchaseOrderReportView) => "/tmp/opencode/guarded.pdf"
    );
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderLifecycle({ onExport });

    // Explicit empty sections need no financial source, so both selections
    // can stay resolvable while the overview re-reads. That is the one state
    // where the button is disabled but the handler's own gate is the only
    // thing refusing a stale Performance snapshot.
    await selectMonth("2026-11");
    await selectCategory("FAB");
    await selectCategory("FAB");
    expect(document.querySelector("[data-month-empty]")).not.toBeNull();
    expect(document.querySelector("[data-category-empty]")).not.toBeNull();

    const pending = deferred<PurchaseOrderReportData>();
    reportMock.mockImplementation(() => pending.promise);
    await act(async () => {
      button("Refresh report").click();
    });
    await flush();
    expect(button("Export PDF").disabled).toBe(true);

    const exportButton = button("Export PDF");
    // React suppresses a real click on a button whose *prop* is disabled, so
    // the only way to reach the handler is the programmatic path the guard
    // exists for: the component's own onClick closure.
    const propsKey = Object.keys(exportButton).find((candidate) =>
      candidate.startsWith("__reactProps$")
    );
    if (propsKey === undefined) throw new Error("no React props on the button");
    const onClick = (
      exportButton as unknown as Record<string, { onClick?: () => void }>
    )[propsKey].onClick;
    if (onClick === undefined) throw new Error("no click handler");
    act(() => {
      onClick();
    });
    await flush();

    expect(onExport).not.toHaveBeenCalled();
    await act(async () => {
      pending.resolve(respond(initial));
    });
    await flush();
  });
});

// Issue #645 Q645-5: one WIB-midnight timeout while active, a focus/visibility
// recheck after sleep, and a return from hidden. Time-dependent evidence is
// revalidated; a relative control rebases to the new WIB date; custom dates and
// explicit local month/category selections are never rewritten.
describe("WIB midnight freshness (issue #645)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  async function settle(): Promise<void> {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
  }

  function annualYear(query: Query): number | null {
    if (query.bucket !== "month") return null;
    const match = /^(\d{4})-01-01$/.exec(query.from);
    if (match === null) return null;
    const year = Number(match[1]);
    return query.to === `${year + 1}-01-01` ? year : null;
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
    if (annualYear(query) === 2026) {
      const report = purchaseOrderReportYearFixture();
      report.filter = { ...query };
      return report;
    }
    return reportFor(query);
  }

  interface WibOptions {
    active?: boolean;
    mutationEpoch?: number;
  }

  async function renderWib(options: WibOptions = {}): Promise<void> {
    await act(async () => {
      root.render(
        <ToastProvider>
          <PurchaseOrderReportsPage
            defaultCurrency="IDR"
            currencyCatalogue={CATALOGUE}
            active={options.active}
            mutationEpoch={options.mutationEpoch}
          />
        </ToastProvider>
      );
    });
    await settle();
  }

  async function clickNode(node: HTMLElement): Promise<void> {
    await act(async () => {
      node.click();
      await Promise.resolve();
    });
    await settle();
  }

  async function chooseLocal(control: string, option: string): Promise<void> {
    if (
      (control === "Bucket" || control === "Date range preset") &&
      document.querySelector(`button[aria-label="${control}"]`) === null
    ) {
      await clickNode(
        document.querySelector<HTMLElement>('button[data-report-period="custom"]')!
      );
    }
    await clickNode(button(control));
    const found = Array.from(
      document.querySelectorAll<HTMLElement>('[role="option"]')
    ).find((node) => node.textContent?.trim() === option);
    if (found === undefined) throw new Error(`no option ${option} in ${control}`);
    await clickNode(found);
  }

  function pickerNode(selector: string): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>(selector);
    if (found === null) throw new Error(`no picker node ${selector}`);
    return found;
  }

  async function selectMonth(month: string): Promise<void> {
    await clickNode(button("Choose months"));
    await clickNode(pickerNode(`[data-picker-key="${month}"]`));
    await clickNode(pickerNode("[data-picker-close]"));
  }

  async function selectCategory(code: string): Promise<void> {
    await clickNode(button("Choose categories"));
    await clickNode(pickerNode(`[data-picker-key="${code}"]`));
    await clickNode(pickerNode("[data-picker-close]"));
  }

  it("revalidates once across WIB midnight with a single timeout, keeping explicit selections", async () => {
    vi.setSystemTime(new Date("2026-11-19T16:59:55Z"));
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderWib();

    const initial = calls()[0];
    expect(calls()).toHaveLength(1);

    await selectMonth("2026-10");
    await selectCategory("FAB");
    expect(calls()).toHaveLength(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    await settle();

    // The single midnight timeout re-reads the required overview exactly once;
    // the month and category sources share it at this scope.
    expect(calls().slice(1)).toEqual([initial]);
    expect(
      button("Choose months").getAttribute("data-month-selection-summary")
    ).toBe("2 months · 2026");
    expect(
      button("Choose categories").getAttribute(
        "data-category-selection-summary"
      )
    ).toBe("Fabrication");

    // No polling: a minute of idle time after the boundary reads nothing more.
    const afterMidnight = calls().length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });
    await settle();
    expect(calls()).toHaveLength(afterMidnight);
  });

  it("never rewrites a custom date range at midnight", async () => {
    vi.setSystemTime(new Date("2026-11-30T16:59:55Z"));
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderWib();
    const initial = calls()[0];

    await clickNode(
      document.querySelector<HTMLElement>('button[data-report-period="custom"]')!
    );
    expect(calls()).toHaveLength(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    await settle();

    // WIB moved to 1 December, but the user's own range is untouched: the
    // revalidation repeats the same query instead of moving it.
    expect(calls()).toEqual([initial, initial]);
  });

  it("rebases a relative 'This year' preset across New Year", async () => {
    vi.setSystemTime(new Date("2026-12-31T16:59:55Z"));
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderWib();
    expect(calls()).toEqual([
      toReportQuery(initialReportFilters("2026-12-31", "IDR")),
    ]);

    await chooseLocal("Date range preset", "This year");
    expect(calls()).toHaveLength(1);
    await selectMonth("2026-10");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    await settle();

    // The rebased 2027 relative range is a new scope; the still-selected
    // November 2026 card needs its own annual source, which the invalidation
    // made stale. Both are fetched once, in surface order.
    expect(calls().slice(1)).toEqual([
      {
        from: "2027-01-01",
        to: "2027-01-02",
        bucket: "month",
        category: "all",
        currency: "IDR",
      },
      toReportQuery(initialReportFilters("2026-12-31", "IDR")),
    ]);
    // The explicit local month selection is not rewritten by the rebase.
    expect(
      button("Choose months").getAttribute("data-month-selection-summary")
    ).toBe("2 months · 2026");
  });

  it("rebases the current-month toggle to the new month", async () => {
    vi.setSystemTime(new Date("2026-11-30T16:59:55Z"));
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderWib();

    await clickNode(button("Month view"));
    expect(calls()).toHaveLength(2);
    expect(calls()[1]).toMatchObject({
      from: "2026-11-01",
      to: "2026-12-01",
      bucket: "day",
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    await settle();

    // The toggle rebases to the new WIB month. The category section still
    // shows the inherited previous scope until the new overview lands, so that
    // scope is revalidated too; the retained November section then refreshes
    // its own annual source. Each is read once.
    expect(calls().slice(2)).toEqual([
      {
        from: "2026-12-01",
        to: "2027-01-01",
        bucket: "day",
        category: "all",
        currency: "IDR",
      },
      {
        from: "2026-11-01",
        to: "2026-12-01",
        bucket: "day",
        category: "all",
        currency: "IDR",
      },
      toReportQuery(initialReportFilters("2026-11-30", "IDR")),
    ]);
  });

  it("waits while hidden, then rebases and revalidates on return", async () => {
    vi.setSystemTime(new Date("2026-12-31T16:59:55Z"));
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderWib();
    await chooseLocal("Date range preset", "This year");
    expect(calls()).toHaveLength(1);

    await renderWib({ active: false });
    const hiddenCalls = calls().length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    await settle();
    expect(calls()).toHaveLength(hiddenCalls);

    await renderWib({ active: true });
    expect(calls().slice(hiddenCalls)).toEqual([
      {
        from: "2027-01-01",
        to: "2027-01-02",
        bucket: "month",
        category: "all",
        currency: "IDR",
      },
      toReportQuery(initialReportFilters("2026-12-31", "IDR")),
    ]);
  });

  it("checks the WIB date on focus and on a visible tab after sleep", async () => {
    vi.setSystemTime(new Date("2026-11-19T16:59:55Z"));
    reportMock.mockImplementation(async (query: Query) => respond(query));
    await renderWib();
    const initial = calls()[0];

    // The machine slept past midnight: no timeout fired, then the window is
    // focused.
    vi.setSystemTime(new Date("2026-11-20T00:00:05Z"));
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
      await Promise.resolve();
    });
    await settle();
    expect(calls().slice(1)).toEqual([initial]);

    // A second day passes behind a hidden tab; the tab becomes visible again.
    vi.setSystemTime(new Date("2026-11-21T00:00:05Z"));
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await Promise.resolve();
    });
    await settle();
    expect(calls().slice(2)).toEqual([initial]);
  });
});
