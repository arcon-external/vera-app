// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderSourceLineDTO } from "../poFinalize";
import type { PurchaseOrderDTO, PurchaseOrderDTOItem } from "../purchaseOrder";
import { PurchaseOrderEditorPage, type POEditorShellActions } from "./PurchaseOrderEditor";
import type { ReconciliationSession } from "../poReconciliation";
import type { ReconciliationTransport } from "../poReconciliationAdapter";

// Ticket 05: line-level reconciliation in the real editor. The reducer, the
// hook's line-set sync, the pane cards and the shell gate are production
// code; only the desktop bindings are replaced and the reconciliation
// transport is a fake. Covers: blur commits create line items, rematch and
// removal flow through the declarative sync, the duplicate blocker (red
// proof), the totals strip and LineIdentities on save.

const {
  reviewMock,
  updateMock,
  documentStateMock,
} = vi.hoisted(() => ({
  reviewMock: vi.fn(),
  updateMock: vi.fn(),
  documentStateMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  CorrectPurchaseOrder: vi.fn(),
  CreatePurchaseOrderBlankDraft: vi.fn(),
  FinalizePurchaseOrder: vi.fn(),
  GetActivePurchaseOrderDocumentContent: vi.fn(),
  GetPurchaseOrder: vi.fn(),
  GetPurchaseOrderDocumentState: (...args: unknown[]) => documentStateMock(...args),
  GetPurchaseOrderDocumentVersionContent: vi.fn(),
  GetPurchaseOrderDocumentVersion: vi.fn(),
  GetPurchaseOrderHistory: vi.fn(async () => []),
  GetPurchaseOrderReview: (...args: unknown[]) => reviewMock(...args),
  LinkPurchaseOrderDraftSource: vi.fn(),
  PreviewPurchaseOrderDraftLink: vi.fn(),
  RemovePurchaseOrderDocument: vi.fn(),
  UpdatePurchaseOrderDraft: (...args: unknown[]) => updateMock(...args),
  AssignPurchaseOrderConfirmationNumber: vi.fn(),
  AttachPurchaseOrderDocument: vi.fn(),
}));

vi.mock("@backend/models", () => {
  class Input {
    constructor(source: Record<string, unknown>) {
      Object.assign(this, source);
    }
  }
  return {
    backend: {
      PurchaseOrderCorrectionInput: Input,
      PurchaseOrderFinalizeInput: Input,
      PurchaseOrderItemInput: Input,
      PurchaseOrderUpdateInput: Input,
      PurchaseOrderBlankCreateInput: Input,
    },
  };
});

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

const UOMS = [
  { value: "Pc", label: "Pc" },
  { value: "Box", label: "Box" },
];
const INCOTERMS = [
  { value: "EXW", label: "EXW" },
  { value: "CIF", label: "CIF" },
];

function sourceLine(overrides: Partial<PurchaseOrderSourceLineDTO> = {}): PurchaseOrderSourceLineDTO {
  return {
    QuotationItemNo: 1,
    QuotationSubNo: null,
    LineLabel: "1",
    Description: "Widget",
    UoM: "Pc",
    QuantityScaled: 20000,
    UnitPriceMinor: 10000,
    DiscountEnabled: false,
    DiscountPercent: 0,
    ...overrides,
  };
}

function dtoItem(overrides: Partial<PurchaseOrderDTOItem> = {}): PurchaseOrderDTOItem {
  return {
    LineNumber: 1,
    Description: "Widget",
    QuantityScaled: 20000,
    UoM: "Pc",
    UnitPriceMinor: 10000,
    DiscountEnabled: false,
    DiscountPercent: 0,
    GrossMinor: 200000000,
    DiscountMinor: 0,
    NetMinor: 200000000,
    QuotationItemNo: 1,
    QuotationSubNo: null,
    ...overrides,
  };
}

function linkedOrder(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    PODate: "2026-10-01",
    RequestedDeliveryDate: null,
    UUID: "uuid-517",
    State: "draft",
    QuoteNumber: "1502260052",
    CustomerName: "PT. Buyer",
    BillingAddress: "",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "PO-517",
    ReceivedDate: "2026-10-02",
    Incoterms: "EXW",
    Currency: "IDR",
    TaxZone: "FTZ",
    TaxCategory: "SupplyOnly",
    TaxType: "Goods",
    ServiceCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-02T10:00:00Z",
    UpdatedAt: "2026-10-02T10:00:00Z",
    Items: [dtoItem()],
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    SourceMissing: false,
    CustomerID: 2,
    PaymentTerms: "45 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 0,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 0,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    Version: 4,
    ...overrides,
  };
}

