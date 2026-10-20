import type {
  PurchaseOrderPrintedTotals,
  PurchaseOrderTotals as PurchaseOrderTotalsValue,
} from "../purchaseOrder";
import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import { Card, SectionHeading } from "./ui";

// Issue #470: the explained calculation, shown as stages rather than a single
// number. Every figure is customer-facing: line gross, row discounts, the
// whole-PO discount, the PPN that will be printed on top, a signed adjustment
// with its reason, and the grand total. The source quotation's printed figures
// sit in their own group, because Vera's arithmetic must never replace the
// evidence it is checked against (parent story 45).
//
// Internal cost, margin, PPH22 and profit are absent by construction: the
// calculation that feeds this component carries no such field.

export interface PurchaseOrderTotalsProps {
  /** `null` while any input cannot be parsed; the panel then says pending. */
  totals: PurchaseOrderTotalsValue | null;
  currency: CurrencyInfo;
  /** `null` while the whole-PO discount field cannot be parsed. */
  discountPercent: number | null;
  ppnBps: number;
  adjustmentReason: string;
  printedTotals: PurchaseOrderPrintedTotals;
  sourceRevision: number;
}

function printedTotalsAvailable(printed: PurchaseOrderPrintedTotals): boolean {
  return [
    printed.subtotalMinor,
    printed.discountMinor,
    printed.ppnMinor,
    printed.grandTotalMinor,
  ].every((value) => Number.isSafeInteger(value) && value >= 0);
}

function formatBps(bps: number): string {
  return `${(bps / 100).toLocaleString("id-ID", { maximumFractionDigits: 2 })}%`;
}

function Row({
  label,
  value,
  emphasis = false,
  muted = false,
  note,
}: {
  label: string;
  value: string;
  emphasis?: boolean;
  muted?: boolean;
  note?: string;
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1">
      <dt className={`text-small ${muted ? "text-ink-700" : "text-ink-900"}`}>
        {label}
        {note !== undefined ? (
          <span className="ml-2 text-caption text-ink-400">{note}</span>
        ) : null}
      </dt>
      <dd
        className={`tnum whitespace-nowrap text-right ${
          emphasis ? "text-lead font-semibold text-ink-950" : "text-small text-ink-900"
        }`}
      >
        {value}
      </dd>
    </div>
  );
}

export function PurchaseOrderTotals({
  totals,
  currency,
  discountPercent,
  ppnBps,
  adjustmentReason,
  printedTotals,
  sourceRevision,
}: PurchaseOrderTotalsProps) {
  const showRowDiscounts = totals !== null && totals.rowDiscountMinor > 0;
  const showWholeDiscount = totals !== null && totals.discountMinor > 0;
  const showPpn = ppnBps > 0;
  const showAdjustment = totals !== null && totals.adjustmentMinor !== 0;
  const printedAvailable = printedTotalsAvailable(printedTotals);
  return (
    <Card className="p-4">
      <SectionHeading title="Totals" hint="Calculated from the entered lines" />
      {totals === null ? (
        <p
          role="status"
          className="mt-2 rounded-lg border border-dashed border-rule px-3 py-4 text-small text-ink-500"
        >
          Calculation pending. Fix the highlighted field to see the totals.
        </p>
      ) : (
        <dl className="mt-2 divide-y divide-rule">
          <Row label="Line totals" value={formatMoney(currency, totals.grossMinor)} />
          {showRowDiscounts ? (
            <Row
              label="Row discounts"
              muted
              value={formatMoney(currency, -totals.rowDiscountMinor)}
            />
          ) : null}
          <Row label="Subtotal" value={formatMoney(currency, totals.subtotalMinor)} />
          {showWholeDiscount ? (
            <Row
              label={
                discountPercent === null
                  ? "Whole-PO discount"
                  : `Whole-PO discount (${discountPercent}%)`
              }
              muted
              value={formatMoney(currency, -totals.discountMinor)}
            />
          ) : null}
          <Row
            label="Net after discount"
            value={formatMoney(currency, totals.afterDiscountMinor)}
          />
          {showPpn ? (
            <Row
              label={`PPN (${formatBps(ppnBps)})`}
              muted
              note="Charged on top"
              value={formatMoney(currency, totals.ppnMinor)}
            />
          ) : null}
          {showAdjustment ? (
            <Row
              label="Adjustment"
              muted
              note={adjustmentReason}
              value={formatMoney(currency, totals.adjustmentMinor)}
            />
          ) : null}
          <Row
            label="Grand total"
            emphasis
            value={formatMoney(currency, totals.grandTotalMinor)}
          />
        </dl>
      )}

      <div className="mt-4 border-t border-rule pt-3">
        <p className="text-caption font-medium uppercase tracking-wide text-ink-500">
          Printed on quotation revision {sourceRevision}
        </p>
        {printedAvailable ? (
          <dl className="mt-1">
            <Row
              label="Printed subtotal"
              value={formatMoney(currency, printedTotals.subtotalMinor)}
            />
            {printedTotals.discountMinor > 0 ? (
              <Row
                label="Printed discount"
                value={formatMoney(currency, -printedTotals.discountMinor)}
              />
            ) : null}
            {printedTotals.ppnMinor > 0 ? (
              <Row label="Printed PPN" value={formatMoney(currency, printedTotals.ppnMinor)} />
            ) : null}
            <Row
              label="Printed grand total"
              value={formatMoney(currency, printedTotals.grandTotalMinor)}
            />
          </dl>
        ) : (
          <p className="mt-1 text-small text-ink-500">Printed totals unavailable.</p>
        )}
        <p className="mt-2 text-caption text-ink-400">
          Source-document evidence, kept as printed. Differences are reviewed
          before the order is finalized.
        </p>
      </div>
    </Card>
  );
}
