// Issue #638: the compact current-period summary under the performance chart.
//
// The summary is a readout of the server's forecast, never a second
// calculation. It shows a low/base/high range only when the server says the
// projection is available, names the basis in days or months, and states the
// stable reason when a guard refused the forecast. "Updates as data arrives"
// is the only promise made about recomputation; no value is extrapolated here.

import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import type {
  PurchaseOrderReportCurrentProjectionRemainder,
  PurchaseOrderReportData,
  PurchaseOrderReportProjectedBucket,
} from "../poReport";
import {
  formatReportCount,
  formatReportDate,
  projectionReasonLabel,
  projectionSourceLabel,
} from "../poReportPage";
import { Badge } from "./ui";

export interface PurchaseOrderReportProjectionSummaryProps {
  report: PurchaseOrderReportData;
  currency: CurrencyInfo | null;
  /**
   * Issue #644: render as disclosure content without a nested card. The
   * default keeps the standalone summary's panel styling for every existing
   * caller.
   */
  plain?: boolean;
}

export interface PurchaseOrderReportProjectionStatusProps {
  report: PurchaseOrderReportData;
}

function money(currency: CurrencyInfo | null, minor: number): string {
  return currency === null ? "-" : formatMoney(currency, minor);
}

function modeLabel(mode: string): string {
  if (mode === "day") return "Month view, daily buckets";
  if (mode === "month") return "Year view, monthly buckets";
  return "Historical range, no current-period projection";
}

function basisLabel(
  report: PurchaseOrderReportData,
  units: string,
  includeAsOf = true
): string {
  const basis = report.currentProjection.basis;
  const unitText =
    units === "day"
      ? `${formatReportCount(basis.units)} elapsed ${
          basis.units === 1 ? "day" : "days"
        }`
      : `${formatReportCount(basis.units)} complete ${
          basis.units === 1 ? "month" : "months"
        }`;
  const observed = `${formatReportCount(basis.poCount)} ${
    basis.poCount === 1 ? "PO" : "POs"
  } observed`;
  const asOf =
    includeAsOf && basis.asOf.trim() !== ""
      ? `, as of ${formatReportDate(basis.asOf)}`
      : "";
  return `${unitText}, ${observed}${asOf}.`;
}

function Range({
  label,
  low,
  base,
  high,
  currency,
  source,
  sampleSize,
}: {
  label: string;
  low: number;
  base: number;
  high: number;
  currency: CurrencyInfo | null;
  source: string;
  sampleSize: number;
}) {
  return (
    <div data-projection-range={label} className="mt-2">
      <p className="text-caption font-medium text-ink-700">{label}</p>
      <dl className="mt-1 grid max-w-xl grid-cols-1 gap-2 text-small sm:grid-cols-3">
        <div>
          <dt className="text-caption uppercase tracking-[0.1em] text-ink-500">
            Low
          </dt>
          <dd className="tnum whitespace-nowrap font-medium text-ink-900">
            {money(currency, low)}
          </dd>
        </div>
        <div>
          <dt className="text-caption uppercase tracking-[0.1em] text-ink-500">
            Base
          </dt>
          <dd className="tnum whitespace-nowrap font-semibold text-ink-950">
            {money(currency, base)}
          </dd>
        </div>
        <div>
          <dt className="text-caption uppercase tracking-[0.1em] text-ink-500">
            High
          </dt>
          <dd className="tnum whitespace-nowrap font-medium text-ink-900">
            {money(currency, high)}
          </dd>
        </div>
      </dl>
      <p className="mt-1 text-caption text-ink-500">
        {projectionSourceLabel(source)} · sample{" "}
        {formatReportCount(sampleSize)}
      </p>
    </div>
  );
}

function bucketRange(
  bucket: PurchaseOrderReportProjectedBucket,
  label: string,
  currency: CurrencyInfo | null
) {
  return (
    <Range
      label={label}
      low={bucket.amountLowMinor}
      base={bucket.amountBaseMinor}
      high={bucket.amountHighMinor}
      currency={currency}
      source={bucket.source}
      sampleSize={bucket.sampleSize}
    />
  );
}

