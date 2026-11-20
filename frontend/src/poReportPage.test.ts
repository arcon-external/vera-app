import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import type { CurrencyInfo } from "./format";
import { formatMoney } from "./format";
import {
  REPORT_BUCKET_OPTIONS,
  REPORT_CATEGORY_OPTIONS,
  REPORT_PRESET_OPTIONS,
  addDaysIso,
  categoryFilterOptions,
  formatReportBps,
  formatReportCount,
  formatReportDate,
  initialReportFilters,
  projectionReasonLabel,
  reportCategoryLabel,
  reportCurrencyOptions,
  reportFilterKey,
  reportFilterValidation,
  reportPeriodLabel,
  reportPresetRange,
  reportRangeLabel,
  toReportQuery,
} from "./poReportPage";
import type { ReportFilterState } from "./poReportPage";
import type { PurchaseOrderReportData } from "./poReport";

// The shared cross-language formatting contract (#630/#632). The Go PDF
// renderer reads the same file, so a sign or separator change cannot land on
// one surface only.
interface ReportFormatFixture {
  money: Array<{ name: string; code: string; minor: number; expected: string }>;
  bps: Array<{ name: string; value: number | null; expected: string }>;
  count: Array<{ name: string; value: number; expected: string }>;
}

const FORMAT_FIXTURE = JSON.parse(
  readFileSync(
    new URL(
      "../../backend/render/report/testdata/report_formats.json",
      import.meta.url
    ),
    "utf8"
  )
) as ReportFormatFixture;

const FORMAT_CURRENCIES: Record<string, CurrencyInfo> = {
  IDR: {
    code: "IDR",
    exponent: 0,
    symbol: "Rp",
    position: "before",
    groupSep: ".",
    decimalSep: ",",
    symbolSpaced: false,
  },
  USD: {
    code: "USD",
    exponent: 2,
    symbol: "$",
    position: "before",
    groupSep: ",",
    decimalSep: ".",
    symbolSpaced: false,
  },
  EUR: {
    code: "EUR",
    exponent: 2,
    symbol: "€",
    position: "after",
    groupSep: ".",
    decimalSep: ",",
    symbolSpaced: true,
  },
};

// Issue #630: the pure half of the global Reports workspace. The screen's
// honesty rests on these decisions, so they are pinned without React:
// inclusive UI dates become an exclusive API `to`, presets are real calendar
// ranges, percentages render exactly from basis points (null is unknown, never
// 0%), and a filter never silently loses Unassigned or the current currency.

function filters(overrides: Partial<ReportFilterState> = {}): ReportFilterState {
  return {
    preset: "this-year",
    from: "2026-01-01",
    to: "2026-11-19",
    bucket: "month",
    category: "all",
    currency: "IDR",
    ...overrides,
  };
}

