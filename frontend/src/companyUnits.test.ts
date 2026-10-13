import {
  UOM_LABEL_MAX,
  UOM_VALUE_MAX,
  addCompanyUnit,
  moveCompanyUnit,
  patchCompanyUnit,
  removeCompanyUnit,
  unitValueShaped,
  validateCompanyUnits,
} from "./companyUnits";
import type { CompanyUnitOption } from "./companyUnits";

// These edit-time rules mirror backend/store/units.go checkCompanyUnits. The
// frontend list must be refused the same way the store would, before a round
// trip that could only bounce. server: value required / ≤12 / shaped / label
// required / ≤20 / case-insensitive-distinct / canonical stays.

const LOT: CompanyUnitOption = { value: "Lot", label: "Lot" };

describe("unitValueShaped", () => {
  it.each(["Lot", "m2", "PC", "kg/m3", "Sq-Ft"])("accepts %s", (value) => {
    expect(unitValueShaped(value)).toBe(true);
  });

  it.each(["two words", "m²", "m³", "unit#1", "", "lot "])("rejects %s", (value) => {
    expect(unitValueShaped(value)).toBe(false);
  });
});

describe("validateCompanyUnits", () => {
  it("accepts a shaped list that keeps the canonical", () => {
    expect(validateCompanyUnits([LOT, { value: "m2", label: "m²" }], "Lot")).toBeNull();
  });

  it("refuses an empty list", () => {
    expect(validateCompanyUnits([], "Lot")).toBe("At least one unit of measure is required.");
  });

  it("names the row of a missing value or label", () => {
    expect(validateCompanyUnits([LOT, { value: "", label: "x" }], "Lot")).toBe("Unit 2: value is required.");
    expect(validateCompanyUnits([LOT, { value: "Day", label: "  " }], "Lot")).toBe("Unit 2: label is required.");
  });

  it("caps the value at UOM_VALUE_MAX and the label at UOM_LABEL_MAX", () => {
    expect(validateCompanyUnits([{ value: "a".repeat(UOM_VALUE_MAX + 1), label: "x" }], "")).toContain("too long");
    expect(validateCompanyUnits([{ value: "Lot", label: "b".repeat(UOM_LABEL_MAX + 1) }], "Lot")).toContain("too long");
  });

  it("refuses an unshaped value", () => {
    expect(validateCompanyUnits([{ value: "m²", label: "m²" }], "")).toContain("not shaped like a unit label");
  });

  it("refuses a value that duplicates an earlier one case-insensitively", () => {
    expect(validateCompanyUnits([{ value: "Pc", label: "Pc" }, { value: "PC", label: "PC" }], "Pc")).toContain(
      "duplicates an earlier unit"
    );
  });

  it("refuses a replacement that drops the canonical unit", () => {
    expect(validateCompanyUnits([{ value: "Pc", label: "Pc" }], "Lot")).toBe(
      'The canonical unit "Lot" must stay on the list.'
    );
  });

  it("leaves an empty canonical to the profile save path", () => {
    // The canonical is saved separately; validateCompanyUnits only guards it
    // when a non-empty value is given.
    expect(validateCompanyUnits([LOT], "")).toBeNull();
  });

  it("trims before it checks the canonical membership", () => {
    expect(validateCompanyUnits([LOT, { value: "m2", label: "m²" }], "  Lot  ")).toBeNull();
  });
});

describe("unit-list editing ops", () => {
  const list: CompanyUnitOption[] = [
    { value: "Lot", label: "Lot" },
    { value: "m2", label: "m²" },
    { value: "Pc", label: "Pc" },
  ];

  it("addCompanyUnit appends one blank row without mutating the input", () => {
    const next = addCompanyUnit(list);
    expect(next).toHaveLength(list.length + 1);
    expect(next[next.length - 1]).toEqual({ value: "", label: "" });
    expect(list).toHaveLength(3); // unchanged
  });

  it("removeCompanyUnit drops only the named row", () => {
    expect(removeCompanyUnit(list, 1)).toEqual([list[0], list[2]]);
    expect(removeCompanyUnit(list, 99)).toHaveLength(3); // out of range is a no-op
  });

  it("patchCompanyUnit changes one field and leaves the rest", () => {
    const next = patchCompanyUnit(list, 1, { label: "m2square" });
    expect(next[1]).toEqual({ value: "m2", label: "m2square" });
    expect(next[0]).toBe(list[0]); // untouched rows keep identity
  });

  it("moveCompanyUnit swaps neighbours and clamps at the edges", () => {
    expect(moveCompanyUnit(list, 0, 1)).toEqual([list[1], list[0], list[2]]);
    expect(moveCompanyUnit(list, 2, -1)).toEqual([list[0], list[2], list[1]]);
    expect(moveCompanyUnit(list, 0, -1)).toBe(list); // already first
    expect(moveCompanyUnit(list, 2, 1)).toBe(list); // already last
  });
});
