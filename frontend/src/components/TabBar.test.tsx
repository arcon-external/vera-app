import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { TabBar, tabId, tabPanelId, type TabItem } from "./TabBar";

// The suite runs in node with no DOM (see vitest.config.ts), so the component
// is rendered to a string and asserted on as markup -- enough to catch a TabBar
// that throws during render or drops the tab roles the styles hang off.
//
// There is deliberately no `className` prop on TabBar: the top-level Settings
// bar and the Company sub-tabs must look identical, and a per-caller class
// override is how two tab bars drift apart. If a future caller needs extra
// classes, that is the moment to add the prop -- not before.

const ITEMS: TabItem[] = [
  { id: "general", label: "General" },
  { id: "company", label: "Company" },
  { id: "backups", label: "Backups" },
];

function render(props?: Partial<Parameters<typeof TabBar>[0]>): string {
  return renderToString(
    <TabBar
      items={ITEMS}
      active="company"
      onSelect={() => {}}
      label="Settings sections"
      {...props}
    />
  );
}

/** Splits the rendered output into one string per <button> element. */
function buttons(html: string): string[] {
  return html.split("<button").slice(1);
}

describe("TabBar", () => {
  it("renders a tablist with the given accessible name", () => {
    const html = render();
    expect(html).toContain('role="tablist"');
    expect(html).toContain('aria-label="Settings sections"');
  });

  it("renders one tab per item with its label", () => {
    const html = render();
    // role="tablist" must not be counted: match the closing quote.
    expect(html.split('role="tab"').length - 1).toBe(ITEMS.length);
    for (const item of ITEMS) {
      expect(html).toContain(`>${item.label}</button>`);
    }
  });

  it("marks exactly the active tab selected", () => {
    const html = render({ active: "company" });
    expect(html.split('aria-selected="true"').length - 1).toBe(1);
    expect(html.split('aria-selected="false"').length - 1).toBe(ITEMS.length - 1);
    const activeButton = buttons(html).find((segment) => segment.includes(">Company</button>"));
    expect(activeButton).toBeDefined();
    expect(activeButton).toContain('aria-selected="true"');
    for (const label of ["General", "Backups"]) {
      const button = buttons(html).find((segment) => segment.includes(`>${label}</button>`));
      expect(button).toBeDefined();
      expect(button).toContain('aria-selected="false"');
    }
  });

  it("renders no id or aria-controls without idPrefix", () => {
    const html = render();
    expect(html).not.toContain("id=");
    expect(html).not.toContain("aria-controls=");
  });

  it("wires tab ids to the panel helpers when idPrefix is set", () => {
    const prefix = "company";
    const html = render({ idPrefix: prefix });
    expect(tabId(prefix, "company")).toBe("company-tab-company");
    expect(tabPanelId(prefix, "company")).toBe("company-panel-company");
    const activeButton = buttons(html).find((segment) => segment.includes(">Company</button>"));
    expect(activeButton).toBeDefined();
    expect(activeButton).toContain(`id="${tabId(prefix, "company")}"`);
    expect(activeButton).toContain(`aria-controls="${tabPanelId(prefix, "company")}"`);
    for (const item of ITEMS) {
      const button = buttons(html).find((segment) => segment.includes(`>${item.label}</button>`));
      expect(button).toBeDefined();
      expect(button).toContain(`id="${tabId(prefix, item.id)}"`);
      expect(button).toContain(`aria-controls="${tabPanelId(prefix, item.id)}"`);
    }
  });

  it("keeps the exact button markup: type, classes, and tab roles", () => {
    const html = render();
    expect(html.split('type="button"').length - 1).toBe(ITEMS.length);
    const baseButton =
      "min-h-8 rounded-md px-4 py-1.5 text-small font-medium transition-colors duration-150 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40";
    for (const segment of buttons(html)) {
      expect(segment).toContain(baseButton);
    }
    const activeButton = buttons(html).find((segment) => segment.includes(">Company</button>"));
    expect(activeButton).toContain(`${baseButton} bg-white text-ink-950 shadow-card`);
    const inactiveButton = buttons(html).find((segment) => segment.includes(">General</button>"));
    expect(inactiveButton).toContain(`${baseButton} text-ink-500 hover:text-ink-800`);
  });

  it("keeps the exact wrapper class with nothing appended", () => {
    // The wrapper class is fixed by design so both tab bars stay identical.
    // Any caller-supplied extra class would show up here -- the assertion is
    // exact on purpose, so re-introducing a className passthrough fails loudly.
    const base = "inline-flex rounded-lg border border-rule bg-paper-muted p-0.5";
    expect(render()).toContain(`class="${base}"`);
    expect(render({ idPrefix: "company" })).toContain(`class="${base}"`);
  });

  it("renders without throwing for an empty items array", () => {
    expect(() => render({ items: [] })).not.toThrow();
    expect(render({ items: [] })).toContain('role="tablist"');
  });
});
