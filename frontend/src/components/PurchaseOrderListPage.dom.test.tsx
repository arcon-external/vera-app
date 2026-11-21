// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PurchaseOrderListPage } from "./PurchaseOrderListPage";
import { PurchaseOrderListView } from "./PurchaseOrderListPage";
import { initialPOListState } from "../poList";
import type { POListQuery } from "../poList";

// Issue #481: the archive's Source missing filter is a real request, not a
// disabled promise. This suite drives the real page with the Wails binding
// replaced at its module boundary and asserts the query the server receives:
// "any" on first load, "only"/"exclude" when the Source control changes, and
// back to "any" when the filters reset. The SSR suite owns the markup; this one
// owns the decision that reaches the binding.

const { listMock, previewMock, challengeMock, confirmMock } = vi.hoisted(() => ({
  listMock: vi.fn<(query: POListQuery) => Promise<unknown>>(),
  previewMock: vi.fn(),
  challengeMock: vi.fn(),
  confirmMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  ListPurchaseOrders: (query: POListQuery) => listMock(query),
  PreviewDeletion: (...args: unknown[]) => previewMock(...args),
  CreateDeletionChallenge: (...args: unknown[]) => challengeMock(...args),
  ConfirmDeletion: (...args: unknown[]) => confirmMock(...args),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function page(overrides: Record<string, unknown> = {}) {
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

  it("queries All categories first, then exactly the chosen category", async () => {
    await act(async () => {
      root.render(
        <PurchaseOrderListPage onOpen={() => undefined} onAdd={() => undefined} />
      );
    });
    await flush();
    expect(listMock.mock.calls[0][0].Category).toBe("");

    click(button("Business category"));
    const options = Array.from(
      document.querySelectorAll<HTMLElement>('[role="option"]')
    );
    expect(options.map((option) => option.textContent)).toEqual([
      "All categories",
      "Facility Management",
      "Fabrication",
      "Goods Supply & General",
      "Others",
    ]);
    const fabrication = options.find(
      (option) => option.textContent === "Fabrication"
    );
    if (fabrication === undefined) throw new Error("no Fabrication option");
    click(fabrication);
    await flush();
    expect(listMock).toHaveBeenCalledTimes(2);
    expect(listMock.mock.calls[1][0].Category).toBe("FAB");
    expect(container.textContent).toContain("Category: Fabrication");
    expect(container.textContent).toContain("1 filter active");
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

describe("PurchaseOrderListView dockets (ticket 03)", () => {
  let viewRoot: Root;
  let viewContainer: HTMLDivElement;

  function docketRow(overrides: Record<string, unknown> = {}) {
    return {
      UUID: "uuid-1",
      State: "draft",
      PONumber: "PO-1",
      QuoteNumber: "1502260012",
      CustomerName: "PT. Buyer",
      ReceivedDate: "2026-10-02",
      Currency: "IDR",
      ID: 7,
      CustomerID: 2,
      SourceRevision: 1,
      SourceMissing: false,
      ...overrides,
    };
  }

  function viewState(overrides: Record<string, unknown> = {}) {
    return {
      ...initialPOListState,
      rows: [docketRow()],
      customers: [{ ID: 2, Name: "PT. Buyer" }],
      total: 1,
      totalPages: 1,
      ...overrides,
    };
  }

  beforeEach(() => {
    viewContainer = document.createElement("div");
    document.body.appendChild(viewContainer);
    viewRoot = createRoot(viewContainer);
  });

  afterEach(() => {
    act(() => viewRoot.unmount());
    viewContainer.remove();
  });

  async function renderView(props: Record<string, unknown> = {}) {
    await act(async () => {
      viewRoot.render(
        <PurchaseOrderListView
          state={viewState()}
          dispatch={() => undefined}
          onOpen={() => undefined}
          onAdd={() => undefined}
          onRetry={() => undefined}
          {...(props as Record<string, never>)}
        />
      );
    });
    await flush();
  }

  it("shows each row's business category and reads empty as Unassigned", async () => {
    await renderView({
      state: viewState({
        rows: [
          docketRow({ ID: 7, BusinessCategory: "FM" }),
          docketRow({ ID: 8, PONumber: "PO-2", BusinessCategory: "" }),
          docketRow({ ID: 9, PONumber: "PO-3", BusinessCategory: undefined }),
          docketRow({ ID: 10, PONumber: "PO-4", BusinessCategory: "LEGACY_CODE" }),
        ],
      }),
    });
    expect(viewContainer.textContent).toContain("Facility Management");
    expect(viewContainer.textContent).toContain("Unassigned");
    // A code this build does not know is shown as stored, never folded into
    // Unassigned.
    expect(viewContainer.textContent).toContain("LEGACY_CODE");
    expect(viewContainer.textContent).not.toContain("FAB");
  });

  it("opens only through the Open button, never through the row body", async () => {
    const onOpen = vi.fn();
    await renderView({ onOpen });
    const open = button("Open purchase order PO-1");
    open.focus();
    expect(document.activeElement).toBe(open);
    click(open);
    expect(onOpen).toHaveBeenCalledTimes(1);
    expect(onOpen).toHaveBeenCalledWith(7);

    // The row body is not a click target: clicking it opens nothing.
    const article = document.querySelector("article");
    if (article === null) throw new Error("no docket card rendered");
    click(article);
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it("keeps Delete in a separate menu with keyboard close and focus return", async () => {
    const onDelete = vi.fn();
    await renderView({ onDelete });
    // No Delete beside Open: the only delete control lives behind ⋯.
    expect(
      document.querySelector('button[aria-label^="Delete purchase order"]')
    ).toBeNull();
    const trigger = button("More actions for purchase order PO-1");
    click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    const item = button("Delete purchase order PO-1");
    click(item);
    expect(onDelete).toHaveBeenCalledTimes(1);

    // Escape closes the menu and returns focus to the trigger.
    click(trigger);
    expect(
      document.querySelector('button[aria-label="Delete purchase order PO-1"]')
    ).not.toBeNull();
    act(() => {
      trigger.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", bubbles: true })
      );
    });
    expect(
      document.querySelector('button[aria-label="Delete purchase order PO-1"]')
    ).toBeNull();
    expect(document.activeElement).toBe(trigger);

    // Outside pointer closes it too.
    click(trigger);
    act(() => {
      document.body.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    });
    expect(
      document.querySelector('button[aria-label="Delete purchase order PO-1"]')
    ).toBeNull();
  });

  it("renders no admin column at all for non-admins", async () => {
    await renderView();
    expect(
      document.querySelector('button[aria-label^="More actions for purchase order"]')
    ).toBeNull();
    expect(document.body.textContent).not.toContain("Delete");
  });

  it("reports Opening… on the activating row and recovers on failure", async () => {
    let rejectOpen!: (error: unknown) => void;
    const onOpen = vi.fn(
      () => new Promise<unknown>((_, reject) => (rejectOpen = reject))
    );
    await renderView({
      onOpen,
      state: viewState({
        rows: [docketRow(), docketRow({ ID: 8, PONumber: "PO-2", UUID: "uuid-2" })],
        total: 2,
      }),
    });
    click(button("Open purchase order PO-1"));
    expect(button("Open purchase order PO-1").textContent).toBe("Opening…");
    expect(
      button("Open purchase order PO-1").hasAttribute("disabled")
    ).toBe(true);
    // The other row stays usable while one opens.
    expect(button("Open purchase order PO-2").textContent).toBe("Open");
    expect(button("Open purchase order PO-2").hasAttribute("disabled")).toBe(
      false
    );

    act(() => rejectOpen(new Error("offline")));
    await flush();
    expect(button("Open purchase order PO-1").textContent).toBe("Open");
    expect(button("Open purchase order PO-1").hasAttribute("disabled")).toBe(
      false
    );
  });

  it("toggles the filter panel and removes one dimension through its chip", async () => {
    const dispatch = vi.fn();
    await renderView({
      dispatch,
      state: viewState({
        filters: {
          search: "ACME",
          customerIds: [2],
          receivedFrom: "",
          receivedTo: "",
          state: "draft",
          sourceMissing: "any",
          category: "",
        },
      }),
    });
    const toggle = button("Show filters");
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    click(toggle);
    expect(button("Hide filters").getAttribute("aria-expanded")).toBe("true");

    click(button("Remove ACME filter"));
    expect(dispatch).toHaveBeenCalledWith({
      type: "filtersChanged",
      patch: { search: "" },
    });
  });

  it("dims loaded rows while reloading and keeps paging disabled", async () => {
    await renderView({ state: viewState({ loading: true }) });
    expect(document.querySelector(".opacity-60")).not.toBeNull();
    expect(button("Previous page").hasAttribute("disabled")).toBe(true);
    expect(button("Next page").hasAttribute("disabled")).toBe(true);
  });
});

// Issue #645: the report workspace needs a success signal for a confirmed,
// permanent deletion. The list page already owns the dialog, so the callback is
// decided here -- and only for a record whose deletion changes report evidence
// (a saved PO). A draft, a cancel and a failed confirmation must stay silent.
describe("deletion success callback (issue #645)", () => {
  let root: Root;
  let container: HTMLDivElement;

  beforeEach(() => {
    Element.prototype.scrollIntoView = () => undefined;
    listMock.mockReset();
    previewMock.mockReset();
    challengeMock.mockReset();
    confirmMock.mockReset();
    previewMock.mockResolvedValue(deletionPreview());
    confirmMock.mockResolvedValue({
      Outcome: { Removed: [deletionRecord()], AuditIDs: [1] },
    });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    document.body.innerHTML = "";
  });

  function deletionRecord(overrides: Record<string, unknown> = {}) {
    return {
      Type: "purchase_order",
      ID: 7,
      UUID: "uuid-7",
      DocumentNumber: "PO-645",
      SourceReference: "1502260045",
      Label: "Purchase order PO-645",
      State: "saved",
      Version: 3,
      Revisions: [] as number[],
      DocumentVersions: 0,
      HistoryEntries: 0,
      ...overrides,
    };
  }

  function deletionPreview(overrides: Record<string, unknown> = {}) {
    return {
      TargetType: "purchase_order",
      TargetID: 7,
      TargetUUID: "uuid-7",
      DocumentNumber: "PO-645",
      DocumentNumberLabel: "Customer PO number",
      QuoteNumber: "1502260045",
      PONumber: "PO-645",
      CustomerName: "PT. Buyer",
      State: "saved",
      SourceMissing: false,
      Version: 3,
      Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
      Scopes: [
        {
          Scope: "record",
          Available: true,
          UnavailableReason: "",
          RequiresChallenge: false,
          Removes: [deletionRecord()],
          Preserves: [
            deletionRecord({
              Type: "quotation",
              ID: 4,
              Label: "Quotation 1502260045",
            }),
          ],
        },
      ],
      ...overrides,
    };
  }

  function savedRow(overrides: Record<string, unknown> = {}) {
    return {
      UUID: "uuid-7",
      State: "saved",
      PONumber: "PO-645",
      QuoteNumber: "1502260045",
      CustomerName: "PT. Buyer",
      ReceivedDate: "2026-10-02",
      Currency: "IDR",
      ID: 7,
      CustomerID: 2,
      SourceRevision: 1,
      SourceMissing: false,
      ...overrides,
    };
  }

  function typeField(label: string, value: string) {
    const field = document.querySelector<
      HTMLInputElement | HTMLTextAreaElement
    >(`input[aria-label="${label}"], textarea[aria-label="${label}"]`);
    if (field === null) throw new Error(`no field labelled ${label}`);
    const prototype =
      field instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    act(() => {
      Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(field, value);
      field.dispatchEvent(new Event("input", { bubbles: true }));
    });
  }

  function buttonWithText(text: string): HTMLButtonElement {
    const found = Array.from(document.querySelectorAll("button")).find(
      (candidate) => candidate.textContent?.trim() === text
    );
    if (found === undefined) throw new Error(`no button reading ${text}`);
    return found;
  }

  async function renderList(
    props: {
      mode?: "drafts" | "archive";
      onDeleted?: () => void;
    } = {}
  ) {
    await act(async () => {
      root.render(
        <PurchaseOrderListPage
          mode={props.mode}
          isAdmin
          onOpen={() => undefined}
          onAdd={() => undefined}
          onDeleted={props.onDeleted}
        />
      );
    });
    await flush();
  }

  async function openDeletionDialog() {
    click(button("More actions for purchase order PO-645"));
    click(button("Delete purchase order PO-645"));
    await flush();
  }

  async function confirmDeletion() {
    typeField("Deletion reason", "duplicate entry");
    typeField("Type PO-645 to confirm", "PO-645");
    click(button("Delete permanently"));
    await flush();
  }

  it("fires once after a confirmed saved purchase-order deletion", async () => {
    listMock.mockResolvedValue(page({ PurchaseOrders: [savedRow()] }));
    const onDeleted = vi.fn();
    await renderList({ mode: "archive", onDeleted });

    await openDeletionDialog();
    expect(confirmMock).not.toHaveBeenCalled();
    expect(onDeleted).not.toHaveBeenCalled();

    await confirmDeletion();
    expect(confirmMock).toHaveBeenCalledTimes(1);
    expect(onDeleted).toHaveBeenCalledTimes(1);
  });

  it("stays silent when the dialog is cancelled", async () => {
    listMock.mockResolvedValue(page({ PurchaseOrders: [savedRow()] }));
    const onDeleted = vi.fn();
    await renderList({ mode: "archive", onDeleted });

    await openDeletionDialog();
    act(() => buttonWithText("Cancel").click());
    await flush();

    expect(confirmMock).not.toHaveBeenCalled();
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("stays silent when the confirmation fails", async () => {
    listMock.mockResolvedValue(page({ PurchaseOrders: [savedRow()] }));
    confirmMock.mockRejectedValue(new Error("the record is locked"));
    const onDeleted = vi.fn();
    await renderList({ mode: "archive", onDeleted });

    await openDeletionDialog();
    await confirmDeletion();

    expect(confirmMock).toHaveBeenCalledTimes(1);
    expect(onDeleted).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("the record is locked");
  });

  it("stays silent when a draft purchase order is deleted", async () => {
    listMock.mockResolvedValue(
      page({ PurchaseOrders: [savedRow({ State: "draft" })] })
    );
    // The server's confirmed preview is the authority: a genuinely draft
    // record stays silent even when the list row was captured as a draft.
    previewMock.mockResolvedValue(deletionPreview({ State: "draft" }));
    const onDeleted = vi.fn();
    await renderList({ mode: "drafts", onDeleted });

    await openDeletionDialog();
    await confirmDeletion();

    expect(confirmMock).toHaveBeenCalledTimes(1);
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("fires when the server confirms a row captured as draft is now saved", async () => {
    listMock.mockResolvedValue(
      page({ PurchaseOrders: [savedRow({ State: "draft" })] })
    );
    // Another user finalized the draft between the list read and the
    // confirmation. The list row still says draft; the server's preview says
    // saved, so the deletion is report evidence and must invalidate.
    previewMock.mockResolvedValue(deletionPreview({ State: "saved" }));
    const onDeleted = vi.fn();
    await renderList({ mode: "drafts", onDeleted });

    await openDeletionDialog();
    await confirmDeletion();

    expect(confirmMock).toHaveBeenCalledTimes(1);
    expect(onDeleted).toHaveBeenCalledTimes(1);
  });
});
