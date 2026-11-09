import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DocumentHomePage } from "./DocumentHomePage";

// Issue #491: Home is the global workspace dashboard. Five cards at admin
// (Quotation, Purchase Order, Delivery Order disabled, Invoice disabled,
// Admin Dashboard) and four for staff; each carries an inline-SVG icon.
//
// The suite runs in node with no DOM (see vitest.config.ts), so the page is
// rendered to a string and asserted on as markup -- the same house pattern as
// ui.test.tsx and TabBar.test.tsx.
//
// What this test does NOT prove: that a click reaches the right workspace, the
// focus order, or that the assembled page reads well at 360/1280. Those stay
// hand-driven plus the ui-snapshot harness cases.

/** Splits the rendered output into one string per <button> element. */
function buttons(html: string): string[] {
  return html.split("<button").slice(1);
}

/** The opening tag of the <button> carrying the given accessible label. */
function buttonTagFor(html: string, label: string): string {
  const found = buttons(html).find((chunk) =>
    chunk.slice(0, chunk.indexOf(">")).includes(`aria-label="${label}"`)
  );
  if (!found) throw new Error(`no <button aria-label="${label}"> in: ${html}`);
  return found.slice(0, found.indexOf(">") + 1);
}

function renderHome(isAdmin = true): string {
  return renderToString(
    <DocumentHomePage
      isAdmin={isAdmin}
      onOpenQuotation={() => {}}
      onOpenPurchaseOrder={() => {}}
      onOpenAdmin={() => {}}
    />
  ).replace(/<!-- -->/g, "");
}

describe("DocumentHomePage workspaces (issue #491)", () => {
  it("labels the section Workspaces", () => {
    expect(renderHome()).toContain(">Workspaces<");
  });

  it("shows all five workspaces to an admin", () => {
    const html = renderHome();
    for (const label of [
      "Open Quotation",
      "Open Purchase Orders",
      "Delivery Order (coming later)",
      "Invoice (coming later)",
      "Open Admin Dashboard",
    ]) {
      expect(buttonTagFor(html, label)).toContain(label);
    }
  });

  it("keeps Quotation, Purchase Orders and Admin enabled", () => {
    const html = renderHome();
    for (const label of [
      "Open Quotation",
      "Open Purchase Orders",
      "Open Admin Dashboard",
    ]) {
      expect(buttonTagFor(html, label)).not.toContain(" disabled=");
    }
  });

  it("renders Delivery Order and Invoice as truly disabled with Coming later", () => {
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

  it("gives every card an inline-SVG icon", () => {
    const html = renderHome();
    // Five cards, five 24×24 inline icons; the house Button draws none.
    expect(html.split('viewBox="0 0 24 24"').length - 1).toBe(5);
  });

  it("lays out two columns at wide with Admin full-width, one at 360", () => {
    const html = renderHome();
    expect(html).toContain("sm:grid-cols-2");
    expect(html).toContain("sm:col-span-2");
  });

  it("hides the Admin card for a non-admin", () => {
    const html = renderHome(false);
    expect(html).not.toContain("Open Admin Dashboard");
    expect(html).not.toContain(">Admin Dashboard<");
    // Four cards, four icons; the other destinations are untouched.
    expect(html.split('viewBox="0 0 24 24"').length - 1).toBe(4);
    expect(buttonTagFor(html, "Open Quotation")).toBeDefined();
    expect(buttonTagFor(html, "Open Purchase Orders")).toBeDefined();
  });
});
