// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import type { ReconciliationHistoryInput } from "../poReconciliationHistory";
import { PurchaseOrderSavedView } from "./PurchaseOrderFinalize";

// #518: the saved reconciliation surface in a real DOM.
//
// The SSR suite proves the markup; this one drives it: the 360 trigger and its
// sheet focus contract, the saved-only filter pair, selecting an earlier
// generation in place, returning to the current one, a deleted source that
// keeps its retained items, and a failed read that retries without ever
// claiming the PO has no session.

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
    UUID: "po-uuid-518",
    State: "saved",
    QuoteNumber: "1502260044",
    CustomerName: "PT. Buyer Nusantara",
    BillingAddress: "Jl. Satu 1",
    DeliveryAddress: "Jl. Dua 2",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "021-555",
    PONumber: "PO-518",
    ReceivedDate: "2026-10-20",
    Incoterms: "FOB",
    Currency: "USD",
    TaxZone: "JKT",
    TaxCategory: "standard",
    AdjustmentReason: "",
    CreatedAt: "2026-10-20T09:00:00Z",
    UpdatedAt: "2026-10-21T09:00:00Z",
    Items: [
      {
        ID: 1,
        ClientKey: "client:line-1",
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
    ID: 518,
    QuotationID: 44,
    SourceRevision: 2,
    CustomerID: 9,
    PaymentTerms: "50% advance",
    DiscountPercent: 0,
    PpnBps: 1100,
    SubtotalMinor: 20000,
    DiscountMinor: 0,
    PpnMinor: 2200,
    AdjustmentMinor: 0,
    GrandTotalMinor: 22200,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 2200,
    PrintedGrandTotalMinor: 22200,
    Version: 6,
    SavedAt: "2026-10-21T09:00:00Z",
    SavedBy: 3,
    PdfAbsentConfirmed: true,
    ...overrides,
  };
}

const READ: ReconciliationHistoryInput = {
  PurchaseOrderID: 518,
  POVersion: 6,
  Sessions: [
    {
      Generation: 1,
      SourceAvailable: true,
      RepairCorrectionID: null,
      Session: {
        ID: 12,
        PurchaseOrderID: 518,
        SourceQuotationID: 44,
        SourceRevision: 2,
        POVersion: 6,
        Active: true,
        ReadOnly: true,
        CreatedBy: 3,
        CreatedAt: "2026-10-21T09:00:00Z",
        UpdatedAt: "2026-10-21T09:05:00Z",
        LineIdentities: [],
        Items: [
          {
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
            ActorID: 3,
            UpdatedAt: "2026-10-21T09:05:00Z",
            History: [],
          },
          {
            Key: "link:taxBasis",
            Field: "link:taxBasis",
            Kind: "tax_basis",
            LineKey: null,
            SourceLineRef: null,
            Label: "Tax basis",
            POValue: { TaxZone: "JKT", TaxType: "PPN", ServiceCategory: "" },
            SourceValue: { TaxZone: "JKT", TaxType: "PPN", ServiceCategory: "" },
            Status: "resolved",
            Resolution: "used_quotation",
            ActorID: 3,
            UpdatedAt: "2026-10-21T09:05:00Z",
            History: [],
          },
        ],
      },
    },
    {
      Generation: 0,
      SourceAvailable: false,
      RepairCorrectionID: null,
      Session: {
        ID: 9,
        PurchaseOrderID: 518,
        SourceQuotationID: 43,
        SourceRevision: 1,
        POVersion: 4,
        Active: false,
        ReadOnly: true,
        CreatedBy: 3,
        CreatedAt: "2026-10-20T09:00:00Z",
        UpdatedAt: "2026-10-20T09:05:00Z",
        LineIdentities: [],
        Items: [
          {
            Key: "header:paymentTerms",
            Field: "header:paymentTerms",
            Kind: "difference",
            LineKey: null,
            SourceLineRef: null,
            Label: "Payment terms",
            POValue: "50% advance",
            SourceValue: "14 days",
            Status: "resolved",
            Resolution: "kept_po",
            ActorID: 3,
            UpdatedAt: "2026-10-20T09:05:00Z",
            History: [],
          },
        ],
      },
    },
  ],
};

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function transportThat(read: ReconciliationHistoryInput | Error) {
  const list = vi.fn(async () => {
    if (read instanceof Error) throw read;
    return read;
  });
  return { list };
}

