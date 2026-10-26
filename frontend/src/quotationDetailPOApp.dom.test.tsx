// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { ToastProvider } from "./toast";
import type { PurchaseOrderDTO } from "./purchaseOrder";

// Issue #499: the real top-level App, not a re-implementation of it. From Home
// the test opens Quotation -> Archive -> one issued Quotation, then acts on the
// archived detail page. Only the desktop bindings are mocked, so the labels,
// the exact displayed revision passed to the source-bound create call, the
// workspace transition, and the occupied-source alternatives are the production
// composition.
//
// Covered here:
//   - free Quotation at its latest revision: Create PO from this Quotation
//     creates from that exact revision and opens the editor;
//   - viewing an older revision: the action follows the displayed revision;
//   - an occupied draft: Open PO draft, no creation attempt;
//   - a finalized PO: View PO, no creation attempt;
//   - a concurrent create answering 409: the page re-reads occupancy and
//     switches to Open PO draft instead of retrying.
//
// Not covered here (hand-driven): keyboard focus after the workspace switch,
// and the real HTTP 409 arriving from a live server.

const mocks = vi.hoisted(() => ({
  restoreSession: vi.fn(),
  currentUser: vi.fn(),
  getCompany: vi.fn(),
  logout: vi.fn(),
  listDrafts: vi.fn(),
  listCustomers: vi.fn(),
  listQuotations: vi.fn(),
  getRevision: vi.fn(),
  getOccupancy: vi.fn(),
  createDraft: vi.fn(),
  getPurchaseOrder: vi.fn(),
  getUnits: vi.fn(),
  getCatalogue: vi.fn(),
  getProfile: vi.fn(),
  getLogo: vi.fn(),
  getLabels: vi.fn(),
  listClients: vi.fn(),
  listUsers: vi.fn(),
  listDeletionRequests: vi.fn(),
  listEmailChangeRequests: vi.fn(),
  myDeletionRequests: vi.fn(),
  myEmailChangeRequests: vi.fn(),
  version: vi.fn(),
  changelog: vi.fn(),
  updates: vi.fn(),
}));

vi.mock("@backend/backend/AuthAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AuthAPI")>();
  return {
    ...actual,
    RestoreSession: (...args: unknown[]) => mocks.restoreSession(...args),
    CurrentUser: (...args: unknown[]) => mocks.currentUser(...args),
    GetCompany: (...args: unknown[]) => mocks.getCompany(...args),
    Logout: (...args: unknown[]) => mocks.logout(...args),
    MyDeletionRequests: (...args: unknown[]) => mocks.myDeletionRequests(...args),
    MyEmailChangeRequests: (...args: unknown[]) => mocks.myEmailChangeRequests(...args),
  };
});

vi.mock("@backend/backend/AdminAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AdminAPI")>();
  return {
    ...actual,
    GetCompanyUnits: (...args: unknown[]) => mocks.getUnits(...args),
    GetCurrencyCatalogue: (...args: unknown[]) => mocks.getCatalogue(...args),
    GetCompanyProfile: (...args: unknown[]) => mocks.getProfile(...args),
    GetCompanyLogo: (...args: unknown[]) => mocks.getLogo(...args),
    GetCompanyLabels: (...args: unknown[]) => mocks.getLabels(...args),
    ListClients: (...args: unknown[]) => mocks.listClients(...args),
    ListUsers: (...args: unknown[]) => mocks.listUsers(...args),
    ListDeletionRequests: (...args: unknown[]) => mocks.listDeletionRequests(...args),
    ListEmailChangeRequests: (...args: unknown[]) => mocks.listEmailChangeRequests(...args),
  };
});

vi.mock("@backend/backend/Quotation", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/Quotation")>();
  return {
    ...actual,
    ClearDraftContext: vi.fn(async () => {}),
    ClearRevisionContext: vi.fn(async () => {}),
    SetDraftContext: vi.fn(async () => {}),
    SetRevisionContext: vi.fn(async () => {}),
    ListDrafts: (...args: unknown[]) => mocks.listDrafts(...args),
    ListCustomers: (...args: unknown[]) => mocks.listCustomers(...args),
    ListQuotations: (...args: unknown[]) => mocks.listQuotations(...args),
    GetQuotationRevision: (...args: unknown[]) => mocks.getRevision(...args),
    GetPOSourceOccupancy: (...args: unknown[]) => mocks.getOccupancy(...args),
    CreatePurchaseOrderDraft: (...args: unknown[]) => mocks.createDraft(...args),
    GetPurchaseOrder: (...args: unknown[]) => mocks.getPurchaseOrder(...args),
    GetPurchaseOrderHistory: vi.fn(async () => []),
    GetPurchaseOrderDocumentState: vi.fn(async () => ({ Document: null, Versions: [] })),
    AttachPurchaseOrderDocument: vi.fn(),
    RemovePurchaseOrderDocument: vi.fn(),
    FinalizePurchaseOrder: vi.fn(),
    CorrectPurchaseOrder: vi.fn(),
    GetPurchaseOrderReview: vi.fn(),
  };
});

