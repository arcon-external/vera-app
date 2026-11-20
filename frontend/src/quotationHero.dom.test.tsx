// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { ToastProvider } from "./toast";

// Issue #623: the Quotation editor opens with a dark, read-only hero that
// mirrors the editor. These cases drive the real top-level App with only the
// desktop bindings replaced, so what is asserted is the production wiring:
// customer/terms/tax selections reach the hero, a line prices through the same
// computePricing call the Summary rail uses, Save draft changes the status,
// and opening an issued revision shows the current/next revision pair.
//
// What this cannot show: pixel placement (jsdom has no layout) and the browser
// rendering at 360/1280, which the UI snapshot harness covers.

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
  listQuotations: vi.fn(async (..._args: unknown[]) => [] as unknown[]),
  getRevision: vi.fn((..._args: unknown[]): unknown => undefined),
  getOccupancy: vi.fn(async (..._args: unknown[]) => ({
    QuotationID: 12,
    ActivePO: null,
  })),
  getDraft: vi.fn((..._args: unknown[]): unknown => undefined),
  saveDraft: vi.fn(async (..._args: unknown[]) => 1),
  prepare: vi.fn(async (..._args: unknown[]) => {}),
  exportPdf: vi.fn(async (..._args: unknown[]) => {}),
  getCustomerPreview: vi.fn(async (..._args: unknown[]) => ({})),
  getInternalBreakdown: vi.fn(async (..._args: unknown[]) => ({})),
  getQuotationPreviewHTML: vi.fn(async (..._args: unknown[]) => ""),
  setDraftContext: vi.fn(async (..._args: unknown[]) => {}),
  setRevisionContext: vi.fn(async (..._args: unknown[]) => {}),
  getCompanyProfile: vi.fn(async (..._args: unknown[]) => ({})),
  getCompanyLogo: vi.fn(async (..._args: unknown[]) => ({
    LogoBase64: "",
    LogoMime: "",
    BuiltIn: true,
  })),
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
    ListQuotations: (...args: unknown[]) => mocks.listQuotations(...args),
    GetQuotationRevision: (...args: unknown[]) => mocks.getRevision(...args),
    GetPOSourceOccupancy: (...args: unknown[]) => mocks.getOccupancy(...args),
    GetDraft: (...args: unknown[]) => mocks.getDraft(...args),
    SaveDraft: (...args: unknown[]) => mocks.saveDraft(...args),
    Prepare: (...args: unknown[]) => mocks.prepare(...args),
    Export: (...args: unknown[]) => mocks.exportPdf(...args),
    GetCustomerPreview: (...args: unknown[]) => mocks.getCustomerPreview(...args),
    GetInternalBreakdown: (...args: unknown[]) => mocks.getInternalBreakdown(...args),
    GetQuotationPreviewHTML: (...args: unknown[]) => mocks.getQuotationPreviewHTML(...args),
    SetDraftContext: (...args: unknown[]) => mocks.setDraftContext(...args),
    ClearDraftContext: vi.fn(async () => {}),
    SetRevisionContext: (...args: unknown[]) => mocks.setRevisionContext(...args),
    ClearRevisionContext: vi.fn(async () => {}),
    CreatePurchaseOrderDraft: vi.fn(),
    GetPurchaseOrder: vi.fn(),
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

const FIRST_CUSTOMER = "PT. Real Customer";
const SECOND_CUSTOMER = "PT. Second Customer";
const QUOTE_NUMBER = "1502260012";

function archivedQuotation(revision: number) {
  return {
    ID: 12,
    QuoteNumber: QUOTE_NUMBER,
    RevisionNumber: revision,
    CustomerName: FIRST_CUSTOMER,
    CustomerCode: 2,
    CustomerAddress: "Jl. Klien 12",
    Subject: "Electrical installation",
    Currency: "IDR",
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

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  window.localStorage.clear();
  for (const mock of Object.values(mocks)) mock.mockClear();
  mocks.restoreSession.mockResolvedValue(USER);
  mocks.currentUser.mockResolvedValue(USER);
  mocks.getCompany.mockResolvedValue(PROFILE);
  mocks.getCompanyProfile.mockResolvedValue(PROFILE);
  mocks.listCustomers.mockResolvedValue([
    { Code: 2, Name: FIRST_CUSTOMER, PrimaryAddress: "", ID: 2, QuotationCount: 0 },
    { Code: 3, Name: SECOND_CUSTOMER, PrimaryAddress: "", ID: 3, QuotationCount: 0 },
  ]);
  mocks.listQuotations.mockResolvedValue([
    {
      ID: 12,
      QuoteNumber: QUOTE_NUMBER,
      RevisionNumber: 2,
      CustomerName: FIRST_CUSTOMER,
      Subject: "Electrical installation",
      Currency: "IDR",
      CreatedAt: "2026-10-20T10:00:00Z",
      GrandTotal: 100,
      DisplayGrand: "Rp100",
    },
  ]);
  mocks.getRevision.mockImplementation(async (...args: unknown[]) =>
    archivedQuotation(Number(args[1]))
  );
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  Element.prototype.scrollIntoView = vi.fn();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  window.localStorage.clear();
  vi.restoreAllMocks();
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function waitFor(check: () => boolean, label: string): Promise<void> {
  const started = Date.now();
  while (Date.now() - started < 8000) {
    if (check()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 25));
    });
  }
  throw new Error(`timed out waiting for ${label}`);
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
    throw new Error(`no button reading ${label}`);
  }
  return found;
}

