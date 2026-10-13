import { describe, expect, it } from "vitest";
import { parsePasteMoneyCell, parsePastedLeadTimeDays } from "./App";
import type { CurrencyInfo } from "./format";

// The paste handlers in App.tsx route every pasted money cell through
// parseMoneyInput with the LIVE profile currency. These tests pin the
// stored minor-unit strings for both conventions plus the refusal cases:
// a paste that cannot be parsed stores "" (the field renders empty) and
// never a silently wrong number.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};
const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

describe("parsePasteMoneyCell", () => {
  it("parses a formatted USD figure to minor units", () => {
    expect(parsePasteMoneyCell(USD, "$1,500.00")).toBe("150000");
  });

  it("parses a whole-dollar USD figure to minor units, not dollars", () => {
    // The old strip-non-digits path stored "1500" here ($15.00): 100x off.
    expect(parsePasteMoneyCell(USD, "$1,500")).toBe("150000");
  });

  it("parses a grouped IDR figure to whole rupiah", () => {
    expect(parsePasteMoneyCell(IDR, "15.000.000")).toBe("15000000");
  });

  it("stores empty for an empty cell", () => {
    expect(parsePasteMoneyCell(USD, "")).toBe("");
    expect(parsePasteMoneyCell(USD, "   ")).toBe("");
  });

  it("never stores a zero for garbage", () => {
    expect(parsePasteMoneyCell(USD, "abc")).toBe("");
    expect(parsePasteMoneyCell(IDR, "abc")).toBe("");
  });

  it("refuses a figure from another convention instead of converting it", () => {
    // There is no rate to convert with: IDR grouping under a USD profile
    // is a refused parse, not $150,000.00.
    expect(parsePasteMoneyCell(USD, "15.000.000")).toBe("");
  });

  it("keeps a genuine zero", () => {
    expect(parsePasteMoneyCell(USD, "0")).toBe("0");
    expect(parsePasteMoneyCell(USD, "$0.00")).toBe("0");
    expect(parsePasteMoneyCell(IDR, "0")).toBe("0");
  });
});

describe("parsePastedLeadTimeDays", () => {
  it("maps a legacy days-only cell into the days segment", () => {
    expect(parsePastedLeadTimeDays("60")).toBe(60);
    expect(parsePastedLeadTimeDays("90")).toBe(90);
  });

  it("reads empty and unreadable cells as unstated, not the old default", () => {
    // The paste used to fall back to 60 when the cell held nothing; an empty
    // cell now means no lead time was stated.
    expect(parsePastedLeadTimeDays("")).toBe(0);
    expect(parsePastedLeadTimeDays(undefined)).toBe(0);
    expect(parsePastedLeadTimeDays("abc")).toBe(0);
  });

  it("strips non-digits and clamps to the uint8 range", () => {
    expect(parsePastedLeadTimeDays("60 days")).toBe(60);
    expect(parsePastedLeadTimeDays("999")).toBe(255);
  });
});
