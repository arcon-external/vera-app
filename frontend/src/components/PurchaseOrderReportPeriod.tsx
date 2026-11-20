// Issue #638: the Month / Year period control that drives the whole report.
//
// The control is intentionally a set of pressed-state buttons rather than a
// free date picker: one click sends exactly the current calendar month or the
// current calendar year, with the bucket the current-period projection needs.
// "Custom range" leaves the mode, reveals the date controls and draws no ghost.

import type { ReportPeriodMode } from "../poReportPage";

export interface PurchaseOrderReportPeriodToggleProps {
  period: ReportPeriodMode | null;
  onPeriodChange: (period: ReportPeriodMode | null) => void;
}

const OPTIONS: Array<{ value: ReportPeriodMode; label: string; aria: string }> = [
  { value: "month", label: "Month", aria: "Month view" },
  { value: "year", label: "Year", aria: "Year view" },
];

export function PurchaseOrderReportPeriodToggle({
  period,
  onPeriodChange,
}: PurchaseOrderReportPeriodToggleProps) {
  return (
    <div
      role="group"
      aria-label="Report period"
      data-report-period-toggle=""
      className="inline-flex flex-wrap items-center gap-1.5 rounded-lg border border-rule bg-paper-subtle p-1"
    >
      {OPTIONS.map((option) => {
        const active = period === option.value;
        return (
          <button
            key={option.value}
            type="button"
            aria-label={option.aria}
            aria-pressed={active}
            data-report-period={option.value}
            onClick={() => onPeriodChange(option.value)}
            className={`inline-flex min-h-8 cursor-pointer items-center rounded-md px-3 text-small font-medium transition-colors ${
              active
                ? "bg-white text-ink-950 shadow-sm ring-1 ring-rule-strong"
                : "text-ink-600 hover:text-ink-900"
            }`}
          >
            {option.label}
          </button>
        );
      })}
      <button
        type="button"
        aria-label="Custom date range"
        aria-pressed={period === null}
        data-report-period="custom"
        data-report-period-custom={period === null ? "true" : "false"}
        onClick={() => onPeriodChange(null)}
        className={`inline-flex min-h-8 cursor-pointer items-center rounded-md px-3 text-small font-medium transition-colors ${
          period === null
            ? "bg-white text-ink-950 shadow-sm ring-1 ring-rule-strong"
            : "text-ink-600 hover:text-ink-900"
        }`}
      >
        Custom range
      </button>
    </div>
  );
}
