import { useEffect, useRef, useState } from "react";
import { Button } from "./ui";

// Issue #656: closing a quotation is an admin decision with a mandatory reason.
// The dialog owns only the typing and its validation; the caller owns the
// request, so a refusal leaves the dialog (and the typed reason) on screen. It
// never closes itself on submit: an optimistic close is exactly how an admin
// loses the sentence they just wrote when the server refuses.

/** The stored reason bound, mirrored from the store's 500-character rule. */
export const QUOTATION_CLOSE_REASON_MAX = 500;

/**
 * Whether a reason may be submitted, and why not. Counting uses code points so
 * 500 ideographs are 500 characters, not 1500 bytes.
 */
export function closeReasonError(reason: string): string | null {
  const trimmed = reason.trim();
  if (trimmed === "") {
    return "Give a reason for closing this quotation.";
  }
  const length = Array.from(trimmed).length;
  if (length > QUOTATION_CLOSE_REASON_MAX) {
    return `Use at most ${QUOTATION_CLOSE_REASON_MAX} characters; this is ${length}.`;
  }
  return null;
}

export interface QuotationCloseDialogProps {
  open: boolean;
  /** Closing is an admin action; a non-admin never sees the dialog. */
  isAdmin: boolean;
  quotationNumber: string;
  /** True while the caller's request is in flight; the dialog stays open. */
  pending: boolean;
  /** The server's refusal, shown without clearing the typed reason. */
  error: string | null;
  onCancel: () => void;
  onConfirm: (reason: string) => void;
}

export function QuotationCloseDialog({
  open,
  isAdmin,
  quotationNumber,
  pending,
  error,
  onCancel,
  onConfirm,
}: QuotationCloseDialogProps) {
  const [reason, setReason] = useState("");
  const fieldRef = useRef<HTMLTextAreaElement | null>(null);

  // A fresh open starts a fresh sentence; the previous attempt's text must not
  // be closed a second time by accident.
  useEffect(() => {
    if (!open) return;
    setReason("");
  }, [open]);

  useEffect(() => {
    if (!open) return;
    fieldRef.current?.focus();
  }, [open]);

  if (!open || !isAdmin) return null;
  const validation = closeReasonError(reason);
  const length = Array.from(reason.trim()).length;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/45 p-5"
      role="presentation"
      onClick={pending ? undefined : onCancel}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`Close quotation ${quotationNumber}`}
        className="w-full max-w-lg rounded-xl border border-rule bg-white shadow-card"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="border-b border-rule px-6 py-4">
          <h2 className="text-title font-semibold text-ink-950">
            Close this quotation?
          </h2>
          <p className="mt-1 text-small text-ink-500">
            The unbought remainder stops being Unsold and becomes Closed Loss.
            Reopening restores it; the who, when and why stay in the record.
          </p>
        </div>

        <div className="grid gap-3 px-6 py-5">
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              Why is it closing?
            </span>
            <textarea
              ref={fieldRef}
              aria-label="Close reason"
              className="min-h-24 w-full resize-y rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-hidden transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20"
              value={reason}
              disabled={pending}
              placeholder="For example: the customer cancelled the remaining quantity"
              onChange={(event) => setReason(event.target.value)}
            />
          </label>
          <p className="tnum text-caption text-ink-500">
            {length} / {QUOTATION_CLOSE_REASON_MAX} characters
          </p>
          {validation !== null && reason !== "" ? (
            <p role="alert" className="text-small text-danger-600">
              {validation}
            </p>
          ) : null}
          {error !== null ? (
            <p role="alert" className="text-small text-danger-600">
              {error}
            </p>
          ) : null}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-rule px-6 py-4">
          <Button
            aria-label="Cancel closing the quotation"
            disabled={pending}
            onClick={onCancel}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            aria-label="Confirm close quotation"
            disabled={pending || validation !== null}
            onClick={() => {
              if (validation !== null || pending) return;
              onConfirm(reason.trim());
            }}
          >
            {pending ? "Closing…" : "Close quotation"}
          </Button>
        </div>
      </div>
    </div>
  );
}
