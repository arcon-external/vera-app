import { describe, expect, it } from "vitest";
import { newItem, normalizeUoM } from "./App";

// Issue #337: the editor's unit-of-measure defaults are the company's served
// list and canonical unit, never a hardcoded "Lot". These pins fail the moment
// a default is reverted to "Lot" or the paste fallback ignores the served list.

// A representative served list, value + display label, in display order.
const SERVED: { value: string; label: string }[] = [
  { value: "Lot", label: "Lot" },
  { value: "Pc", label: "Pc" },
  { value: "meter", label: "meter" },
  { value: "m2", label: "m²" },
  { value: "Week", label: "Week" },
];

describe("newItem default unit", () => {
  it("carries the canonical unit it is given, not a hardcoded one", () => {
    // Reverting newItem to uom:"Lot" fails the "Week" line below.
    expect(newItem(0, "Week").uom).toBe("Week");
    expect(newItem(3, "Pc").uom).toBe("Pc");
  });

  it("keeps its key and the rest of the empty-line shape", () => {
    const item = newItem(7, "m2");
    expect(item.key).toBe(7);
    expect(item.qty).toBe("1");
    expect(item.unitPrice).toBe("");
    expect(item.subItems).toEqual([]);
  });
});

describe("normalizeUoM paste fallback", () => {
  it("falls an empty or unrecognised cell back to the canonical unit, not 'Lot'", () => {
    // Reverting the fallback to "Lot" fails the "Week" expectations below.
    expect(normalizeUoM("", SERVED, "Week")).toBe("Week");
    expect(normalizeUoM("   ", SERVED, "Week")).toBe("Week");
    expect(normalizeUoM("not a unit", SERVED, "Week")).toBe("Week");
    // A sentence where a label belongs (the paste-column-shift bug) folds to the
    // canonical rather than a value the select could not even display.
    expect(normalizeUoM("installation of forty units", SERVED, "Week")).toBe("Week");
  });

  it("folds an exact value case-insensitively onto the served spelling", () => {
    expect(normalizeUoM("pc", SERVED, "Week")).toBe("Pc");
    expect(normalizeUoM("LOT", SERVED, "Week")).toBe("Lot");
    expect(normalizeUoM("week", SERVED, "Week")).toBe("Week");
  });

  it("folds the area/volume and plural aliases onto the served values", () => {
    expect(normalizeUoM("m²", SERVED, "Week")).toBe("m2");
    expect(normalizeUoM("m³", SERVED, "Week")).toBe("Week"); // m3 not on this list → canonical
    expect(normalizeUoM("sqm", SERVED, "Week")).toBe("m2");
    expect(normalizeUoM("meters", SERVED, "Week")).toBe("meter");
    expect(normalizeUoM("mtr", SERVED, "Week")).toBe("meter");
  });

  it("resolves an alias target against the served list, not blindly", () => {
    // 'pcs' folds onto 'Pc'; that target is on the list, so it lands on it.
    expect(normalizeUoM("pcs", SERVED, "Week")).toBe("Pc");
    // With Pc absent from the list, the fold falls through to the canonical
    // rather than storing a unit the company removed.
    const withoutPc = SERVED.filter((unit) => unit.value !== "Pc");
    expect(normalizeUoM("pcs", withoutPc, "Week")).toBe("Week");
    expect(normalizeUoM("mtr", withoutPc, "Week")).toBe("meter");
  });

  it("returns the served spelling, so m2 stays m2 even though its label is m²", () => {
    expect(normalizeUoM("M2", SERVED, "Week")).toBe("m2");
  });
});
