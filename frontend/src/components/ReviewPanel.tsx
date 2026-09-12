import type { CustomerPreview, InternalBreakdown } from "../preview";

interface ReviewPanelProps {
  customer: CustomerPreview;
  internal: InternalBreakdown;
  quotationHTML: string;
}

const sectionLabel =
  "text-[11px] font-medium tracking-[0.2em] uppercase text-slate-400";
const card =
  "rounded-2xl border border-slate-200/80 bg-white shadow-[0_1px_3px_rgba(0,0,0,0.03)]";

function Amount({ value, strong = false }: { value: string; strong?: boolean }) {
  return (
    <span
      className={`whitespace-nowrap text-right tabular-nums ${
        strong ? "font-semibold text-slate-900" : "text-slate-700"
      }`}
    >
      Rp{value}
    </span>
  );
}

function SummaryRow({
  label,
  value,
  strong = false,
  tone = "default",
}: {
  label: string;
  value: string;
  strong?: boolean;
  tone?: "default" | "accent";
}) {
  return (
    <div className="flex items-baseline justify-between gap-6 border-b border-slate-100 py-2.5 last:border-b-0">
      <span
        className={`min-w-0 text-[13px] ${
          tone === "accent" ? "font-medium text-emerald-700" : "text-slate-500"
        }`}
      >
        {label}
      </span>
      <Amount value={value} strong={strong} />
    </div>
  );
}

function TableFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="overflow-x-auto rounded-xl border border-slate-100">
      {children}
    </div>
  );
}

