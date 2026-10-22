import { useState } from "react";
import type { ReactNode } from "react";
import { NotificationBell } from "./NotificationBell";
import { VeraLogo } from "./VeraLogo";
import type { SignedInUser } from "./LoginPage";
import type { AppPage } from "../poLeaveGuard";

// Issue #491: the shell belongs to a workspace, not to one page. Home is the
// one global surface; inside a workspace the brand names that workspace, the
// logo becomes a back arrow, and only that workspace's tabs render. Clients and
// Team left the Quotation nav because they are admin features, not quotation
// pages.

export type WorkspaceId = "quotation" | "purchase-order" | "admin" | "account";

interface WorkspaceTab {
  label: string;
  page: AppPage;
}

const QUOTATION_TABS: WorkspaceTab[] = [
  { label: "Editor", page: "create" },
  { label: "Drafts", page: "drafts" },
  { label: "Archive", page: "archive" },
];

const PURCHASE_ORDER_TABS: WorkspaceTab[] = [
  { label: "Editor", page: "po" },
  { label: "Drafts", page: "po-drafts" },
  { label: "Archive", page: "po-archive" },
];

const ADMIN_TABS: WorkspaceTab[] = [
  { label: "Clients", page: "admin-clients" },
  { label: "Team", page: "admin-team" },
  { label: "Company", page: "admin-company" },
  { label: "Backups", page: "admin-backups" },
];

/**
 * Which workspace a page belongs to. Home belongs to none; Account settings is
 * a profile surface with no tabs of its own.
 */
export function workspaceForPage(page: AppPage): WorkspaceId | null {
  if (page === "home") return null;
  if (page === "settings") return "account";
  if (page.startsWith("admin-")) return "admin";
  if (page === "po" || page === "po-drafts" || page === "po-archive") {
    return "purchase-order";
  }
  return "quotation";
}

export function workspaceName(workspace: WorkspaceId | null): string {
  switch (workspace) {
    case "quotation":
      return "Quotation";
    case "purchase-order":
      return "Purchase Orders";
    case "admin":
      return "Admin Dashboard";
    case "account":
      return "Account settings";
    default:
      return "Vera";
  }
}

export function workspaceTabs(workspace: WorkspaceId | null): WorkspaceTab[] {
  switch (workspace) {
    case "quotation":
      return QUOTATION_TABS;
    case "purchase-order":
      return PURCHASE_ORDER_TABS;
    case "admin":
      return ADMIN_TABS;
    default:
      return [];
  }
}

function tabIsActive(page: AppPage, tabPage: AppPage): boolean {
  // A quotation detail is still Archive work, not a fourth tab.
  if (tabPage === "archive") return page === "archive" || page === "detail";
  return page === tabPage;
}

