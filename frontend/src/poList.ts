// Issue #469: the Purchase Order archive's decisions, separated from markup.
//
// The list is filtered and paged by the server: the page owns which filters and
// which page are current, and everything here is the pure answer to "what
// request does that mean?" plus the reducer that keeps a late answer from
// overwriting a newer one. No totals, fulfilment or payment status exists --
// the stored entry state is draft or saved, and nothing else is invented.

/** The server's fixed page size when no size is requested (issue #469). */
export const PO_LIST_PAGE_SIZE = 15;

/**
 * The list API's page-size bounds (issue #492). The backend accepts an
 * optional PageSize of 1..100; 0 or absent keeps the 15-row default. The
 * settings page uses the same range, so a saved preference is always
 * requestable.
 */
export const PO_LIST_MIN_PAGE_SIZE = 1;
export const PO_LIST_MAX_PAGE_SIZE = 100;

/** Typing pause for the search box, matching the source selector. */
export const PO_LIST_SEARCH_PAUSE_MS = 250;

/** One archive row, as the desktop binding returns it (PascalCase = Wails). */
export interface POListRowDTO {
  UUID: string;
  State: string;
  PONumber: string;
  QuoteNumber: string;
  CustomerName: string;
  ReceivedDate: string;
  Currency: string;
  ID: number;
  CustomerID: number;
  SourceRevision: number;
  /**
   * Independent of Draft/Saved. True when the followed Quotation was
   * permanently deleted (#480) or the draft was never linked (#496).
   * QuoteNumber/SourceRevision then name the former source, if any.
   */
  SourceMissing: boolean;
  /** #496: "live", "never" or "former". Optional so an older payload and older
   * test fixtures still type-check; `sourceStateOfRow` derives it. */
  SourceState?: string;
}

/** One option of the customer multi-select. */
export interface POListCustomerDTO {
  Name: string;
  ID: number;
}

/**
 * The row's live source state. An older payload omitted SourceState, so it is
 * derived from the fields it did send: a linked row has a QuotationID, and a
 * formerly-linked one carries SourceMissing. A never-linked row has neither.
 */
export function sourceStateOfRow(
  row: Pick<POListRowDTO, "SourceState" | "SourceMissing" | "CustomerID" | "QuoteNumber">
): "live" | "never" | "former" {
  if (row.SourceState === "live" || row.SourceState === "never" || row.SourceState === "former") {
    return row.SourceState;
  }
  if (row.SourceMissing === true) return "former";
  return "live";
}

/**
 * The quotation column's second line. A never-linked row must not be described
 * as a "former revision 0": it has no source at all, while a linked revision 00
 * is real and renders "Revision 0".
 */
export function sourceRevisionLabel(row: POListRowDTO): string {
  const state = sourceStateOfRow(row);
  if (state === "never") return "No quotation linked";
  if (state === "former") return `Former quotation · revision ${row.SourceRevision}`;
  return `Revision ${row.SourceRevision}`;
}

/** The quotation number cell. A never-linked row has no number yet. */
export function quotationCellLabel(row: POListRowDTO): string {
  if (sourceStateOfRow(row) === "never") return "—";
  return row.QuoteNumber;
}

/** The customer cell. A never-linked row has no Customer identity yet. */
export function customerCellLabel(row: POListRowDTO): string {
  if (row.CustomerName.trim() !== "") return row.CustomerName;
  return sourceStateOfRow(row) === "never" ? "—" : "";
}

/** One page plus the org-wide customer facet. */
export interface POListPageDTO {
  PurchaseOrders: POListRowDTO[];
  Customers: POListCustomerDTO[];
  Page: number;
  PageSize: number;
  Total: number;
  TotalPages: number;
}

/** The request shape the Wails binding consumes. */
export interface POListQuery {
  Search: string;
  State: string;
  CustomerIDs: number[];
  ReceivedFrom: string;
  ReceivedTo: string;
  SourceMissing: string;
  Page: number;
  /** Issue #492: the requested rows per page (1..100). */
  PageSize: number;
}

export type POListStateFilter = "" | "draft" | "saved";

/**
 * Source missing is an independent predicate, not a third state. "any" is the
 * default; "only" shows the records whose Quotation was deleted and "exclude"
 * hides them. The archive's Source control selects one of the three and the
 * server filters on the same value.
 */
