import { useId, useState } from "react";
import { formatMoney, type CurrencyInfo } from "../format";
import type {
  PurchaseOrderAggregateGroupState,
  PurchaseOrderLine,
  PurchaseOrderTotals,
} from "../purchaseOrder";
import { renderCustomerPrintHTML } from "../poCustomerPrint";
import {
  projectCustomerRows,
  type CustomerPOAggregate,
  type CustomerPOHeader,
  type CustomerPOLeaf,
} from "../poCustomerView";
import { Button, Card, SectionHeading, StatusBanner } from "./ui";

// Issue #654: the customer view card.
//
// The same projection the print document consumes is rendered here, so staff
// see exactly what the customer will see. The card owns no figure of its own:
// the aggregate title-only/child-tree choice is the one stored on the aggregate,
// and the totals are the purchase order's own. Printing opens a blank window,
// writes the standalone document and calls print — there is no server call and
// no second document engine.

export interface PurchaseOrderCustomerViewProps {
  /** The order facts the document carries, normalized by the mounting view. */
  header: CustomerPOHeader;
  /** The entered rows, in display order, with aggregate membership. */
  lines: PurchaseOrderLine[];
  aggregateGroups: PurchaseOrderAggregateGroupState[];
  /**
   * The purchase order's own totals — never null once every line can be
   * totalled. A pending draft shows why, and print stays unavailable.
   */
  totals: PurchaseOrderTotals | null;
  /** The calculation's first problem, when the caller has it. */
  totalsError?: string | null;
  currency: CurrencyInfo;
}

/** A heading row carries no numbers; a priced row carries all of them. */
function isHeading(row: CustomerPOLeaf): boolean {
  return row.depth === 2 || (row.depth === 0 && row.quantity.trim() === "");
}

function unitPriceText(row: CustomerPOLeaf, currency: CurrencyInfo): string {
  if (isHeading(row)) return "";
  return row.unitPriceMinor === null
    ? "—"
    : formatMoney(currency, row.unitPriceMinor);
}

function InAppRow({
  row,
  currency,
}: {
  row: CustomerPOLeaf;
  currency: CurrencyInfo;
}) {
  const heading = isHeading(row);
  const indent = row.depth === 1 ? "pl-8" : row.depth === 2 ? "pl-12" : "";
  const tone = row.depth === 2 ? "text-ink-500" : "text-ink-900";
  return (
    <tr className="border-b border-rule last:border-b-0">
      <td className={`px-3 py-2 align-top ${indent} ${tone}`}>
        {row.description || "-"}
        {row.descriptionLines.length > 0 ? (
          <ul className="mt-1 space-y-0.5 text-caption text-ink-500">
            {row.descriptionLines.map((line, index) => (
              <li key={index}>{line || "-"}</li>
            ))}
          </ul>
        ) : null}
      </td>
      <td className="tnum px-3 py-2 text-right align-top">
        {heading ? "" : row.quantity}
      </td>
      <td className="px-3 py-2 align-top">{heading ? "" : row.uom}</td>
      <td className="tnum whitespace-nowrap px-3 py-2 text-right align-top">
        {unitPriceText(row, currency)}
      </td>
      <td className="tnum whitespace-nowrap px-3 py-2 text-right align-top">
        {heading ? "" : formatMoney(currency, row.amountMinor)}
      </td>
    </tr>
  );
}

function InAppTable({
  lines,
  aggregateGroups,
  currency,
}: {
  lines: PurchaseOrderLine[];
  aggregateGroups: PurchaseOrderAggregateGroupState[];
  currency: CurrencyInfo;
}) {
  const rows = projectCustomerRows(lines, aggregateGroups, currency);
  return (
    <table className="mt-3 w-full text-left text-small">
      <caption className="sr-only">Customer view of the purchase order lines</caption>
      <thead>
        <tr className="border-b border-rule text-micro uppercase tracking-[0.1em] text-ink-500">
          <th className="px-3 py-2 font-medium">Description</th>
          <th className="px-3 py-2 text-right font-medium">Qty</th>
          <th className="px-3 py-2 font-medium">Unit</th>
          <th className="px-3 py-2 text-right font-medium">Unit price</th>
          <th className="px-3 py-2 text-right font-medium">Amount</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row, index) =>
          row.kind === "aggregate" ? (
            <AggregateRowGroup key={`aggregate-${index}`} row={row} currency={currency} />
          ) : (
            <InAppRow key={`line-${index}`} row={row} currency={currency} />
          )
        )}
      </tbody>
    </table>
  );
}

