import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode, type Ref } from "react";
import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import { formatQuantityScaled } from "../purchaseOrder";
import type {
  LatestNotice,
  PaneFilter,
  ReconciliationItem,
  ReconciliationSession,
} from "../poReconciliation";
import { hasAckRecording } from "../poReconciliation.lines";
import { orderPaneItems } from "../poReconciliation";

export interface ReconciliationPaneHandle {
  /** Focuses the first action of the first unresolved item, if any. */
  focusFirstUnresolved: () => boolean;
  /** Focuses the sticky count/status element. */
  focusCount: () => void;
  /** Focuses an item's first action by key, if present. */
  focusItem: (key: string) => boolean;
}

export interface ReconciliationPaneContentProps {
  /** Disambiguates ids when the rail and the sheet both mount. */
  idPrefix: string;
  session: ReconciliationSession | null;
  status: "idle" | "loading" | "ready" | "error";
  error: string | null;
  /** e.g. "QT-2026-018 · revision 02". Null while the source is unknown. */
  sourceLabel: string | null;
  sourceRevision: number | null;
  noSource: boolean;
  formerSource: boolean;
  /** Correction mode and saved views display the stored session read-only. */
  readOnly: boolean;
  readOnlyCaption?: string;
  filter: PaneFilter;
  onFilterChange: (filter: PaneFilter) => void;
  onShowAll: () => void;
  latestNotice: LatestNotice | null;
  keepConfirmKey: string | null;
  onRequestKeep: (key: string) => void;
  onCancelKeep: () => void;
  onConfirmKeep: (key: string) => void;
  onUse: (key: string) => void;
  onLinkQuotation?: () => void;
  onRetry: () => void;
  /** Line-set actions (ticket 05); absent in read-only harnesses. */
  onAddLineFromSource?: (reference: string) => void;
  onAcknowledgeMissing?: (key: string) => void;
  onAcknowledgeExtra?: (key: string) => void;
  onFixMatch?: (lineKey: string) => void;
  /** Formats money and quantity values; plain text needs none. */
  currency?: CurrencyInfo;
  /** Renders item actor ids; null when the name is not known client-side. */
  actorNameFor: (actorId: number) => string | null;
  /** Rendered beside the pane title; the sheet passes its Close button. */
  headerActions?: ReactNode;
  /** Derived totals check, rendered under the heading. Never a decision. */
  totalsStrip?: ReactNode;
  ref?: Ref<ReconciliationPaneHandle>;
}

const RESOLUTION_CHIPS: Record<string, string> = {
  kept_po: "Kept PO",
  used_quotation: "Used Quotation",
  values_now_match: "Values now match",
  acknowledged_missing: "Acknowledged missing",
  acknowledged_extra: "Acknowledged extra",
  match_corrected: "Match corrected",
  line_removed: "Line removed",
};

function formatPaneValue(
  value: unknown,
  field: string,
  currency?: CurrencyInfo
): string {
  if (typeof value === "number") {
    if (field === "header:discountPercent") return `${value}%`;
    if (field === "line:quantity") return formatQuantityScaled(value);
    if (field === "line:unitPrice" && currency !== undefined) {
      return formatMoney(currency, value);
    }
    return String(value);
  }
  if (typeof value === "string") {
    return value === "" ? "(empty)" : value;
  }
  if (value !== null && typeof value === "object" && "TaxZone" in value) {
    // Ticket 05: the link-time tax basis triple (contract section 6).
    const triple = value as { TaxZone?: unknown; TaxType?: unknown; ServiceCategory?: unknown };
    const parts = [triple.TaxZone, triple.TaxType, triple.ServiceCategory].filter(
      (part): part is string => typeof part === "string" && part !== ""
    );
    return parts.length > 0 ? parts.join(" · ") : "(no basis recorded)";
  }
  if (value !== null && typeof value === "object" && "Enabled" in value) {
    const discount = value as { Enabled: unknown; Percent: unknown };
    if (discount.Enabled === false) return "No discount";
    return `${String(discount.Percent)}%`;
  }
  if (value === null || value === undefined) return "(empty)";
  return String(value);
}

