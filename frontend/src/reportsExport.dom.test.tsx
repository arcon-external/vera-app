// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { poreport } from "@backend/models";
import App from "./App";
import type { PurchaseOrderDTO } from "./purchaseOrder";
import type { PurchaseOrderWorkspacePageProps } from "./components/PurchaseOrderWorkspace";
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
// production path: admin Home -> Open Reports -> GetPurchaseOrderReport -> the
// normalized snapshot the screen shows -> Export PDF or Export Excel -> the
// binding receives exactly that snapshot as JSON.
//
// #683 made Reports admin-only, so every session in this suite is an admin; the
// staff side of the gate has its own describe below.
//
// The fixture is the shared normalized report fixture (#630), and the clock is
// pinned to the date it describes, so the initial "this year" filter is the
// report's own filter. The PDF and workbook renderers are not exercised here;
// #632's and #683's own backend tests cover the native documents.

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
  Email: "admin@example.com",
  DisplayName: "Ada Admin",
  Role: "admin",
};

const STAFF = {
  ID: 9,
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
  listPurchaseOrders: vi.fn(async (..._args: unknown[]): Promise<unknown> => ({
    PurchaseOrders: [] as unknown[],
    Customers: [] as unknown[],
    Page: 1,
    PageSize: 15,
    Total: 0,
    TotalPages: 1,
  })),
  getPurchaseOrderReport: vi.fn((..._args: unknown[]): unknown => undefined),
  exportPurchaseOrderReport: vi.fn(
    (..._args: unknown[]): unknown => undefined
  ),
  exportPurchaseOrderReportXlsx: vi.fn(
    (..._args: unknown[]): unknown => undefined
  ),
  listQuotations: vi.fn((..._args: unknown[]): unknown => []),
  previewDeletion: vi.fn((..._args: unknown[]): unknown => undefined),
  createDeletionChallenge: vi.fn(
    (..._args: unknown[]): unknown => undefined
  ),
  confirmDeletion: vi.fn((..._args: unknown[]): unknown => undefined),
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
    ListPurchaseOrders: (...args: unknown[]) => mocks.listPurchaseOrders(...args),
    GetPurchaseOrderReport: (...args: unknown[]) =>
      mocks.getPurchaseOrderReport(...args),
    ExportPurchaseOrderReport: (...args: unknown[]) =>
      mocks.exportPurchaseOrderReport(...args),
    ExportPurchaseOrderReportXLSX: (...args: unknown[]) =>
      mocks.exportPurchaseOrderReportXlsx(...args),
    ListQuotations: (...args: unknown[]) => mocks.listQuotations(...args),
    PreviewDeletion: (...args: unknown[]) => mocks.previewDeletion(...args),
    CreateDeletionChallenge: (...args: unknown[]) =>
      mocks.createDeletionChallenge(...args),
    ConfirmDeletion: (...args: unknown[]) => mocks.confirmDeletion(...args),
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

// Issue #645 B1: the App owns the mutation wiring, so these tests drive the
// real App and replace only the child workspace shell. The editor tabs capture
// the exact callbacks App passes and let a test invoke them with a DTO; the
// archive/drafts tabs delegate to the real composition, so PO deletion still
// runs through the real list page, dialog and App callback.
const poProbe = vi.hoisted(() => ({
  props: null as PurchaseOrderWorkspacePageProps | null,
}));

vi.mock("./components/PurchaseOrderWorkspace", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("./components/PurchaseOrderWorkspace")>();
  const { createElement } = await import("react");
  return {
    ...actual,
    PurchaseOrderWorkspacePage: (props: PurchaseOrderWorkspacePageProps) => {
      if (props.tab === "archive" || props.tab === "drafts") {
        return createElement(actual.PurchaseOrderWorkspacePage, props);
      }
      poProbe.props = props;
      return createElement("div", { id: "po-probe" }, "PO probe");
    },
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
  poProbe.props = null;

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
  mocks.exportPurchaseOrderReportXlsx.mockResolvedValue(
    "/tmp/opencode/report.xlsx"
  );
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
  vi.restoreAllMocks();
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

interface PollControl {
  run: () => Promise<void>;
  restore: () => void;
}

/** Captures App's 60-second session poll without faking every timer. */
function pollControl(): PollControl {
  const intervals = new Map<number, () => void>();
  let nextID = 1;
  const setSpy = vi
    .spyOn(window, "setInterval")
    .mockImplementation((handler) => {
      const id = nextID++;
      intervals.set(id, handler as () => void);
      return id as unknown as ReturnType<typeof window.setInterval>;
    });
  const clearSpy = vi
    .spyOn(window, "clearInterval")
    .mockImplementation((id) => {
      intervals.delete(Number(id));
    });
  return {
    run: async () => {
      const handlers = [...intervals.values()];
      await act(async () => {
        for (const handler of handlers) handler();
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
      await flush();
    },
    restore: () => {
      setSpy.mockRestore();
      clearSpy.mockRestore();
    },
  };
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

  it("wires Export Excel to the workbook binding with the exact visible snapshot", async () => {
    await openReports();

    expect(button("Export Excel").disabled).toBe(false);
    await act(async () => {
      button("Export Excel").click();
    });
    await flush();

    expect(mocks.exportPurchaseOrderReportXlsx).toHaveBeenCalledTimes(1);
    expect(mocks.exportPurchaseOrderReport).not.toHaveBeenCalled();
    // The workbook is a pass-through of the visible snapshot too: no refetch,
    // no filter newer than the figures on screen.
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
    const sent = mocks.exportPurchaseOrderReportXlsx.mock.calls[0][0] as string;
    expect(sent).toBe(JSON.stringify(expectedView));
    const parsed = JSON.parse(sent) as PurchaseOrderReportView;
    expect(parsed.sources[0].filter).toEqual(expected.filter);
    expect(parsed.periods.months).toEqual(["2026-11"]);

    expect(document.body.textContent).toContain(
      "Report saved to /tmp/opencode/report.xlsx"
    );
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("keeps the visible report when the workbook binding rejects", async () => {
    mocks.exportPurchaseOrderReportXlsx.mockRejectedValue(
      new Error("The save dialog was refused.")
    );
    await openReports();

    await act(async () => {
      button("Export Excel").click();
    });
    await flush();

    expect(mocks.exportPurchaseOrderReportXlsx).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("The save dialog was refused.");
    expect(document.body.textContent).not.toContain("Report saved to");
    expect(mocks.getPurchaseOrderReport).toHaveBeenCalledTimes(1);
    expect(button("Export Excel").disabled).toBe(false);
    expect(button("Export PDF").disabled).toBe(false);
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

// Issue #645: the real App is the workspace lifecycle owner. Reports is
// mounted once and hidden on other pages; a relevant successful mutation marks
// every cached report source stale, and the next return revalidates exactly
// the scopes the screen requires while every filter and local selection stays
// where it was. A different account adopted by the session poll clears the
// mounted workspace; the same account's renewal keeps it.
describe("report freshness through the real App (issue #645)", () => {
  function navTab(label: string): HTMLButtonElement {
    const nav = document.querySelector('nav[aria-label="Primary"]');
    if (nav === null) throw new Error("no workspace tabs");
    const found = Array.from(
      nav.querySelectorAll<HTMLButtonElement>("button")
    ).find((candidate) => candidate.textContent?.trim() === label);
    if (found === undefined) throw new Error(`no workspace tab ${label}`);
    return found;
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

  async function selectMonth(month: string, previousPages = 0): Promise<void> {
    await clickNode(button("Choose months"));
    for (let index = 0; index < previousPages; index += 1) {
      await clickNode(button("Previous page"));
    }
    await clickNode(pickerNode(`[data-picker-key="${month}"]`));
    await clickNode(pickerNode("[data-picker-close]"));
  }

  async function selectCategory(code: string): Promise<void> {
    await clickNode(button("Choose categories"));
    await clickNode(pickerNode(`[data-picker-key="${code}"]`));
    await clickNode(pickerNode("[data-picker-close]"));
  }

  function poOrder(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
    return {
      PODate: "2026-10-01",
      RequestedDeliveryDate: null,
      UUID: "uuid-645",
      State: "saved",
      QuoteNumber: "1502260045",
      CustomerName: "PT. Buyer",
      BillingAddress: "Jl. Billing",
      DeliveryAddress: "Jl. Delivery",
      ShipToCode: "SHIP-1",
      ContactEmail: "buyer@example.com",
      ContactPhone: "+62 21 555",
      PONumber: "PO-645",
      ReceivedDate: "2026-10-02",
      Version: 3,
      Incoterms: "CIF",
      Currency: "IDR",
      TaxZone: "FTZ",
      TaxCategory: "",
      AdjustmentReason: "",
      CreatedAt: "2026-10-02T10:00:00Z",
      UpdatedAt: "2026-10-02T10:00:00Z",
      Items: [
        {
          QuotationItemNo: 1,
          QuotationSubNo: null,
          LineNumber: 1,
          Description: "Widget",
          QuantityScaled: 20000,
          UoM: "Pc",
          UnitPriceMinor: 10000,
          DiscountEnabled: false,
          DiscountPercent: 0,
          GrossMinor: 20000,
          DiscountMinor: 0,
          NetMinor: 20000,
        },
      ],
      ID: 7,
      QuotationID: 12,
      SourceRevision: 1,
      CustomerID: 2,
      PaymentTerms: "45 days",
      DiscountPercent: 0,
      PpnBps: 0,
      SubtotalMinor: 20000,
      DiscountMinor: 0,
      PpnMinor: 0,
      AdjustmentMinor: 0,
      GrandTotalMinor: 20000,
      PrintedSubtotalMinor: 20000,
      PrintedDiscountMinor: 0,
      PrintedPpnMinor: 0,
      PrintedGrandTotalMinor: 20000,
      ...overrides,
    };
  }

  function poRow(overrides: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      UUID: "uuid-645",
      State: "saved",
      PONumber: "PO-645",
      QuoteNumber: "1502260045",
      CustomerName: "PT. Buyer",
      ReceivedDate: "2026-10-02",
      Currency: "IDR",
      ID: 7,
      CustomerID: 2,
      SourceRevision: 1,
      SourceMissing: false,
      ...overrides,
    };
  }

  function poListPage(
    rows: Record<string, unknown>[]
  ): Record<string, unknown> {
    return {
      PurchaseOrders: rows,
      Customers: [],
      Page: 1,
      PageSize: 15,
      Total: rows.length,
      TotalPages: 1,
    };
  }

  function poDeletionRecord(
    overrides: Record<string, unknown> = {}
  ): Record<string, unknown> {
    return {
      Type: "purchase_order",
      ID: 7,
      UUID: "uuid-645",
      DocumentNumber: "PO-645",
      SourceReference: "1502260045",
      Label: "Purchase order PO-645",
      State: "saved",
      Version: 3,
      Revisions: [] as number[],
      DocumentVersions: 0,
      HistoryEntries: 0,
      ...overrides,
    };
  }

  function poDeletionPreview(
    overrides: Record<string, unknown> = {}
  ): Record<string, unknown> {
    return {
      TargetType: "purchase_order",
      TargetID: 7,
      TargetUUID: "uuid-645",
      DocumentNumber: "PO-645",
      DocumentNumberLabel: "Customer PO number",
      QuoteNumber: "1502260045",
      PONumber: "PO-645",
      CustomerName: "PT. Buyer",
      State: "saved",
      SourceMissing: false,
      Version: 3,
      Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
      Scopes: [
        {
          Scope: "record",
          Available: true,
          UnavailableReason: "",
          RequiresChallenge: false,
          Removes: [poDeletionRecord()],
          Preserves: [],
        },
      ],
      ...overrides,
    };
  }

  function monthBrowser(): HTMLElement {
    const found = document.querySelector<HTMLElement>("[data-month-browser]");
    if (found === null) throw new Error("no month browser");
    return found;
  }

  /** True while the mounted Reports page sits inside its hidden wrapper. */
  function reportsHidden(): boolean {
    const heading = document.getElementById("reports-heading");
    if (heading === null) throw new Error("Reports is not mounted");
    return heading.closest("[hidden]") !== null;
  }

  function probeProps(): PurchaseOrderWorkspacePageProps {
    if (poProbe.props === null) throw new Error("the PO probe never rendered");
    return poProbe.props;
  }

  async function invokeProbe(
    name: "onSaved" | "onRepaired" | "onLinked" | "onDeleted",
    ...args: unknown[]
  ): Promise<void> {
    const props = probeProps() as unknown as Record<
      string,
      ((...callArgs: unknown[]) => void) | undefined
    >;
    const callback = props[name];
    if (callback === undefined) throw new Error(`no ${name} callback`);
    await act(async () => {
      callback(...args);
    });
    await flush();
  }

  async function openPurchaseOrderWorkspace(): Promise<void> {
    await clickNode(button("Back to Home"));
    await clickNode(button("Open Purchase Orders"));
    expect(document.getElementById("po-probe")).not.toBeNull();
    expect(reportsHidden()).toBe(true);
  }

  async function openPurchaseOrderArchive(): Promise<void> {
    await openPurchaseOrderWorkspace();
    await clickNode(navTab("Archive"));
  }

  async function openPoDeletionDialog(label = "PO-645"): Promise<void> {
    await clickNode(button(`More actions for purchase order ${label}`));
    await clickNode(button(`Delete purchase order ${label}`));
  }

  async function confirmPoDeletion(label = "PO-645"): Promise<void> {
    typeField("Deletion reason", "duplicate entry");
    typeField(`Type ${label} to confirm`, label);
    await clickNode(button("Delete permanently"));
  }

  function buttonWithText(text: string): HTMLButtonElement {
    const found = Array.from(document.querySelectorAll("button")).find(
      (candidate) => candidate.textContent?.trim() === text
    );
    if (found === undefined) throw new Error(`no button reading ${text}`);
    return found;
  }

  /** Makes every later report read carry a visibly new amount. */
  function armReportMutation(): () => void {
    let mutated = false;
    mocks.getPurchaseOrderReport.mockImplementation(async (query: unknown) => {
      const report = reportFor(query as PurchaseOrderReportQuery);
      if (mutated) {
        report.totals = { ...report.totals, amountMinor: 777_000_000 };
      }
      return report;
    });
    return () => {
      mutated = true;
    };
  }

  function typeField(label: string, value: string): void {
    const field = document.querySelector<
      HTMLInputElement | HTMLTextAreaElement
    >(`input[aria-label="${label}"], textarea[aria-label="${label}"]`);
    if (field === null) throw new Error(`no field labelled ${label}`);
    const prototype =
      field instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    act(() => {
      Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(
        field,
        value
      );
      field.dispatchEvent(new Event("input", { bubbles: true }));
    });
  }

  function quotationRow(): Record<string, unknown> {
    return {
      ID: 12,
      QuoteNumber: "1502260012",
      RevisionNumber: 1,
      CustomerName: "PT. Buyer",
      Subject: "AC maintenance",
      Currency: "IDR",
      CreatedAt: "2026-10-02T10:00:00Z",
      GrandTotal: 1_500_000,
      DisplayGrand: "1.500.000",
    };
  }

  function deletionRecord(): Record<string, unknown> {
    return {
      Type: "quotation",
      ID: 12,
      UUID: "uuid-12",
      DocumentNumber: "1502260012",
      SourceReference: "PO-645",
      Label: "Quotation 1502260012",
      State: "saved",
      Version: 2,
      Revisions: [0, 1],
      DocumentVersions: 0,
      HistoryEntries: 0,
    };
  }

  function deletionPreview(): Record<string, unknown> {
    return {
      TargetType: "quotation",
      TargetID: 12,
      TargetUUID: "uuid-12",
      DocumentNumber: "1502260012",
      DocumentNumberLabel: "Quotation number",
      QuoteNumber: "1502260012",
      PONumber: "PO-645",
      CustomerName: "PT. Buyer",
      State: "saved",
      SourceMissing: false,
      Version: 2,
      Fingerprint: "v1;target=quotation;quotation_id=12",
      Scopes: [
        {
          Scope: "record",
          Available: true,
          UnavailableReason: "",
          RequiresChallenge: false,
          Removes: [deletionRecord()],
          Preserves: [],
        },
      ],
    };
  }

  it("revalidates after a confirmed quotation deletion and keeps both selections", async () => {
    mocks.restoreSession.mockResolvedValue({ ...USER, Role: "admin" });
    mocks.currentUser.mockResolvedValue({ ...USER, Role: "admin" });
    mocks.listQuotations.mockResolvedValue([quotationRow()]);
    mocks.previewDeletion.mockResolvedValue(deletionPreview());
    mocks.confirmDeletion.mockResolvedValue({
      Outcome: { Removed: [deletionRecord()], AuditIDs: [1] },
    });

    let deleted = false;
    mocks.getPurchaseOrderReport.mockImplementation(async (query: unknown) => {
      const report = reportFor(query as PurchaseOrderReportQuery);
      if (deleted) {
        report.totals = { ...report.totals, amountMinor: 777_000_000 };
      }
      return report;
    });

    await openReports();
    await selectMonth("2025-01", 1);
    await selectCategory("FAB");
    const initialQuery = toReportQuery(initialReportFilters(todayWIB(), "IDR"));
    const annual2025: PurchaseOrderReportQuery = {
      from: "2025-01-01",
      to: "2026-01-01",
      bucket: "month",
      category: "all",
      currency: "IDR",
    };
    expect(reportCalls()).toEqual([initialQuery, annual2025]);
    expect(
      button("Choose months").getAttribute("data-month-selection-summary")
    ).toBe("2 months · 2025–2026");
    expect(
      button("Choose categories").getAttribute(
        "data-category-selection-summary"
      )
    ).toBe("Fabrication");

    // Leave Reports for the quotation archive and delete one saved quotation.
    await clickNode(button("Back to Home"));
    await clickNode(button("Open Quotation"));
    await clickNode(navTab("Archive"));
    await clickNode(button("Delete quotation 1502260012"));
    expect(document.body.textContent).toContain("Delete permanently");
    typeField("Deletion reason", "duplicate entry");
    typeField("Type 1502260012 to confirm", "1502260012");
    deleted = true;
    await clickNode(button("Delete permanently"));
    expect(document.body.textContent).toContain("Quotation deleted.");
    expect(reportCalls()).toHaveLength(2);
    // The mutation is visible immediately, while Reports is still hidden:
    // the cached evidence is stale and no background read has started. This
    // is what separates App's invalidation from a bare return re-entry.
    expect(reportsHidden()).toBe(true);
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "true"
    );

    await clickNode(button("Back to Home"));
    await clickNode(button("Open Reports"));
    await flush();

    // One revalidation pass over the visible required scopes only: the annual
    // Performance snapshot and the independent 2025 month source. The shared
    // all-category scope rides on the overview, and no second pass happens
    // even though the deletion also bumped the mutation epoch while hidden.
    expect(reportCalls().slice(2)).toEqual([initialQuery, annual2025]);
    expect(reportCalls()).toHaveLength(4);
    // Updated figures are visible under the unchanged selections.
    expect(document.body.textContent).toContain("Rp777.000.000");
    expect(
      button("Choose months").getAttribute("data-month-selection-summary")
    ).toBe("2 months · 2025–2026");
    expect(
      button("Choose categories").getAttribute(
        "data-category-selection-summary"
      )
    ).toBe("Fabrication");
  });

  it("marks hidden Reports stale when a PO is saved and revalidates once on return", async () => {
    const mut = armReportMutation();
    await openReports();
    await selectMonth("2026-10");
    await selectCategory("FAB");
    const query = toReportQuery(initialReportFilters(todayWIB(), "IDR"));
    expect(reportCalls()).toEqual([query]);
    expect(
      button("Choose months").getAttribute("data-month-selection-summary")
    ).toBe("2 months · 2026");
    expect(
      button("Choose categories").getAttribute(
        "data-category-selection-summary"
      )
    ).toBe("Fabrication");

    await openPurchaseOrderWorkspace();
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );

    mut();
    await invokeProbe("onSaved", poOrder({ State: "saved" }), false);

    // The App wired the save into an immediate hidden invalidation: the
    // retained figures are stale, export is refused, and nothing has been
    // fetched while Reports is off-screen.
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "true"
    );
    expect(button("Export PDF").disabled).toBe(true);
    expect(reportCalls()).toHaveLength(1);

    await clickNode(button("Back to Home"));
    await clickNode(button("Open Reports"));
    await flush();

    // One deduplicated revalidation pass for the one required scope, with new
    // figures under both unchanged local selections.
    expect(reportCalls()).toEqual([query, query]);
    expect(document.body.textContent).toContain("Rp777.000.000");
    expect(
      button("Choose months").getAttribute("data-month-selection-summary")
    ).toBe("2 months · 2026");
    expect(
      button("Choose categories").getAttribute(
        "data-category-selection-summary"
      )
    ).toBe("Fabrication");
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );
  });

  it("invalidates hidden Reports when a saved PO is corrected across category, date and currency", async () => {
    const mut = armReportMutation();
    await openReports();
    await selectMonth("2026-10");
    await selectCategory("FAB");
    const query = toReportQuery(initialReportFilters(todayWIB(), "IDR"));
    expect(reportCalls()).toEqual([query]);

    await openPurchaseOrderWorkspace();
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );

    mut();
    // A correction can move money, the reporting category, the date and the
    // currency together; App invalidates the whole cache without reading the
    // DTO's new scopes.
    await invokeProbe(
      "onSaved",
      poOrder({
        State: "saved",
        BusinessCategory: "SUPPLY_GEN",
        PODate: "2026-12-01",
        ReceivedDate: "2026-12-02",
        Currency: "USD",
      }),
      true
    );

    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "true"
    );
    expect(reportCalls()).toHaveLength(1);

    await clickNode(button("Back to Home"));
    await clickNode(button("Open Reports"));
    await flush();

    // The screen's own filters are unchanged: still IDR and All categories,
    // one pass over that scope only, with the corrected figures.
    expect(reportCalls()).toEqual([query, query]);
    expect(reportCalls()[1].currency).toBe("IDR");
    expect(reportCalls()[1].category).toBe("all");
    expect(document.body.textContent).toContain("Rp777.000.000");
    expect(
      button("Choose months").getAttribute("data-month-selection-summary")
    ).toBe("2 months · 2026");
    expect(
      button("Choose categories").getAttribute(
        "data-category-selection-summary"
      )
    ).toBe("Fabrication");
  });

  it("keeps hidden Reports fresh after a draft PO save", async () => {
    await openReports();
    await selectMonth("2026-10");
    await selectCategory("FAB");
    expect(reportCalls()).toHaveLength(1);

    await openPurchaseOrderWorkspace();
    await invokeProbe("onSaved", poOrder({ State: "draft" }), false);

    // Drafts are outside the server's saved-only report evidence.
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );
    expect(reportCalls()).toHaveLength(1);
    expect(button("Export PDF").disabled).toBe(false);
  });

  it("invalidates hidden Reports for a saved inline link but not a draft link", async () => {
    await openReports();
    await selectMonth("2026-10");
    await selectCategory("FAB");
    expect(reportCalls()).toHaveLength(1);

    await openPurchaseOrderWorkspace();
    await invokeProbe("onLinked", poOrder({ State: "draft" }));
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );
    expect(reportCalls()).toHaveLength(1);

    await invokeProbe("onLinked", poOrder({ State: "saved" }));
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "true"
    );
    expect(reportCalls()).toHaveLength(1);
  });

  it("invalidates hidden Reports after an admin source repair", async () => {
    await openReports();
    await selectMonth("2026-10");
    await selectCategory("FAB");
    expect(reportCalls()).toHaveLength(1);

    await openPurchaseOrderWorkspace();
    await invokeProbe(
      "onRepaired",
      poOrder({ QuoteNumber: "1502260046", SourceRevision: 2 })
    );

    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "true"
    );
    expect(reportCalls()).toHaveLength(1);
  });

  it("marks hidden Reports stale after a confirmed saved-PO deletion", async () => {
    mocks.restoreSession.mockResolvedValue({ ...USER, Role: "admin" });
    mocks.currentUser.mockResolvedValue({ ...USER, Role: "admin" });
    mocks.listPurchaseOrders.mockResolvedValue(poListPage([poRow()]));
    mocks.previewDeletion.mockResolvedValue(poDeletionPreview());
    mocks.confirmDeletion.mockResolvedValue({
      Outcome: { Removed: [poDeletionRecord()], AuditIDs: [1] },
    });
    const mut = armReportMutation();

    await openReports();
    await selectMonth("2026-10");
    await selectCategory("FAB");
    expect(reportCalls()).toHaveLength(1);

    await openPurchaseOrderArchive();
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );

    mut();
    await openPoDeletionDialog();
    await confirmPoDeletion();

    // Still on the archive page: the deletion has already marked the hidden
    // report stale and started no background read.
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "true"
    );
    expect(reportCalls()).toHaveLength(1);
  });

  it("keeps hidden Reports fresh when a draft PO is deleted", async () => {
    mocks.restoreSession.mockResolvedValue({ ...USER, Role: "admin" });
    mocks.currentUser.mockResolvedValue({ ...USER, Role: "admin" });
    mocks.listPurchaseOrders.mockResolvedValue(
      poListPage([poRow({ State: "draft" })])
    );
    mocks.previewDeletion.mockResolvedValue(
      poDeletionPreview({ State: "draft" })
    );
    mocks.confirmDeletion.mockResolvedValue({
      Outcome: { Removed: [poDeletionRecord({ State: "draft" })], AuditIDs: [1] },
    });

    await openReports();
    await selectMonth("2026-10");
    await selectCategory("FAB");
    expect(reportCalls()).toHaveLength(1);

    await openPurchaseOrderArchive();
    await openPoDeletionDialog();
    await confirmPoDeletion();

    // A draft is not report evidence, so the hidden page is not invalidated.
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );
    expect(reportCalls()).toHaveLength(1);
  });

  it("keeps hidden Reports fresh when a saved-PO deletion is cancelled or fails", async () => {
    mocks.restoreSession.mockResolvedValue({ ...USER, Role: "admin" });
    mocks.currentUser.mockResolvedValue({ ...USER, Role: "admin" });
    mocks.listPurchaseOrders.mockResolvedValue(poListPage([poRow()]));
    mocks.previewDeletion.mockResolvedValue(poDeletionPreview());

    await openReports();
    await selectMonth("2026-10");
    await selectCategory("FAB");
    expect(reportCalls()).toHaveLength(1);

    await openPurchaseOrderArchive();
    await openPoDeletionDialog();
    await clickNode(buttonWithText("Cancel"));
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );

    mocks.confirmDeletion.mockRejectedValue(new Error("the record is locked"));
    await openPoDeletionDialog();
    await confirmPoDeletion();
    expect(document.body.textContent).toContain("the record is locked");
    expect(monthBrowser().getAttribute("data-month-browser-stale")).toBe(
      "false"
    );
    expect(reportCalls()).toHaveLength(1);
  });

  it("clears Reports when the session poll adopts a different account", async () => {
    const polls = pollControl();
    try {
      mocks.checkSession.mockResolvedValue({ State: "active", User: USER });
      await openReports();
      await selectMonth("2026-10");
      expect(
        button("Choose months").getAttribute("data-month-selection-summary")
      ).toBe("2 months · 2026");
      const requestsBefore = reportCalls().length;

      mocks.checkSession.mockResolvedValue({
        State: "active",
        User: { ...USER, ID: 99, DisplayName: "Someone Else" },
      });
      await polls.run();

      // The whole authenticated workspace was discarded: Reports unmounted and
      // the shell returned Home.
      expect(document.getElementById("reports-heading")).toBeNull();
      expect(document.body.textContent).toContain("Open Reports");

      // Visiting again starts a fresh mount with the default selection; the
      // previous account's cached evidence and selection are gone.
      await clickNode(button("Open Reports"));
      await flush();
      expect(reportCalls().length).toBe(requestsBefore + 1);
      expect(
        button("Choose months").getAttribute("data-month-selection-summary")
      ).toBe("November 2026");
    } finally {
      polls.restore();
    }
  });

  it("keeps Reports and its selections through a same-account renewal", async () => {
    const polls = pollControl();
    try {
      const expiryIn = (ms: number) => new Date(Date.now() + ms).toISOString();
      mocks.checkSession.mockResolvedValue({
        State: "active",
        User: USER,
        Code: "session_active",
        ExpiresAt: expiryIn(5 * 60 * 1000),
        CanRenew: true,
      });
      mocks.renewSession.mockResolvedValue({
        State: "active",
        User: USER,
        Code: "session_active",
        ExpiresAt: expiryIn(8 * 60 * 60 * 1000),
        CanRenew: true,
      });

      await openReports();
      await selectMonth("2026-10");
      const requestsBefore = reportCalls().length;
      await polls.run();

      const renew = Array.from(
        document.querySelectorAll<HTMLButtonElement>("button")
      ).find((node) => node.textContent?.trim() === "Stay signed in");
      if (renew === undefined) throw new Error("no Stay signed in button");
      await clickNode(renew);

      expect(mocks.renewSession).toHaveBeenCalledTimes(1);
      expect(document.getElementById("reports-heading")).not.toBeNull();
      expect(
        button("Choose months").getAttribute("data-month-selection-summary")
      ).toBe("2 months · 2026");
      // Adopting the same account is not a freshness event.
      expect(reportCalls()).toHaveLength(requestsBefore);
    } finally {
      polls.restore();
    }
  });
});

