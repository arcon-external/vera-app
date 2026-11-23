// Issue #654: the customer print document.
//
// The print is a standalone HTML string written into a new window, so it
// carries its own stylesheet and depends on nothing in the app. It reads the
// projection (`CustomerPORow`), never a line or a DTO: the same rows the card
// shows are the rows the customer prints, and the totals are the purchase
// order's own calculation, never the quotation's printed evidence.
//
// Everything textual is escaped before it enters the document. The document has
// no script, no event handler and no internal figure — no supplier cost, no
// margin, no variance, no quoted baseline, no remaining quantity.

import { formatMoney, type CurrencyInfo } from "./format";
import type { PurchaseOrderTotals } from "./purchaseOrder";
import type {
  CustomerPOHeader,
  CustomerPOLeaf,
  CustomerPORow,
} from "./poCustomerView";

/** The one placeholder for a figure the exact arithmetic refused to name. */
const NOT_AVAILABLE = "—";

/**
 * Escapes the three characters that can start markup in text content. The
 * document never interpolates a value into an attribute, so quotes need no
 * treatment; escaping them anyway would only make the printed text differ from
 * what staff typed.
 */
export function escapeHtmlText(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

/** A text field, escaped, with an honest placeholder when it is empty. */
function text(value: string | null | undefined): string {
  if (value === null || value === undefined || value.trim() === "") {
    return NOT_AVAILABLE;
  }
  return escapeHtmlText(value);
}

function money(currency: CurrencyInfo, minor: number): string {
  return formatMoney(currency, minor);
}

/** One aggregate header row: the whole group's exact figures on one line. */
function aggregateRowHtml(
  row: Extract<CustomerPORow, { kind: "aggregate" }>,
  currency: CurrencyInfo
): string {
  const unitPrice =
    row.unitPriceMinor === null
      ? NOT_AVAILABLE
      : money(currency, row.unitPriceMinor);
  return [
    '<tr class="aggregate">',
    `<td class="desc"><strong>${escapeHtmlText(row.title)}</strong></td>`,
    `<td class="num">${escapeHtmlText(row.quantity)}</td>`,
    `<td class="unit">${escapeHtmlText(row.uom)}</td>`,
    `<td class="num">${unitPrice}</td>`,
    `<td class="num">${money(currency, row.totalMinor)}</td>`,
    "</tr>",
  ].join("");
}

/** One child row: a priced line, a package heading, or a sub-sub bullet. */
function leafRowHtml(row: CustomerPOLeaf, currency: CurrencyInfo): string {
  const heading = row.depth === 0 && row.quantity.trim() === "";
  const bullet = row.depth === 2;
  const bullets =
    row.descriptionLines.length === 0
      ? ""
      : `<ul class="bullets">${row.descriptionLines
          .map((line) => `<li>${escapeHtmlText(line)}</li>`)
          .join("")}</ul>`;
  const description = `<td class="desc">${escapeHtmlText(row.description)}${bullets}</td>`;
  const cells =
    heading || bullet
      ? '<td class="num"></td><td class="unit"></td><td class="num"></td><td class="num"></td>'
      : [
          `<td class="num">${escapeHtmlText(row.quantity)}</td>`,
          `<td class="unit">${escapeHtmlText(row.uom)}</td>`,
          `<td class="num">${
            row.unitPriceMinor === null
              ? NOT_AVAILABLE
              : money(currency, row.unitPriceMinor)
          }</td>`,
          `<td class="num">${money(currency, row.amountMinor)}</td>`,
        ].join("");
  return `<tr class="depth-${row.depth}">${description}${cells}</tr>`;
}

/**
 * Splits the flat rows into table bodies. An aggregate and its children are one
 * `tbody.po-group`, which the stylesheet keeps atomic across a page break, so
 * the header can never print at the bottom of one page with its children over
 * the leaf. Ungrouped rows share a plain body and may break between rows.
 */
function lineBodies(rows: CustomerPORow[], currency: CurrencyInfo): string {
  const out: string[] = [];
  let open: "none" | "group" | "plain" = "none";
  const close = () => {
    if (open !== "none") out.push("</tbody>");
    open = "none";
  };
  const ensure = (next: "group" | "plain") => {
    if (open === next) return;
    close();
    out.push(next === "group" ? '<tbody class="po-group">' : "<tbody>");
    open = next;
  };
  for (const row of rows) {
    if (row.kind === "aggregate") {
      ensure("group");
      out.push(aggregateRowHtml(row, currency));
      for (const child of row.children ?? []) {
        out.push(leafRowHtml(child, currency));
      }
      continue;
    }
    ensure("plain");
    out.push(leafRowHtml(row, currency));
  }
  close();
  return out.join("");
}

/** The order's own subtotal, PPN when charged, and grand total. */
function totalsHtml(totals: PurchaseOrderTotals, currency: CurrencyInfo): string {
  const rows = [
    `<tr><td>Subtotal</td><td class="num">${money(currency, totals.subtotalMinor)}</td></tr>`,
  ];
  if (totals.ppnMinor !== 0) {
    rows.push(
      `<tr><td>PPN</td><td class="num">${money(currency, totals.ppnMinor)}</td></tr>`
    );
  }
  rows.push(
    `<tr class="grand"><td>Grand total</td><td class="num">${money(
      currency,
      totals.grandTotalMinor
    )}</td></tr>`
  );
  return `<section class="totals"><table><tbody>${rows.join("")}</tbody></table></section>`;
}

const STYLE = `
@page { size: A4; margin: 1.4cm; }
* { box-sizing: border-box; }
body { margin: 0; font-family: Inter, Arial, Helvetica, sans-serif; font-size: 11px; line-height: 1.45; color: #1d2939; }
.doc-kind { margin: 0; font-size: 9px; letter-spacing: 0.12em; text-transform: uppercase; color: #667085; }
h1 { margin: 2px 0 0; font-size: 20px; font-weight: 600; }
.customer { margin: 2px 0 0; font-size: 12px; color: #344054; }
.po-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px 24px; margin: 16px 0 18px; }
.po-meta div { min-width: 0; }
.po-meta dt { margin: 0; font-size: 9px; letter-spacing: 0.08em; text-transform: uppercase; color: #667085; }
.po-meta dd { margin: 0; }
table { width: 100%; border-collapse: collapse; }
thead th { padding: 6px 8px; border-bottom: 1px solid #d0d5dd; text-align: left; font-size: 9px; letter-spacing: 0.08em; text-transform: uppercase; color: #475467; }
thead th.num { text-align: right; }
td { padding: 6px 8px; border-bottom: 1px solid #eaecf0; vertical-align: top; }
td.desc { white-space: normal; overflow-wrap: anywhere; }
td.num { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
td.unit { white-space: nowrap; }
tr.depth-1 td.desc { padding-left: 30px; }
tr.depth-2 td.desc { padding-left: 46px; color: #667085; }
tr.aggregate td { background: #f9fafb; font-weight: 600; }
tbody.po-group { break-inside: avoid; page-break-inside: avoid; }
.bullets { margin: 3px 0 0; padding-left: 14px; color: #475467; }
.totals { display: flex; justify-content: flex-end; margin-top: 16px; }
.totals table { width: 260px; }
.totals td { padding: 3px 0; border: 0; }
.totals .grand td { padding-top: 6px; border-top: 1px solid #d0d5dd; font-weight: 600; }
`;

/**
 * The whole customer document. `rows` is the projection, `totals` is the
 * purchase order's own calculation or its stored figures — the order's value,
 * never the quotation's printed evidence.
 */
export function renderCustomerPrintHTML(
  rows: CustomerPORow[],
  header: CustomerPOHeader,
  totals: PurchaseOrderTotals,
  currency: CurrencyInfo
): string {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Purchase order ${escapeHtmlText(header.poNumber)}</title>
<style>${STYLE}</style>
</head>
<body>
<header>
<p class="doc-kind">Purchase order</p>
<h1>${escapeHtmlText(header.poNumber)}</h1>
<p class="customer">${escapeHtmlText(header.customerName)}</p>
<dl class="po-meta">
<div><dt>Customer PO date</dt><dd>${text(header.poDate)}</dd></div>
<div><dt>Received date</dt><dd>${text(header.receivedDate)}</dd></div>
<div><dt>Requested delivery</dt><dd>${text(header.requestedDeliveryDate)}</dd></div>
<div><dt>Quotation</dt><dd>${escapeHtmlText(header.quoteNumber)} · revision ${
    header.sourceRevision
  }</dd></div>
<div><dt>Payment terms</dt><dd>${text(header.paymentTerms)}</dd></div>
<div><dt>Incoterms</dt><dd>${text(header.incoterms)}</dd></div>
</dl>
</header>
<table class="lines">
<thead><tr><th>Description</th><th class="num">Qty</th><th>Unit</th><th class="num">Unit price</th><th class="num">Amount</th></tr></thead>
${lineBodies(rows, currency)}
</table>
${totalsHtml(totals, currency)}
</body>
</html>
`;
}
