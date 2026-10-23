import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AdminDashboard, ADMIN_TABS, type AdminTabId } from "./AdminDashboard";
import { ToastProvider } from "../toast";

// Issue #494: the Admin workspace has one navigation surface -- the shell's
// primary bar. The dashboard no longer renders a second tablist, and its body
// is a named section rather than a tabpanel pointing at tabs that no longer
// exist. The Company sub-tabs stay in the body where the duplicate main tabs
// used to be, still owned by CompanyProfileCard.
//
// The suite runs in node with no DOM, so the dashboard is rendered to a string.
// Effects never run under renderToString, so the data panels stay in their
// loading state; AdminDashboard.dom.test.tsx owns the panel swap.

function render(tab: AdminTabId): string {
  return renderToString(
    <ToastProvider>
      <AdminDashboard tab={tab} />
    </ToastProvider>
  );
}

describe("AdminDashboard body navigation (issue #494)", () => {
  it("renders no second main tablist", () => {
    for (const tab of ADMIN_TABS) {
      const html = render(tab.id);
      expect(html).not.toContain('aria-label="Admin sections"');
      expect(html).not.toContain("admin-tab-");
      expect(html).not.toContain("admin-panel-");
      // The shell brand already says "Admin Dashboard"; the body does not
      // repeat it as a title.
      expect(html).not.toContain(">Admin Dashboard<");
    }
  });

  it("names the active section instead of a tabpanel", () => {
    expect(render("clients")).toContain('aria-label="Clients section"');
    expect(render("team")).toContain('aria-label="Team section"');
    expect(render("company")).toContain('aria-label="Company section"');
    expect(render("backups")).toContain('aria-label="Backups section"');
  });

  it("keeps the active section's contextual description", () => {
    expect(render("clients")).toContain(
      "The client directory every quotation and purchase order draws from."
    );
    expect(render("company")).toContain(
      "How the company appears, its currency and units, the quotation wording and the portal theme."
    );
    expect(render("backups")).toContain(
      "Download or restore the nightly database backup."
    );
  });

  it("keeps the Company sub-tabs in the body at the former navigation position", () => {
    const html = render("company");
    // Company's own four sections, not the Admin main tabs.
    expect(html).toContain('aria-label="Company sections"');
    for (const label of ["General", "Units", "Quotation", "Portal"]) {
      expect(html).toContain(`>${label}</button>`);
    }
    // They follow the description and precede the panel content.
    const description = html.indexOf("How the company appears");
    const companyBar = html.indexOf('aria-label="Company sections"');
    expect(description).toBeGreaterThan(-1);
    expect(companyBar).toBeGreaterThan(description);
    // The panel is still wired to the Company tablist that owns it.
    expect(html).toContain('id="company-panel-general"');
    expect(html).toContain('aria-labelledby="company-tab-general"');
  });

  it("does not leave the Admin tab ids orphaned on any section", () => {
    for (const tab of ADMIN_TABS) {
      const html = render(tab.id);
      expect(html).not.toMatch(/aria-labelledby="admin-tab-/);
      expect(html).not.toMatch(/id="admin-panel-/);
    }
  });
});
