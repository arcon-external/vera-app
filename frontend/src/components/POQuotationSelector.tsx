import { useCallback, useEffect, useReducer, useRef } from "react";
import {
  canConfirm,
  canContinue,
  initialPOSourceSelectorState,
  poSourceSelectorReducer,
  selectionOf,
  visibleRows,
} from "../poSelector";
import type {
  POSourcePage,
  POSourceQuery,
  POSourceRevision,
  POSourceSelection,
  POSourceSelectorAction,
  POSourceSelectorState,
  POSourceValidation,
} from "../poSelector";
import { Button, fieldClass } from "./ui";

// Issue #467: the shared three-step source selector. Step 1 chooses an issued
// Quotation, step 2 its exact issued revision (auto-selected when there is only
// one), step 3 confirms. Both PO entry paths mount this same component; the
// caller receives an exact {quotationId, revisionNumber}.
//
// The data loaders are injected rather than imported so the component has no
// hidden transport, and the pure decisions live in ../poSelector. The view is
// exported separately because it is what the SSR suite renders.

export interface POQuotationSelectorProps {
  open: boolean;
  onClose: () => void;
  /** Receives the confirmed pair. The caller opens PO entry with it (#468). */
  onConfirm: (selection: POSourceSelection) => void;
  /** One page of issued Quotations (15 per page, server-fixed). */
  loadPage: (query: POSourceQuery) => Promise<POSourcePage>;
  /** Issued revisions of one Quotation, newest first. */
  loadRevisions: (quotationId: number) => Promise<POSourceRevision[]>;
  /** Confirm-time server revalidation of the exact pair. */
  validate: (selection: POSourceSelection) => Promise<POSourceValidation>;
}

export interface POQuotationSelectorViewProps {
  state: POSourceSelectorState;
  dispatch: (action: POSourceSelectorAction) => void;
  onClose: () => void;
  onConfirm: () => void;
}

function errorText(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== "") return error.message;
  return "The request failed. Try again.";
}

function validationMessage(reason?: string): string {
  if (reason === "quotation_not_found") {
    return "This quotation no longer exists. Go back and choose another source.";
  }
  if (reason === "revision_not_found") {
    return "That revision is no longer available. Go back and choose another revision.";
  }
  return "This source can no longer be used. Go back and choose another.";
}

function issuedDate(issuedAt: string): string {
  // The API sends RFC3339. The date is what distinguishes revisions in a
  // picker; the clock time adds noise, and slicing avoids locale drift
  // between the desktop and the rendered test.
  return issuedAt.length >= 10 ? issuedAt.slice(0, 10) : issuedAt;
}

function SourceSummary({ state }: { state: POSourceSelectorState }) {
  if (!state.selected) return null;
  return (
    <div className="rounded-lg border border-rule bg-paper-muted px-4 py-3">
      <p className="tnum text-small font-medium text-ink-900">
        {state.selected.quoteNumber}
      </p>
      <p className="text-small text-ink-700">{state.selected.customerName}</p>
      <p className="text-small text-ink-500">{state.selected.subject}</p>
    </div>
  );
}

