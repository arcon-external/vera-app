// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderReportData } from "../poReport";
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
  type ReportFilterState,
} from "../poReportPage";
import {
  PurchaseOrderReportOverview,
  type PurchaseOrderReportOverviewProps,
} from "./PurchaseOrderReportOverview";

// Issue #644: the pure Overview is the upper Reports workspace — controls,
// actual KPIs, coverage and the embedded Performance chart. These tests drive
// it directly, so they pin the composition without the live page (#641 owns
// that wiring) and cannot move the report-wide query.

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
const TODAY = "2026-11-19";
const YEAR_FILTERS = initialReportFilters(TODAY, "IDR");
const MONTH_FILTERS: ReportFilterState = {
  ...initialReportFilters(TODAY, "IDR"),
  preset: "custom",
  period: "month",
  ...reportPeriodRange("month", TODAY),
  bucket: "day",
};
const HISTORICAL_FILTERS: ReportFilterState = {
  preset: "last-year",
  period: null,
  from: "2025-01-01",
  to: "2025-12-31",
  bucket: "month",
  category: "all",
  currency: "IDR",
};
const BASE_FILTERS: ReportFilterState = {
  preset: "custom",
  period: null,
  from: "2026-01-01",
  to: "2026-11-19",
  bucket: "month",
  category: "all",
  currency: "IDR",
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  reportMock.mockReset();
  Element.prototype.scrollIntoView = () => undefined;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    right: 960,
    bottom: 300,
    width: 960,
    height: 300,
    toJSON: () => ({}),
  });
  window.matchMedia = ((query: string) => ({
    matches: query.includes("prefers-reduced-motion"),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function props(
  overrides: Partial<PurchaseOrderReportOverviewProps> = {}
): PurchaseOrderReportOverviewProps {
  return {
    filters: YEAR_FILTERS,
    report: purchaseOrderReportYearFixture(),
    catalogue: CATALOGUE,
    loading: false,
    stale: false,
    validation: "",
    onPresetChange: vi.fn(),
    onPeriodChange: vi.fn(),
    onFromChange: vi.fn(),
    onToChange: vi.fn(),
    onBucketChange: vi.fn(),
    onCategoryChange: vi.fn(),
    onCurrencyChange: vi.fn(),
    ...overrides,
  };
}

async function render(
  overrides: Partial<PurchaseOrderReportOverviewProps> = {}
): Promise<PurchaseOrderReportOverviewProps> {
  const value = props(overrides);
  await act(async () => {
    root.render(<PurchaseOrderReportOverview {...value} />);
  });
  await flush();
  return value;
}

function overview(): HTMLElement {
  const found = container.querySelector<HTMLElement>("[data-report-overview]");
  if (found === null) throw new Error("no report overview");
  return found;
}

function button(label: string): HTMLButtonElement {
  const found = overview().querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

async function choose(control: string, option: string): Promise<void> {
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
  const input = overview().querySelector<HTMLInputElement>(
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
  await flush();
}

/**
 * Text a sighted user can read in the default state: collapsed Details and
 * screen-reader-only prose are excluded, so a hidden method paragraph cannot
 * satisfy an "always visible" assertion by accident.
 */
function visibleText(element: HTMLElement): string {
  const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      const parent = node.parentElement;
      if (parent === null) return NodeFilter.FILTER_REJECT;
      if (parent.closest("details:not([open])") !== null) {
        return NodeFilter.FILTER_REJECT;
      }
      if (
        parent.closest("script, style, [aria-hidden='true'], .sr-only, svg") !==
        null
      ) {
        return NodeFilter.FILTER_REJECT;
      }
      return NodeFilter.FILTER_ACCEPT;
    },
  });
  const parts: string[] = [];
  while (walker.nextNode()) parts.push(walker.currentNode.textContent ?? "");
  return parts.join(" ");
}

function emptyReport(): PurchaseOrderReportData {
  const report = purchaseOrderReportYearFixture();
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
  report.categorySeries = [];
  report.notes = [];
  return report;
}

function historicalReport(): PurchaseOrderReportData {
  const report = purchaseOrderReportFixture();
  report.filter = {
    from: "2025-01-01",
    to: "2026-01-01",
    bucket: "month",
    category: "all",
    currency: "IDR",
  };
  return report;
}

function kpiValue(label: string): string {
  const grid = overview().querySelector('[aria-label="Report KPIs"]');
  const cell = Array.from(grid?.querySelectorAll("div") ?? []).find(
    (node) => node.querySelector("dt")?.textContent === label
  );
  return cell?.querySelector("dd")?.textContent ?? "";
}

describe("PurchaseOrderReportOverview (issue #644)", () => {
  it("groups one period toggle, actual KPIs, coverage and the chart with one visible forecast status", async () => {
    const value = await render();
    const view = overview();

    expect(view.querySelectorAll("[data-report-period-toggle]")).toHaveLength(1);
    expect(button("Month view").getAttribute("aria-pressed")).toBe("false");
    expect(button("Year view").getAttribute("aria-pressed")).toBe("true");
    expect(button("Custom date range").getAttribute("aria-pressed")).toBe(
      "false"
    );

    const kpi = view.querySelector('[aria-label="Report KPIs"]');
    expect(kpi).not.toBeNull();
    for (const label of [
      "PO count",
      "PO amount",
      "Capital",
      "PO margin",
      "PO margin %",
    ]) {
      expect(kpi?.textContent).toContain(label);
    }
    expect(kpi?.textContent).toContain("Rp641.200.000");
    expect(kpi?.textContent).toContain("Rp518.600.000");
    expect(kpi?.textContent).toContain("Rp122.600.000");
    expect(kpi?.textContent).toContain("23.64%");
    expect(
      view.querySelector("[data-report-period-context]")?.textContent
    ).toBe("Current year to date");

    const scope = view.querySelector("[data-report-overview-scope]");
    expect(scope?.textContent).toContain(reportRangeLabel(value.report!.filter));
    expect(scope?.textContent).toContain("All categories");
    expect(scope?.textContent).toContain("IDR");
    expect(scope?.textContent).toContain("As of 19 Nov 2026");

    expect(view.querySelectorAll("[data-report-chart]")).toHaveLength(1);
    expect(view.querySelector("[data-report-chart-embedded]")).not.toBeNull();
    expect(view.querySelectorAll("[data-report-forecast-status]")).toHaveLength(
      1
    );
    const status = view.querySelector("[data-report-forecast-status]");
    expect(status?.textContent).toContain("Projected");
    expect(status?.textContent).toContain("Year view, monthly buckets");
    expect(status?.closest("details")).toBeNull();
    expect(view.querySelectorAll('[aria-label="Chart legend"]')).toHaveLength(1);
    expect(view.querySelector("[data-chart-svg]")).not.toBeNull();

    // This coherent year fixture is fully costed: no denominator-less or
    // invented 0% coverage sentence is allowed here. The visible-coverage case
    // is exercised against the month fixture below.
    expect(view.querySelector("[data-report-coverage]")).toBeNull();

    const details = view.querySelector<HTMLDetailsElement>("details");
    expect(details).not.toBeNull();
    expect(details?.open).toBe(false);
    expect(details?.querySelector("summary")?.textContent).toContain(
      "Method and chart details"
    );

    const shown = visibleText(view);
    for (const phrase of [
      "Basis:",
      "Method:",
      "Projected periods follow",
      "never included in actual totals",
      "The hatched part",
      "hatch",
      "No margin percentage",
      "Updates as data arrives",
      "never enter the KPIs",
      "Sample",
    ]) {
      expect(shown, `unexpected always-visible prose: ${phrase}`).not.toContain(
        phrase
      );
    }

    const detailText = details?.textContent ?? "";
    expect(detailText).toContain("Basis:");
    expect(detailText).toContain("Method:");
    expect(detailText).toContain("Updates as data arrives");
    expect(detailText).toContain("Rest of this month");
    expect(view.querySelectorAll("details")).toHaveLength(1);
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("keeps the exact report and chart marks through Details and unrelated parent re-renders", async () => {
    const value = await render();
    const view = overview();
    const marks = view.querySelector("[data-chart-marks]");
    const motion = marks?.getAttribute("data-chart-motion");
    expect(marks).not.toBeNull();

    const details = view.querySelector<HTMLDetailsElement>("details");
    await act(async () => {
      details?.querySelector("summary")?.click();
    });
    await flush();

    expect(details?.open).toBe(true);
    expect(view.querySelector("[data-chart-marks]")).toBe(marks);
    expect(marks?.getAttribute("data-chart-motion")).toBe(motion);

    // A live filter change re-renders the overview with the previous snapshot
    // still on screen. The chart must keep that exact object and its marks.
    await act(async () => {
      root.render(
        <PurchaseOrderReportOverview
          {...value}
          filters={MONTH_FILTERS}
          loading
          stale
        />
      );
    });
    await flush();

    expect(view.querySelector("[data-chart-marks]")).toBe(marks);
    expect(marks?.getAttribute("data-chart-motion")).toBe(motion);
    expect(
      view.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).toContain("Rp641.200.000");
    expect(view.querySelector("[data-report-overview-stale]")).not.toBeNull();
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("keeps the controls and an honest empty message without a zero KPI or chart strip", async () => {
    const report = emptyReport();
    await render({ report });
    const view = overview();

    expect(view.querySelectorAll("[data-report-period-toggle]")).toHaveLength(1);
    expect(button("Month view")).not.toBeNull();
    expect(button("Year view")).not.toBeNull();
    expect(button("Custom date range")).not.toBeNull();
    expect(view.querySelector('[aria-label="Category"]')).not.toBeNull();
    expect(view.querySelector("[data-report-current-period]")).not.toBeNull();

    expect(view.querySelector("[data-report-overview-empty]")).not.toBeNull();
    expect(visibleText(view)).toContain("No purchase orders in this range");
    expect(visibleText(view)).toContain(reportRangeLabel(report.filter));
    expect(view.querySelector('[aria-label="Report KPIs"]')).toBeNull();
    expect(view.querySelector("[data-report-chart]")).toBeNull();
    expect(view.querySelector("[data-report-forecast-status]")).toBeNull();
    expect(view.querySelector("[data-report-coverage]")).toBeNull();
    expect(view.querySelector("details")).toBeNull();
    expect(visibleText(view)).not.toContain("PO amount");
    expect(view.querySelector("[data-report-overview-updating]")).toBeNull();
    // #653 NB1: Pipeline is period/category independent, so the two cards
    // stay visible even when the selected Actual period has no saved POs.
    expect(view.querySelector("[data-report-pipeline-card]")).not.toBeNull();
    expect(view.querySelector("[data-report-variance-card]")).not.toBeNull();
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("keeps operational controls through first load, validation and the error-related empty shell", async () => {
    await render({ report: null, loading: true });
    let view = overview();
    expect(view.querySelectorAll("[data-report-period-toggle]")).toHaveLength(1);
    expect(view.querySelector('[aria-label="Loading report"]')).not.toBeNull();
    expect(button("Category")).not.toBeNull();
    expect(view.querySelector('[aria-label="Report KPIs"]')).toBeNull();
    expect(view.querySelector("[data-report-chart]")).toBeNull();

    await render({ report: null, loading: false });
    view = overview();
    expect(view.querySelectorAll("[data-report-period-toggle]")).toHaveLength(1);
    expect(view.querySelector('[aria-label="Loading report"]')).toBeNull();
    expect(view.querySelector('[aria-label="Report KPIs"]')).toBeNull();
    expect(view.querySelector("[data-report-chart]")).toBeNull();
    expect(visibleText(view)).not.toContain("PO amount");
    expect(visibleText(view)).not.toContain("Rp0");

    await render({
      report: null,
      loading: false,
      validation: "Choose a valid start date.",
    });
    view = overview();
    expect(view.querySelectorAll("[data-report-period-toggle]")).toHaveLength(1);
    expect(visibleText(view)).toContain("Choose a valid start date.");
    expect(view.querySelector('[aria-label="Report KPIs"]')).toBeNull();
    expect(view.querySelector("[data-report-chart]")).toBeNull();
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("keeps cost coverage visible outside Details only when the amount is uncosted", async () => {
    await render({
      filters: BASE_FILTERS,
      report: purchaseOrderReportFixture(),
    });
    let view = overview();
    const coverage = view.querySelector("[data-report-coverage]");
    expect(coverage).not.toBeNull();
    expect(coverage?.closest("details")).toBeNull();
    expect(coverage?.textContent).toContain("90.00%");
    expect(coverage?.textContent).toContain("Rp48.000.000");
    expect(coverage?.textContent).toContain("Rp480.000.000");

    const fullyCosted = purchaseOrderReportYearFixture();
    await render({ report: fullyCosted });
    view = overview();
    expect(view.querySelector("[data-report-coverage]")).toBeNull();
    expect(visibleText(view)).not.toContain("Coverage:");
    expect(visibleText(view)).not.toContain("0.00% coverage");
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("keeps the displayed snapshot's scope and period while a different filter loads", async () => {
    const report = purchaseOrderReportYearFixture();
    await render({
      report,
      filters: MONTH_FILTERS,
      loading: true,
      stale: true,
    });
    const view = overview();

    const scope = view.querySelector("[data-report-overview-scope]");
    expect(scope?.textContent).toContain(reportRangeLabel(report.filter));
    expect(scope?.textContent).not.toContain("1 Nov 2026");
    expect(
      view.querySelector("[data-report-period-context]")?.textContent
    ).toBe("Current year to date");
    expect(
      view.querySelector("[data-report-overview-stale]")?.textContent
    ).toContain("Previous figures");
    expect(
      view.querySelector("[data-report-overview-updating]")?.textContent
    ).toContain("Updating the report");
    expect(
      view.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).toContain("Rp641.200.000");
    expect(
      view.querySelector('[aria-label="Report KPIs"]')?.textContent
    ).not.toContain("Rp190.000.000");
    expect(
      view.querySelector("[data-report-forecast-status]")?.textContent
    ).toContain("Year view, monthly buckets");

    expect(button("Month view").getAttribute("aria-pressed")).toBe("true");
    expect(button("Year view").getAttribute("aria-pressed")).toBe("false");
    expect(
      view.querySelector("[data-report-current-period]")?.textContent
    ).toContain("Current month");
    expect(
      view.querySelector("[data-report-current-period]")?.textContent
    ).toContain("1 Nov 2026");
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("delegates every report-wide control and the one period toggle exactly", async () => {
    const value = await render({
      filters: HISTORICAL_FILTERS,
      report: historicalReport(),
    });

    await act(async () => button("Month view").click());
    expect(value.onPeriodChange).toHaveBeenCalledWith("month");
    await act(async () => button("Custom date range").click());
    expect(value.onPeriodChange).toHaveBeenLastCalledWith(null);

    await choose("Date range preset", "This year");
    expect(value.onPresetChange).toHaveBeenCalledWith("this-year");

    await typeDate("From date", "2025-12-01");
    expect(value.onFromChange).toHaveBeenCalledWith("2025-12-01");
    await typeDate("To date", "2026-12-31");
    expect(value.onToChange).toHaveBeenCalledWith("2026-12-31");

    await choose("Bucket", "Year");
    expect(value.onBucketChange).toHaveBeenCalledWith("year");
    await choose("Category", "Unassigned");
    expect(value.onCategoryChange).toHaveBeenCalledWith("UNASSIGNED");
    await choose("Currency", "USD");
    expect(value.onCurrencyChange).toHaveBeenCalledWith("USD");
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("keeps unresolved currency honest: no substituted amount, coverage still readable", async () => {
    await render({
      filters: BASE_FILTERS,
      report: purchaseOrderReportFixture(),
      catalogue: [USD],
    });
    const view = overview();

    const kpi = view.querySelector('[aria-label="Report KPIs"]');
    expect(kpi?.textContent).not.toContain("Rp480.000.000");
    expect(kpiValue("PO amount")).toBe("-");
    expect(kpiValue("PO margin %")).toBe("10.77%");
    expect(
      view.querySelector("[data-report-overview-currency]")?.textContent
    ).toContain("IDR is not in this app's currency catalogue");
    const coverage = view.querySelector("[data-report-coverage]");
    expect(coverage?.textContent).toContain("90.00%");
    expect(coverage?.textContent).toContain("-");
    const chart = view.querySelector("[data-report-chart]");
    expect(chart?.textContent).toContain("not in this app's currency catalogue");
    expect(chart?.querySelector("[data-chart-axis]")).toBeNull();
    expect(view.querySelectorAll("[data-report-period-toggle]")).toHaveLength(1);
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("renders the complete maximum exact amounts without rounding", async () => {
    const report = purchaseOrderReportMaxFixture();
    report.filter = {
      from: "2026-01-01",
      to: "2027-01-01",
      bucket: "month",
      category: "all",
      currency: "IDR",
    };
    await render({ report });

    expect(kpiValue("PO count")).toBe("9,007,199,254,740,991");
    expect(kpiValue("PO amount")).toBe("Rp9.007.199.254.740.991");
    expect(kpiValue("PO margin %")).toBe("0.00%");
    expect(reportMock).not.toHaveBeenCalled();
  });

  it("keeps Pipeline and Realized Variance in separate cards, never inside Actual (#653)", async () => {
    await render();
    const actual = overview().querySelector<HTMLElement>(
      "[data-report-overview-actual]"
    );
    expect(actual?.textContent).not.toContain("Pipeline");
    expect(actual?.textContent).not.toContain("Realized Variance");

    const pipeline = overview().querySelector<HTMLElement>(
      "[data-report-pipeline-card]"
    );
    expect(pipeline).not.toBeNull();
    expect(pipeline?.textContent).toContain("Pipeline — Unsold");
    expect(pipeline?.textContent).toContain(
      "Unsold is not a loss and is not added to Actual."
    );
    expect(pipeline?.textContent).toContain("Rp42.000.000");

    const variance = overview().querySelector<HTMLElement>(
      "[data-report-variance-card]"
    );
    expect(variance).not.toBeNull();
    expect(variance?.textContent).toContain("Realized Variance");
    expect(variance?.textContent).toContain("Quantity extra income");
    expect(variance?.textContent).toContain("Price extra income");
    expect(variance?.textContent).toContain("Price loss");
    expect(variance?.textContent).toContain("−Rp3.500.000");
    expect(variance?.textContent).toContain("Closed loss");
    expect(variance?.textContent).toContain("Net realized variance");
    expect(variance?.textContent).toContain("Rp4.000.000");
  });

  it("keeps a signed KPI loss and a null percentage instead of inventing zero", async () => {
    const loss = purchaseOrderReportYearFixture();
    loss.totals = {
      ...loss.totals,
      marginMinor: -30_000_000,
      marginBps: -7_692,
    };
    await render({ report: loss });

    expect(kpiValue("PO margin")).toBe("Rp-30.000.000");
    expect(kpiValue("PO margin %")).toBe("-76.92%");
    const marginCell = Array.from(
      overview().querySelectorAll('[aria-label="Report KPIs"] div')
    ).find((node) => node.querySelector("dt")?.textContent === "PO margin");
    expect(marginCell?.querySelector("dd")?.className).toContain(
      "text-danger-700"
    );

    const unknown = purchaseOrderReportYearFixture();
    unknown.totals = { ...unknown.totals, marginBps: null };
    await render({ report: unknown });
    expect(kpiValue("PO margin %")).toBe("-");
    expect(kpiValue("PO margin %")).not.toBe("0.00%");
    expect(reportMock).not.toHaveBeenCalled();
  });
});
