// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderReviewDTO } from "../poFinalize";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import { PurchaseOrderRepairPage } from "./PurchaseOrderRepair";

// Issue #475: the admin repair flow in a real DOM.
//
// The SSR suite proves the markup; this suite drives the decisions the ticket
// is about: no difference is preselected, the reason is required before the
// write, the exact payload names the target revision and the loaded version,
// a stale version is shown as the named conflict (never an overwrite), and
// Cancel leaves the saved record untouched.

const { repairMock } = vi.hoisted(() => ({ repairMock: vi.fn() }));

vi.mock("@backend/backend/Quotation", () => ({
  RepairPurchaseOrderSource: (...args: unknown[]) => repairMock(...args),
  AssignPurchaseOrderConfirmationNumber: vi.fn(),
  GetPurchaseOrderHistory: vi.fn(async () => []),
}));

vi.mock("@backend/models", () => ({
  backend: {
    PurchaseOrderSourceRepairInput: class {
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
    UUID: "uuid-475-dom",
    State: "saved",
    QuoteNumber: "1502260001",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing A",
    DeliveryAddress: "Jl. Billing A",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "+62 21 555",
    PONumber: "PO-475",
    ReceivedDate: "2026-10-02",
    Incoterms: "CIF",
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-01T00:00:00Z",
    UpdatedAt: "2026-10-01T00:00:00Z",
    Items: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Priced widget",
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
    QuotationID: 1,
    SourceRevision: 0,
    CustomerID: 2,
    PaymentTerms: "45 days",
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
    SavedAt: "2026-10-02T00:00:00Z",
    SavedBy: 1,
    PdfAbsentConfirmed: true,
    ConfirmationNumber: null,
    ConfirmationAssignedAt: null,
    ConfirmationWarn: false,
    Version: 4,
    ...overrides,
  };
}

function review(): PurchaseOrderReviewDTO {
  return {
    Order: order(),
    Source: {
      QuotationID: 9,
      RevisionNumber: 1,
      QuoteNumber: "1502260009",
      CustomerID: 2,
      CustomerName: "PT. Buyer",
      Currency: "USD",
      PaymentTerms: "30 days",
      Incoterms: "CIF",
      DiscountPercent: 0,
      Lines: [
        {
          QuotationItemNo: 1,
          QuotationSubNo: null,
          LineLabel: "1",
          Description: "Priced widget",
          UoM: "Pc",
          QuantityScaled: 20000,
          UnitPriceMinor: 10000,
          DiscountEnabled: false,
          DiscountPercent: 0,
        },
      ],
      PrintedSubtotalMinor: 20000,
      PrintedDiscountMinor: 0,
      PrintedPpnMinor: 0,
      PrintedGrandTotalMinor: 20000,
    },
    DocumentState: "none",
  };
}

/**
 * The Judge's scenario: the candidate shares no line reference, unit price,
 * payment terms or Incoterms with the record, so the review can only pass after
 * the match is corrected and the quotation values are chosen.
 */
function mismatchedReview(): PurchaseOrderReviewDTO {
  return {
    Order: order(),
    Source: {
      QuotationID: 9,
      RevisionNumber: 1,
      QuoteNumber: "1502260009",
      CustomerID: 2,
      CustomerName: "PT. Buyer",
      Currency: "USD",
      PaymentTerms: "30 days",
      Incoterms: "FOB",
      DiscountPercent: 0,
      Lines: [
        {
          QuotationItemNo: 2,
          QuotationSubNo: null,
          LineLabel: "2",
          Description: "Revised widget",
          UoM: "Pc",
          QuantityScaled: 20000,
          UnitPriceMinor: 12000,
          DiscountEnabled: false,
          DiscountPercent: 0,
        },
      ],
      PrintedSubtotalMinor: 24000,
      PrintedDiscountMinor: 0,
      PrintedPpnMinor: 0,
      PrintedGrandTotalMinor: 24000,
    },
    DocumentState: "none",
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  repairMock.mockReset();
  // jsdom has no layout, so the Select's scroll-into-view effect would throw.
  Element.prototype.scrollIntoView = () => undefined;
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

async function renderPage(props: {
  review?: PurchaseOrderReviewDTO;
  onCancel?: () => void;
  onRepaired?: (order: PurchaseOrderDTO) => void;
  onDirtyChange?: (dirty: boolean) => void;
} = {}): Promise<void> {
  await act(async () => {
    root.render(
      <PurchaseOrderRepairPage
        review={props.review ?? review()}
        currency={USD}
        onCancel={props.onCancel ?? (() => undefined)}
        onRepaired={props.onRepaired ?? (() => undefined)}
        onDirtyChange={props.onDirtyChange}
      />
    );
  });
  await flush();
}

/** Chooses a quotation line in one line's match picker. */
async function chooseMatch(lineNumber: number, label: string): Promise<void> {
  await click(button(`Line ${lineNumber} quotation match`));
  const option = Array.from(
    document.querySelectorAll<HTMLButtonElement>('[role="option"]')
  ).find((candidate) => candidate.textContent?.includes(label));
  if (option === undefined) throw new Error(`no match option "${label}"`);
  await click(option);
}

function button(label: string): HTMLButtonElement {
  const element = Array.from(document.querySelectorAll("button")).find(
    (candidate) => candidate.getAttribute("aria-label") === label
  );
  if (element === undefined) throw new Error(`no button labelled "${label}"`);
  return element;
}

async function click(element: HTMLElement): Promise<void> {
  await act(async () => {
    element.focus();
    element.click();
  });
  await flush();
}

function textarea(label: string): HTMLTextAreaElement {
  const element = document.querySelector<HTMLTextAreaElement>(
    `textarea[aria-label="${label}"]`
  );
  if (element === null) throw new Error(`no textarea labelled "${label}"`);
  return element;
}

async function type(element: HTMLTextAreaElement, value: string): Promise<void> {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value"
    )?.set;
    setter?.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

describe("PurchaseOrderRepairPage (issue #475)", () => {
  it("requires an explicit difference choice before the reason step", async () => {
    await renderPage();
    expect(document.body.textContent).toContain(
      "Review differences against the new source"
    );
    const continueButton = button("Continue to repair");
    expect(continueButton.disabled).toBe(true);
    await click(button("Use PO entry for Payment terms"));
    expect(button("Continue to repair").disabled).toBe(false);
  });

  it("sends the target, the loaded version and the required reason", async () => {
    repairMock.mockResolvedValue({
      Order: order({ QuotationID: 9, QuoteNumber: "1502260009", Version: 5 }),
    });
    const repaired: PurchaseOrderDTO[] = [];
    await renderPage({ onRepaired: (record) => repaired.push(record) });

    await click(button("Use PO entry for Payment terms"));
    await click(button("Continue to repair"));
    expect(document.body.textContent).toContain("Confirm the source repair");

    const save = button("Repair source link");
    expect(save.disabled).toBe(true);
    await type(textarea("Reason for repairing the source link"), "wrong revision linked");
    expect(save.disabled).toBe(false);
    await click(save);

    expect(repairMock).toHaveBeenCalledTimes(1);
    const payload = repairMock.mock.calls[0][0] as Record<string, unknown>;
    expect(payload.TargetQuotationID).toBe(9);
    expect(payload.TargetRevisionNumber).toBe(1);
    expect(payload.ExpectedVersion).toBe(4);
    expect(payload.Reason).toBe("wrong revision linked");
    expect(payload.PONumber).toBe("PO-475");
    expect(payload.Decisions).toEqual([
      { Key: "header:paymentTerms", Choice: "po" },
    ]);
    expect(repaired).toHaveLength(1);
    expect(repaired[0].QuotationID).toBe(9);
  });

  it("treats a corrected line match as an unsaved change", async () => {
    const dirty: boolean[] = [];
    await renderPage({
      review: mismatchedReview(),
      onDirtyChange: (value) => dirty.push(value),
    });
    expect(dirty).toEqual([]);

    await chooseMatch(1, "2 — Revised widget");

    expect(dirty).toEqual([true]);
  });

  it("saves the corrected match and the values chosen for it", async () => {
    repairMock.mockResolvedValue({
      Order: order({ QuotationID: 9, QuoteNumber: "1502260009", Version: 5 }),
    });
    await renderPage({ review: mismatchedReview() });

    await chooseMatch(1, "2 — Revised widget");
    // The corrected match moves the comparison to the candidate's own line:
    // only its description, price, payment terms and Incoterms differ now.
    expect(document.body.textContent).toContain("Currently quotation line 2");
    for (const label of [
      "Payment terms",
      "Incoterms",
      "Line 1 description",
      "Line 1 unit price",
    ]) {
      await click(button(`Use Quotation for ${label}`));
    }
    await click(button("Continue to repair"));
    expect(document.body.textContent).toContain("Confirm the source repair");
    await type(
      textarea("Reason for repairing the source link"),
      "the order followed another quotation line"
    );
    await click(button("Repair source link"));

    expect(repairMock).toHaveBeenCalledTimes(1);
    const payload = repairMock.mock.calls[0][0] as {
      Items: Array<Record<string, unknown>>;
      Decisions: Array<{ Key: string; Choice: string }>;
      PaymentTerms: string;
      Incoterms: string;
      TargetQuotationID: number;
      TargetRevisionNumber: number;
      ExpectedVersion: number;
      Reason: string;
    };
    expect(payload.TargetQuotationID).toBe(9);
    expect(payload.TargetRevisionNumber).toBe(1);
    expect(payload.ExpectedVersion).toBe(4);
    expect(payload.Reason).toBe("the order followed another quotation line");
    expect(payload.PaymentTerms).toBe("30 days");
    expect(payload.Incoterms).toBe("FOB");
    expect(payload.Items).toEqual([
      {
        QuotationItemNo: 2,
        QuotationSubNo: null,
        Description: "Revised widget",
        QuantityScaled: 20000,
        UoM: "Pc",
        UnitPriceMinor: 12000,
        DiscountEnabled: false,
        DiscountPercent: 0,
      },
    ]);
    expect(payload.Decisions).toEqual([
      { Key: "header:paymentTerms", Choice: "quotation" },
      { Key: "header:incoterms", Choice: "quotation" },
      { Key: "line:1:description", Choice: "quotation" },
      { Key: "line:1:unitPrice", Choice: "quotation" },
    ]);
  });

  it("shows a stale version as the named conflict instead of overwriting", async () => {
    repairMock.mockResolvedValue({
      Conflict: {
        CurrentState: "saved",
        CurrentVersion: 5,
        UpdatedBy: 9,
        UpdatedByName: "Budi",
        UpdatedAt: "2026-10-21T10:15:00+07:00",
      },
    });
    const repaired: PurchaseOrderDTO[] = [];
    await renderPage({ onRepaired: (record) => repaired.push(record) });

    await click(button("Use PO entry for Payment terms"));
    await click(button("Continue to repair"));
    await type(textarea("Reason for repairing the source link"), "wrong revision linked");
    await click(button("Repair source link"));

    expect(document.body.textContent).toContain("Budi saved version 5");
    expect(document.body.textContent).toContain(
      "reload the purchase order and review the new source again"
    );
    // A stale base cannot be retried with the same values: the save is
    // disabled until the record is reloaded.
    expect(button("Repair source link").disabled).toBe(true);
    expect(repaired).toHaveLength(0);
  });

  it("leaves the saved record untouched when the review is cancelled", async () => {
    let cancelled = 0;
    await renderPage({ onCancel: () => (cancelled += 1) });
    await click(button("Back without repairing"));
    expect(cancelled).toBe(1);
    expect(repairMock).not.toHaveBeenCalled();
  });
});
