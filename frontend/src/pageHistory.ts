import type { AppPage } from "./poLeaveGuard";

// Ticket 02: the page-history stack for Home → workspace → settings.
//
// Pure navigation rules with no React: App owns the `AppPage[]` stack, and
// these functions are the decisions it applies. Tab switches inside one
// workspace replace the current page (Back leaves the workspace, never the
// tab); moves across workspaces push the page being left (Back returns to
// the exact workspace page, never straight Home).

export type WorkspaceKey =
  | "home"
  | "quotation"
  | "purchase-order"
  | "admin"
  | "account-settings"
  | "quotation-settings"
  | "purchase-order-settings";

export function isSettingsPage(page: AppPage): boolean {
  return page === "settings" || page === "quotation-settings" || page === "po-settings";
}

export function isPurchaseOrderWorkspacePage(page: AppPage): boolean {
  return page === "po" || page === "po-drafts" || page === "po-archive";
}

export function isQuotationWorkspacePage(page: AppPage): boolean {
  return page === "create" || page === "drafts" || page === "archive" || page === "detail";
}

export function workspaceKeyOf(page: AppPage): WorkspaceKey {
  if (page === "home") return "home";
  if (isPurchaseOrderWorkspacePage(page)) return "purchase-order";
  if (isQuotationWorkspacePage(page)) return "quotation";
  if (page.startsWith("admin-")) return "admin";
  if (page === "settings") return "account-settings";
  if (page === "quotation-settings") return "quotation-settings";
  return "purchase-order-settings";
}

/**
 * Whether leaving `from` for `to` records `from` on the stack. Moves inside
 * one workspace (tab switches, archive to detail) replace instead of pushing,
 * so Back from anywhere in a workspace returns Home rather than the previous
 * tab. Staying on the same page never pushes.
 */
export function shouldPushHistory(from: AppPage, to: AppPage): boolean {
  if (from === to) return false;
  return workspaceKeyOf(from) !== workspaceKeyOf(to);
}

/** Pushes the page being left, or keeps the stack when staying in place. */
export function pushHistory(history: AppPage[], from: AppPage, to: AppPage): AppPage[] {
  if (!shouldPushHistory(from, to)) return history;
  return [...history, from];
}

/** Pops the stack; an empty stack lands on Home. */
export function popHistory(history: AppPage[]): { history: AppPage[]; page: AppPage } {
  if (history.length === 0) return { history, page: "home" };
  return { history: history.slice(0, -1), page: history[history.length - 1] };
}

/** Where the shell Back arrow goes from `current`. */
export function backTarget(current: AppPage, history: AppPage[]): AppPage {
  void current;
  if (history.length === 0) return "home";
  return history[history.length - 1];
}

/**
 * The workspace page to keep alive behind a settings page: the nearest
 * workspace entry in the stack. Null when the current page is not settings
 * or nothing behind it is a workspace, so nothing is kept mounted.
 */
export function lastWorkspacePage(
  history: AppPage[],
  current: AppPage
): AppPage | null {
  if (!isSettingsPage(current)) return null;
  for (let index = history.length - 1; index >= 0; index -= 1) {
    const candidate = history[index];
    if (isPurchaseOrderWorkspacePage(candidate) || isQuotationWorkspacePage(candidate)) {
      return candidate;
    }
  }
  return null;
}
