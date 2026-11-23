import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import type { PurchaseOrderTotals } from "./purchaseOrder";
import {
  projectCustomerRows,
  type CustomerPOHeader,
  type CustomerPORow,
} from "./poCustomerView";
import { renderCustomerPrintHTML } from "./poCustomerPrint";

// Issue #654: the customer print document.
//
// The print is a standalone HTML string written into a new window: no server,
// no app stylesheet, no scripts. It carries the order the customer reads — PO
// number, customer, dates, quotation reference, payment terms and incoterms,
// then description / qty / unit / unit price / amount, then the purchase
// order's own subtotal, PPN (only when charged) and grand total.
//
// Everything textual is escaped. Nothing internal — supplier cost, margin,
// variance, the quoted baseline or the remaining quantity — may reach the
// document.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const HEADER: CustomerPOHeader = {
  poNumber: "PO-654",
  customerName: "PT. Buyer Nusantara",
  poDate: "2026-11-20",
  receivedDate: "2026-11-21",
  requestedDeliveryDate: "2026-12-05",
  quoteNumber: "1502260100",
  sourceRevision: 3,
  paymentTerms: "30 days after invoice",
  incoterms: "CIF Jakarta",
};

const TOTALS: PurchaseOrderTotals = {
  grossMinor: 4450,
  rowDiscountMinor: 50,
  subtotalMinor: 4450,
  discountMinor: 0,
  afterDiscountMinor: 4450,
  ppnMinor: 489,
  grandTotalMinor: 4939,
};

function leaf(overrides: Partial<Extract<CustomerPORow, { kind: "line" }>> = {}) {
  return {
    kind: "line" as const,
    depth: 0 as const,
    description: "Widget",
    descriptionLines: [],
    quantity: "2",
    uom: "Pc",
    unitPriceMinor: 1000,
    amountMinor: 2000,
    ...overrides,
  };
}

function aggregate(
  overrides: Partial<Extract<CustomerPORow, { kind: "aggregate" }>> = {}
) {
  return {
    kind: "aggregate" as const,
    title: "Combined items",
    quantity: "2",
    uom: "Lot",
    unitPriceMinor: 2225,
    totalMinor: 4450,
    children: null,
    ...overrides,
  };
}

function render(rows: CustomerPORow[], totals: PurchaseOrderTotals = TOTALS) {
  return renderCustomerPrintHTML(rows, HEADER, totals, USD);
}

describe("renderCustomerPrintHTML: the order", () => {
  it("heads the document with the PO number, customer, dates, quotation reference, payment terms and incoterms", () => {
    const html = render([leaf()]);
    for (const value of [
      "PO-654",
      "PT. Buyer Nusantara",
      "2026-11-20",
      "2026-11-21",
      "2026-12-05",
      "1502260100",
      "revision 3",
      "30 days after invoice",
      "CIF Jakarta",
    ]) {
      expect(html).toContain(value);
    }
  });

  it("renders the description, quantity, unit, unit price and amount columns", () => {
    const html = render([leaf()]);
    expect(html).toContain("Description");
    expect(html).toContain("Qty");
    expect(html).toContain("Unit");
    expect(html).toContain("Unit price");
    expect(html).toContain("Amount");
    expect(html).toContain("$10.00");
    expect(html).toContain("$20.00");
  });

  it("shows subtotal, PPN only when nonzero, and the grand total", () => {
    const html = render([leaf()]);
    expect(html).toContain("Subtotal");
    expect(html).toContain("PPN");
    expect(html).toContain("$44.50");
    expect(html).toContain("$4.89");
    expect(html).toContain("Grand total");
    expect(html).toContain("$49.39");
  });

  it("omits the PPN line entirely when no PPN is charged", () => {
    const html = render([leaf()], {
      ...TOTALS,
      ppnMinor: 0,
      grandTotalMinor: 4450,
    });
    expect(html).not.toContain("PPN");
    expect(html).toContain("$44.50");
  });

  it("never shows a quantity or amount the purchase order did not calculate", () => {
    const html = render([leaf()]);
    // The totals section is the only place a code-side sum may appear; the
    // line amount comes from the projected row.
    expect(html).toContain("$20.00");
    expect(html).not.toContain("$9,999");
  });
});

