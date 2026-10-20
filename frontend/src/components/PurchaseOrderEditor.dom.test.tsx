// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
// Wave 3: the Judge found two dialogs and no modal behaviour. Cancel used to
// open the editor's inline panel while a tab opened App's overlay, so both
// could stand at once. There is now one pending discard decision and one
// dialog; this suite holds that line.
//
// This is the runtime regression the SSR suite cannot reach. The harness is
// App's own composition: AppShell + the editor + the shared leave-guard hook
// and the shared discard prompt. App supplies only the one-line wiring this
// harness copies.

// The editor page now mounts the document panel, which reads the one optional
// customer PO PDF through the desktop binding (#472). This suite is about tab
// navigation, so the document read is a stable empty state.
vi.mock("@backend/backend/Quotation", () => ({
  GetPurchaseOrderDocumentState: vi.fn(async () => ({ Document: null, Versions: [] })),
  AttachPurchaseOrderDocument: vi.fn(),
  GetActivePurchaseOrderDocumentContent: vi.fn(),
  RemovePurchaseOrderDocument: vi.fn(),
  UpdatePurchaseOrderDraft: vi.fn(),
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
  Version: 1,
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
      DiscountEnabled: false,
      DiscountPercent: 0,
      GrossMinor: 312625,
      DiscountMinor: 0,
      NetMinor: 312625,
    },
  ],
  ID: 7,
  QuotationID: 12,
  SourceRevision: 1,
  CustomerID: 2,
  PaymentTerms: "45 days",
  AdjustmentReason: "",
  DiscountPercent: 0,
  PpnBps: 0,
  SubtotalMinor: 312625,
  DiscountMinor: 0,
  PpnMinor: 0,
  AdjustmentMinor: 0,
  GrandTotalMinor: 312625,
  PrintedSubtotalMinor: 312625,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 312625,
};

const INCOTERMS = [{ value: "CIF", label: "CIF" }];
const UOMS = [{ value: "Pc", label: "Pc" }];

function Workspace() {
  const [page, setPage] = useState<AppPage>("po");
  const leave = usePurchaseOrderLeaveGuard(page, setPage);
  const open = leave.pendingPage !== null;
  return (
    <>
      <AppShell
        page={page}
        inert={open}
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
            onCancel={() => leave.request("home", () => setPage("home"))}
            onDirtyChange={leave.setDirty}
          />
        ) : (
          <p data-testid={`page-${page}`}>Destination {page}</p>
        )}
      </AppShell>
      {open ? (
        <PurchaseOrderDiscardPrompt
          onKeepEditing={leave.keepEditing}
          onDiscard={leave.discard}
          returnFocus={leave.pendingReturnFocus}
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

function dialogCount(): number {
  return document.querySelectorAll('[role="alertdialog"]').length;
}

function paymentTermsValue(): string | undefined {
  return document.querySelector<HTMLInputElement>(
    'input[aria-label="Payment terms"]'
  )?.value;
}

function pressKey(target: EventTarget, key: string, shiftKey = false): void {
  act(() => {
    target.dispatchEvent(
      new KeyboardEvent("keydown", {
        key,
        shiftKey,
        bubbles: true,
        cancelable: true,
      })
    );
  });
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

describe("PO discard is one modal decision (issue #468, wave 3)", () => {
  it("Cancel opens one dialog, a tab click replaces its action, Keep editing stays", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "50% advance");
    const cancel = button("Cancel draft");
    act(() => cancel.focus());
    click(cancel);

    expect(dialogCount()).toBe(1);
    const dialog = prompt();
    expect(dialog).not.toBeNull();
    expect(dialog?.contains(document.activeElement)).toBe(true);

    const shell = document.querySelector<HTMLElement>("[data-app-shell]");
    expect(shell).not.toBeNull();
    expect(shell?.getAttribute("aria-hidden")).toBe("true");
    expect(shell?.hasAttribute("inert")).toBe(true);

    // A tab click while the Cancel confirmation stands must replace the
    // pending action, never stack a second dialog.
    click(tab("Archive"));
    expect(dialogCount()).toBe(1);
    expect(prompt()?.contains(document.activeElement)).toBe(true);

    click(button("Keep editing"));
    expect(dialogCount()).toBe(0);
    expect(document.querySelector('[data-testid="page-archive"]')).toBeNull();
    expect(paymentTermsValue()).toBe("50% advance");
    expect(document.activeElement).toBe(cancel);
    expect(shell?.getAttribute("aria-hidden")).toBeNull();
    expect(shell?.hasAttribute("inert")).toBe(false);
  });

  it("Discard from a Cancel-then-tab prompt performs the tab action", () => {
    act(() => root.render(<Workspace />));

    typeInto("Ship-to code", "SHIP-EDITED");
    click(button("Cancel draft"));
    expect(dialogCount()).toBe(1);

    click(tab("Archive"));
    expect(dialogCount()).toBe(1);

    click(button("Discard changes"));
    expect(dialogCount()).toBe(0);
    expect(document.querySelector('[data-testid="page-archive"]')).not.toBeNull();
    expect(paymentTermsValue()).toBeUndefined();
  });

  it("Discard from Cancel leaves the PO workspace", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "50% advance");
    click(button("Cancel draft"));
    expect(dialogCount()).toBe(1);

    click(button("Discard changes"));
    expect(dialogCount()).toBe(0);
    expect(document.querySelector('[data-testid="page-home"]')).not.toBeNull();
    expect(paymentTermsValue()).toBeUndefined();
  });

  it("Escape keeps editing and restores focus to the button that asked", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "30 days after delivery");
    const cancel = button("Cancel draft");
    act(() => cancel.focus());
    click(cancel);

    expect(dialogCount()).toBe(1);
    expect(document.activeElement).toBe(button("Keep editing"));

    pressKey(document, "Escape");

    expect(dialogCount()).toBe(0);
    expect(paymentTermsValue()).toBe("30 days after delivery");
    expect(document.activeElement).toBe(cancel);
  });

  it("contains Tab and Shift+Tab inside the dialog", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "50% advance");
    click(button("Cancel draft"));

    const keep = button("Keep editing");
    const discard = button("Discard changes");
    expect(dialogCount()).toBe(1);
    expect(document.activeElement).toBe(keep);

    pressKey(keep, "Tab", true);
    expect(document.activeElement).toBe(discard);

    pressKey(discard, "Tab");
    expect(document.activeElement).toBe(keep);
    expect(dialogCount()).toBe(1);
    expect(prompt()?.contains(document.activeElement)).toBe(true);
  });
});