// #683: Reports is admin-only. Home never offers it to staff, an admin session
// still opens it as its own workspace (and never the Admin Dashboard), and a
// session demoted to staff while Reports is open falls back to Home without
// leaving the report page mounted behind it.
describe("admin-only Reports gate through the real App (issue #683)", () => {
  it("offers no Reports or Admin card on Home to staff", async () => {
    mocks.restoreSession.mockResolvedValue(STAFF);
    mocks.currentUser.mockResolvedValue(STAFF);

    await renderApp();

    expect(document.querySelector('button[aria-label="Open Reports"]')).toBeNull();
    expect(
      document.querySelector('button[aria-label="Open Admin Dashboard"]')
    ).toBeNull();
    expect(button("Open Quotation")).toBeDefined();
    expect(button("Open Purchase Orders")).toBeDefined();
  });

  it("opens Reports for an admin without mounting the Admin Dashboard", async () => {
    await openReports();

    expect(document.getElementById("reports-heading")?.textContent).toBe("Reports");
    expect(document.querySelector('[aria-label="Clients section"]')).toBeNull();
  });

  it("returns a demoted session to Home and unmounts the open Reports page", async () => {
    const polls = pollControl();
    try {
      await openReports();
      expect(document.getElementById("reports-heading")).not.toBeNull();

      // Same account, new role: the workspace is kept, so the page state still
      // says "reports" while the session is no longer an admin.
      mocks.checkSession.mockResolvedValue({
        State: "active",
        User: { ...USER, Role: "staff" },
      });
      await polls.run();

      expect(document.getElementById("reports-heading")).toBeNull();
      expect(document.body.textContent).toContain("Workspaces");
      expect(button("Open Quotation")).toBeDefined();
      expect(document.querySelector('button[aria-label="Open Reports"]')).toBeNull();
    } finally {
      polls.restore();
    }
  });
});
