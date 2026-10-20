import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DocumentHomePage } from "./DocumentHomePage";

// Issues #466/#469: after sign-in staff land on a document home with four
// destinations. Quotation and the PO archive open their pages; Delivery Order
// and Invoice are visible but disabled with "Coming later".
//
// The suite runs in node with no DOM (see vitest.config.ts), so the pages are
// rendered to a string and asserted on as markup -- the same house pattern as
// ui.test.tsx and TabBar.test.tsx. renderToString prints the attributes React
// will actually set on the node, so `disabled` in the output is the button's
// real disabled state, not a source scan.
//
// What this test does NOT prove: that a click reaches the right page, that
// focus order is sane, or that the assembled page reads well at 360/1280.
// Those stay hand-driven plus the ui-snapshot harness case.

/** Splits the rendered output into one string per <button> element. */
function buttons(html: string): string[] {
  return html.split("<button").slice(1);
}

/** The opening tag of the <button> carrying the given accessible label. Throws
 *  when no such button exists, so a destination that stops rendering as a
 *  button fails loudly rather than matching nothing. Only the opening tag is
 *  returned, so trailing card markup can never leak another button's
 *  attributes into the assertion. */
function buttonTagFor(html: string, label: string): string {
  const found = buttons(html).find((chunk) =>
    chunk.slice(0, chunk.indexOf(">")).includes(`aria-label="${label}"`)
  );
  if (!found)
    throw new Error(`no <button aria-label="${label}"> in: ${html}`);
  return found.slice(0, found.indexOf(">") + 1);
}

function renderHome(): string {
  return renderToString(
    <DocumentHomePage
      onOpenQuotation={() => {}}
      onOpenPurchaseOrder={() => {}}
    />
  ).replace(/<!-- -->/g, "");
}

describe("DocumentHomePage destinations (issues #466, #469)", () => {
  it("shows all four document destinations", () => {
    const html = renderHome();
    for (const label of [
      "Open Quotation",
      "Find purchase orders",
      "Delivery Order (coming later)",
      "Invoice (coming later)",
    ]) {
      expect(buttonTagFor(html, label)).toContain(label);
    }
  });

  it("keeps Quotation and the PO archive enabled", () => {
    // The attribute, not the substring: the house Button's class list always
    // carries `disabled:cursor-not-allowed` and friends, so a bare "disabled"
    // match would pass on a button that is actually disabled.
    const html = renderHome();
    expect(buttonTagFor(html, "Open Quotation")).not.toContain(" disabled=");
    expect(buttonTagFor(html, "Find purchase orders")).not.toContain(
      " disabled="
    );
  });

  it("names the PO surface for what it does, without pretending later slices exist", () => {
    const html = renderHome();
    expect(html).toContain(
      "Find purchase orders and enter a new one against an issued quotation."
    );
    // No attachments, history, fulfilment or payment vocabulary belongs on
    // the home card: none of those is implemented.
    expect(html).not.toContain("Attachment");
    expect(html).not.toContain("Payment status");
    expect(html).not.toContain("Fulfilment");
  });

  it("renders Delivery Order and Invoice as truly disabled buttons marked Coming later", () => {
    // The key guard: a future restyle that drops `disabled` (leaving only a
    // grey look, or an aria-disabled div that still fires onClick) must fail
    // here. renderToString prints the real disabled attribute, and a <button>
    // with it cannot be clicked, focused, or submitted with -- keyboard and
    // mouse alike stay out.
    const html = renderHome();
    for (const label of [
      "Delivery Order (coming later)",
      "Invoice (coming later)",
    ]) {
      const tag = buttonTagFor(html, label);
      expect(tag).toContain(" disabled=");
      expect(tag).toContain('aria-disabled="true"');
    }
    expect(html).toContain("Coming later");
  });
});
