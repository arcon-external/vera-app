import { useEffect, useRef, useState } from "react";
import type { FocusEvent, KeyboardEvent, ReactNode } from "react";
import { NotificationBell } from "./NotificationBell";
import { VeraLogo } from "./VeraLogo";
import type { SignedInUser } from "./LoginPage";
import type { AppPage } from "../poLeaveGuard";

// Issue #491: the shell belongs to a workspace, not to one page. Home is the
// one global surface; inside a workspace the brand names that workspace, the
// logo becomes a back arrow, and only that workspace's tabs render. Clients and
// Team left the Quotation nav because they are admin features, not quotation
// pages.
//
// Issue #494: the profile menu is contextual. It offers Account settings and
// Sign out everywhere, and a workspace's own settings only while that
// workspace is the current context. A settings page keeps the context it was
// opened from, so the entry does not vanish when the page changes; the page
// title, the back target and the absence of tabs are deliberately unchanged.

export type WorkspaceId =
  | "quotation"
  | "purchase-order"
  | "admin"
  | "account"
  | "quotation-settings"
  | "purchase-order-settings";

/** The three domains that own a settings surface in the profile menu. */
export type WorkspaceDomain = "quotation" | "purchase-order" | "admin";

/**
 * The workspace a settings page was opened from, or null when it was opened
 * from Home. App owns the state; the shell only reads it.
 */
export type SettingsOrigin = WorkspaceDomain | null;

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
 * Which workspace a page belongs to. Home belongs to none; Account settings,
 * Quotation settings and Purchase Order settings are profile surfaces with no
 * tabs of their own.
 */
