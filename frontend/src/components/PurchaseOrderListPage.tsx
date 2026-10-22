import { useEffect, useReducer, useRef, useState } from "react";
import { ListPurchaseOrders } from "@backend/backend/Quotation";
import {
  activeFilterCount,
  buildPOListQuery,
  canOpenRow,
  initialPOListState,
  initialPOListStateFor,
  modeState,
  poListQueryDelay,
  poListReducer,
  poNumberLabel,
  toggleCustomerFilter,
  type POListAction,
  type POListMode,
  type POListPageDTO,
  type POListRowDTO,
  type POListSourceMissingFilter,
  type POListState,
  type POListStateFilter,
} from "../poList";
import { DeletionDialog } from "./DeletionDialog";
import { Select } from "./Select";
import { Badge, Button, Card, fieldClass } from "./ui";

// Issue #469: the Purchase Order archive. Staff find a PO by its customer
// number, its source Quotation number, or its customer, then open the exact
// draft by id through the #468 editor. The server owns filtering, ordering and
// the 15-row page; this page only says which filters and page are current.
//
// #480 adds the admin-only permanent-deletion entry point. A row whose source
// Quotation was deleted renders the Source missing badge from #469's contract;
// the archive's source-missing filter is still #481's surface, so the control
// stays disabled here.

export interface PurchaseOrderListPageProps {
  /** Opens the #468 draft editor for one row id. */
  onOpen: (id: number) => void;
  /** Starts a new PO through the shared source selector. */
  onAdd: () => void;
  /** Whether the signed-in user may permanently delete a record. */
  isAdmin?: boolean;
  /**
   * Issue #491: Drafts and Archive are separate tabs now. The mode pins the
   * stored state the tab is about; the other filters are unchanged.
   */
  mode?: POListMode;
}

export interface PurchaseOrderListViewProps {
  state: POListState;
  dispatch: (action: POListAction) => void;
  onOpen: (id: number) => void;
  onAdd: () => void;
  onRetry: () => void;
  /** Pins the stored state to one tab; absent keeps the State control. */
  mode?: POListMode;
  /** Present only for an admin; absent renders no delete control at all. */
  onDelete?: (row: POListRowDTO) => void;
}

function errorText(error: unknown): string {
  if (typeof error === "string" && error.trim() !== "") return error;
  if (error instanceof Error && error.message.trim() !== "") return error.message;
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message.trim() !== "") return message;
  }
  return "The purchase orders could not be loaded. Try again.";
}

const STATE_OPTIONS = [
  { value: "", label: "All states" },
  { value: "draft", label: "Draft" },
  { value: "saved", label: "Saved" },
];

// #481: the source state is an independent predicate, not a third entry
// state. The server names the modes "only" and "exclude"; the labels say what
// the reader sees so neither a Draft nor a Saved record is implied to move.
const SOURCE_OPTIONS = [
  { value: "any", label: "Any source" },
  { value: "only", label: "Source missing only" },
  { value: "exclude", label: "Source present only" },
];

function SourceMissingBadge() {
  return (
    <span className="inline-flex items-center rounded-md border border-danger-200 bg-danger-50 px-2 py-0.5 text-micro font-medium uppercase text-danger-700">
      Source missing
    </span>
  );
}

