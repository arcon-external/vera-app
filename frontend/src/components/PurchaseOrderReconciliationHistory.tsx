import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode, type Ref } from "react";
import { ListPOReconciliationSessions } from "@backend/backend/Quotation";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderHistoryEntry } from "../poCorrection";
import type { PaneFilter } from "../poReconciliation";
import {
  normalizeReconciliationHistory,
  savedHistoryRow,
  savedReconciliationContext,
  savedReconciliationCounts,
  savedReconciliationView,
  savedSourceDeletedLabel,
  savedSourceLabel,
  savedTriggerState,
  type ReconciliationHistoryInput,
  type ReconciliationHistoryRead,
  type ReconciliationHistoryRecord,
} from "../poReconciliationHistory";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import {
  ReconciliationPaneContent,
  type ReconciliationPaneHandle,
} from "./ReconciliationPane";

// #518: the saved record's reconciliation surface.
//
// One read (`ListPOReconciliationSessions`) feeds three render sites: the
// >=1024 rail, the <1024 trigger, and the sheet the trigger opens. The hook
// owns that read, the current/preview selection and the saved-only filter so
// the three sites cannot disagree. Everything is read-only: the pane receives
// `readOnly` and `savedHistory`, and every action callback is a no-op.

/** The transport seam, mirroring poReconciliationAdapter's pattern. */
export interface ReconciliationHistoryTransport {
  list(poID: string): Promise<ReconciliationHistoryInput>;
}

export const wailsReconciliationHistoryTransport: ReconciliationHistoryTransport = {
  list: (poID: string) => ListPOReconciliationSessions(poID),
};

function errorMessage(error: unknown): string {
  if (typeof error === "string" && error.trim() !== "") return error;
  if (error instanceof Error && error.message.trim() !== "") return error.message;
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message.trim() !== "") return message;
  }
  return "The saved reconciliation history could not be loaded.";
}

/** The state word the trigger and the panel both read. */
export type SavedReconciliationStatus =
  | "loading"
  | "error"
  | "none"
  | "stale"
  | "ready";

export interface SavedReconciliationState {
  status: SavedReconciliationStatus;
  error: string | null;
  view: ReturnType<typeof savedReconciliationView> | null;
  /** The generation on screen: current, or a selected earlier one. */
  selected: ReconciliationHistoryRecord | null;
  isCurrent: boolean;
  previous: ReconciliationHistoryRecord[];
  selectedGeneration: number | null;
  filter: PaneFilter;
  counts: ReturnType<typeof savedReconciliationCounts>;
  announcement: string;
  /** Increments whenever the pane heading should take focus. */
  focusToken: number;
  setFilter: (filter: PaneFilter) => void;
  select: (record: ReconciliationHistoryRecord) => void;
  backToCurrent: () => void;
  retry: () => void;
  /**
   * Re-reads the generations after a stale read. Unlike `retry`, a successful
   * reload moves focus to the pane heading (the content the person asked for
   * has arrived).
   */
  reload: () => void;
  /** Sets the polished live-region sentence (a repair arrival, for example). */
  announce: (message: string) => void;
}

export interface UseSavedReconciliationOptions {
  orderId: number;
  orderVersion: number;
  /**
   * The SSR snapshot / test fixture. Omitted in the app so the saved view
   * performs the one read; supplied it replaces the fetch and its state is
   * authoritative.
   */
  initial?: ReconciliationHistoryInput | null;
  initialError?: string | null;
  transport?: ReconciliationHistoryTransport;
}