interface AppShellProps {
  page: AppPage;
  /** Every navigation goes through the caller's leave guard. */
  onNavigate: (page: AppPage) => void;
  draftCount?: number;
  companyName?: string;
  children: ReactNode;
  actionBar?: ReactNode;
  user?: SignedInUser | null;
  onSignOut?: () => void;
  /**
   * Issue #492 owns these two settings surfaces. They render only when a
   * caller supplies the callback, so this issue does not grow pages it does
   * not implement.
   */
  onOpenQuotationSettings?: () => void;
  onOpenPurchaseOrderSettings?: () => void;
  /**
   * Marks the whole shell unreachable while a modal layer owns the page: the
   * discard prompt sets it so focus and assistance technology stay inside the
   * one dialog. The caller owns the dialog, so the shell only mirrors it.
   */
  inert?: boolean;
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

export function AppShell({
  page,
  onNavigate,
  draftCount = 0,
  companyName = "PT Arcon Penta Persada",
  children,
  actionBar,
  user,
  onSignOut,
  onOpenQuotationSettings,
  onOpenPurchaseOrderSettings,
  inert = false,
}: AppShellProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const workspace = workspaceForPage(page);
  const tabs = workspaceTabs(workspace);
  const name = workspaceName(workspace);
  const isAdmin = user?.Role === "admin";

  return (
    <div
      className="flex min-h-full flex-col bg-paper"
      data-app-shell=""
      data-workspace={workspace ?? "home"}
      aria-hidden={inert ? true : undefined}
      inert={inert ? true : undefined}
    >
      <header className="sticky top-0 z-30 border-b border-rule bg-paper/90 backdrop-blur">
        <div className="mx-auto flex min-h-16 max-w-[1560px] flex-wrap items-center gap-x-6 gap-y-2 px-5 py-2 lg:px-8">
          {workspace === null ? (
            <div className="flex items-center gap-1.5">
              <VeraLogo className="h-9 w-auto" />
              <span className="text-[17px] font-semibold tracking-[-0.03em] text-ink-950">
                Vera
              </span>
              <span className="text-caption text-ink-500">Business Suite</span>
            </div>
          ) : (
            <div className="flex items-center gap-2">
              <button
                type="button"
                aria-label="Back to Home"
                onClick={() => onNavigate("home")}
                className="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-rule bg-white text-ink-600 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
              >
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <path d="M19 12H5M11 18l-6-6 6-6" />
                </svg>
              </button>
              <span className="whitespace-nowrap text-[17px] font-semibold tracking-[-0.03em] text-ink-950">
                {name}
              </span>
            </div>
          )}

          {tabs.length > 0 ? (
            <nav
              aria-label="Primary"
              data-workspace={workspace}
              className="flex items-center gap-1"
            >
              {tabs.map((tab) => {
                const active = tabIsActive(page, tab.page);
                // Only worth a badge when there is something to come back to,
                // and only on an inactive tab: a count on the tab you are
                // already looking at is noise.
                const badge =
                  workspace === "quotation" &&
                  tab.page === "drafts" &&
                  !active
                    ? draftCount
                    : 0;
                return (
                  <button
                    key={tab.page}
                    type="button"
                    aria-current={active ? "page" : undefined}
                    onClick={() => onNavigate(tab.page)}
                    className={`inline-flex min-h-9 items-center gap-1.5 rounded-lg px-4 py-2.5 text-small font-medium transition-colors duration-150 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
                      active
                        ? "bg-ink-900 text-white"
                        : "text-ink-500 hover:bg-paper-muted hover:text-ink-900"
                    }`}
                  >
                    {tab.label}
                    {badge > 0 ? (
                      <span
                        className="tnum flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-accent-100 px-1 text-[11px] font-semibold leading-none text-accent-800"
                        aria-label={`${badge} saved draft${badge === 1 ? "" : "s"}`}
                      >
                        {badge > 99 ? "99+" : badge}
                      </span>
                    ) : null}
                  </button>
                );
              })}
            </nav>
          ) : null}

          {/* The company name must never wrap: at the desktop's own 1024px
              there is room for it on one line, and a wrapped two-line block
              is what pushed the header past its 64px. */}
          <span className="ml-auto hidden max-w-[220px] truncate whitespace-nowrap text-micro font-medium uppercase tracking-[0.16em] text-ink-500 lg:block">
            {companyName}
          </span>
          {user ? (
            <>
              <NotificationBell
                user={user}
                onOpenTeam={isAdmin ? () => onNavigate("admin-team") : undefined}
              />
              <div className="relative ml-auto flex items-center gap-2 lg:ml-0">
                <button
                  type="button"
                  onClick={() => setMenuOpen((open) => !open)}
                  onBlur={(event) => {
                    if (
                      !event.currentTarget.parentElement?.contains(
                        event.relatedTarget as Node
                      )
                    ) {
                      setMenuOpen(false);
                    }
                  }}
                  aria-haspopup="menu"
                  aria-expanded={menuOpen}
                  aria-label={`Account menu for ${user.DisplayName}`}
                  className="inline-flex min-h-9 items-center gap-2 rounded-lg px-2.5 py-2 transition-colors duration-150 hover:bg-paper-muted focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                >
                  <span
                    className="flex h-7 w-7 items-center justify-center overflow-hidden rounded-full bg-accent-100 text-caption font-semibold text-accent-800"
                    aria-hidden="true"
                  >
                    {user.AvatarURL ? (
                      <img
                        src={user.AvatarURL}
                        alt=""
                        className="h-full w-full object-cover"
                      />
                    ) : (
                      initials(user.DisplayName)
                    )}
                  </span>
                  <span className="hidden text-left sm:block">
                    <span className="block max-w-[160px] truncate text-small font-medium leading-tight text-ink-900">
                      {user.DisplayName}
                    </span>
                    <span className="block text-caption leading-tight text-ink-400">
                      {isAdmin ? "Admin" : "Staff"}
                    </span>
                  </span>
                  <svg
                    width="12"
                    height="12"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                    className={`text-ink-400 transition-transform duration-150 ${
                      menuOpen ? "rotate-180" : ""
                    }`}
                  >
                    <path d="m6 9 6 6 6-6" />
                  </svg>
                </button>
                {menuOpen ? (
                  <div
                    role="menu"
                    aria-label="Account"
                    className="absolute right-0 top-full z-40 mt-1.5 w-60 overflow-hidden rounded-lg border border-rule bg-white py-1 shadow-pop"
                  >
                    <div className="border-b border-rule px-4 py-2.5">
                      <p className="truncate text-small font-medium text-ink-900">
                        {user.DisplayName}
                      </p>
                      <p className="truncate text-caption text-ink-400">{user.Email}</p>
                    </div>
                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        setMenuOpen(false);
                        onNavigate("settings");
                      }}
                      className="flex w-full items-center px-4 py-2.5 text-left text-small text-ink-700 transition-colors hover:bg-paper-subtle"
                    >
                      Account settings
                    </button>
                    {onOpenQuotationSettings ? (
                      <button
                        type="button"
                        role="menuitem"
                        onClick={() => {
                          setMenuOpen(false);
                          onOpenQuotationSettings();
                        }}
                        className="flex w-full items-center px-4 py-2.5 text-left text-small text-ink-700 transition-colors hover:bg-paper-subtle"
                      >
                        Quotation settings
                      </button>
                    ) : null}
                    {onOpenPurchaseOrderSettings ? (
                      <button
                        type="button"
                        role="menuitem"
                        onClick={() => {
                          setMenuOpen(false);
                          onOpenPurchaseOrderSettings();
                        }}
                        className="flex w-full items-center px-4 py-2.5 text-left text-small text-ink-700 transition-colors hover:bg-paper-subtle"
                      >
                        Purchase Order settings
                      </button>
                    ) : null}
                    {isAdmin ? (
                      <button
                        type="button"
                        role="menuitem"
                        onClick={() => {
                          setMenuOpen(false);
                          onNavigate("admin-clients");
                        }}
                        className="flex w-full items-center px-4 py-2.5 text-left text-small text-ink-700 transition-colors hover:bg-paper-subtle"
                      >
                        Admin Dashboard
                      </button>
                    ) : null}
                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        setMenuOpen(false);
                        onSignOut?.();
                      }}
                      className="flex w-full items-center px-4 py-2.5 text-left text-small text-ink-700 transition-colors hover:bg-paper-subtle"
                    >
                      Sign out
                    </button>
                  </div>
                ) : null}
              </div>
            </>
          ) : null}
        </div>
      </header>

      <main className="flex-1">
        <div
          className={`mx-auto w-full max-w-[1560px] px-5 pt-7 lg:px-8 lg:pt-9 ${
            actionBar ? "pb-16" : "pb-10"
          }`}
        >
          {children}
        </div>
      </main>

      {actionBar ? (
        <div className="sticky bottom-0 z-30 border-t border-rule bg-paper/95 shadow-rail backdrop-blur">
          <div className="mx-auto flex max-w-[1560px] flex-wrap items-center gap-3 px-5 py-3 lg:px-8">
            {actionBar}
          </div>
        </div>
      ) : null}
    </div>
  );
}
