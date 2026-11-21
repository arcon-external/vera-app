// Issue #643: one vertical six-metric table shared by the month comparison and
// the Business category comparison. The server owns every figure and every
// percentage; this module only formats them. Negative margin is danger-toned
// and a null ratio prints "-", so a loss or an unknown value is never shown as
// a zero. `metricScope` selects the stable data marker each value cell carries
// (`data-month-metric` or `data-category-metric`) without changing the table
// shape.

import type { JSX } from "react";
import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import type { PurchaseOrderReportMetrics } from "../poReport";
import { formatReportBps, formatReportCount } from "../poReportPage";

export interface PurchaseOrderReportMetricTableProps {
  metrics: PurchaseOrderReportMetrics | null;
  entry: CurrencyInfo | null;
  ariaLabel: string;
  id?: string;
  tabIndex?: number;
  /** Emits data-month-metric or data-category-metric on each value td. */
  metricScope: "month" | "category";
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

/**
 * The six vertical metric rows. A null metrics object renders the same six
 * rows with "-" values: the shape of the table never depends on a figure
 * being present, and no value is fabricated.
 */
export function PurchaseOrderReportMetricTable({
  metrics,
  entry,
  ariaLabel,
  id,
  tabIndex,
  metricScope,
}: PurchaseOrderReportMetricTableProps): JSX.Element {
  const rows: { label: string; value: string; negative?: boolean }[] = [
    { label: "POs", value: metrics ? formatReportCount(metrics.poCount) : "-" },
    {
      label: "PO amount",
      value: metrics ? money(entry, metrics.amountMinor) : "-",
    },
    {
      label: "Costed amount",
      value: metrics ? money(entry, metrics.costedMinor) : "-",
    },
    {
      label: "Capital",
      value: metrics ? money(entry, metrics.capitalMinor) : "-",
    },
    {
      label: "PO margin",
      value: metrics ? money(entry, metrics.marginMinor) : "-",
      negative: metrics !== null && metrics.marginMinor < 0,
    },
    {
      label: "PO margin %",
      value: metrics ? formatReportBps(metrics.marginBps) : "-",
    },
  ];
  const metricAttribute =
    metricScope === "month" ? "data-month-metric" : "data-category-metric";

  return (
    <table
      id={id}
      tabIndex={tabIndex}
      aria-label={ariaLabel}
      className="w-full scroll-mt-6 text-left text-body"
    >
      <thead>
        <tr className="border-y border-rule bg-paper-subtle/60 text-micro uppercase tracking-[0.1em] text-ink-500">
          <th className="py-2 pr-4 font-medium">Metric</th>
          <th className="py-2 text-right font-medium">Value</th>
        </tr>
      </thead>
      <tbody className="divide-y divide-rule">
        {rows.map((row) => (
          <tr key={row.label}>
            <th
              scope="row"
              className="py-2.5 pr-4 text-left align-middle font-medium text-ink-700"
            >
              {row.label}
            </th>
            <td
              {...{ [metricAttribute]: row.label }}
              className={`tnum whitespace-nowrap py-2.5 text-right align-middle ${
                row.negative ? "text-danger-700" : "text-ink-900"
              }`}
            >
              {row.value}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