export type POListSourceMissingFilter = "any" | "only" | "exclude";

export interface POListFilters {
  search: string;
  customerIds: number[];
  receivedFrom: string;
  receivedTo: string;
  state: POListStateFilter;
  sourceMissing: POListSourceMissingFilter;
}

export interface POListState {
  filters: POListFilters;
  page: number;
  rows: POListRowDTO[];
  customers: POListCustomerDTO[];
  pageSize: number;
  total: number;
  totalPages: number;
  loading: boolean;
  error: string | null;
  /** The sequence of the newest request; older answers are ignored. */
  seq: number;
}

export type POListAction =
  | { type: "filtersChanged"; patch: Partial<POListFilters> }
  | { type: "pageChanged"; page: number }
  | { type: "resetFilters"; state?: POListStateFilter }
  | { type: "queryStarted"; seq: number }
  | { type: "queryLoaded"; seq: number; page: POListPageDTO }
  | { type: "queryFailed"; seq: number; error: string };

/**
 * The starting filters for one PO workspace tab. The Drafts and Archive tabs
 * pin the state the tab is about; the State control is then not offered, and
 * Reset returns to that pinned state rather than to every-record.
 */
export function resetPOListFilters(state: POListStateFilter = ""): POListFilters {
  return {
    search: "",
    customerIds: [],
    receivedFrom: "",
    receivedTo: "",
    state,
    sourceMissing: "any",
  };
}

export const initialPOListFilters: POListFilters = resetPOListFilters();

export const initialPOListState: POListState = {
  filters: initialPOListFilters,
  page: 1,
  rows: [],
  customers: [],
  pageSize: PO_LIST_PAGE_SIZE,
  total: 0,
  totalPages: 1,
  loading: false,
  error: null,
  seq: 0,
};

export type POListMode = "drafts" | "archive";

/** The pinned state a workspace tab asks the server for. */
export function modeState(mode: POListMode): POListStateFilter {
  return mode === "drafts" ? "draft" : "saved";
}

/** A fresh list state pinned to one workspace tab's stored state. */
export function initialPOListStateFor(
  mode: POListMode,
  pageSize: number = PO_LIST_PAGE_SIZE
): POListState {
  return {
    ...initialPOListState,
    // Issue #492: the account's remembered size seeds the initial state; every
    // request carries it, and the server's effective PageSize replaces it once
    // a page loads.
    pageSize:
      Number.isInteger(pageSize) && pageSize >= 1
        ? pageSize
        : PO_LIST_PAGE_SIZE,
    filters: resetPOListFilters(modeState(mode)),
  };
}

/** True for a blank field or a real YYYY-MM-DD calendar date. */
function validFilterDate(raw: string): boolean {
  const text = raw.trim();
  if (text === "") return true;
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(text);
  if (!match) return false;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month - 1, day));
  return (
    date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day
  );
}

/**
 * Turns the visible filters into the exact binding query, or the reason they
 * cannot be sent. An invalid range is refused rather than interpreted as an
 * empty result: an empty page must never mean "the filter was wrong".
 */
export function buildPOListQuery(
  filters: POListFilters,
  page: number,
  pageSize: number
): { query: POListQuery | null; error: string | null } {
  if (!Number.isInteger(page) || page < 1) {
    return { query: null, error: "Page must be a positive whole number." };
  }
  if (
    !Number.isInteger(pageSize) ||
    pageSize < PO_LIST_MIN_PAGE_SIZE ||
    pageSize > PO_LIST_MAX_PAGE_SIZE
  ) {
    return {
      query: null,
      error: `Page size must be between ${PO_LIST_MIN_PAGE_SIZE} and ${PO_LIST_MAX_PAGE_SIZE}.`,
    };
  }
  if (filters.state !== "" && filters.state !== "draft" && filters.state !== "saved") {
    return { query: null, error: "State must be draft or saved." };
  }
  if (
    filters.sourceMissing !== "any" &&
    filters.sourceMissing !== "only" &&
    filters.sourceMissing !== "exclude"
  ) {
    return { query: null, error: "Source missing must be any, only or exclude." };
  }
  if (!validFilterDate(filters.receivedFrom)) {
    return { query: null, error: "Received from must be a YYYY-MM-DD date." };
  }
  if (!validFilterDate(filters.receivedTo)) {
    return { query: null, error: "Received to must be a YYYY-MM-DD date." };
  }
  const from = filters.receivedFrom.trim();
  const to = filters.receivedTo.trim();
  if (from !== "" && to !== "" && from > to) {
    return {
      query: null,
      error: "Received from must be on or before received to.",
    };
  }
  if (!filters.customerIds.every((id) => Number.isInteger(id) && id > 0)) {
    return { query: null, error: "Customer selection must be a list of customer ids." };
  }
  return {
    query: {
      Search: filters.search.trim(),
      State: filters.state,
      CustomerIDs: [...filters.customerIds],
      ReceivedFrom: from,
      ReceivedTo: to,
      SourceMissing: filters.sourceMissing,
      Page: page,
      PageSize: pageSize,
    },
    error: null,
  };
}

