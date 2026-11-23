// @vitest-environment jsdom
import { act, useReducer } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToString } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EditorItem } from "./ItemEditor";
import { ItemEditor } from "./ItemEditor";
import { PurchaseOrderAggregateHeader } from "./PurchaseOrderAggregateHeader";
import { PurchaseOrderEditorView } from "./PurchaseOrderEditor";
import {
  initialPurchaseOrderEditorState,
  purchaseOrderEditorReducer,
  type PurchaseOrderAggregateGroupState,
  type PurchaseOrderDTO,
  type PurchaseOrderDTOItem,
} from "../purchaseOrder";
import type { PurchaseOrderSourceLineDTO } from "../poFinalize";
import type { CurrencyInfo } from "../format";

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const GROUP: PurchaseOrderAggregateGroupState = {
  key: 0,
  title: "Combined items",
  orderedQuantity: "2",
  orderedUoM: "Lot",
  showChildrenToCustomer: false,
};

// Issue #654 wave 4C: the aggregate line editor. This file covers the shared
// ItemEditor segmentation (a wrapper must never change quotation markup), the
// aggregate header card, and the PO editor's aggregate mode.

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const noop = () => {};

function editorItem(overrides: Partial<EditorItem> = {}): EditorItem {
  return {
    key: 0,
    description: "Widget",
    bullets: [],
    qty: "1",
    uom: "Pc",
    unitPrice: "1000",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    subItems: [],
    showSubPrices: false,
    ...overrides,
  };
}

function renderEditor(
  overrides: Partial<Parameters<typeof ItemEditor>[0]> = {}
): string {
  return renderToString(
    <ItemEditor
      items={[editorItem()]}
      uomOptions={[
        { value: "Lot", label: "Lot" },
        { value: "Pc", label: "Pc" },
      ]}
      onAddItem={noop}
      onRemoveItem={noop}
      onChange={noop}
      onAddBullet={noop}
      onRemoveBullet={noop}
      onBulletChange={noop}
      onEnableSubItems={noop}
      onDisableSubItems={noop}
      onAddSubRow={noop}
      onRemoveSubRow={noop}
      onPatchSubRow={noop}
      onMoveSubRow={noop}
      onPasteSubRows={noop}
      onAddSubBullet={noop}
      onRemoveSubBullet={noop}
      onSubBulletChange={noop}
      onPasteItems={noop}
      {...overrides}
    />
  );
}

function inDom(html: string): HTMLDivElement {
  const host = document.createElement("div");
  host.innerHTML = html;
  return host;
}

