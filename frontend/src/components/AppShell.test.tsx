import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AppShell } from "./AppShell";
import type { AppPage } from "../poLeaveGuard";

// Issue #491: the shell belongs to one workspace at a time. Home is global and
// has no tabs; inside a workspace the brand names it, the logo is a back arrow,
// and only that workspace's tabs render.
//
// The suite runs in node with no DOM (see vitest.config.ts), so AppShell is
// rendered to a string and asserted on as markup -- the same house pattern as
// TabBar.test.tsx. `user` stays null: NotificationBell reads localStorage
// during render, which node has none of. The profile menu's admin gating is
// driven in AppShell.dom.test.tsx instead.
//
// What this test does NOT prove: that clicking a tab reaches its page, that
// focus is managed, or that the tab order reads well. Those stay hand-driven.

function renderShell(page: AppPage): string {
  return renderToString(
    <AppShell page={page} onNavigate={() => {}}>
      <div>page body</div>
    </AppShell>
  );
}

/** Splits the rendered output into one string per <button> element. */
function buttons(html: string): string[] {
  return html.split("<button").slice(1);
}

/** The <button> chunk for the tab with the given label: the label is the
 *  first thing inside the button (a drafts badge may follow it), so the
 *  chunk whose inner markup starts with the label is the tab itself. */
function tabChunk(html: string, label: string): string {
  const found = buttons(html).find((chunk) =>
    chunk.slice(chunk.indexOf(">") + 1).startsWith(label)
  );
  if (!found) throw new Error(`no tab labelled ${label} in: ${html}`);
  return found;
}

/** The opening tag of the button carrying the given accessible label. */
function buttonTagFor(html: string, label: string): string {
  const found = buttons(html).find((chunk) =>
    chunk.slice(0, chunk.indexOf(">")).includes(`aria-label="${label}"`)
  );
  if (!found) throw new Error(`no <button aria-label="${label}"> in: ${html}`);
  return found.slice(0, found.indexOf(">") + 1);
}

/** Whether the button's opening tag carries aria-current="page". */
function isCurrent(chunk: string): boolean {
  return chunk
    .slice(0, chunk.indexOf(">"))
    .includes('aria-current="page"');
}

/** The nav element, so assertions cannot match a body button. */
function nav(html: string): string {
  const start = html.indexOf('<nav aria-label="Primary"');
  if (start === -1) throw new Error(`no primary nav in: ${html}`);
  const end = html.indexOf("</nav>", start);
  return html.slice(start, end);
}

describe("AppShell global Home (issue #491)", () => {
  it("shows Vera + Business Suite and no workspace tabs on Home", () => {
    const html = renderShell("home");
    expect(html).toContain(">Vera<");
    expect(html).toContain(">Business Suite<");
    expect(html).not.toContain('aria-label="Primary"');
    expect(html).not.toContain(">Editor</button>");
    expect(html).not.toContain('aria-label="Back to Home"');
  });
});

describe("AppShell Quotation workspace", () => {
  it("names the workspace, offers a back arrow and only its three tabs", () => {
    const html = renderShell("create");
    expect(html).toContain(">Quotation<");
    expect(buttonTagFor(html, "Back to Home")).toContain(
      'aria-label="Back to Home"'
    );
    const primary = nav(html);
    expect(primary).toContain('data-workspace="quotation"');
    for (const label of ["Editor", "Drafts", "Archive"]) {
      expect(primary).toContain(`>${label}<`);
    }
    // Clients and Team moved to the Admin Dashboard.
    expect(primary).not.toContain(">Clients<");
    expect(primary).not.toContain(">Team<");
    expect(isCurrent(tabChunk(html, "Editor"))).toBe(true);
    expect(isCurrent(tabChunk(html, "Drafts"))).toBe(false);
    expect(html).not.toContain(">Home</button>");
  });

  it("marks Archive current on a quotation detail page", () => {
    const html = renderShell("detail");
    expect(isCurrent(tabChunk(html, "Archive"))).toBe(true);
  });
});

describe("AppShell Purchase Order workspace", () => {
  it("shows Editor / Drafts / Archive and marks the active PO tab", () => {
    const html = renderShell("po-drafts");
    expect(html).toContain(">Purchase Orders<");
    const primary = nav(html);
    expect(primary).toContain('data-workspace="purchase-order"');
    for (const label of ["Editor", "Drafts", "Archive"]) {
      expect(primary).toContain(`>${label}<`);
    }
    expect(isCurrent(tabChunk(html, "Drafts"))).toBe(true);
    expect(isCurrent(tabChunk(html, "Editor"))).toBe(false);
    expect(isCurrent(tabChunk(html, "Archive"))).toBe(false);
  });
});

describe("AppShell Admin workspace", () => {
  it("shows Clients / Team / Company / Backups and marks the active tab", () => {
    const html = renderShell("admin-company");
    expect(html).toContain(">Admin Dashboard<");
    const primary = nav(html);
    expect(primary).toContain('data-workspace="admin"');
    for (const label of ["Clients", "Team", "Company", "Backups"]) {
      expect(primary).toContain(`>${label}<`);
    }
    expect(isCurrent(tabChunk(html, "Company"))).toBe(true);
    expect(isCurrent(tabChunk(html, "Clients"))).toBe(false);
  });
});

describe("AppShell account settings", () => {
  it("is its own surface with a back arrow and no tabs", () => {
    const html = renderShell("settings");
    expect(html).toContain(">Account settings<");
    expect(buttonTagFor(html, "Back to Home")).toBeDefined();
    expect(html).not.toContain('aria-label="Primary"');
  });
});

describe("AppShell workspace settings (issue #492)", () => {
  it("names Quotation settings as its own tab-less surface", () => {
    const html = renderShell("quotation-settings");
    // The shell brand is the page title, not "Account settings".
    expect(html).toContain(">Quotation settings<");
    expect(html).not.toContain(">Account settings<");
    expect(buttonTagFor(html, "Back to Home")).toBeDefined();
    expect(html).not.toContain('aria-label="Primary"');
  });

  it("names Purchase Order settings as its own tab-less surface", () => {
    const html = renderShell("po-settings");
    expect(html).toContain(">Purchase Order settings<");
    expect(html).not.toContain(">Purchase Orders<");
    expect(buttonTagFor(html, "Back to Home")).toBeDefined();
    expect(html).not.toContain('aria-label="Primary"');
  });
});
