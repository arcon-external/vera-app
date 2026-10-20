// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import { PurchaseOrderEditorPage } from "./PurchaseOrderEditor";

// Issue #473: the correction editor and its version warning in a real DOM.
// These are the interactions the SSR suite cannot reach: the required reason,
// Cancel keeping the unsaved working copy, Overwrite rechecking the version and
// refreshing when a second save intervened, and the absence of every protected
// identifier from the correction payload.
//
// The bindings are replaced at their module boundary, which is what the desktop
// provides; the editor, the dialog and the reducer are real.

const { correctMock, updateMock, documentStateMock } = vi.hoisted(() => ({
  correctMock: vi.fn(),
  updateMock: vi.fn(),
  documentStateMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  CorrectPurchaseOrder: (...args: unknown[]) => correctMock(...args),
  UpdatePurchaseOrderDraft: (...args: unknown[]) => updateMock(...args),
  FinalizePurchaseOrder: vi.fn(),
  GetPurchaseOrderReview: vi.fn(),
  GetPurchaseOrderHistory: vi.fn(async () => []),
  AssignPurchaseOrderConfirmationNumber: vi.fn(),
  GetPurchaseOrderDocumentState: (...args: unknown[]) => documentStateMock(...args),
  AttachPurchaseOrderDocument: vi.fn(),
  GetActivePurchaseOrderDocumentContent: vi.fn(),
  GetPurchaseOrderDocumentVersionContent: vi.fn(),
  RemovePurchaseOrderDocument: vi.fn(),
}));

