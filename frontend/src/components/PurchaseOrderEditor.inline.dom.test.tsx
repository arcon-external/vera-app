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

// The inline quotation model: blue/green header fields, quotation ghosts built
// from the real item row, and a final save that never opens a difference
// review. Only the desktop bindings and the reconciliation transport are fake.

const { reviewMock, updateMock, finalizeMock, documentStateMock } = vi.hoisted(() => ({
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

const UOMS = [
  { value: "Pc", label: "Pc" },
  { value: "Box", label: "Box" },
];
const INCOTERMS = [
  { value: "EXW", label: "EXW" },
  { value: "CIF", label: "CIF" },
];

function sourceLine(
  overrides: Partial<PurchaseOrderSourceLineDTO> = {}
): PurchaseOrderSourceLineDTO {
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
    UUID: "uuid-inline",
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

function reviewSource(lines: PurchaseOrderSourceLineDTO[], paymentTerms = "45 days") {
  return {
    QuotationID: 12,
    RevisionNumber: 1,
    QuoteNumber: "1502260052",
    CustomerID: 2,
    CustomerName: "PT. Buyer",
    Currency: "IDR",
    PaymentTerms: paymentTerms,
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
  const transport: ReconciliationTransport & {
    stored: ReconciliationSession | null;
    saves: ReconciliationSession[];
  } = {
    stored: initial,
    saves: [],
    load: vi.fn(async () => {
      if (transport.stored === null) throw new Error("no reconciliation recorded");
      return transport.stored;
    }),
    save: vi.fn(async (_poID: string, session: ReconciliationSession) => {
      transport.saves.push(session);
      transport.stored = {
        ...session,
        ID: 3,
        POVersion: session.POVersion + 1,
        UpdatedAt: "2026-10-28T00:00:02Z",
      };
      return transport.stored;
    }),
  };
  return transport;
}

let container: HTMLDivElement;
let root: Root;
let shellActions: POEditorShellActions | null;

beforeEach(() => {
  Element.prototype.scrollIntoView = () => undefined;
  (Element.prototype as unknown as { scrollTo?: () => void }).scrollTo = () => undefined;
  reviewMock.mockReset();
  updateMock.mockReset();
  finalizeMock.mockReset();
  documentStateMock.mockReset();
  documentStateMock.mockResolvedValue({ Document: null, Versions: [] });
  updateMock.mockImplementation(async (input: unknown) => {
    const fields = input as Record<string, unknown>;
    const submitted = Array.isArray(fields.Items)
      ? (fields.Items as Record<string, unknown>[]).map((item, index) =>
          dtoItem({
            ID: 101 + index,
            LineNumber: index + 1,
            Description: String(item.Description ?? ""),
            QuantityScaled: Number(item.QuantityScaled ?? 0),
            UoM: String(item.UoM ?? ""),
            UnitPriceMinor: Number(item.UnitPriceMinor ?? 0),
            DiscountEnabled: Boolean(item.DiscountEnabled),
            DiscountPercent: Number(item.DiscountPercent ?? 0),
            QuotationItemNo:
              (item.QuotationItemNo as number | null | undefined) ?? null,
            QuotationSubNo:
              (item.QuotationSubNo as number | null | undefined) ?? null,
          })
        )
      : [dtoItem()];
    return {
      Order: linkedOrder({
        PaymentTerms: String(fields.PaymentTerms ?? "45 days"),
        ConfirmationRequested: Boolean(fields.ConfirmationRequested),
        Version: Number(fields.ExpectedVersion ?? 4),
        Items: submitted,
      }),
    };
  });
  finalizeMock.mockImplementation(async (input: unknown) => {
    const fields = input as Record<string, unknown>;
    return {
      Order: linkedOrder({
        State: "saved",
        PaymentTerms: String(fields.PaymentTerms ?? "45 days"),
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

function textButton(label: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (entry) => entry.textContent?.trim() === label
  );
  if (found === undefined) throw new Error(`no button saying ${label}`);
  return found;
}

function review(overrides: Record<string, unknown> = {}) {
  return {
    Order: linkedOrder(),
    Source: reviewSource([sourceLine()]),
    DocumentState: "none",
    ...overrides,
  };
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

describe("inline quotation values", () => {
  it("shows a blue field, then turns green when the quotation value is used", async () => {
    reviewMock.mockResolvedValue(
      review({ Source: reviewSource([sourceLine()], "30 days") })
    );
    await renderEditor(fakeTransport(null));

    const terms = input("Payment terms");
    expect(terms.value).toBe("45 days");
    expect(terms.style.borderColor).toBe("rgb(59, 130, 246)");

    act(() => terms.focus());
    expect(document.body.textContent).toContain("Value from quotation");
    expect(document.body.textContent).toContain("30 days");
    act(() => button("Use quotation value").click());
    await flush();

    expect(input("Payment terms").value).toBe("30 days");
    expect(input("Payment terms").style.borderColor).toBe("rgb(82, 152, 115)");
    expect(document.body.textContent).toContain("Using quotation value");
  });

  it("renders a quotation ghost under its matching line and accepts the whole item", async () => {
    reviewMock.mockResolvedValue(
      review({
        Source: reviewSource([
          sourceLine({ UnitPriceMinor: 12000, Description: "Widget" }),
        ]),
      })
    );
    await renderEditor(fakeTransport(null));

    expect(document.body.textContent).toContain("Quotation line 1");
    expect(document.body.textContent).toContain("Using this replaces them");

    button("Use quotation 1").click();
    await flush();

    expect(document.body.textContent).not.toContain("Quotation line 1");
    expect(input("Unit price for line 1").value).toBe("Rp12.000");
  });

  it("lists every quotation item for an empty order and adds them all", async () => {
    reviewMock.mockResolvedValue(
      review({
        Source: reviewSource([
          sourceLine({ QuotationItemNo: 1, LineLabel: "1", Description: "Widget" }),
          sourceLine({
            QuotationItemNo: 2,
            LineLabel: "2",
            Description: "Cable",
            UnitPriceMinor: 5000,
          }),
        ]),
      })
    );
    await renderEditor(fakeTransport(null), linkedOrder({ Items: [] }));

    expect(document.body.textContent).toContain("Quotation items · 2");
    expect(document.body.textContent).toContain("Not on this order yet");

    textButton("Use all").click();
    await flush();

    expect(document.body.textContent).toContain("2 lines");
    expect(document.body.textContent).not.toContain("Not on this order yet");
  });

  it("records an ignored ghost as an omission instead of blocking the shell", async () => {
    reviewMock.mockResolvedValue(
      review({
        Source: reviewSource([
          sourceLine(),
          sourceLine({
            QuotationItemNo: 2,
            LineLabel: "2",
            Description: "Cable",
            UnitPriceMinor: 5000,
          }),
        ]),
      })
    );
    await renderEditor(fakeTransport(null));

    if (shellActions === null) throw new Error("no shell actions reported");
    expect(shellActions.unresolvedCount).toBe(1);
    expect(document.body.textContent).toContain("Quotation line 2");

    button("Ignore quotation 2").click();
    await flush();

    expect(document.body.textContent).not.toContain("Quotation line 2");
    if (shellActions === null) throw new Error("no shell actions after ignore");
    expect(shellActions.unresolvedCount).toBe(0);
  });
});

describe("inline final save", () => {
  it("opens the confirmation directly and submits without any review step", async () => {
    reviewMock.mockResolvedValue(review());
    await renderEditor(fakeTransport(null));

    if (shellActions === null) throw new Error("no shell actions reported");
    act(() => shellActions?.onFinalSave());
    await flush();
    await flush();

    expect(document.body.textContent).toContain("Final save");
    expect(document.body.textContent).not.toContain("Difference review");
    expect(document.body.textContent).toContain(
      "I confirm this purchase order is saved without its customer PO PDF"
    );
  });

  it("persists the kept session, then finalizes with the session-bumped version", async () => {
    reviewMock.mockResolvedValue(review());
    const transport = fakeTransport(null);
    await renderEditor(transport);

    const quantity = input("Quantity for line 1");
    act(() => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      setter?.call(quantity, "3");
      quantity.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      quantity.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    });
    await flush();

    if (shellActions === null) throw new Error("no shell actions reported");
    await act(async () => {
      shellActions?.onFinalSave();
    });
    await flush();
    await flush();

    expect(document.body.textContent).toContain("Final save");
    expect(transport.saves).toHaveLength(1);
    const session = transport.saves[0];
    const kept = session.Items.find(
      (item) =>
        item.Status === "resolved" &&
        item.Resolution === "kept_po" &&
        item.Key.includes("quantity")
    );
    expect(kept).toBeDefined();

    const confirm = document.querySelector<HTMLInputElement>(
      'input[aria-label="Confirm saving without the customer PDF"]'
    );
    if (confirm === null) throw new Error("no PDF confirmation");
    act(() => confirm.click());
    await flush();
    act(() => textButton("Final save").click());
    await flush();
    await flush();

    expect(finalizeMock).toHaveBeenCalledTimes(1);
    const sent = finalizeMock.mock.calls[0]?.[0] as {
      ExpectedVersion?: number;
      Decisions?: { Key: string; Choice: string }[];
      Items?: { ID?: number }[];
    };
    expect(sent.ExpectedVersion).toBe(5);
    expect(sent.Decisions).toEqual([
      { Key: "line:1:quantity", Choice: "po" },
    ]);
    // The confirmation is built from the saved state, so the finalize payload
    // names the persisted row rather than a pre-save copy with no identity.
    expect(sent.Items?.[0]?.ID).toBe(101);
    // The saved record is read-only: the session is never written after it.
    expect(transport.saves).toHaveLength(1);
  });

  it("blocks a duplicate quotation match with a named line", async () => {
    reviewMock.mockResolvedValue(review());
    await renderEditor(
      fakeTransport(null),
      linkedOrder({
        Items: [dtoItem({ LineNumber: 1 }), dtoItem({ LineNumber: 2 })],
      })
    );

    if (shellActions === null) throw new Error("no shell actions reported");
    act(() => shellActions?.onFinalSave());
    await flush();

    expect(document.body.textContent).toContain("Duplicate match · Quotation 1");
  });
});
