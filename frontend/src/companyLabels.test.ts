import { describe, expect, it, vi } from "vitest";
import {
  companyLabelPayload,
  companyLabelsRefusal,
  groupCompanyLabels,
  patchCompanyLabel,
  placeholderTokens,
  serializeCompanyLabels,
  validateCompanyLabelSet,
  type CompanyLabelEntry,
} from "./companyLabels";

// The served shape, as GET /v1/company/labels answers it: every label in
// document order, each carrying its built-in default (the wording lives on the
// server, so this fixture mirrors it — the fields are pinned against the Go
// declaration in backend/labels, which is what a real server serves). Two
// entries are overridden so the override paths are exercised; the rest print
// their built-in default.
function entry(
  Key: string,
  Default: string,
  MaxLength: number,
  Terms = false,
  Value = Default
): CompanyLabelEntry {
  return { Key, Value, Default, MaxLength, Terms };
}

const CLOSING =
  "We will be happy to supply information that you may need and trust " +
  "that you call on us to fill your order, which will receive our prompt " +
  "and careful attention.";

function served(): CompanyLabelEntry[] {
  return [
    entry("docTitle", "QUOTATION", 24, false, "PROPOSAL"),
    entry("recipientHeading", "Quotation to", 60),
    entry("termsHeading", "Terms and Conditions", 60),
    entry("metaQuotationNo", "Quotation No.", 20),
    entry("metaIssueDate", "Issue Date", 20),
    entry("metaValidUntil", "Valid Until", 20),
    entry("metaRevision", "Revision", 20),
    entry("subject", "Subject", 30),
    entry("columnNo", "No.", 20),
    entry("columnDescription", "Description", 20),
    entry("columnQty", "Qty.", 20),
    entry("columnUoM", "UoM", 20),
    entry("columnUnitPrice", "Unit Price", 20),
    entry("columnAfterDisc", "After Disc.", 20),
    entry("columnTotal", "Total", 20),
    entry("totalSubTotal", "Sub Total", 60),
    entry("totalDiscount", "Discount", 60),
    entry("totalPpn", "PPN 11%", 60),
    entry("totalGrandTotal", "Grand Total", 60),
    entry("closing", CLOSING, 300),
    entry("verifyScan", "Scan to verify this quotation", 60),
    entry("verifyCode", "or enter code", 60),
    entry("termsPayment", "1. Payment terms agreed for this quotation is {payment} days.", 300, true),
    entry("termsValidity", "2. This quotation is valid for {validity} days.", 300, true),
    entry("termsLeadTime", "3. Lead time is {leadtime}.", 300, true),
    entry("termsIncoterm", "4. Incoterms: {incoterm}", 300, true),
  ];
}

const ALLOWED_TOKENS = "{payment}, {validity}, {leadtime}, {incoterm}";

describe("groupCompanyLabels", () => {
  it("puts every served entry in exactly one group, preserving served order within each", () => {
    const entries = served();
    const groups = groupCompanyLabels(entries);
    expect(groups.map((group) => group.id)).toEqual([
      "document",
      "columns",
      "totals",
      "terms",
      "verification",
    ]);
    const flat = groups.flatMap((group) => group.entries.map((entry) => entry.Key));
    expect(flat).toHaveLength(26);
    expect(new Set(flat).size).toBe(26);
    expect(groups.map((group) => group.entries.length)).toEqual([9, 7, 4, 4, 2]);
    // The group a key lands in is a presentation choice (the closing sentence
    // reads with the document but is served after the columns); what must hold
    // is that no group reorders what the server served it.
    for (const group of groups) {
      const servedOrder = entries
        .filter((entry) => group.entries.some((owned) => owned.Key === entry.Key))
        .map((entry) => entry.Key);
      expect(group.entries.map((entry) => entry.Key)).toEqual(servedOrder);
    }
  });

  it("shows a key it does not group in a visible fallback group instead of dropping it", () => {
    const groups = groupCompanyLabels([...served(), entry("siteHeading", "Site", 40)]);
    const other = groups.find((group) => group.id === "other");
    expect(other).toBeDefined();
    expect(other?.entries.map((entry) => entry.Key)).toEqual(["siteHeading"]);
    expect(groups.flatMap((group) => group.entries)).toHaveLength(27);
  });

  it("drops a group with nothing in it rather than printing an empty heading", () => {
    const groups = groupCompanyLabels([entry("docTitle", "QUOTATION", 24)]);
    expect(groups.map((group) => group.id)).toEqual(["document"]);
  });
});

describe("patchCompanyLabel", () => {
  it("replaces one value and leaves every other entry untouched", () => {
    const patched = patchCompanyLabel(served(), "subject", "Chiller supply");
    expect(patched.find((entry) => entry.Key === "subject")?.Value).toBe("Chiller supply");
    expect(patched.filter((entry) => entry.Key !== "subject")).toEqual(
      served().filter((entry) => entry.Key !== "subject")
    );
    expect(patched).toHaveLength(26);
  });

  it("is a no-op for a key the document does not carry", () => {
    const entries = served();
    expect(patchCompanyLabel(entries, "nope", "x")).toEqual(entries);
  });
});

describe("companyLabelPayload", () => {
  it("sends every key with its value, in served order", () => {
    const payload = companyLabelPayload(served());
    expect(payload).toHaveLength(26);
    expect(payload[0]).toEqual({ Key: "docTitle", Value: "PROPOSAL" });
    expect(payload[1]).toEqual({ Key: "recipientHeading", Value: "Quotation to" });
    expect(payload.map((row) => row.Key)).toEqual(served().map((entry) => entry.Key));
  });

  it("sends a cleared label as an empty value, which is how the store clears it", () => {
    const payload = companyLabelPayload(patchCompanyLabel(served(), "docTitle", ""));
    expect(payload.find((row) => row.Key === "docTitle")).toEqual({ Key: "docTitle", Value: "" });
  });
});

