import { useEffect, useReducer, useRef, useState } from "react";
import { ListPurchaseOrders } from "@backend/backend/Quotation";
import { errorMessage } from "../errors";
import {
  activeFilterCount,
  buildPOListQuery,
  canOpenRow,
  customerCellLabel,
  initialPOListState,
  initialPOListStateFor,
  modeState,
  poListQueryDelay,
  poListReducer,
  poNumberLabel,
  quotationCellLabel,
  sourceRevisionLabel,
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
// paging; this page only says which filters, page and page size are current.
// Issue #492: the page size is the account's saved preference (default 15),
// and the server's effective PageSize is authoritative after each load.
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
  /**
   * Issue #492: the account's remembered page size, used for the initial list
   * state. The server's own page size is authoritative once a page loads.
   */
  pageSize?: number;
}

export interface PurchaseOrderListViewProps {
  state: POListState;
  dispatch: (action: POListAction) => void;
  /**
   * Opens one row by id. May return a promise the row awaits: while it is
   * pending the row's Open button reads "Opening…" and is disabled; a
   * rejection re-enables it and the caller shows the failure.
   */
  onOpen: (id: number) => void | Promise<unknown>;
  onAdd: () => void;
  onRetry: () => void;
  /** Pins the stored state to one tab; absent keeps the State control. */
  mode?: POListMode;
  /** Present only for an admin; absent renders no ⋯ menu column at all. */
  onDelete?: (row: POListRowDTO) => void;
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

const RECEIVED_MONTHS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
] as const;

/**
 * Ticket 03: the docket's received date. A real YYYY-MM-DD calendar date
 * reads `2 Oct 2026`; anything else (blank, unparsable) renders as-is so a
 * missing date is never invented. Exported for the list suite.
 */
export function formatReceivedDate(raw: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw.trim());
  if (match === null) return raw;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (month < 1 || month > 12 || day < 1 || day > 31) return raw;
  const date = new Date(Date.UTC(year, month - 1, day));
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return raw;
  }
  return `${day} ${RECEIVED_MONTHS[month - 1]} ${year}`;
}

/**
 * Ticket 03: one row's admin menu. The destructive action lives here, never
 * beside Open: one "⋯" trigger, one danger menu item, Escape and outside
 * pointer close it and return focus. Non-admins never render this component
 * (or its table column), not a disabled copy of it.
 */