describe("renderCustomerPrintHTML: aggregates", () => {
  it("prints a title-only aggregate as one bold row with its display quantity, unit, derived price and exact total", () => {
    const html = render([aggregate()]);
    expect(html).toContain("Combined items");
    expect(html).toContain("$22.25");
    expect(html).toContain("$44.50");
    // Title only means the child descriptions are absent from the whole
    // document, not merely hidden.
    expect(html).not.toContain("Widget");
    expect(html).not.toContain("Panel");
    expect(html).toContain('class="aggregate"');
  });

  it("prints the child tree under its header when the toggle emitted it", () => {
    const html = render([
      aggregate({
        children: [
          leaf({ description: "Widget", quantity: "2", unitPriceMinor: 1000, amountMinor: 2000 }),
          leaf({
            depth: 1,
            description: "Panel",
            quantity: "1",
            unitPriceMinor: 500,
            amountMinor: 450,
          }),
          leaf({
            depth: 2,
            description: "Mild steel",
            quantity: "",
            uom: "",
            unitPriceMinor: null,
            amountMinor: 0,
          }),
        ],
      }),
    ]);
    expect(html.indexOf("Combined items")).toBeLessThan(html.indexOf("Widget"));
    expect(html.indexOf("Widget")).toBeLessThan(html.indexOf("Panel"));
    expect(html.indexOf("Panel")).toBeLessThan(html.indexOf("Mild steel"));
    expect(html).toContain('class="depth-0"');
    expect(html).toContain('class="depth-1"');
    expect(html).toContain('class="depth-2"');
  });

  it("marks an aggregate's rows atomic across a page break", () => {
    const html = render([aggregate({ children: [leaf({ description: "Child" })] })]);
    expect(html).toMatch(/tbody\.po-group[^}]*break-inside:\s*avoid/s);
  });
});

describe("renderCustomerPrintHTML: safety", () => {
  const html = () =>
    render([
      aggregate({
        title: 'Group & <script>alert("x")</script>',
        children: [
          leaf({
            description: "Gear & <b>pin</b>",
            descriptionLines: ["A < 5 > 2"],
          }),
        ],
      }),
    ]);

  it("escapes every &, < and > in text", () => {
    const document = html();
    expect(document).toContain("Group &amp; &lt;script&gt;");
    expect(document).toContain("Gear &amp; &lt;b&gt;pin&lt;/b&gt;");
    expect(document).toContain("A &lt; 5 &gt; 2");
  });

  it("contains no script element and no event handler", () => {
    const document = html();
    expect(document.toLowerCase()).not.toContain("<script");
    expect(document.toLowerCase()).not.toContain("onload=");
    expect(document.toLowerCase()).not.toContain("javascript:");
  });

  it("is a standalone A4 document", () => {
    const document = html();
    expect(document).toContain("<!doctype html>");
    expect(document).toContain('charset="utf-8"');
    expect(document).toMatch(/@page\s*\{[^}]*size:\s*A4/s);
    expect(document).toMatch(/break-inside:\s*avoid/);
  });

  it("wraps long descriptions and keeps money on one line", () => {
    const document = html();
    expect(document).toMatch(/td\.desc[^}]*overflow-wrap:\s*anywhere/s);
    expect(document).toMatch(/white-space:\s*nowrap/s);
  });

  it("prints an unavailable unit price as an em dash, never a made-up figure", () => {
    const document = render([
      aggregate({ unitPriceMinor: null, totalMinor: 4450 }),
    ]);
    expect(document).toContain("—");
  });

  it("contains no supplier cost, margin, variance, quoted baseline or remaining quantity", () => {
    // The stylesheet legitimately names the CSS `margin` property, so the
    // guard reads the document without it: a leaked field lives in the text.
    const document = html()
      .replace(/<style>[\s\S]*?<\/style>/i, "")
      .toLowerCase();
    for (const word of ["cost", "margin", "variance", "pph", "quoted", "remaining", "profit"]) {
      expect(document).not.toContain(word);
    }
  });
});

describe("renderCustomerPrintHTML: composition with the projection", () => {
  it("prints the projected rows of a real order", () => {
    const rows = projectCustomerRows(
      [
        {
          key: 1,
          clientKey: "client:1",
          description: "Bracket",
          quantity: "3",
          uom: "Pcs",
          unitPrice: "$2.50",
          discountEnabled: false,
          discountPercent: "0",
          quotationItemNo: 1,
          quotationSubNo: null,
          lineKind: "quotation",
          groupKey: "",
          parentDescription: "",
          descriptionLines: [],
          aggregateKey: 0,
        },
      ],
      [
        {
          key: 0,
          title: "Brackets lot",
          orderedQuantity: "3",
          orderedUoM: "Pcs",
          showChildrenToCustomer: true,
        },
      ],
      USD
    );
    const document = render(rows, {
      ...TOTALS,
      subtotalMinor: 750,
      grossMinor: 750,
      rowDiscountMinor: 0,
      ppnMinor: 0,
      grandTotalMinor: 750,
    });
    expect(document).toContain("Brackets lot");
    expect(document).toContain("Bracket");
    expect(document).toContain("$7.50");
  });
});