export function ReviewPanel({ customer, internal, quotationHTML }: ReviewPanelProps) {
  return (
    <section aria-labelledby="review-heading" className="space-y-8">
      <div className="flex items-center gap-3">
        <h2 id="review-heading" className={sectionLabel}>
          Review before export
        </h2>
        <div className="h-px flex-1 bg-slate-200" />
        <span className="text-[12px] text-slate-400">Three views, one calculation</span>
      </div>

      <article className={`${card} overflow-hidden`}>
        <header className="flex flex-wrap items-start justify-between gap-4 border-b border-slate-100 px-6 py-5 md:px-8">
          <div>
            <p className={sectionLabel}>01 / Initial preview</p>
            <h3 className="mt-2 text-[18px] font-semibold tracking-[-0.02em] text-slate-900">
              Original prices before margin
            </h3>
            <p className="mt-1 max-w-2xl text-[13px] leading-relaxed text-slate-500">
              This is the cost input used to calculate the quotation. It does not
              include profit margin or tax.
            </p>
          </div>
          <span className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 text-[11px] font-medium text-slate-500">
            Internal reference
          </span>
        </header>

        <div className="p-6 md:px-8">
          <TableFrame>
            <table className="w-full min-w-[820px] table-fixed text-[13px]">
              <colgroup>
                <col className="w-12" />
                <col />
                <col className="w-24" />
                <col className="w-44" />
                <col className="w-48" />
              </colgroup>
              <thead>
                <tr className="border-b border-slate-100 text-left text-[11px] font-medium tracking-[0.08em] uppercase text-slate-400">
                  <th className="px-4 py-3 font-medium">No</th>
                  <th className="px-4 py-3 font-medium">Description</th>
                  <th className="px-4 py-3 text-center font-medium">Qty</th>
                  <th className="px-4 py-3 text-right font-medium">Unit before margin</th>
                  <th className="px-4 py-3 text-right font-medium">Total before tax &amp; margin</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {customer.Items.map((item) => (
                  <tr key={item.No} className="align-top">
                    <td className="px-4 py-3 text-slate-400">{item.No}</td>
                    <td className="whitespace-pre-wrap break-words px-4 py-3 leading-relaxed text-slate-700">
                      {item.Description}
                    </td>
                    <td className="px-4 py-3 text-center whitespace-nowrap text-slate-700">
                      {item.Qty} {item.UoM}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Amount value={item.DisplayOriginalUnitPrice} />
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Amount value={item.DisplayOriginalTotal} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableFrame>

          <div className="mt-6 ml-auto max-w-md border-t border-slate-100 pt-3">
            <SummaryRow label="Original subtotal" value={internal.DisplayOriginalSubtotal} />
            <SummaryRow
              label={`Original discount (${customer.DiscountPercent}%)`}
              value={internal.DisplayOriginalDiscount}
            />
            <SummaryRow label="Original grand total" value={internal.DisplayOriginalGrand} strong />
          </div>
        </div>
      </article>

      <article className={`${card} overflow-hidden border-amber-200/80 bg-amber-50/35`}>
        <header className="flex flex-wrap items-start justify-between gap-4 border-b border-amber-100 px-6 py-5 md:px-8">
          <div>
            <p className={sectionLabel}>02 / Internal breakdown</p>
            <h3 className="mt-2 text-[18px] font-semibold tracking-[-0.02em] text-slate-900">
              Margin and tax calculation
            </h3>
            <p className="mt-1 max-w-2xl text-[13px] leading-relaxed text-slate-500">
              Margins are applied per unit. Tax is deducted once from the
              marginalized grand total below; it is not allocated to individual rows.
            </p>
          </div>
          <span className="rounded-lg border border-amber-200 bg-amber-100 px-3 py-1.5 text-[11px] font-medium text-amber-800">
            Internal only — never exported
          </span>
        </header>

        <div className="p-6 md:px-8">
          <TableFrame>
            <table className="w-full min-w-[820px] table-fixed text-[13px]">
              <colgroup>
                <col className="w-12" />
                <col />
                <col className="w-24" />
                <col className="w-56" />
                <col className="w-52" />
              </colgroup>
              <thead>
                <tr className="border-b border-amber-100 text-left text-[11px] font-medium tracking-[0.08em] uppercase text-slate-500">
                  <th className="px-4 py-3 font-medium">No</th>
                  <th className="px-4 py-3 font-medium">Description</th>
                  <th className="px-4 py-3 text-center font-medium">Qty</th>
                  <th className="px-4 py-3 text-right font-medium">Unit after margin</th>
                  <th className="px-4 py-3 text-right font-medium">Total after margin</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-amber-100/80">
                {internal.Items.map((item) => (
                  <tr key={item.No} className="align-top">
                    <td className="px-4 py-3 text-slate-400">{item.No}</td>
                    <td className="whitespace-pre-wrap break-words px-4 py-3 leading-relaxed text-slate-700">
                      {item.Description}
                    </td>
                    <td className="px-4 py-3 text-center whitespace-nowrap text-slate-700">
                      {item.Qty}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="font-medium tabular-nums text-slate-800">
                        Rp{item.DisplayMarginalizedUnit}
                      </div>
                      <div className="mt-1 text-[11px] tabular-nums text-amber-700">
                        {item.MarginEnabled
                          ? `+${item.MarginPercent}% · +Rp${item.DisplayMarginAmount}`
                          : "Margin off"}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Amount value={item.DisplayMarginalizedTotal} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableFrame>

          <div className="mt-6 grid gap-6 lg:grid-cols-2">
            <div className="rounded-xl border border-amber-100 bg-white/60 p-4">
              <p className={sectionLabel}>Marginalized total</p>
              <div className="mt-2">
                <SummaryRow label="Marginalized subtotal" value={internal.DisplayMarginalizedSubtotal} />
                <SummaryRow
                  label={`Discount (${customer.DiscountPercent}%)`}
                  value={internal.DisplayMarginalizedDiscount}
                />
                <SummaryRow
                  label="Grand total before tax"
                  value={internal.DisplayMarginalizedGrand}
                  strong
                />
                <SummaryRow
                  label={`Tax — ${internal.TaxLabel}`}
                  value={`-${internal.DisplayTax}`}
                />
                <SummaryRow
                  label="Grand total after tax"
                  value={internal.DisplayMarginalizedAfterTax}
                  strong
                />
              </div>
            </div>

            <div className="rounded-xl border border-amber-100 bg-white/60 p-4">
              <p className={sectionLabel}>Profit record</p>
              <div className="mt-2">
                <SummaryRow label="Original grand total" value={internal.DisplayOriginalGrand} />
                <SummaryRow label="Profit before tax" value={internal.DisplayProfit} />
              </div>
              <div className="mt-4 rounded-xl bg-emerald-600 px-4 py-4 text-white">
                <div className="flex items-baseline justify-between gap-4">
                  <span className="text-[13px] font-medium text-emerald-50">
                    Nett profit after tax
                  </span>
                  <span className="text-[18px] font-semibold tabular-nums">
                    Rp{internal.DisplayNett}
                  </span>
                </div>
                <p className="mt-1 text-[11px] text-emerald-100">
                  Marginalized grand after tax − original grand total
                </p>
              </div>
            </div>
          </div>
        </div>
      </article>

      <article className={`${card} overflow-hidden`}>
        <header className="flex flex-wrap items-start justify-between gap-4 border-b border-slate-100 px-6 py-5 md:px-8">
          <div>
            <p className={sectionLabel}>03 / Quotation preview</p>
            <h3 className="mt-2 text-[18px] font-semibold tracking-[-0.02em] text-slate-900">
              Customer-facing quotation
            </h3>
            <p className="mt-1 max-w-2xl text-[13px] leading-relaxed text-slate-500">
              This is the same template used for the exported PDF: marginalized
              prices before tax, with internal calculations hidden.
            </p>
          </div>
          <span className="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-1.5 text-[11px] font-medium text-emerald-700">
            Ready to export
          </span>
        </header>
        <div className="bg-slate-100/70 p-3 md:p-6">
          <iframe
            title="Customer-facing quotation preview"
            srcDoc={quotationHTML}
            sandbox=""
            className="h-[760px] w-full border-0 bg-white shadow-[0_1px_3px_rgba(15,23,42,0.08)]"
          />
        </div>
      </article>
    </section>
  );
}