function remainderRange(
  remainder: PurchaseOrderReportCurrentProjectionRemainder,
  currency: CurrencyInfo | null
) {
  return (
    <Range
      label="Rest of this month"
      low={remainder.low}
      base={remainder.base}
      high={remainder.high}
      currency={currency}
      source={remainder.source}
      sampleSize={remainder.sampleSize}
    />
  );
}

/**
 * Issue #644: the one compact forecast readout the overview keeps visible. It
 * carries the server's status, mode and stable reason; every basis, method,
 * range and update sentence stays in the full summary inside Details.
 */
export function PurchaseOrderReportProjectionStatus({
  report,
}: PurchaseOrderReportProjectionStatusProps) {
  const current = report.currentProjection;
  const available = current.status === "available" && current.mode !== "none";
  const detail = available
    ? basisLabel(report, current.mode === "month" ? "month" : "day", false)
    : current.mode === "none"
      ? ""
      : projectionReasonLabel(current.reason);

  return (
    <div
      data-report-forecast-status=""
      data-projection-status={current.status}
      data-projection-mode={current.mode}
      data-projection-reason={current.reason ?? ""}
      className="mb-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-caption leading-relaxed text-ink-500"
    >
      <span className="text-micro font-medium uppercase tracking-[0.12em] text-ink-500">
        Forecast
      </span>
      <Badge
        tone={available ? "accent" : "neutral"}
      >
        {available
          ? "Projected"
          : current.mode === "none"
            ? "Not applicable"
            : "Unavailable"}
      </Badge>
      <span>
        {modeLabel(current.mode)}
        {detail === "" ? "" : ` · ${detail}`}
      </span>
    </div>
  );
}

export function PurchaseOrderReportProjectionSummary({
  report,
  currency,
  plain = false,
}: PurchaseOrderReportProjectionSummaryProps) {
  const current = report.currentProjection;
  const available = current.status === "available";
  const bucket = current.buckets[0];
  const remainder = current.currentMonth?.remainder;
  const units = current.mode === "month" ? "month" : "day";

  return (
    <div
      data-report-projection-summary=""
      data-projection-status={current.status}
      data-projection-mode={current.mode}
      data-projection-reason={current.reason ?? ""}
      className={
        plain
          ? "mt-0"
          : "mt-4 rounded-xl border border-rule bg-paper-subtle/50 p-4"
      }
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-small font-semibold text-ink-900">
          Current-period projection
        </h3>
        <Badge tone={available ? "accent" : "neutral"}>
          {available ? "Projected" : "Unavailable"}
        </Badge>
        <span className="text-caption text-ink-500">
          {modeLabel(current.mode)}
        </span>
      </div>

      {!available ? (
        <p
          data-projection-unavailable=""
          className="mt-2 max-w-3xl text-small leading-relaxed text-ink-600"
        >
          {projectionReasonLabel(current.reason)}
          {current.mode === "none"
            ? ""
            : " The chart draws actual periods only."}
        </p>
      ) : remainder !== undefined ? (
        remainderRange(remainder, currency)
      ) : bucket !== undefined ? (
        bucketRange(
          bucket,
          current.mode === "month" ? "Per remaining month" : "Per remaining day",
          currency
        )
      ) : (
        <p
          data-projection-empty=""
          className="mt-2 text-small leading-relaxed text-ink-600"
        >
          {current.mode === "month"
            ? "No remaining months to project in this year."
            : "No remaining days to project in this month."}
        </p>
      )}

      {available && current.mode === "month" && bucket !== undefined ? (
        <div className="mt-2 border-t border-rule pt-2">
          {bucketRange(bucket, "Per remaining month", currency)}
        </div>
      ) : null}

      {current.mode !== "none" ? (
        <>
          <p className="mt-3 text-caption leading-relaxed text-ink-600">
            <span className="font-medium text-ink-700">Basis:</span>{" "}
            {basisLabel(report, units)}
          </p>
          {current.method.trim() !== "" ? (
            <p className="mt-1 max-w-3xl text-caption leading-relaxed text-ink-500">
              <span className="font-medium text-ink-600">Method:</span>{" "}
              {current.method}
            </p>
          ) : null}
          <p className="mt-1 text-caption text-ink-500">
            Updates as data arrives. Projected figures never enter the KPIs or
            the actual table totals.
          </p>
        </>
      ) : null}
    </div>
  );
}
