import { useCallback, useRef, useState } from "react";

// Issue #468, wave 2: leaving a workspace must not silently discard unsaved
// reducer state. The editor owns its data and reports a dirty flag; this hook
// owns the navigation decision so the shell's tabs, the back-to-Home control
// and the keyboard shortcuts share one gate instead of each losing the work
// their own way.
//
// Issue #491: the same gate now covers the Quotation editor and the Admin
// dashboard's Company tab. Those pages own their drafts inside App and the
// Company panel respectively, so their dirty flags arrive through
// `dirtyPages`; only the PO editor reports through `setDirty` because its
// state lives in the workspace controller this hook was written for.

export type AppPage =
  | "home"
  | "create"
  | "drafts"
  | "archive"
  | "detail"
  | "po"
  | "po-drafts"
  | "po-archive"
  | "admin-clients"
  | "admin-team"
  | "admin-company"
  | "admin-backups"
  | "settings"
  | "quotation-settings"
  | "po-settings";

export interface LeaveGuardOptions {
  /**
   * Dirty pages whose draft state is owned outside this hook. A page that is
   * true here asks before navigation exactly as a dirty PO editor does.
   */
  dirtyPages?: Partial<Record<AppPage, boolean>>;
  /**
   * The sentence the discard dialog shows for a dirty page. The default names
   * the PO draft because that is the page this guard was built for.
   */
  describe?: (page: AppPage) => string;
  /**
   * Runs after the user confirms Discard and before navigation. The page that
   * owned the changes clears them here, so returning to it later starts from
   * what was actually saved rather than resurrecting the discarded draft.
   */
  onDiscard?: (page: AppPage) => void;
}

export interface AppLeaveGuard {
  /** True while the mounted PO editor holds changes the saved draft lacks. */
  dirty: boolean;
  /** Called by the PO editor with its own isPurchaseOrderDirty answer. */
  setDirty: (dirty: boolean) => void;
  /** The destination waiting behind the discard confirmation, if any. */
  pendingPage: AppPage | null;
  /** The sentence naming what the decision discards, or "". */
  pendingDescription: string;
  /**
   * The control that was focused when the pending decision opened. Captured
   * before the shell becomes inert, because a browser may blur a focused
   * element inside a newly inert subtree and the dialog could then only
   * restore focus to <body>.
   */
  pendingReturnFocus: HTMLElement | null;
  /**
   * Navigate, or ask first when leaving a dirty page. `after` replaces the
   * plain navigation for a destination that needs its own work first
   * (mod+N starts a fresh quotation); it runs only once the guard allows it.
   */
  request: (next: AppPage, after?: () => void) => void;
  /** Keep editing: dismiss the prompt and leave the page mounted. */
  keepEditing: () => void;
  /** Discard changes: navigate to the waiting destination. */
  discard: () => void;
}

const PO_DISCARD_DESCRIPTION =
  "The draft was saved earlier. Changes since then will be lost.";

export function leaveGuardDescription(page: AppPage): string {
  switch (page) {
    case "create":
      return "The quotation in the editor has unsaved changes. They will be lost.";
    case "admin-company":
      return "The company profile has unsaved changes. They will be lost.";
    default:
      return PO_DISCARD_DESCRIPTION;
  }
}

export function usePurchaseOrderLeaveGuard(
  currentPage: AppPage,
  navigate: (page: AppPage) => void,
  options?: LeaveGuardOptions
): AppLeaveGuard {
  const [dirty, setDirty] = useState(false);
  const [pending, setPending] = useState<{
    page: AppPage;
    after: (() => void) | null;
    returnFocus: HTMLElement | null;
  } | null>(null);

  // The callbacks are read at decision time, not captured in the memo, so a
  // caller may pass a fresh object every render without re-creating `request`
  // and re-registering App's global shortcut listener.
  const optionsRef = useRef(options);
  optionsRef.current = options;

  const pageIsDirty = useCallback(
    (candidate: AppPage): boolean =>
      optionsRef.current?.dirtyPages?.[candidate] === true ||
      (candidate === "po" && dirty),
    [dirty]
  );

  const request = useCallback(
    (next: AppPage, after?: () => void) => {
      // Going to the page you are already on is not leaving; everything else
      // is. There is exactly one pending decision: Cancel and a tab click both
      // arrive here, and a second click replaces the first rather than stacking
      // a dialog. A same-page request with its own follow-up (leaving correction
      // mode back to the saved view) also asks first, because it would unmount
      // the editor just the same.
      if (pageIsDirty(currentPage) && (next !== currentPage || after !== undefined)) {
        const active = typeof document === "undefined" ? null : document.activeElement;
        // A click inside the open dialog is not a new place to return to:
        // keep the control that opened the decision. In a browser the shell
        // is inert, so this only matters to the replace-pending path.
        const trigger =
          active instanceof HTMLElement &&
          active.closest('[role="alertdialog"]') === null
            ? active
            : null;
        setPending((previous) => ({
          page: next,
          after: after ?? null,
          returnFocus: trigger ?? previous?.returnFocus ?? null,
        }));
        return;
      }
      if (after) {
        after();
        return;
      }
      navigate(next);
    },
    [currentPage, navigate, pageIsDirty]
  );

  const keepEditing = useCallback(() => setPending(null), []);

  const discard = useCallback(() => {
    if (pending === null) return;
    const target = pending;
    const source = currentPage;
    setPending(null);
    // The editor unmounts on the next render; clearing first stops this gate
    // from asking a second time for the same navigation.
    setDirty(false);
    optionsRef.current?.onDiscard?.(source);
    if (target.after) {
      target.after();
      return;
    }
    navigate(target.page);
  }, [currentPage, navigate, pending]);

  const describe = optionsRef.current?.describe ?? leaveGuardDescription;

  return {
    dirty,
    setDirty,
    pendingPage: pending?.page ?? null,
    pendingDescription: pending === null ? "" : describe(currentPage),
    pendingReturnFocus: pending?.returnFocus ?? null,
    request,
    keepEditing,
    discard,
  };
}
