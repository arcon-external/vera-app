import { useEffect, useState } from "react";
import { GetQuotationRevision } from "@backend/backend/Quotation";
import type { ArchivedQuotation } from "../archive";
import { useToast } from "../toast";

interface QuotationDetailPageProps {
  quotationID: number;
  revision: number;
  onBack: () => void;
  onEdit: (quotation: ArchivedQuotation) => void;
}

function DetailAmount({ value, strong = false }: { value: string; strong?: boolean }) {
  return <span className={`whitespace-nowrap tabular-nums ${strong ? "font-semibold text-slate-900" : "text-slate-800"}`}>Rp{value}</span>;
}

export function QuotationDetailPage({ quotationID, revision, onBack, onEdit }: QuotationDetailPageProps) {
  const [quotation, setQuotation] = useState<ArchivedQuotation | null>(null);
  const [loading, setLoading] = useState(true);
  const { pushToast } = useToast();

  useEffect(() => {
    let active = true;
    setLoading(true);
    void GetQuotationRevision(quotationID, revision)
      .then((result) => {
        if (active) setQuotation(result as ArchivedQuotation);
      })
      .catch(() => {
        pushToast("error", "We couldn't load this quotation. Return to the archive and try again.");
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [quotationID, revision, pushToast]);

  if (loading) {
    return <div className="h-64 animate-pulse rounded-2xl bg-slate-100" aria-label="Loading quotation" />;
  }

  if (!quotation) {
    return (
      <div className="rounded-2xl border border-red-200 bg-red-50 p-8 text-center">
        <p className="text-[14px] font-medium text-red-800">Quotation not found.</p>
        <button type="button" onClick={onBack} className="mt-4 min-h-11 rounded-xl border border-red-200 px-4 text-[13px] font-medium text-red-700">
          Return to archive
        </button>
      </div>
    );
  }

  return (
    <section className="space-y-8" aria-labelledby="quotation-detail-heading">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <button type="button" onClick={onBack} className="text-[12px] font-medium text-slate-400 hover:text-slate-800">
            ← Back to archive
          </button>
          <div className="mt-4 flex flex-wrap items-baseline gap-3">
            <h2 id="quotation-detail-heading" className="text-[26px] font-semibold tracking-[-0.03em] text-slate-900">
              {quotation.QuoteNumber}
            </h2>
            <span className="rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1 text-[12px] font-medium tabular-nums text-slate-500">
              Revision {String(quotation.RevisionNumber).padStart(2, "0")}
            </span>
          </div>
          <p className="mt-2 text-[14px] text-slate-500">
            {quotation.CustomerName} · {quotation.Subject}
          </p>
        </div>
        <button
          type="button"
          onClick={() => onEdit(quotation)}
          className="min-h-11 rounded-xl bg-emerald-600 px-4 text-[13px] font-semibold text-white transition hover:bg-emerald-500 active:scale-[0.98] focus:outline-none focus:ring-2 focus:ring-emerald-500/30"
        >
          Edit quotation
        </button>
      </header>

      <article className="rounded-2xl border border-amber-200/80 bg-amber-50/35 p-6 md:p-8">
        <div className="mb-6">
          <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-slate-400">Internal record</p>
          <h3 className="mt-2 text-[18px] font-semibold text-slate-900">Line economics and totals</h3>
          <p className="mt-1 text-[13px] leading-relaxed text-slate-500">
            These values are retained for internal review and never appear in the customer quotation.
          </p>
        </div>

        <div className="overflow-x-auto rounded-xl border border-amber-100 bg-white/70">
          <table className="w-full min-w-[860px] table-fixed text-[13px]">
            <colgroup>
              <col className="w-12" />
              <col />
              <col className="w-24" />
              <col className="w-56" />
              <col className="w-52" />
            </colgroup>
            <thead>
              <tr className="border-b border-amber-100 text-left text-[11px] font-medium tracking-[0.08em] uppercase text-slate-500">
                <th className="px-4 py-3">No</th>
                <th className="px-4 py-3">Description</th>
                <th className="px-4 py-3 text-center">Qty</th>
                <th className="px-4 py-3 text-right">Unit after margin</th>
                <th className="px-4 py-3 text-right">Total after margin</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-amber-100/80">
              {quotation.Items.map((item) => (
                <tr key={item.ItemNo} className="align-top">
                  <td className="px-4 py-3 text-slate-400">{item.ItemNo}</td>
                  <td className="whitespace-pre-wrap break-words px-4 py-3 leading-relaxed text-slate-700">{item.Description}</td>
                  <td className="px-4 py-3 text-center whitespace-nowrap text-slate-700">{item.Qty} {item.UoM}</td>
                  <td className="px-4 py-3 text-right">
                    <div className="font-medium"><DetailAmount value={item.DisplayMarginalizedUnit} /></div>
                    <div className="mt-1 text-[11px] text-amber-700">
                      {item.MarginEnabled ? `+${item.MarginPercent}% · +Rp${item.DisplayMarginAmount}` : "Margin off"}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right"><DetailAmount value={item.DisplayMarginalizedTotal} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="mt-6 grid gap-6 lg:grid-cols-2">
          <div className="rounded-xl border border-amber-100 bg-white/70 p-4">
            <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-slate-400">Original baseline</p>
            <div className="mt-2 divide-y divide-slate-100">
              <DetailRow label="Original subtotal" value={quotation.DisplayOriginalSubtotal} />
              <DetailRow label={`Discount (${quotation.DiscountPercent}%)`} value={quotation.DisplayOriginalDiscount} />
              <DetailRow label="Original grand total" value={quotation.DisplayOriginalGrand} />
            </div>
          </div>
          <div className="rounded-xl border border-amber-100 bg-white/70 p-4">
            <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-slate-400">Marginalized result</p>
            <div className="mt-2 divide-y divide-slate-100">
              <DetailRow label="Marginalized subtotal" value={quotation.DisplayMarginalizedSubtotal} />
              <DetailRow label={`Discount (${quotation.DiscountPercent}%)`} value={quotation.DisplayMarginalizedDiscount} />
              <DetailRow label="Grand total before tax" value={quotation.DisplayMarginalizedGrand} />
              <DetailRow label={`Tax — ${quotation.TaxCategory || quotation.TaxZone}`} value={quotation.DisplayTax} />
              <DetailRow label="Grand total after tax" value={quotation.DisplayMarginalizedAfterTax} />
              <DetailRow label="Nett profit" value={quotation.DisplayNett} strong />
            </div>
          </div>
        </div>
      </article>

      <article className="overflow-hidden rounded-2xl border border-slate-200/80 bg-white shadow-[0_1px_3px_rgba(0,0,0,0.03)]">
        <header className="border-b border-slate-100 px-6 py-5 md:px-8">
          <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-slate-400">Customer copy</p>
          <h3 className="mt-2 text-[18px] font-semibold text-slate-900">Rendered quotation</h3>
        </header>
        <div className="bg-slate-100/70 p-3 md:p-6">
          <iframe title="Rendered quotation" srcDoc={quotation.HTMLSnapshot} sandbox="" className="h-[760px] w-full border-0 bg-white shadow-[0_1px_3px_rgba(15,23,42,0.08)]" />
        </div>
      </article>
    </section>
  );
}

function DetailRow({ label, value, strong = false }: { label: string; value: string; strong?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-2.5 text-[13px]">
      <span className="text-slate-500">{label}</span>
      <DetailAmount value={value} strong={strong} />
    </div>
  );
}
