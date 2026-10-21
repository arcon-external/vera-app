import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderHistoryEntry } from "../poCorrection";
import { PurchaseOrderHistoryPanel } from "./PurchaseOrderHistoryPanel";

// Issue #473: the readable history panel. Every value is the snapshot's, not the
// live record's: reason, actor, time, attachment version and the replaced
// header, lines and totals.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const CORRECTION: PurchaseOrderHistoryEntry = {
  UUID: "corr-1",
  Kind: "correction",
  State: "saved",
  Reason: "The customer moved to a new warehouse",
  ActorName: "Budi Santoso",
  CreatedAt: "2026-10-21T10:15:00+07:00",
  PONumber: "PO-473",
  DeliveryAddress: "Jl. Old Delivery 9",
  PaymentTerms: "30 days",
  Incoterms: "CIF",
  AdjustmentReason: "Rounded to the printed total",
  Currency: "USD",
  PreviousVersion: 4,
  ResultingVersion: 5,
  SubtotalMinor: 20000,
  DiscountMinor: 2000,
  PpnMinor: 1980,
  AdjustmentMinor: -250,
  GrandTotalMinor: 19730,
  Attachment: {
    Version: 2,
    SHA256: "d2f4a6c8e0b1".padEnd(64, "0"),
    Filename: "Customer PO 4500098123.pdf",
  },
  Items: [
    {
      LineNumber: 1,
      Description: "Widget",
      QuantityScaled: 20000,
      UoM: "Pc",
      UnitPriceMinor: 10000,
      DiscountMinor: 2000,
      NetMinor: 18000,
    },
  ],
};

const DRAFT_OVERWRITE: PurchaseOrderHistoryEntry = {
  ...CORRECTION,
  UUID: "corr-2",
  Kind: "draft_overwrite",
  State: "draft",
  Reason: "Draft overwritten after a concurrent save",
  ActorName: "Sari",
  CreatedAt: "2026-10-21T09:00:00+07:00",
  PreviousVersion: 3,
  ResultingVersion: 4,
  Attachment: null,
};

function markup(entries: PurchaseOrderHistoryEntry[]) {
  return renderToStaticMarkup(
    <PurchaseOrderHistoryPanel entries={entries} currency={USD} />
  );
}

describe("PurchaseOrderHistoryPanel (issue #473)", () => {
  it("says so when nothing has been corrected", () => {
    const html = markup([]);
    expect(html).toContain("No corrections have been recorded for this purchase order.");
    expect(html).toContain("Nothing recorded yet");
  });

  it("renders the reason, actor, time and versions of each snapshot", () => {
    const html = markup([CORRECTION, DRAFT_OVERWRITE]);
    expect(html).toContain("Correction");
    expect(html).toContain("Draft overwritten");
    expect(html).toContain("The customer moved to a new warehouse");
    expect(html).toContain("Draft overwritten after a concurrent save");
    expect(html).toContain("Budi Santoso");
    expect(html).toContain("Sari");
    expect(html).toContain("2026-10-21 10:15 +07:00");
    expect(html).toContain("Version 4 → 5");
    expect(html).toContain("Version 3 → 4");
  });

  it("renders the attachment reference the snapshot kept", () => {
    const html = markup([CORRECTION, DRAFT_OVERWRITE]);
    expect(html).toContain("Customer PO PDF v2 · Customer PO 4500098123.pdf");
    expect(html).toContain("sha256 d2f4a6c8e0b1…");
    expect(html).toContain("No customer PO PDF was attached at that moment.");
  });

  it("renders the replaced header, lines and totals", () => {
    const html = markup([CORRECTION]);
    expect(html).toContain("Previous values");
    expect(html).toContain("PO-473");
    expect(html).toContain("Jl. Old Delivery 9");
    expect(html).toContain("Rounded to the printed total");
    expect(html).toContain("Widget");
    expect(html).toContain("$100.00");
    expect(html).toContain("$180.00");
    expect(html).toContain("$200.00");
    expect(html).toContain("$197.30");
  });

  it("reports a history read failure instead of an empty success", () => {
    const html = renderToStaticMarkup(
      <PurchaseOrderHistoryPanel
        entries={[]}
        currency={USD}
        error="purchase order not found"
      />
    );
    expect(html).toContain("purchase order not found");
    expect(html).not.toContain("No corrections have been recorded");
  });
});
