// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { poreport } from "@backend/models";
import App from "./App";
import { normalizePurchaseOrderReport } from "./poReport";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportQuery,
} from "./poReport";
import { purchaseOrderReportFixture, purchaseOrderReportYearFixture } from "./poReportFixtures";
import { buildPurchaseOrderReportView, type PurchaseOrderReportView } from "./poReportExport";
import { nextMonthStart } from "./poReportComparison";
import {
  initialReportFilters,
  toReportQuery,
  type ReportFilterState,
} from "./poReportPage";
import { ToastProvider } from "./toast";
import { todayWIB } from "./workspaceSettings";

// Issue #632: the export callback is wired in the real App, not re-created by
// a harness. Only the desktop bindings are replaced, so every assertion is the
// production path: Home -> Open Reports -> GetPurchaseOrderReport -> the
// normalized snapshot the screen shows -> Export PDF -> the binding receives
// exactly that snapshot as JSON.
//
// The fixture is the shared normalized report fixture (#630), and the clock is
// pinned to the date it describes, so the initial "this year" filter is the
// report's own filter. The PDF renderer itself is not exercised here; #632's
// own tests cover the native document.

const IDR = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before" as const,
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const USD = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before" as const,
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const USER = {
  ID: 7,
  Email: "staff@example.com",
  DisplayName: "Sam Staff",
  Role: "staff",
};

const PROFILE = {
  LegalName: "PT Arcon Penta Persada",
  Address: "Jl. Test",
  Phone: "",
  Email: "",
  TaxID: "",
  Currency: "IDR",
  CanonicalUoM: "Lot",
};

const mocks = vi.hoisted(() => ({
  restoreSession: vi.fn((..._args: unknown[]): unknown => undefined),
  currentUser: vi.fn((..._args: unknown[]): unknown => undefined),
  checkSession: vi.fn((..._args: unknown[]): unknown => undefined),
  logout: vi.fn(async (..._args: unknown[]) => {}),
  renewSession: vi.fn((..._args: unknown[]): unknown => undefined),
  getCompany: vi.fn((..._args: unknown[]): unknown => undefined),
  loadRecovery: vi.fn(async (..._args: unknown[]) => ""),
  saveRecovery: vi.fn(async (..._args: unknown[]) => {}),
  clearRecovery: vi.fn(async (..._args: unknown[]) => {}),
  getCompanyUnits: vi.fn(async (..._args: unknown[]) => [
    { Value: "Lot", Label: "Lot" },
  ]),
  getCurrencyCatalogue: vi.fn(async (..._args: unknown[]) => [IDR, USD]),
  listDrafts: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listCustomers: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  getPurchaseOrderReport: vi.fn((..._args: unknown[]): unknown => undefined),
  exportPurchaseOrderReport: vi.fn(
    (..._args: unknown[]): unknown => undefined
  ),
  version: vi.fn(async (..._args: unknown[]) => "v0.20.0"),
  getChangelog: vi.fn(async (..._args: unknown[]) => ""),
  checkForUpdates: vi.fn(async (..._args: unknown[]) => ({
    UpdateAvailable: false,
  })),
}));

vi.mock("@backend/backend/AuthAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AuthAPI")>();
  return {
    ...actual,
    RestoreSession: (...args: unknown[]) => mocks.restoreSession(...args),
    CurrentUser: (...args: unknown[]) => mocks.currentUser(...args),
    CheckSession: (...args: unknown[]) => mocks.checkSession(...args),
    Logout: (...args: unknown[]) => mocks.logout(...args),
    RenewSession: (...args: unknown[]) => mocks.renewSession(...args),
    GetCompany: (...args: unknown[]) => mocks.getCompany(...args),
  };
});

vi.mock("@backend/backend/RecoveryAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/RecoveryAPI")>();
  return {
    ...actual,
    LoadRecovery: (...args: unknown[]) => mocks.loadRecovery(...args),
    SaveRecovery: (...args: unknown[]) => mocks.saveRecovery(...args),
    ClearRecovery: (...args: unknown[]) => mocks.clearRecovery(...args),
  };
});

vi.mock("@backend/backend/AdminAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AdminAPI")>();
  return {
    ...actual,
    GetCompanyUnits: (...args: unknown[]) => mocks.getCompanyUnits(...args),
    GetCurrencyCatalogue: (...args: unknown[]) => mocks.getCurrencyCatalogue(...args),
  };
});

