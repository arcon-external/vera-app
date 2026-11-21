// Issue #643: the independent By category comparison browser.
//
// It is presentation only. Selection, source acquisition and export gating live
// in `usePOReportCategoryComparison`; this module owns the stable section
// trigger, the remembered All display control, single/subset card navigation
// and the four truthful presentations: explicit empty, one full-width card, the
// shared two-column comparison grid, and the All/Aggregate wide table. Every
// figure is read from the supplied all-category snapshot; no row is summed and
// no category is inferred from Performance's own restriction.

import { useEffect, useRef, type JSX, type RefObject } from "react";
import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import {
  categorySelectionSummary,
  type PurchaseOrderCategoryItem,
  type PurchaseOrderCategoryReadout,
  type ReportComparisonLayout,
} from "../poReportComparison";
import { formatReportBps, formatReportCount, formatReportDate } from "../poReportPage";
import type { PurchaseOrderReportMetrics } from "../poReport";
import { PurchaseOrderReportComparisonGrid } from "./PurchaseOrderReportComparisonGrid";
import { PurchaseOrderReportMetricTable } from "./PurchaseOrderReportMetricTable";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

export interface PurchaseOrderReportCategoryBrowserProps {
  codes: readonly string[];
  activeCode: string | null;
  allSelected: boolean;
  layout: ReportComparisonLayout;
  readout: PurchaseOrderCategoryReadout;
  entry: CurrencyInfo | null;
  triggerRef: RefObject<HTMLButtonElement | null>;
  onOpenPicker: () => void;
  onLayoutChange: (layout: ReportComparisonLayout) => void;
  onPrevious: () => void;
  onNext: () => void;
  onRetry: () => void;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

function scopeLabel(readout: PurchaseOrderCategoryReadout): string {
  return [
    readout.rangeLabel === "" ? "-" : readout.rangeLabel,
    readout.currency === "" ? "-" : readout.currency,
  ].join(" · ");
}

/**
 * One selected Business category as a full-width vertical card: its heading
 * above the shared six-metric table, exact tabular money, danger-toned negative
 * margin and an inline uncosted disclosure when the category holds uncosted
 * amount. Category evidence is actual only: there is no ghost or forecast
 * treatment here.
 */
export function PurchaseOrderReportCategoryCard({
  item,
  readout,
  entry,
  active = false,
}: {
  item: PurchaseOrderCategoryItem;
  readout: PurchaseOrderCategoryReadout;
  entry: CurrencyInfo | null;
  active?: boolean;
}): JSX.Element {
  return (
    <Card
      className={`min-w-0 ${
        active ? "ring-2 ring-accent-400/70 ring-offset-2 ring-offset-paper" : ""
      }`}
      data-category-card=""
      data-category-key={item.code}
      data-category-active={active ? "true" : "false"}
    >
      <div className="flex flex-wrap items-start justify-between gap-2 px-4 pt-4">
        <h3
          data-category-card-heading=""
          className="min-w-0 text-title font-semibold text-ink-950"
        >
          {item.label}
        </h3>
      </div>
      <div className="px-4 pb-4">
        <p className="mt-1 text-small leading-relaxed text-ink-500">
          {scopeLabel(readout)}
          {readout.asOf === ""
            ? null
            : ` · As of ${formatReportDate(readout.asOf)}`}
        </p>
        <div className="mt-3">
          <PurchaseOrderReportMetricTable
            metrics={item.metrics}
            entry={entry}
            ariaLabel={`${item.label} metrics`}
            metricScope="category"
          />
        </div>
        {item.metrics.uncostedMinor > 0 ? (
          <p
            data-category-uncosted=""
            className="mt-3 text-small leading-relaxed text-ink-600"
          >
            {entry === null ? (
              "Part of this category's amount is not costed."
            ) : (
              `${formatMoney(entry, item.metrics.uncostedMinor)} of this category's amount is not costed.`
            )}{" "}
            Uncosted amount is never counted as margin.
          </p>
        ) : null}
      </div>
    </Card>
  );
}

function totalMetricCells(
  metrics: PurchaseOrderReportMetrics,
  entry: CurrencyInfo | null
): { label: string; value: string; negative?: boolean }[] {
  return [
    { label: "POs", value: formatReportCount(metrics.poCount) },
    { label: "Amount", value: money(entry, metrics.amountMinor) },
    { label: "Costed", value: money(entry, metrics.costedMinor) },
    { label: "Capital", value: money(entry, metrics.capitalMinor) },
    {
      label: "Margin",
      value: money(entry, metrics.marginMinor),
      negative: metrics.marginMinor < 0,
    },
    { label: "Margin %", value: formatReportBps(metrics.marginBps) },
  ];
}

/**
 * The All/Aggregate combined wide table. Rows are every row the all-category
 * snapshot supplied, in supplied order; the footer is that snapshot's own
 * server `totals`, never a sum of the rows.
 */
export function PurchaseOrderReportCategoryAggregate({
  readout,
  entry,
  onRetry,
}: {
  readout: PurchaseOrderCategoryReadout;
  entry: CurrencyInfo | null;
  onRetry: () => void;
}): JSX.Element {
  const anyUncosted = readout.supplied.some(
    (item) => item.metrics.uncostedMinor > 0
  );
  return (
    <Card className="min-w-0 overflow-hidden" data-category-aggregate="">
      <div className="px-5 pt-5">
        <SectionHeading
          title="All categories"
          hint={`${readout.supplied.length} supplied`}
        />
      </div>
      <div className="px-5 pb-5 pt-3">
        {readout.total === null ? (
          <div className="grid gap-3">
            <StatusBanner tone="danger">
              The combined category totals are unavailable.
            </StatusBanner>
            <div>
              <Button aria-label="Retry categories" onClick={onRetry}>
                Retry
              </Button>
            </div>
          </div>
        ) : (
          <>
            <div className="overflow-x-auto scroll-thin">
              <table
                aria-label="Category totals"
                className="w-full min-w-[760px] text-left text-body"
              >
                <thead>
                  <tr className="border-y border-rule bg-paper-subtle/60 text-micro uppercase tracking-[0.1em] text-ink-500">
                    <th className="px-4 py-3 font-medium">Category</th>
                    <th className="py-3 pr-4 text-right font-medium">POs</th>
                    <th className="py-3 pr-4 text-right font-medium">Amount</th>
                    <th className="py-3 pr-4 text-right font-medium">Costed</th>
                    <th className="py-3 pr-4 text-right font-medium">Capital</th>
                    <th className="py-3 pr-4 text-right font-medium">Margin</th>
                    <th className="py-3 pr-5 text-right font-medium">Margin %</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-rule">
                  {readout.supplied.map((item) => (
                    <tr key={item.code} data-category-row={item.code}>
                      <td className="px-4 py-3 align-middle font-medium text-ink-900">
                        {item.label}
                      </td>
                      <td className="tnum whitespace-nowrap py-3 pr-4 text-right align-middle text-ink-700">
                        {formatReportCount(item.metrics.poCount)}
                      </td>
                      <td className="tnum whitespace-nowrap py-3 pr-4 text-right align-middle text-ink-700">
                        {money(entry, item.metrics.amountMinor)}
                      </td>
                      <td className="tnum whitespace-nowrap py-3 pr-4 text-right align-middle text-ink-700">
                        {money(entry, item.metrics.costedMinor)}
                      </td>
                      <td className="tnum whitespace-nowrap py-3 pr-4 text-right align-middle text-ink-700">
                        {money(entry, item.metrics.capitalMinor)}
                      </td>
                      <td
                        className={`tnum whitespace-nowrap py-3 pr-4 text-right align-middle font-medium ${
                          item.metrics.marginMinor < 0
                            ? "text-danger-700"
                            : "text-ink-900"
                        }`}
                      >
                        {money(entry, item.metrics.marginMinor)}
                      </td>
                      <td className="tnum whitespace-nowrap py-3 pr-5 text-right align-middle text-ink-700">
                        {formatReportBps(item.metrics.marginBps)}
                      </td>
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr
                    data-category-total=""
                    className="border-t border-rule-strong bg-paper-subtle/40 font-medium text-ink-900"
                  >
                    <td className="px-4 py-3">Total</td>
                    {totalMetricCells(readout.total, entry).map((value) => (
                      <td
                        key={value.label}
                        data-category-total-metric={value.label}
                        className={`tnum whitespace-nowrap py-3 pr-4 text-right align-middle ${
                          value.negative ? "text-danger-700" : "text-ink-900"
                        }`}
                      >
                        {value.value}
                      </td>
                    ))}
                  </tr>
                </tfoot>
              </table>
            </div>
            {anyUncosted ? (
              <p
                data-category-uncosted=""
                className="mt-3 text-small leading-relaxed text-ink-600"
              >
                Some category amounts are not costed. Uncosted amount is never
                counted as margin; each row's Costed and Margin figures reflect
                costed purchase orders only.
              </p>
            ) : null}
            <p className="mt-2 text-caption leading-relaxed text-ink-500">
              The total is the all-category snapshot's own server total, not a
              sum of the rows above. Actual figures only.
            </p>
          </>
        )}
      </div>
    </Card>
  );
}

export function PurchaseOrderReportCategoryBrowser({
  codes,
  activeCode,
  allSelected,
  layout,
  readout,
  entry,
  triggerRef,
  onOpenPicker,
  onLayoutChange,
  onPrevious,
  onNext,
  onRetry,
}: PurchaseOrderReportCategoryBrowserProps): JSX.Element {
  const cardRefs = useRef(new Map<string, HTMLDivElement | null>());
  const summary = categorySelectionSummary(
    readout.selected,
    readout.supplied.length,
    readout.status
  );
  const ready = readout.status === "ready";
  const aggregateMode = allSelected && layout === "aggregate";
  const showLayoutToggle = ready && allSelected && readout.supplied.length > 0;
  const showNavigation = ready && codes.length > 0 && !aggregateMode;
  // A single category walks the whole supplied Business order and replaces the
  // selection; a subset (or All/Individual) only moves focus among the cards.
  const navigationCodes =
    codes.length === 1
      ? readout.supplied.map((item) => item.code)
      : [...codes];
  const effectiveActive =
    activeCode !== null && codes.includes(activeCode)
      ? activeCode
      : codes[0] ?? null;
  const navigationActive =
    codes.length === 1 ? codes[0] : effectiveActive;
  const navigationIndex =
    navigationActive === null ? -1 : navigationCodes.indexOf(navigationActive);
  const previousDisabled = navigationIndex <= 0;
  const nextDisabled =
    navigationIndex < 0 || navigationIndex >= navigationCodes.length - 1;
  const selectedByCode = new Map(
    readout.selected.map((item) => [item.code, item])
  );

  useEffect(() => {
    if (!showNavigation || codes.length <= 1 || effectiveActive === null) {
      return;
    }
    const card = cardRefs.current.get(effectiveActive);
    if (card === undefined || card === null) return;
    card.scrollIntoView({ block: "nearest" });
  }, [codes, effectiveActive, showNavigation]);

  return (
    <section
      data-category-browser=""
      aria-label="Category comparison"
      className="min-w-0"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <SectionHeading
            title="By category"
            hint="Independent of Performance"
          />
          <p className="mt-1 max-w-2xl text-small leading-relaxed text-ink-500">
            Choose one or more Business categories. This section always reads
            the unrestricted all-category snapshot at Performance's date range,
            currency and business date; Performance's own category restriction
            never narrows it.
          </p>
          <p
            data-category-scope=""
            className="mt-1 text-small leading-relaxed text-ink-500"
          >
            {summary} · {scopeLabel(readout)}
            {readout.sourceLabel === ""
              ? null
              : ` · ${readout.sourceLabel}`}
            {readout.asOf === ""
              ? null
              : ` · As of ${formatReportDate(readout.asOf)}`}
            {readout.generatedAt === "" ? null : (
              <span className="tnum"> · snapshot {readout.generatedAt}</span>
            )}
            {readout.stale ? (
              <span
                data-category-stale=""
                data-category-refreshing={readout.refreshing ? "true" : "false"}
                className="ml-2 inline-flex align-middle"
              >
                <Badge tone="neutral">
                  {readout.refreshing ? "Updating…" : "Stale"}
                </Badge>
              </span>
            ) : null}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            ref={triggerRef}
            aria-label="Choose categories"
            data-category-picker-trigger=""
            data-category-selection-summary={summary}
            disabled={!ready}
            onClick={onOpenPicker}
          >
            Categories: {summary}
          </Button>
          {showLayoutToggle ? (
            <div
              role="group"
              aria-label="All categories display"
              className="flex items-center gap-1 rounded-lg border border-rule bg-paper-subtle p-1"
            >
              <Button
                size="xs"
                variant={layout === "aggregate" ? "dark" : "ghost"}
                aria-label="Aggregate"
                aria-pressed={layout === "aggregate"}
                data-category-layout="aggregate"
                onClick={() => onLayoutChange("aggregate")}
              >
                Aggregate
              </Button>
              <Button
                size="xs"
                variant={layout === "individual" ? "dark" : "ghost"}
                aria-label="Individual"
                aria-pressed={layout === "individual"}
                data-category-layout="individual"
                onClick={() => onLayoutChange("individual")}
              >
                Individual
              </Button>
            </div>
          ) : null}
          {showNavigation ? (
            <div className="flex items-center gap-2">
              <Button
                aria-label="Previous category"
                data-category-nav="previous"
                disabled={previousDisabled}
                onClick={onPrevious}
              >
                Previous
              </Button>
              <Button
                aria-label="Next category"
                data-category-nav="next"
                disabled={nextDisabled}
                onClick={onNext}
              >
                Next
              </Button>
            </div>
          ) : null}
        </div>
      </div>

      <div className="mt-4">
        {readout.status === "loading" ? (
          <Card className="min-w-0 p-5">
            <p role="status" className="text-body text-ink-500">
              Loading categories…
            </p>
          </Card>
        ) : null}
        {readout.status === "error" ? (
          <Card className="min-w-0 p-5">
            <div className="grid gap-3">
              <StatusBanner tone="danger">{readout.error}</StatusBanner>
              <div>
                <Button aria-label="Retry categories" onClick={onRetry}>
                  Retry
                </Button>
              </div>
            </div>
          </Card>
        ) : null}
        {ready ? (
          <div className="grid gap-3">
            {readout.stale ? (
              <p
                data-category-stale-note=""
                className="text-small leading-relaxed text-ink-500"
              >
                Showing the previous all-category snapshot until the new one
                arrives.
              </p>
            ) : null}
            {readout.error === "" ? null : (
              <div className="flex flex-wrap items-center justify-between gap-3">
                <StatusBanner tone="danger">
                  The categories could not be refreshed: {readout.error}{" "}
                  Showing the previous figures.
                </StatusBanner>
                <Button aria-label="Retry categories" onClick={onRetry}>
                  Retry
                </Button>
              </div>
            )}
            {readout.scopeMatchesOverview ? null : (
              <StatusBanner tone="info">
                This all-category snapshot has a different business date from
                the overview. Export stays off until both are refreshed
                together.
              </StatusBanner>
            )}
            {codes.length === 0 ? (
              <Card
                className="min-w-0 p-6"
                data-category-empty=""
              >
                <h3 className="text-title font-semibold text-ink-950">
                  Choose categories
                </h3>
                <p className="mt-1.5 max-w-2xl text-body leading-relaxed text-ink-500">
                  No categories are selected. Open the category picker to
                  compare one or more Business categories; an empty selection
                  exports an explicit empty category section, not a zero-valued
                  business report.
                </p>
              </Card>
            ) : aggregateMode ? (
              <PurchaseOrderReportCategoryAggregate
                readout={readout}
                entry={entry}
                onRetry={onRetry}
              />
            ) : codes.length === 1 ? (
              (() => {
                const item = selectedByCode.get(codes[0]);
                if (item === undefined) return null;
                return (
                  <PurchaseOrderReportCategoryCard
                    item={item}
                    readout={readout}
                    entry={entry}
                    active
                  />
                );
              })()
            ) : (
              <PurchaseOrderReportComparisonGrid>
                {codes.map((code) => {
                  const item = selectedByCode.get(code);
                  if (item === undefined) return null;
                  return (
                    <div
                      key={code}
                      ref={(node) => {
                        cardRefs.current.set(code, node);
                      }}
                      className="min-w-0"
                    >
                      <PurchaseOrderReportCategoryCard
                        item={item}
                        readout={readout}
                        entry={entry}
                        active={code === effectiveActive}
                      />
                    </div>
                  );
                })}
              </PurchaseOrderReportComparisonGrid>
            )}
          </div>
        ) : null}
      </div>
    </section>
  );
}