/** The review source matches the order except for payment terms. */
function reviewSource(lines: PurchaseOrderSourceLineDTO[]) {
  return {
    QuotationID: 12,
    RevisionNumber: 1,
    QuoteNumber: "1502260052",
    CustomerID: 2,
    CustomerName: "PT. Buyer",
    Currency: "IDR",
    PaymentTerms: "45 days",
    Incoterms: "EXW",
    DiscountPercent: 0,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    Lines: lines,
  };
}

function fakeTransport(initial: ReconciliationSession | null): ReconciliationTransport & {
  stored: ReconciliationSession | null;
  saves: ReconciliationSession[];
} {
  const backend: ReconciliationTransport & {
    stored: ReconciliationSession | null;
    saves: ReconciliationSession[];
  } = {
    stored: initial,
    saves: [],
    load: vi.fn(async () => {
      if (backend.stored === null) throw new Error("no reconciliation recorded");
      return backend.stored;
    }),
    save: vi.fn(async (_poID: string, session: ReconciliationSession) => {
      backend.saves.push(session);
      backend.stored = {
        ...session,
        ID: 3,
        POVersion: session.POVersion + 1,
        UpdatedAt: "2026-10-28T00:00:02Z",
      };
      return backend.stored;
    }),
  };
  return backend;
}

let container: HTMLDivElement;
let root: Root;
let shellActions: POEditorShellActions | null;

