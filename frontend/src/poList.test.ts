import { describe, expect, it } from "vitest";
import {
  PO_LIST_PAGE_SIZE,
  PO_LIST_SEARCH_PAUSE_MS,
  activeFilterCount,
  buildPOListQuery,
  canOpenRow,
  initialPOListFilters,
  initialPOListState,
  poListQueryDelay,
  poListReducer,
  poNumberLabel,
  resetPOListFilters,
  toggleCustomerFilter,
  type POListPageDTO,
  type POListRowDTO,
  type POListState,
} from "./poList";

// Issue #469: the PO archive's decisions, separated from its markup. The list
// is server-paged and server-filtered, so these tests pin the request the page
// sends and the state transitions that make a stale answer harmless -- the two
// places a list can lie about what the server returned.

function row(overrides: Partial<POListRowDTO> = {}): POListRowDTO {
  return {
    UUID: "uuid-1",
    State: "draft",
    PONumber: "PO-1",
    QuoteNumber: "1502260001",
    CustomerName: "PT. Alpha",
    ReceivedDate: "2026-02-10",
    Currency: "IDR",
    ID: 1,
    CustomerID: 2,
    SourceRevision: 0,
    SourceMissing: false,
    ...overrides,
  };
}

function page(overrides: Partial<POListPageDTO> = {}): POListPageDTO {
  return {
    PurchaseOrders: [row()],
    Customers: [{ ID: 2, Name: "PT. Alpha" }],
    Page: 1,
    PageSize: PO_LIST_PAGE_SIZE,
    Total: 1,
    TotalPages: 1,
    ...overrides,
  };
}

describe("buildPOListQuery (issue #469)", () => {
  it("builds the unfiltered first page", () => {
    const { query, error } = buildPOListQuery(initialPOListFilters, 1);
    expect(error).toBeNull();
    expect(query).toEqual({
      Search: "",
      State: "",
      CustomerIDs: [],
      ReceivedFrom: "",
      ReceivedTo: "",
      SourceMissing: "any",
      Page: 1,
    });
  });

  it("carries every filter and the page the caller asked for", () => {
    const { query, error } = buildPOListQuery(
      {
        search: "  infineon  ",
        customerIds: [2, 4],
        receivedFrom: "2026-01-01",
        receivedTo: "2026-12-31",
        state: "saved",
        sourceMissing: "only",
      },
      3
    );
    expect(error).toBeNull();
    expect(query).toEqual({
      Search: "infineon",
      State: "saved",
      CustomerIDs: [2, 4],
      ReceivedFrom: "2026-01-01",
      ReceivedTo: "2026-12-31",
      SourceMissing: "only",
      Page: 3,
    });
  });

  it("refuses a received range that starts after it ends", () => {
    const { query, error } = buildPOListQuery(
      { ...initialPOListFilters, receivedFrom: "2026-02-11", receivedTo: "2026-02-10" },
      1
    );
    expect(query).toBeNull();
    expect(error).toBe("Received from must be on or before received to.");
  });

  it("refuses an impossible date or page instead of sending it", () => {
    const badDate = buildPOListQuery(
      { ...initialPOListFilters, receivedFrom: "2026-02-30" },
      1
    );
    expect(badDate.query).toBeNull();
    expect(badDate.error).toContain("YYYY-MM-DD");

    const badPage = buildPOListQuery(initialPOListFilters, 0);
    expect(badPage.query).toBeNull();
    expect(badPage.error).toContain("Page");
  });
});

describe("filter helpers (issue #469)", () => {
  it("counts each active filter dimension once", () => {
    expect(activeFilterCount(initialPOListFilters)).toBe(0);
    expect(activeFilterCount({ ...initialPOListFilters, search: "abc" })).toBe(1);
    expect(
      activeFilterCount({
        ...initialPOListFilters,
        search: "abc",
        customerIds: [2, 4],
        state: "draft",
      })
    ).toBe(3);
  });

  it("toggles a customer in and out without mutating the filter", () => {
    const original = { ...initialPOListFilters };
    const added = toggleCustomerFilter(original, 2);
    expect(added.customerIds).toEqual([2]);
    expect(original.customerIds).toEqual([]);

    const both = toggleCustomerFilter(added, 4);
    expect(both.customerIds).toEqual([2, 4]);

    const removed = toggleCustomerFilter(both, 2);
    expect(removed.customerIds).toEqual([4]);
  });

  it("resets every dimension to a fresh initial value", () => {
    const dirty = {
      search: "abc",
      customerIds: [2],
      receivedFrom: "2026-01-01",
      receivedTo: "2026-12-31",
      state: "saved" as const,
      sourceMissing: "only" as const,
    };
    const reset = resetPOListFilters();
    expect(reset).toEqual(initialPOListFilters);
    expect(reset).not.toBe(initialPOListFilters);
    expect(reset.customerIds).not.toBe(initialPOListFilters.customerIds);
    expect(reset).not.toEqual(dirty);
  });

  it("pauses only a changed, non-empty search", () => {
    expect(poListQueryDelay("", "")).toBe(0);
    expect(poListQueryDelay("", "a")).toBe(PO_LIST_SEARCH_PAUSE_MS);
    expect(poListQueryDelay("a", "ab")).toBe(PO_LIST_SEARCH_PAUSE_MS);
    expect(poListQueryDelay("ab", "")).toBe(0);
    expect(poListQueryDelay("ab", "ab")).toBe(0);
  });
});

