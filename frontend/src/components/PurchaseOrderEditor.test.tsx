import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import {
  initialPurchaseOrderEditorState,
  purchaseOrderEditorReducer,
  type PurchaseOrderDTO,
} from "../purchaseOrder";
import {
  PurchaseOrderDiscardPrompt,
  PurchaseOrderEditorView,
} from "./PurchaseOrderEditor";

// Issue #468: the draft editor's markup, rendered without a DOM. The pure
// suite owns parsing and the dirty guard; this suite proves the assembled page
// shows the read-only customer snapshot, the editable commercial details and
// the shared discard dialog without inventing totals or final actions.
//
// What it cannot prove: typing, Select keyboard behaviour, focus, and how the
// page reads at 360/1280. Those stay hand-driven plus the snapshot harness.

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
  PODate: "2026-10-01",
  RequestedDeliveryDate: "2026-11-20",
  UUID: "uuid-1",
  State: "draft",
  QuoteNumber: "1502260012",
  CustomerName: "PT. Infineon Technologies Batam",
  BillingAddress: "Batamindo Industrial Park",
  DeliveryAddress: "Batamindo Industrial Park, Dock 3",
  ShipToCode: "SHIP-1",
  ContactEmail: "buyer@example.com",
  ContactPhone: "+62 21 555",
  PONumber: "PO-9001",
  ReceivedDate: "2026-10-02",
  Version: 1,
  Incoterms: "CIF",
  Currency: "USD",
  TaxZone: "NonFTZ",
  TaxCategory: "SupplyInstall",
  CreatedAt: "2026-10-02T10:00:00Z",
  UpdatedAt: "2026-10-02T10:00:00Z",
  Items: [
    {
      QuotationItemNo: 1,
      QuotationSubNo: null,
      LineNumber: 1,
      Description: "Widget",
      QuantityScaled: 25000,
      UoM: "Pc",
      UnitPriceMinor: 125050,
      DiscountEnabled: true,
      DiscountPercent: 10,
      GrossMinor: 312625,
      DiscountMinor: 31262,
      NetMinor: 281363,
    },
    {
      QuotationItemNo: 2,
      QuotationSubNo: 1,
      LineNumber: 2,
      Description: "Mounting",
      QuantityScaled: 10000,
      UoM: "Unit",
      UnitPriceMinor: 42000,
      DiscountEnabled: false,
      DiscountPercent: 0,
      GrossMinor: 42000,
      DiscountMinor: 0,
      NetMinor: 42000,
    },
  ],
  ID: 7,
  QuotationID: 12,
  SourceRevision: 1,
  CustomerID: 2,
  PaymentTerms: "45 days",
  AdjustmentReason: "Round to the printed total",
  DiscountPercent: 5,
  PpnBps: 0,
  SubtotalMinor: 323363,
  DiscountMinor: 16168,
  PpnMinor: 0,
  AdjustmentMinor: -250,
  GrandTotalMinor: 306945,
  PrintedSubtotalMinor: 305000,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 305000,
};

const UOM_OPTIONS = [
  { value: "Pc", label: "Pc" },
  { value: "Unit", label: "Unit" },
  { value: "Lot", label: "Lot" },
];

const INCOTERMS = ["EXW", "FOB", "CIF"].map((value) => ({
  value,
  label: value,
}));

function render(state = initialPurchaseOrderEditorState(ORDER, USD)): string {
  return renderToString(
    <PurchaseOrderEditorView
      state={state}
      dispatch={() => {}}
      currency={USD}
      uomOptions={UOM_OPTIONS}
      incotermOptions={INCOTERMS}
      onSave={() => {}}
      onCancel={() => {}}
      onFinalSave={() => {}}
      finalSaving={false}
    />
  ).replace(/<!-- -->/g, "");
}

