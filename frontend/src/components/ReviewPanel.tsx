import type { CustomerPreview, InternalBreakdown } from "../preview";

interface ReviewPanelProps {
  customer: CustomerPreview;
  internal: InternalBreakdown;
}

function Row({
  label,
  value,
  strong = false,
}: {
  label: string;
  value: string;
  strong?: boolean;
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5">
      <span className="flex-1 text-[13px] text-slate-500">{label}</span>
      <span
        className={`shrink-0 whitespace-nowrap text-[13px] tabular-nums ${
          strong ? "font-semibold text-slate-900" : "text-slate-700"
        }`}
      >
        Rp{value}
      </span>
    </div>
  );
}

export function ReviewPanel({ customer, internal }: ReviewPanelProps) {
  return (
    <section>
      <div className="mb-4 flex items-center gap-3">
        <span className="text-[11px] tracking-[0.2em] uppercase text-slate-400">
          Review
        </span>
        <div className="h-[1px] flex-1 bg-slate-200" />
      </div>

      <div
        className="grid grid-cols-1 gap-6 xl:grid-cols-[1.25fr_0.75fr]"
        style={{ animation: "statusIn 200ms ease-out forwards" }}
      >
        <div className="rounded-2xl border border-slate-200/80 bg-white p-6 shadow-[0_1px_3px_rgba(0,0,0,0.03)]">
          <div className="mb-1 flex items-center justify-between">
            <h2 className="text-[14px] font-semibold text-slate-900">
              Customer preview
            </h2>
            <span className="text-[11px] text-slate-400">exactly as PDF</span>
          </div>
          <p className="mb-4 text-[12px] text-slate-400">
            {customer.RecipientName} — {customer.RecipientCompany}
          </p>

          <div className="overflow-hidden rounded-xl border border-slate-100">
            <table className="w-full table-fixed text-[13px]">
              <colgroup>
                <col className="w-10" />
                <col />
                <col className="w-24" />
                <col className="w-32" />
                <col className="w-32" />
              </colgroup>
              <thead>
                <tr className="border-b border-slate-100 text-left text-[11px] font-medium tracking-[0.08em] uppercase text-slate-400">
                  <th className="px-3 py-2 font-medium">No</th>
                  <th className="px-3 py-2 font-medium">Description</th>
                  <th className="px-3 py-2 text-center font-medium">Qty</th>
                  <th className="px-3 py-2 text-right font-medium">Unit (Rp)</th>
                  <th className="px-3 py-2 text-right font-medium">Total (Rp)</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {customer.Items.map((item) => (
                  <tr key={item.No} className="align-top">
                    <td className="px-3 py-2 text-slate-400">{item.No}</td>
                    <td className="px-3 py-2 leading-snug text-slate-700 [display:-webkit-box] [-webkit-box-orient:vertical] [-webkit-line-clamp:2] overflow-hidden">
                      {item.Description}
                    </td>
                    <td className="px-3 py-2 text-center whitespace-nowrap text-slate-700">
                      {item.Qty} {item.UoM}
                    </td>
                    <td className="px-3 py-2 text-right whitespace-nowrap tabular-nums text-slate-700">
                      {item.DisplayUnitPrice}
                    </td>
                    <td className="px-3 py-2 text-right whitespace-nowrap tabular-nums text-slate-700">
                      {item.DisplayTotal}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="mt-4 border-t border-slate-100 pt-3">
            <Row label="Sub Total" value={customer.DisplaySubTotal} />
            <Row
              label={`Discount (${customer.DiscountPercent}%)`}
              value={customer.DisplayDiscountAmount}
            />
            <Row
              label="Grand Total"
              value={customer.DisplayGrandTotal}
              strong
            />
          </div>
          <p className="mt-3 text-[11px] text-slate-400">
            No margin or tax shown here — same as the exported PDF.
          </p>
        </div>

        <div className="self-start rounded-2xl border border-amber-200/80 bg-amber-50/50 p-6 shadow-[0_1px_3px_rgba(0,0,0,0.03)] xl:sticky xl:top-6">
          <div className="mb-4 flex items-center justify-between gap-3">
            <h2 className="shrink-0 text-[14px] font-semibold whitespace-nowrap text-slate-900">
              Internal breakdown
            </h2>
            <span className="rounded-full border border-amber-200 bg-amber-100 px-2.5 py-0.5 text-[11px] font-medium whitespace-nowrap text-amber-800">
              Internal only — never exported
            </span>
          </div>

          <div className="divide-y divide-amber-100/80">
            <Row
              label={`Cost subtotal (before ${internal.MarginPercent}% margin)`}
              value={internal.DisplayCost}
            />
            <Row label="Sell subtotal (after margin)" value={internal.DisplaySell} />
            <Row
              label={`Discount on cost (${internal.DiscountPercent}%)`}
              value={internal.DisplayCostDisc}
            />
            <Row
              label={`Discount on sell (${internal.DiscountPercent}%)`}
              value={internal.DisplaySellDisc}
            />
            <Row label="Cost grand (reference)" value={internal.DisplayCostGrd} />
            <Row label="Grand total (customer price)" value={internal.DisplayGrand} strong />
            <Row label="Profit (grand − cost grand)" value={internal.DisplayProfit} />
            <Row label={`Tax — ${internal.TaxLabel}`} value={internal.DisplayTax} />
          </div>

          <div className="mt-3 rounded-xl bg-emerald-600 px-4 py-3">
            <div className="flex items-baseline justify-between gap-4">
              <span className="text-[13px] font-medium text-emerald-50">
                Nett profit (after tax)
              </span>
              <span className="text-[16px] font-semibold tabular-nums text-white">
                Rp{internal.DisplayNett}
              </span>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
