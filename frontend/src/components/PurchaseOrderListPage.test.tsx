import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  activeFilterCount,
  initialPOListFilters,
  initialPOListState,
  resetPOListFilters,
  type POListPageDTO,
  type POListRowDTO,
  type POListState,
} from "../poList";
import {
  PurchaseOrderListView,
  type PurchaseOrderListViewProps,
} from "./PurchaseOrderListPage";

// Issue #469: the archive's visible states. The suite runs on node with no DOM
// (see vitest.config.ts), so the view is rendered to a string and asserted on
// as markup -- the house pattern. It proves which state the page can show and
// which controls are disabled; it does not prove clicks, focus order or that
// the assembled page reads well at 360/1280 (the ui-snapshot harness is that
// evidence), and it never renders the container's fetch effect.

function row(overrides: Partial<POListRowDTO> = {}): POListRowDTO {
  return {
    UUID: "uuid-1",
    State: "draft",
    PONumber: "PO-INF-2026-091",
    QuoteNumber: "1502260012",
    CustomerName: "PT. Infineon Technologies Batam",
    ReceivedDate: "2026-10-02",
    Currency: "USD",
    ID: 7,
    CustomerID: 2,
    SourceRevision: 2,
    SourceMissing: false,
    ...overrides,
  };
}

function state(overrides: Partial<POListState> = {}): POListState {
  return { ...initialPOListState, ...overrides };
}

function render(overrides: Partial<PurchaseOrderListViewProps> = {}): string {
  const props: PurchaseOrderListViewProps = {
    state: state(),
    dispatch: () => undefined,
    onOpen: () => undefined,
    onAdd: () => undefined,
    onRetry: () => undefined,
    ...overrides,
  };
  // React SSR separates adjacent text nodes with comment markers. The
  // assertions read the rendered text, so drop only those markers.
  return renderToString(<PurchaseOrderListView {...props} />).replace(
    /<!-- -->/g,
    ""
  );
}

/** The opening tag of the first button carrying the accessible label. */
function buttonTagFor(html: string, label: string): string {
  const button = html
    .split("<button")
    .slice(1)
    .find((chunk) =>
      chunk.slice(0, chunk.indexOf(">")).includes(`aria-label="${label}"`)
    );
  if (!button) throw new Error(`no <button aria-label="${label}"> in: ${html}`);
  return button.slice(0, button.indexOf(">") + 1);
}

