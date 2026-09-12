import type { ReactNode } from "react";

interface AppShellProps {
  page: "create" | "archive" | "detail";
  onOpenEditor: () => void;
  onOpenArchive: () => void;
  children: ReactNode;
  actionBar?: ReactNode;
}

const tabs = [
  { id: "create", label: "Editor" },
  { id: "archive", label: "Archive" },
] as const;

export function AppShell({
  page,
  onOpenEditor,
  onOpenArchive,
  children,
  actionBar,
}: AppShellProps) {
  return (
    <div className="flex min-h-full flex-col bg-paper">
      <header className="sticky top-0 z-30 border-b border-rule bg-paper/90 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-[1560px] items-center gap-6 px-5 lg:px-8">
          <div className="flex items-baseline gap-2.5">
            <span className="text-[17px] font-semibold tracking-[-0.03em] text-ink-950">
              Vera
            </span>
            <span className="hidden text-caption text-ink-500 sm:inline">
              Quotation Generator
            </span>
          </div>

          <nav aria-label="Primary" className="flex items-center gap-1">
            {tabs.map((tab) => {
              const active =
                tab.id === "create" ? page === "create" : page !== "create";
              return (
                <button
                  key={tab.id}
                  type="button"
                  aria-current={active ? "page" : undefined}
                  onClick={tab.id === "create" ? onOpenEditor : onOpenArchive}
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
