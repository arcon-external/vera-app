import type { ReactNode } from "react";
import { useState } from "react";
import { NotificationBell } from "./NotificationBell";
import type { SignedInUser } from "./LoginPage";

interface AppShellProps {
  page: "create" | "archive" | "detail" | "users" | "settings";
  onOpenEditor: () => void;
  onOpenArchive: () => void;
  onOpenUsers?: () => void;
  onOpenSettings?: () => void;
  showUsersTab?: boolean;
  children: ReactNode;
  actionBar?: ReactNode;
  user?: SignedInUser | null;
  onSignOut?: () => void;
}

const tabs = [
  { id: "create", label: "Editor" },
  { id: "archive", label: "Archive" },
  { id: "users", label: "Team" },
] as const;

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

function isTabActive(tab: string, page: string): boolean {
  if (tab === "create") return page === "create";
  if (tab === "archive") return page === "archive" || page === "detail";
  return page === "users";
}

export function AppShell({
  page,
  onOpenEditor,
  onOpenArchive,
  onOpenUsers,
  onOpenSettings,
  showUsersTab = false,
  children,
  actionBar,
  user,
  onSignOut,
}: AppShellProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  return (
    <div className="flex min-h-full flex-col bg-paper">
      <header className="sticky top-0 z-30 border-b border-rule bg-paper/90 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-[1560px] items-center gap-6 px-5 lg:px-8">
          <div className="flex items-baseline gap-2.5">
            <span className="text-[17px] font-semibold tracking-[-0.03em] text-ink-950">
              Vera
            </span>
            <span className="hidden text-caption text-ink-500 sm:inline">
              Quotation Generator
            </span>
          </div>

          <nav aria-label="Primary" className="flex items-center gap-1">
            {tabs
              .filter((tab) => tab.id !== "users" || showUsersTab)
              .map((tab) => {
                const active = isTabActive(tab.id, page);
                const open =
                  tab.id === "create"
                    ? onOpenEditor
                    : tab.id === "archive"
                      ? onOpenArchive
                      : onOpenUsers;
                return (
                  <button
                    key={tab.id}
                    type="button"
                    aria-current={active ? "page" : undefined}
                    onClick={open}
                    className={`min-h-9 rounded-lg px-4 py-2.5 text-small font-medium transition-colors duration-150 ease-out-quart focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
                      active
                        ? "bg-ink-900 text-white"
                        : "text-ink-500 hover:bg-paper-muted hover:text-ink-900"
                    }`}
                  >
                    {tab.label}
                  </button>
                );
              })}
          </nav>

          <span className="ml-auto hidden text-micro font-medium uppercase tracking-[0.16em] text-ink-500 lg:block">
            PT Arcon Penta Persada
          </span>
          {user ? (
            <>
              <NotificationBell user={user} />
              <div className="relative ml-auto flex items-center gap-2 lg:ml-0">
              <button
                type="button"
                onClick={() => setMenuOpen((open) => !open)}
                onBlur={(event) => {
                  if (!event.currentTarget.parentElement?.contains(event.relatedTarget as Node)) {
                    setMenuOpen(false);
                  }
                }}
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                aria-label={`Account menu for ${user.DisplayName}`}
                className="inline-flex min-h-9 items-center gap-2 rounded-lg px-2.5 py-2 transition-colors duration-150 hover:bg-paper-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40"
              >
                <span className="flex h-7 w-7 items-center justify-center overflow-hidden rounded-full bg-accent-100 text-caption font-semibold text-accent-800" aria-hidden="true">
                  {user.AvatarURL ? (
                    <img src={user.AvatarURL} alt="" className="h-full w-full object-cover" />
                  ) : (
                    initials(user.DisplayName)
                  )}
                </span>
                <span className="hidden text-left sm:block">
                  <span className="block max-w-[160px] truncate text-small font-medium leading-tight text-ink-900">
                    {user.DisplayName}
                  </span>
                  <span className="block text-caption leading-tight text-ink-400">
                    {user.Role === "admin" ? "Admin" : "Staff"} · Settings
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
                  className={`text-ink-400 transition-transform duration-150 ${menuOpen ? "rotate-180" : ""}`}
                >
                  <path d="m6 9 6 6 6-6" />
                </svg>
              </button>
              {menuOpen ? (
                <div
                  role="menu"
                  aria-label="Account"
                  className="absolute right-0 top-full z-40 mt-1.5 w-52 overflow-hidden rounded-lg border border-rule bg-white py-1 shadow-pop"
                >
                  <div className="border-b border-rule px-4 py-2.5">
                    <p className="truncate text-small font-medium text-ink-900">{user.DisplayName}</p>
                    <p className="truncate text-caption text-ink-400">{user.Email}</p>
                  </div>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setMenuOpen(false);
                      onOpenSettings?.();
                    }}
                    className="flex w-full items-center px-4 py-2.5 text-left text-small text-ink-700 transition-colors hover:bg-paper-subtle"
                  >
                    Settings
                  </button>
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
