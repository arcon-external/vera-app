import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import type { PurchaseOrderReportData } from "../poReport";
import { Card } from "./ui";

/**
 * Issue #653: the two report cards that are deliberately not Actual.
 *
 * Pipeline is open Unsold, still orderable, never added to Actual and never a
 * loss. Realized Variance is the commercial comparison against the quotation;
 * open Unsold is shown beside it but excluded from the net. Closed loss is the
 * unbought remainder of an explicitly closed quotation (#656): it is
 * subtracted from the net, and reopening restores Unsold.
 */
export interface PurchaseOrderReportPipelineVarianceCardsProps {
  report: PurchaseOrderReportData;
  currency: CurrencyInfo | null;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

function AmountRow({
  label,
  value,
  negative = false,
  strong = false,
}: {
  label: string;
  value: string;
  negative?: boolean;
  strong?: boolean;
}) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className="text-small text-ink-600">{label}</dt>
      <dd
        className={`tnum whitespace-nowrap text-right ${
          strong ? "text-body font-semibold" : "text-small font-medium"
        } ${negative ? "text-danger-700" : "text-ink-950"}`}
      >
        {value}
      </dd>
    </div>
  );
}

export function PurchaseOrderReportPipelineVarianceCards({
  report,
  currency,
}: PurchaseOrderReportPipelineVarianceCardsProps) {
  const { pipeline, variance } = report;
  return (
    <div
      className="grid gap-4 border-t border-rule px-5 pb-5 pt-4 lg:grid-cols-2"
      data-report-pipeline-variance=""
    >
      <Card className="p-4" data-report-pipeline-card="">
        <div className="flex items-baseline justify-between gap-2">
          <h3 className="text-body font-semibold text-ink-950">
            Pipeline — Unsold
          </h3>
          <span className="text-caption font-medium uppercase tracking-[0.12em] text-ink-500">
            Still orderable
          </span>
        </div>
        <p className="mt-1.5 text-small leading-relaxed text-ink-500">
          Unsold is not a loss and is not added to Actual.
        </p>
        <dl className="mt-3 space-y-1.5">
          <AmountRow
            label="Unsold amount"
            value={money(currency, pipeline.unsoldAmountMinor)}
          />
          <AmountRow
            label="Unsold margin"
            value={money(currency, pipeline.unsoldMarginMinor)}
            negative={pipeline.unsoldMarginMinor < 0}
          />
        </dl>
      </Card>

      <Card className="p-4" data-report-variance-card="">
        <div className="flex items-baseline justify-between gap-2">
          <h3 className="text-body font-semibold text-ink-950">
            Realized Variance
          </h3>
          <span className="text-caption font-medium uppercase tracking-[0.12em] text-ink-500">
            vs quotation
          </span>
        </div>
        <p className="mt-1.5 text-small leading-relaxed text-ink-500">
          A comparison, not revenue. Open Unsold is excluded from net realized.
        </p>
        <dl className="mt-3 space-y-1.5">
          <AmountRow
            label="Quantity extra income"
            value={`${money(currency, variance.quantityExtraIncomeAmountMinor)} · margin ${money(
              currency,
              variance.quantityExtraIncomeMarginMinor
            )}`}
            negative={variance.quantityExtraIncomeAmountMinor < 0}
          />
          <AmountRow
            label="Price extra income"
            value={money(currency, variance.priceExtraIncomeMinor)}
          />
          <AmountRow
            label="Price loss"
            value={
              variance.priceLossMinor === 0
                ? money(currency, 0)
                : `−${money(currency, variance.priceLossMinor)}`
            }
            negative={variance.priceLossMinor > 0}
          />
          <AmountRow
            label="Closed loss"
            value={money(currency, variance.closedLossAmountMinor)}
            negative={variance.closedLossAmountMinor > 0}
          />
          <div className="border-t border-rule pt-2">
            <AmountRow
              label="Net realized variance"
              value={money(currency, variance.netRealizedVarianceAmountMinor)}
              negative={variance.netRealizedVarianceAmountMinor < 0}
              strong
            />
            <AmountRow
              label="Net realized margin"
              value={money(currency, variance.netRealizedVarianceMarginMinor)}
              negative={variance.netRealizedVarianceMarginMinor < 0}
            />
          </div>
        </dl>
      </Card>
    </div>
  );
}
