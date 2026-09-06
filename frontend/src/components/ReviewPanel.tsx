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
      <span className="text-[13px] text-slate-500">{label}</span>
      <span
        className={`text-[13px] tabular-nums ${
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
        className="grid grid-cols-1 gap-6 lg:grid-cols-[1.2fr_0.8fr]"
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
            <table className="w-full text-[13px]">
              <thead>
                <tr className="border-b border-slate-100 text-left text-[11px] font-medium tracking-[0.08em] uppercase text-slate-400">
                  <th className="px-3 py-2 font-medium">No</th>
                  <th className="px-3 py-2 font-medium">Description</th>
                  <th className="px-3 py-2 text-center font-medium">Qty</th>
                  <th className="px-3 py-2 text-right font-medium">Unit</th>
                  <th className="px-3 py-2 text-right font-medium">Total</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {customer.Items.map((item) => (
                  <tr key={item.No}>
                    <td className="px-3 py-2 text-slate-400">{item.No}</td>
                    <td className="max-w-[220px] truncate px-3 py-2 text-slate-700">
                      {item.Description}
                    </td>
                    <td className="px-3 py-2 text-center text-slate-700">
                      {item.Qty} {item.UoM}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-slate-700">
                      {item.DisplayUnitPrice}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-slate-700">
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

        <div className="rounded-2xl border border-amber-200/80 bg-amber-50/50 p-6 shadow-[0_1px_3px_rgba(0,0,0,0.03)]">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-[14px] font-semibold text-slate-900">
              Internal breakdown
            </h2>
            <span className="rounded-full border border-amber-200 bg-amber-100 px-2.5 py-0.5 text-[11px] font-medium text-amber-800">
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
