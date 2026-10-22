// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import type { PurchaseOrderDTO } from "./purchaseOrder";
import {
  DEFAULT_PURCHASE_ORDER_SETTINGS,
  DEFAULT_QUOTATION_SETTINGS,
  quotationSettingsKey,
  purchaseOrderSettingsKey,
  writeQuotationSettings,
  writePurchaseOrderSettings,
} from "./workspaceSettings";
import { ToastProvider } from "./toast";

// Issue #492: the real App with only the desktop bindings replaced. These
// cases prove the production wiring -- the profile menu reaches the two
// settings pages, edits persist per account, a new quotation starts from the
// defaults while a saved draft keeps its own values, and a newly created PO
// takes the defaults only where the source revision supplied nothing.
//
// What this cannot show: the archive's page size has no visible surface (the
// server is fixed at 15 rows), so its rule is pinned in poList.test.ts; and
// Select menus are driven by pointer/keyboard by hand, so the tests set the
// values through the controls' own callbacks where a Select is involved.

const mocks = vi.hoisted(() => ({
  restoreSession: vi.fn((..._args: unknown[]): unknown => undefined),
  currentUser: vi.fn((..._args: unknown[]): unknown => undefined),
  logout: vi.fn(async (..._args: unknown[]) => {}),
  getCompany: vi.fn((..._args: unknown[]): unknown => undefined),
  getCompanyUnits: vi.fn(async (..._args: unknown[]) => [{ Value: "Lot", Label: "Lot" }]),
  getCurrencyCatalogue: vi.fn(async (..._args: unknown[]) => [
    {
      code: "IDR",
      exponent: 0,
      symbol: "Rp",
      position: "before",
      groupSep: ".",
      decimalSep: ",",
      symbolSpaced: false,
    },
  ]),
  listDrafts: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listCustomers: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  getDraft: vi.fn((..._args: unknown[]): unknown => undefined),
  getCompanyProfile: vi.fn(async (..._args: unknown[]) => ({})),
  getCompanyLogo: vi.fn(async (..._args: unknown[]) => ({ LogoBase64: "", LogoMime: "", BuiltIn: true })),
  getCompanyLabels: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listClients: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listUsers: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listDeletionRequests: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  listEmailChangeRequests: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  myDeletionRequests: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  myEmailChangeRequests: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  version: vi.fn(async (..._args: unknown[]) => ""),
  getChangelog: vi.fn(async (..._args: unknown[]) => ""),
  checkForUpdates: vi.fn(async (..._args: unknown[]) => ({ UpdateAvailable: false })),
  listPOSourceQuotations: vi.fn((..._args: unknown[]): unknown => undefined),
  listPOSourceRevisions: vi.fn((..._args: unknown[]): unknown => undefined),
  validatePOSource: vi.fn(async (..._args: unknown[]) => ({ Valid: true, Reason: "" })),
  createPurchaseOrderDraft: vi.fn((..._args: unknown[]): unknown => undefined),
  getPurchaseOrderExtraction: vi.fn(async (..._args: unknown[]) => ({
    Run: null,
    Document: null,
  })),
  getPurchaseOrderDocumentState: vi.fn(async (..._args: unknown[]) => ({
    Document: null,
    Versions: [],
  })),
}));

vi.mock("@backend/backend/AuthAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AuthAPI")>();
  return {
    ...actual,
    RestoreSession: (...args: unknown[]) => mocks.restoreSession(...args),
    CurrentUser: (...args: unknown[]) => mocks.currentUser(...args),
    Logout: (...args: unknown[]) => mocks.logout(...args),
    GetCompany: (...args: unknown[]) => mocks.getCompany(...args),
    MyDeletionRequests: (...args: unknown[]) => mocks.myDeletionRequests(...args),
    MyEmailChangeRequests: (...args: unknown[]) => mocks.myEmailChangeRequests(...args),
  };
});