vi.mock("@backend/models", () => ({
  backend: {
    PurchaseOrderCorrectionInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderUpdateInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderItemInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderFinalizeInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
  },
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function order(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    PODate: "2026-10-01",
    RequestedDeliveryDate: null,
    UUID: "uuid-473",
    State: "saved",
    QuoteNumber: "1502260012",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing",
    DeliveryAddress: "Jl. Old Delivery",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "+62 21 555",
    PONumber: "PO-473",
    ReceivedDate: "2026-10-02",
    Version: 4,
    Incoterms: "CIF",
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-02T10:00:00Z",
    UpdatedAt: "2026-10-02T10:00:00Z",
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
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    CustomerID: 2,
    PaymentTerms: "30 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 20000,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 20000,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    ConfirmationNumber: "2302260001",
    ConfirmationWarn: false,
    ...overrides,
  };
}

const CONFLICT_BUDI = {
  CurrentState: "saved",
  CurrentVersion: 5,
  UpdatedBy: 9,
  UpdatedByName: "Budi",
  UpdatedAt: "2026-10-21T10:15:00+07:00",
};
const CONFLICT_DEWI = {
  CurrentState: "saved",
  CurrentVersion: 6,
  UpdatedBy: 12,
  UpdatedByName: "Dewi",
  UpdatedAt: "2026-10-21T10:31:00+07:00",
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  correctMock.mockReset();
  updateMock.mockReset();
  documentStateMock.mockReset();
  documentStateMock.mockResolvedValue({ Document: null, Versions: [] });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (!found) throw new Error(`button ${label} not found`);
  return found;
}

function queryButton(label: string): HTMLButtonElement | null {
  return document.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
}

function typeInto(label: string, value: string) {
  const found = document.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    `input[aria-label="${label}"], textarea[aria-label="${label}"]`
  );
  if (!found) throw new Error(`field ${label} not found`);
  const prototype =
    found instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(found, value);
  found.dispatchEvent(new Event("input", { bubbles: true }));
}

function render(mode: "draft" | "correction", onSaved = vi.fn(), state?: PurchaseOrderDTO) {
  act(() =>
    root.render(
      <PurchaseOrderEditorPage
        order={state ?? order()}
        currency={USD}
        uomOptions={[{ value: "Pc", label: "Pc" }]}
        incotermOptions={[{ value: "CIF", label: "CIF" }]}
        onSaved={onSaved}
        onCancel={() => undefined}
        mode={mode}
      />
    )
  );
  return onSaved;
}

describe("correction editor (issue #473)", () => {
  it("requires a reason before any correction call", async () => {
    const onSaved = render("correction");
    await act(async () => {
      button("Save correction").click();
    });

    expect(correctMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("A correction needs a reason.");
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("Cancel keeps the unsaved working copy and never applies the edit", async () => {
    correctMock.mockResolvedValue({ Order: null, Conflict: CONFLICT_BUDI });
    const onSaved = render("correction");

    typeInto("Correction reason", "Customer moved warehouse");
    typeInto("Delivery address", "Jl. New Delivery 9");
    await act(async () => {
      button("Save correction").click();
    });

    expect(document.body.textContent).toContain("This purchase order changed");
    expect(document.body.textContent).toContain("Budi");
    expect(document.body.textContent).toContain("version 5");

    await act(async () => {
      button("Cancel and keep editing").click();
    });
    expect(document.body.textContent).not.toContain("This purchase order changed");
    expect(
      document.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="Delivery address"]'
      )?.value
    ).toBe("Jl. New Delivery 9");
    expect(
      document.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="Correction reason"]'
      )?.value
    ).toBe("Customer moved warehouse");
    expect(onSaved).not.toHaveBeenCalled();
    expect(correctMock).toHaveBeenCalledTimes(1);
    const first = correctMock.mock.calls[0][0] as Record<string, unknown>;
    expect(first.ExpectedVersion).toBe(4);
    expect(first.Overwrite).toBe(false);
    expect(first.Reason).toBe("Customer moved warehouse");
  });

  it("refreshes the warning when another save intervenes before Overwrite", async () => {
    correctMock
      .mockResolvedValueOnce({ Order: null, Conflict: CONFLICT_BUDI })
      .mockResolvedValueOnce({ Order: null, Conflict: CONFLICT_DEWI })
      .mockResolvedValueOnce({
        Order: order({ Version: 7, DeliveryAddress: "Jl. New Delivery 9" }),
        Conflict: null,
      });
    const onSaved = render("correction");

    typeInto("Correction reason", "Customer moved warehouse");
    typeInto("Delivery address", "Jl. New Delivery 9");
    await act(async () => {
      button("Save correction").click();
    });
    expect(document.body.textContent).toContain("Budi");

    await act(async () => {
      button("Overwrite").click();
    });
    // The second intervening save is not silently applied: the warning now
    // names Dewi and version 6, and the working copy is untouched.
    expect(document.body.textContent).toContain("This purchase order changed");
    expect(document.body.textContent).toContain("Dewi");
    expect(document.body.textContent).toContain("version 6");
    expect(document.body.textContent).not.toContain("Budi");
    expect(
      document.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="Delivery address"]'
      )?.value
    ).toBe("Jl. New Delivery 9");
    expect(onSaved).not.toHaveBeenCalled();
    expect(correctMock).toHaveBeenCalledTimes(2);
    expect((correctMock.mock.calls[1][0] as Record<string, unknown>).ExpectedVersion).toBe(5);
    expect((correctMock.mock.calls[1][0] as Record<string, unknown>).Overwrite).toBe(true);

    await act(async () => {
      button("Overwrite").click();
    });
    expect(correctMock).toHaveBeenCalledTimes(3);
    expect((correctMock.mock.calls[2][0] as Record<string, unknown>).ExpectedVersion).toBe(6);
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect((onSaved.mock.calls[0][0] as PurchaseOrderDTO).Version).toBe(7);
  });

  it("sends no protected identifier and offers no field for one", async () => {
    correctMock.mockResolvedValue({ Order: order({ Version: 5 }), Conflict: null });
    render("correction");

    // The confirmation number is visible as text, never as an editable field.
    expect(document.body.textContent).toContain("2302260001");
    for (const label of ["Confirmation number", "Customer", "Quotation", "Currency", "Source revision"]) {
      expect(
        document.querySelector(`input[aria-label="${label}"], textarea[aria-label="${label}"]`)
      ).toBeNull();
    }

    typeInto("Correction reason", "Fix a typo");
    await act(async () => {
      button("Save correction").click();
    });
    expect(correctMock).toHaveBeenCalledTimes(1);
    const payload = correctMock.mock.calls[0][0] as Record<string, unknown>;
    for (const protectedField of [
      "ConfirmationNumber",
      "QuotationID",
      "SourceRevision",
      "CustomerID",
      "Currency",
      "UUID",
      "Version",
      "State",
    ]) {
      expect(payload).not.toHaveProperty(protectedField);
    }
  });

  it("offers Final Save only on a draft, never on a saved correction", () => {
    render("correction");
    expect(queryButton("Final Save")).toBeNull();
    expect(queryButton("Save correction")).not.toBeNull();
  });
});

describe("draft overwrite (issue #473)", () => {
  it("warns and retries with the warned version when Overwrite is chosen", async () => {
    updateMock
      .mockResolvedValueOnce({ Order: null, Conflict: { ...CONFLICT_BUDI, CurrentState: "draft" } })
      .mockResolvedValueOnce({
        Order: order({ State: "draft", Version: 6 }),
        Conflict: null,
      });
    const onSaved = render("draft", vi.fn(), order({ State: "draft", Version: 2 }));

    typeInto("Customer PO number", "PO-473-EDITED");
    await act(async () => {
      button("Save draft").click();
    });
    expect(document.body.textContent).toContain("This purchase order changed");
    expect(document.body.textContent).toContain("Overwrite");

    await act(async () => {
      button("Overwrite").click();
    });
    expect(updateMock).toHaveBeenCalledTimes(2);
    const retry = updateMock.mock.calls[1][0] as Record<string, unknown>;
    expect(retry.ExpectedVersion).toBe(5);
    expect(retry.Overwrite).toBe(true);
    expect(retry.PONumber).toBe("PO-473-EDITED");
    expect(onSaved).toHaveBeenCalledTimes(1);
  });
});
