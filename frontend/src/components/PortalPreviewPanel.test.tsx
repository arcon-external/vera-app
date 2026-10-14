import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DEFAULT_THEME, EMPTY_THEME, type CompanyTheme } from "../company";
import { PortalPreviewPanel, portalColors, portalPreviewArgs } from "./PortalPreviewPanel";

// Same house pattern as the other settings panels: node, no DOM, the panel
// rendered to a string and asserted on as markup.
//
// What this test does NOT prove, stated plainly: the fetch itself, the debounce
// and the iframe all need effects, which renderToString never runs — so the
// rendered markup is the panel's loading state. What is pinned instead is
// everything that can be pinned without them: the five states exist as buttons,
// exactly one is pressed at a time, the wording that must never become editable
// is on the page, and the colour mapping reads the portal's four keys rather
// than the quotation's.

function render(theme: CompanyTheme = DEFAULT_THEME): string {
  return renderToString(<PortalPreviewPanel theme={theme} />);
}

describe("portal preview states", () => {
  it("offers all five states, with Valid pressed on first render", () => {
    const html = render();
    const buttons = html.split("<button").slice(1);
    const pressed = buttons.filter((segment) => segment.includes('aria-pressed="true"'));
    expect(pressed).toHaveLength(1);
    expect(pressed[0]).toContain(">Valid</button>");
    for (const label of ["Valid", "Superseded", "Mismatch", "Not found", "Unavailable"]) {
      expect(
        buttons.find((segment) => segment.includes(`>${label}</button>`)),
        `expected a state button labelled ${label} in: ${html}`
      ).toBeDefined();
    }
  });

  it("groups the toggle under one accessible name", () => {
    const html = render();
    expect(html).toContain('role="group"');
    expect(html).toContain('aria-label="Verification state"');
  });

  it("shows a pending document, not an empty frame, until the first render lands", () => {
    // Effects never run under renderToString, so the panel is exactly where a
    // slow endpoint would leave it. An empty iframe here would be a blank
    // rectangle in the settings page.
    const html = render();
    expect(html).toContain('aria-label="Loading portal preview"');
    expect(html).not.toContain("<iframe");
  });

  it("keeps the wording fixed and says why", () => {
    const html = render();
    expect(html).toContain("wording of the portal is fixed on purpose");
    expect(html).toContain("Only the four colours are editable.");
    // The sentence that must never become an input.
    expect(html).toContain("does not match our records");
  });
});

describe("portal colour mapping", () => {
  it("reads the portal's four colours, not the quotation's", () => {
    const theme: CompanyTheme = {
      ...EMPTY_THEME,
      qDeep: "#111111",
      qAccent: "#222222",
      qSoft: "#333333",
      qInk: "#444444",
      pDeep: "#1A237E",
      pAccent: "#284AF3",
      pSoft: "#D6E4FF",
      pInk: "#1a1a1a",
    };
    expect(portalColors(theme)).toEqual(["#1A237E", "#284AF3", "#D6E4FF", "#1a1a1a"]);
    expect(portalPreviewArgs("mismatch", portalColors(theme))).toEqual([
      "mismatch",
      "#1A237E",
      "#284AF3",
      "#D6E4FF",
      "#1a1a1a",
    ]);
  });
});
