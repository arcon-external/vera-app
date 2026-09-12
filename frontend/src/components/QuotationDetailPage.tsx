import { useEffect, useMemo, useState } from "react";
import { GetQuotationRevision } from "@backend/backend/Quotation";
import type { ArchivedQuotation } from "../archive";
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
}

export function QuotationDetailPage({
  quotationID,
  revision,
  onBack,
  onEdit,
}: QuotationDetailPageProps) {
  const [quotation, setQuotation] = useState<ArchivedQuotation | null>(null);
  const [loading, setLoading] = useState(true);
  const [switching, setSwitching] = useState(false);
  const [activeRevision, setActiveRevision] = useState(revision);
  const { pushToast } = useToast();

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
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <button
            type="button"
            onClick={onBack}
            className="inline-flex min-h-8 items-center gap-1.5 rounded-md px-1.5 text-small text-ink-500 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40"
          >
            <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M10 3.5L5.5 8 10 12.5" />
            </svg>
            Archive
          </button>
          <div className="mt-3 flex flex-wrap items-center gap-3">
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
                  onChange={(next) => setActiveRevision(Number(next))}
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
          <p className="mt-1.5 max-w-3xl text-body text-ink-500">
            {quotation.CustomerName} — {quotation.Subject}
          </p>
        </div>
        <Button variant="primary" onClick={() => onEdit(quotation)}>
          Edit revision {padRevision(quotation.RevisionNumber)}
        </Button>
      </header>

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
          <table className="w-full min-w-[860px] table-fixed text-left">
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
                      Rp{item.DisplayMarginalizedUnit}
                    </div>
                    <div className="tnum mt-0.5 text-caption text-internal-700">
                      {item.MarginEnabled
                        ? `+${item.MarginPercent}% · +Rp${item.DisplayMarginAmount}`
                        : "No margin"}
                    </div>
                  </td>
                  <td className="tnum px-4 py-2.5 text-right text-body text-ink-900">
                    Rp{item.DisplayMarginalizedTotal}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="mt-5 grid gap-4 lg:grid-cols-2">
          <div className="rounded-lg border border-rule bg-white p-4">
            <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
              Original baseline
            </p>
            <dl className="mt-2.5 divide-y divide-rule">
              <DetailRow label="Original subtotal" value={quotation.DisplayOriginalSubtotal} />
              <DetailRow
                label={`Discount ${quotation.DiscountPercent}%`}
                value={quotation.DisplayOriginalDiscount}
              />
              <DetailRow label="Original grand total" value={quotation.DisplayOriginalGrand} />
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
              />
              <DetailRow
                label={`Discount ${quotation.DiscountPercent}%`}
                value={quotation.DisplayMarginalizedDiscount}
              />
              <DetailRow
                label="Grand total before tax"
                value={quotation.DisplayMarginalizedGrand}
              />
              <DetailRow
                label={`Tax — ${quotation.TaxCategory || quotation.TaxZone}`}
                value={quotation.DisplayTax}
              />
              <DetailRow
                label="Grand total after tax"
                value={quotation.DisplayMarginalizedAfterTax}
              />
              <DetailRow label="Nett profit" value={quotation.DisplayNett} strong />
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
  strong = false,
}: {
  label: string;
  value: string;
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
        Rp{value}
      </dd>
    </div>
  );
}
