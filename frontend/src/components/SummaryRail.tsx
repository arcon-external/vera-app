import type { ReactNode } from "react";
import { Badge, Card } from "./ui";
import { taxRateBps, taxRateLabel } from "../tax";
import { applyMargin, formatIdr, parseIntOrZero } from "../format";

export interface PricedItem {
  qty: string;
  unitPrice: string;
  marginEnabled: boolean;
  marginPercent: string;
}

interface SummaryRailProps {
  items: PricedItem[];
  discountPercent: number;
  taxZone: string;
  taxCategory: string;
  variant?: "editor" | "internal";
  footer?: ReactNode;
}

export function computePricing(
  items: PricedItem[],
  discountPercent: number,
  taxZone: string,
  taxCategory: string
) {
  let costSubtotal = 0;
  let sellSubtotal = 0;
  let marginedLines = 0;

  for (const item of items) {
    const qty = parseIntOrZero(item.qty);
    const cost = parseIntOrZero(item.unitPrice);
    const sellUnit = applyMargin(
      cost,
      parseIntOrZero(item.marginPercent),
      item.marginEnabled
    );
    costSubtotal += qty * cost;
    sellSubtotal += qty * sellUnit;
    if (item.marginEnabled) marginedLines += 1;
  }

  const originalDiscount = Math.trunc((costSubtotal * discountPercent) / 100);
  const originalGrand = costSubtotal - originalDiscount;
  const sellDiscount = Math.trunc((sellSubtotal * discountPercent) / 100);
  const grandBeforeTax = sellSubtotal - sellDiscount;

  const rateBps = taxRateBps(taxZone, taxCategory);
  const taxAmount = Math.trunc((grandBeforeTax * rateBps) / 10000);

  return {
    costSubtotal,
    sellSubtotal,
    originalDiscount,
    originalGrand,
    sellDiscount,
    grandBeforeTax,
    taxAmount,
    grandAfterTax: grandBeforeTax - taxAmount,
    netProfit: grandBeforeTax - taxAmount - originalGrand,
    profitBeforeTax: grandBeforeTax - originalGrand,
    marginedLines,
    rateBps,
  };
}

export function SummaryRail({
  items,
  discountPercent,
  taxZone,
  taxCategory,
  variant = "editor",
  footer,
}: SummaryRailProps) {
  const pricing = computePricing(items, discountPercent, taxZone, taxCategory);
  const taxLabel = taxRateLabel(taxZone, taxCategory);

  return (
    <Card className="p-5">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
          Summary
        </h2>
        <span className="tnum text-caption text-ink-400">
          {items.length} {items.length === 1 ? "line" : "lines"}
        </span>
      </div>

      <div className="mt-4 space-y-2.5">
        <AmountRow label="Cost subtotal" value={pricing.costSubtotal} />
        <AmountRow
          label={pricing.marginedLines > 0 ? "Customer subtotal" : "Subtotal"}
          value={pricing.sellSubtotal}
        />
        {pricing.sellDiscount > 0 ? (
          <AmountRow
            label={`Discount ${discountPercent}%`}
            value={-pricing.sellDiscount}
          />
        ) : null}
        <div className="border-t border-rule pt-2.5">
          <div className="flex items-baseline justify-between gap-4">
            <span className="text-small font-medium text-ink-800">
              Grand total
            </span>
            <span className="tnum text-lead font-semibold text-ink-950">
              Rp{formatIdr(pricing.grandBeforeTax)}
            </span>
          </div>
          <p className="mt-0.5 text-caption text-ink-500">
            Customer-facing amount, before withholding tax
          </p>
        </div>
      </div>

      <div className="mt-4 rounded-lg border border-internal-200 bg-internal-50 p-3">
        <div className="flex items-center justify-between gap-2">
          <Badge tone="internal">Internal only</Badge>
          <span className="tnum text-caption text-internal-700">
            {taxZone === "FTZ" ? "FTZ · 0% tax" : `Non-FTZ · ${taxLabel}`}
          </span>
        </div>
        <div className="mt-3 space-y-2">
          <AmountRow label="Tax withheld" value={-pricing.taxAmount} />
          <div className="flex items-baseline justify-between gap-4">
            <span className="text-small text-internal-700">Nett after tax</span>
            <span className="tnum text-body font-semibold text-internal-900">
              Rp{formatIdr(pricing.grandAfterTax)}
            </span>
          </div>
          <div className="flex items-baseline justify-between gap-4 border-t border-internal-200 pt-2">
            <span className="text-caption text-internal-700">
              {pricing.netProfit < 0 ? "Nett loss" : "Nett profit"}
            </span>
            <span
              className={`tnum text-small font-semibold ${
                pricing.netProfit < 0 ? "text-danger-600" : "text-success-700"
              }`}
            >
              {pricing.netProfit < 0 ? "−" : ""}Rp
              {formatIdr(Math.abs(pricing.netProfit))}
            </span>
          </div>
        </div>
      </div>

      {variant === "internal" && pricing.profitBeforeTax !== pricing.netProfit ? (
        <p className="mt-2 text-caption text-ink-500">
          Profit before withholding tax: Rp{formatIdr(pricing.profitBeforeTax)}
        </p>
      ) : null}

      {footer ? <div className="mt-4">{footer}</div> : null}
    </Card>
  );
}

function AmountRow({
  label,
  value,
}: {
  label: string;
  value: number;
}) {
  const negative = value < 0;
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-small text-ink-500">{label}</span>
      <span
        className={`tnum text-small ${
          negative ? "text-ink-600" : "text-ink-900"
        }`}
      >
        {negative ? "−" : ""}Rp{formatIdr(Math.abs(value))}
      </span>
    </div>
  );
}
