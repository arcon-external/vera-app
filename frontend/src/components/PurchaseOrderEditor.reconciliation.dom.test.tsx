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
  createMock,
  previewMock,
  linkMock,
} = vi.hoisted(() => ({
  reviewMock: vi.fn(),
  updateMock: vi.fn(),
  finalizeMock: vi.fn(),
  documentStateMock: vi.fn(),
  createMock: vi.fn(),
  previewMock: vi.fn(),
  linkMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  CorrectPurchaseOrder: vi.fn(),
  CreatePurchaseOrderBlankDraft: (...args: unknown[]) => createMock(...args),
  FinalizePurchaseOrder: (...args: unknown[]) => finalizeMock(...args),
  GetActivePurchaseOrderDocumentContent: vi.fn(),
  GetPurchaseOrder: vi.fn(),
  GetPurchaseOrderDocumentState: (...args: unknown[]) => documentStateMock(...args),
  GetPurchaseOrderDocumentVersionContent: vi.fn(),
  GetPurchaseOrderDocumentVersion: vi.fn(),
  GetPurchaseOrderHistory: vi.fn(async () => []),
  GetPurchaseOrderReview: (...args: unknown[]) => reviewMock(...args),
  LinkPurchaseOrderDraftSource: (...args: unknown[]) => linkMock(...args),
  PreviewPurchaseOrderDraftLink: (...args: unknown[]) => previewMock(...args),
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
      PurchaseOrderDraftLinkInput: Input,
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

  it("Final Save after a save pre-saves at the version the save produced (#584)", async () => {
    const line = {
      QuotationItemNo: null,
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
    };
    // The server advances the row on every successful draft write, so the
    // retained version must follow it; otherwise the Final Save pre-save names
    // the version the editor opened with and the server refuses the write.
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          PaymentTerms: String(fields.PaymentTerms ?? "45 days"),
          Incoterms: String(fields.Incoterms ?? "EXW"),
          Version: Number(fields.ExpectedVersion ?? 4) + 1,
          Items: [line],
          QuotationID: 0,
          SourceRevision: 0,
          QuoteNumber: "",
        }),
      };
    });
    // Source-less: with no live session the save's session sync cannot adopt a
    // version of its own, so the retained draft version is the one under test.
    const orderWithLine = linkedOrder({
      QuotationID: 0,
      SourceRevision: 0,
      QuoteNumber: "",
      Items: [line],
    });
    const transport = fakeTransport(null);
    await renderEditor(transport, orderWithLine);

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onSave();
    });
    await flush();
    expect(updateMock).toHaveBeenCalledTimes(1);
    const saved = updateMock.mock.calls[0][0] as { ExpectedVersion: number };
    expect(saved.ExpectedVersion).toBe(4);

    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(updateMock).toHaveBeenCalledTimes(2);
    const preSave = updateMock.mock.calls[1][0] as { ExpectedVersion: number };
    expect(preSave.ExpectedVersion).toBe(5);
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

    // Invalidate the line fields. Both must be named; nothing is silently
    // sliced after the first.
    typeInto(input("Quantity for line 1"), "");
    await act(async () => {
      input("Quantity for line 1").dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    typeInto(input("Unit price for line 1"), "abc");
    await act(async () => {
      input("Unit price for line 1").dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();

    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.unresolvedCount).toBeGreaterThan(1);
    for (const label of ["Line 1 quantity", "Line 1 unit price"]) {
      expect(shellActions.blockedMessage).toContain(label);
    }

    await act(async () => {
      shellActions?.onBlockedActivate();
    });
    await flush();
    // The first invalid field is focused so its error becomes visible.
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Quantity for line 1");
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
    const field = input("Quantity for line 1");
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
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Quantity for line 1");
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

  it("an unreviewed mixed difference blocks entry and focuses the live field (§5.5)", async () => {
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
    // §5.5: the recorded header choice does not excuse the unreviewed line
    // difference. Entry blocks before any review opens and focuses the live
    // quantity field. (Read-only receipt rendering for the mixed case stays
    // covered at the receipt component seam.)
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 quantity"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Use Quotation for Line 1 quantity"]')
    ).toBeNull();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Quantity for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 quantity");
  });

  it("a persisted tax-only session with an unreviewed line difference blocks entry and focuses the live difference before review (entry readiness)", async () => {
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
    // §5.5 entry readiness: the review must NOT open for an unreviewed
    // persisted difference. Entry names the blocker and focuses the live
    // quantity field so it can be committed and resolved in the pane.
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 quantity"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Use Quotation for Line 1 quantity"]')
    ).toBeNull();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Quantity for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 quantity");
  });

  it("an unreviewed enabled-zero row discount blocks entry and focuses its usable percent control", async () => {
    // Root discount counterexample: quantity matches (2 vs 2); the ONLY
    // difference is PO row discount enabled 0% vs source disabled 0%.
    // The comparison label ("Line 1 discount") names neither live control;
    // the usable target is the enabled percent input, whose blur commits
    // the value unchanged for Keep/Use resolution in the pane.
    const discounted = lineItem({ QuantityScaled: 20000, DiscountEnabled: true, DiscountPercent: 0 });
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
      Order: linkedOrder({ Items: [discounted] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [discounted],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
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
      linkedOrder({ Items: [discounted] })
    );

    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.unresolvedCount).toBe(0);
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 discount"]')
    ).toBeNull();
    // A usable live target receives focus: the enabled percent input keeps
    // the entered 0 reviewable by blur-commit without changing any value.
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Discount percentage for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 discount");
  });

  it("a disabled-zero row discount commits through the switch and Keeps through retry", async () => {
    // The discount switch commits its new state explicitly. Toggling on and
    // back off records the shown disabled 0% for Keep resolution without
    // leaving the row enabled.
    const plain = lineItem({ QuantityScaled: 20000 });
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
        Lines: [{ ...lineSource, DiscountEnabled: true, DiscountPercent: 0 }],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [plain] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [plain],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [plain] }));

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Enable a discount for line 1");
    const toggle = button("Enable a discount for line 1");
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(toggle.hasAttribute("disabled")).toBe(false);
    expect(input("Discount percentage for line 1").disabled).toBe(true);

    // The switch commits explicitly; on then back off records the disabled 0.
    await act(async () => {
      toggle.click();
    });
    await flush();
    expect(button("Enable a discount for line 1").getAttribute("aria-checked")).toBe("true");
    await act(async () => {
      button("Enable a discount for line 1").click();
    });
    await flush();
    expect(button("Enable a discount for line 1").getAttribute("aria-checked")).toBe("false");
    button("Keep PO entry for Line 1 discount").click();
    await flush();
    button("Yes, keep PO entry for Line 1 discount").click();
    await flush();
    expect(button("Enable a discount for line 1").getAttribute("aria-checked")).toBe("false");
    expect(input("Discount percentage for line 1").value).toBe("0");

    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 discount"]')
    ).toBeNull();
    expect(document.body.textContent).toContain("PO entry kept in the reconciliation pane.");
  });

  it("toggling the discount switch on commits once and does not duplicate or reopen anything", async () => {
    // The switch click commits the toggled value; a following focus event
    // must be a no-op: no duplicate item, no reopened choice, no history.
    const plain = lineItem({ QuantityScaled: 20000 });
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
        Lines: [{ ...lineSource, DiscountEnabled: true, DiscountPercent: 0 }],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [plain] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [plain],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [plain] }));

    const toggle = button("Enable a discount for line 1");
    await act(async () => {
      toggle.click();
    });
    await flush();
    expect(button("Enable a discount for line 1").getAttribute("aria-checked")).toBe("true");
    expect(document.body.textContent).toContain("Line 1 discount: matches the Quotation");
    // A later focus event with the toggled value still current: still
    // matched, still no item.
    await act(async () => {
      button("Enable a discount for line 1").dispatchEvent(
        new FocusEvent("focusout", { bubbles: true })
      );
    });
    await flush();
    expect(button("Enable a discount for line 1").getAttribute("aria-checked")).toBe("true");
    expect(document.body.textContent).toContain("Line 1 discount: matches the Quotation");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Line 1 discount"]')
    ).toBeNull();
  });

  it("a kept discount is not reopened by a later switch focus event", async () => {
    const plain = lineItem({ QuantityScaled: 20000 });
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
        Lines: [{ ...lineSource, DiscountEnabled: true, DiscountPercent: 0 }],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [plain] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [plain],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [plain] }));

    // Toggle on then off: the switch commits the disabled 0, exposing Keep.
    await act(async () => {
      button("Enable a discount for line 1").click();
    });
    await flush();
    await act(async () => {
      button("Enable a discount for line 1").click();
    });
    await flush();
    button("Keep PO entry for Line 1 discount").click();
    await flush();
    button("Yes, keep PO entry for Line 1 discount").click();
    await flush();
    expect(document.body.textContent).toContain("Kept PO");

    // A later focus event on the kept switch: the resolved choice stays,
    // history untouched.
    await act(async () => {
      button("Enable a discount for line 1").dispatchEvent(
        new FocusEvent("focusout", { bubbles: true })
      );
    });
    await flush();
    expect(document.body.textContent).toContain("Kept PO");
    expect(
      document.querySelector('button[aria-label="Keep PO entry for Line 1 discount"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Yes, keep PO entry for Line 1 discount"]')
    ).toBeNull();
  });

  it("a disabled-zero row discount against an enabled source focuses the switch", async () => {
    // Reverse direction: PO disabled 0% vs source enabled 10%. The percent
    // input is disabled, so the switch is the usable target; toggling it on
    // commits for Keep/Use review.
    const plain = lineItem({ QuantityScaled: 20000 });
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
        Lines: [{ ...lineSource, DiscountEnabled: true, DiscountPercent: 10 }],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [plain] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [plain],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [plain] }));

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Enable a discount for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 discount");
  });

  it("a both-enabled percent difference focuses the percent control", async () => {
    const marked = lineItem({ QuantityScaled: 20000, DiscountEnabled: true, DiscountPercent: 15 });
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
        Lines: [{ ...lineSource, DiscountEnabled: true, DiscountPercent: 10 }],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [marked] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [marked],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [marked] }));

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Discount percentage for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 discount");
  });

  it("a blocked discount difference commits, keeps and retries into the receipt", async () => {
    const discounted = lineItem({ QuantityScaled: 20000, DiscountEnabled: true, DiscountPercent: 0 });
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
      Order: linkedOrder({ Items: [discounted] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [discounted],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [discounted] }));

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Discount percentage for line 1");

    // Blur commits the shown 0 unchanged, Keep preserves enabled 0, and the
    // retry opens the read-only receipt with kept provenance.
    const percent = input("Discount percentage for line 1");
    await act(async () => {
      percent.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();
    button("Keep PO entry for Line 1 discount").click();
    await flush();
    button("Yes, keep PO entry for Line 1 discount").click();
    await flush();
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 discount"]')
    ).toBeNull();
    expect(document.body.textContent).toContain("PO entry kept in the reconciliation pane.");
  });

  it("an unreviewed description blocks entry and focuses its field", async () => {
    const renamed = lineItem({ QuantityScaled: 20000, Description: "WidgetX" });
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
      Order: linkedOrder({ Items: [renamed] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [renamed],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [renamed] }));

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Description for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 description");
  });

  it("an unreviewed unit of measure blocks entry and focuses its select", async () => {
    const reboxed = lineItem({ QuantityScaled: 20000, UoM: "Box" });
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
      Order: linkedOrder({ Items: [reboxed] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [reboxed],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [reboxed] }));

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Unit of measure for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 unit of measure");
  });

  it("an unreviewed unit price blocks entry and focuses its field", async () => {
    const repriced = lineItem({ QuantityScaled: 20000, UnitPriceMinor: 20000 });
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
      Order: linkedOrder({ Items: [repriced] }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [repriced],
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(transport, linkedOrder({ Items: [repriced] }));

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Unit price for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 unit price");
  });

  it("an unreviewed incoterms value blocks entry and focuses its select", async () => {
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
        Lines: [{ ...lineSource, QuantityScaled: 20000 }],
      },
      DocumentState: "none",
      Order: linkedOrder({ Items: [lineItem({ QuantityScaled: 20000 })], Incoterms: "CIF" }),
    });
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [lineItem({ QuantityScaled: 20000 })],
          Incoterms: String(fields.Incoterms ?? "EXW"),
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    const transport = fakeTransport(savedSession([]));
    await renderEditor(
      transport,
      linkedOrder({ Items: [lineItem({ QuantityScaled: 20000 })], Incoterms: "CIF" })
    );

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Incoterms");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Incoterms");
  });

  it("a persisted empty session with an unreviewed line difference blocks entry and focuses the live field", async () => {    reviewMock.mockResolvedValue({
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
    // A stored session (ID 3) with no items at all: still recorded
    // provenance, so the unreviewed quantity blocks entry.
    const transport = fakeTransport(savedSession([]));
    await renderEditor(
      transport,
      linkedOrder({ Items: [lineItem()] })
    );

    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.unresolvedCount).toBe(0);
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 quantity"]')
    ).toBeNull();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Quantity for line 1");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Line 1 quantity");
  });

  it("a first save of recorded content still blocks on a further uncommitted difference", async () => {
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
      Order: linkedOrder({ Items: [lineItem()], PaymentTerms: "50% advance" }),
    });
    // The draft write echoes the working copy including its header, so the
    // uncommitted payment-terms divergence survives the persist.
    updateMock.mockImplementation(async (input: unknown) => {
      const fields = input as Record<string, unknown>;
      return {
        Order: linkedOrder({
          Items: [lineItem()],
          PaymentTerms: String(fields.PaymentTerms ?? "45 days"),
          Version: Number(fields.ExpectedVersion ?? 4),
        }),
      };
    });
    // No stored session: the first commit builds the local shell.
    const transport = fakeTransport(null);
    await renderEditor(
      transport,
      linkedOrder({ Items: [lineItem()], PaymentTerms: "50% advance" })
    );

    // Commit and resolve the quantity in the pane; the header divergence
    // stays uncommitted with no pane item.
    const quantity = input("Quantity for line 1");
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
    expect(shellActions.unresolvedCount).toBe(0);
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    // The first save persists the recorded line choice (authoritative), but
    // the uncommitted header difference still blocks entry: no review, and
    // the live payment-terms field is focused.
    expect(transport.saves.length).toBeGreaterThan(0);
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Payment terms");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Payment terms");
  });

  it("blur, resolve and retry after an entry block opens the receipt (N4/U2 cycle)", async () => {
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
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    // Entry blocks first: no review, live quantity focused.
    expect(document.body.textContent).not.toContain("Review differences before saving");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Quantity for line 1");

    // Commit on blur, Keep in the pane, then retry: the recorded choice
    // opens the read-only receipt.
    const quantity = input("Quantity for line 1");
    typeInto(quantity, "3");
    await act(async () => {
      quantity.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();
    button("Keep PO entry for Line 1 quantity").click();
    await flush();
    button("Yes, keep PO entry for Line 1 quantity").click();
    await flush();
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();
    expect(document.body.textContent).toContain("Review differences before saving");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 quantity"]')
    ).toBeNull();
    expect(document.body.textContent).toContain("PO entry kept in the reconciliation pane.");
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
    const quantity = input("Quantity for line 1");
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
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Description for line 2");
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

  it("a linked session resolves a review-time price difference through the pane (#586/#587)", async () => {
    const line: PurchaseOrderDTOItem = {
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
    };
    const order = linkedOrder({
      SourceState: "former",
      SourceMissing: true,
      QuotationID: 0,
      QuoteNumber: "1502260099",
      SourceRevision: 1,
      PaymentTerms: "45 days",
      Items: [line],
    });
    const source: PurchaseOrderSourceDTO = {
      ...SOURCE,
      PaymentTerms: "45 days",
      Incoterms: "EXW",
      Lines: [
        {
          QuotationItemNo: 1,
          QuotationSubNo: null,
          LineLabel: "1",
          Description: "Widget",
          UoM: "Pc",
          QuantityScaled: 20000,
          UnitPriceMinor: 12000,
          DiscountEnabled: false,
          DiscountPercent: 0,
        },
      ],
    };
    let latestItems: PurchaseOrderDTOItem[] = [line];
    const itemsFrom = (input: Record<string, unknown>): PurchaseOrderDTOItem[] =>
      (input.Items as Array<Record<string, unknown>>).map((item, index) => ({
        ID: item.ID === null || item.ID === undefined ? undefined : Number(item.ID),
        ClientKey: String(item.ClientKey ?? ""),
        QuotationItemNo:
          item.QuotationItemNo === null || item.QuotationItemNo === undefined
            ? null
            : Number(item.QuotationItemNo),
        QuotationSubNo:
          item.QuotationSubNo === null || item.QuotationSubNo === undefined
            ? null
            : Number(item.QuotationSubNo),
        LineNumber: index + 1,
        Description: String(item.Description ?? ""),
        QuantityScaled: Number(item.QuantityScaled ?? 0),
        UoM: String(item.UoM ?? ""),
        UnitPriceMinor: Number(item.UnitPriceMinor ?? 0),
        DiscountEnabled: Boolean(item.DiscountEnabled),
        DiscountPercent: Number(item.DiscountPercent ?? 0),
        GrossMinor: 0,
        DiscountMinor: 0,
        NetMinor: 0,
      }));
    updateMock.mockImplementation(async (input: Record<string, unknown>) => {
      latestItems = itemsFrom(input);
      return {
        Order: linkedOrder({
          ...order,
          Items: latestItems,
          PaymentTerms: String(input.PaymentTerms ?? order.PaymentTerms),
          Version: Number(input.ExpectedVersion ?? 4),
        }),
      };
    });
    previewMock.mockResolvedValue({
      Order: order,
      Source: source,
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
    linkMock.mockImplementation(async (input: Record<string, unknown>) => {
      const linked = linkedOrder({
        ...order,
        Items: latestItems,
        Version: Number(input.ExpectedVersion ?? 4) + 1,
        SourceState: "live",
        SourceMissing: false,
        PrintedTotalsKnown: true,
        QuotationID: 12,
        SourceRevision: 1,
        QuoteNumber: "1502260052",
      });
      return { Order: linked };
    });
    const transport = fakeTransport(null);
    await act(async () => {
      root.render(
        <PurchaseOrderEditorPage
          order={order}
          currency={IDR}
          uomOptions={UOMS}
          incotermOptions={INCOTERMS}
          pendingLinkTarget={{ quotationId: 12, revisionNumber: 1 }}
          onLinkQuotation={() => undefined}
          onSaved={() => undefined}
          onCancel={() => undefined}
          reconciliationTransport={transport}
          actorId={7}
          actorName="Rina"
          mode="draft"
        />
      );
    });
    await flush();
    await flush();

    // The review matched the draft's line to quotation line 1 and kept the PO
    // price, so linking succeeds with an unreviewed price difference.
    expect(document.body.textContent).toContain("Link this draft to a quotation");
    act(() => button("Use PO entry for Line 1 unit price").click());
    await flush();
    act(() => button("Link source").click());
    await flush();
    await flush();
    await flush();
    expect(linkMock).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toContain("Link this draft to a quotation");

    // The link's source is adopted: leaving the unit-price field now commits a
    // difference and creates the pane item the old flow could not build.
    const price = input("Unit price for line 1");
    act(() => {
      price.focus();
      price.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();
    await flush();

    expect(
      document.querySelector('button[aria-label="Keep PO entry for Line 1 unit price"]')
    ).not.toBeNull();
    expect(
      document.querySelector(
        'button[aria-label="Use Quotation value for Line 1 unit price"]'
      )
    ).not.toBeNull();

    // Resolving that item clears the gate: Final Save reaches the review
    // instead of dead-ending on a difference the pane could not represent.
    act(() => button("Keep PO entry for Line 1 unit price").click());
    await flush();
    act(() => button("Yes, keep PO entry for Line 1 unit price").click());
    await flush();
    reviewMock.mockImplementation(async () => ({
      Order: linkedOrder({ ...order, Items: latestItems, SourceState: "live" }),
      Source: source,
      DocumentState: "none",
    }));
    act(() => button("Final Save").click());
    await flush();
    await flush();

    expect(document.body.textContent).toContain("Review differences before saving");
  });
});
