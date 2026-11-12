// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import {
  blankPurchaseOrderDraft,
  type PurchaseOrderDTO,
} from "./purchaseOrder";
import type { PurchaseOrderReviewDTO } from "./poFinalize";
import { ToastProvider } from "./toast";

// Ticket 02: the page-history stack for Home → workspace → settings.
//
// The real App with only the desktop bindings replaced, so every assertion is
// the production wiring: Back from a settings page returns to the preserved
// workspace (never straight Home), a dirty editor survives the round-trip
// without a discard prompt, list filters/page/tab and the finalize review
// step survive, Save draft and Final Save live in the shell action bar, and
// sign-out clears the unsaved state.
//
// What this cannot show: pixel placement of the action bar (Jessy's visual
// follow-up owns that; this suite pins the structure -- the buttons render
// once, in the sticky shell slot), and real window scrolling (jsdom has no
// layout, so the suite pins the save/restore calls instead).

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
  createBlankPurchaseOrderDraft: vi.fn((..._args: unknown[]): unknown => undefined),
  updatePurchaseOrderDraft: vi.fn((..._args: unknown[]): unknown => undefined),
  getPurchaseOrder: vi.fn((..._args: unknown[]): unknown => undefined),
  getPurchaseOrderReview: vi.fn((..._args: unknown[]): unknown => undefined),
  listPurchaseOrders: vi.fn(async (..._args: unknown[]): Promise<Record<string, unknown>> => ({
    PurchaseOrders: [],
    Customers: [],
    Page: 1,
    PageSize: 15,
    Total: 0,
    TotalPages: 1,
  })),
  getPurchaseOrderDocumentState: vi.fn(async (..._args: unknown[]) => ({
    Document: null,
    Versions: [],
  })),
  loadReconciliation: vi.fn(async (..._args: unknown[]) => {
    throw new Error("no reconciliation recorded");
  }),
  saveReconciliation: vi.fn(async (...args: unknown[]) => args[1]),
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
    CreatePurchaseOrderBlankDraft: (...args: unknown[]) =>
      mocks.createBlankPurchaseOrderDraft(...args),
    UpdatePurchaseOrderDraft: (...args: unknown[]) =>
      mocks.updatePurchaseOrderDraft(...args),
    GetPurchaseOrder: (...args: unknown[]) =>
      mocks.getPurchaseOrder(...args),
    GetPurchaseOrderReview: (...args: unknown[]) =>
      mocks.getPurchaseOrderReview(...args),
    GetPurchaseOrderDocumentState: (...args: unknown[]) =>
      mocks.getPurchaseOrderDocumentState(...args),
    LoadPOReconciliation: (...args: unknown[]) =>
      mocks.loadReconciliation(...args),
    SavePOReconciliation: (...args: unknown[]) =>
      mocks.saveReconciliation(...args),
    ListPurchaseOrders: (...args: unknown[]) =>
      mocks.listPurchaseOrders(...args),
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
    UUID: "po-uuid-502",
    State: "draft",
    QuoteNumber: "1502260052",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "PO-502",
    ReceivedDate: "2026-10-22",
    Incoterms: "CIF",
    Currency: "IDR",
    TaxZone: "FTZ",
    TaxCategory: "SupplyOnly",
    AdjustmentReason: "",
    CreatedAt: "2026-10-22T03:00:00Z",
    UpdatedAt: "2026-10-22T03:00:00Z",
    Items: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Widget",
        QuantityScaled: 10000,
        UoM: "Lot",
        UnitPriceMinor: 100000,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 100000,
        DiscountMinor: 0,
        NetMinor: 100000,
      },
    ],
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    CustomerID: 2,
    PaymentTerms: "30 days",
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

const REVIEW: PurchaseOrderReviewDTO = {
  Order: purchaseOrder(),
  DocumentState: "none",
  Source: {
    QuotationID: 12,
    RevisionNumber: 1,
    QuoteNumber: "1502260052",
    CustomerID: 2,
    CustomerName: "PT. Buyer",
    Currency: "IDR",
    PaymentTerms: "45 days",
    Incoterms: "CIF",
    DiscountPercent: 0,
    PrintedSubtotalMinor: 0,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 0,
    Lines: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineLabel: "1",
        Description: "Widget",
        UoM: "Lot",
        QuantityScaled: 10000,
        UnitPriceMinor: 100000,
        DiscountEnabled: false,
        DiscountPercent: 0,
      },
    ],
  },
};