async function render(element: React.ReactElement) {
  await act(async () => {
    root.render(element);
  });
  await act(async () => {});
}

function saved(element: React.ReactElement) {
  return element;
}

const baseProps = {
  order: order(),
  currency: USD,
  onBack: () => undefined,
  documentInitialState: { Document: null, Versions: [] as never[] },
  historyInitial: [],
};

function rail(): HTMLElement {
  const aside = container.querySelector<HTMLElement>('aside[aria-label="Reconciliation"]');
  if (aside === null) throw new Error("the saved rail did not render");
  return aside;
}

describe("PurchaseOrderSavedView reconciliation surface (#518)", () => {
  it("focuses the saved heading on arrival", async () => {
    await render(
      saved(
        <PurchaseOrderSavedView
          {...baseProps}
          reconciliationTransport={transportThat(READ)}
        />
      )
    );
    const heading = container.querySelector("h1");
    expect(heading).not.toBeNull();
    expect(heading?.getAttribute("tabindex")).toBe("-1");
    expect(document.activeElement).toBe(heading);
  });

  it("opens the 360 sheet on its heading and returns focus to the trigger on Escape", async () => {
    await render(
      saved(
        <PurchaseOrderSavedView
          {...baseProps}
          reconciliationTransport={transportThat(READ)}
        />
      )
    );
    const trigger = container.querySelector<HTMLButtonElement>(
      '[aria-label="Reconciliation, read-only. Open panel."]'
    );
    expect(trigger).not.toBeNull();
    act(() => trigger?.click());
    await act(async () => {});
    expect(document.getElementById("saved-sheet-pane-heading")).toBe(
      document.activeElement
    );

    const sheet = container.querySelector<HTMLElement>(
      'div[role="region"][aria-label="Reconciliation"]'
    );
    expect(sheet).not.toBeNull();
    act(() => {
      sheet?.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", bubbles: true })
      );
    });
    await act(async () => {});
    expect(
      container.querySelector('div[role="region"][aria-label="Reconciliation"]')
    ).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("offers only All and Resolved, and a saved record exposes no decision control", async () => {
    await render(
      saved(
        <PurchaseOrderSavedView
          {...baseProps}
          reconciliationTransport={transportThat(READ)}
        />
      )
    );
    const tabs = Array.from(rail().querySelectorAll('[role="tab"]'));
    expect(tabs).toHaveLength(2);
    expect(tabs[0]?.textContent).toContain("All");
    expect(tabs[1]?.textContent).toContain("Resolved");
    expect(tabs[0]?.textContent).toContain("2");
    expect(rail().querySelector('[role="tab"][id*="unresolved"]')).toBeNull();
    // Read-only: the pane renders values, never a Keep/Use/acknowledge action.
    expect(rail().textContent).not.toContain("Keep PO");
    expect(rail().textContent).not.toContain("Use Quotation");
    expect(rail().textContent).not.toContain("Acknowledge");
    expect(rail().querySelectorAll("button[aria-pressed]")).toHaveLength(1);

    // Two tabs, one tab stop: Right from the active tab lands on the other.
    const tablist = rail().querySelector<HTMLElement>('[role="tablist"]');
    expect(tablist).not.toBeNull();
    act(() => {
      tablist?.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })
      );
    });
    await act(async () => {});
    expect(document.getElementById("saved-rail-filter-resolved")).toBe(
      document.activeElement
    );
  });

  it("selects an earlier generation in place, keeps its retained items and announces it", async () => {
    await render(
      saved(
        <PurchaseOrderSavedView
          {...baseProps}
          reconciliationTransport={transportThat(READ)}
        />
      )
    );
    const row = rail().querySelector<HTMLButtonElement>("button[aria-pressed]");
    expect(row).not.toBeNull();
    act(() => row?.click());
    await act(async () => {});
    expect(rail().textContent).toContain(
      "Previous saved session · revision 1 · generation 0"
    );
    // A deleted source is availability context: the notice appears AND the
    // stored difference still renders.
    expect(rail().textContent).toContain("Source deleted · Quotation 43 · revision 1");
    expect(rail().textContent).toContain("Payment terms");
    expect(document.activeElement).toBe(
      document.getElementById("saved-rail-pane-heading")
    );
    expect(container.textContent).toContain(
      "Showing saved session, revision 1, generation 0."
    );

    const back = Array.from(rail().querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Back to current session")
    );
    expect(back).toBeDefined();
    act(() => back?.click());
    await act(async () => {});
    expect(rail().textContent).toContain("Saved session · revision 2 · generation 1");
    expect(rail().textContent).not.toContain("Back to current session");
    expect(container.textContent).toContain("Showing the current saved session.");
  });

  it("focuses the new current generation after a repair and announces it", async () => {
    const handled = vi.fn();
    await render(
      saved(
        <PurchaseOrderSavedView
          {...baseProps}
          reconciliationTransport={transportThat(READ)}
          repairReturn={{ kind: "repaired", revision: 2 }}
          onRepairReturnHandled={handled}
        />
      )
    );
    expect(document.activeElement).toBe(
      document.getElementById("saved-rail-pane-heading")
    );
    expect(container.textContent).toContain(
      "Source repaired. Showing saved session revision 2."
    );
    expect(handled).toHaveBeenCalledTimes(1);
  });

  it("returns focus to the repair action when the repair is cancelled", async () => {
    const handled = vi.fn();
    const repair = vi.fn();
    await render(
      saved(
        <PurchaseOrderSavedView
          {...baseProps}
          canRepairSource
          onRepairSource={repair}
          reconciliationTransport={transportThat(READ)}
          repairReturn={{ kind: "cancelled" }}
          onRepairReturnHandled={handled}
        />
      )
    );
    const action = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Repair source link"]'
    );
    expect(action).not.toBeNull();
    expect(document.activeElement).toBe(action);
    expect(handled).toHaveBeenCalledTimes(1);
  });

  it("reloads a stale read, keeps the record reload and focuses the pane heading", async () => {
    let calls = 0;
    const staleThenReady = {
      list: vi.fn(async () => {
        calls += 1;
        return calls === 1 ? { ...READ, POVersion: 5 } : READ;
      }),
    };
    const reloadRecord = vi.fn();
    await render(
      saved(
        <PurchaseOrderSavedView
          {...baseProps}
          reconciliationTransport={staleThenReady}
          onReload={reloadRecord}
        />
      )
    );
    expect(rail().textContent).toContain(
      "This purchase order changed while its reconciliation was loading."
    );
    const reload = Array.from(rail().querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Reload")
    );
    expect(reload).toBeDefined();
    act(() => reload?.click());
    await act(async () => {});
    expect(reloadRecord).toHaveBeenCalledTimes(1);
    expect(staleThenReady.list).toHaveBeenCalledTimes(2);
    expect(rail().textContent).toContain("Saved session · revision 2 · generation 1");
    expect(document.activeElement).toBe(
      document.getElementById("saved-rail-pane-heading")
    );
  });

  it("retries a failed read through the same surface", async () => {
    let calls = 0;
    const flaky = {
      list: vi.fn(async () => {
        calls += 1;
        if (calls === 1) throw new Error("connection reset");
        return READ;
      }),
    };
    await render(
      saved(
        <PurchaseOrderSavedView {...baseProps} reconciliationTransport={flaky} />
      )
    );
    expect(rail().textContent).toContain(
      "The saved reconciliation history could not be loaded."
    );
    expect(container.textContent).not.toContain(
      "No reconciliation session was recorded for this purchase order."
    );

    const retry = Array.from(rail().querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Try again")
    );
    expect(retry).toBeDefined();
    act(() => retry?.click());
    await act(async () => {});
    expect(flaky.list).toHaveBeenCalledTimes(2);
    expect(rail().textContent).toContain("Saved session · revision 2 · generation 1");
    expect(rail().textContent).not.toContain(
      "The saved reconciliation history could not be loaded."
    );
  });
});