function input(ariaLabel: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    `input[aria-label="${ariaLabel}"]`
  );
  if (found === null) throw new Error(`no input labelled ${ariaLabel}`);
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

function pickOption(triggerLabel: string, optionLabel: string): void {
  act(() => button(triggerLabel).click());
  const option = Array.from(
    document.querySelectorAll<HTMLElement>('[role="option"]')
  ).find((candidate) => candidate.textContent?.trim() === optionLabel);
  if (option === undefined) throw new Error(`no option reading ${optionLabel}`);
  act(() => option.click());
}

function pickRadio(groupLabel: string, optionLabel: string): void {
  const group = document.querySelector<HTMLElement>(
    `[role="radiogroup"][aria-label="${groupLabel}"]`
  );
  if (group === null) throw new Error(`no radiogroup labelled ${groupLabel}`);
  const radio = Array.from(
    group.querySelectorAll<HTMLButtonElement>('[role="radio"]')
  ).find((candidate) => candidate.textContent?.trim() === optionLabel);
  if (radio === undefined) throw new Error(`no radio reading ${optionLabel}`);
  act(() => radio.click());
}

function heroText(): string {
  return (
    document.querySelector<HTMLElement>('[data-testid="quotation-hero"]')
      ?.textContent ?? ""
  );
}

function hero(): HTMLElement {
  const found = document.querySelector<HTMLElement>(
    '[data-testid="quotation-hero"]'
  );
  if (found === null) throw new Error("no quotation hero");
  return found;
}

function heroTotal(): string {
  return (
    document.querySelector<HTMLElement>('[data-testid="quotation-hero-total"]')
      ?.textContent ?? ""
  );
}

/** The Summary rail's own "Grand total" figure, read from the rail it rendered. */
function railGrandTotal(): string {
  const text =
    document.querySelector<HTMLElement>('[data-tutorial="quotation-totals"]')
      ?.textContent ?? "";
  const match = /Grand total\s*(Rp[\d.,]+)/.exec(text);
  if (match === null) throw new Error("no grand total in the Summary rail");
  return match[1];
}

async function openQuotationEditor(): Promise<void> {
  await act(async () => {
    root.render(
      <ToastProvider>
        <App />
      </ToastProvider>
    );
  });
  await waitFor(
    () => document.querySelector('button[aria-label="Open Quotation"]') !== null,
    "the home page"
  );
  act(() => button("Open Quotation").click());
  await waitFor(() => heroText().includes(FIRST_CUSTOMER), "the hero customer");
}

