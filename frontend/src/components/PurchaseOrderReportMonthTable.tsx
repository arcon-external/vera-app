// Issue #641/#642: one selected month, six vertical metrics. The actual,
// future-dated recorded and estimated quantities are separate blocks; the
// estimate alone gets the muted/hatched treatment and an accessible label, so
// a reader never mistakes it for recorded evidence. The server owns every
// figure and every percentage. The card composition reuses this same vertical
// table instead of introducing a parallel per-month implementation.

import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import {
  monthLabel,
  monthUnavailableLabel,
  type PurchaseOrderMonthEstimate,
  type PurchaseOrderMonthReadout,
} from "../poReportComparison";
import {
  formatReportBps,
  formatReportCount,
  formatReportDate,
  projectionSourceLabel,
  reportCategoryLabel,
} from "../poReportPage";
import type { PurchaseOrderReportMetrics } from "../poReport";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";
import { PurchaseOrderReportMetricTable } from "./PurchaseOrderReportMetricTable";

export interface PurchaseOrderReportMonthTableProps {
  readout: PurchaseOrderMonthReadout;
  entry: CurrencyInfo | null;
  onPrevious: () => void;
  onNext: () => void;
  onRetry: () => void;
}

export interface PurchaseOrderReportMonthCardProps {
  readout: PurchaseOrderMonthReadout;
  entry: CurrencyInfo | null;
  /** The focused comparison card; arrows move this in subset/All-Individual. */
  active?: boolean;
  /** Only the first visible table carries the chart's period anchor. */
  anchor?: boolean;
  onRetry: () => void;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

function EvidenceGrid({
  metrics,
  entry,
  testId,
}: {
  metrics: PurchaseOrderReportMetrics;
  entry: CurrencyInfo | null;
  testId: string;
}) {
  const values: { label: string; value: string; negative?: boolean }[] = [
    { label: "POs", value: formatReportCount(metrics.poCount) },
    { label: "PO amount", value: money(entry, metrics.amountMinor) },
    { label: "Costed amount", value: money(entry, metrics.costedMinor) },
    { label: "Capital", value: money(entry, metrics.capitalMinor) },
    {
      label: "PO margin",
      value: money(entry, metrics.marginMinor),
      negative: metrics.marginMinor < 0,
    },
    { label: "PO margin %", value: formatReportBps(metrics.marginBps) },
  ];
  return (
    <dl
      data-month-evidence={testId}
      className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2 lg:grid-cols-3"
    >
      {values.map((value) => (
        <div
          key={value.label}
          data-month-recorded-metric={value.label}
          className="min-w-0"
        >
          <dt className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
            {value.label}
          </dt>
          <dd
            className={`tnum mt-0.5 whitespace-nowrap text-body ${
              value.negative ? "text-danger-700" : "text-ink-900"
            }`}
          >
            {value.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

function EstimateBlock({
  estimate,
  entry,
}: {
  estimate: PurchaseOrderMonthEstimate;
  entry: CurrencyInfo | null;
}) {
  const label =
    estimate.kind === "remainder"
      ? "Estimated remainder (not actual)"
      : "Estimated forecast (not actual)";
  const values: {
    label: string;
    value?: string;
    amount?: { base: string; low: string; high: string };
    negative?: boolean;
  }[] = [
    { label: "POs", value: "-" },
    {
      label: "PO amount",
      amount: {
        base: money(entry, estimate.base),
        low: money(entry, estimate.low),
        high: money(entry, estimate.high),
      },
    },
    { label: "Costed amount", value: "-" },
    { label: "Capital", value: money(entry, estimate.capital) },
    {
      label: "PO margin",
      value: money(entry, estimate.margin),
      negative: estimate.margin < 0,
    },
    { label: "PO margin %", value: formatReportBps(estimate.marginBps) },
  ];
  return (
    <div
      role="group"
      data-month-evidence="estimated"
      data-estimated="true"
      aria-label={`${label}. Estimates are not actual evidence.`}
      className="rounded-md border border-dashed border-rule bg-paper-subtle/50 p-3"
    >
      <p className="mb-2 text-small font-semibold text-ink-600">{label}</p>
      <dl
        data-month-estimate-grid=""
        className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2 lg:grid-cols-3"
      >
        {values.map((value) => (
          <div
            key={value.label}
            data-month-estimate-metric={value.label}
            className={`min-w-0 ${
              value.amount === undefined ? "" : "sm:col-span-2 lg:col-span-3"
            }`}
          >
            <dt className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
              {value.label}
            </dt>
            {value.amount === undefined ? (
              <dd
                className={`tnum mt-0.5 whitespace-nowrap text-body ${
                  value.negative ? "text-danger-700" : "text-ink-600"
                }`}
              >
                {value.value}
              </dd>
            ) : (
              <dd
                data-month-estimate-amount=""
                className="mt-0.5 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-body text-ink-600"
              >
                <span
                  data-amount-part="base"
                  className="tnum whitespace-nowrap font-medium text-ink-700"
                >
                  {value.amount.base}
                </span>
                <span className="inline-flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <span className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
                    Range
                  </span>
                  <span className="inline-flex items-baseline gap-1 whitespace-nowrap">
                    <span className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
                      Low
                    </span>
                    <span data-amount-part="low" className="tnum whitespace-nowrap">
                      {value.amount.low}
                    </span>
                  </span>
                  <span className="inline-flex items-baseline gap-1 whitespace-nowrap">
                    <span className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
                      High
                    </span>
                    <span data-amount-part="high" className="tnum whitespace-nowrap">
                      {value.amount.high}
                    </span>
                  </span>
                </span>
              </dd>
            )}
          </div>
        ))}
      </dl>
      <p className="mt-2 text-caption leading-relaxed text-ink-500">
        POs and Costed amount are not forecast. Sample {formatReportCount(
          estimate.sampleSize
        )}{" "}
        · {projectionSourceLabel(estimate.source)}
      </p>
    </div>
  );
}

/** The section's own scope, as-of, currency and snapshot provenance. */
function MonthScope({ readout }: { readout: PurchaseOrderMonthReadout }) {
  return (
    <p
      data-month-scope=""
      className="mt-1 text-small leading-relaxed text-ink-500"
    >
      {reportCategoryLabel(readout.category)} · {readout.currency || "-"}
      {readout.sourceLabel === "" ? null : ` · ${readout.sourceLabel}`}
      {readout.asOf === ""
        ? null
        : ` · As of ${formatReportDate(readout.asOf)}`}
      {readout.generatedAt === "" ? null : (
        <span className="tnum"> · snapshot {readout.generatedAt}</span>
      )}
    </p>
  );
}

/**
 * The vertical six-metric table plus its truthful loading/error/zero/estimate
 * states. `anchor` is true for the one table the chart's `#report-period-table`
 * link targets; in a comparison grid only the first card carries it.
 */
function MonthReadoutBody({
  readout,
  entry,
  onRetry,
  anchor,
}: {
  readout: PurchaseOrderMonthReadout;
  entry: CurrencyInfo | null;
  onRetry: () => void;
  anchor: boolean;
}) {
  const primary = readout.primary;

  return (
    <>
      {readout.status === "loading" ? (
        <p role="status" className="text-body text-ink-500">
          Loading {readout.month === "" ? "month" : monthLabel(readout.month)}…
        </p>
      ) : null}
      {readout.status === "error" ? (
        <div className="grid gap-3">
          <StatusBanner tone="danger">{readout.error}</StatusBanner>
          <div>
            <Button aria-label="Retry month" onClick={onRetry}>
              Retry
            </Button>
          </div>
        </div>
      ) : null}
      {readout.status === "ready" ? (
        <div className="grid gap-4">
          {readout.stale ? (
            <p className="flex flex-wrap items-center gap-2 text-small leading-relaxed text-ink-500">
              <span
                data-month-stale=""
                data-month-refreshing={
                  readout.refreshing ? "true" : "false"
                }
                className="inline-flex items-center"
              >
                <Badge tone="neutral">
                  {readout.refreshing ? "Updating…" : "Stale"}
                </Badge>
              </span>
              <span>
                Showing this month's previous snapshot until the new one
                arrives.
              </span>
            </p>
          ) : null}
          {readout.error === "" ? null : (
            <div className="grid gap-3">
              <StatusBanner tone="danger">
                This month could not be refreshed: {readout.error} Showing the
                previous figures.
              </StatusBanner>
              <div>
                <Button aria-label="Retry month" onClick={onRetry}>
                  Retry
                </Button>
              </div>
            </div>
          )}
          {readout.scopeMatchesOverview ? null : (
            <StatusBanner tone="info">
              This month's snapshot has a different business date from the
              overview. Export stays off until both are refreshed together.
            </StatusBanner>
          )}
          {readout.message === "" ? null : (
            <p className="text-small leading-relaxed text-ink-500">
              {readout.message}
            </p>
          )}
          <div>
            {readout.primaryLabel === "" ? null : (
              <p
                data-month-primary-label={readout.primaryLabel}
                className="mb-2 text-caption font-medium uppercase tracking-[0.1em] text-ink-500"
              >
                {readout.primaryLabel}
              </p>
            )}
            <PurchaseOrderReportMetricTable
              metrics={primary}
              entry={entry}
              ariaLabel={
                anchor
                  ? "Period totals"
                  : `${monthLabel(readout.month)} metrics`
              }
              id={anchor ? "report-period-table" : undefined}
              tabIndex={anchor ? -1 : undefined}
              metricScope="month"
            />
          </div>

          {readout.recordedFuture === null ? null : (
            <div data-month-evidence="recorded-future-label">
              <p className="mb-2 text-small font-semibold text-ink-900">
                Future-dated recorded
              </p>
              <EvidenceGrid
                metrics={readout.recordedFuture}
                entry={entry}
                testId="recorded-future"
              />
            </div>
          )}

          {readout.estimate === null ? (
            readout.unavailableReason === "" ? null : (
              <div data-month-evidence="unavailable">
                <p className="text-small font-semibold text-ink-700">
                  Forecast unavailable
                </p>
                <p className="text-small leading-relaxed text-ink-500">
                  {monthUnavailableLabel(readout.unavailableReason)}
                </p>
              </div>
            )
          ) : (
            <EstimateBlock estimate={readout.estimate} entry={entry} />
          )}

          <p className="text-caption leading-relaxed text-ink-500">
            Actual, future-dated recorded and estimated figures are separate.
            Estimates are never added to recorded evidence or to the month's
            actual figures.
          </p>
        </div>
      ) : null}
    </>
  );
}

/**
 * #641's standalone single-month browser. It stays callable directly by
 * fixture/harness callers and keeps its own heading, scope, navigation and
 * chart anchor.
 */
export function PurchaseOrderReportMonthTable({
  readout,
  entry,
  onPrevious,
  onNext,
  onRetry,
}: PurchaseOrderReportMonthTableProps) {
  const heading = readout.month === "" ? "By period" : monthLabel(readout.month);

  return (
    <Card className="min-w-0" data-month-browser="">
      <div className="flex flex-wrap items-start justify-between gap-3 px-5 pt-5">
        <div className="min-w-0">
          <SectionHeading
            title="By period"
            hint="One calendar month. Navigating it never changes Performance."
          />
          <h3 className="mt-2 text-title font-semibold text-ink-950">
            {heading}
          </h3>
          <MonthScope readout={readout} />
        </div>
        <div className="flex items-center gap-2">
          <Button
            aria-label="Previous month"
            data-month-nav="previous"
            onClick={onPrevious}
          >
            Previous
          </Button>
          <Button aria-label="Next month" data-month-nav="next" onClick={onNext}>
            Next
          </Button>
        </div>
      </div>

      <div className="px-5 pb-5 pt-3">
        <MonthReadoutBody
          readout={readout}
          entry={entry}
          onRetry={onRetry}
          anchor
        />
      </div>
    </Card>
  );
}

/**
 * One comparison card: the same vertical metric table, headed by its own month
 * and carrying its own scope/provenance. It is the unit the comparison grid
 * arranges; no second table implementation exists.
 */
export function PurchaseOrderReportMonthCard({
  readout,
  entry,
  active = false,
  anchor = false,
  onRetry,
}: PurchaseOrderReportMonthCardProps) {
  const heading = readout.month === "" ? "By period" : monthLabel(readout.month);
  return (
    <Card
      className={`min-w-0 ${
        active ? "ring-2 ring-accent-400/70 ring-offset-2 ring-offset-paper" : ""
      }`}
      data-month-card=""
      data-month-key={readout.month}
      data-month-active={active ? "true" : "false"}
    >
      <div className="flex flex-wrap items-start justify-between gap-2 px-4 pt-4">
        <h3
          data-month-card-heading=""
          className="min-w-0 text-title font-semibold text-ink-950"
        >
          {heading}
        </h3>
      </div>
      <div className="px-4 pb-4">
        <MonthScope readout={readout} />
        <div className="mt-3">
          <MonthReadoutBody
            readout={readout}
            entry={entry}
            onRetry={onRetry}
            anchor={anchor}
          />
        </div>
      </div>
    </Card>
  );
}