function buttonTagFor(html: string, label: string): string {
  const button = html
    .split("<button")
    .slice(1)
    .find((chunk) => chunk.slice(0, chunk.indexOf(">")).includes(`aria-label="${label}"`));
  if (!button) throw new Error(`no <button aria-label="${label}"> in: ${html}`);
  return button.slice(0, button.indexOf(">") + 1);
}

describe("PurchaseOrderEditorView header (issue #468)", () => {
  it("shows the quotation identity, read-only customer snapshot and editable terms", () => {
    const html = render();
    // #580: the hero is the customer's PO number; the quotation identity stays
    // in the Source quotation block.
    expect(html).toContain(">PO-9001</h1>");
    expect(html).toContain("Source quotation");
    expect(html).toContain("rev 01");
    expect(html).toContain("1502260012");
    expect(html).toContain("PT. Infineon Technologies Batam");
    expect(html).toContain("revision 1");
    expect(html).toContain("USD");
    expect(html).toContain("Services / NonConstruction");
    expect(html).toContain("Draft");

    // #580: the PO-owned details have their own card under the hero.
    expect(html).toContain("Purchase order details");
    expect(html).toContain('aria-label="Billing address"');
    expect(html).toContain('aria-label="Delivery address"');
    expect(html).toContain('aria-label="Ship-to code"');
    expect(html).toContain('aria-label="Contact email"');
    expect(html).toContain('aria-label="Contact phone"');
    expect(html).toContain('aria-label="Customer PO number"');
    expect(html).toContain('aria-label="PO date"');
    expect(html).toContain('aria-label="Received date"');
    expect(html).toContain('aria-label="Requested delivery date"');
    expect(html).toContain('aria-label="Payment terms"');
    expect(html).toContain('value="45 days"');
    expect(html).toContain('aria-label="Incoterms"');

    // One control per value: moving the fields must not duplicate them.
    expect(html.match(/aria-label="Customer PO number"/g)).toHaveLength(1);
    expect(html.match(/aria-label="PO date"/g)).toHaveLength(1);

    // The snapshot is displayed, never offered as an editable customer field.
    expect(html).not.toContain('aria-label="Customer name"');
    expect(html).not.toContain('aria-label="Currency"');
    expect(html).toContain("cannot be edited here");
  });

  it("uses a clear fallback in the hero when the customer PO number is empty (#580)", () => {
    const blank = initialPurchaseOrderEditorState(
      { ...ORDER, PONumber: "" },
      USD
    );
    const html = render(blank);
    expect(html).toContain(">New purchase order</h1>");
    expect(html).toContain("Source quotation");
    expect(html).toContain("1502260012");
  });

  it("renders the exact copied lines with quantity and money re-formatted", () => {
    const html = render();
    expect(html).toContain('aria-label="Description for line 1"');
    expect(html).toContain('aria-label="Quantity for line 1"');
    expect(html).toContain('aria-label="Unit price for line 1"');
    expect(html).toContain("Quotation item 1");
    expect(html).toContain("Quotation item 2.1");
    expect(html).toContain('value="2.5"');
    expect(html).toContain('value="$1,250.50"');
    expect(html).toContain('value="1"');
    expect(html).toContain('value="$420.00"');
  });

  it("offers Save draft and Cancel, with the calculation but no final actions", () => {
    const html = render();
    expect(buttonTagFor(html, "Save draft")).not.toContain(" disabled=");
    expect(buttonTagFor(html, "Cancel draft")).not.toContain(" disabled=");
    expect(buttonTagFor(html, "Add line")).not.toContain(" disabled=");
    // #470 owns the explained calculation; later slices own the rest, and a
    // draft must not pretend they exist.
    expect(html).toContain("Grand total");
    expect(html).toContain("Subtotal");
    expect(html).toContain("$3,233.63");
    expect(html).not.toContain("Upload PDF");
    expect(html).not.toContain("Confirm and save");
  });

  it("shows the row discount controls and the line's own figures", () => {
    const html = render();
    expect(html).toContain('aria-label="Enable a discount for line 1"');
    expect(html).toContain('aria-label="Discount percentage for line 1"');
    // Line 2 is a quotation sub-item, so its discount lives on the sub-item row.
    expect(html).toContain(
      'aria-label="Discount percentage for sub-item 1 of line 2"'
    );
    // The whole-PO discount and the signed adjustment are retired (#573).
    expect(html).not.toContain('aria-label="Whole-PO discount"');
    expect(html).not.toContain('aria-label="Adjustment"');
    expect(html).not.toContain('aria-label="Adjustment reason"');
    expect(html).toContain("−10% row discount");
    // The line's own figures are visible beside its inputs.
    expect(html).toContain("Sells for");
  });

  it("replaces the totals with a pending state while an input cannot be parsed", () => {
    const bad = purchaseOrderEditorReducer(
      initialPurchaseOrderEditorState(ORDER, USD),
      { type: "lineChanged", key: 0, patch: { quantity: "1.23456" } }
    );
    const html = render(bad);
    expect(html).toContain("Calculation pending");
    expect(html).toContain(
      "Line 1: quantity must be positive with up to 4 decimal places and within the supported range."
    );
    // The old grand total must not survive beside the parse error, and the
    // pending panel must never fabricate a zero for the calculation.
    expect(html).not.toContain("$3,233.63");
    const totalsPanel = html.slice(html.indexOf("Calculation pending"));
    expect(totalsPanel).not.toContain("$0.00");
  });

  it("prints PPN separately only when the frozen basis charges it", () => {
    const services = render();
    expect(services).not.toContain("PPN (11%)");

    const goods: PurchaseOrderDTO = {
      ...ORDER,
      TaxCategory: "SupplyOnly",
      PpnBps: 1100,
    };
    const html = render(initialPurchaseOrderEditorState(goods, USD));
    expect(html).toContain("PPN (11%)");
    expect(html).toContain("Charged on top");
    // The customer-facing grand total includes PPN on top of the price.
    expect(html).toContain("$3,589.32");
    expect(html).toContain("Printed on quotation revision 1");
  });
});

