// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type {
  PurchaseOrderAggregateGroupState,
  PurchaseOrderLine,
  PurchaseOrderTotals,
} from "../purchaseOrder";
import type { CustomerPOHeader } from "../poCustomerView";
import { PurchaseOrderCustomerView } from "./PurchaseOrderCustomerView";

// Issue #654: the customer view card.
//
// The card renders the same projection the print document carries, so staff see
// exactly what the customer will see. It holds no figure of its own: the totals
// are the purchase order's, and the aggregate toggle is the one stored on the
// aggregate, not a second switch in this card.
//
// The print opens a blank window, writes the generated HTML and calls print.
// It never calls the server; a blocked popup is a message, never a silent
// failure.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const HEADER: CustomerPOHeader = {
  poNumber: "PO-654",
  customerName: "PT. Buyer",
  poDate: "2026-11-20",
  receivedDate: "2026-11-21",
  requestedDeliveryDate: null,
  quoteNumber: "1502260100",
  sourceRevision: 3,
  paymentTerms: "30 days",
  incoterms: "CIF",
};

const TOTALS: PurchaseOrderTotals = {
  grossMinor: 2500,
  rowDiscountMinor: 0,
  subtotalMinor: 2500,
  discountMinor: 0,
  afterDiscountMinor: 2500,
  ppnMinor: 0,
  grandTotalMinor: 2500,
};

function line(overrides: Partial<PurchaseOrderLine> = {}): PurchaseOrderLine {
  return {
    key: 1,
    clientKey: "client:1",
    description: "Widget",
    quantity: "2",
    uom: "Pc",
    unitPrice: "$10.00",
    discountEnabled: false,
    discountPercent: "0",
    quotationItemNo: 1,
    quotationSubNo: null,
    lineKind: "quotation",
    groupKey: "",
    parentDescription: "",
    descriptionLines: [],
    aggregateKey: 0,
    ...overrides,
  };
}

function group(
  overrides: Partial<PurchaseOrderAggregateGroupState> = {}
): PurchaseOrderAggregateGroupState {
  return {
    key: 0,
    title: "Combined items",
    orderedQuantity: "2",
    orderedUoM: "Lot",
    showChildrenToCustomer: false,
    ...overrides,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const DEFAULT_LINES: PurchaseOrderLine[] = [
  line(),
  line({ key: 2, description: "Spare", quantity: "1", unitPrice: "$5.00" }),
];

function renderCard(props: {
  lines?: PurchaseOrderLine[];
  groups?: PurchaseOrderAggregateGroupState[];
  totals?: PurchaseOrderTotals | null;
}) {
  act(() => {
    root.render(
      <PurchaseOrderCustomerView
        header={HEADER}
        lines={props.lines ?? DEFAULT_LINES}
        aggregateGroups={props.groups ?? [group()]}
        totals={props.totals === undefined ? TOTALS : props.totals}
        currency={USD}
      />
    );
  });
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

describe("PurchaseOrderCustomerView", () => {
  it("shows the aggregate title only and never a child description by default", () => {
    renderCard({});
    act(() => button("Expand customer view").click());
    const text = document.body.textContent ?? "";
    expect(text).toContain("Customer view");
    expect(text).toContain("Combined items");
    expect(text).not.toContain("Widget");
    expect(text).not.toContain("Spare");
    expect(text).toContain("$25.00");
  });

  it("shows the child tree when the aggregate's stored toggle says so", () => {
    renderCard({ groups: [group({ showChildrenToCustomer: true })] });
    act(() => button("Expand customer view").click());
    expect(document.body.textContent).toContain("Widget");
    expect(document.body.textContent).toContain("$20.00");
  });

  it("collapses and expands again without losing the projection", () => {
    renderCard({});
    expect(document.querySelector("table")).toBeNull();
    act(() => button("Expand customer view").click());
    expect(document.querySelector("table")).not.toBeNull();
    act(() => button("Collapse customer view").click());
    expect(document.querySelector("table")).toBeNull();
  });

  it("prints the generated document in a new window without calling the server", () => {
    const written: string[] = [];
    const fakeWindow = {
      document: {
        open: vi.fn(),
        write: (html: string) => written.push(html),
        close: vi.fn(),
      },
      focus: vi.fn(),
      print: vi.fn(),
    };
    const open = vi
      .spyOn(window, "open")
      .mockReturnValue(fakeWindow as unknown as Window);
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    renderCard({});

    act(() => button("Print customer view").click());

    expect(open).toHaveBeenCalledWith("", "_blank");
    expect(written).toHaveLength(1);
    expect(written[0]).toContain("PO-654");
    expect(written[0]).toContain("Combined items");
    expect(written[0]).not.toContain("Widget");
    expect(written[0]).not.toContain("Spare");
    expect(fakeWindow.print).toHaveBeenCalledTimes(1);
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("print dialog");
  });

  it("shows a message when the print window is blocked, and prints nothing", () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    renderCard({});

    act(() => button("Print customer view").click());

    expect(open).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("blocked");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("shows no supplier cost, margin, variance, quoted baseline or remaining quantity", () => {
    renderCard({
      lines: [
        line({ quotedUnitPriceMinor: 900, remainingQuantityScaled: 5000 }),
        line({ key: 2, description: "Spare", unitPrice: "$5.00" }),
      ],
      groups: [group({ showChildrenToCustomer: true })],
    });
    act(() => button("Expand customer view").click());
    const text = (document.body.textContent ?? "").toLowerCase();
    for (const word of ["cost", "margin", "variance", "quoted", "remaining"]) {
      expect(text).not.toContain(word);
    }
  });

  it("refuses to project or print while a line cannot be totalled", () => {
    renderCard({ totals: null });
    act(() => button("Expand customer view").click());
    expect(document.body.textContent).toContain("once every line can be totalled");
    expect(document.querySelector("table")).toBeNull();
    expect(button("Print customer view").disabled).toBe(true);
  });
});