function RowMenu({ label, onDelete }: { label: string; onDelete: () => void }) {
  const [open, setOpen] = useState(false);
  const groupRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (groupRef.current?.contains(event.target as Node) !== true) {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open ]);

  const close = () => {
    setOpen(false);
    groupRef.current?.querySelector("button")?.focus();
  };

  return (
    <div
      ref={groupRef}
      className="relative inline-flex"
      onKeyDown={(event) => {
        if (event.key === "Escape" && open) {
          event.preventDefault();
          close();
        }
      }}
    >
      <Button
        variant="ghost"
        size="sm"
        className="h-11 w-11 px-0 lg:h-auto lg:w-8"
        aria-label={`More actions for purchase order ${label}`}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
      >
        <span aria-hidden="true" className="text-lead leading-none">
          ⋯
        </span>
      </Button>
      {open ? (
        <div
          role="menu"
          aria-label={`More actions for purchase order ${label}`}
          className="absolute right-0 top-full z-30 mt-1 w-56 overflow-hidden rounded-lg border border-rule bg-white py-1 shadow-pop"
        >
          <button
            type="button"
            role="menuitem"
            aria-label={`Delete purchase order ${label}`}
            onClick={() => {
              setOpen(false);
              onDelete();
            }}
            className="flex w-full items-center gap-2.5 px-4 py-2.5 text-left text-small text-danger-600 transition-colors hover:bg-danger-50 focus-visible:outline-hidden focus-visible:bg-danger-50 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-danger-500/40"
          >
            Delete purchase order…
          </button>
        </div>
      ) : null}
    </div>
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
  // Ticket 03: the filter panel collapses below 1024. Its open state is local
  // UI state, so the keep-alive from ticket 02 restores it (with the chips)
  // exactly after a settings round-trip.
  const [filtersOpen, setFiltersOpen] = useState(false);
  // One row opening at a time. Set on activation, cleared when the open
  // settles; a rejection re-enables the button and the caller toasts.
  const [openingId, setOpeningId] = useState<number | null>(null);
  const showAdmin = onDelete !== undefined;

  const openRow = (id: number) => {
    if (openingId !== null) return;
    setOpeningId(id);
    const done = () => setOpeningId(null);
    try {
      const result = onOpen(id);
      if (
        result !== null &&
        typeof (result as Promise<unknown>).then === "function"
      ) {
        void (result as Promise<unknown>).then(done, done);
      } else {
        done();
      }
    } catch {
      done();
    }
  };
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

  // Ticket 03: removable chips under search below 1024. The pinned tab state
  // is the tab, never a chip.
  const chips: { key: string; label: string; clear: () => void }[] = [];
  const searchText = state.filters.search.trim();
  if (searchText !== "") {
    chips.push({
      key: "search",
      label: searchText,
      clear: () => dispatch({ type: "filtersChanged", patch: { search: "" } }),
    });
  }
  const receivedFrom = state.filters.receivedFrom.trim();
  const receivedTo = state.filters.receivedTo.trim();
  if (receivedFrom !== "" || receivedTo !== "") {
    chips.push({
      key: "received",
      label:
        receivedFrom !== "" && receivedTo !== ""
          ? `Received ${formatReceivedDate(receivedFrom)}–${formatReceivedDate(receivedTo)}`
          : receivedFrom !== ""
            ? `Received from ${formatReceivedDate(receivedFrom)}`
            : `Received to ${formatReceivedDate(receivedTo)}`,
      clear: () =>
        dispatch({
          type: "filtersChanged",
          patch: { receivedFrom: "", receivedTo: "" },
        }),
    });
  }
  if (state.filters.sourceMissing !== "any") {
    chips.push({
      key: "source",
      label:
        state.filters.sourceMissing === "only"
          ? "Source missing only"
          : "Source present only",
      clear: () =>
        dispatch({
          type: "filtersChanged",
          patch: { sourceMissing: "any" },
        }),
    });
  }
  for (const customerId of state.filters.customerIds) {
    const customer = state.customers.find((entry) => entry.ID === customerId);
    chips.push({
      key: `customer-${customerId}`,
      label: `Customer: ${customer?.Name ?? `#${customerId}`}`,
      clear: () =>
        dispatch({
          type: "filtersChanged",
          patch: {
            customerIds: state.filters.customerIds.filter(
              (id) => id !== customerId
            ),
          },
        }),
    });
  }

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
        <div className="flex items-end gap-3">
          <label className="grid min-w-0 flex-1 gap-1.5">
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
          <Button
            aria-label={filtersOpen ? "Hide filters" : "Show filters"}
            aria-expanded={filtersOpen}
            aria-controls="po-list-filters"
            onClick={() => setFiltersOpen((current) => !current)}
            className="shrink-0 lg:hidden"
          >
            Filters
            {filterCount > 0 ? (
              <span
                className="tnum flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-accent-100 px-1 text-[11px] font-semibold leading-none text-accent-800"
                aria-label={`${filterCount} active`}
              >
                {filterCount > 99 ? "99+" : filterCount}
              </span>
            ) : null}
          </Button>
        </div>
        {chips.length > 0 ? (
          <ul
            aria-label="Active filters"
            className="mt-3 flex flex-wrap gap-2 lg:hidden"
          >
            {chips.map((chip) => (
              <li key={chip.key}>
                <button
                  type="button"
                  aria-label={`Remove ${chip.label} filter`}
                  onClick={chip.clear}
                  className="inline-flex min-h-9 cursor-pointer items-center gap-1.5 rounded-full border border-rule bg-white px-3 text-small text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                >
                  <span className="max-w-44 truncate">{chip.label}</span>
                  <span aria-hidden="true" className="text-ink-400">
                    ×
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : null}

        <div
          id="po-list-filters"
          className={filtersOpen ? "lg:block" : "hidden lg:block"}
        >
          <div
            className={`mt-4 grid gap-4 sm:grid-cols-2 ${
              pinnedState === null ? "xl:grid-cols-5" : "xl:grid-cols-4"
            }`}
          >
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
        </div>
      </Card>

      <Card className="overflow-hidden">
        {state.loading && state.rows.length === 0 ? (
          <>
            <div
              className="hidden space-y-3 p-6 lg:block"
              aria-label="Loading purchase orders"
            >
              {[0, 1, 2, 3].map((item) => (
                <div
                  key={item}
                  className="h-12 animate-pulse rounded-lg bg-paper-muted"
                />
              ))}
            </div>
            <div
              className="flex flex-col gap-3 p-4 lg:hidden"
              aria-label="Loading purchase orders"
            >
              {[0, 1, 2, 3].map((item) => (
                <div
                  key={item}
                  className="h-12 animate-pulse rounded-lg bg-paper-muted"
                />
              ))}
            </div>
          </>
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
          <>
            <div
              className={`hidden lg:block ${state.loading ? "opacity-60" : ""}`}
              aria-busy={state.loading || undefined}
            >
              <table className="w-full table-fixed text-left text-body">
                <caption className="sr-only">Purchase orders</caption>
                <thead>
                  <tr className="h-10 border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                    <th className="w-[24%] px-5 py-3 font-medium">
                      Customer PO
                    </th>
                    <th className="w-[26%] px-4 py-3 font-medium">
                      Quotation
                    </th>
                    <th className="w-[22%] px-4 py-3 font-medium">Customer</th>
                    <th className="w-[14%] px-4 py-3 font-medium">Received</th>
                    <th className="w-[9%] px-4 py-3 font-medium">State</th>
                    <th className="w-20 px-4 py-3">
                      <span className="sr-only">Open</span>
                    </th>
                    {showAdmin ? (
                      <th className="w-12 py-3 pl-0 pr-4">
                        <span className="sr-only">More actions</span>
                      </th>
                    ) : null}
                  </tr>
                </thead>
                <tbody className="divide-y divide-rule">
                  {state.rows.map((row) => {
                    const openable = canOpenRow(row);
                    const opening = openingId === row.ID;
                    return (
                      <tr
                        key={row.ID}
                        className="h-14 transition-colors duration-150 hover:bg-paper-subtle/60 active:bg-paper-muted/70"
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
                          <p className="mt-0.5 text-micro uppercase text-ink-400">
                            {row.Currency}
                          </p>
                        </td>
                        <td className="px-4 py-3 align-middle">
                          <span className="tnum block truncate text-body text-ink-700">
                            {quotationCellLabel(row)}
                          </span>
                          <p className="text-caption text-ink-400">
                            {sourceRevisionLabel(row)}
                          </p>
                          {row.SourceMissing ? <SourceMissingBadge /> : null}
                        </td>
                        <td className="px-4 py-3 align-middle text-body text-ink-700">
                          <span className="block max-w-full truncate">
                            {customerCellLabel(row)}
                          </span>
                        </td>
                        <td className="tnum whitespace-nowrap px-4 py-3 align-middle text-body text-ink-700">
                          {formatReceivedDate(row.ReceivedDate)}
                        </td>
                        <td className="px-4 py-3 align-middle">
                          <Badge
                            tone={row.State === "draft" ? "accent" : "success"}
                          >
                            {row.State === "draft" ? "Draft" : "Saved"}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 align-middle">
                          <Button
                            size="xs"
                            className="h-8 w-16 px-0"
                            aria-label={`Open purchase order ${poNumberLabel(row)}`}
                            disabled={!openable || opening}
                            title={
                              row.State === "draft"
                                ? "Open this draft"
                                : "Open this saved purchase order (read-only)"
                            }
                            onClick={() => openRow(row.ID)}
                          >
                            {opening ? "Opening…" : "Open"}
                          </Button>
                        </td>
                        {showAdmin ? (
                          <td className="py-3 pl-0 pr-4 align-middle">
                            <div className="border-l border-rule pl-4">
                              <RowMenu
                                label={poNumberLabel(row)}
                                onDelete={() => onDelete?.(row)}
                              />
                            </div>
                          </td>
                        ) : null}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <ul
              className={`flex flex-col gap-3 p-4 lg:hidden ${state.loading ? "opacity-60" : ""}`}
              aria-busy={state.loading || undefined}
              aria-label="Purchase orders"
            >
              {state.rows.map((row) => {
                const openable = canOpenRow(row);
                const opening = openingId === row.ID;
                return (
                  <li key={row.ID}>
                    <article
                      aria-label={`Purchase order ${poNumberLabel(row)}`}
                      className="rounded-lg border border-rule bg-white p-3 transition-colors duration-150 active:bg-paper-subtle/60"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                          <span
                            className={`tnum text-body font-medium ${
                              row.PONumber.trim() === ""
                                ? "text-ink-400"
                                : "text-ink-900"
                            }`}
                          >
                            {poNumberLabel(row)}
                          </span>
                          <span className="text-micro uppercase text-ink-400">
                            {row.Currency}
                          </span>
                          <Badge
                            tone={row.State === "draft" ? "accent" : "success"}
                          >
                            {row.State === "draft" ? "Draft" : "Saved"}
                          </Badge>
                        </div>
                        {showAdmin ? (
                          <span className="shrink-0">
                            <RowMenu
                              label={poNumberLabel(row)}
                              onDelete={() => onDelete?.(row)}
                            />
                          </span>
                        ) : null}
                      </div>
                      <p className="mt-1.5 text-body text-ink-700">
                        {customerCellLabel(row)}
                      </p>
                      <p className="mt-0.5 text-small text-ink-700">
                        <span className="tnum">{quotationCellLabel(row)}</span>
                        <span className="text-ink-400">
                          {" "}
                          · {sourceRevisionLabel(row)}
                        </span>
                      </p>
                      {row.SourceMissing ? (
                        <p className="mt-1">
                          <SourceMissingBadge />
                        </p>
                      ) : null}
                      <p className="mt-1 text-caption text-ink-500">
                        Received {formatReceivedDate(row.ReceivedDate)}
                      </p>
                      <Button
                        variant="secondary"
                        aria-label={`Open purchase order ${poNumberLabel(row)}`}
                        disabled={!openable || opening}
                        title={
                          row.State === "draft"
                            ? "Open this draft"
                            : "Open this saved purchase order (read-only)"
                        }
                        onClick={() => openRow(row.ID)}
                        className="mt-3 min-h-11 w-full"
                      >
                        {opening ? "Opening…" : "Open"}
                      </Button>
                    </article>
                  </li>
                );
              })}
            </ul>
          </>
        )}
      </Card>

      {state.rows.length > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-caption text-ink-500">
            {state.total} purchase{" "}
            {state.total === 1 ? "order" : "orders"} · page {state.page} of{" "}
            {state.totalPages}
          </p>
          <div className="flex items-center gap-2 [&_button]:max-lg:min-h-11">
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
  pageSize,
}: PurchaseOrderListPageProps) {
  const [state, dispatch] = useReducer(poListReducer, undefined, () =>
    mode === undefined
      ? initialPOListState
      : initialPOListStateFor(mode, pageSize)
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
    const { query, error } = buildPOListQuery(
      state.filters,
      state.page,
      state.pageSize
    );
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
          dispatch({
            type: "queryFailed",
            seq,
            error: errorMessage(cause, "The purchase orders could not be loaded. Try again."),
          });
        });
    }, delay);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [state.filters, state.page, state.pageSize, reload]);

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