describe("PurchaseOrderEditorView dirty guard (issue #468)", () => {
  it("is clean on first render", () => {
    const html = render();
    expect(html).toContain("All changes saved");
    expect(html).not.toContain("Discard unsaved changes?");
  });

  it("marks an edited draft as unsaved without owning the confirmation", () => {
    const dirty = purchaseOrderEditorReducer(
      initialPurchaseOrderEditorState(ORDER, USD),
      { type: "lineChanged", key: 0, patch: { quantity: "3" } }
    );
    const html = render(dirty);
    expect(html).toContain("Unsaved changes");
    // The one confirmation is App's modal guard, not an inline panel inside
    // the page: two paths must never be able to show two dialogs.
    expect(html).not.toContain("Discard unsaved changes?");
  });

  it("renders the shared discard dialog with real choices", () => {
    const html = renderToString(
      <PurchaseOrderDiscardPrompt
        onKeepEditing={() => {}}
        onDiscard={() => {}}
      />
    ).replace(/<!-- -->/g, "");
    expect(html).toContain('role="alertdialog"');
    expect(html).toContain('aria-modal="true"');
    expect(html).toContain("Discard unsaved changes?");
    expect(buttonTagFor(html, "Keep editing")).not.toContain(" disabled=");
    expect(buttonTagFor(html, "Discard changes")).not.toContain(" disabled=");
  });

  it("shows a save failure without losing the entered values", () => {
    const dirty = purchaseOrderEditorReducer(
      initialPurchaseOrderEditorState(ORDER, USD),
      { type: "lineChanged", key: 0, patch: { description: "Edited widget" } }
    );
    const failed = purchaseOrderEditorReducer(dirty, {
      type: "saveFailed",
      error: "Line 1: quantity must be positive with up to 4 decimal places.",
    });
    const html = render(failed);
    expect(html).toContain("Line 1: quantity must be positive");
    expect(html).toContain("Edited widget");
  });
});