function formatPaneTime(iso: string): string | null {
  if (iso === "") return null;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return null;
  return date.toLocaleString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const FILTERS: { id: PaneFilter; label: string }[] = [
  { id: "all", label: "All" },
  { id: "unresolved", label: "Unresolved" },
  { id: "resolved", label: "Resolved" },
];

/**
 * Ticket 04: the reconciliation pane content, shared by the rail (aside, at
 * 1024 and above) and the sheet (below 1024). One component, two shells: the
 * editor renders it twice with different id prefixes, and only the layout
 * CSS decides which one shows. Never a modal: no aria-modal, no scrim.
 */
export function ReconciliationPaneContent(props: ReconciliationPaneContentProps) {
  const {
    idPrefix,
    session,
    status,
    error,
    sourceLabel,
    sourceRevision,
    noSource,
    formerSource,
    readOnly,
    readOnlyCaption,
    filter,
    onFilterChange,
    onShowAll,
    latestNotice,
    keepConfirmKey,
    onRequestKeep,
    onCancelKeep,
    onConfirmKeep,
    onUse,
    onLinkQuotation,
    onRetry,
    onAddLineFromSource,
    onAcknowledgeMissing,
    onAcknowledgeExtra,
    onFixMatch,
    currency,
    actorNameFor,
    headerActions,
    totalsStrip,
    ref,
  } = props;

  const items = session?.Items ?? [];
  const unresolved = items.filter((entry) => entry.Status === "unresolved");
  const unresolvedCount = unresolved.length;
  const ordered = orderPaneItems(items, filter);
  const counts = {
    all: items.length,
    unresolved: items.filter((entry) => entry.Status === "unresolved").length,
    resolved: items.filter((entry) => entry.Status === "resolved").length,
  };

  const listRef = useRef<HTMLDivElement | null>(null);
  const countRef = useRef<HTMLSpanElement | null>(null);
  const actionRefs = useRef(new Map<string, HTMLButtonElement>());
  const confirmYesRefs = useRef(new Map<string, HTMLButtonElement>());
  const handleRef = useRef<ReconciliationPaneHandle | null>(null);
  // Resolved rows are 44px summaries; their read-only values expand in place.
  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);
  const toggleExpanded = (key: string) =>
    setExpandedKeys((keys) =>
      keys.includes(key) ? keys.filter((entry) => entry !== key) : [...keys, key]
    );

  // Focus the confirmed keep step's Yes action when it opens.
  useEffect(() => {
    if (keepConfirmKey === null) return;
    confirmYesRefs.current.get(keepConfirmKey)?.focus();
  }, [keepConfirmKey]);

  useEffect(() => {
    handleRef.current = {
      focusFirstUnresolved: () => {
        const first = ordered.find((entry) => entry.Status === "unresolved");
        if (first === undefined) return false;
        const target = actionRefs.current.get(first.Key);
        if (target === undefined) return false;
        target.focus();
        return true;
      },
      focusCount: () => {
        countRef.current?.focus();
      },
      focusItem: (key: string) => {
        const target = actionRefs.current.get(key);
        if (target === undefined) return false;
        target.focus();
        return true;
      },
    };
  });

  useEffect(() => {
    if (typeof ref === "function") {
      ref(handleRef.current);
    } else if (ref !== null && ref !== undefined) {
      (ref as { current: ReconciliationPaneHandle | null }).current =
        handleRef.current;
    }
  });

  const onFilterKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const current = FILTERS.findIndex((entry) => entry.id === filter);
    let next = current;
    if (event.key === "ArrowRight") next = (current + 1) % FILTERS.length;
    else if (event.key === "ArrowLeft")
      next = (current + FILTERS.length - 1) % FILTERS.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = FILTERS.length - 1;
    else return;
    event.preventDefault();
    const target = FILTERS[next];
    if (target === undefined) return;
    onFilterChange(target.id);
    document.getElementById(`${idPrefix}-filter-${target.id}`)?.focus();
  };

  const listId = `${idPrefix}-pane-items`;
  const headingId = `${idPrefix}-pane-heading`;

  const cardShell = (key: string, title: string, pill: ReactNode, body: ReactNode) => (
    <li key={key}>
      <div className="rounded-lg border border-rule bg-white p-3">
        <div className="flex items-center justify-between gap-2">
          <span className="text-small font-medium text-ink-900">{title}</span>
          {pill}
        </div>
        {body}
      </div>
    </li>
  );

  const neutralPill = (text: string) => (
    <span className="inline-flex items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
      {text}
    </span>
  );

  const dangerPill = (text: string) => (
    <span className="inline-flex items-center rounded-md border border-danger-200 bg-danger-50 px-2 py-0.5 text-micro font-medium uppercase text-danger-700">
      {text}
    </span>
  );

  const cardButtonClass =
    "inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-lg border border-rule-strong bg-white px-3.5 text-small font-medium text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 lg:min-h-8";
  const cardAccentButtonClass =
    "inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-lg border border-accent-200 bg-accent-50 px-3.5 text-small font-medium text-accent-700 transition-colors duration-150 hover:bg-accent-100 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 lg:min-h-8";

  const renderMissingCard = (item: ReconciliationItem) => {
    const acknowledged = item.Status === "resolved";
    return cardShell(
      item.Key,
      item.Label,
      acknowledged ? neutralPill("Acknowledged missing") : neutralPill("Missing line"),
      <>
        <p className="mt-1 text-small text-ink-700">
          {typeof item.SourceValue === "string" && item.SourceValue !== ""
            ? item.SourceValue
            : "This quotation line is not on the purchase order."}
        </p>
        {acknowledged || readOnly ? null : (
          <div className="mt-2 flex flex-wrap gap-2">
            <button
              type="button"
              ref={(node) => {
                if (node === null) actionRefs.current.delete(item.Key);
                else actionRefs.current.set(item.Key, node);
              }}
              aria-label={`Add line from Quotation ${item.SourceLineRef ?? ""}`}
              onClick={() => onAddLineFromSource?.(String(item.SourceLineRef ?? ""))}
              className={cardAccentButtonClass}
            >
              Add line from Quotation
            </button>
            <button
              type="button"
              aria-label={`Acknowledge omission of ${item.Label}`}
              onClick={() => onAcknowledgeMissing?.(item.Key)}
              className={cardButtonClass}
            >
              Acknowledge omission
            </button>
          </div>
        )}
      </>
    );
  };

  const renderExtraCard = (item: ReconciliationItem) => {
    const acknowledged = item.Status === "resolved";
    return cardShell(
      item.Key,
      item.Label,
      acknowledged ? neutralPill("Acknowledged extra") : neutralPill("Extra line"),
      <>
        <p className="mt-1 text-small text-ink-700">
          {formatPaneValue(item.POValue, item.Field, currency)}
        </p>
        {acknowledged || readOnly ? null : (
          <div className="mt-2 flex flex-wrap gap-2">
            <button
              type="button"
              ref={(node) => {
                if (node === null) actionRefs.current.delete(item.Key);
                else actionRefs.current.set(item.Key, node);
              }}
              aria-label={`Match to Quotation for ${item.Label}`}
              onClick={() => item.LineKey != null && onFixMatch?.(item.LineKey)}
              className={cardAccentButtonClass}
            >
              Match to Quotation…
            </button>
            <button
              type="button"
              aria-label={`Acknowledge ${item.Label} as extra`}
              onClick={() => onAcknowledgeExtra?.(item.Key)}
              className={cardButtonClass}
            >
              Acknowledge extra
            </button>
          </div>
        )}
      </>
    );
  };

  const renderUnknownCard = (item: ReconciliationItem) => {
    const reference = typeof item.SourceLineRef === "string" ? item.SourceLineRef : "";
    const acked = hasAckRecording(item, "acknowledged_extra");
    return cardShell(
      item.Key,
      `Unknown reference · ${reference}`,
      dangerPill("Unknown reference"),
      <>
        <p className="mt-1 text-small text-ink-700">
          {formatPaneValue(item.POValue, item.Field, currency)}
        </p>
        <p className="mt-1 text-caption text-ink-500">
          No Quotation line {reference} exists.
          {acked
            ? " Acknowledged: link a valid Quotation line to finalize."
            : ""}
        </p>
        {readOnly ? null : (
          <div className="mt-2 flex flex-wrap gap-2">
            <button
              type="button"
              ref={(node) => {
                if (node === null) actionRefs.current.delete(item.Key);
                else actionRefs.current.set(item.Key, node);
              }}
              aria-label={`Match to Quotation for unknown reference ${reference}`}
              onClick={() => item.LineKey != null && onFixMatch?.(item.LineKey)}
              className={cardAccentButtonClass}
            >
              Match to Quotation…
            </button>
            {acked ? null : (
              <button
                type="button"
                aria-label={`Acknowledge unknown reference ${reference} as extra`}
                onClick={() => onAcknowledgeExtra?.(item.Key)}
                className={cardButtonClass}
              >
                Acknowledge extra
              </button>
            )}
          </div>
        )}
      </>
    );
  };

  const renderDuplicateCard = (item: ReconciliationItem) => {
    const reference =
      typeof item.SourceLineRef === "string" ? item.SourceLineRef : "";
    return cardShell(
      item.Key,
      `Duplicate match · Quotation ${reference}`,
      dangerPill("Blocking"),
      <>
        <p className="mt-1 text-caption text-ink-500">
          Two lines claim this quotation line. Correct one match before Final Save.
        </p>
        {readOnly ? null : (
          <div className="mt-2 flex flex-wrap gap-2">
            <button
              type="button"
              ref={(node) => {
                if (node === null) actionRefs.current.delete(item.Key);
                else actionRefs.current.set(item.Key, node);
              }}
              aria-label={`Fix match for Quotation ${reference}`}
              onClick={() => item.LineKey != null && onFixMatch?.(item.LineKey)}
              className={cardAccentButtonClass}
            >
              Fix match
            </button>
          </div>
        )}
      </>
    );
  };

  const renderItem = (item: ReconciliationItem) => {
    const actionable =
      !readOnly && item.Status === "unresolved" && item.Kind === "difference";
    const chip =
      item.Resolution !== null && item.Resolution !== undefined
        ? (RESOLUTION_CHIPS[item.Resolution] ?? item.Resolution)
        : null;
    const expanded = expandedKeys.includes(item.Key);
    const attribution =
      item.Resolution !== null && item.Resolution !== undefined
        ? `${actorNameFor(item.ActorID) !== null ? `${chip} by ${actorNameFor(item.ActorID)} · ` : ""}${formatPaneTime(item.UpdatedAt) ?? ""}`
        : null;
    if (item.Kind === "missing_line") return renderMissingCard(item);
    if (item.Kind === "extra_line" && item.SourceLineRef !== null && item.SourceLineRef !== undefined && item.SourceLineRef !== "") {
      return renderUnknownCard(item);
    }
    if (item.Kind === "extra_line") return renderExtraCard(item);
    if (item.Kind === "duplicate_match") return renderDuplicateCard(item);
    if (
      !actionable &&
      item.Status === "resolved" &&
      item.Kind === "difference"
    ) {
      return (
        <li key={item.Key}>
          <div className="rounded-lg border border-rule bg-white">
            <button
              type="button"
              aria-expanded={expanded}
              aria-label={`${item.Label}, ${chip ?? "resolved"}. ${expanded ? "Collapse" : "Show values"}`}
              onClick={() => toggleExpanded(item.Key)}
              className="flex min-h-11 w-full cursor-pointer items-center justify-between gap-2 px-3 py-2 text-left transition-colors duration-150 hover:bg-paper-subtle/60 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-500/40"
            >
              <span className="truncate text-small font-medium text-ink-900">
                {item.Label}
              </span>
              <span className="flex shrink-0 items-center gap-1.5">
                {chip !== null ? (
                  <span className="inline-flex items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
                    {chip}
                  </span>
                ) : null}
                <svg
                  width="12"
                  height="12"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                  className={`shrink-0 text-ink-400 transition-transform duration-150 ${expanded ? "rotate-180" : ""}`}
                >
                  <path d="m6 9 6 6 6-6" />
                </svg>
              </span>
            </button>
            {attribution !== null && attribution !== "" ? (
              <p className="px-3 pb-1 text-caption text-ink-500">{attribution}</p>
            ) : null}
            {expanded ? (
              <dl className="grid gap-1 px-3 pb-3 text-small">
                <div className="flex gap-2">
                  <dt className="w-[72px] shrink-0 text-caption text-ink-500">PO</dt>
                  <dd className="min-w-0 flex-1 text-ink-900">
                    {formatPaneValue(item.POValue, item.Field, currency)}
                  </dd>
                </div>
                <div className="flex gap-2">
                  <dt className="w-[72px] shrink-0 text-caption text-ink-500">
                    Quotation
                  </dt>
                  <dd className="min-w-0 flex-1 text-ink-700">
                    {formatPaneValue(item.SourceValue, item.Field, currency)}
                  </dd>
                </div>
              </dl>
            ) : null}
          </div>
        </li>
      );
    }
    if (!actionable) {
      const taxRow = item.Kind === "tax_basis";
      return (
        <li key={item.Key}>
          <div className="rounded-lg border border-rule bg-white p-3">
            <div className="flex items-center justify-between gap-2">
              <span className="text-small font-medium text-ink-900">
                {taxRow
                  ? `Tax basis · linked at revision ${sourceRevision ?? item.SourceValue ?? ""}`
                  : item.Label}
              </span>
              {chip !== null ? (
                <span className="inline-flex items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
                  {chip}
                </span>
              ) : item.Status === "unresolved" ? (
                <span className="inline-flex items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
                  Unresolved
                </span>
              ) : null}
            </div>
            {taxRow ? (
              <p className="mt-1 text-small tabular-nums text-ink-700">
                {formatPaneValue(item.POValue, item.Field, currency)}{" "}
                <svg
                  width="12"
                  height="12"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.7"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                  className="inline-block shrink-0 text-ink-400"
                >
                  <rect x="5" y="11" width="14" height="9" rx="1.5" />
                  <path d="M8 11V8a4 4 0 0 1 8 0v3" />
                </svg>
                <span className="sr-only">Protected: changing the basis later is a correction or source repair.</span>
              </p>
            ) : (
              <dl className="mt-1.5 grid gap-1 text-small">
                <div className="flex gap-2">
                  <dt className="w-[72px] shrink-0 text-caption text-ink-500">PO</dt>
                  <dd className="min-w-0 flex-1 text-ink-900">
                    {formatPaneValue(item.POValue, item.Field, currency)}
                  </dd>
                </div>
                <div className="flex gap-2">
                  <dt className="w-[72px] shrink-0 text-caption text-ink-500">
                    Quotation
                  </dt>
                  <dd className="min-w-0 flex-1 text-ink-700">
                    {formatPaneValue(item.SourceValue, item.Field, currency)}
                  </dd>
                </div>
              </dl>
            )}
            {item.Resolution !== null ? (
              <p className="mt-1 text-caption text-ink-500">
                {actorNameFor(item.ActorID) !== null
                  ? `${chip} by ${actorNameFor(item.ActorID)} · `
                  : ""}
                {formatPaneTime(item.UpdatedAt) ?? ""}
              </p>
            ) : null}
          </div>
        </li>
      );
    }

    const confirming = keepConfirmKey === item.Key;
    return (
      <li key={item.Key}>
        <div className="rounded-lg border border-rule bg-white p-3">
          <div className="flex items-center justify-between gap-2">
            <span className="text-small font-medium text-ink-900">{item.Label}</span>
            <span className="inline-flex items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
              Unresolved
            </span>
          </div>
          <dl className="mt-1.5 grid gap-1 text-small">
            <div className="flex gap-2">
              <dt className="w-[72px] shrink-0 text-caption text-ink-500">PO</dt>
              <dd className="min-w-0 flex-1 text-ink-900">
                {formatPaneValue(item.POValue, item.Field, currency)}
              </dd>
            </div>
            <div className="flex gap-2">
              <dt className="w-[72px] shrink-0 text-caption text-ink-500">Quotation</dt>
              <dd className="min-w-0 flex-1 text-ink-700">
                {formatPaneValue(item.SourceValue, item.Field, currency)}
              </dd>
            </div>
          </dl>
          {confirming ? (
            <div className="mt-2 rounded-md border border-rule bg-paper-subtle p-2.5">
              <p className="text-small font-medium text-ink-900">Is this correct?</p>
              <p className="mt-0.5 text-caption leading-relaxed text-ink-500">
                Keeping the PO value records a deliberate difference.
              </p>
              <div className="mt-2 flex flex-wrap gap-2">
                <button
                  ref={(node) => {
                    if (node === null) confirmYesRefs.current.delete(item.Key);
                    else confirmYesRefs.current.set(item.Key, node);
                  }}
                  type="button"
                  aria-label={`Yes, keep PO entry for ${item.Label}`}
                  onClick={() => onConfirmKeep(item.Key)}
                  className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-lg border border-accent-200 bg-accent-50 px-3.5 text-small font-medium text-accent-700 transition-colors duration-150 hover:bg-accent-100 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 lg:min-h-8"
                >
                  Yes, keep PO entry
                </button>
                <button
                  type="button"
                  aria-label={`Cancel keeping PO entry for ${item.Label}`}
                  onClick={onCancelKeep}
                  className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-lg border border-rule-strong bg-white px-3.5 text-small font-medium text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 lg:min-h-8"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <div className="mt-2 flex flex-wrap gap-2">
              <button
                ref={(node) => {
                  if (node === null) actionRefs.current.delete(item.Key);
                  else actionRefs.current.set(item.Key, node);
                }}
                type="button"
                aria-label={`Keep PO entry for ${item.Label}`}
                onClick={() => onRequestKeep(item.Key)}
                className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-lg border border-rule-strong bg-white px-3.5 text-small font-medium text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 lg:min-h-8"
              >
                Keep PO
              </button>
              <button
                type="button"
                aria-label={`Use Quotation value for ${item.Label}`}
                onClick={() => onUse(item.Key)}
                className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-lg border border-accent-200 bg-accent-50 px-3.5 text-small font-medium text-accent-700 transition-colors duration-150 hover:bg-accent-100 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 lg:min-h-8"
              >
                Use Quotation
              </button>
            </div>
          )}
        </div>
      </li>
    );
  };

  return (
    <div className="flex min-h-0 flex-col">
      <div className="flex items-center justify-between gap-2">
        <h2
          id={headingId}
          tabIndex={-1}
          className="text-[15px] font-semibold leading-6 text-ink-950 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
        >
          Reconciliation
        </h2>
        {headerActions}
        <span
          ref={countRef}
          tabIndex={-1}
          role="status"
          aria-label={
            unresolvedCount === 0
              ? "Nothing to resolve"
              : `${unresolvedCount} ${unresolvedCount === 1 ? "item" : "items"} to resolve`
          }
          className="tnum inline-flex h-7 shrink-0 items-center rounded-full bg-accent-100 px-2.5 text-caption font-semibold text-accent-800 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
        >
          {unresolvedCount === 0
            ? "Nothing to resolve"
            : `${unresolvedCount} to resolve`}
        </span>
      </div>
      {sourceLabel !== null ? (
        <p className="mt-0.5 text-caption text-ink-500">
          Compared with {sourceLabel}
        </p>
      ) : null}
      <p className="text-caption text-ink-400">Checked when you leave a field.</p>
      {totalsStrip !== undefined && totalsStrip !== null ? (
        <div className="mt-2 rounded-lg border border-rule bg-white p-3">{totalsStrip}</div>
      ) : null}

      {readOnly && readOnlyCaption !== undefined ? (
        <p className="mt-2 text-caption leading-relaxed text-ink-500">{readOnlyCaption}</p>
      ) : null}

      {status === "loading" ? (
        <div className="mt-3 space-y-3" aria-busy="true">
          <span className="sr-only">Loading the reconciliation…</span>
          {[0, 1, 2].map((row) => (
            <div key={row} className="h-11 animate-pulse rounded-lg bg-paper-muted" />
          ))}
        </div>
      ) : status === "error" ? (
        <div className="mt-3 rounded-lg border border-danger-200 bg-danger-50 p-4" role="alert">
          <p className="text-small font-medium text-danger-700">
            The reconciliation could not be loaded.
          </p>
          <p className="mt-1 text-caption text-danger-700/80">
            Save draft still works. Final Save is blocked until it loads.
          </p>
          <button
            type="button"
            onClick={onRetry}
            className="mt-2 inline-flex min-h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border border-danger-200 bg-white px-3.5 text-small font-medium text-danger-600 transition-colors duration-150 hover:bg-danger-50 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-danger-500/40"
          >
            Try again
          </button>
        </div>
      ) : noSource ? (
        <div className="mt-3">
          <p className="text-small text-ink-700">Link a Quotation to start the comparison.</p>
          <p className="mt-1 text-caption text-ink-400">
            Currency and tax basis stay editable until then.
          </p>
          {onLinkQuotation !== undefined ? (
            <button
              type="button"
              onClick={onLinkQuotation}
              className="mt-2 inline-flex min-h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border border-rule-strong bg-white px-3.5 text-small font-medium text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
            >
              Link Quotation
            </button>
          ) : null}
        </div>
      ) : formerSource ? (
        <div className="mt-3">
          <p className="text-small text-ink-700">The source quotation was deleted.</p>
          <p className="mt-1 text-caption text-ink-400">
            This reconciliation is read-only until a new source is linked.
          </p>
        </div>
      ) : items.length === 0 ? (
        <div className="mt-3">
          <p className="text-small font-medium text-ink-900">No differences yet</p>
          <p className="mt-1 text-caption text-ink-500">
            Values are checked when you leave a field.
          </p>
        </div>
      ) : (
        <>
          <div
            role="tablist"
            aria-label="Reconciliation filters"
            onKeyDown={onFilterKeyDown}
            className="mt-3 grid grid-cols-3 gap-1 rounded-lg bg-paper-muted p-1"
          >
            {FILTERS.map((entry) => {
              const count =
                entry.id === "all"
                  ? counts.all
                  : entry.id === "unresolved"
                    ? counts.unresolved
                    : counts.resolved;
              const active = filter === entry.id;
              return (
                <button
                  key={entry.id}
                  id={`${idPrefix}-filter-${entry.id}`}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  aria-controls={listId}
                  tabIndex={active ? 0 : -1}
                  onClick={() => onFilterChange(entry.id)}
                  className={`tnum inline-flex min-h-8 cursor-pointer items-center justify-center gap-1.5 rounded-md px-2 text-small transition-colors duration-150 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
                    active
                      ? "bg-white font-medium text-ink-900 shadow-sm ring-1 ring-rule"
                      : "text-ink-500 hover:text-ink-900"
                  }`}
                >
                  {entry.label}
                  <span className="tabular-nums text-caption">{count}</span>
                </button>
              );
            })}
          </div>

          {latestNotice !== null ? (
            <p className="mt-2 truncate text-caption text-ink-500" aria-hidden="true">
              {latestNotice.kind === "empty"
                ? `${latestNotice.label} is empty`
                : `${latestNotice.label}: matches the Quotation`}
            </p>
          ) : null}

          {ordered.length === 0 ? (
            <div className="mt-3">
              <p className="text-small font-medium text-ink-900">
                {filter === "unresolved" ? "No unresolved items" : "No resolved items yet"}
              </p>
              <p className="mt-1 text-caption text-ink-500">
                {filter === "unresolved"
                  ? "No differences to resolve."
                  : "Kept and accepted values will appear here."}
              </p>
              <button
                type="button"
                onClick={onShowAll}
                className="mt-2 inline-flex min-h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border border-rule-strong bg-white px-3.5 text-small font-medium text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
              >
                Show all
              </button>
            </div>
          ) : (
            <div ref={listRef} role="tabpanel" id={listId} className="mt-2 min-h-0 flex-1 overflow-y-auto overscroll-contain">
              <ul className="flex flex-col gap-2 pb-1">{ordered.map(renderItem)}</ul>
            </div>
          )}
        </>
      )}
    </div>
  );
}

