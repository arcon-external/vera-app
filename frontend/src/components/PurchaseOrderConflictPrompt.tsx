import { useEffect, useRef } from "react";
import {
  conflictWarning,
  formatHistoryTime,
  type PurchaseOrderConflictDTO,
} from "../poCorrection";
import { Button, Card } from "./ui";

// Issue #473: the one warning a stale save receives. It is modal, because the
// decision is exactly two choices -- Cancel, which keeps every unsaved value in
// the editor, or Overwrite, which rechecks the version and applies the working
// copy while keeping the replaced version in the history.
//
// Wave 2: when the record is saved, Overwrite would be guaranteed to fail
// (po_not_draft). The same dialog then offers the correction flow instead, which
// carries the working copy into the form and requires the reason a saved-record
// write needs. The text always comes from the server's current version, actor
// and time, so a second intervening save refreshes these words instead of
// silently applying the older warning.

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusableWithin(dialog: HTMLElement): HTMLElement[] {
  return Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
}

export interface PurchaseOrderConflictPromptProps {
  conflict: PurchaseOrderConflictDTO;
  overwriting: boolean;
  /**
   * True when the editor is a draft and the record is already saved: the only
   * safe offer is the correction transition, never a promised overwrite.
   */
  savedTransition?: boolean;
  onCancel: () => void;
  /** Opens the correction form with the working copy and a required reason. */
  onCorrectSaved: () => void;
  onOverwrite: () => void;
}

export function PurchaseOrderConflictPrompt({
  conflict,
  overwriting,
  savedTransition = false,
  onCancel,
  onCorrectSaved,
  onOverwrite,
}: PurchaseOrderConflictPromptProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const cancel = useRef(onCancel);
  cancel.current = onCancel;

  useEffect(() => {
    const dialog = dialogRef.current;
    const initial = dialog === null ? null : focusableWithin(dialog)[0] ?? dialog;
    initial?.focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        cancel.current();
        return;
      }
      if (event.key !== "Tab" || dialog === null) return;
      const focusable = focusableWithin(dialog);
      if (focusable.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      const inside = dialog.contains(active);
      if (event.shiftKey && (active === first || !inside)) {
        event.preventDefault();
        last.focus();
        return;
      }
      if (!event.shiftKey && (active === last || !inside)) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", onKeyDown, true);
    return () => document.removeEventListener("keydown", onKeyDown, true);
    // The dialog lives for one decision; callbacks stay fresh through the ref.
  }, []);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/45 p-5"
      role="presentation"
    >
      <div className="w-full max-w-lg">
        <Card className="border-danger-200 p-4">
          <div
            ref={dialogRef}
            role="alertdialog"
            aria-modal="true"
            aria-label="This purchase order changed"
            tabIndex={-1}
          >
            <p className="text-lead font-medium text-ink-900">
              This purchase order changed
            </p>
            <p className="mt-2 text-small text-ink-700">
              {conflictWarning(conflict, savedTransition)}
            </p>
            <dl className="mt-3 grid gap-1 text-caption text-ink-500 sm:grid-cols-2">
              <div>
                <dt className="inline">Current version: </dt>
                <dd className="inline font-medium text-ink-800">
                  {conflict.CurrentVersion}
                </dd>
              </div>
              {formatHistoryTime(conflict.UpdatedAt) === "" ? null : (
                <div>
                  <dt className="inline">Saved at: </dt>
                  <dd className="inline font-medium text-ink-800">
                    {formatHistoryTime(conflict.UpdatedAt)}
                  </dd>
                </div>
              )}
            </dl>
            <p className="mt-3 text-caption text-ink-500">
              {savedTransition
                ? "Cancel keeps everything you have typed. Correct saved record carries your working copy into the correction form, where a reason is required and the replaced values are kept in the history."
                : "Cancel keeps everything you have typed. Overwrite applies your working copy to the current record and keeps the replaced values in the correction history."}
            </p>
            <div className="mt-3 flex flex-wrap gap-2">
              <Button aria-label="Cancel and keep editing" onClick={onCancel}>
                Cancel
              </Button>
              {savedTransition ? (
                <Button
                  variant="primary"
                  aria-label="Correct saved record"
                  disabled={overwriting}
                  onClick={onCorrectSaved}
                >
                  {overwriting ? "Opening the correction…" : "Correct saved record"}
                </Button>
              ) : (
                <Button
                  variant="primary"
                  aria-label="Overwrite"
                  disabled={overwriting}
                  onClick={onOverwrite}
                >
                  {overwriting ? "Overwriting…" : "Overwrite"}
                </Button>
              )}
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
}