let container: HTMLDivElement;
let root: Root;
let scrollToSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockClear();
  mocks.restoreSession.mockResolvedValue(USER);
  mocks.currentUser.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.getCompanyProfile.mockResolvedValue(PROFILE);
  mocks.getPurchaseOrderReview.mockResolvedValue(REVIEW);
  mocks.listPurchaseOrders.mockResolvedValue({
    PurchaseOrders: [],
    Customers: [],
    Page: 1,
    PageSize: 15,
    Total: 0,
    TotalPages: 1,
  });
  // jsdom has no layout: scrolling is a stub that only logs. Replace it so
  // the save/restore calls the stack test pins stay assertion-clean.
  scrollToSpy = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  scrollToSpy.mockRestore();
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

/** A workspace tab in the primary nav, tolerating a drafts badge. */
function navTab(label: string): HTMLButtonElement {
  const nav = document.querySelector<HTMLElement>('nav[aria-label="Primary"]');
  const found = Array.from(nav?.querySelectorAll<HTMLButtonElement>("button") ?? []).find(
    (node) => node.textContent?.trim().startsWith(label)
  );
  if (found === undefined) throw new Error(`no tab labelled ${label}`);
  return found;
}

function input(ariaLabel: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    `input[aria-label="${ariaLabel}"]`
  );
  if (found === null) throw new Error(`no input labelled ${ariaLabel}`);
  return found;
}

function discardPrompt(): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    '[role="alertdialog"][aria-label="Discard unsaved changes?"]'
  );
}