/** One aggregate: its exact header row, then the children the toggle allowed. */
function AggregateRowGroup({
  row,
  currency,
}: {
  row: CustomerPOAggregate;
  currency: CurrencyInfo;
}) {
  return (
    <>
      <tr className="border-b border-rule bg-paper-subtle/50">
        <td className="px-3 py-2 align-top font-semibold text-ink-950">
          {row.title}
        </td>
        <td className="tnum px-3 py-2 text-right align-top">{row.quantity}</td>
        <td className="px-3 py-2 align-top">{row.uom}</td>
        <td className="tnum whitespace-nowrap px-3 py-2 text-right align-top">
          {row.unitPriceMinor === null ? "—" : formatMoney(currency, row.unitPriceMinor)}
        </td>
        <td className="tnum whitespace-nowrap px-3 py-2 text-right align-top font-semibold">
          {formatMoney(currency, row.totalMinor)}
        </td>
      </tr>
      {(row.children ?? []).map((child, index) => (
        <InAppRow key={`child-${index}`} row={child} currency={currency} />
      ))}
    </>
  );
}

export function PurchaseOrderCustomerView({
  header,
  lines,
  aggregateGroups,
  totals,
  totalsError = null,
  currency,
}: PurchaseOrderCustomerViewProps) {
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const bodyId = useId();

  const printCustomerView = () => {
    if (totals === null) return;
    const html = renderCustomerPrintHTML(
      projectCustomerRows(lines, aggregateGroups, currency),
      header,
      totals,
      currency
    );
    const target = window.open("", "_blank");
    if (target === null) {
      setMessage(
        "The print window was blocked. Allow pop-ups for Vera, then try again."
      );
      return;
    }
    target.document.open();
    target.document.write(html);
    target.document.close();
    target.focus();
    target.print();
    setMessage("The customer view was sent to the print dialog.");
  };

  return (
    <Card className="p-4" data-testid="po-customer-view">
      <SectionHeading title="Customer view" hint="What the customer sees" />
      <p className="mt-1 max-w-3xl text-caption text-ink-500">
        A presentation of the order as entered, built from the same values that
        print. It is not the customer&apos;s own document: the customer PO PDF
        remains the evidence for this record.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          aria-label="Print customer view"
          disabled={totals === null}
          onClick={printCustomerView}
        >
          Print customer view
        </Button>
        <Button
          size="sm"
          aria-label={open ? "Collapse customer view" : "Expand customer view"}
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={() => setOpen((value) => !value)}
        >
          {open ? "Collapse" : "Expand"}
        </Button>
      </div>
      {message !== null ? (
        <p role="status" className="mt-2 text-caption text-ink-600">
          {message}
        </p>
      ) : null}
      <div id={bodyId}>
        {open ? (
          totals === null ? (
            <div className="mt-3">
              <StatusBanner tone="info">
                {totalsError ??
                  "The customer view appears once every line can be totalled."}
              </StatusBanner>
            </div>
          ) : (
            <>
              <InAppTable
                lines={lines}
                aggregateGroups={aggregateGroups}
                currency={currency}
              />
              <dl className="mt-3 flex flex-col items-end gap-1 text-small">
                <div className="flex w-64 justify-between gap-4">
                  <dt className="text-ink-500">Subtotal</dt>
                  <dd className="tnum text-ink-900">
                    {formatMoney(currency, totals.subtotalMinor)}
                  </dd>
                </div>
                {totals.ppnMinor !== 0 ? (
                  <div className="flex w-64 justify-between gap-4">
                    <dt className="text-ink-500">PPN</dt>
                    <dd className="tnum text-ink-900">
                      {formatMoney(currency, totals.ppnMinor)}
                    </dd>
                  </div>
                ) : null}
                <div className="flex w-64 justify-between gap-4 border-t border-rule pt-1">
                  <dt className="font-medium text-ink-900">Grand total</dt>
                  <dd className="tnum font-semibold text-ink-950">
                    {formatMoney(currency, totals.grandTotalMinor)}
                  </dd>
                </div>
              </dl>
            </>
          )
        ) : null}
      </div>
    </Card>
  );
}
