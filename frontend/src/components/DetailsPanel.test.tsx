import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  DetailsPanel,
  formatLeadTimeSegment,
  parseLeadTimeSegment,
  type DetailsState,
} from "./DetailsPanel";
import { TAX_TYPE_OPTIONS, TAX_ZONE_OPTIONS } from "../tax";

const DETAILS: DetailsState = {
  subject: "",
  recipientName: "",
  validity: 30,
  paymentTerm: 90,
  leadTimeYears: 0,
  leadTimeMonths: 0,
  leadTimeWeeks: 0,
  leadTimeDays: 0,
  discountPercent: 0,
  incoterm: "EXW",
  taxZone: "FTZ",
  taxType: "Goods",
  serviceCategory: "",
};

function render(details: DetailsState = DETAILS): string {
  return renderToString(
    <DetailsPanel
      value={details}
      customerOptions={[{ value: 2, label: "Client" }]}
      customerId={2}
      incotermOptions={[{ value: "EXW", label: "EXW" }]}
      taxZoneOptions={TAX_ZONE_OPTIONS}
      taxTypeOptions={TAX_TYPE_OPTIONS}
      onChange={() => undefined}
      onCustomerChange={() => undefined}
      onPaste={() => undefined}
    />
  );
}

describe("parseLeadTimeSegment", () => {
  it("keeps plain figures", () => {
    expect(parseLeadTimeSegment("60")).toBe(60);
    expect(parseLeadTimeSegment("0")).toBe(0);
  });

  it("reads empty and garbage as unstated, never NaN", () => {
    expect(parseLeadTimeSegment("")).toBe(0);
    expect(parseLeadTimeSegment("abc")).toBe(0);
  });

  it("strips non-digits instead of refusing the input", () => {
    // One guard site covers typing, paste and autofill alike: "60 days"
    // pasted from a spreadsheet still lands as 60.
    expect(parseLeadTimeSegment("60 days")).toBe(60);
    expect(parseLeadTimeSegment("-5")).toBe(5);
  });

  it("clamps to the uint8 range the server decodes", () => {
    expect(parseLeadTimeSegment("255")).toBe(255);
    expect(parseLeadTimeSegment("999")).toBe(255);
    // Three digits are kept, then clamped: a longer figure keeps its head.
    expect(parseLeadTimeSegment("1234")).toBe(123);
    expect(parseLeadTimeSegment("9999")).toBe(255);
  });
});

describe("formatLeadTimeSegment", () => {
  it("stores 0 but displays empty over a zero placeholder", () => {
    // The 0 is a sentinel for "unstated", not a duration: an untouched field
    // must never read as zero (the formatMoneyInput convention).
    expect(formatLeadTimeSegment(0)).toBe("");
    expect(formatLeadTimeSegment(60)).toBe("60");
  });
});

describe("DetailsPanel lead time", () => {
  it("renders four labelled segments with house suffixes", () => {
    const html = render();
    for (const label of [
      "Lead time years",
      "Lead time months",
      "Lead time weeks",
      "Lead time days",
    ]) {
      expect(html).toContain(`aria-label="${label}"`);
    }
    for (const suffix of ["yr", "mo", "wk", "day"]) {
      expect(html).toContain(`>${suffix}<`);
    }
  });

  it("groups the segments under one labelled group", () => {
    const html = render();
    expect(html).toContain('role="group"');
    expect(html).toContain('aria-labelledby="lead-time-label"');
    expect(html).toContain("Lead time");
  });

  it("shows empty fields with a zero placeholder when unstated", () => {
    const html = render();
    expect(html).toContain('placeholder="0"');
    // React renders an empty-string value as value="".
    expect(html.match(/aria-label="Lead time \w+"[^>]*value=""/g)).toHaveLength(4);
  });

  it("shows the entered figures and never a composed sentence", () => {
    // The sentence ("2 Years, 10 Months, 4 Weeks, and 5 Days") is composed
    // once in Go and consumed from the DTO: composing it here would let the
    // editor disagree with the quotation.
    const html = render({
      ...DETAILS,
      leadTimeYears: 2,
      leadTimeMonths: 10,
      leadTimeWeeks: 4,
      leadTimeDays: 5,
    });
    expect(html).toContain('value="2"');
    expect(html).toContain('value="10"');
    expect(html).toContain('value="4"');
    expect(html).toContain('value="5"');
    expect(html).not.toContain("2 Years");
    expect(html).not.toContain("and 5 Days");
  });
});