describe("PurchaseOrderListView states (issue #469)", () => {
  it("renders the initial loading state as a skeleton", () => {
    const html = render({ state: state({ loading: true }) });
    expect(html).toContain('aria-label="Loading purchase orders"');
    expect(html).not.toContain("<table");
  });

  it("keeps the current page visible while a refresh is in flight", () => {
    // Typing or paging must not blank the list for 250ms plus a round trip;
    // the pager is disabled and the old rows stay readable until the answer
    // replaces them (the stale-response rule is what makes that safe).
    const html = render({
      state: state({ loading: true, rows: [row()], total: 1, totalPages: 1 }),
    });
    expect(html).not.toContain('aria-label="Loading purchase orders"');
    expect(html).toContain("PO-INF-2026-091");
    expect(buttonTagFor(html, "Next page")).toContain(" disabled=");
  });

  it("renders an error state with a retry action", () => {
    const html = render({
      state: state({
        loading: false,
        error: "We couldn't load the purchase orders.",
      }),
    });
    expect(html).toContain('role="alert"');
    expect(html).toContain("We couldn&#x27;t load the purchase orders.");
    expect(html).toContain("Try again");
    expect(html).not.toContain("<table");
  });

  it("shows the unfiltered empty state with the real entry point", () => {
    const html = render();
    expect(html).toContain("No purchase orders yet");
    expect(html).toContain("Start one from an issued quotation revision.");
    const add = buttonTagFor(html, "Add Purchase Order from a Quotation");
    expect(add).not.toContain(" disabled=");
    expect(html).not.toContain("<table");
  });

  it("shows a filtered empty state with an enabled reset", () => {
    const html = render({
      state: state({
        filters: { ...initialPOListFilters, search: "zzz" },
      }),
    });
    expect(html).toContain("No purchase order matches these filters");
    expect(html).toContain("Clear the filters to see the whole archive.");
    const reset = buttonTagFor(html, "Reset filters");
    expect(reset).not.toContain(" disabled=");
  });

  it("renders rows with the three searchable fields and the stored state", () => {
    const html = render({
      state: state({
        rows: [
          row(),
          row({
            ID: 8,
            State: "saved",
            PONumber: "",
            QuoteNumber: "1503260004",
            CustomerName: "PT. H-Tech Oilfield Equipment, NOV XL Systems",
            ReceivedDate: "2026-09-30",
            Currency: "IDR",
            SourceRevision: 0,
          }),
        ],
        customers: [
          { ID: 2, Name: "PT. Infineon Technologies Batam" },
          {
            ID: 3,
            Name: "PT. H-Tech Oilfield Equipment, NOV XL Systems",
          },
        ],
        total: 2,
        totalPages: 1,
      }),
    });
    expect(html).toContain("<table");
    expect(html).toContain("PO-INF-2026-091");
    // An absent customer PO number says so rather than rendering a zero or a
    // blank cell that reads as a rendering bug.
    expect(html).toContain("Not entered");
    expect(html).toContain("1502260012");
    expect(html).toContain("1503260004");
    expect(html).toContain("PT. H-Tech Oilfield Equipment, NOV XL Systems");
    expect(html).toContain("2026-10-02");
    expect(html).toContain(">Draft<");
    expect(html).toContain(">Saved<");
    expect(html).toContain("2 purchase orders");
  });

  it("opens drafts into the editor and saved records into the read-only view", () => {
    const html = render({
      state: state({
        rows: [row(), row({ ID: 8, State: "saved", PONumber: "PO-SAVED" })],
        total: 2,
        totalPages: 1,
      }),
    });
    const draft = buttonTagFor(html, "Open purchase order PO-INF-2026-091");
    expect(draft).not.toContain(" disabled=");
    expect(draft).toContain("Open this draft");

    // #471 enabled saved rows: the record opens, and the title says the view is
    // read-only rather than pretending the row is unavailable.
    const saved = buttonTagFor(html, "Open purchase order PO-SAVED");
    expect(saved).not.toContain(" disabled=");
    expect(saved).toContain("Open this saved purchase order (read-only)");
  });

  it("renders a Source missing badge only when the row carries the flag", () => {
    const present = render({
      state: state({ rows: [row()], total: 1, totalPages: 1 }),
    });
    expect(present).not.toContain(">Source missing<");

    const missing = render({
      state: state({
        rows: [row({ SourceMissing: true })],
        total: 1,
        totalPages: 1,
      }),
    });
    expect(missing).toContain(">Source missing<");
  });

  it("offers the Source missing filter as an independent, enabled predicate", () => {
    const html = render({
      state: state({
        rows: [row()],
        customers: [{ ID: 2, Name: "PT. Infineon Technologies Batam" }],
        total: 1,
        totalPages: 1,
      }),
    });
    const trigger = buttonTagFor(html, "Source");
    expect(trigger).not.toContain(" disabled=");
    expect(html).toContain("Any source");
    expect(html).toContain("Source missing is independent of Draft/Saved.");
    // #481 activates the filter; the #480 arrival note must be gone.
    expect(html).not.toContain("Source-missing filtering arrives");
    expect(html).not.toContain("No purchase order can be missing its source today.");
  });

  it("renders each source-missing filter mode and counts it", () => {
    const only = render({
      state: state({
        filters: { ...initialPOListFilters, sourceMissing: "only" },
        rows: [row({ SourceMissing: true })],
        total: 1,
        totalPages: 1,
      }),
    });
    expect(only).toContain("Source missing only");
    expect(only).toContain("1 filter active");
    expect(only).toContain("Former quotation");
    expect(only).toContain(">Source missing<");

    const excluded = render({
      state: state({
        filters: { ...initialPOListFilters, sourceMissing: "exclude" },
        rows: [row()],
        total: 1,
        totalPages: 1,
      }),
    });
    expect(excluded).toContain("Source present only");
    expect(excluded).toContain("1 filter active");
    expect(excluded).not.toContain("Former quotation");
  });

  it("resets the Source missing filter with the rest of the filters", () => {
    // The reset action is the reducer's; the view must count the dimension so
    // Reset is enabled and the label names it.
    const missing = { ...initialPOListFilters, sourceMissing: "only" as const };
    expect(activeFilterCount(missing)).toBe(1);
    expect(resetPOListFilters().sourceMissing).toBe("any");
  });

  it("offers no delete control to a non-admin view", () => {
    const html = render({
      state: state({
        rows: [row()],
        total: 1,
        totalPages: 1,
      }),
    });
    expect(html).not.toContain("Delete purchase order");
  });

  it("offers the admin delete entry point without changing the row's Open control", () => {
    const html = render({
      state: state({
        rows: [row()],
        total: 1,
        totalPages: 1,
      }),
      onDelete: () => undefined,
    });
    expect(html).toContain('aria-label="Delete purchase order PO-INF-2026-091"');
    expect(html).toContain('aria-label="Open purchase order PO-INF-2026-091"');
  });

  it("lists the customer facet and marks the selected customers", () => {
    const html = render({
      state: state({
        filters: { ...initialPOListFilters, customerIds: [2] },
        customers: [
          { ID: 2, Name: "PT. Infineon Technologies Batam" },
          { ID: 3, Name: "PT. H-Tech Oilfield Equipment, NOV XL Systems" },
        ],
      }),
    });
    const alpha = html
      .split("<input")
      .find((chunk) => chunk.includes('aria-label="Customer PT. Infineon'));
    expect(alpha).toBeDefined();
    expect(alpha?.slice(0, alpha.indexOf(">"))).toContain(" checked=");
    const beta = html
      .split("<input")
      .find((chunk) => chunk.includes('aria-label="Customer PT. H-Tech'));
    expect(beta?.slice(0, beta.indexOf(">"))).not.toContain(" checked=");
    expect(html).toContain("1 filter active");
  });

  it("disables Reset when no filter is active and enables it when one is", () => {
    const clean = render();
    expect(buttonTagFor(clean, "Reset filters")).toContain(" disabled=");
    expect(clean).toContain("No filters");

    const dirty = render({
      state: state({
        filters: { ...initialPOListFilters, state: "saved" },
      }),
    });
    expect(buttonTagFor(dirty, "Reset filters")).not.toContain(" disabled=");
    expect(dirty).toContain("1 filter active");
  });

  it("pages on the server with a 15-row contract and disabled edges", () => {
    const first = render({
      state: state({ rows: [row()], total: 31, totalPages: 3, page: 1 }),
    });
    expect(first).toContain("31 purchase orders");
    expect(first).toContain("page 1 of 3");
    expect(buttonTagFor(first, "Previous page")).toContain(" disabled=");
    expect(buttonTagFor(first, "Next page")).not.toContain(" disabled=");

    const last = render({
      state: state({ rows: [row()], total: 31, totalPages: 3, page: 3 }),
    });
    expect(buttonTagFor(last, "Previous page")).not.toContain(" disabled=");
    expect(buttonTagFor(last, "Next page")).toContain(" disabled=");
  });

  it("shows the selected state in the filter control", () => {
    const html = render({
      state: state({
        filters: { ...initialPOListFilters, state: "saved" },
      }),
    });
    const trigger = buttonTagFor(html, "State");
    expect(trigger).toContain('aria-haspopup="listbox"');
    expect(html).toContain("Saved");
  });
});

describe("PurchaseOrderListView fixture sanity (issue #469)", () => {
  it("renders the same page shape the API returns", () => {
    // The DTO type is the binding's shape; this assignment fails the build if
    // a field is renamed on either side.
    const page: POListPageDTO = {
      PurchaseOrders: [row()],
      Customers: [{ ID: 2, Name: "PT. Infineon Technologies Batam" }],
      Page: 1,
      PageSize: 15,
      Total: 1,
      TotalPages: 1,
    };
    expect(page.PurchaseOrders[0].SourceMissing).toBe(false);
  });
});