describe("Quotation hero (issue #623)", () => {
  it("mirrors the customer, validity, payment, terms and tax as they change", async () => {
    await openQuotationEditor();

    expect(heroText()).toContain("New quotation");
    expect(heroText()).toContain("DRAFT");
    expect(heroText()).toContain("30 days");
    expect(heroText()).toContain("90 days");
    expect(heroText()).toContain("EXW · IDR");
    expect(heroText()).toContain("FTZ · Goods");
    expect(heroText()).toContain(
      "Not saved yet. Save draft stores it so you can continue later."
    );
    expect(heroTotal()).toBe("Rp0");

    // Issue #627: the hero sits in the left column of the PO editor's grid,
    // not above it. It is the column's first child, the details and lines
    // follow it, and the Summary rail is the grid's second column, so the rail
    // starts level with the hero from lg rather than under the whole form.
    const heroColumn = hero().parentElement;
    const grid = heroColumn?.parentElement;
    const details = document.querySelector<HTMLElement>(
      '[data-tutorial="quotation-details"]'
    );
    const rail = document.querySelector<HTMLElement>(
      '[data-tutorial="quotation-totals"]'
    )?.parentElement;
    expect(heroColumn?.className).toContain("flex");
    expect(heroColumn?.firstElementChild).toBe(hero());
    expect(details?.parentElement).toBe(heroColumn);
    expect(grid?.className).toContain(
      "lg:grid-cols-[minmax(0,1fr)_320px]"
    );
    expect(grid?.className).toContain(
      "xl:grid-cols-[minmax(0,1fr)_340px]"
    );
    expect(grid?.children[0]).toBe(heroColumn);
    expect(grid?.children[1]).toBe(rail);
    expect(rail?.contains(document.querySelector('[data-tutorial="quotation-totals"]'))).toBe(
      true
    );
    expect(grid?.parentElement?.className).toContain("max-w-[1280px]");

    pickOption("Customer", SECOND_CUSTOMER);
    typeInto(input("Validity"), "45");
    typeInto(input("Payment"), "60");
    pickOption("Incoterm", "DAP");
    pickRadio("Tax zone", "Non-FTZ");

    expect(heroText()).toContain(SECOND_CUSTOMER);
    expect(heroText()).not.toContain(FIRST_CUSTOMER);
    expect(heroText()).toContain("45 days");
    expect(heroText()).toContain("60 days");
    expect(heroText()).toContain("DAP · IDR");
    expect(heroText()).toContain("Non-FTZ · Goods");
  });

  it("keeps the Prepare status banner above the grid, full width", async () => {
    // Issue #627: the PO editor's banners sit above its grid, full width; the
    // Quotation editor's Prepare status now does the same, so a failure is not
    // squeezed into the left column beside the rail.
    await openQuotationEditor();

    // Prepare is gated on a renderable line, and a refused Prepare is the
    // only status that stays on screen long enough to read its placement.
    typeInto(input("Description for line 1"), "Banner layout");
    mocks.prepare.mockRejectedValueOnce(new Error("server said no"));
    act(() => textButton("Prepare").click());

    const grid = hero().parentElement?.parentElement;
    const wrapper = grid?.parentElement;
    await waitFor(
      () => wrapper?.querySelector('[role="alert"]') != null,
      "the Prepare banner"
    );
    const banner = wrapper?.querySelector<HTMLElement>('[role="alert"]');
    expect(banner?.textContent).toContain("server said no");
    expect(banner?.parentElement).toBe(wrapper);
    expect(grid?.parentElement).toBe(wrapper);
    expect(grid?.contains(banner!)).toBe(false);
    expect(
      banner!.compareDocumentPosition(grid!) & Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0);
  });

  it("prices the live customer total through computePricing, PPN included", async () => {
    await openQuotationEditor();

    pickRadio("Tax zone", "Non-FTZ");
    typeInto(input("Unit price for line 1"), "1000000");

    // 1,000,000 cost, no margin, 11% PPN on top: the customer-facing total,
    // not the sell subtotal the internal rail starts from.
    expect(heroTotal()).toBe("Rp1.110.000");
    expect(heroText()).toContain("Includes PPN");

    pickRadio("Tax type", "Services");
    pickOption("Service category", "Non-construction — 2% PPh23");

    expect(heroText()).toContain("Non-FTZ · Services · Non-construction");
    expect(heroTotal()).toBe("Rp1.000.000");
    expect(heroText()).not.toContain("Includes PPN");
  });

  it("shows the same customer total as the Summary rail, live", async () => {
    // Issue #622: App prices once and hands the object to both surfaces. This
    // is the behavioural half of that: whatever the editor is showing, the
    // hero figure and the rail figure are the same figure.
    await openQuotationEditor();

    expect(heroTotal()).toBe(railGrandTotal());

    pickRadio("Tax zone", "Non-FTZ");
    typeInto(input("Unit price for line 1"), "1000000");
    expect(heroTotal()).toBe("Rp1.110.000");
    expect(railGrandTotal()).toBe("Rp1.110.000");

    typeInto(input("Quantity for line 1"), "3");
    expect(heroTotal()).toBe(railGrandTotal());

    pickRadio("Tax type", "Services");
    expect(heroTotal()).toBe(railGrandTotal());
    expect(heroTotal()).toBe("Rp3.000.000");
  });

  it("reports a saved draft without leaving its draft state", async () => {
    await openQuotationEditor();

    typeInto(
      document.querySelector<HTMLInputElement>(
        'input[placeholder="Supply and installation of electrical and mechanical systems"]'
      )!,
      "Hero draft"
    );
    act(() => textButton("Save draft").click());
    await waitFor(
      () => heroText().includes("Draft saved"),
      "the saved draft status"
    );

    expect(mocks.saveDraft).toHaveBeenCalledTimes(1);
    expect(heroText()).toContain(
      "Draft saved just now — save again to update it."
    );
    expect(heroText()).toContain("DRAFT");
    expect(heroText()).toContain("New quotation");
    expect(heroText()).not.toContain("CURRENT REV");
  });

  it("shows the quotation number and both revisions when editing an issued one", async () => {
    await openQuotationEditor();

    act(() => textButton("Archive").click());
    await waitFor(
      () =>
        Array.from(document.querySelectorAll("button")).some(
          (candidate) => candidate.textContent?.trim() === QUOTE_NUMBER
        ),
      "the archive row"
    );
    act(() => textButton(QUOTE_NUMBER).click());
    await flush();
    await flush();
    act(() => textButton("Edit revision 02").click());
    await waitFor(() => heroText().includes(QUOTE_NUMBER), "the issued hero");

    expect(mocks.setRevisionContext).toHaveBeenCalledWith(QUOTE_NUMBER, 3);
    expect(heroText()).toContain(QUOTE_NUMBER);
    expect(heroText()).not.toContain("New quotation");
    expect(heroText()).toContain("CURRENT REV 02");
    expect(heroText()).toContain("NEXT REV 03");
    expect(heroText()).not.toContain("DRAFT");
    expect(heroText()).toContain(
      "Revision 02 stays as issued. Export creates revision 03."
    );
    // The issued revision's own terms arrive through the editor state.
    expect(heroText()).toContain("DAP · IDR");
    expect(heroText()).toContain("45 days");
  });
});