describe("poListReducer (issue #469)", () => {
  it("starts with one empty page and no loading work", () => {
    expect(initialPOListState.page).toBe(1);
    expect(initialPOListState.pageSize).toBe(PO_LIST_PAGE_SIZE);
    expect(initialPOListState.totalPages).toBe(1);
    expect(initialPOListState.rows).toEqual([]);
    expect(initialPOListState.loading).toBe(false);
    expect(initialPOListState.error).toBeNull();
  });

  it("resets the page whenever a filter changes", () => {
    const onPageThree: POListState = { ...initialPOListState, page: 3 };
    const next = poListReducer(onPageThree, {
      type: "filtersChanged",
      patch: { state: "saved" },
    });
    expect(next.page).toBe(1);
    expect(next.filters.state).toBe("saved");
    expect(next.filters.search).toBe("");
  });

  it("clamps paging to the pages the server reported", () => {
    const loaded = poListReducer(
      { ...initialPOListState, seq: 1 },
      { type: "queryLoaded", seq: 1, page: page({ TotalPages: 3 }) }
    );
    expect(poListReducer(loaded, { type: "pageChanged", page: 2 }).page).toBe(2);
    expect(poListReducer(loaded, { type: "pageChanged", page: 9 }).page).toBe(3);
    expect(poListReducer(loaded, { type: "pageChanged", page: 0 }).page).toBe(1);
    // Asking for the page already shown must not trigger a request.
    expect(poListReducer(loaded, { type: "pageChanged", page: 1 })).toBe(loaded);
  });

  it("clears filters and returns to page one on reset", () => {
    const dirty: POListState = {
      ...initialPOListState,
      page: 4,
      filters: { ...initialPOListFilters, search: "x", customerIds: [2], state: "saved" },
    };
    const next = poListReducer(dirty, { type: "resetFilters" });
    expect(next.page).toBe(1);
    expect(next.filters).toEqual(initialPOListFilters);
  });

  it("ignores a stale response after a newer request started", () => {
    const loading = poListReducer(initialPOListState, { type: "queryStarted", seq: 1 });
    const first = poListReducer(loading, { type: "queryLoaded", seq: 1, page: page() });
    expect(first.rows).toHaveLength(1);

    // A newer query starts (say, the user typed), then the older request's
    // answer lands late. It must not replace the newer result.
    const newer = poListReducer(first, { type: "queryStarted", seq: 2 });
    const fresh = poListReducer(newer, {
      type: "queryLoaded",
      seq: 2,
      page: page({
        PurchaseOrders: [row({ ID: 9, PONumber: "PO-NEW", CustomerName: "PT. New" })],
        Total: 1,
      }),
    });
    const stale = poListReducer(fresh, { type: "queryLoaded", seq: 1, page: page() });
    expect(stale.rows).toHaveLength(1);
    expect(stale.rows[0].ID).toBe(9);
    expect(stale.rows[0].PONumber).toBe("PO-NEW");
  });

  it("ignores a stale failure and keeps the newer rows", () => {
    const fresh = poListReducer(
      poListReducer(initialPOListState, { type: "queryStarted", seq: 2 }),
      { type: "queryLoaded", seq: 2, page: page() }
    );
    const staleFailure = poListReducer(fresh, {
      type: "queryFailed",
      seq: 1,
      error: "old failure",
    });
    expect(staleFailure.rows).toHaveLength(1);
    expect(staleFailure.error).toBeNull();
  });

  it("shows the latest failure and clears the stale rows", () => {
    const started = poListReducer(
      poListReducer(initialPOListState, { type: "queryStarted", seq: 1 }),
      { type: "queryLoaded", seq: 1, page: page() }
    );
    const failed = poListReducer(started, {
      type: "queryStarted",
      seq: 2,
    });
    const error = poListReducer(failed, {
      type: "queryFailed",
      seq: 2,
      error: "The archive could not be loaded.",
    });
    expect(error.loading).toBe(false);
    expect(error.error).toBe("The archive could not be loaded.");
    expect(error.rows).toEqual([]);
  });

  it("marks the newest request as the one loading", () => {
    const started = poListReducer(initialPOListState, { type: "queryStarted", seq: 7 });
    expect(started.loading).toBe(true);
    expect(started.seq).toBe(7);
    expect(started.error).toBeNull();
  });
});

describe("row decisions (issue #469)", () => {
  it("labels an absent customer PO number honestly", () => {
    expect(poNumberLabel(row({ PONumber: "" }))).toBe("Not entered");
    expect(poNumberLabel(row({ PONumber: "PO-9" }))).toBe("PO-9");
  });

  it("opens drafts and leaves saved records to the later saved view", () => {
    // #471 has not shipped a saved-detail route, so opening a saved row in the
    // draft editor would mislabel it. The list states the limitation instead.
    expect(canOpenRow(row({ State: "draft" }))).toBe(true);
    expect(canOpenRow(row({ State: "saved" }))).toBe(false);
  });
});