describe("serializeCompanyLabels", () => {
  it("is stable for equal sets and changes when a value changes", () => {
    expect(serializeCompanyLabels(served())).toBe(serializeCompanyLabels(served()));
    expect(serializeCompanyLabels(served())).not.toBe(
      serializeCompanyLabels(patchCompanyLabel(served(), "subject", "Chiller supply"))
    );
  });
});

describe("placeholderTokens", () => {
  it("reads the tokens off the served default rather than hard-coding them", () => {
    const terms = served().filter((entry) => entry.Terms);
    expect(terms.map((entry) => placeholderTokens(entry))).toEqual([
      ["{payment}"],
      ["{validity}"],
      ["{leadtime}"],
      ["{incoterm}"],
    ]);
  });

  it("finds nothing in a label that carries no placeholder", () => {
    expect(placeholderTokens(entry("subject", "Subject", 30))).toEqual([]);
  });
});

describe("validateCompanyLabelSet mirrors the store", () => {
  it("accepts the served set and a two-token terms sentence", () => {
    expect(validateCompanyLabelSet(served())).toEqual({});
    const combined = patchCompanyLabel(
      served(),
      "termsPayment",
      "1. Payment terms agreed for this quotation is {payment} days, valid for {validity} days."
    );
    expect(validateCompanyLabelSet(combined)).toEqual({});
  });

  it("names the label over the cap", () => {
    const invalid = validateCompanyLabelSet(
      patchCompanyLabel(served(), "docTitle", "P".repeat(25))
    );
    expect(invalid.docTitle).toBe('label "docTitle" is over 24 characters');
  });

  it("refuses markup, template actions and control characters", () => {
    expect(
      validateCompanyLabelSet(patchCompanyLabel(served(), "docTitle", "<b>QUOTATION</b>")).docTitle
    ).toBe('label "docTitle" may not contain "<" or ">": it is printed as plain text');
    expect(
      validateCompanyLabelSet(patchCompanyLabel(served(), "docTitle", "{{.DocTitle}}")).docTitle
    ).toBe('label "docTitle" may not contain "{{": it is printed as plain text');
    expect(
      validateCompanyLabelSet(patchCompanyLabel(served(), "docTitle", "QUO\nTATION")).docTitle
    ).toBe('label "docTitle" may not contain control characters: it prints on one line');
  });

  it("checks placeholders only on the terms sentences", () => {
    // Braces are plain text everywhere except the four terms lines, exactly as
    // backend/labels.ValidateOverride treats them.
    expect(
      validateCompanyLabelSet(patchCompanyLabel(served(), "subject", "Subject {payment}"))
    ).toEqual({});
    expect(
      validateCompanyLabelSet(
        patchCompanyLabel(served(), "termsPayment", "1. Payment terms are {duedate} days.")
      ).termsPayment
    ).toBe(
      `label "termsPayment" uses the unknown placeholder {duedate}: use one of ${ALLOWED_TOKENS}`
    );
    expect(
      validateCompanyLabelSet(
        patchCompanyLabel(served(), "termsPayment", "1. Payment terms are {payment days.")
      ).termsPayment
    ).toBe(`label "termsPayment" has an unclosed placeholder: use one of ${ALLOWED_TOKENS}`);
    expect(
      validateCompanyLabelSet(
        patchCompanyLabel(served(), "termsPayment", "2. This quotation is valid for {validity} days}.")
      ).termsPayment
    ).toBe(`label "termsPayment" has a stray "{" or "}": use one of ${ALLOWED_TOKENS}`);
  });
});

describe("companyLabelsRefusal", () => {
  // The wording saves manually now (#432), so the refusal that used to hold the
  // autosave ("held" — a set the store would refuse never leaves the machine)
  // disables the save row instead. Same rule, same mirror of the store; it is
  // pinned here and the sequence that used to carry it lives in
  // companySaves.test.ts.

  it("lets a served set be saved", () => {
    expect(companyLabelsRefusal(served())).toBeNull();
  });

  it("names the offending rows, singular and plural, exactly as the panel says", () => {
    const one = patchCompanyLabel(served(), "docTitle", "{{.DocTitle}}");
    expect(companyLabelsRefusal(one)).toBe(
      "One field is highlighted below and must be fixed before anything saves."
    );
    const two = patchCompanyLabel(patchCompanyLabel(served(), "docTitle", "{{.DocTitle}}"), "subject", "<b>x</b>");
    expect(companyLabelsRefusal(two)).toBe(
      "2 fields are highlighted below and must be fixed before anything saves."
    );
    // The per-key map it is built from still points at the row.
    expect(Object.keys(validateCompanyLabelSet(two)).sort()).toEqual(["docTitle", "subject"]);
  });

  it("refuses for the same reasons the store does, not only markup", () => {
    // An over-long value, an unknown placeholder and a stray brace are all
    // store refusals; each must disable the button rather than bounce.
    expect(companyLabelsRefusal(patchCompanyLabel(served(), "docTitle", "x".repeat(25)))).toContain(
      "must be fixed before anything saves"
    );
    expect(
      companyLabelsRefusal(patchCompanyLabel(served(), "termsPayment", "1. {duedate} days."))
    ).toContain("must be fixed before anything saves");
    expect(
      companyLabelsRefusal(patchCompanyLabel(served(), "termsValidity", "2. {validity days}"))
    ).toContain("must be fixed before anything saves");
  });
});

