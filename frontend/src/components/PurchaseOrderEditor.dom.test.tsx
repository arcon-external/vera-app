// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import { usePurchaseOrderLeaveGuard, type AppPage } from "../poLeaveGuard";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import { AppShell } from "./AppShell";
import {
  PurchaseOrderDiscardPrompt,
  PurchaseOrderEditorPage,
} from "./PurchaseOrderEditor";

// Issue #468, wave 2: the tab-navigation loss the Judge reproduced. The editor
// owns its reducer state, so leaving the PO page unmounts it; the guard must
// ask before that happens, and Keep editing must leave the typed value alive.
//
// This is the runtime regression the SSR suite cannot reach. The harness is
// App's own composition: AppShell + the editor + the shared leave-guard hook
// and the shared discard prompt. App supplies only the one-line wiring this
// harness copies.

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

const ORDER: PurchaseOrderDTO = {
  PODate: null,
  RequestedDeliveryDate: null,
  UUID: "uuid-1",
  State: "draft",
  QuoteNumber: "1502260012",
  CustomerName: "PT. Buyer",
  BillingAddress: "Jl. Billing",
  DeliveryAddress: "Jl. Delivery",
  ShipToCode: "SHIP-1",
  ContactEmail: "buyer@example.com",
  ContactPhone: "+62 21 555",
  PONumber: "PO-1",
  ReceivedDate: "2026-10-20",
  Incoterms: "CIF",
  Currency: "USD",
  TaxZone: "NonFTZ",
  TaxCategory: "SupplyInstall",
  CreatedAt: "2026-10-20T10:00:00Z",
  UpdatedAt: "2026-10-20T10:00:00Z",
  Items: [
    {
      QuotationItemNo: 1,
      QuotationSubNo: null,
      LineNumber: 1,
      Description: "Widget",
      QuantityScaled: 25000,
      UoM: "Pc",
      UnitPriceMinor: 125050,
    },
  ],
  ID: 7,
  QuotationID: 12,
  SourceRevision: 1,
  CustomerID: 2,
  PaymentTerms: "45 days",
};

const INCOTERMS = [{ value: "CIF", label: "CIF" }];
const UOMS = [{ value: "Pc", label: "Pc" }];

function Workspace() {
  const [page, setPage] = useState<AppPage>("po");
  const leave = usePurchaseOrderLeaveGuard(page, setPage);
  return (
    <>
      <AppShell
        page={page}
        onOpenHome={() => leave.request("home")}
        onOpenEditor={() => leave.request("create")}
        onOpenDrafts={() => leave.request("drafts")}
        onOpenArchive={() => leave.request("archive")}
        companyName="PT Arcon Penta Persada"
        user={null}
      >
        {page === "po" ? (
          <PurchaseOrderEditorPage
            order={ORDER}
            currency={USD}
            uomOptions={UOMS}
            incotermOptions={INCOTERMS}
            onSaved={() => {}}
            onCancel={() => setPage("home")}
            onDirtyChange={leave.setDirty}
          />
        ) : (
          <p data-testid={`page-${page}`}>Destination {page}</p>
        )}
      </AppShell>
      {leave.pendingPage !== null ? (
        <PurchaseOrderDiscardPrompt
          overlay
          onKeepEditing={leave.keepEditing}
          onDiscard={leave.discard}
        />
      ) : null}
    </>
  );
}

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
  document.body.innerHTML = "";
});

function tab(label: string): HTMLButtonElement {
  const nav = document.querySelector<HTMLElement>('nav[aria-label="Primary"]');
  const found = Array.from(nav?.querySelectorAll<HTMLButtonElement>("button") ?? []).find(
    (button) => button.textContent?.trim() === label
  );
  if (!found) throw new Error(`tab ${label} not found`);
  return found;
}

function click(element: HTMLElement): void {
  act(() => element.click());
}

function typeInto(label: string, value: string): void {
  const input = document.querySelector<HTMLInputElement>(
    `input[aria-label="${label}"]`
  );
  if (!input) throw new Error(`input ${label} not found`);
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value"
  )?.set;
  act(() => {
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function prompt(): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    '[role="alertdialog"][aria-label="Discard unsaved changes?"]'
  );
}

function button(label: string, scope: ParentNode = document): HTMLButtonElement {
  const found = Array.from(
    scope.querySelectorAll<HTMLButtonElement>(`button[aria-label="${label}"]`)
  )[0];
  if (!found) throw new Error(`button ${label} not found`);
  return found;
}

describe("PO tab navigation leaves unsaved edits (issue #468)", () => {
  it("asks before a tab unmounts an edited draft, and Keep editing keeps it", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "50% advance");
    expect(
      document.querySelector<HTMLInputElement>('input[aria-label="Payment terms"]')
        ?.value
    ).toBe("50% advance");

    click(tab("Editor"));
    expect(prompt()).not.toBeNull();
    // Still on the PO page: the editor is mounted underneath the prompt.
    expect(document.querySelector('[data-testid="page-create"]')).toBeNull();

    click(button("Keep editing"));
    expect(prompt()).toBeNull();
    expect(
      document.querySelector<HTMLInputElement>('input[aria-label="Payment terms"]')
        ?.value
    ).toBe("50% advance");
  });

  it("Discard changes navigates to the requested tab", () => {
    act(() => root.render(<Workspace />));

    typeInto("Ship-to code", "SHIP-EDITED");
    click(tab("Archive"));
    expect(prompt()).not.toBeNull();

    click(button("Discard changes"));
    expect(prompt()).toBeNull();
    expect(document.querySelector('[data-testid="page-archive"]')).not.toBeNull();
    expect(
      document.querySelector('input[aria-label="Payment terms"]')
    ).toBeNull();
  });

  it("navigates without a prompt when nothing changed", () => {
    act(() => root.render(<Workspace />));

    click(tab("Drafts"));
    expect(prompt()).toBeNull();
    expect(document.querySelector('[data-testid="page-drafts"]')).not.toBeNull();
  });
});