function StepOne({
  state,
  dispatch,
}: Pick<POQuotationSelectorViewProps, "state" | "dispatch">) {
  const rows = visibleRows(state.rows, state.hideUsed);
  return (
    <div className="grid gap-4">
      <label className="grid gap-1.5">
        <span className="text-small font-medium text-ink-700">Search quotations</span>
        <input
          type="search"
          aria-label="Search quotations"
          className={fieldClass}
          value={state.search}
          placeholder="Number, subject, customer, or item"
          onChange={(event) =>
            dispatch({ type: "searchChanged", search: event.target.value })
          }
        />
      </label>
      <label className="inline-flex w-fit cursor-pointer items-center gap-2 text-small text-ink-700">
        <input
          type="checkbox"
          className="h-4 w-4 cursor-pointer rounded border-rule accent-accent-700"
          checked={state.hideUsed}
          onChange={(event) =>
            dispatch({ type: "hideUsedChanged", hideUsed: event.target.checked })
          }
        />
        Hide quotations already used
      </label>

      {state.rowsError !== null ? (
        <p role="alert" className="text-small text-danger-600">
          {state.rowsError}
        </p>
      ) : null}

      {state.rowsLoading && rows.length === 0 ? (
        <p className="py-8 text-center text-small text-ink-500">Loading quotations…</p>
      ) : rows.length === 0 ? (
        <p className="py-8 text-center text-small text-ink-500">
          {state.search.trim() === ""
            ? "No issued quotations yet."
            : "No issued quotation matches that search."}
        </p>
      ) : (
        <ul className="grid max-h-[42vh] gap-2 overflow-y-auto scroll-thin pr-1">
          {rows.map((row) => {
            const used = row.hasActivePO;
            return (
              <li key={row.id}>
                <button
                  type="button"
                  disabled={used}
                  aria-disabled={used ? "true" : undefined}
                  aria-label={`Quotation ${row.quoteNumber}, ${row.customerName}`}
                  onClick={() => dispatch({ type: "quotationSelected", quotation: row })}
                  className={`grid w-full gap-1 rounded-lg border px-4 py-3 text-left transition ${
                    used
                      ? "cursor-not-allowed border-rule bg-paper-muted text-ink-400"
                      : "cursor-pointer border-rule bg-white text-ink-900 hover:border-accent-400 hover:bg-paper-muted"
                  }`}
                >
                  <span className="flex flex-wrap items-baseline justify-between gap-2">
                    <span className="tnum text-small font-semibold">{row.quoteNumber}</span>
                    <span className="text-caption text-ink-500">
                      Revision {row.latestRevision} ·{" "}
                      {row.issuedRevisionCount === 1
                        ? "1 issued revision"
                        : `${row.issuedRevisionCount} issued revisions`}
                    </span>
                  </span>
                  <span className="text-small">{row.customerName}</span>
                  <span className="text-small text-ink-500">{row.subject}</span>
                  {used ? (
                    <span className="text-caption font-medium text-danger-600">
                      Already has a purchase order
                    </span>
                  ) : null}
                </button>
              </li>
            );
          })}
        </ul>
      )}

      <div className="flex items-center justify-between gap-3 border-t border-rule pt-3">
        <Button
          size="sm"
          aria-label="Previous page"
          disabled={state.page <= 1 || state.rowsLoading}
          onClick={() => dispatch({ type: "pageChanged", page: state.page - 1 })}
        >
          Previous
        </Button>
        <span className="tnum text-small text-ink-500">
          {state.rowsLoading ? "Loading… · " : ""}Page {state.page} of {state.totalPages}
        </span>
        <Button
          size="sm"
          aria-label="Next page"
          disabled={state.page >= state.totalPages || state.rowsLoading}
          onClick={() => dispatch({ type: "pageChanged", page: state.page + 1 })}
        >
          Next
        </Button>
      </div>
    </div>
  );
}

