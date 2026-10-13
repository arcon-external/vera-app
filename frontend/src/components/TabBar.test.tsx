import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { TabBar, tabAfterKey, tabId, tabPanelId, type TabItem } from "./TabBar";

// The suite runs in node with no DOM (see vitest.config.ts), so the component
// is rendered to a string and asserted on as markup -- enough to catch a TabBar
// that throws during render or drops the tab roles the styles hang off.
//
// There is deliberately no `className` prop on TabBar: the top-level Settings
// bar and the Company sub-tabs must look identical, and a per-caller class
// override is how two tab bars drift apart. If a future caller needs extra
// classes, that is the moment to add the prop -- not before.
//
// The keyboard is pinned in two parts, and the split is stated rather than
// glossed: WHAT is pinned here is the output markup -- the roving tabindex
// (one tab stop, on the active tab) -- and the navigation decision itself,
// `tabAfterKey`, which is a pure function of (items, active, key) so node can
// run it. What is NOT pinned: that React actually attaches onKeyDown to the
// tablist, that focus() lands on the right node when a key moves the
// selection, and that the browser suppresses arrow-key scrolling. Those need
// a DOM and stay hand-driven -- confirmed by tabbing into the Settings tab
// bars and pressing the arrows/Home/End in the running app.

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

describe("TabBar roving tabindex (WAI-ARIA tabs pattern)", () => {
  // The tablist is one tab stop, not one per tab: Tab moves focus in, the
  // arrows move within. Losing the tabIndex turns every tab into its own
  // stop, which is what these pins catch.
  it("gives exactly one tab stop, and it is the active tab", () => {
    const html = render();
    expect(html.split('tabindex="0"').length - 1).toBe(1);
    expect(html.split('tabindex="-1"').length - 1).toBe(ITEMS.length - 1);
    const activeButton = buttons(html).find((segment) => segment.includes(">Company</button>"));
    expect(activeButton).toContain('tabindex="0"');
    for (const label of ["General", "Backups"]) {
      const button = buttons(html).find((segment) => segment.includes(`>${label}</button>`));
      expect(button).toContain('tabindex="-1"');
    }
  });

  it("follows the active prop when it moves", () => {
    const html = render({ active: "general" });
    expect(html.split('tabindex="0"').length - 1).toBe(1);
    const first = buttons(html).find((segment) => segment.includes(">General</button>"));
    expect(first).toContain('tabindex="0"');
    for (const label of ["Company", "Backups"]) {
      const button = buttons(html).find((segment) => segment.includes(`>${label}</button>`));
      expect(button).toContain('tabindex="-1"');
    }
  });

  it("keeps the single tab stop when idPrefix is set, wiring intact (both call sites)", () => {
    // The Company sub-tabs are the same component with idPrefix set: identical
    // roving tabindex, and the id/aria-controls/role wiring untouched.
    const html = render({ idPrefix: "company" });
    expect(html.split('tabindex="0"').length - 1).toBe(1);
    expect(html.split('tabindex="-1"').length - 1).toBe(ITEMS.length - 1);
    const activeButton = buttons(html).find((segment) => segment.includes(">Company</button>"));
    expect(activeButton).toContain('tabindex="0"');
    expect(activeButton).toContain('role="tab"');
    expect(activeButton).toContain('aria-selected="true"');
    expect(activeButton).toContain(`id="${tabId("company", "company")}"`);
    expect(activeButton).toContain(`aria-controls="${tabPanelId("company", "company")}"`);
  });

  it("parks the tab stop on the first tab when active matches no item", () => {
    // A caller whose active id is gone (a filtered-away tab, a stale state)
    // must not strand the bar with zero tab stops -- keyboard unreachable.
    const html = render({ active: "does-not-exist" });
    expect(html.split('tabindex="0"').length - 1).toBe(1);
    expect(html.split('tabindex="-1"').length - 1).toBe(ITEMS.length - 1);
    const first = buttons(html).find((segment) => segment.includes(">General</button>"));
    expect(first).toContain('tabindex="0"');
  });
});

describe("tabAfterKey", () => {
  // The navigation the tablist's key handler performs, as a pure function.
  // Arrows wrap (first -> last, last -> first); Home/End jump to the ends.
  it("moves left and right with wrapping", () => {
    expect(tabAfterKey(ITEMS, "general", "ArrowRight")).toBe("company");
    expect(tabAfterKey(ITEMS, "company", "ArrowRight")).toBe("backups");
    expect(tabAfterKey(ITEMS, "backups", "ArrowRight")).toBe("general");
    expect(tabAfterKey(ITEMS, "company", "ArrowLeft")).toBe("general");
    expect(tabAfterKey(ITEMS, "general", "ArrowLeft")).toBe("backups");
    expect(tabAfterKey(ITEMS, "backups", "ArrowLeft")).toBe("company");
  });

  it("never lands back on the tab it started from when there is more than one", () => {
    for (const key of ["ArrowLeft", "ArrowRight"]) {
      for (const item of ITEMS) {
        expect(tabAfterKey(ITEMS, item.id, key)).not.toBe(item.id);
      }
    }
  });

  it("jumps to the first and last tab with Home and End", () => {
    expect(tabAfterKey(ITEMS, "backups", "Home")).toBe("general");
    expect(tabAfterKey(ITEMS, "company", "Home")).toBe("general");
    expect(tabAfterKey(ITEMS, "general", "Home")).toBe("general");
    expect(tabAfterKey(ITEMS, "general", "End")).toBe("backups");
    expect(tabAfterKey(ITEMS, "company", "End")).toBe("backups");
    expect(tabAfterKey(ITEMS, "backups", "End")).toBe("backups");
  });

  it("returns null for keys that are not tab navigation", () => {
    expect(tabAfterKey(ITEMS, "company", "a")).toBeNull();
    expect(tabAfterKey(ITEMS, "company", " ")).toBeNull();
    expect(tabAfterKey(ITEMS, "company", "Enter")).toBeNull();
    expect(tabAfterKey(ITEMS, "company", "ArrowUp")).toBeNull();
    expect(tabAfterKey(ITEMS, "company", "ArrowDown")).toBeNull();
    expect(tabAfterKey([], "company", "ArrowRight")).toBeNull();
  });

  it("starts from the first tab when active matches no item", () => {
    expect(tabAfterKey(ITEMS, "missing", "ArrowRight")).toBe("company");
    expect(tabAfterKey(ITEMS, "missing", "ArrowLeft")).toBe("backups");
    expect(tabAfterKey(ITEMS, "missing", "Home")).toBe("general");
    expect(tabAfterKey(ITEMS, "missing", "End")).toBe("backups");
  });
});