/**
 * The sticky count bar that opens the sheet below 1024. Never a modal: the
 * shell header and action bar stay interactive, Escape closes with focus back
 * on this trigger.
 */
export function ReconciliationSheetTrigger({
  unresolvedCount,
  sourceLabel,
  expanded,
  onOpen,
  ref,
}: {
  unresolvedCount: number;
  sourceLabel: string | null;
  expanded: boolean;
  onOpen: () => void;
  ref?: Ref<HTMLButtonElement>;
}) {
  return (
    <button
      ref={ref}
      type="button"
      aria-expanded={expanded}
      aria-label={
        unresolvedCount === 0
          ? "Reconciliation, nothing to resolve. Open panel."
          : `Reconciliation, ${unresolvedCount} ${unresolvedCount === 1 ? "item" : "items"} to resolve. Open panel.`
      }
      onClick={onOpen}
      className="sticky top-16 z-20 flex min-h-14 w-full cursor-pointer items-center justify-between gap-2 border-b border-rule bg-paper/95 px-5 py-2 backdrop-blur transition-colors duration-150 hover:bg-paper-muted focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-500/40 lg:hidden"
    >
      <span className="flex min-w-0 items-baseline gap-2">
        <span className="text-small font-medium text-ink-700">Reconciliation</span>
        {sourceLabel !== null ? (
          <span className="hidden truncate text-caption text-ink-400 min-[420px]:inline">
            {sourceLabel}
          </span>
        ) : null}
      </span>
      <span className="flex shrink-0 items-center gap-2">
        <span className="tnum inline-flex h-7 items-center rounded-full bg-accent-100 px-2.5 text-caption font-semibold text-accent-800">
          {unresolvedCount === 0
            ? "Nothing to resolve"
            : `${unresolvedCount} to resolve`}
        </span>
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className="text-ink-400"
        >
          <path d="m6 9 6 6 6-6" />
        </svg>
      </span>
    </button>
  );
}
