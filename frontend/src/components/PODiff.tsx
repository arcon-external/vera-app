// Issue #582: the two-sided difference primitives.
//
// The difference review is a diff, not two values inline: the PO entry is a
// neutral surface, the quotation revision is an accent-tinted one, and a
// divider (`≠` wide, `↓` narrow) separates them. A value can be selected, in
// which case it says "Will be used" while the other side stays readable but
// muted; an empty value is a dashed `(empty)` placeholder so a blank never
// looks like missing layout.
//
// Everything here is presentation. The comparison, the decisions and the
// finalize payload belong to ../poFinalize.ts and are untouched by the
// redesign.

import type { ReactNode } from "react";
import { Badge } from "./ui";

export type DiffSide = "po" | "quotation";

/** The four states a review row can carry, as the spec names them. */
export type DiffStatus = "changed" | "missing" | "extra" | "matched";

const SIDE_LABEL: Record<DiffSide, string> = {
  po: "PO ENTRY",
  quotation: "QUOTATION",
};

const SIDE_SURFACE: Record<DiffSide, string> = {
  po: "border-rule bg-paper-subtle",
  quotation: "border-accent-200 bg-accent-50/60",
};

const STATUS_LABEL: Record<DiffStatus, string> = {
  changed: "Changed",
  missing: "Missing",
  extra: "Extra",
  matched: "Matched",
};

const STATUS_TONE: Record<
  DiffStatus,
  "neutral" | "internal" | "success" | "accent" | "danger"
> = {
  changed: "accent",
  missing: "danger",
  extra: "internal",
  matched: "success",
};

/** A status chip. Colour is never the only signal: the word is always shown. */
export function DiffStatusChip({ status }: { status: DiffStatus }) {
  return <Badge tone={STATUS_TONE[status]}>{STATUS_LABEL[status]}</Badge>;
}

/** The `≠` divider; narrow layouts stack the sides under a `↓`. */
export function DiffDivider() {
  return (
    <span
      aria-hidden="true"
      className="flex items-center justify-center py-0.5 text-title leading-none text-ink-300 sm:py-0"
    >
      <span className="hidden sm:inline">≠</span>
      <span className="sm:hidden">↓</span>
    </span>
  );
}

export interface DiffValueProps {
  side: DiffSide;
  value: ReactNode;
  /** A secondary descriptor under the side label, e.g. "Quotation revision". */
  caption?: ReactNode;
  selected?: boolean;
  muted?: boolean;
  /** Renders the dashed `(empty)` placeholder instead of the value. */
  empty?: boolean;
  /**
   * Money and other atomic values never wrap. Free text is left alone so a
   * long address or payment term can still wrap inside its side.
   */
  nowrap?: boolean;
  /** Actions rendered under the value. */
  children?: ReactNode;
  /**
   * When set, the whole side is the selection target (the tax-basis card).
   * The visible content stays inside the same surface; the accessible name is
   * supplied by `ariaLabel`.
   */
  onSelect?: () => void;
  ariaLabel?: string;
  ariaPressed?: boolean;
}

/**
 * One side of a diff. It is a `div` for a value comparison and a `button` for
 * a whole-card choice; both surface labels are the spec's labels, so a reader
 * never has to infer which side is which from colour alone.
 */
