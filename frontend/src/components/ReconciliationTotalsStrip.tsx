import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";

// Ticket 05: the derived totals check strip. Read-only evidence, never a
// decision item and never a target: it compares the PO's own calculation with
// the printed source figures and names the first differing input (or the R-09
// fallback when no input explains the printed total).

export interface TotalsStripProps {
  /** The PO's own calculated grand total, or null while any input is invalid. */
  calculatedMinor: number | null;
  /** The printed source grand total. */
  printedMinor: number;
  /** First differing input, or the R-09 fallback. Null while pending. */
  reason: string | null;
  currency: CurrencyInfo;
}

export function ReconciliationTotalsStrip(props: TotalsStripProps) {
  const { calculatedMinor, printedMinor, reason, currency } = props;
  const status = calculatedMinor === null ? "pending" : calculatedMinor === printedMinor ? "matched" : "different";
  const pill =
    status === "matched" ? (
      <span className="inline-flex items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
        Matched
      </span>
    ) : status === "different" ? (
      <span className="inline-flex items-center rounded-md border border-danger-200 bg-danger-50 px-2 py-0.5 text-micro font-medium uppercase text-danger-700">
        Different
      </span>
    ) : (
      <span className="inline-flex items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
        Pending
      </span>
    );
  return (
    <section aria-label="Totals check">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-small font-medium text-ink-900">Totals check</h3>
        {pill}
      </div>
      <dl className="mt-1.5 grid gap-1 text-small">
        <div className="flex items-baseline justify-between gap-2">
          <dt className="text-ink-500">Purchase order total</dt>
          <dd className="tnum font-medium text-ink-900">
            {calculatedMinor === null ? "(pending)" : formatMoney(currency, calculatedMinor)}
          </dd>
        </div>
        <div className="flex items-baseline justify-between gap-2">
          <dt className="text-ink-500">Printed quotation total</dt>
          <dd className="tnum font-medium text-ink-900">{formatMoney(currency, printedMinor)}</dd>
        </div>
      </dl>
      {status === "different" && reason !== null ? (
        <p className="mt-1 text-caption text-ink-500">Explaining input: {reason}.</p>
      ) : status === "different" ? (
        <p className="mt-1 text-caption text-ink-500">No explaining input recorded.</p>
      ) : status === "pending" ? (
        <p className="mt-1 text-caption text-ink-500">Complete every amount to check the totals.</p>
      ) : null}
      <p className="mt-1 text-caption text-ink-500">
        Derived only: the printed total is evidence, never a target.
      </p>
    </section>
  );
}
