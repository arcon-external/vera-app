import { useState } from "react";
import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";

// U6: derived totals check. Read-only evidence, never a decision item and
// never a target. Compares every applicable stage (subtotal, discount, PPN,
// grand total) so offsetting stage differences cannot read Matched. The
// whole-PO discount is retired (#573), so the PO side is always zero and a
// printed discount still shows as a difference. Differences are evidence in
// neutral ink, never danger red, and never block Final Save.

export interface TotalsStageFigures {
  subtotalMinor: number;
  discountMinor: number;
  ppnMinor: number;
  grandTotalMinor: number;
}

export interface TotalsPrintedFigures {
  subtotalMinor: number;
  discountMinor: number;
  ppnMinor: number;
  grandTotalMinor: number;
}

export interface TotalsStripProps {
  /** The PO's own calculated stages, or null while any input is invalid. */
  calculated: TotalsStageFigures | null;
  /** The printed source stages, or null when no live source is linked. */
  printed: TotalsPrintedFigures | null;
  /** First differing input, or the R-09 fallback. Null while pending. */
  reason: string | null;
  currency: CurrencyInfo;
}

type StripStatus = "matched" | "different" | "pending" | "unavailable";

function stageStatus(calculated: TotalsStageFigures | null, printed: TotalsPrintedFigures | null): StripStatus {
  if (printed === null) return "unavailable";
  if (calculated === null) return "pending";
  const same =
    calculated.subtotalMinor === printed.subtotalMinor &&
    calculated.discountMinor === printed.discountMinor &&
    calculated.ppnMinor === printed.ppnMinor &&
    calculated.grandTotalMinor === printed.grandTotalMinor;
  return same ? "matched" : "different";
}

function money(currency: CurrencyInfo, value: number | null): string {
  if (value === null) return "(pending)";
  return formatMoney(currency, value);
}

export function ReconciliationTotalsStrip(props: TotalsStripProps) {
  const { calculated, printed, reason, currency } = props;
  const [expanded, setExpanded] = useState(false);
  const status = stageStatus(calculated, printed);
  const statusText =
    status === "matched"
      ? "Matches the printed totals"
      : status === "different"
        ? "Different"
        : status === "pending"
          ? "Pending"
          : "Unavailable";
  const statusClass =
    status === "matched"
      ? "text-success-700"
      : status === "different"
        ? "text-ink-700"
        : "text-ink-500";
  // A stage row appears only when it applies: either side non-zero or the
  // stages differ. Grand total always shows. Adjustment is PO-only context.
  const rows: { key: string; label: string; po: number | null; printed: number | null; strong?: boolean }[] = [
    { key: "subtotal", label: "Subtotal", po: calculated?.subtotalMinor ?? null, printed: printed?.subtotalMinor ?? null },
    { key: "discount", label: "Discount", po: calculated?.discountMinor ?? null, printed: printed?.discountMinor ?? null },
    { key: "ppn", label: "PPN", po: calculated?.ppnMinor ?? null, printed: printed?.ppnMinor ?? null },
    { key: "grand", label: "Grand total", po: calculated?.grandTotalMinor ?? null, printed: printed?.grandTotalMinor ?? null, strong: true },
  ].filter((row) => row.key === "grand" || row.po !== 0 || row.printed !== 0 || (row.po ?? 0) !== (row.printed ?? 0));
  return (
    <section aria-label="Totals check">
      <button
        type="button"
        aria-expanded={expanded}
        aria-label={`Totals check, ${statusText}. ${expanded ? "Collapse" : "Show stages"}`}
        onClick={() => setExpanded((current) => !current)}
        className="flex min-h-11 w-full cursor-pointer items-center justify-between gap-2 text-left focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-500/40"
      >
        <span className="text-small font-medium text-ink-900">Totals check</span>
        <span className="flex items-baseline gap-2 text-small">
          <span className="tnum text-ink-700">
            {printed === null ? "—" : money(currency, calculated?.grandTotalMinor ?? null)}
            {" · "}
            {printed === null ? "—" : formatMoney(currency, printed.grandTotalMinor)}
          </span>
          <span className={`text-caption font-medium ${statusClass}`}>{statusText}</span>
        </span>
      </button>
      {expanded ? (
        <dl className="mt-1 grid gap-1 text-small">
          <div className="grid grid-cols-[1fr_auto_auto] gap-x-3 text-caption text-ink-500">
            <span aria-hidden="true" />
            <span>PO</span>
            <span>Printed</span>
          </div>
          {rows.map((row) => (
            <div key={row.key} className="grid grid-cols-[1fr_auto_auto] items-baseline gap-x-3">
              <dt className={row.strong === true ? "text-small font-semibold text-ink-900" : "text-ink-500"}>
                {row.label}
              </dt>
              <dd className={`tnum ${row.strong === true ? "font-semibold text-ink-900" : "text-ink-700"}`}>
                {money(currency, row.po)}
              </dd>
              <dd className={`tnum ${row.strong === true ? "font-semibold text-ink-900" : "text-ink-700"}`}>
                {row.printed === null ? "—" : formatMoney(currency, row.printed)}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
      {status === "different" && reason !== null ? (
        <p className="mt-1 text-caption text-ink-500">Different · {reason}.</p>
      ) : status === "different" ? (
        <p className="mt-1 text-caption text-ink-500">
          Different · the inputs do not fully explain it.
        </p>
      ) : status === "pending" ? (
        <p className="mt-1 text-caption text-ink-500">Totals check pending. Fix the highlighted input.</p>
      ) : status === "unavailable" ? (
        <p className="mt-1 text-caption text-ink-500">
          Totals check unavailable until a Quotation is linked.
        </p>
      ) : null}
      <p className="mt-1 text-caption text-ink-500">
        Derived only: the printed total is evidence, never a target.
      </p>
    </section>
  );
}
