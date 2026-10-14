import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { CompanyLabelEntry } from "../companyLabels";
import { patchCompanyLabel } from "../companyLabels";
import { CompanyTemplatePanel } from "./CompanyTemplatePanel";

// Same house pattern as CompanyUnitsPanel.test.tsx: node, no DOM, the panel
// rendered to a string and asserted on as markup. The panel is a
// presentational slice (the parent owns the set and the autosave), so a loaded
// render needs only props.
//
// The fixture below is the served shape — the same entries GET
// /v1/company/labels answers. The wording in it is server data mirrored for the
// test, which is the point: nothing here asserts the document's words, only
// that whatever the server serves is what the panel renders. What is NOT
// proven here: the save round-trip and the debounce (need effects and the
// backend), and any interaction (needs a DOM) — those stay hand-driven.

const CLOSING =
  "We will be happy to supply information that you may need and trust " +
  "that you call on us to fill your order, which will receive our prompt " +
  "and careful attention.";

function entry(
  Key: string,
  Default: string,
  MaxLength: number,
  Terms = false,
  Value = Default
): CompanyLabelEntry {
  return { Key, Value, Default, MaxLength, Terms };
}

const SERVED: CompanyLabelEntry[] = [
  entry("docTitle", "QUOTATION", 24, false, "PROPOSAL"),
  entry("recipientHeading", "Quotation to", 60),
  entry("termsHeading", "Terms and Conditions", 60),
  entry("metaQuotationNo", "Quotation No.", 20),
  entry("metaIssueDate", "Issue Date", 20),
  entry("metaValidUntil", "Valid Until", 20),
  entry("metaRevision", "Revision", 20),
  entry("subject", "Subject", 30, false, "Chiller supply"),
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

function render(
  overrides: Partial<Parameters<typeof CompanyTemplatePanel>[0]> = {}
): string {
  return renderToString(
    <CompanyTemplatePanel
      labels={SERVED}
      saveState="saved"
      error={null}
      notice={null}
      onChange={() => {}}
      {...overrides}
    />
  );
}

/** Every <input> opening tag, in render order. */
function inputs(html: string): string[] {
  return html.split("<input").slice(1).map((segment) => segment.split(">")[0]);
}

/** The input opening tag whose accessible name is the served default. */
function inputFor(html: string, Default: string): string {
  const found = inputs(html).find((tag) =>
    tag.includes(`aria-label="Document wording: ${Default}"`)
  );
  expect(found, `expected an input named after the default ${Default} in: ${html}`).toBeDefined();
  return found as string;
}

describe("CompanyTemplatePanel", () => {
  it("renders the heading and the explainer", () => {
    const html = render();
    expect(html).toContain("Document wording");
    expect(html).toContain("What prints on every customer quotation");
    // The wording saves manually now (#432): the copy names the button rather
    // than promising an autosave that no longer exists.
    expect(html).toContain("saves with the button below");
    expect(html).not.toContain("save automatically");
  });

  it("renders one field per served entry — all 26, each with a distinct name", () => {
    const html = render();
    const names = inputs(html).map((tag) =>
      /aria-label="Document wording: ([^"]*)"/.exec(tag)?.[1]
    );
    expect(names).toHaveLength(26);
    expect(new Set(names).size).toBe(26);
    for (const item of SERVED) {
      expect(names, `expected a field for ${item.Key}`).toContain(item.Default);
    }
  });

  it("prefills from the served value, with the default as placeholder and the store's cap", () => {
    const html = render();
    for (const item of SERVED) {
      const tag = inputFor(html, item.Default);
      expect(tag, item.Key).toContain(`value="${item.Value}"`);
      expect(tag, item.Key).toContain(`placeholder="${item.Default}"`);
      expect(tag.toLowerCase(), item.Key).toContain(`maxlength="${item.MaxLength}"`);
    }
  });

  it("renders the five groups in document order", () => {
    const html = render();
    const order = ["Document", "Table columns", "Totals", "Terms", "Verification"].map((title) =>
      html.indexOf(`>${title}</h3>`)
    );
    for (const position of order) expect(position).toBeGreaterThan(-1);
    expect([...order].sort((a, b) => a - b)).toEqual(order);
  });

  it("gives the four terms sentences their placeholder hint, read off the served default", () => {
    const html = render();
    for (const token of ["{payment}", "{validity}", "{leadtime}", "{incoterm}"]) {
      expect(html).toContain(`Placeholders: ${token}`);
    }
    expect(html).toContain("Each fills in from the quotation when it prints.");
    // A non-terms label carries no placeholder hint at all.
    expect(html).not.toContain("Placeholders: {payment}, {validity}, {leadtime}, {incoterm}. Each");
  });

  it("marks a cleared field as built-in and offers Reset only on an override", () => {
    const cleared = render({ labels: patchCompanyLabel(SERVED, "columnQty", "") });
    // columnQty now prints the built-in "Qty.", so the row says so.
    expect(cleared).toContain("Built-in");
    expect(cleared).not.toContain('aria-label="Reset Qty. to the built-in wording"');
    // The two overridden rows are the only ones with a Reset.
    const resets = cleared.split('aria-label="Reset ').slice(1);
    expect(resets).toHaveLength(2);
    expect(cleared).toContain('aria-label="Reset QUOTATION to the built-in wording"');
    expect(cleared).toContain('aria-label="Reset Subject to the built-in wording"');
    // An untouched row shows neither.
    expect(cleared).not.toContain('aria-label="Reset UoM to the built-in wording"');
  });

  it("marks a value the store would refuse and says the save is held", () => {
    const html = render({ labels: patchCompanyLabel(SERVED, "docTitle", "<b>OFFER</b>") });
    const tag = inputFor(html, "QUOTATION");
    expect(tag).toContain('aria-invalid="true"');
    // The escaped form is asserted on purpose: the message prints the value's
    // offence, and React escaping it is what keeps the refusal text inert.
    expect(html).toContain(
      'label &quot;docTitle&quot; may not contain &quot;&lt;&quot; or &quot;&gt;&quot;: it is printed as plain text'
    );
    expect(html).toContain("must be fixed before anything saves");
  });

  it("shows the live save state", () => {
    expect(render({ saveState: "saved" })).toContain("Saved");
    expect(render({ saveState: "dirty" })).toContain("Unsaved changes");
    expect(render({ saveState: "saving" })).toContain("Saving…");
    // The readout is a live region: a silent repaint would not announce it.
    expect(render({ saveState: "dirty" })).toContain('role="status"');
  });

  it("shows an error banner and a note when one is passed", () => {
    const failed = render({ error: "The server refused the wording." });
    expect(failed).toContain("The server refused the wording.");
    expect(failed).toContain('role="alert"');
    const noted = render({ notice: "Saved, but this device may keep the old wording." });
    expect(noted).toContain("Saved, but this device may keep the old wording.");
  });

  it("stays out of the profile form: no form, no submit, Reset is a plain button", () => {
    const html = render();
    expect(html).not.toContain("<form");
    expect(html).not.toContain('type="submit"');
    const reset = html.split("<button").slice(1).find((segment) => segment.includes(">Reset</button>"));
    expect(reset).toBeDefined();
    expect(reset).toContain('type="button"');
  });

  it("spans the two-column grid for the long fields only", () => {
    // The closing sentence and the four terms sentences; every other row is
    // half width.
    expect(render().split("sm:col-span-2").length - 1).toBe(5);
  });

  it("lets every field row shrink, so one long served label cannot widen the track", () => {
    // A grid item will not shrink past its content's intrinsic width without
    // min-w-0, and the closing label is one long served sentence shown on a
    // single line: its min-content is the whole sentence, so a row missing it
    // sizes the single-column track to that sentence and every field in the
    // panel overflows the card at 360px (scripts/ui-snapshot.sh, the 360 PNG).
    const html = render();
    expect(html.split('<div class="min-w-0').length - 1).toBe(inputs(html).length);
  });

  it("shows a loading skeleton while the served set is null", () => {
    const html = render({ labels: null });
    expect(html).toContain('aria-label="Loading document wording"');
    expect(inputs(html)).toHaveLength(0);
  });

  it("renders a key it does not group in the Other group instead of dropping it", () => {
    const html = render({ labels: [...SERVED, entry("siteHeading", "Site", 40)] });
    expect(html).toContain(">Other</h3>");
    expect(html).toContain('aria-label="Document wording: Site"');
    expect(inputs(html)).toHaveLength(27);
  });

  it("wires the change handler without calling it during render", () => {
    const onChange = vi.fn();
    render({ onChange });
    expect(onChange).not.toHaveBeenCalled();
  });
});
