// @vitest-environment jsdom
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import { PurchaseOrderReportMonthAggregate } from "./PurchaseOrderReportMonthAggregate";
import { resolveMonthAggregate } from "../poReportComparison";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportYearFixture,
  purchaseOrderReportYearWithRecordedFutureFixture,
} from "../poReportFixtures";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderMonthAggregate } from "../poReportComparison";

// Issue #642: the wide All/Aggregate table is server-owned. These are direct
// structural reads of the caption, cells, Recorded total and separate forecast
// block; they do not claim a browser layout measurement.

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

function render(aggregate: PurchaseOrderMonthAggregate): HTMLElement {
  document.body.innerHTML = renderToString(
    <PurchaseOrderReportMonthAggregate
      aggregate={aggregate}
      entry={IDR}
      onRetry={() => undefined}
    />
  );
  return document.body;
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("All month aggregate (issue #642)", () => {
  it("prints the twelve month rows, the server total and the forecast separately", () => {
    const overview = purchaseOrderReportYearFixture();
    overview.totals = { ...overview.totals, amountMinor: 999_000_000 };
    const aggregate = resolveMonthAggregate({
      overview,
      year: "2026",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    const body = render(aggregate);

    expect(document.querySelector("[data-month-aggregate]")).not.toBeNull();
    expect(body.textContent).toContain("Recorded total");
    expect(body.textContent).toContain("not a sum of the rows");
    expect(
      document.querySelector('[data-aggregate-total-metric="Amount"]')
        ?.textContent
    ).toBe("Rp999.000.000");
    expect(document.querySelectorAll("[data-aggregate-row]")).toHaveLength(12);

    const december = document.querySelector(
      '[data-aggregate-row="2026-12"]'
    );
    expect(december?.getAttribute("data-aggregate-recorded")).toBe("false");
    expect(december?.textContent).toContain("No recorded POs");
    for (const metric of ["POs", "Amount", "Costed", "Capital", "Margin", "Margin %"]) {
      expect(
        december?.querySelector(`[data-aggregate-metric="${metric}"]`)
          ?.textContent
      ).toBe("-");
    }

    const forecast = document.querySelector(
      '[data-aggregate-estimate="2026-12"]'
    );
    expect(forecast).not.toBeNull();
    expect(forecast?.textContent).toContain("Estimated forecast (not actual)");
    expect(forecast?.textContent).toContain("Rp50.000.000");
    expect(forecast?.getAttribute("data-estimated")).toBe("true");
    expect(
      document.querySelector(
        '[data-aggregate-total-metric="Amount"]'
      )?.textContent
    ).not.toContain("50.000.000");
  });

  it("discloses future-dated recorded evidence without treating it as a forecast", () => {
    const overview = purchaseOrderReportYearWithRecordedFutureFixture();
    const aggregate = resolveMonthAggregate({
      overview,
      year: "2026",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    const body = render(aggregate);
    expect(aggregate.includesFutureDated).toBe(true);
    expect(body.textContent).toContain(
      "Future-dated purchase orders are included as recorded evidence, never as a forecast."
    );
    expect(body.textContent).not.toContain("Estimated forecast (not actual) — November");
  });

  it("renders an explicit loading or failed state with no old money", () => {
    const overview = purchaseOrderReportFixture();
    const loading = resolveMonthAggregate({
      overview,
      year: "2025",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    render(loading);
    expect(document.body.textContent).toContain("Loading the 2025 annual snapshot");
    expect(document.querySelector("[data-aggregate-total-metric]")).toBeNull();
    expect(document.body.textContent).not.toContain("Rp");

    const failed = resolveMonthAggregate({
      overview,
      year: "2025",
      annualSource: null,
      annualStatus: "error",
      annualError: "The annual API is unreachable.",
    });
    render(failed);
    expect(document.body.textContent).toContain("The annual API is unreachable.");
    expect(document.body.textContent).toContain("Retry");
    expect(document.querySelector("[data-aggregate-total-metric]")).toBeNull();
  });

  it("says an incomplete annual snapshot cannot be exported", () => {
    const overview = purchaseOrderReportYearFixture();
    overview.periods = overview.periods.filter(
      (period) => period.periodStart !== "2026-03-01"
    );
    const aggregate = resolveMonthAggregate({
      overview,
      year: "2026",
      annualSource: null,
      annualStatus: "loading",
      annualError: "",
    });
    render(aggregate);
    expect(aggregate.complete).toBe(false);
    expect(document.body.textContent).toContain(
      "has no row for at least one month"
    );
    expect(document.body.textContent).toContain("cannot be exported");
  });
});