beforeEach(() => {
  Element.prototype.scrollIntoView = () => undefined;
  (Element.prototype as unknown as { scrollTo?: () => void }).scrollTo = () => undefined;
  reviewMock.mockReset();
  updateMock.mockReset();
  documentStateMock.mockReset();
  documentStateMock.mockResolvedValue({ Document: null, Versions: [] });
  updateMock.mockImplementation(async (input: unknown) => {
    const fields = input as Record<string, unknown>;
    return {
      Order: linkedOrder({
        PaymentTerms: String(fields.PaymentTerms ?? "45 days"),
        ConfirmationRequested: Boolean(fields.ConfirmationRequested),
        Version: Number(fields.ExpectedVersion ?? 4),
      }),
    };
  });
  shellActions = null;
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

function input(label: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`);
  if (found === null) throw new Error(`no input labelled ${label}`);
  return found;
}

function checkbox(label: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    `input[type="checkbox"][aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no checkbox labelled ${label}`);
  return found;
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function typeInto(element: HTMLInputElement, value: string): void {
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    setter?.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function blur(element: HTMLElement): Promise<void> {
  return act(async () => {
    element.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
  });
}

async function renderEditor(
  transport: ReconciliationTransport,
  order: PurchaseOrderDTO = linkedOrder()
) {
  await act(async () => {
    root.render(
      <PurchaseOrderEditorPage
        order={order}
        currency={IDR}
        uomOptions={UOMS}
        incotermOptions={INCOTERMS}
        onSaved={() => undefined}
        onCancel={() => undefined}
        actionsInShell
        onShellActions={(actions) => {
          shellActions = actions;
        }}
        reconciliationTransport={transport}
        actorId={7}
        actorName="Rina"
      />
    );
  });
  await flush();
  await flush();
}

describe("line reconciliation in the editor (ticket 05)", () => {
  it("a differing line blur commit creates one unresolved line item", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    // The match chip names the linked quotation line.
    expect(document.body.textContent).toContain("Quotation 1");

    const quantity = input("Quantity for line 1");
    typeInto(quantity, "3");
    await blur(quantity);
    await flush();

    expect(document.body.textContent).toContain("Line 1 quantity");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Line 1 quantity"]')
    ).not.toBeNull();
    // The typed value is untouched: the decision is recorded, not the edit.
    expect(input("Quantity for line 1").value).toBe("3");

    // The Final Save gate names the line blocker.
    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.unresolvedCount).toBe(1);
    expect(shellActions.blockedMessage).toContain("Line 1 quantity");
  });

  it("a matching line blur commit shows the match notice and creates no item", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const quantity = input("Quantity for line 1");
    typeInto(quantity, "2.0000");
    await blur(quantity);
    await flush();

    expect(document.body.textContent).toContain("Line 1 quantity: matches the Quotation");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Line 1 quantity"]')
    ).toBeNull();
    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.unresolvedCount).toBe(0);
  });

  it("duplicate matches block Final Save until one match is corrected", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(
      transport,
      linkedOrder({
        Items: [
          dtoItem({ LineNumber: 1 }),
          dtoItem({ LineNumber: 2, Description: "Widget again", QuotationItemNo: 1 }),
        ],
      })
    );

    // Both lines claim quotation line 1: the duplicate blocks by name.
    await flush();
    if (shellActions === null) throw new Error("no shell actions reported");
    expect(document.body.textContent).toContain("Duplicate match · Quotation 1");
    expect(shellActions.blockedMessage).toContain("Duplicate match · Quotation 1");

    // Correcting line 2 to an extra line clears the duplicate blocker (the
    // resolved duplicate stays visible as history; the gate moves on).
    button("Change quotation match for Line 2").click();
    await flush();
    button("Line 2 quotation match").click();
    await flush();
    const extra = document.querySelector<HTMLButtonElement>('button[role="option"]');
    if (extra === null) throw new Error("no match options opened");
    // Pick the Extra line option (the first option).
    extra.click();
    await flush();
    await flush();

    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.blockedMessage?.includes("Duplicate match")).toBe(false);
    expect(shellActions.blockedMessage).toContain("Extra line");
  });

  it("an unknown reference blocks until corrected to a valid match", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(
      transport,
      linkedOrder({
        Items: [dtoItem({ QuotationItemNo: 99, QuotationSubNo: null, Description: "Mystery" })],
      })
    );

    // Canonical unknown card: names the reference, offers correction.
    await flush();
    if (shellActions === null) throw new Error("no shell actions reported");
    expect(document.body.textContent).toContain("Unknown reference · 99");
    expect(document.body.textContent).toContain("No Quotation line 99 exists.");
    expect(shellActions.blockedMessage).toContain("Unknown reference · 99");

    // Correcting the match to the valid reference keeps a valid shape.
    button("Change quotation match for Line 1").click();
    await flush();
    button("Line 1 quotation match").click();
    await flush();
    const allOptions = Array.from(
      document.querySelectorAll<HTMLButtonElement>('button[role="option"]')
    );
    const valid = allOptions.find((el) => (el.textContent ?? "").includes("Quotation item 1 ·"));
    if (valid === undefined) throw new Error("no Quotation item 1 option");
    valid.click();
    await flush();
    await flush();

    // The correction resolves everything: no blocker remains.
    expect(shellActions.blockedMessage).toBeNull();
    expect(document.body.textContent).toContain("Match corrected");
  });

  it("removing a matched line resolves its items as line_removed with history kept", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const quantity = input("Quantity for line 1");
    typeInto(quantity, "3");
    await blur(quantity);
    await flush();
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Line 1 quantity"]')
    ).not.toBeNull();

    // Add a second line so the matched one can be removed and its items can
    // be inspected as history while a line remains.
    button("Add line").click();
    await flush();
    button("Remove line 1").click();
    await flush();

    // The removed row is gone; the blank added row moved up into its place.
    const remainingDescriptions = Array.from(
      document.querySelectorAll<HTMLInputElement>('input[aria-label^="Description for line"]')
    );
    expect(remainingDescriptions).toHaveLength(1);
    expect(remainingDescriptions[0]?.value).toBe("");
    const resolvedTab = document.querySelector<HTMLButtonElement>("#rail-filter-resolved");
    if (resolvedTab === null) throw new Error("no Resolved filter tab");
    await act(async () => {
      resolvedTab.click();
    });
    await flush();
    expect(document.body.textContent).toContain("Line removed");
  });

  it("the totals strip reads matched, then names the differing input", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    expect(document.body.textContent).toContain("Totals check");
    expect(document.body.textContent).toContain("Matches the printed totals");

    const quantity = input("Quantity for line 1");
    typeInto(quantity, "3");
    await blur(quantity);
    await flush();
    await flush();

    expect(document.body.textContent).toContain("Different");
  });

  it("a draft save submits the persisted id and the editor alias per line", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(
      transport,
      linkedOrder({
        ID: 7,
        Version: 4,
        Items: [
          {
            ID: 101,
            ClientKey: "client:aaa",
            LineNumber: 1,
            Description: "Widget",
            QuantityScaled: 20000,
            UoM: "Pc",
            UnitPriceMinor: 10000,
            DiscountEnabled: false,
            DiscountPercent: 0,
            GrossMinor: 200000000,
            DiscountMinor: 0,
            NetMinor: 200000000,
            QuotationItemNo: 1,
            QuotationSubNo: null,
          },
        ],
      })
    );

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onSave();
    });
    await flush();
    await flush();
    const sent = updateMock.mock.calls[0]?.[0] as { Items?: { ID?: unknown; ClientKey?: unknown }[] };
    expect(sent.Items).toHaveLength(1);
    expect(sent.Items?.[0]?.ID).toBe(101);
    expect(sent.Items?.[0]?.ClientKey).toBe("client:aaa");

    // The session sync records the same alias as the line identity.
    const save = transport.save as ReturnType<typeof vi.fn>;
    expect(save).toHaveBeenCalledTimes(1);
    const session = save.mock.calls[0][1] as ReconciliationSession;
    expect(session.LineIdentities).toEqual([
      { Key: "client:aaa", ID: 101, Removed: false },
    ]);
  });
});

describe("committed select and checkbox values (F3)", () => {
  it("selecting a different UoM commits the new value, not the stale one", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const select = document.querySelector<HTMLSelectElement>(
      'select[aria-label="Unit of measure for line 1"]'
    );
    if (select === null) throw new Error("no UoM select");
    await act(async () => {
      select.value = "Box";
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await blur(select);
    await flush();
    await flush();

    expect(document.body.textContent).toContain("Line 1 unit of measure");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Line 1 unit of measure"]')
    ).not.toBeNull();
  });

  it("toggling a row discount commits the new enabled state, not the stale one", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const toggle = button("Enable a discount for line 1");
    await act(async () => {
      toggle.click();
    });
    await flush();
    await flush();

    // Enabled 0% differs from the source disabled 0%: a decision is recorded.
    expect(button("Enable a discount for line 1").getAttribute("aria-checked")).toBe("true");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Line 1 discount"]')
    ).not.toBeNull();
  });

  it("changing Incoterms commits the selected value, not the stale one", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    button("Incoterms").click();
    await flush();
    const cif = Array.from(
      document.querySelectorAll<HTMLButtonElement>('button[role="option"]')
    ).find((el) => el.textContent === "CIF");
    if (cif === undefined) throw new Error("no CIF option");
    cif.click();
    await flush();
    await flush();

    expect(document.body.textContent).toContain("Incoterms");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Incoterms"]')
    ).not.toBeNull();
  });
});

describe("small-screen sheet bounds (U4)", () => {
  it("sheet starts at the viewport top below 640px and below the header above", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const trigger = document.querySelector<HTMLButtonElement>(
      'button[aria-expanded="false"]'
    );
    if (trigger === null) throw new Error("no sheet trigger");
    await act(async () => {
      trigger.click();
    });
    await flush();
    const sheet = document.querySelector('[role="region"][aria-label="Reconciliation"]');
    if (sheet === null) throw new Error("no open sheet");
    const tokens = sheet.className.split(/\s+/);
    // Round-2 U4: the sheet starts below the measured shell header instead of
    // viewport top, so the heading and Close stay visible at 360.
    expect(tokens).toContain("top-[var(--shell-header-h,64px)]");
  });
});

describe("line identity and match control (U1)", () => {
  it("shows Line n with its match chip and opens one select at a time", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    // The row identity is the numbered description input; the chip names the
    // linked quotation line.
    expect(document.querySelector('input[aria-label="Description for line 1"]')).not.toBeNull();
    // The narrow 360 header never wraps the identity chip (P3).
    const chip = Array.from(document.querySelectorAll('span')).find((node) => (node.textContent ?? '').startsWith('Quotation 1'));
    if (chip === undefined) throw new Error('no match chip');
    expect(chip.className).toContain('whitespace-nowrap');
    // Only the Change control is visible; the select opens on demand.
    expect(document.querySelector('button[aria-label="Change quotation match for Line 1"]')).not.toBeNull();
    expect(document.querySelector('button[aria-label="Line 1 quotation match"]')).toBeNull();

    button("Change quotation match for Line 1").click();
    await flush();
    expect(document.querySelector('button[aria-label="Line 1 quotation match"]')).not.toBeNull();
  });

  it("adding a line focuses the new description and removing focuses the next line", async () => {
    reviewMock.mockResolvedValue({ Source: reviewSource([sourceLine()]), DocumentState: "none" });
    const transport = fakeTransport(null);
    await renderEditor(transport);

    button("Add line").click();
    await flush();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Description for line 2");
    expect(document.body.textContent).toContain("Line 2 added.");

    button("Remove line 1").click();
    await flush();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Description for line 1");
    expect(document.body.textContent).toContain("Line 1 removed.");
  });
});

// Issue #573: the line rows are the quotation ItemEditor. Splitting a plain
// line keeps its priced row as the first sub-item and adds one empty sibling,
// with the same sub-item and sub-sub controls the quotation uses.
describe("split lines in the PO editor (#573)", () => {
  it("splits a plain line into a parent and two sub-items", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null));

    const split = Array.from(document.querySelectorAll("button")).find((candidate) =>
      candidate.textContent?.includes("Split into sub-items")
    );
    if (split === undefined) throw new Error("no split control");
    act(() => split.click());
    await flush();

    expect(document.body.textContent).toContain("2 sub-items");
    expect(document.body.textContent).toContain("Sub-items — priced individually");
    expect(input("Description for line 1").value).toBe("Widget");
    expect(input("Description for sub-item 1 of line 1").value).toBe("Widget");
    expect(input("Description for sub-item 2 of line 1").value).toBe("");

    const addSubSub = Array.from(document.querySelectorAll("button")).filter(
      (candidate) => candidate.textContent?.includes("Add sub-sub-item")
    );
    expect(addSubSub).toHaveLength(2);
    expect(document.body.textContent).toContain("1 line · 2 sub-items");
  });
});

// Issue #580: the workspace is PO-centric -- the hero names the customer's PO
// number, the PO-owned details sit directly under it, and the quotation
// identity stays in the Source quotation block. The same issue lets the last
// line or split group be removed and saves a zero-line draft.
describe("PO workspace header and line controls (#580)", () => {
  function heroText(): string {
    return document.querySelector("section")?.textContent ?? "";
  }

  function detailsCard(): HTMLElement {
    const heading = Array.from(document.querySelectorAll("h2")).find(
      (node) => node.textContent === "Purchase order details"
    );
    if (heading === undefined) throw new Error("no Purchase order details card");
    const card = heading.parentElement?.parentElement;
    if (!card) throw new Error("no card around the details heading");
    return card as HTMLElement;
  }

  it("shows the customer PO number in the hero and the quotation in the source block", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null), linkedOrder({ PONumber: "PO-580-HERO" }));

    expect(document.querySelector("h1")?.textContent).toBe("PO-580-HERO");
    expect(document.querySelector("h1")?.textContent).not.toContain("1502260052");
    expect(heroText()).toContain("Source quotation");
    expect(heroText()).toContain("1502260052");
    expect(heroText()).toContain("rev 01");
  });

  it("falls back to New purchase order when the customer PO number is empty", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null), linkedOrder({ PONumber: "" }));

    expect(document.querySelector("h1")?.textContent).toBe("New purchase order");
    // The source quotation block still says which revision this follows.
    expect(heroText()).toContain("1502260052");
    expect(heroText()).toContain("rev 01");
  });

  it("owns every editable PO detail in the card under the hero and updates the hero live", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null));

    const hero = document.querySelector("section");
    const card = detailsCard();
    expect(hero?.nextElementSibling).toBe(card);

    for (const label of [
      "Customer PO number",
      "PO date",
      "Received date",
      "Requested delivery date",
      "Payment terms",
      "Incoterms",
    ]) {
      expect(card.querySelector(`[aria-label="${label}"]`)).not.toBeNull();
      // One control per value: the fields moved here, they were not copied.
      expect(document.querySelectorAll(`[aria-label="${label}"]`)).toHaveLength(1);
    }

    // Customer and quotation identity remain read-only snapshots.
    expect(document.querySelector('[aria-label="Customer name"]')).toBeNull();
    expect(document.querySelector('[aria-label="Quote number"]')).toBeNull();
    expect(document.querySelector('[aria-label="Revision"]')).toBeNull();

    typeInto(input("Customer PO number"), "PO-580-LIVE");
    expect(document.querySelector("h1")?.textContent).toBe("PO-580-LIVE");

    typeInto(input("PO date"), "2026-11-01");
    typeInto(input("Requested delivery date"), "2026-12-15");
    expect(heroText()).toContain("1 Nov 2026");
    expect(heroText()).toContain("15 Dec 2026");

    button("Incoterms").click();
    await flush();
    const cif = Array.from(
      document.querySelectorAll<HTMLButtonElement>('button[role="option"]')
    ).find((el) => el.textContent === "CIF");
    if (cif === undefined) throw new Error("no CIF option");
    cif.click();
    await flush();
    expect(heroText()).toContain("CIF");
  });

  it("removes the last line, shows the empty state and saves a zero-line draft", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null));

    const remove = button("Remove line 1");
    // A visible, enabled control: the last line is no longer protected.
    expect(remove.disabled).toBe(false);
    act(() => remove.click());
    await flush();

    expect(document.body.textContent).toContain(
      "No lines yet. Add the customer's first line."
    );
    expect(document.querySelector('input[aria-label="Description for line 1"]')).toBeNull();

    // Zero lines is a valid draft: the save sends an empty Items list.
    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onSave();
    });
    await flush();
    await flush();
    const sent = updateMock.mock.calls[0]?.[0] as { Items?: unknown[] };
    expect(sent.Items).toEqual([]);

    // Reopen the stored zero-line draft: the empty state is what loads.
    act(() => root.unmount());
    root = createRoot(container);
    await renderEditor(fakeTransport(null), linkedOrder({ Items: [] }));
    expect(document.body.textContent).toContain(
      "No lines yet. Add the customer's first line."
    );
  });

  it("refuses a zero-line final save before any review opens", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null), linkedOrder({ Items: [] }));

    // The live-source load reads the review once on mount; Final Save must not
    // ask for another one after the zero-line blocker refuses it.
    reviewMock.mockClear();
    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();

    expect(reviewMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain(
      "Add at least one line before Final Save."
    );
  });

  it("removes the last split group with a labelled control for the whole line", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null));

    const split = Array.from(document.querySelectorAll("button")).find((candidate) =>
      candidate.textContent?.includes("Split into sub-items")
    );
    if (split === undefined) throw new Error("no split control");
    act(() => split.click());
    await flush();

    const remove = button("Remove split line 1");
    expect(remove.disabled).toBe(false);
    // The sub-item controls stay distinct and visible.
    expect(
      document.querySelector('button[aria-label="Remove sub-item 1 of line 1"]')
    ).not.toBeNull();
    expect(
      document.querySelector('button[aria-label="Remove sub-item 2 of line 1"]')
    ).not.toBeNull();

    act(() => remove.click());
    await flush();

    // The parent takes its sub-items with it: no orphan rows remain.
    expect(document.querySelector('input[aria-label="Description for line 1"]')).toBeNull();
    expect(
      document.querySelector('input[aria-label="Description for sub-item 1 of line 1"]')
    ).toBeNull();
    expect(document.body.textContent).toContain(
      "No lines yet. Add the customer's first line."
    );
  });

  it("removing the last sub-item returns the editor to the empty state", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null));

    const split = Array.from(document.querySelectorAll("button")).find((candidate) =>
      candidate.textContent?.includes("Split into sub-items")
    );
    if (split === undefined) throw new Error("no split control");
    act(() => split.click());
    await flush();

    act(() => button("Remove sub-item 2 of line 1").click());
    await flush();
    expect(document.body.textContent).toContain("1 sub-item");

    act(() => button("Remove sub-item 1 of line 1").click());
    await flush();
    expect(document.body.textContent).toContain(
      "No lines yet. Add the customer's first line."
    );
  });
});

// Issue #581: the confirmation-number request is a draft preference. It starts
// off, travels with Save draft, survives reopen, and seeds (but does not bind)
// the final confirmation checkbox.
describe("confirmation-number request (#581)", () => {
  it("starts off, saves with the draft and survives reopen", async () => {
    reviewMock.mockResolvedValue({
      Source: reviewSource([sourceLine()]),
      DocumentState: "none",
    });
    await renderEditor(fakeTransport(null));

    const label = "Generate a confirmation number when this purchase order is saved";
    const toggle = checkbox(label);
    expect(toggle.checked).toBe(false);
    expect(document.body.textContent).toContain("issued at Final Save");
    expect(document.body.textContent).toContain("cannot be edited afterwards");

    act(() => toggle.click());
    await flush();
    expect(checkbox(label).checked).toBe(true);

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onSave();
    });
    await flush();
    await flush();

    const sent = updateMock.mock.calls[0]?.[0] as { ConfirmationRequested?: boolean };
    expect(sent.ConfirmationRequested).toBe(true);
    // The save round-trip returns the persisted request; the toggle stays on.
    expect(checkbox(label).checked).toBe(true);

    act(() => root.unmount());
    root = createRoot(container);
    await renderEditor(fakeTransport(null), linkedOrder({ ConfirmationRequested: true }));
    expect(checkbox(label).checked).toBe(true);
  });
});
