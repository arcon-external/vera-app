// Issue #642: the month comparison browser. It composes the one stable
// section-level picker trigger, the remembered All display control, card
// navigation and the three truthful presentations: explicit empty, one
// vertical table, and the comparison grid/one-year aggregate. It never owns
// selection or fetches; the page hook does, so a picker year page or a layout
// change cannot mutate the query population.

import { useEffect, useRef, type RefObject } from "react";
import type { CurrencyInfo } from "../format";
import {
  monthSelectionSummary,
  type PurchaseOrderMonthAggregate,
  type PurchaseOrderMonthReadout,
  type ReportMonthLayout,
} from "../poReportComparison";
import { PurchaseOrderReportComparisonGrid } from "./PurchaseOrderReportComparisonGrid";
import { PurchaseOrderReportMonthAggregate } from "./PurchaseOrderReportMonthAggregate";
import { PurchaseOrderReportMonthCard } from "./PurchaseOrderReportMonthTable";
import { Button, Card, SectionHeading, StatusBanner } from "./ui";

export interface PurchaseOrderReportMonthBrowserProps {
  months: readonly string[];
  activeMonth: string | null;
  allYear: string | null;
  layout: ReportMonthLayout;
  notice: string;
  readouts: readonly PurchaseOrderMonthReadout[];
  aggregate: PurchaseOrderMonthAggregate | null;
  entry: CurrencyInfo | null;
  triggerRef: RefObject<HTMLButtonElement | null>;
  onOpenPicker: () => void;
  onLayoutChange: (layout: ReportMonthLayout) => void;
  onPrevious: () => void;
  onNext: () => void;
  onRetryMonth: (month: string) => void;
}