describe("report filter catalogue", () => {
  it("offers the approved presets and buckets in order", () => {
    expect(REPORT_PRESET_OPTIONS.map((option) => option.value)).toEqual([
      "this-year",
      "last-year",
      "last-12-months",
      "last-5-years",
      "custom",
    ]);
    expect(REPORT_PRESET_OPTIONS.map((option) => option.label)).toEqual([
      "This year",
      "Last year",
      "Last 12 months",
      "Last 5 years",
      "Custom",
    ]);
    expect(REPORT_BUCKET_OPTIONS.map((option) => option.value)).toEqual([
      "year",
      "quarter",
      "month",
    ]);
    expect(REPORT_CATEGORY_OPTIONS).toEqual([
      { value: "all", label: "All categories" },
      { value: "FM", label: "Facility Management" },
      { value: "FAB", label: "Fabrication" },
      { value: "SUPPLY_GEN", label: "Goods Supply & General" },
      { value: "OTHERS", label: "Others" },
    ]);
  });

  it("adds Unassigned only when the report population has it, or it is selected", () => {
    expect(categoryFilterOptions(false, "all").map((option) => option.value)).toEqual([
      "all",
      "FM",
      "FAB",
      "SUPPLY_GEN",
      "OTHERS",
    ]);
    expect(categoryFilterOptions(true, "all").at(-1)).toEqual({
      value: "UNASSIGNED",
      label: "Unassigned",
    });
    // A selected Unassigned stays visible even while the previous snapshot
    // says the population does not contain one, so the trigger never reads
    // blank during a fetch.
    expect(categoryFilterOptions(false, "UNASSIGNED").at(-1)).toEqual({
      value: "UNASSIGNED",
      label: "Unassigned",
    });
  });

  it("keeps the selected currency in the partition list", () => {
    expect(reportCurrencyOptions(["IDR", "USD"], "IDR")).toEqual(["IDR", "USD"]);
    expect(reportCurrencyOptions(["USD"], "IDR")).toEqual(["IDR", "USD"]);
    expect(reportCurrencyOptions([], "USD")).toEqual(["USD"]);
    expect(reportCurrencyOptions(["IDR"], "")).toEqual(["IDR"]);
  });

  it("names categories for prose without carrying a code into the UI", () => {
    expect(reportCategoryLabel("all")).toBe("All categories");
    expect(reportCategoryLabel("SUPPLY_GEN")).toBe("Goods Supply & General");
    expect(reportCategoryLabel("UNASSIGNED")).toBe("Unassigned");
    expect(reportCategoryLabel("LEGACY_CODE")).toBe("LEGACY_CODE");
  });
});

describe("presets and exclusive dates", () => {
  it("turns each preset into an inclusive calendar range", () => {
    expect(reportPresetRange("this-year", "2026-11-19")).toEqual({
      from: "2026-01-01",
      to: "2026-11-19",
    });
    expect(reportPresetRange("last-year", "2026-11-19")).toEqual({
      from: "2025-01-01",
      to: "2025-12-31",
    });
    expect(reportPresetRange("last-12-months", "2026-11-19")).toEqual({
      from: "2025-12-01",
      to: "2026-11-19",
    });
    expect(reportPresetRange("last-5-years", "2026-11-19")).toEqual({
      from: "2022-01-01",
      to: "2026-11-19",
    });
  });

  it("moves across month, year and leap-day boundaries", () => {
    expect(addDaysIso("2026-11-30", 1)).toBe("2026-12-01");
    expect(addDaysIso("2026-12-31", 1)).toBe("2027-01-01");
    expect(addDaysIso("2028-02-28", 1)).toBe("2028-02-29");
    expect(addDaysIso("2028-02-29", 1)).toBe("2028-03-01");
    expect(addDaysIso("2026-01-01", -1)).toBe("2025-12-31");
  });

  it("translates the inclusive UI end into the API's exclusive to", () => {
    expect(toReportQuery(filters({ to: "2026-12-31" }))).toEqual({
      from: "2026-01-01",
      to: "2027-01-01",
      bucket: "month",
      category: "all",
      currency: "IDR",
    });
    expect(toReportQuery(filters({ to: "2028-02-29" })).to).toBe("2028-03-01");
  });

  it("starts the workspace on the current year to date", () => {
    expect(initialReportFilters("2026-11-19", "usd")).toMatchObject({
      preset: "this-year",
      from: "2026-01-01",
      to: "2026-11-19",
      bucket: "month",
      category: "all",
      currency: "USD",
    });
  });
});

