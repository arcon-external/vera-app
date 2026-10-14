import { describe, expect, it, vi } from "vitest";
import {
  LABEL_SAVE_DEBOUNCE_MS,
  companyLabelPayload,
  groupCompanyLabels,
  labelSaveDecision,
  patchCompanyLabel,
  placeholderTokens,
  saveCompanyLabels,
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

describe("saveCompanyLabels", () => {
  type Row = { Key: string; Value: string };
  const deps = () => {
    const calls: string[] = [];
    return {
      calls,
      update: vi.fn(async (_rows: Row[]) => {
        calls.push("update");
      }),
      refreshIdentity: vi.fn(async () => {
        calls.push("refresh");
      }),
    };
  };

  it("sends the full replacement, then refreshes the identity that order", async () => {
    const fake = deps();
    const entries = patchCompanyLabel(served(), "docTitle", "");
    const result = await saveCompanyLabels(entries, fake);
    expect(fake.update).toHaveBeenCalledTimes(1);
    expect(fake.update.mock.calls[0][0]).toEqual(companyLabelPayload(entries));
    expect(fake.update.mock.calls[0][0]).toHaveLength(26);
    expect(fake.update.mock.calls[0][0][0]).toEqual({ Key: "docTitle", Value: "" });
    expect(fake.refreshIdentity).toHaveBeenCalledTimes(1);
    expect(fake.calls).toEqual(["update", "refresh"]);
    expect(result).toEqual({ refreshFailed: false });
  });

  it("never refreshes when the save was refused, and surfaces the refusal", async () => {
    const fake = deps();
    fake.update = vi.fn(async () => {
      fake.calls.push("update");
      throw new Error('label "docTitle" is over 24 characters');
    });
    await expect(saveCompanyLabels(served(), fake)).rejects.toThrow("over 24 characters");
    expect(fake.refreshIdentity).not.toHaveBeenCalled();
  });

  it("reports a failed refresh instead of throwing: the wording already changed", async () => {
    const fake = deps();
    fake.refreshIdentity = vi.fn(async () => {
      fake.calls.push("refresh");
      throw new Error("offline");
    });
    const result = await saveCompanyLabels(served(), fake);
    expect(result).toEqual({ refreshFailed: true });
    expect(fake.update).toHaveBeenCalledTimes(1);
  });
});

describe("labelSaveDecision drives the autosave", () => {
  it("does nothing for a set that matches what the server holds", () => {
    expect(labelSaveDecision(served(), serializeCompanyLabels(served()))).toBe("unchanged");
  });

  it("holds a set carrying a value the store would refuse", () => {
    const edited = patchCompanyLabel(served(), "docTitle", "{{.DocTitle}}");
    expect(labelSaveDecision(edited, serializeCompanyLabels(served()))).toBe("held");
  });

  it("saves an edited, valid set", () => {
    const edited = patchCompanyLabel(served(), "docTitle", "QUOTE");
    expect(labelSaveDecision(edited, serializeCompanyLabels(served()))).toBe("save");
  });
});

describe("LABEL_SAVE_DEBOUNCE_MS", () => {
  it("is a real pause, not a save per keystroke", () => {
    expect(LABEL_SAVE_DEBOUNCE_MS).toBeGreaterThanOrEqual(300);
    expect(LABEL_SAVE_DEBOUNCE_MS).toBeLessThanOrEqual(2000);
  });
});
