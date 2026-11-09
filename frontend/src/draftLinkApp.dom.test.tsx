// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { ToastProvider } from "./toast";
import type { PurchaseOrderDraftLinkInput } from "./poDraftLink";
import type { PurchaseOrderDTO, PurchaseOrderUpdateInput } from "./purchaseOrder";

// Issue #497: the real top-level App wiring, not a re-implementation of it.
// App is rendered with only the desktop bindings replaced. Home -> Purchase
// Orders -> Link a Quotation opens the shared entry selector; confirming the
// exact pair hands it to the real editor, which saves the working copy, loads
// the review, and commits the link. This is the selector -> review -> link path
// in the production composition.

const mocks = vi.hoisted(() => ({
  restoreSession: vi.fn(),
  currentUser: vi.fn(),
  getCompany: vi.fn(),
  logout: vi.fn(),
  listDrafts: vi.fn(),
  listCustomers: vi.fn(),
  listPOSources: vi.fn(),
  listPORevisions: vi.fn(),
  validatePOSource: vi.fn(),
  createBlank: vi.fn(),
  updateDraft: vi.fn(),
  previewLink: vi.fn(),
  linkSource: vi.fn(),
  documentState: vi.fn(),
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
    ListPOSourceQuotations: (...args: unknown[]) => mocks.listPOSources(...args),
    ListPOSourceRevisions: (...args: unknown[]) => mocks.listPORevisions(...args),
    ValidatePOSource: (...args: unknown[]) => mocks.validatePOSource(...args),
    CreatePurchaseOrderBlankDraft: (...args: unknown[]) => mocks.createBlank(...args),
    UpdatePurchaseOrderDraft: (...args: unknown[]) => mocks.updateDraft(...args),
    PreviewPurchaseOrderDraftLink: (...args: unknown[]) => mocks.previewLink(...args),
    LinkPurchaseOrderDraftSource: (...args: unknown[]) => mocks.linkSource(...args),
    GetPurchaseOrderDocumentState: (...args: unknown[]) => mocks.documentState(...args),
    GetPurchaseOrderHistory: vi.fn(async () => []),
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

const USD = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before" as const,
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function blank(): PurchaseOrderDTO {
  return {
    PODate: null,
    RequestedDeliveryDate: null,
    UUID: "",
    State: "draft",
    QuoteNumber: "",
    CustomerName: "",
    BillingAddress: "",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "",
    ReceivedDate: "2026-10-22",
    Incoterms: "EXW",
    Currency: "IDR",
    TaxZone: "FTZ",
    TaxType: "Goods",
    ServiceCategory: "",
    TaxCategory: "",
    AdjustmentReason: "",
    CreatedAt: "",
    UpdatedAt: "",
    Items: [],
    ID: 0,
    QuotationID: 0,
    SourceRevision: 0,
    CustomerID: 0,
    PaymentTerms: "",
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
    SourceMissing: true,
    SourceState: "never",
    PrintedTotalsKnown: false,
    ConfirmationNumber: null,
    ConfirmationAssignedAt: null,
    ConfirmationWarn: false,
    Version: 0,
  };
}

let latestOrder: PurchaseOrderDTO;
let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockReset();
  latestOrder = { ...blank(), ID: 41, UUID: "uuid-497-app", Version: 1 };

  mocks.restoreSession.mockResolvedValue(USER);
  mocks.currentUser.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.getProfile.mockResolvedValue(PROFILE);
  mocks.listDrafts.mockResolvedValue([]);
  mocks.listCustomers.mockResolvedValue([]);
  mocks.getUnits.mockResolvedValue([{ Value: "Lot", Label: "Lot" }]);
  mocks.getCatalogue.mockResolvedValue([IDR, USD]);
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
  mocks.listPOSources.mockResolvedValue({
    Quotations: [
      {
        ID: 12,
        QuoteNumber: "1502260001",
        Subject: "Link source",
        CustomerName: "PT. Source",
        LatestRevision: 0,
        IssuedRevisionCount: 1,
        ActivePO: null,
      },
    ],
    Page: 1,
    PageSize: 15,
    Total: 1,
    TotalPages: 1,
  });
  mocks.listPORevisions.mockResolvedValue({
    Revisions: [{ Revision: 0, IssuedAt: "2026-10-20T10:00:00Z" }],
  });
  mocks.validatePOSource.mockResolvedValue({ Valid: true, Reason: "" });
  mocks.createBlank.mockImplementation(async () => latestOrder);
  mocks.updateDraft.mockImplementation(async (input: PurchaseOrderUpdateInput) => {
    latestOrder = {
      ...latestOrder,
      ID: input.ID,
      Version: input.ExpectedVersion + 1,
      PONumber: input.PONumber,
      BillingAddress: input.BillingAddress,
      DeliveryAddress: input.DeliveryAddress,
      ShipToCode: input.ShipToCode,
      ContactEmail: input.ContactEmail,
      ContactPhone: input.ContactPhone,
      ReceivedDate: input.ReceivedDate,
      PaymentTerms: input.PaymentTerms,
      Incoterms: input.Incoterms,
      DiscountPercent: input.DiscountPercent,
      AdjustmentMinor: input.AdjustmentMinor,
      AdjustmentReason: input.AdjustmentReason,
      Items: input.Items.map((item, index) => ({
        QuotationItemNo: item.QuotationItemNo ?? null,
        QuotationSubNo: item.QuotationSubNo ?? null,
        LineNumber: index + 1,
        Description: item.Description,
        QuantityScaled: item.QuantityScaled,
        UoM: item.UoM,
        UnitPriceMinor: item.UnitPriceMinor,
        DiscountEnabled: item.DiscountEnabled,
        DiscountPercent: item.DiscountPercent,
        GrossMinor: 0,
        DiscountMinor: 0,
        NetMinor: 0,
      })),
    };
    return { Order: latestOrder };
  });
  mocks.previewLink.mockResolvedValue({
    Order: latestOrder,
    Source: {
      QuotationID: 12,
      RevisionNumber: 0,
      QuoteNumber: "1502260001",
      CustomerID: 2,
      CustomerName: "PT. Source",
      Currency: "IDR",
      PaymentTerms: "",
      Incoterms: "EXW",
      DiscountPercent: 0,
      Lines: [
        {
          QuotationItemNo: 1,
          QuotationSubNo: null,
          LineLabel: "1",
          Description: "Source line 1",
          UoM: "Pc",
          QuantityScaled: 10000,
          UnitPriceMinor: 100,
          DiscountEnabled: false,
          DiscountPercent: 0,
        },
      ],
      PrintedSubtotalMinor: 100,
      PrintedDiscountMinor: 0,
      PrintedPpnMinor: 0,
      PrintedGrandTotalMinor: 100,
    },
    DocumentState: "none",
    DraftTax: {
      TaxZone: "FTZ",
      TaxType: "Goods",
      ServiceCategory: "",
      TaxCategory: "",
      PpnBps: 0,
    },
    SourceTax: {
      TaxZone: "FTZ",
      TaxType: "Goods",
      ServiceCategory: "",
      TaxCategory: "",
      PpnBps: 0,
    },
    TaxChoiceRequired: false,
  });
  mocks.linkSource.mockImplementation(async (input: PurchaseOrderDraftLinkInput) => {
    latestOrder = {
      ...latestOrder,
      ID: input.ID,
      Version: input.ExpectedVersion + 1,
      State: "draft",
      SourceState: "live",
      SourceMissing: false,
      PrintedTotalsKnown: true,
      QuotationID: 12,
      SourceRevision: 0,
      QuoteNumber: "1502260001",
      CustomerID: 2,
      CustomerName: "PT. Source",
      PrintedSubtotalMinor: 100,
      PrintedDiscountMinor: 0,
      PrintedPpnMinor: 0,
      PrintedGrandTotalMinor: 100,
    };
    return { Order: latestOrder };
  });
  mocks.documentState.mockResolvedValue({ Document: null, Versions: [] });

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

function input(label: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    `input[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no input labelled ${label}`);
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

describe("draft source link through the real App (issue #497)", () => {
  it("selects, reviews and links from Home -> Purchase Orders", async () => {
    await renderApp();

    act(() => button("Open Purchase Orders").click());
    await flush();
    act(() => button("Link a Quotation").click());
    await flush();

    // Step 1: the exact quotation, then its issued revision.
    act(() => button("Quotation 1502260001, PT. Source").click());
    await flush();
    act(() => button("Revision 0").click());
    await flush();
    act(() => button("Continue to confirmation").click());
    await flush();
    act(() => button("Confirm source").click());
    await flush();
    await flush();

    // The editor persisted the working copy and loaded the review for the
    // confirmed pair; the shared selector was in entry mode, not repair.
    expect(mocks.createBlank).toHaveBeenCalledTimes(1);
    expect(mocks.updateDraft).toHaveBeenCalledTimes(1);
    expect(mocks.previewLink).toHaveBeenCalledWith(41, 12, 0);
    expect(mocks.listPOSources).toHaveBeenCalled();
    expect(document.body.textContent).toContain("Link this draft to a quotation");

    act(() => input("Acknowledge omission of quotation line 1").click());
    await flush();
    act(() => button("Link source").click());
    await flush();
    await flush();

    expect(mocks.linkSource).toHaveBeenCalledTimes(1);
    const linkInput = mocks.linkSource.mock.calls[0][0] as PurchaseOrderDraftLinkInput;
    expect(linkInput.ID).toBe(41);
    expect(linkInput.TargetQuotationID).toBe(12);
    expect(linkInput.TargetRevisionNumber).toBe(0);
    expect(linkInput.AcknowledgedMissing).toEqual(["1"]);
    expect(document.body.textContent).toContain("Linked source");
    expect(document.body.textContent).toContain(
      "Linked to quotation 1502260001, revision 0. Your entered values were kept."
    );
  });
});
