import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ToastProvider } from "../toast";
import { CompanyProfileCard } from "./SettingsPage";
import { tabId, tabPanelId } from "./TabBar";

// The suite runs in node with no DOM (see vitest.config.ts), so the component
// is rendered to a string and asserted on as markup -- the same house pattern
// as ReviewPanel.test.tsx and CompanyGeneralPanel.test.tsx. No DOM, no
// testing-library, no new dependency.
//
// Effects never run under renderToString, so load()/loadLogo() never fire and
// `loading` stays true: the first render shows the shared <form> root with
// the sub-tabs, the tabpanel shell holding the skeleton, and the save button
// disabled — which is exactly what these tests pin. There is no separate
// loading branch anymore (loading and loaded share one root), so "first
// render" here means the loading state of the single shape. The Wails
// bindings are imported but never called (calls happen only inside
// effects and handlers), and every binding touches window only when called,
// so importing SettingsPage in node is safe. CompanyProfileCard calls
// useToast, so it renders inside a ToastProvider.
//
// What this test does NOT prove: tab switching (needs clicks), the loaded
// panels (needs effects), or the save round-trip (needs the backend). Those
// stay hand-driven.

function render(): string {
  return renderToString(
    <ToastProvider>
      <CompanyProfileCard />
    </ToastProvider>
  );
}

/** Splits the rendered output into one string per <button> element. */
function buttons(html: string): string[] {
  return html.split("<button").slice(1);
}

/** The three sub-tab buttons: role="tab" excludes the role="tablist" wrapper
 *  by matching the closing quote. */
function tabs(html: string): string[] {
  return buttons(html).filter((segment) => segment.includes('role="tab"'));
}

describe("CompanyProfileCard sub-tabs", () => {
  it("renders four sub-tabs labelled General, Units, Quotation and Portal, each role=tab", () => {
    const html = render();
    const found = tabs(html);
    expect(found).toHaveLength(4);
    for (const label of ["General", "Units", "Quotation", "Portal"]) {
      expect(
        found.find((segment) => segment.includes(`>${label}</button>`)),
        `expected a tab labelled ${label} in: ${html}`
      ).toBeDefined();
    }
  });

  it("marks exactly one tab selected, and it is General on first render", () => {
    const html = render();
    expect(html.split('aria-selected="true"').length - 1).toBe(1);
    const general = tabs(html).find((segment) => segment.includes(">General</button>"));
    expect(general).toBeDefined();
    expect(general).toContain('aria-selected="true"');
    for (const label of ["Quotation", "Portal"]) {
      const button = tabs(html).find((segment) => segment.includes(`>${label}</button>`));
      expect(button).toBeDefined();
      expect(button).toContain('aria-selected="false"');
    }
  });

  it("labels the tablist Company sections and wires tab ids through tabId/tabPanelId", () => {
    const html = render();
    expect(html).toContain('role="tablist"');
    expect(html).toContain('aria-label="Company sections"');
    for (const [id, label] of [["general", "General"], ["units", "Units"], ["quotation", "Quotation"], ["portal", "Portal"]]) {
      const button = tabs(html).find((segment) => segment.includes(`>${label}</button>`));
      expect(button).toBeDefined();
      expect(button).toContain(`id="${tabId("company", id)}"`);
      expect(button).toContain(`aria-controls="${tabPanelId("company", id)}"`);
    }
  });

  it("renders a real tabpanel for the active tab, not a decorative one", () => {
    const html = render();
    // The general tab's button points at the panel...
    const general = tabs(html).find((segment) => segment.includes(">General</button>"));
    expect(general).toContain(`aria-controls="${tabPanelId("company", "general")}"`);
    // ...and a role=tabpanel element answers with the matching id and owner.
    expect(html).toContain('role="tabpanel"');
    expect(html).toContain(`id="${tabPanelId("company", "general")}"`);
    expect(html).toContain(`aria-labelledby="${tabId("company", "general")}"`);
  });

  it("renders a disabled save button inside the shared form root while loading", () => {
    const html = render();
    // Loading and loaded states share one <form> root, so flipping `loading`
    // swaps the panel content without remounting (and without dropping focus):
    // the form — and the save button inside it — must already exist here.
    expect(html).toContain("<form");
    expect(html).toContain("</form>");
    expect(html).toContain('aria-label="Loading company profile"');
    const save = buttons(html).find((segment) =>
      segment.includes(">Save company profile</button>")
    );
    expect(save).toBeDefined();
    expect(save).toContain('type="submit"');
    expect(save).toContain("disabled");
    expect(html.indexOf("<form")).toBeLessThan(html.indexOf("Save company profile"));
  });

  it("renders without throwing", () => {
    let html = "";
    expect(() => {
      html = render();
    }).not.toThrow();
    expect(html).toContain("Company sections");
  });
});
