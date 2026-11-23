// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type {
  PurchaseOrderFulfillmentItem,
  PurchaseOrderFulfillmentPage,
  PurchaseOrderFulfillmentQuery,
} from "../poFulfillment";
import {
  PurchaseOrderFulfillmentCard,
  type PurchaseOrderFulfillmentCardProps,
} from "./PurchaseOrderFulfillmentCard";

// Issue #657 phase B: the Fulfillment drill-down card. These tests drive the
// real component with an injected loader, so they pin what staff see — the
// status badge, per-leaf quantities and variance figures, the aggregate title
// above its children (never a third line), extras listed separately, the
// status/search/page controls — and prove a PO mutation cannot leave stale
// figures on screen.

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const CATALOGUE = [IDR];

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  Element.prototype.scrollIntoView = () => undefined;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function item(
  overrides: Partial<PurchaseOrderFulfillmentItem> = {}
): PurchaseOrderFulfillmentItem {
  return {
    quotationItemNo: 1,
    quotationSubNo: null,
    label: "1",
    description: "WIP rack",
    groupTitle: "",
    removed: false,
    quotedQuantityScaled: 50000,
    boughtQuantityScaled: 30000,
    unsoldQuantityScaled: 20000,
    overBoughtQuantityScaled: 0,
    realizedAmountMinor: 60000,
    realizedMarginMinor: 30000,
    unsoldAmountMinor: 40000,
    unsoldMarginMinor: 20000,
    quantityExtraIncomeAmountMinor: 0,
    quantityExtraIncomeMarginMinor: 0,
    priceExtraIncomeMinor: 0,
    priceLossMinor: 0,
    closedLossAmountMinor: 0,
    closedLossMarginMinor: 0,
    purchaseOrders: [
      {
        id: 11,
        poNumber: "PO-657-1",
        state: "saved",
        receivedDate: "2026-03-10",
        allocatedQuantityScaled: 30000,
      },
    ],
    ...overrides,
  };
}

function page(
  overrides: Partial<PurchaseOrderFulfillmentPage> = {}
): PurchaseOrderFulfillmentPage {
  return {
    asOf: "2026-11-19",
    filter: {
      currency: "IDR",
      status: "all",
      search: "",
      page: 1,
      pageSize: 25,
    },
    quotations: [
      {
        quotationId: 7,
        quoteNumber: "1502260007",
        customerName: "PT. Infineon Technologies Batam",
        currency: "IDR",
        latestRevision: 1,
        issuedAt: "2026-10-10",
        validUntil: "2026-11-09",
        status: "closed",
        items: [item()],
        extraExpenses: [
          {
            purchaseOrderId: 11,
            poNumber: "PO-657-1",
            lineNumber: 2,
            description: "Freight surcharge",
            quantityScaled: 20000,
            uom: "Lot",
            amountMinor: 1000000,
            groupTitle: "",
          },
        ],
      },
    ],
    page: 1,
    pageSize: 25,
    total: 1,
    totalPages: 1,
    ...overrides,
  };
}

function props(
  overrides: Partial<PurchaseOrderFulfillmentCardProps> = {}
): PurchaseOrderFulfillmentCardProps {
  return {
    currency: "IDR",
    catalogue: CATALOGUE,
    load: vi.fn(async () => page()),
    ...overrides,
  };
}

async function render(
  overrides: Partial<PurchaseOrderFulfillmentCardProps> = {}
): Promise<PurchaseOrderFulfillmentCardProps> {
  const value = props(overrides);
  await act(async () => {
    root.render(<PurchaseOrderFulfillmentCard {...value} />);
  });
  await flush();
  return value;
}

async function rerender(
  value: PurchaseOrderFulfillmentCardProps
): Promise<void> {
  await act(async () => {
    root.render(<PurchaseOrderFulfillmentCard {...value} />);
  });
  await flush();
}

function card(): HTMLElement {
  const found = container.querySelector<HTMLElement>(
    "[data-purchase-order-fulfillment]"
  );
  if (found === null) throw new Error("no fulfillment card");
  return found;
}

function text(): string {
  return card().textContent ?? "";
}