export function DiffValue({
  side,
  value,
  caption,
  selected = false,
  muted = false,
  empty = false,
  nowrap = false,
  children,
  onSelect,
  ariaLabel,
  ariaPressed,
}: DiffValueProps) {
  const classes = [
    "grid min-w-0 content-start gap-2 rounded-lg border p-3 text-left",
    SIDE_SURFACE[side],
    selected ? "ring-2 ring-accent-500/40" : "",
    muted ? "opacity-80" : "",
    onSelect
      ? "cursor-pointer transition-[border-color,background-color,box-shadow] duration-150 ease-out-quart hover:border-accent-300 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50"
      : "",
  ]
    .filter((part) => part !== "")
    .join(" ");

  const content = (
    <>
      <span className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-micro font-medium uppercase tracking-[0.12em] text-ink-500">
          {SIDE_LABEL[side]}
        </span>
        {selected ? (
          <span className="inline-flex items-center gap-1 text-caption font-medium text-accent-700">
            <span aria-hidden="true">✓</span>
            Will be used
          </span>
        ) : null}
      </span>
      {caption !== undefined ? (
        <span className="text-caption text-ink-500">{caption}</span>
      ) : null}
      {empty ? (
        <span className="flex">
          <EmptyValue />
        </span>
      ) : (
        <span
          data-value="true"
          className={[
            "tnum text-small",
            // #582/#579: the changed value carries the emphasis, not only the
            // chip; money stays on one line while free text may wrap.
            nowrap ? "whitespace-nowrap" : "",
            muted
              ? "font-medium text-ink-500"
              : side === "quotation"
                ? "font-semibold text-accent-800"
                : "font-semibold text-ink-950",
          ]
            .filter((part) => part !== "")
            .join(" ")}
        >
          {value}
        </span>
      )}
      {children ? (
        <span className="mt-1 flex flex-wrap items-center gap-2">{children}</span>
      ) : null}
    </>
  );

  const data = {
    "data-side": side,
    "data-selected": selected ? "true" : "false",
  };

  if (onSelect !== undefined) {
    return (
      <button
        type="button"
        {...data}
        aria-label={ariaLabel}
        aria-pressed={ariaPressed ?? selected}
        onClick={onSelect}
        className={classes}
      >
        {content}
      </button>
    );
  }
  return (
    <div {...data} className={classes}>
      {content}
    </div>
  );
}

export interface DiffChoiceProps {
  label: string;
  poValue: string;
  quotationValue: string;
  /** The chosen side, if any: none is preselected when the review opens. */
  choice?: DiffSide;
  /** Both values are formatted money and must never wrap. */
  money?: boolean;
  poAction?: ReactNode;
  quotationAction?: ReactNode;
  /** Extra content under the two sides, such as provenance copy. */
  children?: ReactNode;
}

/**
 * One differing value as two labelled sides. The buttons are passed in so the
 * caller keeps the exact accessible names and callbacks the review already
 * used.
 */
export function DiffChoice({
  label,
  poValue,
  quotationValue,
  choice,
  money = false,
  poAction,
  quotationAction,
  children,
}: DiffChoiceProps) {
  return (
    <div
      role="group"
      aria-label={label}
      className="grid gap-2 rounded-lg border border-rule bg-white p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-small font-medium text-ink-900">{label}</p>
        <DiffStatusChip status="changed" />
      </div>
      <div className="grid min-w-0 gap-2 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] sm:items-stretch">
        <DiffValue
          side="po"
          value={poValue}
          empty={poValue === ""}
          nowrap={money}
          selected={choice === "po"}
          muted={choice === "quotation"}
        >
          {poAction}
        </DiffValue>
        <DiffDivider />
        <DiffValue
          side="quotation"
          value={quotationValue}
          empty={quotationValue === ""}
          nowrap={money}
          selected={choice === "quotation"}
          muted={choice === "po"}
        >
          {quotationAction}
        </DiffValue>
      </div>
      {children}
    </div>
  );
}

export interface DiffSummaryProps {
  differences: number;
  missing: number;
  extra: number;
}

function summaryCount(value: number, singular: string, plural: string): string {
  return `${value} ${value === 1 ? singular : plural}`;
}

/**
 * The review's size before scrolling: how many values differ, how many source
 * lines are missing and how many PO lines are extra, plus the two-side legend
 * the rows use. Rendered above the rows.
 */