vi.mock("@backend/backend/AdminAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AdminAPI")>();
  return {
    ...actual,
    GetCompanyUnits: (...args: unknown[]) => mocks.getCompanyUnits(...args),
    GetCurrencyCatalogue: (...args: unknown[]) => mocks.getCurrencyCatalogue(...args),
    GetCompanyProfile: (...args: unknown[]) => mocks.getCompanyProfile(...args),
    GetCompanyLogo: (...args: unknown[]) => mocks.getCompanyLogo(...args),
    GetCompanyLabels: (...args: unknown[]) => mocks.getCompanyLabels(...args),
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
    ListDrafts: (...args: unknown[]) => mocks.listDrafts(...args),
    ListCustomers: (...args: unknown[]) => mocks.listCustomers(...args),
    GetDraft: (...args: unknown[]) => mocks.getDraft(...args),
    ClearDraftContext: vi.fn(async () => {}),
    ClearRevisionContext: vi.fn(async () => {}),
    SetDraftContext: vi.fn(async () => {}),
    SetRevisionContext: vi.fn(async () => {}),
    ListPOSourceQuotations: (...args: unknown[]) => mocks.listPOSourceQuotations(...args),
    ListPOSourceRevisions: (...args: unknown[]) => mocks.listPOSourceRevisions(...args),
    ValidatePOSource: (...args: unknown[]) => mocks.validatePOSource(...args),
    CreatePurchaseOrderDraft: (...args: unknown[]) =>
      mocks.createPurchaseOrderDraft(...args),
    GetPurchaseOrderExtraction: (...args: unknown[]) =>
      mocks.getPurchaseOrderExtraction(...args),
    GetPurchaseOrderDocumentState: (...args: unknown[]) =>
      mocks.getPurchaseOrderDocumentState(...args),
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

function purchaseOrder(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    PODate: null,
    RequestedDeliveryDate: null,
    UUID: "po-uuid-1",
    State: "draft",
    QuoteNumber: "1502260012",
    CustomerName: "PT. Infineon Technologies Batam",
    BillingAddress: "Batamindo Industrial Park",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "",
    ReceivedDate: "",
    Incoterms: "",
    Currency: "IDR",
    TaxZone: "FTZ",
    TaxCategory: "SupplyOnly",
    AdjustmentReason: "",
    CreatedAt: "2026-10-22T03:00:00Z",
    UpdatedAt: "2026-10-22T03:00:00Z",
    Items: [],
    ID: 21,
    QuotationID: 12,
    SourceRevision: 0,
    CustomerID: 2,
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
    Version: 1,
    ...overrides,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockClear();
  mocks.restoreSession.mockResolvedValue(USER);
  mocks.currentUser.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.getCompanyProfile.mockResolvedValue(PROFILE);
  mocks.listPOSourceQuotations.mockResolvedValue({
    Quotations: [
      {
        ID: 12,
        QuoteNumber: "1502260012",
        Subject: "Supply and installation",
        CustomerName: "PT. Infineon Technologies Batam",
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
  mocks.listPOSourceRevisions.mockResolvedValue({
    Revisions: [{ Revision: 0, IssuedAt: "2026-10-20T10:00:00+07:00" }],
  });
  mocks.createPurchaseOrderDraft.mockResolvedValue(purchaseOrder());
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  window.localStorage.clear();
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
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

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function buttonWithText(label: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (node) => node.textContent?.trim() === label
  );
  if (found === undefined) throw new Error(`no button reading ${label}`);
  return found;
}

function menuItem(label: string): HTMLButtonElement {  const found = Array.from(
    document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
  ).find((node) => node.textContent?.trim() === label);
  if (found === undefined) throw new Error(`no menu item ${label}`);
  return found;
}

/** A workspace tab in the primary nav, tolerating a drafts badge. */
function navTab(label: string): HTMLButtonElement {
  const nav = document.querySelector<HTMLElement>('nav[aria-label="Primary"]');
  const found = Array.from(nav?.querySelectorAll<HTMLButtonElement>("button") ?? []).find(
    (node) => node.textContent?.trim().startsWith(label)
  );
  if (found === undefined) throw new Error(`no tab labelled ${label}`);
  return found;
}

async function openMenuItem(label: string): Promise<void> {
  await act(async () => {
    button("Account menu for Sam Staff").click();
  });
  await act(async () => {
    menuItem(label).click();
  });
  await flush();
}

function input(ariaLabel: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    `input[aria-label="${ariaLabel}"]`
  );
  if (found === null) throw new Error(`no input labelled ${ariaLabel}`);
  return found;
}

function textarea(ariaLabel: string): HTMLTextAreaElement {
  const found = document.querySelector<HTMLTextAreaElement>(
    `textarea[aria-label="${ariaLabel}"]`
  );
  if (found === null) throw new Error(`no textarea labelled ${ariaLabel}`);
  return found;
}

function typeInto(
  element: HTMLInputElement | HTMLTextAreaElement,
  value: string
): void {
  act(() => {
    const prototype =
      element instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(prototype, "value")?.set;
    setter?.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

/** The input inside the editor's DetailsPanel field with this label. */
function detailsInput(label: string): HTMLInputElement {
  const span = Array.from(document.querySelectorAll("span")).find(
    (node) => node.textContent?.trim() === label
  );
  const found = span?.parentElement?.parentElement?.querySelector("input");
  if (found === null || found === undefined) {
    throw new Error(`no details field labelled ${label}`);
  }
  return found;
}

describe("Quotation settings page (issue #492)", () => {
  it("saves as the user edits and reloads from this device", async () => {
    await renderApp();
    await openMenuItem("Quotation settings");

    // Its own surface: the shell names it, and it has no workspace tabs.
    expect(document.body.textContent).toContain("Quotation settings");
    expect(document.querySelector('nav[aria-label="Primary"]')).toBeNull();
    expect(button("Back to Home")).not.toBeNull();

    typeInto(input("Default validity days"), "45");
    typeInto(input("Default subject"), "Turbine overhaul");

    expect(window.localStorage.getItem(quotationSettingsKey(USER.ID))).toContain(
      '"validityDays":45'
    );

    // A reload is a fresh App reading the same device store.
    act(() => root.unmount());
    container.remove();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    await renderApp();
    await openMenuItem("Quotation settings");
    expect(input("Default validity days").value).toBe("45");
    expect(input("Default subject").value).toBe("Turbine overhaul");
  });

  it("clears the overlay back to the effective defaults", async () => {
    writeQuotationSettings(window.localStorage, USER.ID, {
      ...DEFAULT_QUOTATION_SETTINGS,
      validityDays: 45,
    });
    await renderApp();
    await openMenuItem("Quotation settings");
    expect(input("Default validity days").value).toBe("45");

    await act(async () => {
      button("Clear quotation settings").click();
    });

    expect(input("Default validity days").value).toBe("30");
    expect(input("Default payment term days").value).toBe("90");
    expect(window.localStorage.getItem(quotationSettingsKey(USER.ID))).toBeNull();
  });

  it("clamps numbers to the bounds the editors accept", async () => {
    await renderApp();
    await openMenuItem("Quotation settings");

    typeInto(input("Default validity days"), "0");
    expect(input("Default validity days").value).toBe("1");
    typeInto(input("Default validity days"), "999");
    expect(input("Default validity days").value).toBe("255");
    typeInto(input("Default payment term days"), "0");
    expect(input("Default payment term days").value).toBe("1");
  });

  it("asks before opening settings from a dirty quotation", async () => {
    await renderApp();
    await act(async () => {
      button("Open Quotation").click();
    });
    const subject = document.querySelector<HTMLInputElement>(
      'input[placeholder="Supply and installation of electrical and mechanical systems"]'
    );
    if (subject === null) throw new Error("subject field not found");
    typeInto(subject, "Unsaved quotation work");

    await openMenuItem("Quotation settings");

    expect(
      document.querySelector('[role="alertdialog"][aria-label="Discard unsaved changes?"]')
    ).not.toBeNull();
    expect(document.getElementById("quotation-settings-heading")).toBeNull();
    expect(document.body.textContent).toContain(
      "The quotation in the editor has unsaved changes."
    );
    await act(async () => {
      button("Keep editing").click();
    });
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder="Supply and installation of electrical and mechanical systems"]'
      )?.value
    ).toBe("Unsaved quotation work");
  });
});

describe("Purchase Order settings page (issue #492)", () => {
  it("saves, reloads and clears the PO defaults", async () => {
    await renderApp();
    await openMenuItem("Purchase Order settings");

    expect(document.querySelector('nav[aria-label="Primary"]')).toBeNull();
    typeInto(textarea("Default payment terms"), "50% advance, balance 30 days");
    typeInto(input("Default received date"), "2026-10-01");
    typeInto(input("Default PO list page size"), "25");

    expect(window.localStorage.getItem(purchaseOrderSettingsKey(USER.ID))).toContain(
      '"listPageSize":25'
    );

    act(() => root.unmount());
    container.remove();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    await renderApp();
    await openMenuItem("Purchase Order settings");
    expect(textarea("Default payment terms").value).toBe(
      "50% advance, balance 30 days"
    );
    expect(input("Default received date").value).toBe("2026-10-01");
    expect(input("Default PO list page size").value).toBe("25");

    await act(async () => {
      button("Clear purchase order settings").click();
    });
    expect(textarea("Default payment terms").value).toBe("");
    expect(input("Default received date").value).toBe("");
    expect(input("Default PO list page size").value).toBe("15");
    // A blank received date means "today"; the page names the effective day.
    expect(document.body.textContent).toContain("Today in WIB is");
    expect(window.localStorage.getItem(purchaseOrderSettingsKey(USER.ID))).toBeNull();
  });

  it("clamps the page size to its bounds", async () => {
    await renderApp();
    await openMenuItem("Purchase Order settings");
    typeInto(input("Default PO list page size"), "0");
    expect(input("Default PO list page size").value).toBe("1");
    typeInto(input("Default PO list page size"), "500");
    expect(input("Default PO list page size").value).toBe("100");
  });
});

describe("new drafts take the defaults (issue #492)", () => {
  it("the first blank editor of the session shows the saved defaults", async () => {
    writeQuotationSettings(window.localStorage, USER.ID, {
      ...DEFAULT_QUOTATION_SETTINGS,
      validityDays: 45,
      subject: "Turbine overhaul",
    });
    await renderApp();
    await act(async () => {
      button("Open Quotation").click();
    });

    // Opening the editor is starting a new draft, before any reset click.
    expect(detailsInput("Validity").value).toBe("45");
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder="Supply and installation of electrical and mechanical systems"]'
      )?.value
    ).toBe("Turbine overhaul");
  });

  it("a new quotation starts from the saved defaults", async () => {
    writeQuotationSettings(window.localStorage, USER.ID, {
      ...DEFAULT_QUOTATION_SETTINGS,
      validityDays: 45,
      paymentTermDays: 30,
      taxZone: "NonFTZ",
      subject: "Turbine overhaul",
    });
    await renderApp();
    await act(async () => {
      button("Open Quotation").click();
    });
    await act(async () => {
      buttonWithText("New").click();
    });

    expect(detailsInput("Validity").value).toBe("45");
    expect(detailsInput("Payment").value).toBe("30");
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder="Supply and installation of electrical and mechanical systems"]'
      )?.value
    ).toBe("Turbine overhaul");
    const checkedZone = Array.from(
      document.querySelectorAll<HTMLElement>('[role="radio"][aria-checked="true"]')
    ).map((node) => node.textContent?.trim());
    expect(checkedZone).toContain("Non-FTZ");
  });

  it("an existing draft keeps the values it was saved with", async () => {
    // No default subject here: a non-empty subject would make the freshly
    // opened editor dirty and the Drafts tab would correctly ask first.
    writeQuotationSettings(window.localStorage, USER.ID, {
      ...DEFAULT_QUOTATION_SETTINGS,
      validityDays: 45,
    });
    mocks.listDrafts.mockResolvedValue([
      {
        ID: 3,
        DisplayName: "Saved work",
        Label: "Saved work",
        Subject: "Saved work",
        CustomerName: "PT. Infineon Technologies Batam",
        ItemCount: 0,
        UpdatedAt: "2026-10-20T10:00:00Z",
      },
    ]);
    mocks.getDraft.mockResolvedValue({
      ID: 3,
      Label: "Saved work",
      Subject: "Saved work",
      CustomerName: "PT. Infineon Technologies Batam",
      Currency: "IDR",
      CustomerID: 2,
      UpdatedAt: "2026-10-20T10:00:00Z",
      Payload: JSON.stringify({
        items: [],
        customerId: 2,
        details: {
          subject: "Saved subject",
          recipientName: "",
          validity: 60,
          paymentTerm: 20,
          leadTimeYears: 0,
          leadTimeMonths: 0,
          leadTimeWeeks: 0,
          leadTimeDays: 0,
          discountPercent: 0,
          incoterm: "EXW",
          taxZone: "FTZ",
          taxType: "Goods",
          serviceCategory: "",
        },
      }),
    });

    await renderApp();
    await act(async () => {
      button("Open Quotation").click();
    });
    await act(async () => {
      navTab("Drafts").click();
    });
    await flush();
    const row = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
      (node) => node.textContent?.trim() === "Saved work"
    );
    if (row === undefined) throw new Error("draft row not found");
    await act(async () => {
      row.click();
    });
    await flush();

    expect(detailsInput("Validity").value).toBe("60");
    expect(detailsInput("Payment").value).toBe("20");
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder="Supply and installation of electrical and mechanical systems"]'
      )?.value
    ).toBe("Saved subject");
  });

  it("a new PO uses the PO defaults where the quotation supplies nothing", async () => {
    writePurchaseOrderSettings(window.localStorage, USER.ID, {
      ...DEFAULT_PURCHASE_ORDER_SETTINGS,
      paymentTerms: "50% advance",
      incoterms: "FOB",
      receivedDate: "2026-10-01",
    });
    await renderApp();
    await createPurchaseOrder();

    expect(mocks.createPurchaseOrderDraft).toHaveBeenCalledWith(
      expect.objectContaining({ ReceivedDate: "2026-10-01" })
    );
    expect(input("Payment terms").value).toBe("50% advance");
    expect(input("Received date").value).toBe("2026-10-01");
    expect(
      document.querySelector<HTMLButtonElement>('button[aria-label="Incoterms"]')
        ?.textContent
    ).toContain("FOB");
  });

  it("a source-supplied PO keeps its own values", async () => {
    mocks.createPurchaseOrderDraft.mockResolvedValue(
      purchaseOrder({
        PaymentTerms: "45 days",
        Incoterms: "CIF",
        ReceivedDate: "2026-09-30",
      })
    );
    writePurchaseOrderSettings(window.localStorage, USER.ID, {
      ...DEFAULT_PURCHASE_ORDER_SETTINGS,
      paymentTerms: "50% advance",
      incoterms: "FOB",
      receivedDate: "2026-10-01",
    });
    await renderApp();
    await createPurchaseOrder();

    expect(input("Payment terms").value).toBe("45 days");
    expect(input("Received date").value).toBe("2026-09-30");
    expect(
      document.querySelector<HTMLButtonElement>('button[aria-label="Incoterms"]')
        ?.textContent
    ).toContain("CIF");
  });
});

/** Opens the PO workspace and creates a draft through the real selector. */
async function createPurchaseOrder(): Promise<void> {
  await act(async () => {
    button("Open Purchase Orders").click();
  });
  await flush();
  await act(async () => {
    button("Add Purchase Order from a Quotation").click();
  });
  await flush();
  const row = document.querySelector<HTMLButtonElement>(
    'button[aria-label="Quotation 1502260012, PT. Infineon Technologies Batam"]'
  );
  if (row === null) throw new Error("source quotation row not found");
  await act(async () => {
    row.click();
  });
  await flush();
  await act(async () => {
    button("Continue to confirmation").click();
  });
  await flush();
  await act(async () => {
    button("Confirm source").click();
  });
  await flush();
}
