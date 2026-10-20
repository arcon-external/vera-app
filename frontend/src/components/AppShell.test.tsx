import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AppShell } from "./AppShell";

// Issue #466: Home is the first primary tab and the way back from every page.
// The suite runs in node with no DOM (see vitest.config.ts), so AppShell is
// rendered to a string and asserted on as markup -- the same house pattern as
// TabBar.test.tsx. `user` stays null: NotificationBell reads localStorage
// during render, which node has none of.
//
// What this test does NOT prove: that clicking a tab reaches its page, that
// focus is managed, or that the tab order reads well. Those stay hand-driven.

function renderShell(
  page: Parameters<typeof AppShell>[0]["page"]
): string {
  return renderToString(
    <AppShell
      page={page}
      onOpenHome={() => {}}
      onOpenEditor={() => {}}
      onOpenDrafts={() => {}}
      onOpenArchive={() => {}}
    >
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
 *  chunk whose inner markup starts with the label is the tab itself -- not a
 *  later tab named in an earlier chunk's trailing markup. */
function tabChunk(html: string, label: string): string {
  const found = buttons(html).find((chunk) =>
    chunk.slice(chunk.indexOf(">") + 1).startsWith(label)
  );
  if (!found) throw new Error(`no tab labelled ${label} in: ${html}`);
  return found;
}

/** Whether the tab's opening tag carries aria-current="page". */
function isCurrent(chunk: string): boolean {
  return chunk
    .slice(0, chunk.indexOf(">"))
    .includes('aria-current="page"');
}

describe("AppShell Home tab (issue #466)", () => {
  it("offers Home as the first primary tab", () => {
    const html = renderShell("home");
    expect(html).toContain('aria-label="Primary"');
    const homeAt = html.indexOf(">Home<");
    for (const label of ["Editor", "Drafts", "Archive"]) {
      expect(html.indexOf(`>${label}<`)).toBeGreaterThan(homeAt);
    }
  });

  it("marks Home current on the home page", () => {
    const html = renderShell("home");
    expect(isCurrent(tabChunk(html, "Home"))).toBe(true);
    expect(isCurrent(tabChunk(html, "Editor"))).toBe(false);
  });

  it("keeps a way back to Home from the Quotation editor", () => {
    const html = renderShell("create");
    expect(isCurrent(tabChunk(html, "Editor"))).toBe(true);
    // Present but not current: the house-consistent affordance back home.
    expect(tabChunk(html, "Home")).toBeDefined();
    expect(isCurrent(tabChunk(html, "Home"))).toBe(false);
  });

  it("keeps a way back to Home from the Purchase Order placeholder", () => {
    // The placeholder is its own destination, not home: no tab claims it.
    const html = renderShell("po");
    expect(tabChunk(html, "Home")).toBeDefined();
    expect(isCurrent(tabChunk(html, "Home"))).toBe(false);
    expect(html).not.toContain('aria-current="page"');
  });

  it("still marks Archive current on a quotation detail page", () => {
    const html = renderShell("detail");
    expect(isCurrent(tabChunk(html, "Archive"))).toBe(true);
    expect(isCurrent(tabChunk(html, "Home"))).toBe(false);
  });
});
