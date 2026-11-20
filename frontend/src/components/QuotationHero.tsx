import { formatMoney, type CurrencyInfo } from "../format";
import {
  SERVICE_CATEGORY_OPTIONS,
  TAX_TYPE_OPTIONS,
  TAX_ZONE_OPTIONS,
} from "../tax";
import type { DetailsState } from "./DetailsPanel";

/**
 * Issue #622: the details fields the hero mirrors, taken straight from the
 * editor's state so a field rename cannot drift through hand-kept props.
 */
export type QuotationHeroDetails = Pick<
  DetailsState,
  "validity" | "paymentTerm" | "incoterm" | "taxZone" | "taxType" | "serviceCategory"
>;

/**
 * Issue #623: the Quotation editor's dark hero, matching the Purchase Order
 * editor's card. It is a read-only mirror of the editor state: every value is
 * a prop, it owns no state and renders no control, so the form stays the only
 * editing surface.
 *
 * The customer total is the one `computePricing` produced for the Summary
 * rail -- the hero never prices anything itself -- and it prints a dash while
 * the live currency entry is unresolved, rather than guessing a denomination.
 */
export interface QuotationHeroProps {
  customerName: string;
  /** The details fields the hero mirrors (issue #622). */
  details: QuotationHeroDetails;
  /** The profile currency code, shown beside the Incoterm. */
  currencyCode: string;
  /** The live currency entry; null while unresolved. */
  currency: CurrencyInfo | null;
  /** Customer grand total in minor units (PPN included when charged). */
  customerGrand: number;
  /** PPN included in `customerGrand`; 0 when the tax basis charges none. */
  ppnAmount: number;
  /** The issued quotation number, or null for a new or saved draft. */
  quoteNumber?: string | null;
  /** The revision Export will create; the current revision is this minus one. */
  nextRevision?: number | null;
  /** A saved draft's relative timestamp ("just now"), or null when unsaved. */
  draftSavedLabel?: string | null;
  /** Whether Prepare has produced an exportable review. */
  ready?: boolean;
  /**
   * Replaces the derived status line. The practice tutorial passes its
   * sandbox copy here: its draft is never saved, so the default "Save draft
   * stores it" line would promise something the tour cannot do.
   */
  statusOverride?: string | null;
}

function optionLabel(
  options: readonly { value: string; label: string }[],
  value: string
): string {
  return options.find((option) => option.value === value)?.label ?? value;
}

function padRevision(revision: number): string {
  return String(revision).padStart(2, "0");
}

export function QuotationHero({
  customerName,
  details,
  currencyCode,
  currency,
  customerGrand,
  ppnAmount,
  quoteNumber = null,
  nextRevision = null,
  draftSavedLabel = null,
  ready = false,
  statusOverride = null,
}: QuotationHeroProps) {
  const { validity, paymentTerm, incoterm, taxZone, taxType, serviceCategory } =
    details;
  const number = quoteNumber?.trim() ?? "";
  const currentRevision = nextRevision === null ? null : nextRevision - 1;
  // Issue #622: a record is issued because a stored revision exists, not
  // because its number is non-empty. `nextRevision` is null only for a new or
  // saved draft, so its presence is the whole signal -- and an issued record
  // whose number went missing still reads as issued (the title falls back to
  // "Quotation" rather than announcing a new draft).
  const issued = currentRevision !== null;

  const taxLabel =
    [
      optionLabel(TAX_ZONE_OPTIONS, taxZone),
      optionLabel(TAX_TYPE_OPTIONS, taxType),
      taxType === "Services"
        ? optionLabel(SERVICE_CATEGORY_OPTIONS, serviceCategory)
        : "",
    ]
      .filter((part) => part !== "")
      .join(" · ") || "-";

  const status =
    statusOverride ??
    (issued
      ? `Revision ${padRevision(currentRevision)} stays as issued. Export creates revision ${padRevision(
          currentRevision + 1
        )}.`
      : draftSavedLabel
        ? `Draft saved ${draftSavedLabel}. Save again to update it.`
        : ready
          ? "Ready to export. Review the three stages, then export."
          : "Not saved yet. Save draft stores it so you can continue later.");

  return (
    <section
      aria-label="Quotation summary"
      data-testid="quotation-hero"
      className="relative overflow-hidden rounded-[14px] bg-accent-800 px-5 py-5 text-white shadow-card sm:px-6 sm:py-6"
    >
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -bottom-8 right-4 select-none text-[122px] font-bold leading-none tracking-[-0.08em] text-white/5"
      >
        QT
      </div>
      <div className="relative z-10 flex flex-wrap items-center justify-between gap-3">
        <span className="text-micro uppercase tracking-[0.18em] text-accent-200">
          Quotation
        </span>
        <div className="flex flex-wrap items-center gap-2">
          {issued ? (
            <>
              <span className="rounded-md border border-white/25 bg-white/10 px-2 py-0.5 text-micro uppercase tracking-wide">
                {`CURRENT REV ${padRevision(currentRevision)}`}
              </span>
              <span className="rounded-md border border-white/40 bg-white/15 px-2 py-0.5 text-micro uppercase tracking-wide">
                {`NEXT REV ${padRevision(currentRevision + 1)}`}
              </span>
            </>
          ) : (
            <span className="rounded-md border border-white/25 bg-white/10 px-2 py-0.5 text-micro uppercase tracking-wide">
              DRAFT
            </span>
          )}
        </div>
      </div>

      <div className="relative z-10 mt-4 flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
        <div className="min-w-0">
          <h1 className="tnum text-display-lg font-semibold sm:text-display-xl">
            {issued ? (number !== "" ? number : "Quotation") : "New quotation"}
          </h1>
          <p className="mt-2 truncate text-small text-accent-100">
            {customerName.trim() !== "" ? customerName : "No customer yet"}
          </p>
        </div>
        <div className="shrink-0 text-micro uppercase tracking-[0.1em] text-accent-300 sm:text-right">
          <span>Customer total</span>
          <strong
            data-testid="quotation-hero-total"
            className="tnum mt-1 block whitespace-nowrap text-metric font-semibold normal-case tracking-normal text-white"
          >
            {currency === null ? "-" : formatMoney(currency, customerGrand)}
          </strong>
          {ppnAmount > 0 ? (
            <span className="mt-0.5 block text-micro-sm normal-case tracking-normal text-accent-200">
              Includes PPN
            </span>
          ) : null}
        </div>
      </div>

      <dl className="relative z-10 mt-5 flex flex-wrap gap-x-7 gap-y-3 border-t border-white/15 pt-3 text-small text-accent-100">
        <div>
          <dt className="text-micro-xs uppercase tracking-[0.14em] text-accent-300">
            Validity
          </dt>
          <dd>{`${validity} days`}</dd>
        </div>
        <div>
          <dt className="text-micro-xs uppercase tracking-[0.14em] text-accent-300">
            Payment
          </dt>
          <dd>{`${paymentTerm} days`}</dd>
        </div>
        <div>
          <dt className="text-micro-xs uppercase tracking-[0.14em] text-accent-300">
            Terms
          </dt>
          <dd>{`${incoterm.trim() !== "" ? incoterm : "-"} · ${
            currencyCode.trim() !== "" ? currencyCode : "-"
          }`}</dd>
        </div>
        <div>
          <dt className="text-micro-xs uppercase tracking-[0.14em] text-accent-300">
            Tax
          </dt>
          <dd>{taxLabel}</dd>
        </div>
      </dl>

      <p className="relative z-10 mt-3 text-caption text-accent-200">{status}</p>
    </section>
  );
}
