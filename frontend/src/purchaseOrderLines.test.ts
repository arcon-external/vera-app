import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import type { PurchaseOrderLine } from "./purchaseOrder";
import {
  addSubLine,
  editorItemsFromGroups,
  effectiveGroupKey,
  effectiveParentDescription,
  groupPurchaseOrderLines,
  moveSubLine,
  patchGroupParentDescription,
  patchLine,
  patchSubLine,
  removeGroup,
  splitPlainLine,
} from "./purchaseOrderLines";

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function line(overrides: Partial<PurchaseOrderLine> = {}): PurchaseOrderLine {
  return {
    key: 1,
    clientKey: "client:test-1",
    description: "Widget",
    quantity: "1",
    uom: "Pc",
    unitPrice: "$1,250.50",
    discountEnabled: false,
    discountPercent: "0",
    quotationItemNo: null,
    quotationSubNo: null,
    groupKey: "",
    parentDescription: "",
    descriptionLines: [],
    ...overrides,
  };
}

describe("PO line grouping (#573)", () => {
  it("keeps plain lines separate and in document order", () => {
    const lines = [
      line({ key: 1 }),
      line({ key: 2, description: "Mounting" }),
      line({ key: 3, description: "Freight" }),
    ];
    const groups = groupPurchaseOrderLines(lines);
    expect(groups.map((group) => group.key)).toEqual([1, 2, 3]);
    expect(groups.every((group) => group.groupKey === "")).toBe(true);
  });

  it("groups rows by their stored key at the first row's position", () => {
    const lines = [
      line({ key: 1, groupKey: "g:panel", parentDescription: "Panel" }),
      line({ key: 2, description: "Plain" }),
      line({ key: 3, groupKey: "g:panel", parentDescription: "Panel" }),
    ];
    const groups = groupPurchaseOrderLines(lines);
    expect(groups).toHaveLength(2);
    expect(groups[0].key).toBe(1);
    expect(groups[0].lines.map((row) => row.key)).toEqual([1, 3]);
    expect(groups[1].key).toBe(2);
  });

  it("groups a pre-migration priced part under its source item", () => {
    const lines = [
      line({ key: 1, quotationItemNo: 3, quotationSubNo: 1 }),
      line({ key: 2, quotationItemNo: 3, quotationSubNo: 2 }),
    ];
    expect(effectiveGroupKey(lines[0])).toBe("q:3");
    const groups = groupPurchaseOrderLines(lines);
    expect(groups).toHaveLength(1);
    expect(groups[0].lines.map((row) => row.key)).toEqual([1, 2]);
    expect(effectiveParentDescription(groups[0])).toBe("Quotation item 3");
  });
});

describe("PO editor view model (#573)", () => {
  it("maps a plain line's display money back to raw minor units", () => {
    const items = editorItemsFromGroups(
      groupPurchaseOrderLines([
        line({
          key: 7,
          quantity: "2.5",
          descriptionLines: ["With bracket"],
        }),
      ]),
      USD
    );
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      key: 7,
      description: "Widget",
      bullets: ["With bracket"],
      qty: "2.5",
      uom: "Pc",
      unitPrice: "125050",
      marginEnabled: false,
      discountEnabled: false,
      subItems: [],
    });
  });

  it("maps a split group to a parent row and priced sub-items", () => {
    const groups = groupPurchaseOrderLines([
      line({
        key: 1,
        groupKey: "g:panel",
        parentDescription: "LV panel",
        description: "Panel",
        descriptionLines: ["IP54"],
        discountEnabled: true,
        discountPercent: "5",
      }),
      line({
        key: 2,
        groupKey: "g:panel",
        parentDescription: "LV panel",
        description: "Wiring",
        quantity: "2",
        unitPrice: "$50.00",
        descriptionLines: ["Two technicians"],
      }),
    ]);
    const items = editorItemsFromGroups(groups, USD);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      key: 1,
      description: "LV panel",
      qty: "1",
      unitPrice: "",
      subItems: [
        {
          id: 1,
          description: "Panel",
          unitCost: "125050",
          discountEnabled: true,
          discountPercent: "5",
          bullets: ["IP54"],
        },
        {
          id: 2,
          description: "Wiring",
          qty: "2",
          unitCost: "5000",
          bullets: ["Two technicians"],
        },
      ],
    });
  });
});