export function PurchaseOrderReportMonthBrowser({
  months,
  activeMonth,
  allYear,
  layout,
  notice,
  readouts,
  aggregate,
  entry,
  triggerRef,
  onOpenPicker,
  onLayoutChange,
  onPrevious,
  onNext,
  onRetryMonth,
}: PurchaseOrderReportMonthBrowserProps) {
  const cardRefs = useRef(new Map<string, HTMLDivElement | null>());
  const summary = monthSelectionSummary(months);
  const aggregateMode = allYear !== null && layout === "aggregate";
  const showNavigation = months.length > 0 && !aggregateMode;
  const activeIndex = months.findIndex(
    (month) => month === (activeMonth ?? months[0])
  );
  const previousDisabled =
    months.length > 1 && (activeIndex < 0 || activeIndex <= 0);
  const nextDisabled =
    months.length > 1 && activeIndex >= months.length - 1;
  const snapshotStamps = new Set(
    readouts
      .map((readout) => readout.generatedAt)
      .filter((generatedAt) => generatedAt !== "")
  );
  // The note names cards, so it belongs to card layouts only. An aggregate has
  // no cards and draws every row and its Recorded total from one annual source;
  // the aggregate itself names that source. Cross-snapshot readouts can still
  // exist beside it (the day-snapshot current month), but they are not what it
  // displays.
  const showProvenanceNote = !aggregateMode && snapshotStamps.size > 1;
  // Issue #645: the browser owns no source itself; it surfaces the state the
  // cards and the aggregate already carry. The attributes name the section's
  // aggregate freshness without adding a second banner or changing the
  // trigger/navigation/grid structure.
  const staleEvidence =
    aggregate?.stale === true ||
    readouts.some(
      (readout) => readout.status === "ready" && readout.stale === true
    );
  const refreshingEvidence =
    aggregate?.refreshing === true ||
    readouts.some((readout) => readout.refreshing === true);
  const refreshError =
    (aggregate !== null &&
      aggregate.status === "ready" &&
      aggregate.error !== "") ||
    readouts.some(
      (readout) => readout.status === "ready" && readout.error !== ""
    );

  // Subset/All-Individual arrows move focus and scroll; they never change
  // membership. Aggregate and empty have no card target, so they get no nav.
  useEffect(() => {
    if (!showNavigation || layout === "aggregate") return;
    if (months.length <= 1 || activeMonth === null) return;
    const card = cardRefs.current.get(activeMonth);
    if (card === undefined || card === null) return;
    card.scrollIntoView({ block: "nearest" });
  }, [activeMonth, layout, months, showNavigation]);

  return (
    <section
      data-month-browser=""
      data-month-browser-stale={staleEvidence ? "true" : "false"}
      data-month-browser-refreshing={refreshingEvidence ? "true" : "false"}
      data-month-browser-error={refreshError ? "true" : "false"}
      aria-label="Month comparison"
      className="min-w-0"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <SectionHeading
            title="By period"
            hint="Independent of Performance"
          />
          <p className="mt-1 max-w-2xl text-small leading-relaxed text-ink-500">
            Choose one or more calendar months. Selected months inherit the
            Performance category and currency, but not its date range.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            ref={triggerRef}
            aria-label="Choose months"
            data-month-picker-trigger=""
            data-month-selection-summary={summary}
            onClick={onOpenPicker}
          >
            Months: {summary}
          </Button>
          {allYear === null ? null : (
            <div
              role="group"
              aria-label="All months display"
              className="flex items-center gap-1 rounded-lg border border-rule bg-paper-subtle p-1"
            >
              <Button
                size="xs"
                variant={layout === "aggregate" ? "dark" : "ghost"}
                aria-label="Aggregate"
                aria-pressed={layout === "aggregate"}
                data-month-layout="aggregate"
                onClick={() => onLayoutChange("aggregate")}
              >
                Aggregate
              </Button>
              <Button
                size="xs"
                variant={layout === "individual" ? "dark" : "ghost"}
                aria-label="Individual"
                aria-pressed={layout === "individual"}
                data-month-layout="individual"
                onClick={() => onLayoutChange("individual")}
              >
                Individual
              </Button>
            </div>
          )}
          {showNavigation ? (
            <div className="flex items-center gap-2">
              <Button
                aria-label="Previous month"
                data-month-nav="previous"
                disabled={previousDisabled}
                onClick={onPrevious}
              >
                Previous
              </Button>
              <Button
                aria-label="Next month"
                data-month-nav="next"
                disabled={nextDisabled}
                onClick={onNext}
              >
                Next
              </Button>
            </div>
          ) : null}
        </div>
      </div>

      {notice === "" ? null : (
        <div data-month-limit-notice="" role="alert" className="mt-3">
          <StatusBanner tone="danger">{notice}</StatusBanner>
        </div>
      )}
      {showProvenanceNote ? (
        <p
          data-month-provenance=""
          className="mt-3 text-caption leading-relaxed text-ink-500"
        >
          These months come from independently generated snapshots. Each card
          names its own timestamp; the set is not one atomic observation.
        </p>
      ) : null}

      <div className="mt-4">
        {months.length === 0 ? (
          <Card
            className="min-w-0 p-6"
            data-month-empty=""
            id="report-period-table"
            tabIndex={-1}
            aria-label="Period totals"
          >
            <h3 className="text-title font-semibold text-ink-950">
              Choose months
            </h3>
            <p className="mt-1.5 max-w-2xl text-body leading-relaxed text-ink-500">
              No months are selected. Open the month picker to compare one or
              more calendar months; an empty selection exports an explicit
              empty period section, not a zero-valued report.
            </p>
          </Card>
        ) : months.length === 1 ? (
          <PurchaseOrderReportMonthCard
            readout={readouts[0]}
            entry={entry}
            active
            anchor
            onRetry={() => onRetryMonth(months[0])}
          />
        ) : aggregateMode ? (
          aggregate === null ? (
            <Card className="min-w-0 p-5">
              <p role="status" className="text-body text-ink-500">
                Loading the {allYear} aggregate…
              </p>
            </Card>
          ) : (
            <PurchaseOrderReportMonthAggregate
              aggregate={aggregate}
              entry={entry}
              onRetry={() => onRetryMonth(`${aggregate.year}-01`)}
            />
          )
        ) : (
          <PurchaseOrderReportComparisonGrid>
            {months.map((month, index) => (
              <div
                key={month}
                ref={(node) => {
                  cardRefs.current.set(month, node);
                }}
                className="min-w-0"
              >
                <PurchaseOrderReportMonthCard
                  readout={readouts[index]}
                  entry={entry}
                  active={month === activeMonth}
                  anchor={index === 0}
                  onRetry={() => onRetryMonth(month)}
                />
              </div>
            ))}
          </PurchaseOrderReportComparisonGrid>
        )}
      </div>
    </section>
  );
}
