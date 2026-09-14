import type { ReactNode } from "react";
import type { CustomerPreview, InternalBreakdown } from "../preview";
import { Badge, Card, SectionHeading } from "./ui";

interface ReviewPanelProps {
  customer: CustomerPreview;
  internal: InternalBreakdown;
  quotationHTML: string;
}

function Amount({
  value,
  strong = false,
  tone = "default",
}: {
  value: string;
  strong?: boolean;
  tone?: "default" | "internal" | "success";
}) {
  const tones = {
    default: strong ? "font-semibold text-ink-950" : "text-ink-900",
    internal: "text-internal-800",
    success: "font-semibold text-success-700",
  } as const;
  return (
    <span className={`tnum whitespace-nowrap text-body ${tones[tone]}`}>
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
  tone?: "default" | "internal" | "success";
}) {
  return (
    <div className="flex items-baseline justify-between gap-6 border-b border-rule py-2 last:border-b-0">
      <span className="min-w-0 text-body text-ink-500">{label}</span>
      <Amount value={value} strong={strong} tone={tone} />
    </div>
  );
}

function StageHeader({
  step,
  title,
  description,
  badge,
}: {
  step: string;
  title: string;
  description: string;
  badge: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4 border-b border-rule px-5 py-4 lg:px-6">
      <div className="max-w-3xl">
        <div className="flex items-center gap-2">
          <span className="tnum text-micro font-medium uppercase tracking-[0.14em] text-ink-500">
            {step}
          </span>
          <span className="h-px w-6 bg-rule-strong" />
          {badge}
        </div>
        <h3 className="mt-2 text-title font-semibold text-ink-950">{title}</h3>
        <p className="mt-1 text-small leading-relaxed text-ink-500">{description}</p>
      </div>
    </header>
  );
}

function TableFrame({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto scroll-thin rounded-lg border border-rule">
      {children}
    </div>
  );
}