describe("ItemEditor aggregate segmentation (#654)", () => {
  it("wraps consecutive rows that share a non-empty aggregate key, header once", () => {
    const items = [
      editorItem({ key: 0, description: "First" }),
      editorItem({ key: 1, description: "Second" }),
      editorItem({ key: 2, description: "Loose" }),
    ];
    const host = inDom(
      renderEditor({
        items,
        aggregateKeyOf: (item) => (item.key < 2 ? "a" : ""),
        renderAggregateHeader: (key, groupItems) => (
          <p data-stub-header={key}>{groupItems.length} children</p>
        ),
      })
    );

    const wrappers = host.querySelectorAll("[data-aggregate-key]");
    expect(wrappers).toHaveLength(1);
    expect(wrappers[0]?.getAttribute("data-aggregate-key")).toBe("a");
    expect(wrappers[0]?.querySelectorAll("[data-stub-header]")).toHaveLength(1);
    expect(wrappers[0]?.textContent).toContain("2 children");
    const descriptions = Array.from(
      wrappers[0]?.querySelectorAll<HTMLInputElement>(
        'input[aria-label^="Description for line"]'
      ) ?? []
    ).map((input) => input.value);
    expect(descriptions).toEqual(["First", "Second"]);
    const loose = Array.from(
      host.querySelectorAll<HTMLInputElement>(
        'input[aria-label^="Description for line"]'
      )
    ).map((input) => input.value);
    expect(loose).toContain("Loose");
  });

  it("leaves the flat list untouched when every key is empty", () => {
    const absent = renderEditor();
    const allEmpty = renderEditor({
      aggregateKeyOf: () => "",
      renderAggregateHeader: () => null,
    });
    expect(allEmpty).toBe(absent);
    expect(allEmpty).not.toContain("data-aggregate-key");
  });

  it("leaves quotation markup untouched when the new props are absent", () => {
    const html = renderEditor();
    expect(html).not.toContain("data-aggregate-key");
    expect(html).not.toContain("data-aggregate-header");
  });

  it("stops header keydowns before the line grid keyboard handler", () => {
    const bubbled = vi.fn();
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    act(() =>
      root.render(
        <div onKeyDown={bubbled}>
          <ItemEditor
            items={[editorItem({ description: "First" })]}
            uomOptions={[{ value: "Pc", label: "Pc" }]}
            aggregateKeyOf={() => "a"}
            renderAggregateHeader={() => (
              <input aria-label="Stub aggregate title" />
            )}
            onAddItem={noop}
            onRemoveItem={noop}
            onChange={noop}
            onAddBullet={noop}
            onRemoveBullet={noop}
            onBulletChange={noop}
            onEnableSubItems={noop}
            onDisableSubItems={noop}
            onAddSubRow={noop}
            onRemoveSubRow={noop}
            onPatchSubRow={noop}
            onMoveSubRow={noop}
            onPasteSubRows={noop}
            onAddSubBullet={noop}
            onRemoveSubBullet={noop}
            onSubBulletChange={noop}
            onPasteItems={noop}
          />
        </div>
      )
    );

    const title = container.querySelector<HTMLInputElement>(
      'input[aria-label="Stub aggregate title"]'
    );
    if (title === null) throw new Error("no stub header input");
    act(() => {
      title.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Enter", bubbles: true })
      );
    });
    expect(bubbled).not.toHaveBeenCalled();

    // A child row's input still bubbles, so the assertion above is measuring
    // the header guard rather than a broken test harness.
    const description = container.querySelector<HTMLInputElement>(
      'input[aria-label="Description for line 1"]'
    );
    if (description === null) throw new Error("no line description input");
    act(() => {
      description.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Enter", bubbles: true })
      );
    });
    expect(bubbled).toHaveBeenCalledTimes(1);

    act(() => root.unmount());
    container.remove();
  });
});