function StepTwo({
  state,
  dispatch,
}: Pick<POQuotationSelectorViewProps, "state" | "dispatch">) {
  return (
    <div className="grid gap-4">
      <SourceSummary state={state} />
      {state.revisionsError !== null ? (
        <p role="alert" className="text-small text-danger-600">
          {state.revisionsError}
        </p>
      ) : null}
      {state.revisionsLoading ? (
        <p className="py-6 text-center text-small text-ink-500">Loading revisions…</p>
      ) : (
        <>
          {state.revisions.length === 1 ? (
            <p className="text-small text-ink-500">
              Only issued revision — selected automatically.
            </p>
          ) : (
            <p className="text-small text-ink-500">
              This quotation has {state.revisions.length} issued revisions. Choose
              the one the purchase order follows.
            </p>
          )}
          <ul className="grid max-h-[42vh] gap-2 overflow-y-auto scroll-thin pr-1">
            {state.revisions.map((revision) => {
              const selected = state.selectedRevision === revision.revision;
              return (
                <li key={revision.revision}>
                  <button
                    type="button"
                    aria-pressed={selected}
                    aria-label={`Revision ${revision.revision}`}
                    onClick={() =>
                      dispatch({ type: "revisionSelected", revision: revision.revision })
                    }
                    className={`flex w-full cursor-pointer items-center justify-between gap-3 rounded-lg border px-4 py-3 text-left transition ${
                      selected
                        ? "border-accent-500 bg-accent-50 text-ink-900"
                        : "border-rule bg-white text-ink-900 hover:border-accent-400 hover:bg-paper-muted"
                    }`}
                  >
                    <span className="text-small font-medium">
                      Revision {revision.revision}
                    </span>
                    <span className="tnum text-caption text-ink-500">
                      Issued {issuedDate(revision.issuedAt)}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </>
      )}
      {state.selectedRevision === null && !state.revisionsLoading ? (
        <p className="text-caption text-ink-500">
          Choose a revision to continue.
        </p>
      ) : null}
    </div>
  );
}

function StepThree({ state }: Pick<POQuotationSelectorViewProps, "state">) {
  return (
    <div className="grid gap-4">
      <SourceSummary state={state} />
      <div className="rounded-lg border border-rule bg-white px-4 py-3">
        <p className="text-small font-medium text-ink-900">
          Revision {state.selectedRevision ?? "not selected"}
        </p>
        <p className="text-small text-ink-500">
          The purchase order will be linked to this exact issued revision.
        </p>
      </div>
      {state.confirmError !== null ? (
        <p role="alert" className="text-small text-danger-600">
          {state.confirmError}
        </p>
      ) : null}
    </div>
  );
}

export function POQuotationSelectorView({
  state,
  dispatch,
  onClose,
  onConfirm,
}: POQuotationSelectorViewProps) {
  const confirming = state.confirming;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/45 p-5"
      role="presentation"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Select a quotation for the purchase order"
        className="flex max-h-[85vh] w-full max-w-2xl flex-col rounded-xl border border-rule bg-white shadow-card"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-3 border-b border-rule px-6 py-4">
          <div>
            <h2 className="text-title font-semibold text-ink-950">
              {state.step === 1
                ? "Select a quotation"
                : state.step === 2
                  ? "Select a revision"
                  : "Confirm the source"}
            </h2>
            <p className="text-small text-ink-500">Step {state.step} of 3</p>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto scroll-thin px-6 py-5">
          {state.step === 1 ? <StepOne state={state} dispatch={dispatch} /> : null}
          {state.step === 2 ? <StepTwo state={state} dispatch={dispatch} /> : null}
          {state.step === 3 ? <StepThree state={state} /> : null}
        </div>

        <div className="flex items-center justify-between gap-3 border-t border-rule px-6 py-4">
          <div>
            {state.step > 1 ? (
              <Button
                aria-label="Back"
                disabled={confirming}
                onClick={() => dispatch({ type: "back" })}
              >
                Back
              </Button>
            ) : null}
          </div>
          <div>
            {state.step === 2 ? (
              <Button
                variant="primary"
                aria-label="Continue to confirmation"
                disabled={!canContinue(state)}
                onClick={() => dispatch({ type: "continue" })}
              >
                Continue
              </Button>
            ) : null}
            {state.step === 3 ? (
              <Button
                variant="primary"
                aria-label="Confirm source"
                disabled={!canConfirm(state)}
                onClick={onConfirm}
              >
                {confirming ? "Checking…" : "Confirm source"}
              </Button>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  );
}

export function POQuotationSelector({
  open,
  onClose,
  onConfirm,
  loadPage,
  loadRevisions,
  validate,
}: POQuotationSelectorProps) {
  const [state, dispatch] = useReducer(
    poSourceSelectorReducer,
    initialPOSourceSelectorState
  );

  // Loaders are kept in refs: App may pass fresh closures each render, and a
  // loader identity in the dependency list would refetch on every keystroke
  // elsewhere in the page.
  const pageLoader = useRef(loadPage);
  pageLoader.current = loadPage;
  const revisionLoader = useRef(loadRevisions);
  revisionLoader.current = loadRevisions;
  const validator = useRef(validate);
  validator.current = validate;

  useEffect(() => {
    if (!open) {
      dispatch({ type: "reset" });
    }
  }, [open]);

  useEffect(() => {
    if (!open || state.step !== 1) return;
    let stale = false;
    dispatch({ type: "rowsLoading" });
    // A search is typed one character at a time; without a pause the modal
    // would send a request per keystroke. The list keeps its previous rows
    // visible while the answer is in flight, so the pause is not a blank
    // screen. Clearing the box, and paging/filtering, query immediately.
    const delay = state.search.trim() === "" ? 0 : 250;
    const timer = setTimeout(() => {
      pageLoader
        .current({ page: state.page, search: state.search, hideUsed: state.hideUsed })
        .then((page) => {
          if (!stale) dispatch({ type: "rowsLoaded", page });
        })
        .catch((error: unknown) => {
          if (!stale) dispatch({ type: "rowsFailed", error: errorText(error) });
        });
    }, delay);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  }, [open, state.step, state.page, state.search, state.hideUsed]);

  const selectedID = state.selected?.id ?? null;
  useEffect(() => {
    if (!open || state.step !== 2 || selectedID === null) return;
    let stale = false;
    dispatch({ type: "revisionsLoading" });
    revisionLoader
      .current(selectedID)
      .then((revisions) => {
        if (!stale) {
          dispatch({ type: "revisionsLoaded", quotationId: selectedID, revisions });
        }
      })
      .catch((error: unknown) => {
        if (!stale) {
          dispatch({
            type: "revisionsFailed",
            quotationId: selectedID,
            error: errorText(error),
          });
        }
      });
    return () => {
      stale = true;
    };
  }, [open, state.step, selectedID]);

  const confirm = useCallback(async () => {
    const selection = selectionOf(state);
    if (!canConfirm(state) || selection === null) return;
    dispatch({ type: "confirmStarted" });
    try {
      const outcome = await validator.current(selection);
      if (!outcome.valid) {
        dispatch({ type: "confirmFailed", error: validationMessage(outcome.reason) });
        return;
      }
      onConfirm(selection);
    } catch (error) {
      dispatch({ type: "confirmFailed", error: errorText(error) });
    }
  }, [state, onConfirm]);

  if (!open) return null;
  return (
    <POQuotationSelectorView
      state={state}
      dispatch={dispatch}
      onClose={onClose}
      onConfirm={() => void confirm()}
    />
  );
}