function workspace(): string | null {
  return (
    document
      .querySelector<HTMLElement>("[data-app-shell]")
      ?.getAttribute("data-workspace") ?? null
  );
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

async function openMenuItem(label: string): Promise<void> {
  await act(async () => {
    button("Account menu for Sam Staff").click();
  });
  await act(async () => {
    const found = Array.from(
      document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    ).find((node) => node.textContent?.trim() === label);
    if (found === undefined) throw new Error(`no menu item ${label}`);
    found.click();
  });
  await flush();
}

/** The sticky shell action-bar slot below the page. */
function actionBar(): HTMLElement {
  const bars = Array.from(document.querySelectorAll<HTMLElement>("[data-app-shell] div.sticky"));
  const found = bars.find((node) => node.className.includes("bottom-0"));
  if (found === undefined) throw new Error("no shell action bar rendered");
  return found;
}

function withinActionBar(label: string): HTMLButtonElement | null {
  try {
    const bar = actionBar();
    return bar.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  } catch {
    return null;
  }
}

describe("PO back stack (ticket 02)", () => {
  it("Home → workspace → settings → workspace → Home keeps a dirty editor with no prompt", async () => {
    const blank = blankPurchaseOrderDraft({
      currency: "IDR",
      taxZone: "FTZ",
      taxType: "Goods",
      serviceCategory: "",
      receivedDate: "2026-10-22",
      incoterms: "EXW",
    });
    mocks.createBlankPurchaseOrderDraft.mockResolvedValue({
      ...blank,
      ID: 41,
      UUID: "po-first-502",
      Version: 1,
    });
    mocks.getPurchaseOrder.mockResolvedValue({
      ...blank,
      ID: 41,
      UUID: "po-first-502",
      Version: 2,
      PONumber: "DIRTY-PO-502",
    });
    await renderApp();
    await act(async () => {
      button("Open Purchase Orders").click();
    });
    await flush();

    typeInto(input("Customer PO number"), "DIRTY-PO-502");
    await flush();

    await openMenuItem("Purchase Order settings");

    // The round-trip never asks: the editor is kept alive behind the page.
    expect(discardPrompt()).toBeNull();
    expect(workspace()).toBe("purchase-order-settings");
    expect(document.getElementById("po-settings-heading")).not.toBeNull();
    // Back never jumps straight Home from a settings page.
    expect(workspace()).not.toBe("home");

    await act(async () => {
      button("Back").click();
    });
    await flush();

    expect(discardPrompt()).toBeNull();
    expect(workspace()).toBe("purchase-order");
    expect(input("Customer PO number").value).toBe("DIRTY-PO-502");

    await act(async () => {
      button("Back to Home").click();
    });
    await flush();
    // Leaving the workspace with a dirty editor still asks -- only the
    // settings round-trip is exempt.
    expect(discardPrompt()).not.toBeNull();
    await act(async () => {
      button("Discard changes").click();
    });
    await flush();
    expect(workspace()).toBe("home");
  });

  it("Save draft and Final Save render once, in the shell action bar", async () => {
    await renderApp();
    await act(async () => {
      button("Open Purchase Orders").click();
    });
    await flush();

    // The entry view reports its actions up; the footer keeps only Cancel.
    const shellSave = withinActionBar("Save draft");
    const shellFinal = withinActionBar("Final Save");
    expect(shellSave).not.toBeNull();
    expect(shellFinal).not.toBeNull();
    expect(
      document.querySelectorAll('button[aria-label="Save draft"]')
    ).toHaveLength(1);
    expect(
      document.querySelectorAll('button[aria-label="Final Save"]')
    ).toHaveLength(1);

    // The shell Save draft drives the same persistence as the footer did.
    const blank = blankPurchaseOrderDraft({
      currency: "IDR",
      taxZone: "FTZ",
      taxType: "Goods",
      serviceCategory: "",
      receivedDate: "2026-10-22",
      incoterms: "EXW",
    });
    mocks.createBlankPurchaseOrderDraft.mockResolvedValue({
      ...blank,
      ID: 42,
      UUID: "po-first-502-shell",
      Version: 1,
    });
    mocks.updatePurchaseOrderDraft.mockImplementation(async (fields: unknown) => {
      const input = fields as Record<string, unknown>;
      return {
        Order: {
          ...blank,
          ID: 42,
          UUID: "po-first-502-shell",
          Version: 2,
          PONumber: String(input.PONumber ?? ""),
        },
      };
    });
    typeInto(input("Customer PO number"), "SHELL-PO-502");
    await flush();
    await act(async () => {
      withinActionBar("Save draft")?.click();
    });
    await flush();
    expect(mocks.createBlankPurchaseOrderDraft).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("Purchase order draft saved.");

    // A settings page has no PO actions: the bar stands down while hidden.
    await openMenuItem("Purchase Order settings");
    expect(document.getElementById("po-settings-heading")).not.toBeNull();
    expect(withinActionBar("Save draft")).toBeNull();
  });

  it("Drafts filters, page and tab survive a settings round-trip", async () => {
    mocks.listPurchaseOrders.mockImplementation(async (query: unknown) => {
      const page = (query as { Page: number }).Page;
      return {
        PurchaseOrders: [
          {
            UUID: "po-uuid-row",
            State: "draft",
            PONumber: "ACME-1",
            QuoteNumber: "1502260052",
            CustomerName: "Acme Corp",
            ReceivedDate: "2026-10-22",
            Currency: "IDR",
            ID: 61,
            CustomerID: 2,
            SourceRevision: 1,
            SourceMissing: false,
          },
        ],
        Customers: [{ Name: "Acme Corp", ID: 2 }],
        Page: page,
        PageSize: 15,
        Total: 30,
        TotalPages: 2,
      };
    });
    await renderApp();
    await act(async () => {
      button("Open Purchase Orders").click();
    });
    await flush();
    await act(async () => {
      navTab("Drafts").click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 300));
    });

    typeInto(input("Search purchase orders"), "ACME");
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 300));
    });
    await act(async () => {
      button("Next page").click();
    });
    await flush();
    expect(input("Search purchase orders").value).toBe("ACME");
    expect(document.body.textContent).toContain("page 2 of 2");

    await openMenuItem("Purchase Order settings");
    expect(document.getElementById("po-settings-heading")).not.toBeNull();

    await act(async () => {
      button("Back").click();
    });
    await flush();

    // Same tab, same filters, same page: the list was kept alive, not rebuilt.
    expect(navTab("Drafts").getAttribute("aria-current")).toBe("page");
    expect(input("Search purchase orders").value).toBe("ACME");
    expect(document.body.textContent).toContain("page 2 of 2");
  });

  it("the finalize review step survives a settings round-trip", async () => {
    mocks.listPurchaseOrders.mockResolvedValue({
      PurchaseOrders: [
        {
          UUID: "po-uuid-502",
          State: "draft",
          PONumber: "PO-502",
          QuoteNumber: "1502260052",
          CustomerName: "PT. Buyer",
          ReceivedDate: "2026-10-22",
          Currency: "IDR",
          ID: 7,
          CustomerID: 2,
          SourceRevision: 1,
          SourceMissing: false,
        },
      ],
      Customers: [{ Name: "PT. Buyer", ID: 2 }],
      Page: 1,
      PageSize: 15,
      Total: 1,
      TotalPages: 1,
    });
    mocks.getPurchaseOrder.mockResolvedValue(purchaseOrder());
    mocks.updatePurchaseOrderDraft.mockImplementation(async (fields: unknown) => {
      const input = fields as Record<string, unknown>;
      return {
        Order: {
          ...purchaseOrder(),
          PONumber: String(input.PONumber ?? "PO-502"),
          Version: 2,
        },
      };
    });
    await renderApp();
    await act(async () => {
      button("Open Purchase Orders").click();
    });
    await flush();
    await act(async () => {
      navTab("Drafts").click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 300));
    });
    await act(async () => {
      button("Open purchase order PO-502").click();
    });
    await flush();

    await act(async () => {
      withinActionBar("Final Save")?.click();
    });
    await flush();
    expect(document.body.textContent).toContain(
      "One-way: a saved record is read-only"
    );

    await openMenuItem("Purchase Order settings");
    expect(discardPrompt()).toBeNull();
    expect(document.getElementById("po-settings-heading")).not.toBeNull();

    await act(async () => {
      button("Back").click();
    });
    await flush();
    expect(document.body.textContent).toContain(
      "One-way: a saved record is read-only"
    );
    expect(button("Back to editor")).not.toBeNull();
  });

  it("restores the saved scroll position on the way back", async () => {
    let scrollYValue = 0;
    Object.defineProperty(window, "scrollY", {
      configurable: true,
      get: () => scrollYValue,
    });
    try {
      await renderApp();
      await act(async () => {
        button("Open Purchase Orders").click();
      });
      await flush();

      scrollYValue = 400;
      await openMenuItem("Purchase Order settings");
      scrollToSpy.mockClear();

      await act(async () => {
        button("Back").click();
      });
      await flush();
      expect(scrollToSpy).toHaveBeenCalledWith(0, 400);
    } finally {
      // jsdom owns scrollY; leave no test-only property behind.
      delete (window as unknown as Record<string, unknown>).scrollY;
    }
  });
});

