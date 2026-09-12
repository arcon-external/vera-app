import type { ReactNode } from "react";

interface AppShellProps {
  page: "create" | "archive" | "detail" | "users";
  onOpenEditor: () => void;
  onOpenArchive: () => void;
  onOpenUsers?: () => void;
  showUsersTab?: boolean;
  children: ReactNode;
  actionBar?: ReactNode;
  user?: { DisplayName: string; Role: string } | null;
  onSignOut?: () => void;
}

const tabs = [
  { id: "create", label: "Editor" },
  { id: "archive", label: "Archive" },
  { id: "users", label: "Team" },
] as const;

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
  showUsersTab = false,
  children,
  actionBar,
  user,
  onSignOut,
}: AppShellProps) {
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
            <div className="ml-auto flex items-center gap-2 lg:ml-0">
              <span className="hidden text-caption text-ink-500 sm:inline">
                {user.DisplayName}
                {user.Role === "admin" ? (
                  <span className="ml-1.5 rounded-md border border-accent-200 bg-accent-50 px-1.5 py-0.5 text-micro font-medium uppercase text-accent-700">
                    Admin
                  </span>
                ) : null}
              </span>
              <button
                type="button"
                onClick={onSignOut}
                className="inline-flex min-h-8 items-center rounded-md px-2 py-1.5 text-caption text-ink-500 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40"
              >
                Sign out
              </button>
            </div>
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