describe("PurchaseOrderAggregateHeader (#654)", () => {
  let host: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    document.body.innerHTML = "";
  });

  function renderHeader(
    overrides: Partial<Parameters<typeof PurchaseOrderAggregateHeader>[0]> = {},
    onChange = vi.fn(),
    onUngroup = vi.fn()
  ): { onChange: ReturnType<typeof vi.fn>; onUngroup: ReturnType<typeof vi.fn> } {
    act(() =>
      root.render(
        <PurchaseOrderAggregateHeader
          group={GROUP}
          index={0}
          childCount={2}
          totalMinor={28000}
          displayUnitPriceMinor={14000}
          currency={USD}
          uomOptions={[
            { value: "Lot", label: "Lot" },
            { value: "Pc", label: "Pc" },
          ]}
          onChange={onChange}
          onUngroup={onUngroup}
          {...overrides}
        />
      )
    );
    return { onChange, onUngroup };
  }

  function box(label: string): HTMLInputElement {
    const found = host.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`);
    if (found === null) throw new Error(`no input labelled ${label}`);
    return found;
  }

  function select(label: string): HTMLSelectElement {
    const found = host.querySelector<HTMLSelectElement>(`select[aria-label="${label}"]`);
    if (found === null) throw new Error(`no select labelled ${label}`);
    return found;
  }

  it("shows the title, display quantity/unit, print toggle and derived figures", () => {
    renderHeader();

    expect(box("Aggregate line 1 title").value).toBe("Combined items");
    expect(host.textContent).toContain("14/500");
    expect(box("Aggregate line 1 display quantity").value).toBe("2");
    expect(select("Aggregate line 1 display unit").value).toBe("Lot");
    expect(
      box("Show children to the customer for aggregate line 1").checked
    ).toBe(false);
    expect(host.textContent).toContain(
      "Display only — it does not change what was ordered."
    );

    const total = host.querySelector('[data-aggregate-total="0"]');
    expect(total?.textContent).toBe("$280.00");
    expect(total?.className).toContain("tnum");
    expect(total?.className).toContain("whitespace-nowrap");
    const unitPrice = host.querySelector('[data-aggregate-unit-price="0"]');
    expect(unitPrice?.textContent).toBe("$140.00");
    expect(host.textContent).toContain("2 items");
    expect(host.textContent).toContain("Ungroup");
  });

  it("edits the header through onChange and ungroups explicitly", () => {
    const onChange = vi.fn();
    const onUngroup = vi.fn();
    renderHeader({}, onChange, onUngroup);

    act(() => {
      const title = box("Aggregate line 1 title");
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      setter?.call(title, "Site works");
      title.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(onChange).toHaveBeenLastCalledWith({ title: "Site works" });

    act(() => box("Show children to the customer for aggregate line 1").click());
    expect(onChange).toHaveBeenLastCalledWith({ showChildrenToCustomer: true });

    const ungroup = Array.from(host.querySelectorAll("button")).find(
      (candidate) => candidate.getAttribute("aria-label") === "Ungroup Combined items"
    );
    if (ungroup === undefined) throw new Error("no ungroup button");
    act(() => ungroup.click());
    expect(onUngroup).toHaveBeenCalledTimes(1);
  });

  it("carries no cost, margin, variance or supplier text and no variance badge", () => {
    renderHeader();
    expect(host.textContent ?? "").not.toMatch(/cost|margin|variance|supplier/i);
    expect(host.querySelector("[data-variance-badge]")).toBeNull();
  });

  it("renders an honest dash when the derived figures cannot be shown", () => {
    renderHeader({ totalMinor: null, displayUnitPriceMinor: null });
    expect(host.querySelector('[data-aggregate-total="0"]')?.textContent).toBe("—");
    expect(
      host.querySelector('[data-aggregate-unit-price="0"]')?.textContent
    ).toBe("—");
  });

  it("names an untitled aggregate honestly in the ungroup control", () => {
    renderHeader({ group: { ...GROUP, title: "" } });
    expect(
      Array.from(host.querySelectorAll("button")).some(
        (candidate) =>
          candidate.getAttribute("aria-label") === "Ungroup aggregate line 1"
      )
    ).toBe(true);
  });
});

function aggregateDtoItem(
  overrides: Partial<PurchaseOrderDTOItem> = {}
): PurchaseOrderDTOItem {
  return {
    LineNumber: 1,
    Description: "Steel plate",
    QuantityScaled: 20000,
    UoM: "Pc",
    UnitPriceMinor: 10000,
    DiscountEnabled: false,
    DiscountPercent: 0,
    GrossMinor: 20000,
    DiscountMinor: 0,
    NetMinor: 20000,
    QuotationItemNo: 1,
    QuotationSubNo: null,
    ...overrides,
  };
}

function aggregateOrder(
  overrides: Partial<PurchaseOrderDTO> = {}
): PurchaseOrderDTO {
  return {
    PODate: "2026-10-01",
    RequestedDeliveryDate: null,
    UUID: "uuid-654",
    State: "draft",
    QuoteNumber: "1502260654",
    CustomerName: "PT. Buyer",
    BillingAddress: "",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "PO-654",
    ReceivedDate: "2026-10-02",
    Incoterms: "EXW",
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "Goods",
    BusinessCategory: "OTHERS",
    TaxType: "Goods",
    ServiceCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-02T10:00:00Z",
    UpdatedAt: "2026-10-02T10:00:00Z",
    Items: [aggregateDtoItem()],
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    SourceMissing: false,
    SourceState: "live",
    CustomerID: 2,
    PaymentTerms: "45 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 0,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 0,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    Version: 4,
    ...overrides,
  };
}

const MIXED_DEPTH_ORDER = aggregateOrder({
  Items: [
    aggregateDtoItem({
      ID: 1,
      LineNumber: 1,
      AggregateGroupID: 1,
    }),
    aggregateDtoItem({
      ID: 2,
      LineNumber: 2,
      Description: "Mounting and cable termination",
      QuantityScaled: 10000,
      UoM: "Unit",
      UnitPriceMinor: 5000,
      GrossMinor: 5000,
      NetMinor: 5000,
      QuotationSubNo: 1,
      GroupKey: "q:2",
      ParentDescription: "Mounting package",
      DescriptionLines: ["Cable tray"],
      AggregateGroupID: 1,
    }),
    aggregateDtoItem({
      ID: 3,
      LineNumber: 3,
      Description: "Cable termination and labelling",
      QuantityScaled: 10000,
      UoM: "Unit",
      UnitPriceMinor: 3000,
      GrossMinor: 3000,
      NetMinor: 3000,
      QuotationSubNo: 2,
      GroupKey: "q:2",
      ParentDescription: "Mounting package",
      DescriptionLines: ["Two technicians"],
      AggregateGroupID: 1,
    }),
  ],
  AggregateGroups: [
    {
      ID: 1,
      DisplayOrder: 1,
      Title: "Combined items",
      OrderedQuantityScaled: 20000,
      OrderedUoM: "Lot",
      ShowChildrenToCustomer: false,
      TotalMinor: 28000,
      DisplayUnitPriceMinor: 14000,
    },
  ],
});

const UOMS = [
  { value: "Lot", label: "Lot" },
  { value: "Pc", label: "Pc" },
  { value: "Unit", label: "Unit" },
];
const INCOTERMS = [
  { value: "EXW", label: "EXW" },
  { value: "CIF", label: "CIF" },
];

function sourceRow(
  overrides: Partial<PurchaseOrderSourceLineDTO> = {}
): PurchaseOrderSourceLineDTO {
  return {
    QuotationItemNo: 2,
    QuotationSubNo: null,
    LineLabel: "2",
    Description: "Spare parts",
    UoM: "Pc",
    QuantityScaled: 30000,
    UnitPriceMinor: 5000,
    DiscountEnabled: false,
    DiscountPercent: 0,
    ...overrides,
  };
}

function ViewHarness({
  order,
  quoteLines,
}: {
  order: PurchaseOrderDTO;
  quoteLines?: PurchaseOrderSourceLineDTO[] | null;
}) {
  const [state, dispatch] = useReducer(purchaseOrderEditorReducer, undefined, () =>
    initialPurchaseOrderEditorState(order, USD)
  );
  return (
    <PurchaseOrderEditorView
      state={state}
      dispatch={dispatch}
      currency={USD}
      uomOptions={UOMS}
      incotermOptions={INCOTERMS}
      quoteLines={quoteLines ?? null}
      onSave={noop}
      onCancel={noop}
      onFinalSave={noop}
      finalSaving={false}
    />
  );
}

describe("PO editor aggregate mode (#654)", () => {
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
  });

  function render(order: PurchaseOrderDTO, quoteLines?: PurchaseOrderSourceLineDTO[]) {
    act(() => root.render(<ViewHarness order={order} quoteLines={quoteLines} />));
  }

  function checkbox(label: string): HTMLInputElement {
    const found = document.querySelector<HTMLInputElement>(
      `input[type="checkbox"][aria-label="${label}"]`
    );
    if (found === null) throw new Error(`no checkbox labelled ${label}`);
    return found;
  }

  function button(label: string): HTMLButtonElement {
    const found = document.querySelector<HTMLButtonElement>(
      `button[aria-label="${label}"]`
    );
    if (found === null) throw new Error(`no button labelled ${label}`);
    return found;
  }

  function lineInput(label: string): HTMLInputElement {
    const found = Array.from(
      document.querySelectorAll<HTMLInputElement>(`input[aria-label="${label}"]`)
    ).find((entry) => entry.closest("[data-quotation-ghost]") === null);
    if (found === undefined) throw new Error(`no line input labelled ${label}`);
    return found;
  }

  function typeInto(element: HTMLInputElement, value: string): void {
    act(() => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value"
      )?.set;
      setter?.call(element, value);
      element.dispatchEvent(new Event("input", { bubbles: true }));
    });
  }

  function toggleMode(): void {
    act(() => checkbox("Aggregate mode").click());
  }

  function headerTotal(key: string): string | null {
    return (
      document
        .querySelector(`[data-aggregate-total="${key}"]`)
        ?.textContent ?? null
    );
  }

  it("groups a selected line and a ghost into one line with the defaults", () => {
    render(
      aggregateOrder({
        Items: [
          aggregateDtoItem({
            ID: 1,
            Description: "Steel plate",
            QuantityScaled: 20000,
            UnitPriceMinor: 10000,
          }),
        ],
      }),
      [
        sourceRow({
          QuotationItemNo: 1,
          QuotationSubNo: null,
          LineLabel: "1",
          Description: "Steel plate",
          UoM: "Pc",
          QuantityScaled: 20000,
          UnitPriceMinor: 10000,
        }),
        sourceRow(),
      ]
    );

    toggleMode();
    const groupButton = button("Group selected items into an aggregate line");
    expect(groupButton.disabled).toBe(true);

    act(() => checkbox("Select line 1 for an aggregate line").click());
    act(() =>
      checkbox("Select quotation item 2 for an aggregate line").click()
    );
    expect(button("Group selected items into an aggregate line").disabled).toBe(
      false
    );

    act(() => button("Group selected items into an aggregate line").click());

    const wrappers = document.querySelectorAll("[data-aggregate-key]");
    expect(wrappers).toHaveLength(1);
    expect(headerTotal("0")).toBe("$350.00");
    expect(document.body.textContent).toContain("2 items in this aggregate");
    expect(
      document.querySelector<HTMLInputElement>(
        'input[aria-label="Aggregate line 1 title"]'
      )?.value
    ).toBe("Combined items");
    expect(
      document.querySelector<HTMLInputElement>(
        'input[aria-label="Aggregate line 1 display quantity"]'
      )?.value
    ).toBe("1");
    expect(
      document.querySelector<HTMLSelectElement>(
        'select[aria-label="Aggregate line 1 display unit"]'
      )?.value
    ).toBe("Lot");
    expect(
      checkbox("Show children to the customer for aggregate line 1").checked
    ).toBe(false);
    expect(lineInput("Description for line 1").value).toBe("Spare parts");
    expect(lineInput("Description for line 2").value).toBe("Steel plate");
    // The title is focused for immediate renaming, the selection clears, and
    // aggregate mode stays on for the next group.
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "Aggregate line 1 title"
    );
    expect(checkbox("Aggregate mode").checked).toBe(true);
    expect(checkbox("Select line 1 for an aggregate line").checked).toBe(false);
  });

  it("keeps a split package whole when a ghost is grouped", () => {
    render(aggregateOrder({ Items: [] }), [
      sourceRow({ QuotationItemNo: 2, QuotationSubNo: 1, LineLabel: "2.1", Description: "Part A", QuantityScaled: 10000, UnitPriceMinor: 5000, ParentDescription: "" }),
      sourceRow({ QuotationItemNo: 2, QuotationSubNo: 2, LineLabel: "2.2", Description: "Part B", QuantityScaled: 20000, UnitPriceMinor: 3000, ParentDescription: "" }),
    ]);

    toggleMode();
    act(() =>
      checkbox("Select quotation item 2 for an aggregate line").click()
    );
    act(() => button("Group selected items into an aggregate line").click());

    const wrapper = document.querySelector('[data-aggregate-key="0"]');
    if (wrapper === null) throw new Error("no aggregate wrapper");
    expect(wrapper.textContent).toContain("1 item in this aggregate");
    expect(headerTotal("0")).toBe("$110.00");
    const subDescriptions = Array.from(
      wrapper.querySelectorAll<HTMLInputElement>(
        'input[aria-label^="Description for sub-item"]'
      )
    ).map((input) => input.value);
    expect(subDescriptions).toEqual(["Part A", "Part B"]);
  });

  it("keeps mixed depths editable and derives the total on every render", () => {
    render(MIXED_DEPTH_ORDER);

    const wrapper = document.querySelector('[data-aggregate-key="0"]');
    if (wrapper === null) throw new Error("no aggregate wrapper");
    expect(headerTotal("0")).toBe("$280.00");
    expect(
      document.querySelector('[data-aggregate-unit-price="0"]')?.textContent
    ).toBe("$140.00");
    expect(wrapper.querySelector<HTMLInputElement>('input[aria-label="Description for line 1"]')?.value).toBe("Steel plate");
    expect(wrapper.querySelector<HTMLInputElement>('input[aria-label="Description for line 2"]')?.value).toBe("Mounting package");
    const bullets = Array.from(
      wrapper.querySelectorAll<HTMLTextAreaElement>('textarea[aria-label^="Sub-sub-item"]')
    ).map((textarea) => textarea.value);
    expect(bullets).toEqual(["Cable tray", "Two technicians"]);
    expect(wrapper.querySelectorAll("[data-variance-badge]")).toHaveLength(0);

    typeInto(lineInput("Description for line 1"), "Steel plate cut to size");
    expect(lineInput("Description for line 1").value).toBe(
      "Steel plate cut to size"
    );
    expect(headerTotal("0")).toBe("$280.00");

    typeInto(lineInput("Quantity for line 1"), "3");
    expect(headerTotal("0")).toBe("$380.00");

    typeInto(lineInput("Unit price for line 1"), "12000");
    expect(headerTotal("0")).toBe("$440.00");

    act(() => {
      const uom = document.querySelector<HTMLSelectElement>(
        'select[aria-label="Unit of measure for line 1"]'
      );
      if (uom === null) throw new Error("no line UoM select");
      uom.value = "Unit";
      uom.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(lineInput("Quantity for line 1").value).toBe("3");
    expect(headerTotal("0")).toBe("$440.00");
  });

  it("does not let the display quantity change bought state or variance", () => {
    render(
      aggregateOrder({
        Items: [
          aggregateDtoItem({
            ID: 1,
            AggregateGroupID: 1,
            QuantityScaled: 20000,
            UnitPriceMinor: 9000,
            QuotedUnitPriceMinor: 10000,
            RemainingQuantityScaled: 30000,
          }),
        ],
        AggregateGroups: [
          {
            ID: 1,
            DisplayOrder: 1,
            Title: "Combined items",
            OrderedQuantityScaled: 10000,
            OrderedUoM: "Lot",
            ShowChildrenToCustomer: false,
            TotalMinor: 18000,
            DisplayUnitPriceMinor: 18000,
          },
        ],
      })
    );

    const badges = () =>
      Array.from(document.querySelectorAll("[data-variance-badge]"))
        .map((node) => node.textContent)
        .join("|");
    const before = badges();
    expect(before).toContain("Unsold");
    expect(before).toContain("Price loss");
    expect(
      document.querySelector("[data-aggregate-header-card] [data-variance-badge]")
    ).toBeNull();

    typeInto(
      document.querySelector<HTMLInputElement>(
        'input[aria-label="Aggregate line 1 display quantity"]'
      ) as HTMLInputElement,
      "5"
    );
    expect(badges()).toBe(before);
    expect(headerTotal("0")).toBe("$180.00");
  });

  it("reparents a child from another aggregate and never nests groups", () => {
    render(
      aggregateOrder({
        Items: [
          aggregateDtoItem({ ID: 1, LineNumber: 1, Description: "Alpha", AggregateGroupID: 1 }),
          aggregateDtoItem({ ID: 2, LineNumber: 2, Description: "Beta", AggregateGroupID: 2 }),
        ],
        AggregateGroups: [
          { ID: 1, DisplayOrder: 1, Title: "First group", OrderedQuantityScaled: 10000, OrderedUoM: "Lot", ShowChildrenToCustomer: false, TotalMinor: 20000, DisplayUnitPriceMinor: 20000 },
          { ID: 2, DisplayOrder: 2, Title: "Second group", OrderedQuantityScaled: 10000, OrderedUoM: "Lot", ShowChildrenToCustomer: false, TotalMinor: 20000, DisplayUnitPriceMinor: 20000 },
        ],
      })
    );

    toggleMode();
    act(() => checkbox("Select line 1 for an aggregate line").click());
    act(() => checkbox("Select line 2 for an aggregate line").click());
    act(() => button("Group selected items into an aggregate line").click());

    const keys = Array.from(
      document.querySelectorAll("[data-aggregate-key]")
    ).map((node) => node.getAttribute("data-aggregate-key"));
    expect(keys).toEqual(["2"]);
    expect(
      document.querySelectorAll("[data-aggregate-key] [data-aggregate-key]")
    ).toHaveLength(0);
    const wrapper = document.querySelector('[data-aggregate-key="2"]');
    const descriptions = Array.from(
      wrapper?.querySelectorAll<HTMLInputElement>('input[aria-label^="Description for line"]') ?? []
    ).map((input) => input.value);
    expect(descriptions).toEqual(["Alpha", "Beta"]);
    expect(headerTotal("2")).toBe("$400.00");
    expect(document.body.textContent).not.toContain("First group");
    expect(document.body.textContent).not.toContain("Second group");
  });

  it("ungroups back to normal lines without deleting anything", () => {
    render(MIXED_DEPTH_ORDER);
    act(() => button("Ungroup Combined items").click());

    expect(document.querySelector("[data-aggregate-key]")).toBeNull();
    expect(document.querySelector("[data-aggregate-header-card]")).toBeNull();
    expect(lineInput("Description for line 1").value).toBe("Steel plate");
    expect(lineInput("Description for sub-item 1 of line 2").value).toBe(
      "Mounting and cable termination"
    );
    expect(
      document.querySelector('[aria-label="Remove split line 2"]')
    ).not.toBeNull();
  });

  it("prunes the header when its last child is removed", () => {
    render(MIXED_DEPTH_ORDER);

    act(() => button("Remove line 1").click());
    expect(headerTotal("0")).toBe("$80.00");
    expect(document.body.textContent).toContain("1 item in this aggregate");

    const removeSplit = Array.from(
      document.querySelectorAll<HTMLButtonElement>("button")
    ).find((candidate) =>
      (candidate.getAttribute("aria-label") ?? "").startsWith(
        "Remove split line"
      )
    );
    if (removeSplit === undefined) throw new Error("no remove split control");
    act(() => removeSplit.click());

    expect(document.querySelector("[data-aggregate-header-card]")).toBeNull();
    expect(document.querySelector("[data-aggregate-key]")).toBeNull();
    expect(document.body.textContent).toContain(
      "No lines yet. Add the customer's first line."
    );
  });

  it("keeps an aggregate when a reopened ghost replaces its display group", () => {
    render(
      aggregateOrder({
        Items: [
          aggregateDtoItem({
            ID: 1,
            AggregateGroupID: 1,
            Description: "Steel plate",
            QuantityScaled: 20000,
            UnitPriceMinor: 10000,
          }),
        ],
        AggregateGroups: [
          {
            ID: 1,
            DisplayOrder: 1,
            Title: "Combined items",
            OrderedQuantityScaled: 10000,
            OrderedUoM: "Lot",
            ShowChildrenToCustomer: false,
            TotalMinor: 20000,
            DisplayUnitPriceMinor: 20000,
          },
        ],
      }),
      [
        sourceRow({
          QuotationItemNo: 1,
          QuotationSubNo: null,
          LineLabel: "1",
          Description: "Steel plate",
          UoM: "Pc",
          QuantityScaled: 30000,
          UnitPriceMinor: 10000,
        }),
      ]
    );

    // The line no longer matches the quotation (quantity 2 vs 3), so the ghost
    // reopens under it; accepting the quotation replaces the display group.
    expect(headerTotal("0")).toBe("$200.00");
    act(() => button("Use quotation 1").click());

    const wrapper = document.querySelector('[data-aggregate-key="0"]');
    if (wrapper === null) throw new Error("aggregate membership was lost");
    expect(lineInput("Quantity for line 1").value).toBe("3");
    expect(headerTotal("0")).toBe("$300.00");
    expect(document.querySelector("[data-aggregate-header-card]")).not.toBeNull();
  });

  it("drops a ghost from the selection when it is ignored", () => {
    render(aggregateOrder({ Items: [] }), [sourceRow()]);

    toggleMode();
    act(() =>
      checkbox("Select quotation item 2 for an aggregate line").click()
    );
    expect(button("Group selected items into an aggregate line").disabled).toBe(
      false
    );

    act(() => button("Ignore quotation 2").click());
    expect(button("Group selected items into an aggregate line").disabled).toBe(
      true
    );
    expect(
      document.querySelector(
        'input[aria-label="Select quotation item 2 for an aggregate line"]'
      )
    ).toBeNull();
  });

  it("still shows a saved aggregate with no children so it can be ungrouped", () => {
    render(
      aggregateOrder({
        Items: [],
        AggregateGroups: [
          {
            ID: 1,
            DisplayOrder: 1,
            Title: "Combined items",
            OrderedQuantityScaled: 10000,
            OrderedUoM: "Lot",
            ShowChildrenToCustomer: false,
            TotalMinor: 0,
            DisplayUnitPriceMinor: null,
          },
        ],
      })
    );

    const header = document.querySelector("[data-aggregate-header-card]");
    expect(header).not.toBeNull();
    expect(document.body.textContent).toContain(
      "This saved aggregate has no lines left. Ungroup it to clear it."
    );
    act(() => button("Ungroup Combined items").click());
    expect(document.querySelector("[data-aggregate-header-card]")).toBeNull();
    expect(document.body.textContent).not.toContain("This saved aggregate");
  });

  it("carries no internal economics on the header and no header badge", () => {
    render(MIXED_DEPTH_ORDER);
    const header = document.querySelector("[data-aggregate-header-card]");
    if (header === null) throw new Error("no aggregate header");
    expect(header.textContent ?? "").not.toMatch(/cost|margin|variance|supplier/i);
    expect(header.querySelector("[data-variance-badge]")).toBeNull();
  });
});