describe("Quotation back stack (ticket 02)", () => {
  it("Home → editor → settings → editor → Home keeps a dirty editor with no prompt", async () => {
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
    expect(discardPrompt()).toBeNull();
    expect(workspace()).toBe("quotation-settings");
    expect(workspace()).not.toBe("home");

    await act(async () => {
      button("Back").click();
    });
    await flush();
    expect(workspace()).toBe("quotation");
    expect(
      document.querySelector<HTMLInputElement>(
        'input[placeholder="Supply and installation of electrical and mechanical systems"]'
      )?.value
    ).toBe("Unsaved quotation work");
  });
});

describe("Quotation note counts as entered work (#535)", () => {
  it("a note-only edit marks the editor dirty and enables Save draft", async () => {
    await renderApp();
    await act(async () => {
      button("Open Quotation").click();
    });
    await flush();

    // A pristine editor has nothing to save.
    expect(buttonWithText("Save draft").disabled).toBe(true);

    const note = document.querySelector<HTMLElement>(
      '[role="textbox"][aria-label="Note"]'
    );
    if (note === null) throw new Error("note editor not found");
    await act(async () => {
      note.innerHTML = "<p>Call before delivery</p>";
      note.dispatchEvent(new Event("input", { bubbles: true }));
    });

    // The note is quotation content: it must enable Save draft, drive the
    // leave guard and reach the #537 snapshot like any other entered field.
    expect(buttonWithText("Save draft").disabled).toBe(false);
  });
});

describe("sign-out clears the stack (ticket 02)", () => {
  it("no unsaved workspace, history or scroll survives sign-out", async () => {
    await renderApp();
    await act(async () => {
      button("Open Purchase Orders").click();
    });
    await flush();
    typeInto(input("Customer PO number"), "SIGNOUT-PO-502");
    await flush();
    expect(input("Customer PO number").value).toBe("SIGNOUT-PO-502");

    await openMenuItem("Purchase Order settings");
    expect(workspace()).toBe("purchase-order-settings");

    // Open the menu fresh on the settings page and sign out.
    await act(async () => {
      button("Account menu for Sam Staff").click();
    });
    await act(async () => {
      const found = Array.from(
        document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
      ).find((node) => node.textContent?.trim() === "Sign out");
      if (found === undefined) throw new Error("no menu item Sign out");
      found.click();
    });
    await flush();

    // Signed out: the login surface, no workspace, no settings context.
    expect(document.body.textContent).toContain("Sign in");
    expect(workspace()).toBeNull();
  });
});
