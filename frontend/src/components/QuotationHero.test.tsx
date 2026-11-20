import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import {
  QuotationHero,
  type QuotationHeroDetails,
  type QuotationHeroProps,
} from "./QuotationHero";

// Issue #623: the Quotation editor's dark hero is a read-only mirror of the
// editor state. These cases render it to a string, so what is asserted is the
// markup a staff member sees in each state -- the draft badge, the issued
// revision pair, the meta values, the customer total and the status line.
//
// What this cannot prove: live updates from the real editor (the App-level DOM
// suite drives those), pixel placement, and interactions -- the hero has none
// by design, and the no-controls case pins that.

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const DETAILS: QuotationHeroDetails = {
  validity: 30,
  paymentTerm: 90,
  incoterm: "EXW",
  taxZone: "FTZ",
  taxType: "Goods",
  serviceCategory: "",
};

const DRAFT: QuotationHeroProps = {
  customerName: "PT. Infineon Technologies Batam",
  details: DETAILS,
  currencyCode: "IDR",
  currency: IDR,
  customerGrand: 0,
  ppnAmount: 0,
  quoteNumber: null,
  nextRevision: null,
  draftSavedLabel: null,
  ready: false,
};

function markup(overrides: Partial<QuotationHeroProps> = {}): string {
  return renderToString(<QuotationHero {...DRAFT} {...overrides} />);
}

function totalOf(html: string): string {
  const match =
    /data-testid="quotation-hero-total"[^>]*>([^<]*)</.exec(html);
  if (match === null) throw new Error("no customer total in the hero");
  return match[1];
}

function titleOf(html: string): string {
  const match = /<h1[^>]*>([^<]*)<\/h1>/.exec(html);
  if (match === null) throw new Error("no title in the hero");
  return match[1];
}

describe("QuotationHero", () => {
  it("renders a new draft with the draft badge and save status", () => {
    const html = markup();

    expect(html).toContain("Quotation");
    expect(html).toContain("New quotation");
    expect(html).toContain("DRAFT");
    expect(html).not.toContain("CURRENT REV");
    expect(html).not.toContain("NEXT REV");
    expect(html).toContain("PT. Infineon Technologies Batam");
    expect(html).toContain("30 days");
    expect(html).toContain("90 days");
    expect(html).toContain("EXW · IDR");
    expect(html).toContain("FTZ · Goods");
    expect(html).toContain(
      "Not saved yet. Save draft stores it so you can continue later."
    );
    expect(totalOf(html)).toBe("Rp0");
  });

  it("says a saved draft was saved and can be updated", () => {
    const html = markup({ draftSavedLabel: "just now" });

    expect(html).toContain("DRAFT");
    expect(html).toContain("New quotation");
    expect(html).toContain("Draft saved just now. Save again to update it.");
    expect(html).not.toContain("Not saved yet");
  });

  it("reads ready once Prepare has produced the review", () => {
    const html = markup({ ready: true });

    expect(html).toContain("Ready to export. Review the three stages, then export.");
    expect(html).not.toContain("Not saved yet");
  });

  it("replaces the derived status line when a surface supplies its own", () => {
    // The practice tutorial's draft is never saved, so it must not promise
    // that Save draft stores the work (issue #624).
    const html = markup({
      statusOverride: "Practice: nothing is saved to your workspace.",
    });

    expect(html).toContain("Practice: nothing is saved to your workspace.");
    expect(html).not.toContain("Not saved yet");
    // The override changes the sentence, never the state it describes.
    expect(html).toContain("DRAFT");
    expect(html).toContain("New quotation");
  });

  it("names the quotation number and both revisions when editing an issued revision", () => {
    const html = markup({ quoteNumber: "1502260012", nextRevision: 3 });

    expect(html).toContain("1502260012");
    expect(html).not.toContain("New quotation");
    expect(html).toContain("CURRENT REV 02");
    expect(html).toContain("NEXT REV 03");
    expect(html).not.toContain("DRAFT");
    expect(html).toContain(
      "Revision 02 stays as issued. Export creates revision 03."
    );
  });

  it("pads a first revision pair to two digits", () => {
    const html = markup({ quoteNumber: "1502260001", nextRevision: 1 });

    expect(html).toContain("CURRENT REV 00");
    expect(html).toContain("NEXT REV 01");
    expect(html).toContain(
      "Revision 00 stays as issued. Export creates revision 01."
    );
  });

  it("stays issued when the stored number is empty", () => {
    // Issue #622: issuedness is the presence of the next revision, not the
    // number. A record whose number is missing must not tell staff it is a
    // new draft -- that would invite a second quotation on a live one.
    const html = markup({ quoteNumber: "", nextRevision: 3 });

    expect(titleOf(html)).toBe("Quotation");
    expect(html).not.toContain("New quotation");
    expect(html).toContain("CURRENT REV 02");
    expect(html).toContain("NEXT REV 03");
    expect(html).not.toContain("DRAFT");
    expect(html).toContain(
      "Revision 02 stays as issued. Export creates revision 03."
    );
  });

  it("falls back to no customer when none is selected", () => {
    expect(markup({ customerName: "" })).toContain("No customer yet");
  });

  it("prints the customer total with PPN when charged", () => {
    const html = markup({
      customerGrand: 1_110_000,
      ppnAmount: 110_000,
    });

    expect(totalOf(html)).toBe("Rp1.110.000");
    expect(html).toContain("Includes PPN");
  });

  it("omits the PPN note when no PPN is charged", () => {
    const html = markup({ customerGrand: 1_000_000, ppnAmount: 0 });

    expect(totalOf(html)).toBe("Rp1.000.000");
    expect(html).not.toContain("Includes PPN");
  });

  it("shows a dash instead of a denomination it cannot resolve", () => {
    const html = markup({
      currency: null,
      currencyCode: "IDR",
      customerGrand: 1_110_000,
      ppnAmount: 110_000,
    });

    expect(totalOf(html)).toBe("-");
    expect(html).not.toContain("Rp");
    expect(html).toContain("EXW · IDR");
  });

  it("adds the service category when the tax type is Services", () => {
    const html = markup({
      details: {
        ...DETAILS,
        taxZone: "NonFTZ",
        taxType: "Services",
        serviceCategory: "NonConstruction",
      },
    });

    expect(html).toContain("Non-FTZ · Services · Non-construction");
  });

  it("leaves the service category out for goods", () => {
    const html = markup({
      details: {
        ...DETAILS,
        taxZone: "NonFTZ",
        taxType: "Goods",
        serviceCategory: "NonConstruction",
      },
    });

    expect(html).toContain("Non-FTZ · Goods");
    expect(html).not.toContain("Non-construction");
  });

  it("sets its type from the scale, leaving no arbitrary size but the watermark", () => {
    // Issue #622: the hero speaks the type scale. The QT watermark keeps its
    // one-off size -- it is decoration, not type. The PPN note is rendered
    // here so its token is covered too.
    const html = markup({ customerGrand: 1_110_000, ppnAmount: 110_000 });
    for (const token of [
      "text-display-lg",
      "sm:text-display-xl",
      "text-metric",
      "text-micro-xs",
      "text-micro-sm",
    ]) {
      expect(html).toContain(token);
    }
    expect(html.match(/text-\[\d+px\]/g)).toEqual(["text-[122px]"]);
  });

  it("is read-only: the hero owns no controls", () => {
    const html = markup({ quoteNumber: "1502260012", nextRevision: 3 });

    expect(html).not.toMatch(/<(input|button|select|textarea)\b/);
  });

  it("stamps the QT watermark", () => {
    expect(markup()).toContain("QT");
  });
});
