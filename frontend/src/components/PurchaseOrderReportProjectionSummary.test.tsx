import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
} from "../poReportFixtures";
import {
  PurchaseOrderReportProjectionStatus,
  PurchaseOrderReportProjectionSummary,
} from "./PurchaseOrderReportProjectionSummary";

// Issue #644: the overview keeps exactly one compact forecast status visible and
// moves the full basis/method/range prose into Details. The compact readout is a
// new public surface; the full summary stays the disclosure's content and keeps
// its existing standalone render.

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

describe("PurchaseOrderReportProjectionStatus (issue #644)", () => {
  it("states one available forecast with the exact server-owned basis", () => {
    const markup = renderToStaticMarkup(
      <PurchaseOrderReportProjectionStatus
        report={purchaseOrderReportMonthFixture()}
      />
    );

    expect(markup).toContain('data-report-forecast-status=""');
    expect(markup).toContain('data-projection-status="available"');
    expect(markup).toContain('data-projection-mode="day"');
    expect(markup).toContain("Projected");
    expect(markup).toContain("Month view, daily buckets");
    expect(markup).toContain("19 elapsed days");
    expect(markup).toContain("12 POs observed");
    expect(markup).not.toContain("as of 19 Nov 2026");
    expect(markup).not.toContain("Basis:");
    expect(markup).not.toContain("Method:");
  });

  it("keeps the stable reason in the status when the forecast is unavailable", () => {
    const report = purchaseOrderReportMonthFixture();
    report.currentProjection.status = "unavailable";
    report.currentProjection.reason = "insufficient_days";
    report.currentProjection.buckets = [];

    const markup = renderToStaticMarkup(
      <PurchaseOrderReportProjectionStatus report={report} />
    );

    expect(markup).toContain('data-projection-status="unavailable"');
    expect(markup).toContain('data-projection-reason="insufficient_days"');
    expect(markup).toContain("Unavailable");
    expect(markup).toContain("Not enough elapsed days");
    expect(markup).not.toContain("Per remaining day");
  });

  it("names a historical range as not applicable instead of implying a zero forecast", () => {
    const markup = renderToStaticMarkup(
      <PurchaseOrderReportProjectionStatus report={purchaseOrderReportFixture()} />
    );

    expect(markup).toContain('data-projection-mode="none"');
    expect(markup).toContain("Not applicable");
    expect(markup).toContain(
      "Historical range, no current-period projection"
    );
  });
});

describe("PurchaseOrderReportProjectionSummary (issue #644)", () => {
  it("keeps the full basis, method and range content for the Details region", () => {
    const markup = renderToStaticMarkup(
      <PurchaseOrderReportProjectionSummary
        report={purchaseOrderReportYearFixture()}
        currency={IDR}
        plain
      />
    );

    expect(markup).toContain("Current-period projection");
    expect(markup).toContain("Rest of this month");
    expect(markup).toContain("Per remaining month");
    expect(markup).toContain("Low");
    expect(markup).toContain("Base");
    expect(markup).toContain("High");
    expect(markup).toContain("Basis:");
    expect(markup).toContain("Method:");
    expect(markup).toContain("Updates as data arrives");
  });
});
