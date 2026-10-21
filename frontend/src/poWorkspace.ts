// Issue #473 wave 3: the Purchase Order workspace's state controller.
//
// App used to decide the PO page by re-deriving three booleans in JSX: whether
// a record was open, whether it was saved, and whether a correction was in
// progress. Cancel from a correction then only cleared the third flag, so a
// stale draft -- a draft opened before someone else finalized it -- kept the
// editor on screen. The controller owns those transitions in one place, and
// Cancel from a correction reloads the server's record before showing it, so
// the page can never present a working copy as the saved record.
//
// The hook is separated from App so the two entry paths into a saved-record
// correction (the conflict says `saved`, and Overwrite racing into
// `po_not_draft`) can be exercised through this controller and the real page
// composition in a DOM test, instead of a mocked `onCancel`.

import { useCallback, useRef, useState } from "react";
import { GetPurchaseOrder } from "@backend/backend/Quotation";
import { isSavedPurchaseOrder } from "./poFinalize";
import type { AppPage } from "./poLeaveGuard";
import type { PurchaseOrderDTO } from "./purchaseOrder";

/**
 * What the PO page shows.
 *
 * - `list` -- no record open; the archive.
 * - `refreshing` -- a correction was cancelled and the saved record is being
 *   re-read. Neither the stale editor nor a stale saved view is shown.
 * - `saved` -- a saved record, read-only.
 * - `editor` -- a draft, or a saved record in correction mode.
 */
export type PurchaseOrderWorkspacePhase =
  | "list"
  | "refreshing"
  | "saved"
  | "editor";

export interface PurchaseOrderWorkspaceOptions {
  /**
   * A reload that fails after Cancel. The controller drops the stale record
   * rather than showing it, and the page owns the message.
   */
  onRefreshError?: (error: unknown) => void;
}

export interface PurchaseOrderWorkspace {
  /** The record the page is showing, or null for the archive. */
  draft: PurchaseOrderDTO | null;
  /** True while the open record is being edited as a correction. */
  correcting: boolean;
  phase: PurchaseOrderWorkspacePhase;
  /** Opens a record fetched by any entry path (list row, selector). */
  open: (order: PurchaseOrderDTO) => void;
  /** Leaves the record for the archive without requesting navigation. */
  close: () => void;
  /** Saved view's Correct: edit the open saved record. */
  correct: () => void;
  /**
   * The editor moved its working copy into a correction after the server said
   * the record is saved. The record itself is untouched until it saves.
   */
  correctionStarted: () => void;
  /** The editor stored the record; the server's answer becomes the record. */
  saved: (order: PurchaseOrderDTO) => void;
  /** The saved view assigned a confirmation number. */
  assigned: (order: PurchaseOrderDTO) => void;
  /**
   * The editor's Cancel. A draft leaves the workspace through App's guard. A
   * correction exits only after the guard allows it, then re-reads the saved
   * record so the saved view shows the server's current values rather than the
   * working copy Cancel just discarded.
   */
  cancel: (
    requestLeave: (next: AppPage, after?: () => void) => void,
    setDirty: (dirty: boolean) => void
  ) => void;
}

export function usePurchaseOrderWorkspace(
  options: PurchaseOrderWorkspaceOptions = {}
): PurchaseOrderWorkspace {
  const [draft, setDraft] = useState<PurchaseOrderDTO | null>(null);
  const [correcting, setCorrecting] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  // Every transition that replaces or closes the open record invalidates an
  // in-flight reload, so a slow read cannot reopen a record the user left.
  const loadToken = useRef(0);

  const open = useCallback((order: PurchaseOrderDTO) => {
    loadToken.current += 1;
    setRefreshing(false);
    setCorrecting(false);
    setDraft(order);
  }, []);

  const close = useCallback(() => {
    loadToken.current += 1;
    setRefreshing(false);
    setCorrecting(false);
    setDraft(null);
  }, []);

  const correct = useCallback(() => {
    setCorrecting(true);
  }, []);

  const correctionStarted = useCallback(() => {
    setCorrecting(true);
  }, []);

  const saved = useCallback((order: PurchaseOrderDTO) => {
    loadToken.current += 1;
    setRefreshing(false);
    setCorrecting(false);
    setDraft(order);
  }, []);

  const assigned = useCallback((order: PurchaseOrderDTO) => {
    setDraft(order);
  }, []);

  const onRefreshError = options.onRefreshError;
  const cancel = useCallback(
    (
      requestLeave: (next: AppPage, after?: () => void) => void,
      setDirty: (dirty: boolean) => void
    ) => {
      if (!correcting) {
        requestLeave("home", () => {
          setDirty(false);
          close();
        });
        return;
      }
      const id = draft?.ID;
      // Cancel is a discard decision, so the guard asks first when the working
      // copy is dirty; the reload runs only once it is allowed.
      requestLeave("po", () => {
        setDirty(false);
        if (id === undefined) {
          close();
          return;
        }
        // The record Cancel returns to is the server's, not the working copy:
        // another writer may have corrected or finalized it since this editor
        // loaded it, and a correction transition can start from a draft the
        // server has already saved. Reading it back is what makes the saved
        // view truthful, and the refreshing phase unmounts the editor so none
        // of its local correction state (savedCorrection, applied review
        // choices) can survive the exit.
        const token = loadToken.current + 1;
        loadToken.current = token;
        setRefreshing(true);
        void GetPurchaseOrder(id)
          .then((fresh) => {
            if (loadToken.current !== token) return;
            setDraft(fresh);
            setCorrecting(false);
            setRefreshing(false);
          })
          .catch((error: unknown) => {
            if (loadToken.current !== token) return;
            // Showing the stale draft would repeat the defect; the archive is
            // the honest destination when the saved record cannot be read.
            close();
            onRefreshError?.(error);
          });
      });
    },
    [correcting, close, draft, onRefreshError]
  );

  const phase: PurchaseOrderWorkspacePhase =
    draft === null
      ? "list"
      : refreshing
        ? "refreshing"
        : isSavedPurchaseOrder(draft) && !correcting
          ? "saved"
          : "editor";

  return {
    draft,
    correcting,
    phase,
    open,
    close,
    correct,
    correctionStarted,
    saved,
    assigned,
    cancel,
  };
}
