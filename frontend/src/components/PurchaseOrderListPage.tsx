import { useEffect, useReducer, useRef, useState } from "react";
import { ListPurchaseOrders } from "@backend/backend/Quotation";
import {
  activeFilterCount,
  buildPOListQuery,
  canOpenRow,
  initialPOListState,
  poListQueryDelay,
  poListReducer,
  poNumberLabel,
  toggleCustomerFilter,
  type POListAction,
  type POListPageDTO,
  type POListState,
  type POListStateFilter,
} from "../poList";
import { Select } from "./Select";
import { Badge, Button, Card, fieldClass } from "./ui";

// Issue #469: the Purchase Order archive. Staff find a PO by its customer
// number, its source Quotation number, or its customer, then open the exact
// draft by id through the #468 editor. The server owns filtering, ordering and
// the 15-row page; this page only says which filters and page are current.
//
// What it deliberately does not render: attachments and correction history
// (no such table exists yet, #472/#473), any fulfilment or payment status
// (never implemented, and the parent spec forbids inventing one), and a
// Source missing badge for a state that cannot exist until deletion ships
// (#480/#481). The filter is present but disabled, with that stated.

export interface PurchaseOrderListPageProps {
  /** Opens the #468 draft editor for one row id. */
  onOpen: (id: number) => void;
  /** Starts a new PO through the shared source selector. */
  onAdd: () => void;
}

export interface PurchaseOrderListViewProps {
  state: POListState;
  dispatch: (action: POListAction) => void;
  onOpen: (id: number) => void;
  onAdd: () => void;
  onRetry: () => void;
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
}: PurchaseOrderListViewProps) {
  const filterCount = activeFilterCount(state.filters);
  const filtered = filterCount > 0;
  return (
    <section aria-labelledby="po-list-heading" className="flex flex-col gap-5">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1
            id="po-list-heading"
            className="text-title font-semibold text-ink-950"
          >
            Purchase orders
          </h1>
          <p className="mt-1 max-w-2xl text-body text-ink-500">
            Find a purchase order by its customer PO number, quotation number,
            or customer. Open a draft to continue entry, or a saved record to
            read it. Saved records are read-only in this release.
          </p>
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
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
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
          <div className="grid gap-1">
            <label
              className="inline-flex w-fit cursor-not-allowed items-center gap-2 text-small text-ink-400"
              title="Available when Quotation deletion ships"
            >
              <input
                type="checkbox"
                aria-label="Source missing only"
                className="h-4 w-4 rounded border-rule"
                checked={state.filters.sourceMissing === "only"}
                disabled
                aria-disabled="true"
                readOnly
              />
              Source missing only
            </label>
            <p className="max-w-md text-caption text-ink-400">
              Available when Quotation deletion ships. No purchase order can be
              missing its source today.
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
              onClick={() => dispatch({ type: "resetFilters" })}
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
            <p className="text-lead font-medium text-ink-900">
              {filtered
                ? "No purchase order matches these filters"
                : "No purchase orders yet"}
            </p>
            <p className="max-w-md text-body text-ink-500">
              {filtered
                ? "Clear the filters to see the whole archive."
                : "Start one from an issued quotation revision."}
            </p>
            {filtered ? (
              <Button
                aria-label="Reset filters"
                onClick={() => dispatch({ type: "resetFilters" })}
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
                          Revision {row.SourceRevision}
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
}: PurchaseOrderListPageProps) {
  const [state, dispatch] = useReducer(
    poListReducer,
    undefined,
    () => initialPOListState
  );
  // Every effect run is a new request; the reducer drops an older answer that
  // lands after a newer one started. `reload` is the explicit retry token.
  const requestSeq = useRef(0);
  const lastSearch = useRef(state.filters.search);
  const [reload, setReload] = useState(0);

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
    <PurchaseOrderListView
      state={state}
      dispatch={dispatch}
      onOpen={onOpen}
      onAdd={onAdd}
      onRetry={() => setReload((current) => current + 1)}
    />
  );
}