describe("PO line transforms (#573)", () => {
  it("splits a plain line without losing its price or match", () => {
    const lines = [
      line({
        key: 1,
        quotationItemNo: 2,
        quotationSubNo: null,
        discountEnabled: true,
        discountPercent: "10",
      }),
    ];
    const split = splitPlainLine(
      lines,
      1,
      "g:new",
      line({ key: 2, clientKey: "client:test-2", unitPrice: "0" })
    );
    expect(split).toHaveLength(2);
    expect(split[0]).toMatchObject({
      key: 1,
      groupKey: "g:new",
      parentDescription: "Widget",
      quotationItemNo: 2,
      unitPrice: "$1,250.50",
      discountEnabled: true,
    });
    expect(split[1]).toMatchObject({
      key: 2,
      groupKey: "g:new",
      parentDescription: "Widget",
      quotationItemNo: null,
    });
  });

  it("adds a sub-item after the group's last row", () => {
    const lines = [
      line({ key: 1, groupKey: "g:panel", parentDescription: "Panel" }),
      line({ key: 2, groupKey: "g:panel", parentDescription: "Panel" }),
      line({ key: 3, description: "After" }),
    ];
    const added = addSubLine(
      lines,
      "g:panel",
      line({ key: 4, clientKey: "client:test-4", description: "Extra" })
    );
    expect(added.map((row) => row.key)).toEqual([1, 2, 4, 3]);
    expect(added[2]).toMatchObject({
      groupKey: "g:panel",
      parentDescription: "Panel",
    });
  });

  it("moves a sub-item only inside its group", () => {
    const lines = [
      line({ key: 1, groupKey: "g:panel" }),
      line({ key: 2, groupKey: "g:panel" }),
      line({ key: 3, description: "Plain" }),
    ];
    const moved = moveSubLine(lines, "g:panel", 1, 1);
    expect(moved.map((row) => row.key)).toEqual([2, 1, 3]);
    expect(moveSubLine(lines, "g:panel", 2, 1)).toBe(lines);
  });

  it("removes every row of a group, and only the row of a plain line", () => {
    const lines = [
      line({ key: 1, groupKey: "g:panel" }),
      line({ key: 2, groupKey: "g:panel" }),
      line({ key: 3 }),
    ];
    expect(removeGroup(lines, "g:panel", 1).map((row) => row.key)).toEqual([3]);
    expect(removeGroup(lines, "", 3).map((row) => row.key)).toEqual([1, 2]);
  });

  it("patches a plain line and converts edited money back to display", () => {
    const lines = [line({ key: 1 })];
    const patched = patchLine(lines, 1, "unitPrice", "130000", USD);
    expect(patched[0].unitPrice).toBe("$1,300.00");
    const quantity = patchLine(patched, 1, "qty", "2.25", USD);
    expect(quantity[0].quantity).toBe("2.25");
    const bullets = patchLine(quantity, 1, "bullets", ["A", "B"], USD);
    expect(bullets[0].descriptionLines).toEqual(["A", "B"]);
  });

  it("patches a sub-item and converts its money back to display", () => {
    const lines = [line({ key: 1, groupKey: "g:panel" })];
    const patched = patchSubLine(lines, 1, { unitCost: "9900", qty: "3" }, USD);
    expect(patched[0].unitPrice).toBe("$99.00");
    expect(patched[0].quantity).toBe("3");
  });

  it("copies a parent title onto every row of the group", () => {
    const lines = [
      line({ key: 1, groupKey: "g:panel", parentDescription: "Old" }),
      line({ key: 2, groupKey: "g:panel", parentDescription: "Old" }),
      line({ key: 3 }),
    ];
    const patched = patchGroupParentDescription(lines, "g:panel", "New title");
    expect(patched[0].parentDescription).toBe("New title");
    expect(patched[1].parentDescription).toBe("New title");
    expect(patched[2].parentDescription).toBe("");
  });
});