export function DiffSummary({ differences, missing, extra }: DiffSummaryProps) {
  return (
    <section
      aria-label="Review summary"
      data-testid="po-diff-summary"
      data-differences={differences}
      data-missing={missing}
      data-extra={extra}
      className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-rule bg-white px-4 py-3 shadow-card"
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <span className="text-caption text-ink-500">
          <span className="tnum font-semibold text-ink-900">{differences}</span>{" "}
          {differences === 1 ? "difference" : "differences"}
        </span>
        <span className="text-caption text-ink-500">
          <span className="tnum font-semibold text-ink-900">{missing}</span>{" "}
          {missing === 1 ? "missing line" : "missing lines"}
        </span>
        <span className="text-caption text-ink-500">
          <span className="tnum font-semibold text-ink-900">{extra}</span>{" "}
          {extra === 1 ? "extra line" : "extra lines"}
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-3 text-caption text-ink-500">
        <span className="inline-flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className="h-2.5 w-2.5 rounded-full border border-rule bg-paper-subtle"
          />
          PO entry
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className="h-2.5 w-2.5 rounded-full border border-accent-300 bg-accent-100"
          />
          Quotation
        </span>
      </div>
      <span className="sr-only">
        Differences: {summaryCount(differences, "value", "values")}. Missing:{" "}
        {summaryCount(missing, "line", "lines")}. Extra:{" "}
        {summaryCount(extra, "line", "lines")}.
      </span>
    </section>
  );
}

export interface LineMatchRowProps {
  status: DiffStatus;
  title: ReactNode;
  /** The existing "Currently ..." sentence, kept verbatim. */
  detail?: ReactNode;
  /** The PO side's identity or value. */
  po: ReactNode;
  /** The quotation side's identity or value. */
  quotation: ReactNode;
  /** The row's action: the match picker or the acknowledgement control. */
  actions?: ReactNode;
  /** A recorded-state chip such as ACKNOWLEDGED. */
  note?: ReactNode;
}

/**
 * One ledger row: status, line identity, the two sides and the row action.
 * Split groups render their parent heading above these rows; the row itself
 * never knows whether its line is a parent or a sub-item.
 */
export function LineMatchRow({
  status,
  title,
  detail,
  po,
  quotation,
  actions,
  note,
}: LineMatchRowProps) {
  return (
    <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-2 rounded-lg border border-rule bg-white p-3">
      <div className="flex flex-wrap items-center gap-2">
        <DiffStatusChip status={status} />
        <p className="text-small font-medium text-ink-900">{title}</p>
        {note}
      </div>
      {detail !== undefined ? (
        <p className="text-caption text-ink-500">{detail}</p>
      ) : null}
      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-2 sm:grid-cols-2">
        <div className="rounded-md border border-rule bg-paper-subtle px-2.5 py-2">
          <span className="text-micro font-medium uppercase tracking-[0.12em] text-ink-500">
            {SIDE_LABEL.po}
          </span>
          <div className="mt-1 text-small text-ink-800">{po}</div>
        </div>
        <div className="rounded-md border border-accent-200 bg-accent-50/60 px-2.5 py-2">
          <span className="text-micro font-medium uppercase tracking-[0.12em] text-ink-500">
            {SIDE_LABEL.quotation}
          </span>
          <div className="mt-1 text-small text-ink-800">{quotation}</div>
        </div>
      </div>
      {actions !== undefined ? (
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
          {actions}
        </div>
      ) : null}
    </div>
  );
}

/** The dashed `(empty)` placeholder, shared by diff values and ledger rows. */
export function EmptyValue() {
  return (
    <span
      data-empty="true"
      className="inline-flex min-h-6 items-center rounded-md border border-dashed border-ink-300 px-2 text-caption italic text-ink-400"
    >
      (empty)
    </span>
  );
}

/** "ACKNOWLEDGED" once a missing/extra row has been acknowledged. */
export function AcknowledgedChip() {
  return <Badge tone="success">ACKNOWLEDGED</Badge>;
}