vi.mock("@backend/backend/UpdateAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/UpdateAPI")>();
  return {
    ...actual,
    Version: (...args: unknown[]) => mocks.version(...args),
    GetChangelog: (...args: unknown[]) => mocks.changelog(...args),
    CheckForUpdates: (...args: unknown[]) => mocks.updates(...args),
  };
});

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const USER = {
  ID: 1,
  Email: "admin@example.com",
  DisplayName: "Ada Admin",
  Role: "admin",
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

const IDR = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before" as const,
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const QUOTATION_ID = 12;
const QUOTE_NUMBER = "1502260012";

function summary() {
  return {
    ID: QUOTATION_ID,
    QuoteNumber: QUOTE_NUMBER,
    RevisionNumber: 2,
    CustomerName: "PT. Infineon Technologies Batam",
    Subject: "Electrical installation",
    Currency: "IDR",
    CreatedAt: "2026-10-20T10:00:00Z",
    GrandTotal: 100,
    DisplayGrand: "Rp100",
  };
}

function archived(revision: number) {
  return {
    ...summary(),
    ID: QUOTATION_ID,
    RevisionNumber: revision,
    CustomerCode: 2,
    CustomerAddress: "Jl. Klien 12",
    CreatedAt: "2026-10-20T10:00:00Z",
    DisplayGrand: "Rp100",
    RecipientName: "Jane",
    RecipientAddress: "Gedung Penerima 3",
    Incoterm: "DAP",
    ValidityDays: 45,
    PaymentTermDays: 60,
    DisplayLeadTime: "-",
    LeadTimeYears: 0,
    LeadTimeMonths: 1,
    LeadTimeWeeks: 0,
    LeadTimeDays: 0,
    DiscountPercent: 0,
    TaxZone: "FTZ",
    TaxCategory: "",
    TaxType: "Goods",
    ServiceCategory: "",
    TaxRateBps: 0,
    OriginalSubtotal: 0,
    OriginalDiscount: 0,
    OriginalGrand: 0,
    MarginalizedSubtotal: 0,
    MarginalizedDiscount: 0,
    MarginalizedGrand: 0,
    PpnAmount: 0,
    DisplayPpn: "",
    TaxAmount: 0,
    MarginalizedAfterTax: 0,
    ProfitAmount: 0,
    NettProfit: 0,
    DisplayOriginalSubtotal: "",
    DisplayOriginalDiscount: "",
    DisplayOriginalGrand: "",
    DisplayMarginalizedSubtotal: "",
    DisplayMarginalizedDiscount: "",
    DisplayMarginalizedGrand: "",
    DisplayTax: "",
    DisplayMarginalizedAfterTax: "",
    DisplayProfit: "",
    DisplayNett: "",
    HTMLSnapshot: "",
    AvailableRevisions: [2, 1, 0],
    Items: [],
  };
}

function draftFor(revision: number): PurchaseOrderDTO {
  return {
    PODate: null,
    RequestedDeliveryDate: null,
    UUID: "uuid-po-499",
    State: "draft",
    QuoteNumber: QUOTE_NUMBER,
    CustomerName: "PT. Infineon Technologies Batam",
    BillingAddress: "Jl. Klien 12",
    DeliveryAddress: "Jl. Klien 12",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "",
    ReceivedDate: "2026-10-22",
    Incoterms: "DAP",
    Currency: "IDR",
    TaxZone: "FTZ",
    TaxType: "Goods",
    ServiceCategory: "",
    TaxCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-22T10:00:00Z",
    UpdatedAt: "2026-10-22T10:00:00Z",
    Items: [],
    ID: 41,
    QuotationID: QUOTATION_ID,
    SourceRevision: revision,
    CustomerID: 2,
    PaymentTerms: "60 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 0,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 0,
    PrintedSubtotalMinor: 0,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 0,
    SavedAt: null,
    SavedBy: null,
    PdfAbsentConfirmed: false,
    SourceMissing: false,
    SourceState: "live",
    PrintedTotalsKnown: true,
    ConfirmationNumber: null,
    ConfirmationAssignedAt: null,
    ConfirmationWarn: false,
    Version: 1,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockReset();

  mocks.restoreSession.mockResolvedValue(USER);
  mocks.currentUser.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.getProfile.mockResolvedValue(PROFILE);
  mocks.listDrafts.mockResolvedValue([]);
  mocks.listCustomers.mockResolvedValue([]);
  mocks.listQuotations.mockResolvedValue([summary()]);
  mocks.getRevision.mockImplementation(async (_id: number, revision: number) =>
    archived(revision)
  );
  mocks.getOccupancy.mockImplementation(async () => ({
    QuotationID: QUOTATION_ID,
    ActivePO: null,
  }));
  mocks.createDraft.mockImplementation(
    async (input: { QuotationID: number; RevisionNumber: number }) =>
      draftFor(input.RevisionNumber)
  );
  mocks.getPurchaseOrder.mockImplementation(async () => draftFor(0));
  mocks.getUnits.mockResolvedValue([{ Value: "Lot", Label: "Lot" }]);
  mocks.getCatalogue.mockResolvedValue([IDR]);
  mocks.getLogo.mockResolvedValue({ LogoBase64: "", LogoMime: "", BuiltIn: true });
  mocks.getLabels.mockResolvedValue([]);
  mocks.listClients.mockResolvedValue([]);
  mocks.listUsers.mockResolvedValue([]);
  mocks.listDeletionRequests.mockResolvedValue([]);
  mocks.listEmailChangeRequests.mockResolvedValue([]);
  mocks.myDeletionRequests.mockResolvedValue([]);
  mocks.myEmailChangeRequests.mockResolvedValue([]);
  mocks.version.mockResolvedValue("v0.0.0");
  mocks.changelog.mockResolvedValue("");
  mocks.updates.mockResolvedValue({ UpdateAvailable: false });

  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  Element.prototype.scrollIntoView = vi.fn();
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

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function textButton(label: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll("button")).find(
    (candidate) => candidate.textContent?.trim() === label
  );
  if (found === undefined) {
    throw new Error(
      `no button reading ${label}; saw ${Array.from(
        document.querySelectorAll("button")
      )
        .map((candidate) => candidate.textContent?.trim())
        .filter(Boolean)
        .join(" | ")}`
    );
  }
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

/** Home -> Quotation -> Archive -> the one issued Quotation. */
async function openArchivedQuotation(): Promise<void> {
  await renderApp();
  act(() => button("Open Quotation").click());
  await flush();
  act(() => textButton("Archive").click());
  await flush();
  act(() => textButton(QUOTE_NUMBER).click());
  await flush();
  await flush();
}

describe("create PO from the displayed Quotation revision (issue #499)", () => {
  it("creates from the displayed latest revision and opens the PO editor", async () => {
    await openArchivedQuotation();

    // The occupancy read is per Quotation, before any revision is chosen.
    expect(mocks.getOccupancy).toHaveBeenCalledWith(QUOTATION_ID);
    expect(document.body.textContent).toContain("Create PO from this Quotation");
    expect(document.body.textContent).not.toContain("Open PO draft");
    expect(document.body.textContent).not.toContain("View PO");

    act(() => button("Create PO from this Quotation").click());
    await flush();
    await flush();

    expect(mocks.createDraft).toHaveBeenCalledTimes(1);
    expect(mocks.createDraft.mock.calls[0][0]).toEqual(
      expect.objectContaining({ QuotationID: QUOTATION_ID, RevisionNumber: 2 })
    );
    // No selector, no second revision choice: the create path opened the
    // editor for the record it just made.
    expect(document.body.textContent).toContain(
      `Purchase order draft created from quotation ${QUOTE_NUMBER}, revision 2.`
    );
    expect(document.body.textContent).toContain("Save draft");
    expect(document.body.textContent).not.toContain("Create PO from this Quotation");
  });

  it("uses the older revision on screen, not the latest one", async () => {
    await openArchivedQuotation();

    // Switch the revision selector to revision 00 and wait for its load.
    act(() => button("Quotation revision").click());
    await flush();
    act(() =>
      Array.from(document.querySelectorAll<HTMLElement>('[role="option"]'))
        .find((option) => option.textContent?.trim() === "Revision 00")!
        .click()
    );
    await flush();
    await flush();

    expect(mocks.getRevision).toHaveBeenCalledWith(QUOTATION_ID, 0);
    act(() => button("Create PO from this Quotation").click());
    await flush();
    await flush();

    expect(mocks.createDraft).toHaveBeenCalledTimes(1);
    expect(mocks.createDraft.mock.calls[0][0]).toEqual(
      expect.objectContaining({ QuotationID: QUOTATION_ID, RevisionNumber: 0 })
    );
  });

  it("opens the occupying draft instead of creating a second PO", async () => {
    mocks.getOccupancy.mockResolvedValue({
      QuotationID: QUOTATION_ID,
      ActivePO: { ID: 41, State: "draft" },
    });
    await openArchivedQuotation();

    expect(document.body.textContent).toContain("Open PO draft");
    expect(document.body.textContent).not.toContain("Create PO from this Quotation");

    act(() => button("Open PO draft").click());
    await flush();
    await flush();

    expect(mocks.getPurchaseOrder).toHaveBeenCalledWith(41);
    expect(mocks.createDraft).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("Save draft");
  });

  it("views the finalized PO instead of creating a second PO", async () => {
    mocks.getOccupancy.mockResolvedValue({
      QuotationID: QUOTATION_ID,
      ActivePO: { ID: 41, State: "saved" },
    });
    mocks.getPurchaseOrder.mockImplementation(async () => {
      const saved = draftFor(0);
      return {
        ...saved,
        State: "saved",
        PONumber: "PO-499",
        Version: 3,
        SavedAt: "2026-10-22T10:00:00Z",
        ConfirmationNumber: "2302260001",
        ConfirmationAssignedAt: "2026-10-22T10:00:00Z",
        PdfAbsentConfirmed: true,
      };
    });
    await openArchivedQuotation();

    expect(document.body.textContent).toContain("View PO");
    expect(document.body.textContent).not.toContain("Create PO from this Quotation");
    expect(document.body.textContent).not.toContain("Open PO draft");

    act(() => button("View PO").click());
    await flush();
    await flush();

    expect(mocks.getPurchaseOrder).toHaveBeenCalledWith(41);
    expect(mocks.createDraft).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("Saved record 41");
  });

  it("turns a concurrent occupied create into Open PO draft, never a retry", async () => {
    mocks.getOccupancy
      .mockResolvedValueOnce({ QuotationID: QUOTATION_ID, ActivePO: null })
      .mockResolvedValue({
        QuotationID: QUOTATION_ID,
        ActivePO: { ID: 41, State: "draft" },
      });
    mocks.createDraft.mockRejectedValue(
      new Error("this quotation already has a purchase order")
    );
    await openArchivedQuotation();

    act(() => button("Create PO from this Quotation").click());
    await flush();
    await flush();

    // One attempt only: the 409 is not retried, and the page adopts the
    // record the server says now occupies the Quotation.
    expect(mocks.createDraft).toHaveBeenCalledTimes(1);
    expect(mocks.getOccupancy).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("Open PO draft");
    expect(document.body.textContent).not.toContain("Create PO from this Quotation");
    expect(document.body.textContent).toContain(
      "This quotation already has a purchase order draft. Use Open PO draft to continue it."
    );
  });

  it("returns to the loaded revision after a failed older-revision switch, and creates only what is shown", async () => {
    // R4: a failed switch used to leave the selector on Revision 00 while the
    // mounted record, its Edit label and the create payload stayed revision 02.
    // The failure must put the selector back on the record that is displayed,
    // and Create must never fire while the two disagree.
    let rejectOlder!: (error: unknown) => void;
    mocks.getRevision.mockImplementation((_id: number, revision: number) => {
      if (revision === 0) {
        return new Promise((_resolve, reject) => {
          rejectOlder = reject;
        });
      }
      return Promise.resolve(archived(revision));
    });
    await openArchivedQuotation();
    expect(button("Create PO from this Quotation").disabled).toBe(false);

    act(() => button("Quotation revision").click());
    await flush();
    act(() =>
      Array.from(document.querySelectorAll<HTMLElement>('[role="option"]'))
        .find((option) => option.textContent?.trim() === "Revision 00")!
        .click()
    );
    await flush();

    // The older revision is still loading: the action cannot fire against the
    // record that is still mounted.
    expect(button("Quotation revision").textContent).toContain("Revision 00");
    expect(button("Create PO from this Quotation").disabled).toBe(true);

    // The fetch fails. The selector returns to the revision the page is
    // actually showing instead of claiming the older one.
    act(() => rejectOlder(new Error("revision unavailable")));
    await flush();
    await flush();

    expect(button("Quotation revision").textContent).toContain(
      "Revision 02 — latest"
    );
    expect(button("Quotation revision").textContent).not.toContain("Revision 00");
    expect(document.body.textContent).toContain("Edit revision 02");
    // The rollback re-reads the record the selector now names.
    expect(mocks.getRevision).toHaveBeenCalledTimes(3);
    expect(mocks.getRevision).toHaveBeenLastCalledWith(QUOTATION_ID, 2);
    expect(button("Create PO from this Quotation").disabled).toBe(false);

    act(() => button("Create PO from this Quotation").click());
    await flush();
    await flush();

    expect(mocks.createDraft).toHaveBeenCalledTimes(1);
    expect(mocks.createDraft.mock.calls[0][0]).toEqual(
      expect.objectContaining({ QuotationID: QUOTATION_ID, RevisionNumber: 2 })
    );
  });
});