export function PurchaseOrderListView({
  state,
  dispatch,
  onOpen,
  onAdd,
  onRetry,
  mode,
  onDelete,
}: PurchaseOrderListViewProps) {
  const pinnedState = mode === undefined ? null : modeState(mode);
  const filterCount =
    activeFilterCount(state.filters) -
    (pinnedState !== null && state.filters.state !== "" ? 1 : 0);
  const filtered = filterCount > 0;
  const heading =
    mode === "drafts"
      ? "Draft purchase orders"
      : mode === "archive"
        ? "Purchase order archive"
        : "Purchase orders";
  const description =
    mode === "drafts"
      ? "Purchase orders that are still being entered. Open one to continue entry."
      : mode === "archive"
        ? "Saved purchase orders, read-only in this release. Find one by its customer PO number, quotation number, or customer."
        : "Find a purchase order by its customer PO number, quotation number, or customer. Open a draft to continue entry, or a saved record to read it. Saved records are read-only in this release.";
  const emptyTitle = filtered
    ? "No purchase order matches these filters"
    : mode === "drafts"
      ? "No draft purchase orders"
      : mode === "archive"
        ? "No saved purchase orders yet"
        : "No purchase orders yet";
  const emptyBody = filtered
    ? "Clear the filters to see the whole archive."
    : mode === "drafts"
      ? "Start one from an issued quotation revision."
      : mode === "archive"
        ? "Saved records appear here after a draft is finalized."
        : "Start one from an issued quotation revision.";
  return (
    <section aria-labelledby="po-list-heading" className="flex flex-col gap-5">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1
            id="po-list-heading"
            className="text-title font-semibold text-ink-950"
          >
            {heading}
          </h1>
          <p className="mt-1 max-w-2xl text-body text-ink-500">{description}</p>
        </div>
        <Button
          variant="primary"
          aria-label="Add Purchase Order from a Quotation"
          onClick={onAdd}
        >
          Add Purchase Order from a Quotation
        </Button>
      </header>

      <Card className="p-4">
        <div
          className={`grid gap-4 sm:grid-cols-2 ${
            pinnedState === null ? "xl:grid-cols-5" : "xl:grid-cols-4"
          }`}
        >
          <label className="grid gap-1.5 sm:col-span-2">
            <span className="text-small font-medium text-ink-700">
              Search purchase orders
            </span>
            <input
              type="search"
              aria-label="Search purchase orders"
              className={fieldClass}
              value={state.filters.search}
              placeholder="PO number, quotation number, or customer"
              onChange={(event) =>
                dispatch({
                  type: "filtersChanged",
                  patch: { search: event.target.value },
                })
              }
            />
          </label>
          {pinnedState === null ? (
            <div className="grid gap-1.5">
              <span className="text-small font-medium text-ink-700">State</span>
              <Select
                ariaLabel="State"
                value={state.filters.state}
                options={STATE_OPTIONS}
                onChange={(value) =>
                  dispatch({
                    type: "filtersChanged",
                    patch: { state: value as POListStateFilter },
                  })
                }
              />
            </div>
          ) : null}
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              Received from
            </span>
            <input
              type="date"
              aria-label="Received from"
              className={fieldClass}
              value={state.filters.receivedFrom}
              onChange={(event) =>
                dispatch({
                  type: "filtersChanged",
                  patch: { receivedFrom: event.target.value },
                })
              }
            />
          </label>
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              Received to
            </span>
            <input
              type="date"
              aria-label="Received to"
              className={fieldClass}
              value={state.filters.receivedTo}
              onChange={(event) =>
                dispatch({
                  type: "filtersChanged",
                  patch: { receivedTo: event.target.value },
                })
              }
            />
          </label>
        </div>

        <fieldset className="mt-4 grid gap-2">
          <legend className="text-small font-medium text-ink-700">
            Customers
          </legend>
          {state.customers.length === 0 ? (
            <p className="text-caption text-ink-400">
              No customer has a purchase order yet.
            </p>
          ) : (
            <div className="grid max-h-40 gap-1 overflow-y-auto rounded-md border border-rule bg-paper-subtle/40 p-3 sm:grid-cols-2">
              {state.customers.map((customer) => (
                <label
                  key={customer.ID}
                  className="inline-flex cursor-pointer items-center gap-2 text-small text-ink-700"
                >
                  <input
                    type="checkbox"
                    className="h-4 w-4 cursor-pointer rounded border-rule accent-accent-700"
                    checked={state.filters.customerIds.includes(customer.ID)}
                    aria-label={`Customer ${customer.Name}`}
                    onChange={() =>
                      dispatch({
                        type: "filtersChanged",
                        patch: {
                          customerIds: toggleCustomerFilter(
                            state.filters,
                            customer.ID
                          ).customerIds,
                        },
                      })
                    }
                  />
                  {customer.Name}
                </label>
              ))}
            </div>
          )}
        </fieldset>

        <div className="mt-4 flex flex-wrap items-start justify-between gap-3 border-t border-rule pt-3">
          <div className="grid min-w-44 gap-1.5">
            <span className="text-small font-medium text-ink-700">Source</span>
            <Select
              ariaLabel="Source"
              value={state.filters.sourceMissing}
              options={SOURCE_OPTIONS}
              onChange={(value) =>
                dispatch({
                  type: "filtersChanged",
                  patch: { sourceMissing: value as POListSourceMissingFilter },
                })
              }
            />
            <p className="max-w-xs text-caption text-ink-400">
              Source missing is independent of Draft/Saved.
            </p>
          </div>
          <div className="flex items-center gap-2.5">
            <span className="text-caption text-ink-500">
              {filtered
                ? `${filterCount} ${filterCount === 1 ? "filter" : "filters"} active`
                : "No filters"}
            </span>
            <Button
              aria-label="Reset filters"
              disabled={!filtered}
              onClick={() =>
                dispatch({
                  type: "resetFilters",
                  state: pinnedState ?? undefined,
                })
              }
            >
              Reset filters
            </Button>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden">
        {state.loading && state.rows.length === 0 ? (
          <div className="space-y-3 p-6" aria-label="Loading purchase orders">
            {[0, 1, 2, 3].map((item) => (
              <div
                key={item}
                className="h-12 animate-pulse rounded-lg bg-paper-muted"
              />
            ))}
          </div>
        ) : state.error !== null ? (
          <div className="flex flex-col items-center gap-3 px-6 py-14 text-center">
            <p role="alert" className="max-w-md text-body text-danger-700">
              {state.error}
            </p>
            <Button onClick={onRetry}>Try again</Button>
          </div>
        ) : state.rows.length === 0 ? (
          <div className="flex flex-col items-center gap-3 px-6 py-16 text-center">
            <p className="text-lead font-medium text-ink-900">{emptyTitle}</p>
            <p className="max-w-md text-body text-ink-500">{emptyBody}</p>
            {filtered ? (
              <Button
                aria-label="Reset filters"
                onClick={() =>
                  dispatch({
                    type: "resetFilters",
                    state: pinnedState ?? undefined,
                  })
                }
              >
                Reset filters
              </Button>
            ) : (
              <Button
                variant="primary"
                aria-label="Add Purchase Order from a Quotation"
                onClick={onAdd}
              >
                Add Purchase Order from a Quotation
              </Button>
            )}
          </div>
        ) : (
          <div className="overflow-x-auto scroll-thin">
            <table className="w-full min-w-[900px] text-left text-body">
              <caption className="sr-only">Purchase orders</caption>
              <thead>
                <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                  <th className="px-5 py-3 font-medium">Customer PO</th>
                  <th className="px-4 py-3 font-medium">Quotation</th>
                  <th className="px-4 py-3 font-medium">Customer</th>
                  <th className="px-4 py-3 font-medium">Received</th>
                  <th className="px-4 py-3 font-medium">State</th>
                  <th className="px-5 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-rule">
                {state.rows.map((row) => {
                  const openable = canOpenRow(row);
                  return (
                    <tr
                      key={row.ID}
                      className="transition-colors duration-150 hover:bg-paper-subtle/60"
                    >
                      <td className="px-5 py-3 align-middle">
                        <span
                          className={`tnum text-body font-medium ${
                            row.PONumber.trim() === ""
                              ? "text-ink-400"
                              : "text-ink-900"
                          }`}
                        >
                          {poNumberLabel(row)}
                        </span>
                        <p className="mt-0.5 text-caption uppercase text-ink-400">
                          {row.Currency}
                        </p>
                      </td>
                      <td className="px-4 py-3 align-middle">
                        <span className="tnum text-body text-ink-700">
                          {row.QuoteNumber}
                        </span>
                        <p className="text-caption text-ink-400">
                          {row.SourceMissing
                            ? `Former quotation · revision ${row.SourceRevision}`
                            : `Revision ${row.SourceRevision}`}
                        </p>
                        {row.SourceMissing ? <SourceMissingBadge /> : null}
                      </td>
                      <td className="px-4 py-3 align-middle text-body text-ink-700">
                        {row.CustomerName}
                      </td>
                      <td className="tnum whitespace-nowrap px-4 py-3 align-middle text-body text-ink-700">
                        {row.ReceivedDate}
                      </td>
                      <td className="px-4 py-3 align-middle">
                        <Badge tone={row.State === "draft" ? "accent" : "success"}>
                          {row.State === "draft" ? "Draft" : "Saved"}
                        </Badge>
                      </td>
                      <td className="px-5 py-3 text-right align-middle">
                        <span className="inline-flex items-center gap-1.5">
                          <Button
                            size="sm"
                            aria-label={`Open purchase order ${poNumberLabel(row)}`}
                            disabled={!openable}
                            title={
                              row.State === "draft"
                                ? "Open this draft"
                                : "Open this saved purchase order (read-only)"
                            }
                            onClick={() => onOpen(row.ID)}
                          >
                            Open
                          </Button>
                          {onDelete !== undefined ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              aria-label={`Delete purchase order ${poNumberLabel(row)}`}
                              className="hover:!bg-danger-50 hover:!text-danger-600"
                              onClick={() => onDelete(row)}
                            >
                              Delete
                            </Button>
                          ) : null}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {state.rows.length > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-caption text-ink-500">
            {state.total} purchase{" "}
            {state.total === 1 ? "order" : "orders"} · page {state.page} of{" "}
            {state.totalPages}
          </p>
          <div className="flex items-center gap-2">
            <Button
              aria-label="Previous page"
              disabled={state.loading || state.page <= 1}
              onClick={() =>
                dispatch({ type: "pageChanged", page: state.page - 1 })
              }
            >
              Previous
            </Button>
            <Button
              aria-label="Next page"
              disabled={state.loading || state.page >= state.totalPages}
              onClick={() =>
                dispatch({ type: "pageChanged", page: state.page + 1 })
              }
            >
              Next
            </Button>
          </div>
        </div>
      ) : null}
    </section>
  );
}

export function PurchaseOrderListPage({
  onOpen,
  onAdd,
  isAdmin = false,
  mode,
}: PurchaseOrderListPageProps) {
  const [state, dispatch] = useReducer(poListReducer, undefined, () =>
    mode === undefined ? initialPOListState : initialPOListStateFor(mode)
  );
  // Every effect run is a new request; the reducer drops an older answer that
  // lands after a newer one started. `reload` is the explicit retry token.
  const requestSeq = useRef(0);
  const lastSearch = useRef(state.filters.search);
  const [reload, setReload] = useState(0);
  const [deletionTarget, setDeletionTarget] = useState<{
    id: number;
    label: string;
  } | null>(null);

  useEffect(() => {
    const seq = ++requestSeq.current;
    const previous = lastSearch.current;
    lastSearch.current = state.filters.search;
    const delay = poListQueryDelay(previous, state.filters.search);
    const { query, error } = buildPOListQuery(state.filters, state.page);
    if (query === null) {
      dispatch({ type: "queryStarted", seq });
      dispatch({
        type: "queryFailed",
        seq,
        error: error ?? "These filters cannot be queried.",
      });
      return;
    }
    dispatch({ type: "queryStarted", seq });
    let cancelled = false;
    const timer = setTimeout(() => {
      void ListPurchaseOrders(query)
        .then((page) => {
          if (cancelled) return;
          dispatch({
            type: "queryLoaded",
            seq,
            page: page as POListPageDTO,
          });
        })
        .catch((cause: unknown) => {
          if (cancelled) return;
          dispatch({ type: "queryFailed", seq, error: errorText(cause) });
        });
    }, delay);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [state.filters, state.page, reload]);

  return (
    <>
      <PurchaseOrderListView
        state={state}
        dispatch={dispatch}
        onOpen={onOpen}
        onAdd={onAdd}
        mode={mode}
        onRetry={() => setReload((current) => current + 1)}
        onDelete={
          isAdmin
            ? (row) =>
                setDeletionTarget({
                  id: row.ID,
                  label: row.PONumber.trim() === ""
                    ? `purchase order #${row.ID}`
                    : `purchase order ${row.PONumber}`,
                })
            : undefined
        }
      />
      {deletionTarget !== null ? (
        <DeletionDialog
          targetType="purchase_order"
          targetId={deletionTarget.id}
          targetLabel={deletionTarget.label}
          onClose={() => setDeletionTarget(null)}
          onDeleted={() => {
            setDeletionTarget(null);
            setReload((current) => current + 1);
          }}
        />
      ) : null}
    </>
  );
}
