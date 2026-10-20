import { useEffect, useMemo, useState } from "react";
import { GetQuotationRevision } from "@backend/backend/Quotation";
import { GetCurrencyCatalogue } from "@backend/backend/AdminAPI";
import type { ArchivedQuotation } from "../archive";
import { resolveCurrencyEntry } from "../archive";
import {
  decorateMoney,
  formatMoney,
  type CurrencyInfo,
} from "../format";

// The IDR entry, transcribed field-for-field from
// backend/currency/currency.go. This is the prop default only -- the live
// entry always arrives from the caller -- not a catalogue mirror.
const IDR_ENTRY: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

// signedMoney renders a profit figure that can be negative. A discount below
// cost is a loss, and printing it as a positive number would be a lie the
// reader has to notice for themselves. The sign stays outside the decorated
// figure (today's "−Rp" order), so IDR renders byte-identically.
function signedMoney(currency: CurrencyInfo, value: number): string {
  const text = formatMoney(currency, Math.abs(value));
  return `${value < 0 ? "−" : "+"}${text}`;
}

function taxDetailLabel(quotation: ArchivedQuotation): string {
  if (quotation.ServiceCategory === "Construction") {
    return "Construction 1,75% PPh22";
  }
  if (quotation.ServiceCategory === "NonConstruction") {
    return "Non-construction 2% PPh22";
  }
  return quotation.TaxCategory || quotation.TaxZone;
}
import { useToast } from "../toast";
import { Select } from "./Select";
import { Badge, Button, Card, SectionHeading } from "./ui";

function padRevision(revision: number): string {
  return String(revision).padStart(2, "0");
}

interface QuotationDetailPageProps {
  quotationID: number;
  revision: number;
  onBack: () => void;
  onEdit: (quotation: ArchivedQuotation) => void;
  /** Second PO entry path (#467): opens the shared source selector. */
  onAddPurchaseOrder: () => void;
  /** The live profile entry: the initial render's currency only. Once the
   * revision loads, every figure renders in its REVISION currency
   * (quotation.Currency) resolved below; the live entry is then just the
   * fallback for an unresolvable code. */
  currency?: CurrencyInfo;
}

export interface QuotationDetailHeaderProps {
  quotation: ArchivedQuotation;
  activeRevision: number;
  revisionOptions: { value: number; label: string }[];
  onRevisionChange: (revision: number) => void;
  onBack: () => void;
  onEdit: () => void;
  onAddPurchaseOrder: () => void;
}