describe("filter validation", () => {
  it("requires real, ordered dates", () => {
    expect(reportFilterValidation(filters())).toBe("");
    expect(reportFilterValidation(filters({ from: "not-a-date" }))).toContain(
      "start date"
    );
    expect(reportFilterValidation(filters({ to: "2026-02-30" }))).toContain(
      "end date"
    );
    expect(
      reportFilterValidation(filters({ from: "2026-12-01", to: "2026-11-30" }))
    ).toContain("on or before");
  });

  it("requires a bucket, category and currency", () => {
    expect(
      reportFilterValidation(filters({ bucket: "week" as never }))
    ).toContain("bucket");
    expect(
      reportFilterValidation(filters({ category: "LEGACY" }))
    ).toContain("category");
    expect(reportFilterValidation(filters({ currency: "  " }))).toContain(
      "currency"
    );
  });

  it("keeps a stable key for the exact API query", () => {
    const query = toReportQuery(filters({ category: "UNASSIGNED" }));
    expect(reportFilterKey(query)).toBe(
      "2026-01-01|2026-11-20|month|UNASSIGNED|IDR"
    );
    expect(reportFilterKey({ ...query, to: "2026-11-21" })).not.toBe(
      reportFilterKey(query)
    );
  });
});

describe("report labels and exact percentages", () => {
  it("labels each bucket's period", () => {
    expect(reportPeriodLabel("2026-01-01", "month")).toBe("Jan 2026");
    expect(reportPeriodLabel("2026-04-01", "quarter")).toBe("Q2 2026");
    expect(reportPeriodLabel("2026-01-01", "year")).toBe("2026");
    expect(reportPeriodLabel("garbage", "month")).toBe("garbage");
  });

  it("names the inclusive range a reader actually selected", () => {
    expect(
      reportRangeLabel({
        from: "2026-01-01",
        to: "2027-01-01",
        bucket: "year",
        category: "all",
        currency: "IDR",
      })
    ).toBe("1 Jan 2026 – 31 Dec 2026");
    expect(formatReportDate("2026-11-19")).toBe("19 Nov 2026");
    expect(formatReportDate("")).toBe("");
  });

  it("renders basis points exactly and null as unknown", () => {
    expect(formatReportBps(null)).toBe("-");
    expect(formatReportBps(0)).toBe("0.00%");
    expect(formatReportBps(1250)).toBe("12.50%");
    expect(formatReportBps(6667)).toBe("66.67%");
    expect(formatReportBps(-500)).toBe("-5.00%");
  });

  it("renders stable projection reasons without inventing a forecast", () => {
    expect(projectionReasonLabel("no_data_in_range")).toContain(
      "No purchase orders"
    );
    expect(projectionReasonLabel("insufficient_history")).toContain(
      "Not enough"
    );
    expect(projectionReasonLabel("incomplete_cost_basis")).toContain("cost");
    expect(projectionReasonLabel("value_out_of_range")).toContain("exact");
    expect(projectionReasonLabel("something_new")).toContain("something_new");
    expect(projectionReasonLabel(undefined)).toContain("Not enough");
  });
});

describe("the normalized report is the only shape the page reads", () => {
  it("accepts a report fixture without touching generated wire fields", () => {
    // Compile-time guard: this module imports the normalized contract, not
    // poreport.Report. The runtime assertion pins the filter key it exposes.
    const report = {
      filter: {
        from: "2026-01-01",
        to: "2027-01-01",
        bucket: "month",
        category: "all",
        currency: "IDR",
      },
    } as PurchaseOrderReportData;
    expect(reportFilterKey(report.filter)).toBe("2026-01-01|2027-01-01|month|all|IDR");
  });
});

describe("cross-surface formatting fixture (issues #630/#632)", () => {
  it("formats every money case exactly as the PDF renderer does", () => {
    for (const entry of FORMAT_FIXTURE.money) {
      const currency = FORMAT_CURRENCIES[entry.code];
      if (currency === undefined) {
        throw new Error(`fixture names an unmapped currency ${entry.code}`);
      }
      expect(formatMoney(currency, entry.minor), entry.name).toBe(entry.expected);
    }
  });

  it("formats every basis-point case with two exact decimals", () => {
    for (const entry of FORMAT_FIXTURE.bps) {
      expect(formatReportBps(entry.value), entry.name).toBe(entry.expected);
    }
  });

  it("groups every count in en-US threes", () => {
    for (const entry of FORMAT_FIXTURE.count) {
      expect(formatReportCount(entry.value), entry.name).toBe(entry.expected);
    }
  });
});
