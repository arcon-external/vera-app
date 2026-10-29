// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderSourceDTO } from "../poFinalize";
import type { PurchaseOrderDTO } from "../purchaseOrder";
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
