// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderSourceDTO } from "../poFinalize";
import type { PurchaseOrderDTO, PurchaseOrderDTOItem } from "../purchaseOrder";
import { PurchaseOrderEditorPage, type POEditorShellActions } from "./PurchaseOrderEditor";
import type { ReconciliationSession } from "../poReconciliation";
import type { ReconciliationTransport } from "../poReconciliationAdapter";

// Ticket 04: the live header reconciliation in the real editor. The reducer,
// the pane, the session hook and the shell bridge are production code; only
// the desktop bindings are replaced and the reconciliation transport is a
// fake. Covers: blur commits create items without touching other fields,
// keep/use resolution, draft save persisting the session, reopen rehydrating
// it, and the Final Save gate with its red proof.

const {
  reviewMock,
  updateMock,
  finalizeMock,
  documentStateMock,
} = vi.hoisted(() => ({
  reviewMock: vi.fn(),
  updateMock: vi.fn(),
  finalizeMock: vi.fn(),
  documentStateMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  CorrectPurchaseOrder: vi.fn(),
  CreatePurchaseOrderBlankDraft: vi.fn(),
  FinalizePurchaseOrder: (...args: unknown[]) => finalizeMock(...args),
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

const UOMS = [{ value: "Pc", label: "Pc" }];
const INCOTERMS = [
  { value: "EXW", label: "EXW" },
  { value: "CIF", label: "CIF" },
];

const SOURCE: PurchaseOrderSourceDTO = {
  QuotationID: 12,
  RevisionNumber: 1,
  QuoteNumber: "1502260052",
  CustomerID: 2,
  CustomerName: "PT. Buyer",
  Currency: "IDR",
  PaymentTerms: "30 days",
  Incoterms: "CIF",
  DiscountPercent: 0,
  PrintedSubtotalMinor: 0,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 0,
  Lines: [],
};

function linkedOrder(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    PODate: "2026-10-01",
    RequestedDeliveryDate: null,
    UUID: "uuid-515",
    State: "draft",
    QuoteNumber: "1502260052",
    CustomerName: "PT. Buyer",
    BillingAddress: "",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "PO-515",
    ReceivedDate: "2026-10-02",
    Incoterms: "EXW",
    Currency: "IDR",
    TaxZone: "FTZ",
    TaxCategory: "SupplyOnly",
    AdjustmentReason: "",
    CreatedAt: "2026-10-02T10:00:00Z",
    UpdatedAt: "2026-10-02T10:00:00Z",
    Items: [],
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
    PrintedSubtotalMinor: 0,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 0,
    Version: 4,
    ...overrides,
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
  reviewMock.mockReset();
  updateMock.mockReset();
  finalizeMock.mockReset();
  documentStateMock.mockReset();
  documentStateMock.mockResolvedValue({ Document: null, Versions: [] });
  reviewMock.mockResolvedValue({ Source: SOURCE, DocumentState: "none" });
  updateMock.mockImplementation(async (input: unknown) => {
    const fields = input as Record<string, unknown>;
    return {
      Order: linkedOrder({
        PaymentTerms: String(fields.PaymentTerms ?? "45 days"),
        Incoterms: String(fields.Incoterms ?? "EXW"),
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

async function renderEditor(
  transport: ReconciliationTransport,
  order: PurchaseOrderDTO = linkedOrder(),
  mode: "draft" | "correction" = "draft"
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
        mode={mode}
      />
    );
  });
  await flush();
  await flush();
}

describe("live header reconciliation in the editor (ticket 04)", () => {
  it("a differing blur commit creates one unresolved session item", async () => {
    const transport = fakeTransport(null);
    await renderEditor(transport);

    // The PO says 45 days, the source says 30: leaving the field commits.
    const field = input("Payment terms");
    typeInto(field, "50% advance");
    await act(async () => {
      field.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();

    expect(document.body.textContent).toContain("Payment terms");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Payment terms"]')
    ).not.toBeNull();
    // The unsaved value is untouched: the decision is recorded, not the edit.
    expect(input("Payment terms").value).toBe("50% advance");
  });

  it("a matching blur commit shows the match notice and creates no item", async () => {
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const field = input("Payment terms");
    typeInto(field, "30 days");
    await act(async () => {
      field.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();

    expect(document.body.textContent).toContain("Matches the Quotation");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Payment terms"]')
    ).toBeNull();
  });

  it("keep asks Is this correct and resolves; use updates the field", async () => {
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const field = input("Payment terms");
    typeInto(field, "50% advance");
    await act(async () => {
      field.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();

    button("Keep PO entry for Payment terms").click();
    await flush();
    expect(document.body.textContent).toContain("Is this correct?");
    button("Yes, keep PO entry for Payment terms").click();
    await flush();
    expect(document.body.textContent).toContain("Kept PO");

    // Saving the draft persists the kept decision through the adapter.
    const save = transport.save as ReturnType<typeof vi.fn>;
    updateMock.mockClear();
    save.mockClear();
    // The shell bridge reports Save draft; call it like the shell would.
    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onSave();
    });
    await flush();
    expect(updateMock).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledTimes(1);
    const sent = save.mock.calls[0][1] as ReconciliationSession;
    expect(sent.Items).toHaveLength(1);
    expect(sent.Items[0]).toMatchObject({
      Key: "header:paymentTerms",
      Status: "resolved",
      Resolution: "kept_po",
    });
    // R-04: the session write carries the row version the draft save produced.
    expect(sent.POVersion).toBe(4);
  });

  it("Use Quotation writes the source value into the field and resolves", async () => {
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const field = input("Payment terms");
    typeInto(field, "50% advance");
    await act(async () => {
      field.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();

    button("Use Quotation value for Payment terms").click();
    await flush();
    expect(input("Payment terms").value).toBe("30 days");
    expect(document.body.textContent).toContain("Used Quotation");
  });

  it("reopening rehydrates the saved items and decisions", async () => {
    const kept = {
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      POValue: "50% advance",
      SourceValue: "30 days",
      Status: "resolved",
      Resolution: "kept_po",
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:05Z",
      History: [],
    };
    const transport = fakeTransport({
      ID: 3,
      PurchaseOrderID: 7,
      SourceQuotationID: 12,
      SourceRevision: 1,
      POVersion: 5,
      Active: true,
      ReadOnly: false,
      CreatedBy: 7,
      CreatedAt: "2026-10-28T00:00:00Z",
      UpdatedAt: "2026-10-28T00:00:05Z",
      LineIdentities: [],
      Items: [kept],
    });
    await renderEditor(transport);
    expect(document.body.textContent).toContain("Kept PO");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Payment terms"]')
    ).toBeNull();
  });

  it("Final Save stays blocked while a header item is unresolved", async () => {
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const field = input("Payment terms");
    typeInto(field, "50% advance");
    await act(async () => {
      field.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();

    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.unresolvedCount).toBe(1);
    expect(shellActions.blockedMessage).toContain("Resolve 1 blocker to enable Final Save");
    expect(shellActions.blockedMessage).toContain("Payment terms");

    // The review must not open: activating the blocked save focuses the
    // first unresolved item and announces the blocker assertively.
    await act(async () => {
      shellActions?.onBlockedActivate();
    });
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain(
      "Final Save is blocked. 1 blocker: Payment terms."
    );
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "Keep PO entry for Payment terms"
    );
  });

  it("the shell names every blocker and validation failures focus their field", async () => {
    const orderWithLine = linkedOrder({
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
    });
    const transport = fakeTransport(null);
    await renderEditor(transport, orderWithLine);

    // Invalidate four text fields. All four must be named; nothing is
    // silently sliced after the third.
    typeInto(input("Line 1 quantity"), "");
    await act(async () => {
      input("Line 1 quantity").dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    typeInto(input("Line 1 unit price"), "abc");
    await act(async () => {
      input("Line 1 unit price").dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    typeInto(input("Whole-PO discount"), "999");
    await act(async () => {
      input("Whole-PO discount").dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    typeInto(input("Adjustment"), "abc");
    await act(async () => {
      input("Adjustment").dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();

    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.unresolvedCount).toBeGreaterThan(3);
    for (const label of ["Line 1 quantity", "Line 1 unit price", "Whole-PO discount", "Adjustment"]) {
      expect(shellActions.blockedMessage).toContain(label);
    }

    await act(async () => {
      shellActions?.onBlockedActivate();
    });
    await flush();
    // The first invalid field is focused so its error becomes visible.
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Line 1 quantity");
    const alerts = Array.from(document.querySelectorAll('[role="alert"]')).map(
      (node) => node.textContent
    );
    expect(
      alerts.some((text) => (text ?? "").includes("Final Save is blocked."))
    ).toBe(true);
  });

  it("Final Save commits the focused field before readiness and blocks on its error", async () => {
    const orderWithLine = linkedOrder({
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
    });
    const transport = fakeTransport(null);
    await renderEditor(transport, orderWithLine);

    // Focus the quantity, make it invalid, and invoke Final Save without
    // blurring first. Readiness must commit the pending value and block.
    const field = input("Line 1 quantity");
    await act(async () => {
      field.focus();
    });
    typeInto(field, "");
    await flush();
    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Line 1 quantity");
  });
});

describe("round-2 Final Save receipt and persistence (real page)", () => {
  const lineItem = (overrides: Partial<PurchaseOrderDTOItem> = {}): PurchaseOrderDTOItem => ({
    ID: 101,
    ClientKey: "client:a",
    LineNumber: 1,
    Description: "Widget",
    QuantityScaled: 30000,
    UoM: "Pc",
    UnitPriceMinor: 10000,
    DiscountEnabled: false,
    DiscountPercent: 0,
    GrossMinor: 30000,
    DiscountMinor: 0,
    NetMinor: 30000,
    QuotationItemNo: 1,
    QuotationSubNo: null,
    ...overrides,
  });

  const lineSource = {
    QuotationItemNo: 1,
    QuotationSubNo: null,
    LineLabel: "1",
    Description: "Widget",
    UoM: "Pc",
    QuantityScaled: 20000,
    UnitPriceMinor: 10000,
    DiscountEnabled: false,
    DiscountPercent: 0,
  };

  function savedSession(items: ReconciliationSession["Items"]): ReconciliationSession {
    return {
      ID: 3,
      PurchaseOrderID: 7,
      SourceQuotationID: 12,
      SourceRevision: 1,
      POVersion: 4,
      Active: true,
      ReadOnly: false,
      CreatedBy: 7,
      CreatedAt: "2026-10-28T00:00:00Z",
      UpdatedAt: "2026-10-28T00:00:05Z",
      LineIdentities: [{ Key: "client:a", ID: 101, Removed: false }],
      Items: items,
    };
  }

  it("an unreviewed difference renders read-only with a route back, never a choice workbench", async () => {
    reviewMock.mockResolvedValue({
      Source: {
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
        Lines: [lineSource],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem()] }),
    });
    // The draft write round-trips the working copy with its lines, exactly
    // what Save Draft stores; without the echo the fake would drop them and
    // the composed difference would vanish.
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [lineItem()],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    // A saved session with a recorded header choice: the receipt has
    // provenance, but the line difference was never reviewed and carries no
    // choice of its own.
    const transport = fakeTransport(
      savedSession([
        {
          Key: "header:paymentTerms",
          Field: "header:paymentTerms",
          Kind: "difference",
          LineKey: null,
          SourceLineRef: null,
          Label: "Payment terms",
          POValue: "50% advance",
          SourceValue: "45 days",
          Status: "resolved",
          Resolution: "kept_po",
          ActorID: 7,
          UpdatedAt: "2026-10-28T00:00:05Z",
          History: [],
        },
      ])
    );
    await renderEditor(
      transport,
      linkedOrder({ Items: [lineItem()], PaymentTerms: "50% advance" })
    );

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).toContain("Review differences before saving");
    // No second workbench: an unreviewed row never offers Use buttons.
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 quantity"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Use Quotation for Line 1 quantity"]')
    ).toBeNull();
    expect(document.body.textContent).toContain("Needs review in the reconciliation pane.");
    expect(
      document.querySelector('button[aria-label="Change Line 1 quantity"]')
    ).not.toBeNull();
  });

  it("a persisted tax-only session with an unreviewed line difference opens a read-only receipt, never a choice workbench (N4/U2)", async () => {
    reviewMock.mockResolvedValue({
      Source: {
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
        Lines: [lineSource],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem()] }),
    });
    // Same production-like update echo as the sibling receipt test: the
    // draft write round-trips the working copy with its lines.
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [lineItem()],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    // A persisted session (stored ID 3) whose only recorded provenance is
    // the resolved link-time tax basis: no recorded comparison choice or
    // ack, and the line quantity was never committed in the pane.
    const transport = fakeTransport(
      savedSession([
        {
          Key: "link:taxBasis",
          Field: "link:taxBasis",
          Kind: "tax_basis",
          LineKey: null,
          SourceLineRef: null,
          Label: "Tax basis",
          POValue: { TaxType: "Goods", TaxZone: "FTZ", ServiceCategory: "" },
          SourceValue: { TaxType: "Goods", TaxZone: "FTZ", ServiceCategory: "" },
          Status: "resolved",
          Resolution: "used_quotation",
          ActorID: 7,
          UpdatedAt: "2026-10-28T00:00:05Z",
          History: [],
        },
      ])
    );
    await renderEditor(
      transport,
      linkedOrder({ Items: [lineItem()] })
    );

    if (shellActions === null) throw new Error("no shell actions reported");
    // The quantity difference was never committed, so no pane item blocks it.
    expect(shellActions.unresolvedCount).toBe(0);
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    // The review still opens, but as a read-only receipt: no second
    // choice workbench for the unreviewed difference.
    expect(document.body.textContent).toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 quantity"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Use Quotation for Line 1 quantity"]')
    ).toBeNull();
    expect(document.body.textContent).toContain("Needs review in the reconciliation pane.");
    expect(
      document.querySelector('button[aria-label="Change Line 1 quantity"]')
    ).not.toBeNull();
    // Readiness still blocks: the unreviewed difference has no recorded
    // choice, even though zero pane items are unresolved.
    expect(
      document.querySelector<HTMLButtonElement>('button[aria-label="Continue final save"]')?.disabled
    ).toBe(true);
  });

  it("an identity-only local session keeps the legacy choice workbench (N4/U2 guard)", async () => {
    reviewMock.mockResolvedValue({
      Source: {
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
        Lines: [lineSource],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem()] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [lineItem()],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    // Identity-only local shell: stored ID 0, no recorded items at all.
    // A never-reviewed draft keeps its first decision point.
    const transport = fakeTransport({
      ID: 0,
      PurchaseOrderID: 7,
      SourceQuotationID: 12,
      SourceRevision: 1,
      POVersion: 4,
      Active: true,
      ReadOnly: false,
      CreatedBy: 7,
      CreatedAt: "2026-10-28T00:00:00Z",
      UpdatedAt: "2026-10-28T00:00:05Z",
      LineIdentities: [{ Key: "client:a", ID: 101, Removed: false }],
      Items: [],
    });
    await renderEditor(
      transport,
      linkedOrder({ Items: [lineItem()] })
    );

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).toContain("Review differences before saving");
    // Legacy workbench preserved: the unreviewed difference still offers
    // its first decision point, with no invented receipt provenance.
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 quantity"]')
    ).not.toBeNull();
    expect(
      document.querySelector('button[aria-label="Use Quotation for Line 1 quantity"]')
    ).not.toBeNull();
  });

  it("a first save flips a local session to receipt provenance (N4/U2)", async () => {
    reviewMock.mockResolvedValue({
      Source: {
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
        Lines: [lineSource],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem()] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [lineItem()],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    // No stored session: the first commit builds an identity-only local
    // shell (ID 0), and Final Save persists it before the review loads.
    const transport = fakeTransport(null);
    await renderEditor(
      transport,
      linkedOrder({ Items: [lineItem()] })
    );

    // Commit the quantity difference, then resolve it in the pane so the
    // review has recorded provenance to render.
    const quantity = input("Line 1 quantity");
    typeInto(quantity, "3");
    await act(async () => {
      quantity.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();
    button("Keep PO entry for Line 1 quantity").click();
    await flush();
    button("Yes, keep PO entry for Line 1 quantity").click();
    await flush();

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    // The session save ran inside startReview (no manual Save Draft), the
    // stored answer carries a positive ID, and the review is a receipt.
    expect(transport.saves.length).toBeGreaterThan(0);
    expect(document.body.textContent).toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 quantity"]')
    ).toBeNull();
    expect(document.body.textContent).toContain("PO entry kept in the reconciliation pane.");
  });

  it("Add line from Quotation focuses the new description and announces once", async () => {
    reviewMock.mockResolvedValue({
      Source: {
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
        Lines: [
          lineSource,
          {
            QuotationItemNo: 2,
            QuotationSubNo: null,
            LineLabel: "2",
            Description: "Sprocket",
            UoM: "Pc",
            QuantityScaled: 10000,
            UnitPriceMinor: 5000,
            DiscountEnabled: false,
            DiscountPercent: 0,
          },
        ],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem()] }),
    });
    // Order claims only line 1, so Quotation 2 is missing with an Add action.
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [lineItem()] }));
    await flush();

    button("Add line from Quotation 2").click();
    await flush();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Line 2 description");
    expect(document.body.textContent).toContain("Line 2 added.");
  });

  it("the sheet trigger never claims Nothing to resolve without a live source", async () => {
    // No quotation linked: the pane says to link one, so the trigger must
    // not report a resolved count of zero as Nothing to resolve.
    const transport = fakeTransport(null);
    await renderEditor(transport, linkedOrder({ QuotationID: 0 }));
    await flush();

    const trigger = document.querySelector<HTMLButtonElement>(
      'button[aria-expanded="false"]'
    );
    if (trigger === null) throw new Error("no sheet trigger");
    expect(trigger.textContent?.includes("Nothing to resolve")).toBe(false);
    expect(trigger.getAttribute("aria-label")).toContain("no quotation linked");
    expect(document.body.textContent).toContain("Link a Quotation to start the comparison.");
  });

  it("the sheet trigger reports its loading state truthfully", async () => {
    // A load that never settles: the trigger must not claim resolution.
    const pending = new Promise<ReconciliationSession>(() => undefined);
    const loadingTransport: ReconciliationTransport = {
      load: () => pending,
      save: vi.fn(async (_poID: string, next: ReconciliationSession) => next),
    };
    await renderEditor(loadingTransport);
    await flush();
    const loadingTrigger = document.querySelector<HTMLButtonElement>(
      'button[aria-expanded="false"]'
    );
    if (loadingTrigger === null) throw new Error("no sheet trigger while loading");
    expect(loadingTrigger.textContent?.includes("Nothing to resolve")).toBe(false);
    expect(loadingTrigger.getAttribute("aria-label")).toContain("loading");
  });

  it("the sheet trigger reports a failed load as unavailable, never a zero count", async () => {
    // A failed load: unavailable, never a zero count.
    const failing: ReconciliationTransport = {
      load: async () => {
        throw new Error("boom");
      },
      save: vi.fn(async (_poID: string, next: ReconciliationSession) => next),
    };
    await renderEditor(failing);
    await flush();
    await flush();
    const errorTrigger = document.querySelector<HTMLButtonElement>(
      'button[aria-expanded="false"]'
    );
    if (errorTrigger === null) throw new Error("no sheet trigger on error");
    expect(errorTrigger.textContent?.includes("Nothing to resolve")).toBe(false);
    expect(errorTrigger.getAttribute("aria-label")).toContain("failed to load");
  });

  it("the sheet trigger shows the resolved count in read-only mode", async () => {
    const resolved = {
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      POValue: "50% advance",
      SourceValue: "30 days",
      Status: "resolved",
      Resolution: "kept_po",
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:05Z",
      History: [],
    };
    const transport = fakeTransport(savedSession([resolved, { ...resolved, Key: "header:incoterms", Field: "header:incoterms", Label: "Incoterms" }]));
    await renderEditor(transport, linkedOrder(), "correction");
    await flush();

    const trigger = document.querySelector<HTMLButtonElement>(
      'button[aria-expanded="false"]'
    );
    if (trigger === null) throw new Error("no sheet trigger in read-only mode");
    expect(trigger.textContent?.includes("Nothing to resolve")).toBe(false);
    expect(trigger.textContent).toContain("2 resolved");
  });

  it("Final Save persists the resolved session before opening the review", async () => {
    reviewMock.mockResolvedValue({
      Source: {
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
        Lines: [lineSource],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem()] }),
    });
    // Production-like update echo: the working copy round-trips with its
    // lines and the server version, exactly what Save Draft stores.
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [lineItem()],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const persisted = savedSession([
      {
        Key: "line:client:a:quantity",
        Field: "line:quantity",
        Kind: "difference",
        LineKey: "client:a",
        SourceLineRef: null,
        Label: "Line 1 quantity",
        POValue: 30000,
        SourceValue: 20000,
        Status: "unresolved",
        Resolution: null,
        ActorID: 7,
        UpdatedAt: "2026-10-28T00:00:05Z",
        History: [],
      },
    ]);
    const transport = fakeTransport(persisted);
    await renderEditor(transport, linkedOrder({ Items: [lineItem()] }));
    await flush();

    // Resolve the reopened persisted item in the pane.
    button("Keep PO entry for Line 1 quantity").click();
    await flush();
    button("Yes, keep PO entry for Line 1 quantity").click();
    await flush();
    expect(document.body.textContent).toContain("Kept PO");

    // No manual Save Draft: straight to Final Save.
    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).toContain("Review differences before saving");
    // The resolved decision reached the persisted session first: without
    // this write the backend gate refuses the finalize as unresolved.
    expect(transport.saves.length).toBeGreaterThan(0);
    const sent = transport.saves[transport.saves.length - 1];
    expect(
      sent.Items.find((entry) => entry.Key === "line:client:a:quantity")
    ).toMatchObject({ Status: "resolved", Resolution: "kept_po" });
    // The kept choice is retained into the receipt.
    expect(document.body.textContent).toContain("PO entry kept in the reconciliation pane.");
  });

  it("Save Draft then Final Save finalizes with the session-bumped row version", async () => {
    reviewMock.mockResolvedValue({
      Source: {
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
        Lines: [lineSource],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem()] }),
    });
    // Server-accurate versions: every draft write and every session save
    // bumps the row version, exactly like the real store (R-04).
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      const next = Number(fields.ExpectedVersion ?? 4) + 1;
      return {
        Order: linkedOrder({ Items: [lineItem()], Version: next }),
      };
    });
    finalizeMock.mockImplementation(async (input: unknown) => {
      const saved = linkedOrder({ Items: [lineItem()], Version: 99, State: "saved" });
      void input;
      return { Order: saved, Conflict: null };
    });
    const persisted = savedSession([
      {
        Key: "line:client:a:quantity",
        Field: "line:quantity",
        Kind: "difference",
        LineKey: "client:a",
        SourceLineRef: null,
        Label: "Line 1 quantity",
        POValue: 30000,
        SourceValue: 20000,
        Status: "unresolved",
        Resolution: null,
        ActorID: 7,
        UpdatedAt: "2026-10-28T00:00:05Z",
        History: [],
      },
    ]);
    const transport = fakeTransport(persisted);
    await renderEditor(transport, linkedOrder({ Items: [lineItem()] }));
    await flush();

    button("Keep PO entry for Line 1 quantity").click();
    await flush();
    button("Yes, keep PO entry for Line 1 quantity").click();
    await flush();

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).toContain("Review differences before saving");

    await act(async () => {
      button("Continue final save").click();
    });
    await flush();
    const confirmBox = document.querySelector<HTMLInputElement>(
      'input[aria-label="Confirm saving without the customer PDF"]'
    );
    if (confirmBox === null) throw new Error("no document confirmation");
    await act(async () => {
      confirmBox.click();
    });
    await flush();
    await act(async () => {
      button("Final save").click();
    });
    await flush();
    await flush();

    expect(finalizeMock).toHaveBeenCalledTimes(1);
    const payload = finalizeMock.mock.calls[0][0] as Record<string, unknown>;
    // Draft write 4->5, session save 5->6: the finalize must carry 6, not
    // the pre-save version 4 the editor loaded.
    expect(payload.ExpectedVersion).toBe(6);
  });

  it("a successful Final Save never writes the session again", async () => {
    reviewMock.mockResolvedValue({
      Source: {
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
        Lines: [lineSource],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem()] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [lineItem()],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    finalizeMock.mockImplementation(async () => {
      const saved = linkedOrder({ Items: [lineItem()], Version: 7, State: "saved" });
      return { Order: saved, Conflict: null };
    });
    const persisted = savedSession([
      {
        Key: "line:client:a:quantity",
        Field: "line:quantity",
        Kind: "difference",
        LineKey: "client:a",
        SourceLineRef: null,
        Label: "Line 1 quantity",
        POValue: 30000,
        SourceValue: 20000,
        Status: "unresolved",
        Resolution: null,
        ActorID: 7,
        UpdatedAt: "2026-10-28T00:00:05Z",
        History: [],
      },
    ]);
    const transport = fakeTransport(persisted);
    await renderEditor(transport, linkedOrder({ Items: [lineItem()] }));
    await flush();

    button("Keep PO entry for Line 1 quantity").click();
    await flush();
    button("Yes, keep PO entry for Line 1 quantity").click();
    await flush();

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    const savesAtReview = transport.saves.length;
    expect(savesAtReview).toBeGreaterThan(0);

    await act(async () => {
      button("Continue final save").click();
    });
    await flush();
    const confirmBox = document.querySelector<HTMLInputElement>(
      'input[aria-label="Confirm saving without the customer PDF"]'
    );
    if (confirmBox === null) throw new Error("no document confirmation");
    await act(async () => {
      confirmBox.click();
    });
    await flush();
    await act(async () => {
      button("Final save").click();
    });
    await flush();
    await flush();

    expect(finalizeMock).toHaveBeenCalledTimes(1);
    // The saved record is read-only: no session write may follow success,
    // and no false sync banner may appear.
    expect(transport.saves.length).toBe(savesAtReview);
    expect(document.body.textContent?.includes("could not be saved")).toBe(false);
  });
});