function button(label: string): HTMLButtonElement {
  const found = buttonOrNull(label);
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function buttonOrNull(label: string): HTMLButtonElement | null {
  return container.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
}

describe("PurchaseOrderFulfillmentCard", () => {
  it("renders the status, quantities, figures and contributing saved POs", async () => {
    await render();

    expect(card().querySelector("[data-fulfillment-status]")?.textContent).toBe(
      "Closed"
    );
    expect(text()).toContain("1502260007");
    expect(text()).toContain("PT. Infineon Technologies Batam");
    expect(text()).toContain("No date filter");

    const row = card().querySelector<HTMLElement>("[data-fulfillment-item]");
    if (row === null) throw new Error("no item row");
    expect(
      row.querySelector('[data-fulfillment-quantity="quoted"]')?.textContent
    ).toBe("5");
    expect(
      row.querySelector('[data-fulfillment-quantity="bought"]')?.textContent
    ).toBe("3");
    expect(
      row.querySelector('[data-fulfillment-quantity="unsold"]')?.textContent
    ).toBe("2");
    expect(
      row.querySelector('[data-fulfillment-quantity="over-bought"]')
        ?.textContent
    ).toBe("0");

    const money = (name: string): string =>
      row.querySelector(`[data-fulfillment-money="${name}"]`)?.textContent ??
      "";
    expect(money("realized-amount")).toBe("Rp60.000");
    expect(money("realized-margin")).toBe("Rp30.000");
    expect(money("unsold-amount")).toBe("Rp40.000");
    expect(money("unsold-margin")).toBe("Rp20.000");
    expect(text()).toContain("PO-657-1");
  });

  it("lists extras separately (standalone and grouped) and never inside the item quantities", async () => {
    const extraPage = page({
      quotations: [
        {
          ...page().quotations[0],
          extraExpenses: [
            ...page().quotations[0].extraExpenses,
            {
              purchaseOrderId: 12,
              poNumber: "PO-657-2",
              lineNumber: 3,
              description: "Grouped surcharge",
              quantityScaled: 10000,
              uom: "Lot",
              amountMinor: 250000,
              groupTitle: "Delivered package",
            },
          ],
        },
      ],
    });
    await render({ load: vi.fn(async () => extraPage) });

    const extras = card().querySelectorAll<HTMLElement>(
      "[data-fulfillment-extra]"
    );
    expect(extras).toHaveLength(2);
    expect(extras[0].textContent).toContain("Freight surcharge");
    expect(extras[0].textContent).toContain("1.000.000");
    expect(extras[0].textContent).not.toContain("quoted");
    expect(extras[0].textContent).not.toContain("bought");
    expect(extras[1].textContent).toContain("Grouped surcharge");
    expect(extras[1].textContent).toContain("Delivered package");

    const row = card().querySelector<HTMLElement>("[data-fulfillment-item]");
    expect(row?.textContent).not.toContain("Freight surcharge");
    expect(row?.textContent).not.toContain("Grouped surcharge");
  });

  it("puts the aggregate title once above its children and adds no third priced line", async () => {
    const groupPage = page({
      quotations: [
        {
          ...page().quotations[0],
          status: "open",
          items: [
            item({ quotationItemNo: 1, label: "1", groupTitle: "Delivered package" }),
            item({
              quotationItemNo: 2,
              quotationSubNo: 3,
              label: "2.3",
              description: "Child two",
              groupTitle: "Delivered package",
            }),
          ],
        },
      ],
    });
    await render({ load: vi.fn(async () => groupPage) });

    expect(
      card().querySelectorAll("[data-fulfillment-group-title]").length
    ).toBe(1);
    expect(card().querySelectorAll("[data-fulfillment-item]").length).toBe(2);
    expect(
      card().querySelectorAll('[data-fulfillment-item][data-fulfillment-group]')
        .length
    ).toBe(2);
    expect(text()).toContain("Delivered package");

    // #657 layout fix: the same page has a stacked narrow representation. It
    // carries every figure without a table, and its group title sits above both
    // children. The table wrapper is hidden below md; the stacked block is
    // hidden at md and up.
    const narrow = card().querySelector<HTMLElement>(
      "[data-fulfillment-narrow]"
    );
    if (narrow === null) throw new Error("no narrow fulfillment layout");
    expect(narrow.className).toContain("md:hidden");
    expect(
      card().querySelector("[data-fulfillment-table]")?.parentElement?.className
    ).toContain("hidden md:block");
    expect(card().querySelector(".overflow-x-auto")).toBeNull();

    const narrowItems = narrow.querySelectorAll<HTMLElement>(
      "[data-fulfillment-narrow-item]"
    );
    expect(narrowItems).toHaveLength(2);
    expect(
      narrow.querySelector("[data-fulfillment-narrow-group-title]")?.textContent
    ).toBe("Delivered package");
    const firstNarrow = narrowItems[0];
    for (const [name, value] of [
      ["quoted", "5"],
      ["bought", "3"],
      ["unsold", "2"],
      ["over-bought", "0"],
    ]) {
      expect(
        firstNarrow.querySelector(
          `[data-fulfillment-narrow-quantity="${name}"]`
        )?.textContent
      ).toBe(value);
    }
    expect(
      firstNarrow.querySelector(
        '[data-fulfillment-narrow-money="realized-amount"]'
      )?.textContent
    ).toBe("Rp60.000");
    expect(firstNarrow.textContent).toContain("10 Mar 2026");
  });

  it("gives every quotation table the same fixed column model and compact PO dates", async () => {
    const base = page();
    const secondQuotation = {
      ...base.quotations[0],
      quotationId: 8,
      quoteNumber: "1502260008",
      status: "open",
      items: [item({ quotationItemNo: 2, label: "2" })],
    };
    const twoQuotations = page({
      quotations: [base.quotations[0], secondQuotation],
    });
    await render({ load: vi.fn(async () => twoQuotations) });

    const tables = Array.from(
      card().querySelectorAll<HTMLTableElement>("[data-fulfillment-table]")
    );
    expect(tables).toHaveLength(2);
    const widths = [
      "5%",
      "16%",
      "6%",
      "6%",
      "6%",
      "6%",
      "12%",
      "12%",
      "13%",
      "18%",
    ];
    for (const table of tables) {
      expect(table.className).toContain("table-fixed");
      expect(table.className).toContain("w-full");
      const columns = Array.from(
        table.querySelectorAll<HTMLTableColElement>("colgroup col")
      );
      expect(columns.map((column) => column.style.width)).toEqual(widths);
    }

    // The saved-PO date is the compact readable form, so it fits the fixed
    // cell and never relies on a clipped raw ISO string.
    const tableText = tables[0].textContent ?? "";
    expect(tableText).toContain("10 Mar 2026");
    expect(tableText).not.toContain("2026-03-10");
  });

  it("marks a removed leaf and shows zero quoted on it", async () => {
    const removedPage = page({
      quotations: [
        {
          ...page().quotations[0],
          items: [
            item({
              removed: true,
              quotedQuantityScaled: 0,
              boughtQuantityScaled: 30000,
              unsoldQuantityScaled: 0,
              overBoughtQuantityScaled: 0,
            }),
          ],
        },
      ],
    });
    await render({ load: vi.fn(async () => removedPage) });

    expect(text()).toContain("Removed");
    const row = card().querySelector<HTMLElement>("[data-fulfillment-item]");
    expect(
      row?.querySelector('[data-fulfillment-quantity="quoted"]')?.textContent
    ).toBe("0");
    expect(
      row?.querySelector('[data-fulfillment-quantity="over-bought"]')
        ?.textContent
    ).toBe("0");
  });

  it("shows closed loss and zero unsold on a closed quotation", async () => {
    const closedPage = page({
      quotations: [
        {
          ...page().quotations[0],
          items: [
            item({
              unsoldQuantityScaled: 0,
              unsoldAmountMinor: 0,
              unsoldMarginMinor: 0,
              closedLossAmountMinor: 40000,
              closedLossMarginMinor: 20000,
            }),
          ],
        },
      ],
    });
    await render({ load: vi.fn(async () => closedPage) });

    const row = card().querySelector<HTMLElement>("[data-fulfillment-item]");
    const money = (name: string): string =>
      row?.querySelector(`[data-fulfillment-money="${name}"]`)?.textContent ??
      "";
    expect(
      row?.querySelector('[data-fulfillment-quantity="unsold"]')?.textContent
    ).toBe("0");
    expect(money("unsold-amount")).toBe("Rp0");
    expect(money("closed-loss-amount")).toBe("−Rp40.000");
    expect(money("closed-loss-margin")).toBe("−Rp20.000");
  });

  it("never renders a supplier unit cost or the word supplier", async () => {
    await render();
    expect(text().toLowerCase()).not.toContain("supplier");
    expect(text().toLowerCase()).not.toContain("unit cost");
    expect(text().toLowerCase()).not.toContain("cost/unit");
    expect(text().toLowerCase()).not.toContain("cost unit");
  });

  it("sends the status, trimmed search and 25-row page the server expects", async () => {
    const load = vi.fn(async (_query: PurchaseOrderFulfillmentQuery) => page());
    await render({ load });

    expect(load).toHaveBeenCalledTimes(1);
    expect(load.mock.calls[0][0]).toEqual({
      currency: "IDR",
      status: "all",
      search: "",
      page: 1,
      pageSize: 25,
    });

    const select = card().querySelector<HTMLSelectElement>(
      'select[aria-label="Fulfillment status"]'
    );
    if (select === null) throw new Error("no status filter");
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLSelectElement.prototype,
        "value"
      )?.set;
      setter?.call(select, "closed");
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    expect(load.mock.calls[1][0]).toMatchObject({
      status: "closed",
      page: 1,
    });

    const search = card().querySelector<HTMLInputElement>(
      'input[aria-label="Search quotations"]'
    );
    if (search === null) throw new Error("no search input");
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      setter?.call(search, "  infineon  ");
      search.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      card()
        .querySelector<HTMLButtonElement>('button[aria-label="Search fulfillment"]')
        ?.click();
    });
    await flush();
    expect(load.mock.calls[2][0]).toMatchObject({
      search: "infineon",
      page: 1,
    });
  });

  it("pages 25 at a time with honest Previous/Next bounds", async () => {
    const load = vi.fn(async (query: PurchaseOrderFulfillmentQuery) =>
      page({
        filter: query,
        page: query.page,
        pageSize: 25,
        total: 26,
        totalPages: 2,
      })
    );
    await render({ load });
    expect(text()).toContain("Page 1 of 2");
    expect(text()).toContain("26 quotations");

    await act(async () => {
      card()
        .querySelector<HTMLButtonElement>('button[aria-label="Next fulfillment page"]')
        ?.click();
    });
    await flush();
    expect(load.mock.calls[1][0]).toMatchObject({ page: 2, pageSize: 25 });
    expect(text()).toContain("Page 2 of 2");

    await act(async () => {
      card()
        .querySelector<HTMLButtonElement>('button[aria-label="Previous fulfillment page"]')
        ?.click();
    });
    await flush();
    expect(load.mock.calls[2][0]).toMatchObject({ page: 1 });
  });

  it("clears the old figures when a mutation refreshes and shows the new ones", async () => {
    const first = page();
    let releaseSecond: ((value: PurchaseOrderFulfillmentPage) => void) | null =
      null;
    const load = vi
      .fn<(query: PurchaseOrderFulfillmentQuery) => Promise<PurchaseOrderFulfillmentPage>>()
      .mockResolvedValueOnce(first)
      .mockImplementationOnce(
        () =>
          new Promise<PurchaseOrderFulfillmentPage>((resolve) => {
            releaseSecond = resolve;
          })
      );

    const value = props({ load, mutationEpoch: 0 });
    await act(async () => {
      root.render(<PurchaseOrderFulfillmentCard {...value} />);
    });
    await flush();
    expect(text()).toContain("Rp60.000");

    await act(async () => {
      root.render(<PurchaseOrderFulfillmentCard {...value} mutationEpoch={1} />);
    });
    await flush();
    // The old page is gone while the new read is in flight, so a closed or
    // corrected quotation can never keep its old figures on screen.
    expect(text()).not.toContain("Rp60.000");
    expect(text()).toContain("Loading fulfillment");

    const refreshed = page({
      quotations: [
        {
          ...page().quotations[0],
          items: [item({ realizedAmountMinor: 80000, realizedMarginMinor: 40000 })],
        },
      ],
    });
    await act(async () => {
      releaseSecond?.(refreshed);
    });
    await flush();
    expect(text()).toContain("Rp80.000");
    expect(text()).not.toContain("Rp60.000");
  });

  it("shows the admin close and reopen actions, and nothing to staff (#648)", async () => {
    const open = page({
      quotations: [{ ...page().quotations[0], status: "open" }],
    });
    const closeQuotation = vi.fn(async () => undefined);
    const reopenQuotation = vi.fn(async () => undefined);

    await render({
      load: vi.fn(async () => open),
      isAdmin: true,
      onCloseQuotation: closeQuotation,
      onReopenQuotation: reopenQuotation,
    });
    expect(buttonOrNull("Close quotation 1502260007")).not.toBeNull();
    expect(buttonOrNull("Reopen quotation 1502260007")).toBeNull();

    const closed = page();
    await render({
      load: vi.fn(async () => closed),
      isAdmin: true,
      onCloseQuotation: closeQuotation,
      onReopenQuotation: reopenQuotation,
    });
    expect(buttonOrNull("Reopen quotation 1502260007")).not.toBeNull();
    expect(buttonOrNull("Close quotation 1502260007")).toBeNull();

    await render({
      load: vi.fn(async () => open),
      isAdmin: false,
      onCloseQuotation: closeQuotation,
      onReopenQuotation: reopenQuotation,
    });
    expect(buttonOrNull("Close quotation 1502260007")).toBeNull();
    expect(buttonOrNull("Reopen quotation 1502260007")).toBeNull();
  });

  it("closes with the typed reason, and keeps the dialog open on a refusal (#648)", async () => {
    const open = page({
      quotations: [{ ...page().quotations[0], status: "open" }],
    });
    const closeQuotation = vi
      .fn<(id: number, reason: string) => Promise<unknown>>()
      .mockRejectedValueOnce(new Error("quotation_close_forbidden"));
    await render({
      load: vi.fn(async () => open),
      isAdmin: true,
      onCloseQuotation: closeQuotation,
      onReopenQuotation: vi.fn(async () => undefined),
    });

    await act(async () => {
      button("Close quotation 1502260007").click();
    });
    const reason = document.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="Close reason"]'
    );
    if (reason === null) throw new Error("no close reason field");
    act(() => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        "value"
      )?.set;
      setter?.call(reason, "Customer cancelled the remainder");
      reason.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      button("Confirm close quotation").click();
    });
    await flush();

    expect(closeQuotation).toHaveBeenCalledTimes(1);
    expect(closeQuotation).toHaveBeenCalledWith(
      7,
      "Customer cancelled the remainder"
    );
    // No optimistic close: the refusal and the typed sentence stay on screen.
    expect(document.body.textContent).toContain("quotation_close_forbidden");
    expect(
      document.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="Close reason"]'
      )?.value
    ).toBe("Customer cancelled the remainder");

    closeQuotation.mockResolvedValueOnce(undefined);
    await act(async () => {
      button("Confirm close quotation").click();
    });
    await flush();
    expect(closeQuotation).toHaveBeenCalledTimes(2);
    expect(document.querySelector('textarea[aria-label="Close reason"]')).toBeNull();
  });

  it("reopens a closed quotation through the binding (#648)", async () => {
    const reopenQuotation = vi.fn(async () => undefined);
    await render({
      isAdmin: true,
      onCloseQuotation: vi.fn(async () => undefined),
      onReopenQuotation: reopenQuotation,
    });

    await act(async () => {
      button("Reopen quotation 1502260007").click();
    });
    await flush();

    expect(reopenQuotation).toHaveBeenCalledTimes(1);
    expect(reopenQuotation).toHaveBeenCalledWith(7);
  });

  it("surfaces an error with a retry, and never invents figures", async () => {
    const load = vi
      .fn<(query: PurchaseOrderFulfillmentQuery) => Promise<PurchaseOrderFulfillmentPage>>()
      .mockRejectedValueOnce(new Error("the API is unreachable"))
      .mockResolvedValueOnce(page());
    await render({ load });

    expect(text()).toContain("the API is unreachable");
    expect(text()).not.toContain("Rp60.000");

    await act(async () => {
      card()
        .querySelector<HTMLButtonElement>('button[aria-label="Retry fulfillment"]')
        ?.click();
    });
    await flush();
    expect(load).toHaveBeenCalledTimes(2);
    expect(text()).toContain("Rp60.000");
  });

  it("does not read while the Reports page is hidden, and reads on activation", async () => {
    const load = vi.fn(async () => page());
    await render({ load, active: false });
    expect(load).not.toHaveBeenCalled();
    expect(text()).toContain("Loading fulfillment");

    await rerender({ ...props({ load, active: true }) });
    expect(load).toHaveBeenCalledTimes(1);
    expect(text()).toContain("1502260007");
  });

  it("states the empty result and the no-date-filter rule", async () => {
    await render({ load: vi.fn(async () => page({ quotations: [], total: 0, totalPages: 1 })) });
    expect(text()).toContain("No date filter");
    expect(text()).toContain("No quotations match these filters");
  });
});
