import { useCallback, useState } from "react";

// Issue #468, wave 2: leaving the PO workspace must not silently discard a
// draft's unsaved reducer state. The editor owns the reducer and reports its
// dirty flag; this hook owns the navigation decision so AppShell tabs and the
// keyboard shortcuts share one gate instead of each losing the work their own
// way.

export type AppPage =
  | "home"
  | "po"
  | "create"
  | "drafts"
  | "archive"
  | "detail"
  | "users"
  | "clients"
  | "settings";

export interface PurchaseOrderLeaveGuard {
  /** True while the mounted editor holds changes the saved draft lacks. */
  dirty: boolean;
  /** Called by the editor with its own isPurchaseOrderDirty answer. */
  setDirty: (dirty: boolean) => void;
  /** The destination waiting behind the discard confirmation, if any. */
  pendingPage: AppPage | null;
  /**
   * Navigate, or ask first when leaving a dirty PO editor. `after` replaces
   * the plain navigation for a destination that needs its own work first
   * (mod+N starts a fresh quotation); it runs only once the guard allows it.
   */
  request: (next: AppPage, after?: () => void) => void;
  /** Keep editing: dismiss the prompt and leave the editor mounted. */
  keepEditing: () => void;
  /** Discard changes: navigate to the waiting destination. */
  discard: () => void;
}

export function usePurchaseOrderLeaveGuard(
  currentPage: AppPage,
  navigate: (page: AppPage) => void
): PurchaseOrderLeaveGuard {
  const [dirty, setDirty] = useState(false);
  const [pending, setPending] = useState<{
    page: AppPage;
    after: (() => void) | null;
  } | null>(null);

  const request = useCallback(
    (next: AppPage, after?: () => void) => {
      // Going to "po" from "po" is not leaving; everything else is. The
      // editor's own Cancel path still uses its inline panel, and this gate
      // never re-derives dirtiness -- it trusts the lifted flag.
      if (currentPage === "po" && dirty && next !== "po") {
        setPending({ page: next, after: after ?? null });
        return;
      }
      if (after) {
        after();
        return;
      }
      navigate(next);
    },
    [currentPage, dirty, navigate]
  );

  const keepEditing = useCallback(() => setPending(null), []);

  const discard = useCallback(() => {
    if (pending === null) return;
    const target = pending;
    setPending(null);
    // The editor unmounts on the next render; clearing first stops this gate
    // from asking a second time for the same navigation.
    setDirty(false);
    if (target.after) {
      target.after();
      return;
    }
    navigate(target.page);
  }, [navigate, pending]);

  return {
    dirty,
    setDirty,
    pendingPage: pending?.page ?? null,
    request,
    keepEditing,
    discard,
  };
}
