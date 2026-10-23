// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PurchaseOrderListPage } from "./PurchaseOrderListPage";
import type { POListQuery } from "../poList";

// Issue #481: the archive's Source missing filter is a real request, not a
// disabled promise. This suite drives the real page with the Wails binding
// replaced at its module boundary and asserts the query the server receives:
// "any" on first load, "only"/"exclude" when the Source control changes, and
// back to "any" when the filters reset. The SSR suite owns the markup; this one
// owns the decision that reaches the binding.

const { listMock } = vi.hoisted(() => ({
  listMock: vi.fn<(query: POListQuery) => Promise<unknown>>(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  ListPurchaseOrders: (query: POListQuery) => listMock(query),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function page(overrides: { PageSize?: number } = {}) {
  return {
    PurchaseOrders: [],
    Customers: [],
    Page: 1,
    PageSize: 15,
    Total: 0,
    TotalPages: 1,
    ...overrides,
  };
}

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function button(label: string): HTMLButtonElement {
  const node = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (node === null) throw new Error(`no button labelled ${label}`);
  return node;
}

function click(element: Element): void {
  act(() => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

describe("PurchaseOrderListPage source-missing filter (issue #481)", () => {
  let root: Root;
  let container: HTMLDivElement;

  beforeEach(() => {
    // jsdom does not implement scrolling; Select only keeps its highlighted
    // option in view.
    Element.prototype.scrollIntoView = () => undefined;
    listMock.mockReset();
    listMock.mockResolvedValue(page());
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("queries every source state first, then the chosen predicate", async () => {
    await act(async () => {
      root.render(
        <PurchaseOrderListPage onOpen={() => undefined} onAdd={() => undefined} />
      );
    });
    await flush();
    expect(listMock).toHaveBeenCalledTimes(1);
    expect(listMock.mock.calls[0][0].SourceMissing).toBe("any");
    // Issue #492: every query states the list's page size; the default is 15.
    expect(listMock.mock.calls[0][0].PageSize).toBe(15);

    // The control is enabled and opens the three real modes.
    click(button("Source"));
    const options = Array.from(
      document.querySelectorAll<HTMLElement>('[role="option"]')
    );
    expect(options.map((option) => option.textContent)).toEqual([
      "Any source",
      "Source missing only",
      "Source present only",
    ]);

    const only = options.find(
      (option) => option.textContent === "Source missing only"
    );
    if (only === undefined) throw new Error("no source-missing option");
    click(only);
    await flush();
    expect(listMock).toHaveBeenCalledTimes(2);
    expect(listMock.mock.calls[1][0].SourceMissing).toBe("only");
    expect(container.textContent).toContain("1 filter active");

    // Exclude is the other real predicate, and the count still reads one.
    click(button("Source"));
    const exclude = Array.from(
      document.querySelectorAll<HTMLElement>('[role="option"]')
    ).find((option) => option.textContent === "Source present only");
    if (exclude === undefined) throw new Error("no source-present option");
    click(exclude);
    await flush();
    expect(listMock.mock.calls[2][0].SourceMissing).toBe("exclude");
    expect(container.textContent).toContain("1 filter active");

    // Reset clears the source predicate with the rest of the filters.
    click(button("Reset filters"));
    await flush();
    expect(listMock.mock.calls[3][0].SourceMissing).toBe("any");
    expect(container.textContent).toContain("No filters");
  });

  it("sends the remembered page size, and the answer's size stays authoritative", async () => {
    // A real server echoes the requested size. One request proves the page
    // sends the remembered size and adopts the answer's size without re-asking.
    listMock.mockResolvedValue(page({ PageSize: 25 }));
    await act(async () => {
      root.render(
        <PurchaseOrderListPage
          mode="archive"
          pageSize={25}
          onOpen={() => undefined}
          onAdd={() => undefined}
        />
      );
    });
    await flush();
    expect(listMock).toHaveBeenCalledTimes(1);
    expect(listMock.mock.calls[0][0].PageSize).toBe(25);
  });
});