vi.mock("@backend/backend/Quotation", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/Quotation")>();
  return {
    ...actual,
    ListDrafts: (...args: unknown[]) => mocks.listDrafts(...args),
    ListCustomers: (...args: unknown[]) => mocks.listCustomers(...args),
    GetPurchaseOrderReport: (...args: unknown[]) =>
      mocks.getPurchaseOrderReport(...args),
    ExportPurchaseOrderReport: (...args: unknown[]) =>
      mocks.exportPurchaseOrderReport(...args),
    ClearDraftContext: vi.fn(async () => {}),
    ClearRevisionContext: vi.fn(async () => {}),
    SetDraftContext: vi.fn(async () => {}),
    SetRevisionContext: vi.fn(async () => {}),
    GetPurchaseOrderDocumentState: vi.fn(async () => ({
      Document: null,
      Versions: [],
    })),
    LoadPOReconciliation: vi.fn(async () => {
      throw new Error("no reconciliation recorded");
    }),
    SavePOReconciliation: vi.fn(async (...args: unknown[]) => args[1]),
  };
});

vi.mock("@backend/backend/UpdateAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/UpdateAPI")>();
  return {
    ...actual,
    Version: (...args: unknown[]) => mocks.version(...args),
    GetChangelog: (...args: unknown[]) => mocks.getChangelog(...args),
    CheckForUpdates: (...args: unknown[]) => mocks.checkForUpdates(...args),
  };
});

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

