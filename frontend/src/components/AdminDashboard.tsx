import type { AppPage } from "../poLeaveGuard";
import { ClientsPage } from "./ClientsPage";
import { BackupCard, CompanyProfileCard, RestoreCard } from "./SettingsPage";
import { UsersPage } from "./UsersPage";

// Issue #491: the admin surfaces used to sit in the same Settings page as the
// signed-in person's own account. They are company-wide tools, so they moved
// to their own workspace with their own tabs. Theme stays inside Company, and
// Account settings (General/Keyboard) stays in the profile menu.
//
// Issue #494: that workspace's main navigation exists only in the shell's
// primary bar. The dashboard used to render a second Clients/Team/Company/
// Backups tablist whose panel then pointed back at it with aria-labelledby;
// both are gone. What remains is one named section for the page the shell
// navigated to, with the Company profile's own General/Units/Quotation/Portal
// sub-tabs still living in the body where the duplicate bar used to sit.

export type AdminTabId = "clients" | "team" | "company" | "backups";

export const ADMIN_TABS: { id: AdminTabId; label: string }[] = [
  { id: "clients", label: "Clients" },
  { id: "team", label: "Team" },
  { id: "company", label: "Company" },
  { id: "backups", label: "Backups" },
];

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

const DESCRIPTIONS: Record<AdminTabId, string> = {
  clients: "The client directory every quotation and purchase order draws from.",
  team: "People who can sign in, their roles, and pending account requests.",
  company:
    "How the company appears, its currency and units, the quotation wording and the portal theme.",
  backups: "Download or restore the nightly database backup.",
};

export function AdminDashboard({
  tab,
  onCompanyChanged,
  onCompanyDirtyChange,
}: {
  tab: AdminTabId;
  /** A company save landed; App re-reads the profile and served unit list. */
  onCompanyChanged?: () => void;
  /** The Company card's unsaved-work flag, for the leave guard. */
  onCompanyDirtyChange?: (dirty: boolean) => void;
}) {
  const label = ADMIN_TABS.find((item) => item.id === tab)?.label ?? "Admin";
  return (
    <section aria-label={`${label} section`} className="space-y-6">
      {/* The shell brand already announces "Admin Dashboard", so the body keeps
          only the contextual sentence. That is what lets the Company sub-tabs
          sit at the position the duplicate main tabs occupied (#494). */}
      <p className="max-w-3xl text-body text-ink-500">{DESCRIPTIONS[tab]}</p>

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
    </section>
  );
}
