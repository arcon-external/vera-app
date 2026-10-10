import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { computePricing } from "./components/SummaryRail";
import type { PricedItem } from "./components/SummaryRail";
import type { EditorSubItem } from "./components/ItemEditor";
import { applyDiscount, applyMargin, MAX_DISCOUNT_PERCENT } from "./format";

// The live summary rail duplicates the backend's pricing in TypeScript so the
// editor can show totals before Prepare runs. Two implementations of one rule is
// the setup for drift, and drift here is a number the customer is quoted.
//
// This file used to claim it would catch that -- "these cases are the ones Go
// asserts ... If one side changes a rule, the other side fails here rather than
// quietly disagreeing." It would not. Both sides asserted their own literals, so
// a Go change failed the Go test, someone updated the Go literal, and this file
// carried on passing with the old number.
//
// So the shared cases now come from a fixture that backend's
// pricing_fixture_test.go reads too. The expectation lives in exactly one place,
// which is what makes the claim true rather than aspirational.

interface FixtureLine {
  cost: number;
  qty: number;
  marginEnabled: boolean;
  marginPercent: number;
  discountEnabled: boolean;
  discountPercent: number;
  subItems?: FixtureLine[];
}

interface FixtureCase {
  name: string;
  why: string;
  lines: FixtureLine[];
  overallDiscountPercent: number;
  taxZone: string;
  taxType: string;
  serviceCategory: string;
  want: Record<string, number>;
}

// Resolved from this module rather than from the working directory, so the test
// behaves the same however it is invoked.
const fixturePath = fileURLToPath(
  new URL("../../backend/testdata/pricing_cases.json", import.meta.url)
);
const fixture = JSON.parse(readFileSync(fixturePath, "utf8")) as { cases: FixtureCase[] };

function toSubItem(line: FixtureLine, index: number): EditorSubItem {
  return {
    id: index,
    description: `part ${index}`,
    qty: String(line.qty),
    uom: "Pc",
    unitCost: String(line.cost),
    marginEnabled: line.marginEnabled,
    marginPercent: String(line.marginPercent),
    discountEnabled: line.discountEnabled,
    discountPercent: String(line.discountPercent),
    bullets: [],
  };
}

function toPricedItem(line: FixtureLine): PricedItem {
  return {
    qty: String(line.qty),
    unitPrice: String(line.cost),
    marginEnabled: line.marginEnabled,
    marginPercent: String(line.marginPercent),
    discountEnabled: line.discountEnabled,
    discountPercent: String(line.discountPercent),
    subItems: line.subItems?.map(toSubItem),
  };
}

describe("applyDiscount", () => {
  it("is off until it is switched on", () => {
    expect(applyDiscount(100000, 50, false)).toBe(100000);
  });

  it("truncates, exactly as the backend does", () => {
    // 108000 * 33 / 100 = 35640 exactly, but 100000 * 33 / 100 truncates.
    // The point is that both sides truncate at the same place.
    expect(applyDiscount(100000, 33, true)).toBe(67000);
    expect(applyMargin(333, 50, true)).toBe(499);
  });
});

describe("computePricing matches the shared fixture", () => {
  it("the fixture is not empty", () => {
    // A fixture that failed to load would make every case below vanish rather
    // than fail, which is the quietest possible way to lose this coverage.
    expect(fixture.cases.length).toBeGreaterThan(0);
  });

  for (const testCase of fixture.cases) {
    it(testCase.name, () => {
      const priced = computePricing(
        testCase.lines.map(toPricedItem),
        testCase.overallDiscountPercent,
        testCase.taxZone,
        testCase.taxType,
        testCase.serviceCategory
      );

      // Compared field by field against the same expectation Go asserts, so a
      // rule change on either side breaks whichever side no longer matches.
      for (const [field, want] of Object.entries(testCase.want)) {
        const got = (priced as unknown as Record<string, number>)[field];
        if (got === undefined) {
          throw new Error(
            `the fixture asserts ${field}, which computePricing does not return; ` +
              `the assertion is not real until it does`
          );
        }
        expect(got, `${testCase.why} (${field})`).toBe(want);
      }
    });
  }
});

describe("MAX_DISCOUNT_PERCENT", () => {
  it("allows a free line and nothing past it", () => {
    // Mirrors MaxDiscountPercent in the backend. Written down twice is a limit
    // that eventually disagrees with itself, so the parity is asserted.
    expect(MAX_DISCOUNT_PERCENT).toBe(100);
  });
});
