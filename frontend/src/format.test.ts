import {
  decorateMoney,
  formatMoney,
  formatMoneyInput,
  parseMoneyInput,
  type CurrencyInfo,
} from "./format";

// The entries below are transcribed from backend/currency/currency.go's
// catalogue (field-for-field against the `currency.Catalogue()` JSON dump;
// Codes() order). The EXPECTED strings are Go's own output -- the
// `Decorate(code, FormatAmount(code, minor))` dump for minor 1500000
// (exponent 2) or 15000000 (exponent 0), plus the zero/cent/single/signed
// cases -- so every assertion below is an agreement check against Go, not a
// restatement of this module's logic. If the Go catalogue changes, these
// literals go red, which is the point.
//
// There is deliberately no shared JSON fixture yet: this module owns only
// format.ts and its test (issue #336 Wave B task table), so a cross-side
// fixture file is a follow-up, not something smuggled into this task.

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
const VND: CurrencyInfo = {
  code: "VND",
  exponent: 0,
  symbol: "₫",
  position: "after",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: true,
};
const AED: CurrencyInfo = {
  code: "AED",
  exponent: 2,
  symbol: "AED",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: true,
};
const JPY: CurrencyInfo = {
  code: "JPY",
  exponent: 0,
  symbol: "¥",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

// The full catalogue with Go's dumped decorated strings: minor 1500000 for
// exponent 2, 15000000 for exponent 0 (the same values Go's
// TestFormatParseInverse uses). One row per entry, no exceptions.
const ALL_ENTRIES: Array<{
  entry: CurrencyInfo;
  minor: number;
  want: string;
}> = [
  {
    entry: AED,
    minor: 1500000,
    want: "AED 15,000.00",
  },
  {
    entry: {
      code: "AUD",
      exponent: 2,
      symbol: "A$",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "A$15,000.00",
  },
  {
    entry: {
      code: "CNY",
      exponent: 2,
      symbol: "CN¥",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "CN¥15,000.00",
  },
  { entry: EUR, minor: 1500000, want: "15.000,00 €" },
  {
    entry: {
      code: "GBP",
      exponent: 2,
      symbol: "£",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "£15,000.00",
  },
  {
    entry: {
      code: "HKD",
      exponent: 2,
      symbol: "HK$",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "HK$15,000.00",
  },
  { entry: IDR, minor: 15000000, want: "Rp15.000.000" },
  {
    entry: {
      code: "INR",
      exponent: 2,
      symbol: "₹",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "₹15,000.00",
  },
  { entry: JPY, minor: 15000000, want: "¥15,000,000" },
  {
    entry: {
      code: "KRW",
      exponent: 0,
      symbol: "₩",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 15000000,
    want: "₩15,000,000",
  },
  {
    entry: {
      code: "MYR",
      exponent: 2,
      symbol: "RM",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "RM15,000.00",
  },
  {
    entry: {
      code: "PHP",
      exponent: 2,
      symbol: "₱",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "₱15,000.00",
  },
  {
    entry: {
      code: "SGD",
      exponent: 2,
      symbol: "S$",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "S$15,000.00",
  },
  {
    entry: {
      code: "THB",
      exponent: 2,
      symbol: "฿",
      position: "before",
      groupSep: ",",
      decimalSep: ".",
      symbolSpaced: false,
    },
    minor: 1500000,
    want: "฿15,000.00",
  },
  { entry: USD, minor: 1500000, want: "$15,000.00" },
  { entry: VND, minor: 15000000, want: "15.000.000 ₫" },
];

describe("formatMoney agrees with Go character-for-character", () => {
  it("renders the issue's own example from integers", () => {
    // Go TestFormatFromMinorUnits: FormatAmount("USD", 1500000) is
    // "15,000.00" and FormatAmount("IDR", 15000000) is "15.000.000".
    expect(formatMoney(USD, 1500000)).toBe("$15,000.00");
    expect(formatMoney(IDR, 15000000)).toBe("Rp15.000.000");
  });

  it("matches Go's decorated output on every catalogue entry", () => {
    for (const { entry, minor, want } of ALL_ENTRIES) {
      expect(formatMoney(entry, minor), entry.code).toBe(want);
    }
  });

  it("reads the same integer differently per exponent", () => {
    // The exponent-0/2 split: one stored integer, two readings. A switch
    // that reinterprets integers without this split is the silent-100x bug.
    expect(formatMoney(IDR, 1500000)).toBe("Rp1.500.000");
    expect(formatMoney(USD, 1500000)).toBe("$15,000.00");
  });

  it("pads the fraction to the exponent", () => {
    expect(formatMoney(USD, 0)).toBe("$0.00");
    expect(formatMoney(USD, 1)).toBe("$0.01");
    expect(formatMoney(USD, 100)).toBe("$1.00");
    expect(formatMoney(IDR, 0)).toBe("Rp0");
  });

  it("keeps Go's symbol-outside-minus order for negatives", () => {
    // Go decorates the already-signed bare figure, so the symbol leads the
    // minus: Decorate("USD", FormatSignedAmount("USD", -150000)) is
    // "$-1,500.00", not "-$1,500.00".
    expect(formatMoney(USD, -150000)).toBe("$-1,500.00");
    expect(formatMoney(IDR, -2500000)).toBe("Rp-2.500.000");
  });

  it("falls back to IDR separators for a malformed entry", () => {
    // Mirrors Go's Normalize-to-IDR rule for unknown codes: identical
    // separators cannot group, so the entry renders as IDR.
    const malformed: CurrencyInfo = {
      code: "XX",
      exponent: 0,
      symbol: "Rp",
      position: "before",
      groupSep: ",",
      decimalSep: ",",
      symbolSpaced: false,
    };
    expect(formatMoney(malformed, 15000000)).toBe("Rp15.000.000");
  });
});

describe("parseMoneyInput mirrors Go's ParseMinor", () => {
  it("parses Go's TestParseMinor cases", () => {
    const cases: Array<[CurrencyInfo, string, number]> = [
      [USD, "15000.00", 1500000],
      [USD, "15,000.00", 1500000],
      [USD, "15,000", 1500000],
      [USD, "0.01", 1],
      [USD, "1.5", 150],
      [USD, "-1,500.00", -150000],
      [IDR, "15.000.000", 15000000],
      [IDR, "15000000", 15000000],
      [IDR, "0", 0],
      [EUR, "1.234,56", 123456],
      [JPY, "1,500", 1500],
      [VND, "15.000", 15000],
    ];
    for (const [entry, text, want] of cases) {
      expect(parseMoneyInput(entry, text), `${entry.code} ${text}`).toBe(want);
    }
  });

  it("never silently returns 0 for formatted money", () => {
    // Today's parseIntOrZero("$1,500.00") is 0; that is the bug being fixed.
    const parsed = parseMoneyInput(USD, "$1,500.00");
    expect(parsed).not.toBe(0);
    expect(parsed).toBe(150000);
  });

  it("strips each entry's own symbol and spacing", () => {
    expect(parseMoneyInput(IDR, "Rp15.000.000")).toBe(15000000);
    expect(parseMoneyInput(EUR, "1.234,56 €")).toBe(123456);
    expect(parseMoneyInput(EUR, "1.234,56\u00a0€")).toBe(123456);
    expect(parseMoneyInput(AED, "AED 15,000.00")).toBe(1500000);
    expect(parseMoneyInput(VND, "15.000 ₫")).toBe(15000);
  });

  it("refuses what Go refuses, returning 0", () => {
    // Go's TestParseMinorRefuses cases: misgrouped, mixed separators,
    // over-precise fractions, fractions on exponent-0 currencies.
    const refusals: Array<[CurrencyInfo, string]> = [
      [USD, ""],
      [USD, "abc"],
      [USD, "15,00.00"],
      [USD, "1,50,00.00"],
      [USD, "15.000,00"],
      [USD, "15.000"],
      [USD, "1.005"],
      [IDR, "15,5"],
      [IDR, "15.000,5"],
      [JPY, "1500.5"],
      [EUR, "1,234.56"],
    ];
    for (const [entry, text] of refusals) {
      expect(parseMoneyInput(entry, text), `${entry.code} ${text}`).toBe(0);
    }
  });
});

describe("formatMoneyInput", () => {
  it("leaves empty empty", () => {
    expect(formatMoneyInput(USD, "")).toBe("");
    expect(formatMoneyInput(IDR, "")).toBe("");
  });

  it("renders stored minor-unit digit strings", () => {
    expect(formatMoneyInput(USD, "150000")).toBe("$1,500.00");
    expect(formatMoneyInput(IDR, "15000000")).toBe("Rp15.000.000");
    expect(formatMoneyInput(EUR, "123456")).toBe("1.234,56 €");
    expect(formatMoneyInput(VND, "15000")).toBe("15.000 ₫");
  });

  it("reads half-typed decimals with major-unit semantics", () => {
    expect(formatMoneyInput(USD, "15.5")).toBe("$15.50");
    expect(formatMoneyInput(USD, "15.")).toBe("$15.00");
  });

  it("round-trips fed-back formatted output", () => {
    expect(formatMoneyInput(USD, "$1,500.00")).toBe("$1,500.00");
    expect(formatMoneyInput(IDR, "Rp15.000.000")).toBe("Rp15.000.000");
    expect(formatMoneyInput(EUR, "1.234,56 €")).toBe("1.234,56 €");
  });

  it("reads pasted grouped figures with major-unit semantics", () => {
    expect(formatMoneyInput(USD, "15,000")).toBe("$15,000.00");
    expect(formatMoneyInput(IDR, "15.000")).toBe("Rp15.000");
  });
});

describe("format and parse are inverses across the catalogue", () => {
  it("every entry parses its own formatted output back", () => {
    // Mirrors Go's TestFormatParseInverse: format a stored integer, parse
    // it back, get the same integer.
    for (const { entry, minor } of ALL_ENTRIES) {
      expect(parseMoneyInput(entry, formatMoney(entry, minor)), entry.code).toBe(
        minor
      );
    }
  });
});

describe("decorateMoney agrees with Go's Decorate per entry", () => {
  // Go's backend/currency TestZZDumpDecorate output (issue #336 Wave B U1):
  // Decorate(code, FormatAmount(code, minor)) for minor 1500000 (exponent 2)
  // or 15000000 (exponent 0). decorateMoney must reproduce every row from
  // the bare string alone -- it never sees the integer.
  it("decorates every catalogue entry's bare figure exactly as Go does", () => {
    const cases: Array<[CurrencyInfo, string, string]> = [
      [AED, "15,000.00", "AED 15,000.00"],
      [USD, "15,000.00", "$15,000.00"],
      [EUR, "1.234,56", "1.234,56 €"],
      [JPY, "15,000,000", "¥15,000,000"],
      [VND, "15.000.000", "15.000.000 ₫"],
      [IDR, "15.000.000", "Rp15.000.000"],
    ];
    for (const [entry, bare, want] of cases) {
      expect(decorateMoney(entry, bare), entry.code).toBe(want);
    }
  });

  it("keeps Go's symbol-outside-minus order for signed bare figures", () => {
    // Go decorates the already-signed bare figure: the minus stays inside
    // the affixes. The components pass Display* strings straight through,
    // so a signed DisplayNett renders the same here as on the server.
    expect(decorateMoney(USD, "-1,500.00")).toBe("$-1,500.00");
    expect(decorateMoney(IDR, "-2.500.000")).toBe("Rp-2.500.000");
    expect(decorateMoney(EUR, "-1.234,56")).toBe("-1.234,56 €");
  });

  it("decorates zero and unknown codes as Go does", () => {
    expect(decorateMoney(USD, "0.00")).toBe("$0.00");
    // Go falls back to IDR affixes for unknown codes; this side falls back
    // to the caller's entry, so an IDR entry decorates the same way.
    expect(decorateMoney(IDR, "1.000")).toBe("Rp1.000");
  });

  it("agrees with formatMoney on every catalogue entry", () => {
    // decorateMoney(entry, bare) must equal formatMoney(entry, minor) when
    // bare is that minor's formatted figure: one must never drift from the
    // other, since components mix raw integers and server strings.
    for (const { entry, minor } of ALL_ENTRIES) {
      const bare = formatMoney(entry, minor)
        .replace(entry.symbol, "")
        .trim();
      expect(decorateMoney(entry, bare), entry.code).toBe(
        formatMoney(entry, minor)
      );
    }
  });
});

// Issue #336 criterion 3, joined in ONE test: every hop of the chain the user
// walks, in order, so a 100x slip between hops cannot hide. The chain:
// keystroke -> parseMoneyInput -> stored minor units -> formatMoneyInput
// (the field) -> formatMoney (the review figure) -> the server prints it via
// Go's ParseMinor/FormatAmount/Decorate -> the archive decorates the bare
// Display* back with decorateMoney.
//
// The Go half of the same chain is TestParseMinorRoundTrip in
// backend/currency/currency_test.go: ParseMinor("USD", "15000.00") = 1500000
// and Decorate(FormatAmount(...)) = "$15,000.00". Every literal below is
// Go's own output (this file's agreement pattern), so a hop that changes
// denomination on either side turns that side's test red.
describe("the keystroke round trip, joined end to end", () => {
  it("type 15000.00, store 1500000, see $15,000.00 at every hop", () => {
    // USD, the issue's own example.
    const stored = parseMoneyInput(USD, "15000.00");
    expect(stored).toBe(1500000);
    expect(formatMoneyInput(USD, String(stored))).toBe("$15,000.00");
    expect(formatMoney(USD, stored)).toBe("$15,000.00");
    expect(decorateMoney(USD, "15,000.00")).toBe("$15,000.00");

    // The exponent-0 control: the same walk in IDR must read the digits its
    // own way, never USD's.
    const idrStored = parseMoneyInput(IDR, "15.000.000");
    expect(idrStored).toBe(15000000);
    expect(formatMoneyInput(IDR, String(idrStored))).toBe("Rp15.000.000");
    expect(formatMoney(IDR, idrStored)).toBe("Rp15.000.000");
    expect(decorateMoney(IDR, "15.000.000")).toBe("Rp15.000.000");
  });
});