export function useSavedReconciliation(
  options: UseSavedReconciliationOptions
): SavedReconciliationState {
  const {
    orderId,
    orderVersion,
    initial,
    initialError = null,
    transport = wailsReconciliationHistoryTransport,
  } = options;
  const supplied = initial !== undefined && initial !== null;
  const [read, setRead] = useState<ReconciliationHistoryRead | null>(() =>
    initial !== undefined && initial !== null
      ? normalizeReconciliationHistory(initial)
      : null
  );
  const [status, setStatus] = useState<"loading" | "ready" | "error">(() => {
    if (initialError !== null) return "error";
    return initial !== undefined && initial !== null ? "ready" : "loading";
  });
  const [error, setError] = useState<string | null>(initialError);
  const [token, setToken] = useState(0);
  const [filter, setFilterState] = useState<PaneFilter>("all");
  const [selectedGeneration, setSelectedGeneration] = useState<number | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const [focusToken, setFocusToken] = useState(0);
  const focusAfterLoad = useRef(false);

  useEffect(() => {
    // A supplied snapshot (SSR, tests) is the state; never shadow it with a
    // fetch. The app omits it and takes this path.
    if (supplied) return;
    let cancelled = false;
    setStatus("loading");
    setError(null);
    transport
      .list(String(orderId))
      .then((raw) => {
        if (cancelled) return;
        setRead(normalizeReconciliationHistory(raw));
        setStatus("ready");
        // A deliberate reload (the stale card) lands on the pane heading; an
        // initial load and a retry do not move focus.
        if (focusAfterLoad.current) {
          focusAfterLoad.current = false;
          setFocusToken((value) => value + 1);
        }
      })
      .catch((cause: unknown) => {
        if (cancelled) return;
        setRead(null);
        setError(errorMessage(cause));
        setStatus("error");
      });
    return () => {
      cancelled = true;
    };
  }, [orderId, orderVersion, token, transport, supplied]);

  const view = useMemo(
    () => (read === null ? null : savedReconciliationView(read, orderVersion)),
    [read, orderVersion]
  );

  const selected = useMemo(() => {
    if (view === null || view.kind !== "ready") return null;
    if (selectedGeneration === null) return view.current;
    if (view.current.Generation === selectedGeneration) return view.current;
    return (
      view.previous.find((record) => record.Generation === selectedGeneration) ??
      view.current
    );
  }, [view, selectedGeneration]);

  const isCurrent =
    view !== null &&
    view.kind === "ready" &&
    selected !== null &&
    selected === view.current;

  const counts = useMemo(
    () =>
      selected === null
        ? { all: 0, resolved: 0, unresolved: 0 }
        : savedReconciliationCounts(selected),
    [selected]
  );

  const statusOf: SavedReconciliationStatus =
    status === "loading"
      ? "loading"
      : status === "error"
        ? "error"
        : view === null
          ? "loading"
          : view.kind === "none"
            ? "none"
            : view.kind === "stale"
              ? "stale"
              : "ready";

  const setFilter = useCallback((next: PaneFilter) => {
    // A saved session offers All and Resolved; anything else stays All.
    setFilterState(next === "resolved" ? "resolved" : "all");
  }, []);

  const select = useCallback((record: ReconciliationHistoryRecord) => {
    setSelectedGeneration(record.Generation);
    setFilterState("all");
    setAnnouncement(
      `Showing saved session, revision ${record.Session.SourceRevision}, generation ${record.Generation}.`
    );
    setFocusToken((value) => value + 1);
  }, []);

  const backToCurrent = useCallback(() => {
    setSelectedGeneration(null);
    setFilterState("all");
    setAnnouncement("Showing the current saved session.");
    setFocusToken((value) => value + 1);
  }, []);

  const retry = useCallback(() => setToken((value) => value + 1), []);

  const reload = useCallback(() => {
    focusAfterLoad.current = true;
    setToken((value) => value + 1);
  }, []);

  const announce = useCallback((message: string) => setAnnouncement(message), []);

  return {
    status: statusOf,
    error,
    view,
    selected,
    isCurrent,
    previous: view !== null && view.kind === "ready" ? view.previous : [],
    selectedGeneration,
    filter,
    counts,
    announcement,
    focusToken,
    setFilter,
    select,
    backToCurrent,
    retry,
    reload,
    announce,
  };
}

const PANE_ACTION_NOOP = () => undefined;

export interface SavedReconciliationPanelProps {
  /** "saved-rail" or "saved-sheet": names the pane heading and the lists. */
  idPrefix: string;
  state: SavedReconciliationState;
  order: PurchaseOrderDTO;
  history: PurchaseOrderHistoryEntry[];
  currency: CurrencyInfo;
  actorNameFor: (actorId: number) => string | null;
  headerActions?: ReactNode;
  /** Reloads the record itself; the stale read names a newer PO version. */
  onReload?: () => void;
  fillHeight?: boolean;
  /**
   * Whether this shell owns focus after a session swap. The rail yields to
   * the sheet while the sheet is open; otherwise both would focus at once.
   */
  takesFocus?: boolean;
  ref?: Ref<ReconciliationPaneHandle>;
}