export function workspaceForPage(page: AppPage): WorkspaceId | null {
  if (page === "home") return null;
  if (page === "settings") return "account";
  if (page === "quotation-settings") return "quotation-settings";
  if (page === "po-settings") return "purchase-order-settings";
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
    case "quotation-settings":
      return "Quotation settings";
    case "purchase-order-settings":
      return "Purchase Order settings";
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

/**
 * Issue #494: which workspace the profile menu is speaking for. A domain page
 * names its own workspace; a settings page retains the one it was opened from;
 * Home, and a settings page opened from Home, have no context.
 */
export function profileMenuContext(
  page: AppPage,
  settingsOrigin: SettingsOrigin = null
): WorkspaceDomain | null {
  const workspace = workspaceForPage(page);
  switch (workspace) {
    case "quotation":
    case "purchase-order":
    case "admin":
      return workspace;
    case "account":
    case "quotation-settings":
    case "purchase-order-settings":
      return settingsOrigin;
    default:
      return null;
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
   * Issue #494: the workspace a settings page was opened from. It only widens
   * the profile menu; the page title, back arrow and tabs ignore it.
   */
  settingsOrigin?: SettingsOrigin;
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

/**
 * Issue #494: every profile-menu entry carries a small decorative icon. The
 * label is the accessible name, so the glyph is hidden from assistive
 * technology rather than given a name of its own.
 */
function MenuIcon({ children }: { children: ReactNode }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="shrink-0 text-ink-400"
    >
      {children}
    </svg>
  );
}

const MENU_ITEM_CLASS =
  "flex w-full items-center gap-2.5 px-4 py-2.5 text-left text-small text-ink-700 transition-colors hover:bg-paper-subtle focus-visible:outline-hidden focus-visible:bg-paper-subtle focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-500/40";

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
  settingsOrigin = null,
  inert = false,
}: AppShellProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  // The trigger and its popover are one dismissal group: focus or a pointer
  // leaving this node closes the menu, whichever gesture opened it.
  const menuGroupRef = useRef<HTMLDivElement>(null);
  // Set only when a keyboard gesture opens the menu, so a pointer click does
  // not steal focus from the trigger.
  const focusOnOpen = useRef<"first" | "last" | null>(null);
  const workspace = workspaceForPage(page);
  const tabs = workspaceTabs(workspace);
  const name = workspaceName(workspace);
  const isAdmin = user?.Role === "admin";
  const profileContext = profileMenuContext(page, settingsOrigin);
  // Issue #494: the entry is offered by context, never by the mere presence of
  // a callback. A context with no supplied callback still shows nothing, so the
  // menu can never offer an entry that does nothing.
  const offersQuotationSettings =
    profileContext === "quotation" && onOpenQuotationSettings !== undefined;
  const offersPurchaseOrderSettings =
    profileContext === "purchase-order" &&
    onOpenPurchaseOrderSettings !== undefined;
  const offersWorkspaceSettings =
    offersQuotationSettings || offersPurchaseOrderSettings;

  const menuItems = (): HTMLButtonElement[] =>
    Array.from(
      menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ??
        []
    );

  // A keyboard user may open the menu with ArrowDown/ArrowUp from the trigger;
  // the menu then takes focus at the end they asked for, as the menu-button
  // pattern expects. Effects run after the items exist.
  useEffect(() => {
    if (!menuOpen || focusOnOpen.current === null) return;
    const items = menuItems();
    const target = focusOnOpen.current === "last" ? items[items.length - 1] : items[0];
    focusOnOpen.current = null;
    target?.focus();
  }, [menuOpen]);

  // A pointer on anything outside the trigger+popover group dismisses it even
  // when the target is not focusable, so no focusout ever fires. Inside the
  // group the menu's own items and the trigger keep their click behaviour.
  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!menuGroupRef.current?.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [menuOpen]);

  const closeMenu = () => {
    setMenuOpen(false);
    triggerRef.current?.focus();
  };

  // Escape belongs to the whole group, not just the list: a keyboard user can
  // Shift+Tab back to the trigger while the menu is open, and Enter/Space opens
  // with focus inside the list. Either way Escape closes and returns focus.
  const onMenuGroupKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!menuOpen || event.key !== "Escape") return;
    event.preventDefault();
    closeMenu();
  };

  // Focus leaving the group closes the popover. Moving between the trigger and
  // its items stays inside and does not close it.
  const onMenuGroupBlur = (event: FocusEvent<HTMLDivElement>) => {
    if (!menuGroupRef.current?.contains(event.relatedTarget as Node | null)) {
      setMenuOpen(false);
    }
  };

  // Issue #494: the menu keeps its menu/menuitem roles, so it honours the keys
  // that pattern promises. Arrow keys move with wrapping, Home/End jump to the
  // ends; Escape is handled for the whole group above. Tab is left alone.
  const onMenuKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const items = menuItems();
    if (items.length === 0) return;
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    let next: number;
    switch (event.key) {
      case "ArrowDown":
        next = current === -1 ? 0 : (current + 1) % items.length;
        break;
      case "ArrowUp":
        next = current === -1 ? items.length - 1 : (current + items.length - 1) % items.length;
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = items.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    items[next]?.focus();
  };

  return (
    // Issue #494: data-settings-origin exposes the remembered context the menu
    // reads, the same way data-workspace exposes the page's workspace, so the
    // Home-reset and no-capture-on-Keep behaviours are observable in tests.
    <div
      className="flex min-h-full flex-col bg-paper"
      data-app-shell=""
      data-workspace={workspace ?? "home"}
      data-settings-origin={settingsOrigin ?? undefined}
      aria-hidden={inert ? true : undefined}
      inert={inert ? true : undefined}
    >
      <header className="sticky top-0 z-30 border-b border-rule bg-paper/90 backdrop-blur">
        <div className="mx-auto flex min-h-16 max-w-[1560px] flex-wrap items-center gap-x-6 gap-y-2 px-5 py-2 lg:px-8">
          {workspace === null ? (
            <div className="flex items-center gap-2">
              <VeraLogo className="h-9 w-auto" />
              {/* Issue #494: one vertically centred group — Vera | Business
                  Suite — with the mark beside it. The divider is a literal
                  glyph, hidden from assistive technology because the two names
                  already read in order. */}
              <span className="flex items-center gap-2 leading-none">
                <span className="text-[17px] font-semibold leading-none tracking-[-0.03em] text-ink-950">
                  Vera
                </span>
                <span
                  aria-hidden="true"
                  className="text-caption leading-none text-ink-300"
                >
                  |
                </span>
                <span className="text-caption leading-none text-ink-500">
                  Business Suite
                </span>
              </span>
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
              <div
                ref={menuGroupRef}
                className="relative ml-auto flex items-center gap-2 lg:ml-0"
                onKeyDown={onMenuGroupKeyDown}
                onBlur={onMenuGroupBlur}
              >
                <button
                  ref={triggerRef}
                  type="button"
                  onClick={() => setMenuOpen((open) => !open)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      // Enter/Space take the same opener path as ArrowDown:
                      // open on the first entry. The keydown default would
                      // otherwise synthesise a second, toggling click.
                      event.preventDefault();
                      if (menuOpen) {
                        setMenuOpen(false);
                        return;
                      }
                      focusOnOpen.current = "first";
                      setMenuOpen(true);
                      return;
                    }
                    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
                    event.preventDefault();
                    focusOnOpen.current = event.key === "ArrowUp" ? "last" : "first";
                    setMenuOpen(true);
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
                    ref={menuRef}
                    role="menu"
                    aria-label="Account"
                    onKeyDown={onMenuKeyDown}
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
                      className={MENU_ITEM_CLASS}
                    >
                      <MenuIcon>
                        <circle cx="12" cy="8" r="3.5" />
                        <path d="M5 20c0-3.3 3.1-5.5 7-5.5s7 2.2 7 5.5" />
                      </MenuIcon>
                      Account settings
                    </button>
                    {offersWorkspaceSettings ? (
                      <div role="separator" className="my-1 h-px bg-rule" />
                    ) : null}
                    {offersQuotationSettings ? (
                      <button
                        type="button"
                        role="menuitem"
                        onClick={() => {
                          setMenuOpen(false);
                          onOpenQuotationSettings?.();
                        }}
                        className={MENU_ITEM_CLASS}
                      >
                        <MenuIcon>
                          <path d="M6 3h9l4 4v14H6z" />
                          <path d="M15 3v4h4M9 12h6M9 16h6" />
                        </MenuIcon>
                        Quotation settings
                      </button>
                    ) : null}
                    {offersPurchaseOrderSettings ? (
                      <button
                        type="button"
                        role="menuitem"
                        onClick={() => {
                          setMenuOpen(false);
                          onOpenPurchaseOrderSettings?.();
                        }}
                        className={MENU_ITEM_CLASS}
                      >
                        <MenuIcon>
                          <path d="M3 6h2l2 10h11l2-7H6" />
                          <circle cx="9" cy="19" r="1.5" />
                          <circle cx="17" cy="19" r="1.5" />
                          <path d="M9 10h9" />
                        </MenuIcon>
                        Purchase Order settings
                      </button>
                    ) : null}
                    <div role="separator" className="my-1 h-px bg-rule" />
                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        setMenuOpen(false);
                        onSignOut?.();
                      }}
                      className={MENU_ITEM_CLASS}
                    >
                      <MenuIcon>
                        <path d="M14 4h4a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-4" />
                        <path d="M10 8l-4 4 4 4M6 12h9" />
                      </MenuIcon>
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