// The issued-Quotation header, exported so its two entry actions can be
// rendered without a DOM: the page itself fetches in an effect and prints a
// loading skeleton to SSR.
export function QuotationDetailHeader({
  quotation,
  activeRevision,
  revisionOptions,
  onRevisionChange,
  onBack,
  onEdit,
  onAddPurchaseOrder,
}: QuotationDetailHeaderProps) {
  return (
    <header>
      <button
        type="button"
        onClick={onBack}
        className="inline-flex min-h-8 items-center gap-1.5 rounded-md px-2 py-1.5 text-small text-ink-500 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
      >
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M10 3.5L5.5 8 10 12.5" />
        </svg>
        Archive
      </button>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <h1
            id="quotation-detail-heading"
            className="tnum text-display font-semibold text-ink-950"
          >
            {quotation.QuoteNumber}
          </h1>
          {revisionOptions.length > 1 ? (
            <div className="w-[220px]">
              <Select
                value={activeRevision}
                options={revisionOptions}
                ariaLabel="Quotation revision"
                onChange={(next) => onRevisionChange(Number(next))}
              />
            </div>
          ) : (
            <Badge tone="accent">
              <span className="tnum">
                Revision {padRevision(quotation.RevisionNumber)}
              </span>
            </Badge>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="secondary"
            aria-label="Add Purchase Order from a Quotation"
            onClick={onAddPurchaseOrder}
          >
            Add Purchase Order from a Quotation
          </Button>
          <Button variant="primary" onClick={onEdit}>
            Edit revision {padRevision(quotation.RevisionNumber)}
          </Button>
        </div>
      </div>
      <p className="mt-2 max-w-3xl text-body text-ink-500">
        {quotation.CustomerName} — {quotation.Subject}
      </p>
    </header>
  );
}

export function QuotationDetailPage({
  quotationID,
  revision,
  onBack,
  onEdit,
  onAddPurchaseOrder,
  currency = IDR_ENTRY,
}: QuotationDetailPageProps) {
  const [quotation, setQuotation] = useState<ArchivedQuotation | null>(null);
  const [loading, setLoading] = useState(true);
  const [switching, setSwitching] = useState(false);
  const [activeRevision, setActiveRevision] = useState(revision);
  const { pushToast } = useToast();
  // The catalogue, fetched like App.tsx fetches it: the revision code
  // resolves to its entry once loaded. Null until then -- figures fall back
  // to the live entry rather than rendering nothing.
  const [catalogue, setCatalogue] = useState<CurrencyInfo[] | null>(null);
  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const entries = (await GetCurrencyCatalogue()) as CurrencyInfo[];
        if (active) setCatalogue(entries);
      } catch {
        if (active) setCatalogue(null);
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  // Reset to the revision requested by the archive when the target changes.
  // Clearing the loaded record makes a different quotation show the skeleton
  // rather than the previous quotation's data.
  useEffect(() => {
    setActiveRevision(revision);
    setQuotation(null);
  }, [revision, quotationID]);

  useEffect(() => {
    let active = true;
    if (quotation === null) {
      setLoading(true);
    } else {
      setSwitching(true);
    }
    void GetQuotationRevision(quotationID, activeRevision)
      .then((result) => {
        if (active) setQuotation(result as ArchivedQuotation);
      })
      .catch(() => {
        pushToast(
          "error",
          "We couldn't load this quotation. Return to the archive and try again."
        );
      })
      .finally(() => {
        if (active) {
          setLoading(false);
          setSwitching(false);
        }
      });
    return () => {
      active = false;
    };
    // `quotation` is intentionally excluded: it only distinguishes the first
    // load from a revision switch and would otherwise re-trigger the fetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [quotationID, activeRevision, pushToast]);

  const revisionOptions = useMemo(() => {
    const revisions = quotation?.AvailableRevisions ?? [activeRevision];
    return revisions.map((value, index) => ({
      value,
      label:
        index === 0
          ? `Revision ${padRevision(value)} — latest`
          : `Revision ${padRevision(value)}`,
    }));
  }, [quotation?.AvailableRevisions, activeRevision]);

  // The REVISION currency once loaded; the live entry before that and for
  // any code the catalogue cannot resolve.
  const entry = resolveCurrencyEntry(catalogue, quotation?.Currency, currency);

  if (loading) {
    return (
      <div className="space-y-4" aria-label="Loading quotation">
        <div className="h-8 w-64 animate-pulse rounded-md bg-paper-muted" />
        <div className="h-72 animate-pulse rounded-xl bg-paper-muted" />
      </div>
    );
  }

  if (!quotation) {
    return (
      <Card className="flex flex-col items-center gap-3 px-6 py-16 text-center">
        <p className="text-body text-danger-700">Quotation not found.</p>
        <Button onClick={onBack}>Return to archive</Button>
      </Card>
    );
  }

  return (
    <section
      aria-labelledby="quotation-detail-heading"
      aria-busy={switching}
      className={`space-y-6 transition-opacity duration-200 ease-out-quart ${
        switching ? "opacity-60" : "opacity-100"
      }`}
    >
      <QuotationDetailHeader
        quotation={quotation}
        activeRevision={activeRevision}
        revisionOptions={revisionOptions}
        onRevisionChange={(next) => setActiveRevision(Number(next))}
        onBack={onBack}
        onEdit={() => onEdit(quotation)}
        onAddPurchaseOrder={onAddPurchaseOrder}
      />

      <Card className="border-internal-200 bg-internal-50/40 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <SectionHeading title="Internal record" />
            <h2 className="text-title font-semibold text-ink-950">
              Line economics and totals
            </h2>
            <p className="mt-1 max-w-2xl text-small text-ink-500">
              Retained for internal review. None of these values appear in the
              customer quotation.
            </p>
          </div>
          <Badge tone="internal">Internal only</Badge>
        </div>

        <div className="mt-5 overflow-x-auto scroll-thin rounded-lg border border-internal-200 bg-white">
          <table className="w-full min-w-[860px] table-fixed text-left text-body">
            <colgroup>
              <col className="w-12" />
              <col />
              <col className="w-24" />
              <col className="w-56" />
              <col className="w-48" />
            </colgroup>
            <thead>
              <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                <th className="px-4 py-2.5 font-medium">No</th>
                <th className="px-4 py-2.5 font-medium">Description</th>
                <th className="px-4 py-2.5 text-center font-medium">Qty</th>
                <th className="px-4 py-2.5 text-right font-medium">
                  Unit after margin
                </th>
                <th className="px-4 py-2.5 text-right font-medium">
                  Total after margin
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-rule">
              {quotation.Items.map((item) => (
                <tr key={item.ItemNo} className="align-top">
                  <td className="tnum px-4 py-2.5 text-ink-400">
                    {item.ItemNo}
                  </td>
                  <td className="whitespace-pre-wrap break-words px-4 py-2.5 text-body leading-relaxed text-ink-700">
                    {item.Description}
                  </td>
                  <td className="px-4 py-2.5 text-center text-body tnum text-ink-700">
                    {item.Qty} {item.UoM}
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <div className="tnum text-body font-medium text-ink-900">
                      {decorateMoney(entry, item.DisplayMarginalizedUnit)}
                    </div>
                    {item.DiscountEnabled ? (
                      <div className="tnum mt-0.5 text-caption text-accent-700">
                        −{item.DiscountPercent}% off · was{" "}
                        {formatMoney(entry, item.ListUnitPrice)}
                      </div>
                    ) : null}
                    <div className="tnum mt-0.5 text-caption text-internal-700">
                      {item.MarginEnabled
                        ? `+${item.MarginPercent}% · ${signedMoney(entry, item.MarginAmount)}`
                        : "No margin"}
                    </div>
                  </td>
                  <td className="tnum px-4 py-2.5 text-right text-body text-ink-900">
                    {decorateMoney(entry, item.DisplayMarginalizedTotal)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="mt-5 grid items-start gap-4 lg:grid-cols-2">
          <div className="rounded-lg border border-rule bg-white p-4">
            <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
              Original baseline
            </p>
            <dl className="mt-2.5 divide-y divide-rule">
              {/* No cost-side discount: the cost subtotal is the grand total,
                  so showing both would print the same figure twice. */}
              <DetailRow
                label="Cost total (undiscounted)"
                value={quotation.DisplayOriginalGrand}
                currency={entry}
              />
            </dl>
          </div>
          <div className="rounded-lg border border-rule bg-white p-4">
            <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
              Marginalized result
            </p>
            <dl className="mt-2.5 divide-y divide-rule">
              <DetailRow
                label="Customer subtotal"
                value={quotation.DisplayMarginalizedSubtotal}
                currency={entry}
              />
              {quotation.MarginalizedDiscount > 0 ? (
                <DetailRow
                  label={`Discount ${quotation.DiscountPercent}% (on the price)`}
                  value={`−${quotation.DisplayMarginalizedDiscount}`}
                  currency={entry}
                />
              ) : null}
              <DetailRow
                label="Grand total before tax"
                value={quotation.DisplayMarginalizedGrand}
                currency={entry}
              />
              {quotation.PpnAmount > 0 ? (
                <DetailRow
                  label="PPN 11% (on the quotation)"
                  value={`+${quotation.DisplayPpn}`}
                  currency={entry}
                />
              ) : null}
              {quotation.TaxAmount > 0 ? (
                <DetailRow
                  label={`Services tax, internal — ${taxDetailLabel(quotation)}`}
                  value={quotation.DisplayTax}
                  currency={entry}
                />
              ) : null}
              <DetailRow
                label="Grand total after tax"
                value={quotation.DisplayMarginalizedAfterTax}
                currency={entry}
              />
              <DetailRow label="Nett profit" value={quotation.DisplayNett} currency={entry} strong />
            </dl>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden">
        <header className="border-b border-rule px-5 py-4">
          <h2 className="text-title font-semibold text-ink-950">
            Customer copy
          </h2>
          <p className="mt-1 text-small text-ink-500">
            The rendered quotation as the customer receives it. Margin, tax,
            and nett profit are excluded.
          </p>
        </header>
        <div className="bg-paper-muted p-3 lg:p-5">
          <iframe
            title="Rendered quotation"
            srcDoc={quotation.HTMLSnapshot}
            sandbox=""
            className="h-[min(820px,72vh)] min-h-[420px] w-full border-0 bg-white shadow-card"
          />
        </div>
      </Card>
    </section>
  );
}

function DetailRow({
  label,
  value,
  currency,
  strong = false,
}: {
  label: string;
  value: string;
  currency: CurrencyInfo;
  strong?: boolean;
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-2">
      <dt className="text-small text-ink-500">{label}</dt>
      <dd
        className={`tnum text-small ${
          strong ? "font-semibold text-ink-950" : "text-ink-900"
        }`}
      >
        {decorateMoney(currency, value)}
      </dd>
    </div>
  );
}