/** The saved-state sentences, in one place so both shells read identically. */
function SavedPanelState({ state, onReload }: {
  state: SavedReconciliationState;
  onReload?: () => void;
}) {
  if (state.status === "stale") {
    return (
      <div className="mt-3 rounded-lg border border-danger-200 bg-danger-50 p-4" role="alert">
        <p className="text-small font-medium text-danger-700">
          This purchase order changed while its reconciliation was loading.
        </p>
        <button
          type="button"
          // Reload refetches both halves: the record (the parent's read) and
          // the generations (this read). A successful reload focuses the pane
          // heading; a read failure keeps Try again and never touches the PO.
          onClick={() => {
            onReload?.();
            state.reload();
          }}
          className="mt-2 inline-flex min-h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border border-danger-200 bg-white px-3.5 text-small font-medium text-danger-600 transition-colors duration-150 hover:bg-danger-50 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-danger-500/40"
        >
          Reload
        </button>
      </div>
    );
  }
  if (state.status === "none") {
    return (
      <p className="mt-3 text-small text-ink-700">
        No reconciliation session was recorded for this purchase order.
      </p>
    );
  }
  return null;
}

export function SavedReconciliationPanel({
  idPrefix,
  state,
  order,
  history,
  currency,
  actorNameFor,
  headerActions,
  onReload,
  fillHeight = false,
  takesFocus = true,
  ref,
}: SavedReconciliationPanelProps) {
  const headingId = `${idPrefix}-pane-heading`;
  const listId = `${idPrefix}-history`;

  // Selection and "back to current" move focus to the pane heading once the
  // new generation is on screen; a row must never keep focus after its
  // content swaps beneath it. Only the visible shell takes focus.
  useEffect(() => {
    if (state.focusToken === 0 || !takesFocus) return;
    document.getElementById(headingId)?.focus();
  }, [state.focusToken, headingId, takesFocus]);

  const selected = state.selected;
  const readOnlyState =
    state.status === "ready" || state.status === "none" || state.status === "stale";

  return (
    <section
      aria-label="Saved reconciliation"
      className={`flex min-h-0 flex-col${fillHeight ? " h-full" : ""}`}
    >
      <div className="flex items-center justify-between gap-2">
        <h2
          id={headingId}
          tabIndex={-1}
          className="text-[15px] font-semibold leading-6 text-ink-950 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
        >
          Reconciliation
        </h2>
        <span className="inline-flex shrink-0 items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
          Read-only
        </span>
        {headerActions}
      </div>

      {readOnlyState ? <SavedPanelState state={state} onReload={onReload} /> : null}

      {state.status === "loading" ? (
        <div className="mt-3 space-y-3" aria-busy="true">
          <span className="sr-only">Loading the saved reconciliation…</span>
          {[0, 1, 2].map((row) => (
            <div key={row} className="h-11 animate-pulse rounded-lg bg-paper-muted" />
          ))}
        </div>
      ) : null}

      {state.status === "error" ? (
        <div className="mt-3 rounded-lg border border-danger-200 bg-danger-50 p-4" role="alert">
          <p className="text-small font-medium text-danger-700">
            The saved reconciliation history could not be loaded.
          </p>
          <button
            type="button"
            onClick={state.retry}
            className="mt-2 inline-flex min-h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border border-danger-200 bg-white px-3.5 text-small font-medium text-danger-600 transition-colors duration-150 hover:bg-danger-50 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-danger-500/40"
          >
            Try again
          </button>
        </div>
      ) : null}

      {state.status === "ready" && selected !== null ? (
        <>
          <p className="mt-0.5 text-caption text-ink-500">
            {savedReconciliationContext(selected, state.isCurrent)}
          </p>
          {selected.SourceAvailable ? null : (
            <p className="mt-2 rounded-lg border border-danger-200 bg-danger-50 px-3 py-2 text-caption text-danger-700">
              {savedSourceDeletedLabel(selected)}
            </p>
          )}
          {state.isCurrent ? null : (
            <button
              type="button"
              onClick={state.backToCurrent}
              className="mt-2 inline-flex min-h-9 cursor-pointer items-center justify-center gap-2 self-start rounded-lg border border-rule-strong bg-white px-3 text-small font-medium text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
            >
              Back to current session
            </button>
          )}
          <div className="mt-2 min-h-0 flex-1">
            <ReconciliationPaneContent
              idPrefix={idPrefix}
              ref={ref}
              session={selected.Session}
              status="ready"
              error={null}
              sourceLabel={savedSourceLabel(selected, order)}
              sourceRevision={selected.Session.SourceRevision}
              noSource={false}
              formerSource={false}
              readOnly
              savedHistory
              filter={state.filter}
              onFilterChange={state.setFilter}
              onShowAll={() => state.setFilter("all")}
              latestNotice={null}
              keepConfirmKey={null}
              onRequestKeep={PANE_ACTION_NOOP}
              onCancelKeep={PANE_ACTION_NOOP}
              onConfirmKeep={PANE_ACTION_NOOP}
              onUse={PANE_ACTION_NOOP}
              onRetry={state.retry}
              currency={currency}
              actorNameFor={actorNameFor}
              fillHeight={fillHeight}
            />
          </div>
          {state.previous.length === 0 ? null : (
            <div className="mt-3 shrink-0 border-t border-rule pt-2">
              <details>
                <summary className="cursor-pointer text-small font-medium text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40">
                  Session history ({state.previous.length})
                </summary>
                <ul id={listId} className="mt-2 flex flex-col gap-1">
                  {state.previous.map((record) => {
                    const row = savedHistoryRow(record, history);
                    const active = state.selectedGeneration === record.Generation;
                    return (
                      <li key={`${record.Generation}-${record.Session.ID}`}>
                        <button
                          type="button"
                          aria-pressed={active}
                          aria-current={active ? "true" : undefined}
                          onClick={() => state.select(record)}
                          className={`w-full rounded-lg border px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
                            active
                              ? "border-accent-300 bg-accent-50"
                              : "border-rule bg-white hover:bg-paper-subtle/60"
                          }`}
                        >
                          <span className="block text-small font-medium text-ink-900">
                            {row.primary}
                          </span>
                          <span className="mt-0.5 block text-caption text-ink-500">
                            {row.secondary}
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              </details>
            </div>
          )}
        </>
      ) : null}
    </section>
  );
}

/** The <1024 trigger. Exact labels come from the saved state matrix. */
export function SavedReconciliationTrigger({
  status,
  resolvedCount,
  open,
  onOpen,
  ref,
}: {
  status: SavedReconciliationStatus;
  resolvedCount: number;
  open: boolean;
  onOpen: () => void;
  ref?: Ref<HTMLButtonElement>;
}) {
  const words = savedTriggerState(status);
  return (
    <button
      ref={ref}
      type="button"
      aria-expanded={open}
      aria-label={words.ariaLabel}
      onClick={onOpen}
      className="sticky top-[var(--shell-header-h,64px)] z-20 flex min-h-14 w-full cursor-pointer items-center justify-between gap-2 border-b border-rule bg-paper/95 px-5 py-2 backdrop-blur transition-colors duration-150 hover:bg-paper-muted focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-500/40 lg:hidden"
    >
      <span className="flex min-w-0 items-baseline gap-2">
        <span className="text-small font-medium text-ink-700">{words.text}</span>
        {words.pill ? (
          <span className="inline-flex shrink-0 items-center rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
            Read-only
          </span>
        ) : null}
      </span>
      <span className="flex shrink-0 items-center gap-2">
        {status === "ready" ? (
          <span className="tnum inline-flex h-7 items-center rounded-full bg-accent-100 px-2.5 text-caption font-semibold text-accent-800">
            {resolvedCount} resolved
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
          className="text-ink-400"
        >
          <path d="m6 9 6 6 6-6" />
        </svg>
      </span>
    </button>
  );
}
