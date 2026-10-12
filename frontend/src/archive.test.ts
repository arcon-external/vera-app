import type { CurrencyInfo } from "./format";
import { resolveCurrencyEntry } from "./archive";

// Issue #336 Wave B U1: archive rows render in their REVISION currency. The
// row carries a code string; the entry comes from the catalogue fetched over
// GetCurrencyCatalogue(), with the live profile entry as the fallback when
// the code is missing or unknown (the server normalises codes, so unknown
// means a catalogue that has not loaded, not a third currency).

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
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

describe("resolveCurrencyEntry", () => {
  it("resolves a revision code to its catalogue entry, not the live one", () => {
    // An IDR-live office opening a USD revision must see the USD entry:
    // resolving to live would print Rp around a dollar bare figure.
    expect(resolveCurrencyEntry([IDR, USD, EUR], "USD", IDR)).toBe(USD);
    expect(resolveCurrencyEntry([IDR, USD, EUR], "EUR", IDR)).toBe(EUR);
  });

  it("matches codes case-insensitively with surrounding space", () => {
    expect(resolveCurrencyEntry([IDR, USD], " usd ", IDR)).toBe(USD);
  });

  it("falls back to the live entry when the code is missing", () => {
    expect(resolveCurrencyEntry([IDR, USD], "", USD)).toBe(USD);
    expect(resolveCurrencyEntry([IDR, USD], undefined, USD)).toBe(USD);
  });

  it("falls back to the live entry when the catalogue has not loaded", () => {
    expect(resolveCurrencyEntry(null, "USD", IDR)).toBe(IDR);
    expect(resolveCurrencyEntry(undefined, "USD", USD)).toBe(USD);
  });

  it("falls back to the live entry for an unknown code", () => {
    expect(resolveCurrencyEntry([IDR, USD], "XX", USD)).toBe(USD);
  });
});
