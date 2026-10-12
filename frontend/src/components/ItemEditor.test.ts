import type { CurrencyInfo } from "../format";
import { parseMoneyKeystroke } from "./ItemEditor";

// Issue #336 Wave B U1: the editor's money inputs display through
// formatMoneyInput(entry, storedMinorDigits) and must parse keystrokes back
// into that same stored semantic (minor-unit digit strings). A keystroke
// parser with major-unit semantics (parseMoneyInput) would 100x digit
// entry: typing "5" would store 500 instead of 5.

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

const EUR: CurrencyInfo = {
  code: "EUR",
  exponent: 2,
  symbol: "€",
  position: "after",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: true,
};

describe("parseMoneyKeystroke", () => {
  it("keeps digit-by-digit entry in minor units (no 100x)", () => {
    // parseMoneyInput(USD, "5") is 500; the keystroke path must store "5".
    expect(parseMoneyKeystroke(USD, "5")).toBe("5");
    expect(parseMoneyKeystroke(USD, "150000")).toBe("150000");
    expect(parseMoneyKeystroke(IDR, "15000000")).toBe("15000000");
  });

  it("leaves empty empty so untouched fields never read as zero", () => {
    expect(parseMoneyKeystroke(USD, "")).toBe("");
  });

  it("reads half-typed decimals with major-unit semantics", () => {
    // Stripping non-digits would store "155" ($1.55); the user typed $15.50.
    expect(parseMoneyKeystroke(USD, "15.5")).toBe("1550");
    expect(parseMoneyKeystroke(USD, "15.")).toBe("1500");
    expect(parseMoneyKeystroke(EUR, "1,5")).toBe("150");
  });

  it("round-trips fed-back formatted output instead of zeroing it", () => {
    expect(parseMoneyKeystroke(USD, "$1,500.00")).toBe("150000");
    expect(parseMoneyKeystroke(IDR, "Rp15.000.000")).toBe("15000000");
    expect(parseMoneyKeystroke(EUR, "1.234,56 €")).toBe("123456");
  });

  it("reads pasted grouped figures with major-unit semantics", () => {
    expect(parseMoneyKeystroke(USD, "15,000")).toBe("1500000");
    expect(parseMoneyKeystroke(IDR, "15.000")).toBe("15000");
  });

  it("never silently zeroes: garbage falls back to its digits", () => {
    // parseMoneyInput refuses "15,00.00" (misgrouped); storing "0" would
    // erase the user's figure, so the digit fallback (the old behaviour)
    // keeps what was typed.
    expect(parseMoneyKeystroke(USD, "15,00.00")).not.toBe("0");
    expect(parseMoneyKeystroke(USD, "abc")).toBe("");
  });

  it("stores a genuine zero as zero", () => {
    expect(parseMoneyKeystroke(USD, "0")).toBe("0");
    expect(parseMoneyKeystroke(USD, "0.00")).toBe("0");
    expect(parseMoneyKeystroke(IDR, "0")).toBe("0");
  });
});
