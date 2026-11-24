import { describe, expect, it } from "vitest";
import type { EditorItem, EditorSubItem } from "./components/ItemEditor";
import {
  boughtLeafKey,
  boughtQuantitiesByLeaf,
  buildQuotationItemInputs,
  nextItemNo,
  numberMissingItems,
  takeItemNo,
  takeSubNos,
} from "./quotationItemIdentity";

// Issue #648: a quotation line and each sub-item carry a stable number that is
// the allocation identity a purchase order names when it buys. The number must
// survive removal and reordering, and a new line must never take a number a
// bought line already used. These tests drive the editor's own state helpers
// and the exact payload builder Prepare calls.

function sub(no: number, description: string): EditorSubItem {
  return {
    id: no,
    no,
    description,
    qty: "1",
    uom: "Lot",
    unitCost: "1000",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    bullets: [],
  };
}

function line(
  no: number,
  key: number,
  description: string,
  subItems: EditorSubItem[] = []
): EditorItem {
  return {
    key,
    no,
    description,
    bullets: [],
    qty: "1",
    uom: "Lot",
    unitPrice: subItems.length === 0 ? "1000" : "0",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    subItems,
    showSubPrices: false,
  };
}

describe("quotation item identity (#648)", () => {
  it("carries the stable line and sub-item numbers into the Prepare payload", () => {
    const payload = buildQuotationItemInputs(
      [line(1, 0, "Pump", [sub(1, "Casing")]), line(2, 1, "Installation")],
      "Lot"
    );
    expect(payload.map((item) => item.No)).toEqual([1, 2]);
    expect(payload[0].SubItems.map((subItem) => subItem.No)).toEqual([1]);
    // The builder is the real payload path, not a parallel one: the pricing
    // fields Prepare has always sent are still there.
    expect(payload[0].Qty).toBe(1);
    expect(payload[1].Qty).toBe(1);
    expect(payload[0].Description).toBe("Pump");
  });

  it("still names lines 2 and 3 after line 1 of 3 is removed", () => {
    const items = [line(1, 0, "A"), line(2, 1, "B"), line(3, 2, "C")];
    const afterRemoval = items.filter((item) => item.key !== 0);
    const payload = buildQuotationItemInputs(afterRemoval, "Lot");
    expect(payload.map((item) => [item.No, item.Description])).toEqual([
      [2, "B"],
      [3, "C"],
    ]);
  });

  it("gives a newly added line the loaded maximum + 1, never a removed number", () => {
    const items = [line(1, 0, "A"), line(2, 1, "B"), line(3, 2, "C")];
    const counter = { next: nextItemNo(items) };
    expect(counter.next).toBe(4);
    // The line number the new line receives is taken before the removal is
    // even considered, so removing line 3 cannot release its number.
    const removed = items.filter((item) => item.key !== 2);
    const addedNo = takeItemNo(counter);
    const withAdded = [...removed, line(addedNo, 3, "D")];
    expect(withAdded[withAdded.length - 1].no).toBe(4);
    expect(buildQuotationItemInputs(withAdded, "Lot").map((item) => item.No)).toEqual([
      1, 2, 4,
    ]);
  });

  it("starts an empty editor at 1 and a loaded revision one past its highest number", () => {
    expect(nextItemNo([])).toBe(1);
    expect(nextItemNo([line(1, 0, "A"), line(3, 1, "C")])).toBe(4);
  });

  it("advances sub-item numbers past a removed sub and never repeats one", () => {
    const composed = line(2, 0, "Package", [sub(1, "One"), sub(2, "Two"), sub(3, "Three")]);
    // Removing sub 3 leaves the high-water mark on the line, exactly as the
    // editor does; the next sub must be 4, not the removed 3.
    const afterRemoval: EditorItem = {
      ...composed,
      subItems: composed.subItems.filter((subItem) => subItem.no !== 3),
      nextSubNo: 4,
    };
    const taken = takeSubNos(afterRemoval, 2);
    expect(taken.nos).toEqual([4, 5]);
    expect(taken.nextSubNo).toBe(6);
  });

  it("numbers each line's sub-items independently", () => {
    const first = line(1, 0, "A", [sub(1, "One"), sub(2, "Two")]);
    const second = line(2, 1, "B");
    expect(takeSubNos(first, 1).nos).toEqual([3]);
    expect(takeSubNos(second, 1).nos).toEqual([1]);
  });

  it("numbers a draft written before identity existed by position, keeping numbered lines", () => {
    const old = [line(0, 0, "A", [sub(0, "part")]), line(0, 1, "B")];
    const numbered = numberMissingItems(old);
    expect(numbered.items.map((item) => item.no)).toEqual([1, 2]);
    expect(numbered.items[0].subItems.map((subItem) => subItem.no)).toEqual([1]);
    expect(numbered.nextItemNo).toBe(3);

    const mixed = numberMissingItems([line(5, 0, "A"), line(0, 1, "B")]);
    expect(mixed.items.map((item) => item.no)).toEqual([5, 6]);
    expect(mixed.nextItemNo).toBe(7);
  });

  it("maps bought availability onto the line and sub-item keys", () => {
    const bought = boughtQuantitiesByLeaf([
      { quotationItemNo: 1, quotationSubNo: null, boughtQuantityScaled: 20000 },
      { quotationItemNo: 2, quotationSubNo: 3, boughtQuantityScaled: 25000 },
      { quotationItemNo: 3, quotationSubNo: null, boughtQuantityScaled: 0 },
    ]);
    expect(boughtLeafKey(1)).toBe("1:0");
    expect(bought.get(boughtLeafKey(1))).toBe(20000);
    expect(bought.get(boughtLeafKey(2, 3))).toBe(25000);
    // A leaf nothing was bought against has no entry: the badge must not
    // invent a zero for it.
    expect(bought.has(boughtLeafKey(3))).toBe(false);
  });
});
