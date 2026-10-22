import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AdminDashboard, ADMIN_TABS, type AdminTabId } from "./AdminDashboard";
import { ToastProvider } from "../toast";

// Issue #491: Clients, Team, Company and Backups are the Admin Dashboard's
// four tabs. Theme stays inside Company; Account settings is not here.
//
// The suite runs in node with no DOM, so the dashboard is rendered to a string.
// Effects never run under renderToString, so the data panels stay in their
// loading state; this suite owns the tab set and the active-panel wiring, while
// AdminDashboard.dom.test.tsx owns switching.

function render(tab: AdminTabId): string {
  return renderToString(
    <ToastProvider>
      <AdminDashboard tab={tab} onTabChange={() => {}} />
    </ToastProvider>
  );
}

describe("AdminDashboard tabs (issue #491)", () => {
  it("renders Clients, Team, Company and Backups", () => {
    const html = render("clients");
    expect(html).toContain('aria-label="Admin sections"');
    for (const tab of ADMIN_TABS) {
      expect(html).toContain(`>${tab.label}</button>`);
    }
    // Theme lives inside Company, not as a fifth tab.
    expect(html).not.toContain(">Theme</button>");
  });

  it("marks exactly one tab selected, matching the page", () => {
    const html = render("company");
    // The Company panel renders its own sub-tab bar, so count only the Admin
    // tablist: from its accessible name to the panel that follows it.
    const adminBar = html
      .slice(html.indexOf('aria-label="Admin sections"'))
      .split("</div>")[0];
    expect(adminBar.split('aria-selected="true"').length - 1).toBe(1);
    const company = adminBar
      .split("<button")
      .find((chunk) => chunk.includes(">Company</button>"));
    expect(company).toContain('aria-selected="true"');
    expect(html).toContain('id="admin-panel-company"');
    expect(html).toContain('aria-labelledby="admin-tab-company"');
  });

  it("names the dashboard and the active section's purpose", () => {
    expect(render("clients")).toContain(">Clients<");
    expect(render("team")).toContain(">Team<");
    expect(render("backups")).toContain(">Backups<");
    expect(render("backups")).toContain("Download latest backup");
  });
});
