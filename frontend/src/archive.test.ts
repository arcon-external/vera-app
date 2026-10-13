import type { CurrencyInfo } from "./format";
import { resolveCurrencyEntry, resolveLiveEntry } from "./archive";

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

// Issue #390: the LIVE profile entry, not a revision's. resolveCurrencyEntry
// above takes an explicit fallback and always answers, so a caller handing it
// the IDR entry under a USD profile prints Rp over USD minor units. The live
// resolver refuses instead: null means the entry cannot be known.
const CATALOGUE: CurrencyInfo[] = [IDR, USD, EUR];

describe("resolveLiveEntry", () => {
  it("returns null for a non-IDR profile when the catalogue has not loaded", () => {
    // The #390 regression: the resolution line read `found ??
    // IDR_FALLBACK_ENTRY`, so a USD profile with a failed catalogue read
    // rendered Rp 150,000 over integers stored as USD minor units -- the 100x
    // misread through the failure path, and not display-only either, because
    // the same entry parsed every pasted cell. Removing this assertion
    // reintroduces #390.
    expect(resolveLiveEntry(null, "USD", IDR)).toBeNull();
    expect(resolveLiveEntry(null, "USD", IDR)).not.toBe(IDR);
    expect(resolveLiveEntry(undefined, "USD", IDR)).toBeNull();
  });

  it("keeps the IDR fallback for the IDR path, with no catalogue at all", () => {
    // Every pre-currency integer was written in IDR, so this fallback is
    // honest and today's offline behaviour is preserved byte-identically.
    // Blank and missing mean IDR, exactly as normalizeCurrencyCode reads them.
    expect(resolveLiveEntry(null, "IDR", IDR)).toBe(IDR);
    expect(resolveLiveEntry(null, " idr ", IDR)).toBe(IDR);
    expect(resolveLiveEntry(null, "", IDR)).toBe(IDR);
    expect(resolveLiveEntry(null, undefined, IDR)).toBe(IDR);
  });

  it("resolves the profile code from the loaded catalogue", () => {
    expect(resolveLiveEntry(CATALOGUE, "USD", IDR)).toBe(USD);
    expect(resolveLiveEntry(CATALOGUE, " usd ", IDR)).toBe(USD);
    expect(resolveLiveEntry(CATALOGUE, "EUR", IDR)).toBe(EUR);
  });

  it("returns null for any code the catalogue does not carry", () => {
    // No allowlist: an unlisted code is not a currency Vera may guess at.
    expect(resolveLiveEntry(CATALOGUE, "JPY", IDR)).toBeNull();
    expect(resolveLiveEntry(CATALOGUE, "", IDR)).toBe(IDR);
  });

  it("returns null for every non-IDR catalogue code while the catalogue is absent", () => {
    // The property, not the example: whatever the catalogue would answer, it
    // cannot answer while it is gone.
    for (const entry of CATALOGUE) {
      if (entry.code === "IDR") continue;
      expect(resolveLiveEntry(null, entry.code, IDR)).toBeNull();
    }
  });
});