/** The shared normalized fixture, read through the wire shape the binding returns. */
function reportWire(): poreport.Report {
  const report = purchaseOrderReportFixture();
  const month = report.asOf.slice(0, 7);
  const period = report.periods.find(
    (row) => row.periodStart === `${month}-01`
  );
  if (period !== undefined) {
    // The base fixture predates the server-owned monthSummary. The page now
    // requires current-month evidence before export, so attach the fixture's
    // own month-to-date row with an explicitly unavailable forecast.
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
  return report as unknown as poreport.Report;
}

function reportFor(query: PurchaseOrderReportQuery): poreport.Report {
  const wire = reportWire();
  wire.filter = { ...query };
  return wire;
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

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockReset();

  // The fixture is "this year to 19 November 2026"; pin the clock so the
  // page's initial filter is that same range, on any day the suite runs.
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-11-19T03:00:00.000Z"));

  mocks.restoreSession.mockResolvedValue(USER);
  mocks.currentUser.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.listDrafts.mockResolvedValue([]);
  mocks.listCustomers.mockResolvedValue([]);
  mocks.getPurchaseOrderReport.mockImplementation(
    async (query: unknown) => reportFor(query as PurchaseOrderReportQuery)
  );
  mocks.exportPurchaseOrderReport.mockResolvedValue("/tmp/opencode/report.pdf");
  mocks.checkForUpdates.mockResolvedValue({ UpdateAvailable: false });
  Element.prototype.scrollIntoView = () => undefined;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  window.localStorage.clear();
  vi.useRealTimers();
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

async function renderApp(): Promise<void> {
  await act(async () => {
    root.render(
      <ToastProvider>
        <App />
      </ToastProvider>
    );
  });
  await flush();
}

async function openReports(): Promise<void> {
  await renderApp();
  await act(async () => {
    button("Open Reports").click();
  });
  await flush();
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

function reportCalls(): PurchaseOrderReportQuery[] {
  return mocks.getPurchaseOrderReport.mock.calls.map(
    (call) => call[0] as PurchaseOrderReportQuery
  );
}

function sentSnapshot(call = 0): PurchaseOrderReportView {
  const sent = mocks.exportPurchaseOrderReport.mock.calls[call]?.[0];
  if (typeof sent !== "string") {
    throw new Error("the export binding received no JSON string");
  }
  return JSON.parse(sent) as PurchaseOrderReportView;
}

describe("Reports export wiring through the real App (issue #632)", () => {
  it("exports the exact visible normalized snapshot from Home -> Reports", async () => {
    const pending = deferred<poreport.Report>();
    mocks.getPurchaseOrderReport.mockImplementationOnce(() => pending.promise);

    await openReports();

    // A real fetch has not answered yet: the page keeps export disabled.
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(1);
    const initialFilter: ReportFilterState = initialReportFilters(
      todayWIB(),
      "IDR"
    );
    expect(reportCalls()[0]).toEqual(toReportQuery(initialFilter));
    expect(button("Export PDF").disabled).toBe(true);

    pending.resolve(reportFor(reportCalls()[0]));
    await flush();

    // The snapshot the server returned is now the visible one.
    expect(button("Export PDF").disabled).toBe(false);

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(mocks.exportPurchaseOrderReport).toHaveBeenCalledTimes(1);
    // Export is a pure pass-through of the visible snapshot: it never asks
    // the API again, so it cannot use a filter newer than the document.
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(1);

    const expected = normalizePurchaseOrderReport(
      reportFor(reportCalls()[0])
    );
    const expectedView = buildPurchaseOrderReportView(expected, {
      months: [
        { month: "2026-11", source: expected, usesOverviewMonthSummary: false },
      ],
      layout: "individual",
    });
    const sent = mocks.exportPurchaseOrderReport.mock.calls[0][0] as string;
    expect(sent).toBe(JSON.stringify(expectedView));

    const parsed = sentSnapshot();
    expect(parsed).toEqual(expectedView);
    expect(parsed.version).toBe(1);
    expect(parsed.sources).toHaveLength(1);
    expect(parsed.overview).toBe(0);
    expect(parsed.periods).toEqual({
      sources: [0],
      months: ["2026-11"],
      layout: "individual",
    });
    const overview = parsed.sources[0];
    expect(overview.filter).toEqual(expected.filter);
    expect(overview.generatedAt).toBe(expected.generatedAt);
    expect(overview.asOf).toBe(expected.asOf);
    // The current-period forecast is part of the exported snapshot, not only
    // of the screen state, so the PDF renderer can draw the same ghost.
    expect(overview.currentProjection).toEqual(expected.currentProjection);
    expect(overview.currentProjection.mode).toBe("none");
    // The fixture carries the two values a lossy serializer would flatten.
    expect(
      overview.categories.find(
        (category) => category.businessCategory === "UNASSIGNED"
      )?.marginBps
    ).toBeNull();
    expect(overview.categories.some((category) => category.marginMinor < 0)).toBe(
      true
    );

    expect(document.body.textContent).toContain(
      "Report saved to /tmp/opencode/report.pdf"
    );
  });

  it("exports the All/Aggregate shape chosen through the live month picker", async () => {
    // The annual source is the coherent 12-row year fixture; the base export
    // fixture is a partial-year payload and an Aggregate of it is correctly
    // refused as incomplete.
    mocks.getPurchaseOrderReport.mockImplementation(async (query: unknown) => {
      const report = purchaseOrderReportYearFixture();
      report.filter = { ...(query as PurchaseOrderReportQuery) };
      return report as unknown as poreport.Report;
    });
    await openReports();
    expect(button("Export PDF").disabled).toBe(false);

    await act(async () => {
      button("Choose months").click();
    });
    await flush();
    const all = document.querySelector<HTMLButtonElement>("[data-picker-all]");
    if (all === null) throw new Error("no All months control");
    await act(async () => {
      all.click();
    });
    await flush();
    const close = document.querySelector<HTMLButtonElement>("[data-picker-close]");
    if (close === null) throw new Error("no picker close");
    await act(async () => {
      close.click();
    });
    await flush();

    expect(document.querySelector("[data-month-aggregate]")).not.toBeNull();
    expect(button("Export PDF").disabled).toBe(false);
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(1);
    const view = sentSnapshot();
    expect(view.periods.layout).toBe("aggregate");
    expect(view.periods.months).toHaveLength(12);
    expect(view.periods.months[0]).toBe("2026-01");
    expect(view.periods.months[11]).toBe("2026-12");
    expect(view.periods.sources).toEqual([0]);
    expect(view.sources).toHaveLength(1);
    expect(view.sources[0].filter).toEqual(reportCalls()[0]);
  });

  it("treats an empty binding result as a cancel, with no success toast", async () => {
    mocks.exportPurchaseOrderReport.mockResolvedValue("");
    await openReports();

    expect(button("Export PDF").disabled).toBe(false);
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(mocks.exportPurchaseOrderReport).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toContain("Report saved");
    expect(document.body.textContent).not.toContain(
      "The PDF could not be saved. Choose another location and try again."
    );
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("reports a failed binding actionably and re-enables export", async () => {
    mocks.exportPurchaseOrderReport.mockRejectedValue(
      new Error("The save dialog was refused.")
    );
    await openReports();

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(mocks.exportPurchaseOrderReport).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("The save dialog was refused.");
    expect(document.body.textContent).not.toContain("Report saved");
    expect(button("Export PDF").disabled).toBe(false);
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(1);
  });

  it("disables export while a new filter loads and cannot send the previous snapshot", async () => {
    const first = deferred<poreport.Report>();
    const second = deferred<poreport.Report>();
    mocks.getPurchaseOrderReport
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);

    await openReports();
    expect(button("Export PDF").disabled).toBe(true);
    first.resolve(reportFor(reportCalls()[0]));
    await flush();
    expect(button("Export PDF").disabled).toBe(false);

    await act(async () => {
      button("Custom date range").click();
    });
    await flush();
    await choose("Bucket", "Year");

    // The previous monthly snapshot is still visible but stale while the
    // yearly request is in flight; export must stay off.
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(2);
    expect(reportCalls()[1].bucket).toBe("year");
    expect(button("Export PDF").disabled).toBe(true);

    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
    expect(mocks.exportPurchaseOrderReport).not.toHaveBeenCalled();

    second.resolve(reportFor(reportCalls()[1]));
    await flush();

    expect(button("Export PDF").disabled).toBe(false);
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();

    expect(mocks.exportPurchaseOrderReport).toHaveBeenCalledTimes(1);
    // The year-bucketed overview plus the exact monthly snapshot the initial
    // Year view already loaded. The overview alone is not a monthly source.
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(2);
    const view = sentSnapshot();
    expect(view.sources[0].filter).toEqual(reportCalls()[1]);
    expect(view.sources[1].filter).toEqual(reportCalls()[0]);
    expect(view.periods.months).toEqual(["2026-11"]);
    expect(view.periods.sources).toEqual([1]);
  });
});

describe("category comparison export through the real App (issue #643)", () => {
  async function openCategoryPicker(): Promise<void> {
    await act(async () => {
      button("Choose categories").click();
    });
    await flush();
  }

  function categoryPickerKey(code: string): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>(
      `[data-picker-key="${code}"]`
    );
    if (found === null) throw new Error(`no category picker item ${code}`);
    return found;
  }

  async function clickPicker(control: HTMLElement): Promise<void> {
    await act(async () => {
      control.click();
    });
    await flush();
  }

  async function closeCategoryPicker(): Promise<void> {
    const close = document.querySelector<HTMLButtonElement>("[data-picker-close]");
    if (close === null) throw new Error("no category picker close");
    await clickPicker(close);
  }

  async function exportPdf(): Promise<void> {
    await act(async () => {
      button("Export PDF").click();
    });
    await flush();
  }

  it("exports a restricted overview with a category=all Individual subset", async () => {
    await openReports();
    expect(button("Export PDF").disabled).toBe(false);
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(1);

    await choose("Category", "Facility Management");
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(2);
    await openCategoryPicker();
    await clickPicker(categoryPickerKey("FAB"));
    await closeCategoryPicker();

    expect(button("Export PDF").disabled).toBe(false);
    await exportPdf();

    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(2);
    expect(mocks.exportPurchaseOrderReport).toHaveBeenCalledTimes(1);
    const view = sentSnapshot();
    expect(view.version).toBe(1);
    expect(view.sources).toHaveLength(2);
    expect(view.sources[0].filter.category).toBe("FM");
    expect(view.sources[1].filter).toEqual(reportCalls()[0]);
    expect(view.sources[1].filter.category).toBe("all");
    expect(view.categories).toEqual({
      source: 1,
      codes: ["FAB"],
      layout: "individual",
    });
    expect(view.periods.months).toEqual(["2026-11"]);
    expect(view.periods.sources).toEqual([0]);
    expect(view.overview).toBe(0);
  });

  it("exports All/Individual from the independent all-category snapshot", async () => {
    await openReports();
    await choose("Category", "Facility Management");
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(2);

    await openCategoryPicker();
    await clickPicker(
      document.querySelector<HTMLButtonElement>("[data-picker-all]")!
    );
    await closeCategoryPicker();
    await act(async () => {
      button("Individual").click();
    });
    await flush();
    expect(document.querySelectorAll("[data-category-card]")).toHaveLength(5);

    await exportPdf();
    const view = sentSnapshot();
    expect(view.categories).toEqual({
      source: 1,
      codes: ["FM", "FAB", "SUPPLY_GEN", "OTHERS", "UNASSIGNED"],
      layout: "individual",
    });
    expect(view.sources).toHaveLength(2);
    expect(view.sources[0].filter.category).toBe("FM");
    expect(view.sources[1].filter.category).toBe("all");
    // The whole local journey made no additional API read.
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(2);
  });

  it("exports an explicit empty category section without a financial source", async () => {
    await openReports();
    await choose("Category", "Facility Management");
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(2);

    await openCategoryPicker();
    await clickPicker(categoryPickerKey("FAB"));
    await clickPicker(categoryPickerKey("FAB"));
    await closeCategoryPicker();
    expect(document.querySelector("[data-category-empty]")).not.toBeNull();

    await exportPdf();
    const view = sentSnapshot();
    expect(view.version).toBe(1);
    expect(view.categories).toEqual({
      source: 0,
      codes: [],
      layout: "individual",
    });
    // No extra all-category snapshot is carried for an empty section.
    expect(view.sources).toHaveLength(1);
    expect(view.sources[0].filter.category).toBe("FM");
    expect(view.overview).toBe(0);
    expect(view.periods).toEqual({
      sources: [0],
      months: ["2026-11"],
      layout: "individual",
    });
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(2);
  });
});