export function ReviewPanel({ customer, internal, quotationHTML }: ReviewPanelProps) {
  return (
    <section aria-labelledby="review-heading" className="space-y-6">
      <SectionHeading
        title="Review before export"
        action={
          <span className="text-caption text-ink-500">
            Three views, one calculation
          </span>
        }
      />

      <Card className="overflow-hidden">
        <StageHeader
          step="01 — Initial preview"
          title="Original prices before margin"
          description="The cost input used to build this quotation. No profit margin and no tax are included here."
          badge={<Badge tone="neutral">Reference</Badge>}
        />
        <div className="p-5 lg:p-6">
          <TableFrame>
            <table className="w-full min-w-[840px] table-fixed text-left text-body">
              <colgroup>
                <col className="w-12" />
                <col />
                <col className="w-24" />
                <col className="w-44" />
                <col className="w-48" />
              </colgroup>
              <thead>
                <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                  <th className="px-4 py-2.5 font-medium">No</th>
                  <th className="px-4 py-2.5 font-medium">Description</th>
                  <th className="px-4 py-2.5 text-center font-medium">Qty</th>
                  <th className="px-4 py-2.5 text-right font-medium">
                    Unit before margin
                  </th>
                  <th className="px-4 py-2.5 text-right font-medium">
                    Total before margin
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-rule">
                {customer.Items.map((item) => (
                  <tr key={item.No} className="align-top">
                    <td className="tnum px-4 py-2.5 text-ink-400">{item.No}</td>
                    <td className="whitespace-pre-wrap break-words px-4 py-2.5 text-body leading-relaxed text-ink-700">
                      {item.Description}
                    </td>
                    <td className="px-4 py-2.5 text-center text-body tnum text-ink-700">
                      {item.Qty} {item.UoM}
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <Amount value={item.DisplayOriginalUnitPrice} />
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <Amount value={item.DisplayOriginalTotal} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableFrame>

          <div className="mt-5 ml-auto max-w-md">
            <SummaryRow label="Original subtotal" value={internal.DisplayOriginalSubtotal} />
            <SummaryRow
              label={`Discount ${customer.DiscountPercent}%`}
              value={internal.DisplayOriginalDiscount}
            />
            <SummaryRow
              label="Original grand total"
              value={internal.DisplayOriginalGrand}
              strong
            />
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden border-internal-200">
        <StageHeader
          step="02 — Internal breakdown"
          title="Margin and tax calculation"
          description="Margin is applied per unit. PPN (non-FTZ goods only) joins the customer quotation; services tax stays internal and is never allocated to individual lines."
          badge={<Badge tone="internal">Internal only — never exported</Badge>}
        />
        <div className="p-5 lg:p-6">
          <TableFrame>
            <table className="w-full min-w-[860px] table-fixed text-left text-body">
              <colgroup>
                <col className="w-12" />
                <col />
                <col className="w-24" />
                <col className="w-60" />
                <col className="w-48" />
              </colgroup>
              <thead>
                <tr className="border-b border-rule bg-internal-50 text-micro uppercase tracking-[0.1em] text-internal-700">
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
                {internal.Items.map((item) => (
                  <tr key={item.No} className="align-top">
                    <td className="tnum px-4 py-2.5 text-ink-400">{item.No}</td>
                    <td className="whitespace-pre-wrap break-words px-4 py-2.5 text-body leading-relaxed text-ink-700">
                      {item.Description}
                    </td>
                    <td className="px-4 py-2.5 text-center text-body tnum text-ink-700">
                      {item.Qty}
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <div className="tnum text-body font-medium text-ink-950">
                        Rp{item.DisplayMarginalizedUnit}
                      </div>
                      <div className="tnum mt-0.5 text-caption text-internal-700">
                        {item.MarginEnabled
                          ? `+${item.MarginPercent}% · +Rp${item.DisplayMarginAmount}`
                          : "No margin"}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <Amount value={item.DisplayMarginalizedTotal} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableFrame>

          <div className="mt-5 grid items-start gap-4 lg:grid-cols-2">
            <div className="rounded-lg border border-rule bg-white p-4">
              <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
                Marginalized total
              </p>
              <div className="mt-2">
                <SummaryRow
                  label="Customer subtotal"
                  value={internal.DisplayMarginalizedSubtotal}
                />
                <SummaryRow
                  label={`Discount ${customer.DiscountPercent}%`}
                  value={internal.DisplayMarginalizedDiscount}
                />
                <SummaryRow
                  label="Grand total before tax"
                  value={internal.DisplayMarginalizedGrand}
                  strong
                />
                {internal.PpnAmount > 0 ? (
                  <SummaryRow
                    label="PPN 11% (on the quotation)"
                    value={`+${internal.DisplayPpn}`}
                  />
                ) : null}
                {internal.TaxAmount > 0 ? (
                  <SummaryRow
                    label={`Services tax, internal — ${internal.TaxLabel}`}
                    value={`−${internal.DisplayTax}`}
                  />
                ) : null}
                <SummaryRow
                  label="Grand total after tax"
                  value={internal.DisplayMarginalizedAfterTax}
                  strong
                />
              </div>
            </div>

            <div className="rounded-lg border border-rule bg-white p-4">
              <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
                Profit record
              </p>
              <div className="mt-2">
                <SummaryRow
                  label="Original grand total"
                  value={internal.DisplayOriginalGrand}
                />
                <SummaryRow
                  label="Profit before tax"
                  value={internal.DisplayProfit}
                />
              </div>
              <div className="mt-3 rounded-lg bg-success-700 px-4 py-3.5">
                <div className="flex items-baseline justify-between gap-4">
                  <span className="text-small font-medium text-success-50">
                    Nett profit after tax
                  </span>
                  <span className="tnum text-title font-semibold text-white">
                    Rp{internal.DisplayNett}
                  </span>
                </div>
                <p className="mt-1 text-caption text-success-100">
                  Customer grand total minus original grand total, less
                  services tax
                </p>
              </div>
            </div>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden">
        <StageHeader
          step="03 — Quotation preview"
          title="Customer-facing quotation"
          description="The exact template used for the exported PDF: marginalized prices before tax, with all internal calculations hidden."
          badge={<Badge tone="success">Ready to export</Badge>}
        />
        <div className="bg-paper-muted p-3 lg:p-5">
          <iframe
            title="Customer-facing quotation preview"
            srcDoc={quotationHTML}
            sandbox=""
            className="h-[min(820px,72vh)] min-h-[420px] w-full border-0 bg-white shadow-card"
          />
        </div>
      </Card>
    </section>
  );
}