/** Counts active dimensions, for the Reset control and its label. */
export function activeFilterCount(filters: POListFilters): number {
  let count = 0;
  if (filters.search.trim() !== "") count += 1;
  if (filters.customerIds.length > 0) count += 1;
  if (filters.receivedFrom.trim() !== "") count += 1;
  if (filters.receivedTo.trim() !== "") count += 1;
  if (filters.state !== "") count += 1;
  if (filters.sourceMissing !== "any") count += 1;
  return count;
}

export function hasActiveFilters(filters: POListFilters): boolean {
  return activeFilterCount(filters) > 0;
}

/** Adds or removes one customer id, preserving the rest of the selection. */
export function toggleCustomerFilter(
  filters: POListFilters,
  customerId: number
): POListFilters {
  const selected = filters.customerIds.includes(customerId);
  return {
    ...filters,
    customerIds: selected
      ? filters.customerIds.filter((id) => id !== customerId)
      : [...filters.customerIds, customerId],
  };
}

/**
 * How long the page waits before querying. Only a changed, non-empty search is
 * paused; paging, every other filter, the first render and clearing the box
 * are explicit actions and query immediately. `previousSearch` is the term
 * behind the last request.
 */
export function poListQueryDelay(previousSearch: string, search: string): number {
  if (previousSearch === search) return 0;
  return search.trim() === "" ? 0 : PO_LIST_SEARCH_PAUSE_MS;
}

/** The customer PO number cell's honest empty value. */
export function poNumberLabel(row: POListRowDTO): string {
  return row.PONumber.trim() === "" ? "Not entered" : row.PONumber;
}

/**
 * Whether a row can open its record. Drafts open the entry editor; saved
 * records open the read-only saved view #471 added, so the saved PO is visible
 * while editing stays gated until the correction workflow (#473).
 */
export function canOpenRow(row: POListRowDTO): boolean {
  return row.State === "draft" || row.State === "saved";
}

export function poListReducer(state: POListState, action: POListAction): POListState {
  switch (action.type) {
    case "filtersChanged":
      return {
        ...state,
        filters: {
          ...state.filters,
          ...action.patch,
          customerIds: action.patch.customerIds
            ? [...action.patch.customerIds]
            : state.filters.customerIds,
        },
        page: 1,
      };
    case "pageChanged": {
      const page = Math.min(Math.max(action.page, 1), Math.max(state.totalPages, 1));
      if (page === state.page) return state;
      return { ...state, page };
    }
    case "resetFilters":
      return {
        ...state,
        filters: resetPOListFilters(action.state ?? ""),
        page: 1,
        error: null,
      };
    case "queryStarted":
      return { ...state, seq: action.seq, loading: true, error: null };
    case "queryLoaded": {
      if (action.seq !== state.seq) return state;
      const rows = Array.isArray(action.page.PurchaseOrders)
        ? action.page.PurchaseOrders
        : [];
      const customers = Array.isArray(action.page.Customers)
        ? action.page.Customers
        : [];
      return {
        ...state,
        rows,
        customers,
        page: action.page.Page >= 1 ? action.page.Page : state.page,
        pageSize:
          action.page.PageSize >= 1 ? action.page.PageSize : PO_LIST_PAGE_SIZE,
        total: Math.max(action.page.Total, 0),
        totalPages: Math.max(action.page.TotalPages, 1),
        loading: false,
        error: null,
      };
    }
    case "queryFailed":
      if (action.seq !== state.seq) return state;
      return { ...state, rows: [], loading: false, error: action.error };
    default:
      return state;
  }
}
