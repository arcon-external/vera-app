import type { AppPage } from "../poLeaveGuard";
import { ClientsPage } from "./ClientsPage";
import { BackupCard, CompanyProfileCard, RestoreCard } from "./SettingsPage";
import { TabBar, tabId, tabPanelId } from "./TabBar";
import { UsersPage } from "./UsersPage";

// Issue #491: the admin surfaces used to sit in the same Settings page as the
// signed-in person's own account. They are company-wide tools, so they moved
// to their own workspace with their own tabs. Theme stays inside Company, and
// Account settings (General/Keyboard) stays in the profile menu.

export type AdminTabId = "clients" | "team" | "company" | "backups";

export const ADMIN_TABS: { id: AdminTabId; label: string }[] = [
  { id: "clients", label: "Clients" },
  { id: "team", label: "Team" },
  { id: "company", label: "Company" },
  { id: "backups", label: "Backups" },
];

export const ADMIN_TAB_PAGES: Record<AdminTabId, AppPage> = {
  clients: "admin-clients",
  team: "admin-team",
  company: "admin-company",
  backups: "admin-backups",
};

export function adminTabForPage(page: AppPage): AdminTabId {
  switch (page) {
    case "admin-team":
      return "team";
    case "admin-company":
      return "company";
    case "admin-backups":
      return "backups";
    default:
      return "clients";
  }
}

export function adminPageForTab(tab: AdminTabId): AppPage {
  return ADMIN_TAB_PAGES[tab];
}

const DESCRIPTIONS: Record<AdminTabId, string> = {
  clients: "The client directory every quotation and purchase order draws from.",
  team: "People who can sign in, their roles, and pending account requests.",
  company:
    "How the company appears, its currency and units, the quotation wording and the portal theme.",
  backups: "Download or restore the nightly database backup.",
};

export function AdminDashboard({
  tab,
  onTabChange,
  onCompanyChanged,
  onCompanyDirtyChange,
}: {
  tab: AdminTabId;
  /** Every tab switch goes through App's leave guard. */
  onTabChange: (tab: AdminTabId) => void;
  /** A company save landed; App re-reads the profile and served unit list. */
  onCompanyChanged?: () => void;
  /** The Company card's unsaved-work flag, for the leave guard. */
  onCompanyDirtyChange?: (dirty: boolean) => void;
}) {
  return (
    <section aria-labelledby="admin-heading" className="space-y-6">
      <div>
        <h1 id="admin-heading" className="text-display font-semibold text-ink-950">
          Admin Dashboard
        </h1>
        <p className="mt-1.5 max-w-3xl text-body text-ink-500">
          {DESCRIPTIONS[tab]}
        </p>
      </div>

      <TabBar
        items={ADMIN_TABS}
        active={tab}
        onSelect={(id) => onTabChange(id as AdminTabId)}
        label="Admin sections"
        idPrefix="admin"
      />

      <div
        role="tabpanel"
        id={tabPanelId("admin", tab)}
        aria-labelledby={tabId("admin", tab)}
      >
        {tab === "clients" ? <ClientsPage /> : null}
        {tab === "team" ? <UsersPage /> : null}
        {tab === "company" ? (
          <CompanyProfileCard
            onSaved={onCompanyChanged}
            onDirtyChange={onCompanyDirtyChange}
          />
        ) : null}
        {tab === "backups" ? (
          <div className="grid gap-6">
            <BackupCard />
            <RestoreCard />
          </div>
        ) : null}
      </div>
    </section>
  );
}
