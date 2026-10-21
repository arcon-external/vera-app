import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import {
  ConfirmDeletion,
  CreateDeletionChallenge,
  PreviewDeletion,
} from "@backend/backend/Quotation";
import {
  buildDeletionChallengeInput,
  buildDeletionConfirmInput,
  canConfirmDeletion,
  deletionConfirmErrors,
  deletionReducer,
  deletionScopeConsequences,
  deletionTitle,
  formatChallengeExpiry,
  initialDeletionState,
  normalizeChallengeLetters,
  selectedDeletionScope,
  type DeletionChangedDTO,
  type DeletionPreviewDTO,
  type DeletionScope,
  type DeletionState,
  type DeletionTargetType,
} from "../deletion";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";

// Issue #480: the permanent-deletion dialog.
//
// It is deliberately a two-part surface: a pure view (what an admin sees and
// what is disabled) and a container that owns the three API calls. The view is
// rendered by the SSR suite and the snapshot harness; the container is driven by
// the DOM suite. Nothing here decides eligibility: the preview's scopes are the
// server's answer, and a changed chain replaces the preview with the fresh one.

export interface DeletionDialogViewProps {
  state: DeletionState;
  /** The record named by the opener, shown while the preview loads. */
  targetLabel: string;
  onClose: () => void;
  onScope: (scope: DeletionScope) => void;
  onReason: (value: string) => void;
  onTypedNumber: (value: string) => void;
  onChallengeInput: (value: string) => void;
  onRequestChallenge: () => void;
  onChangeReviewed: () => void;
  onConfirm: () => void;
}

function SourceMissingBadge() {
  return (
    <span className="inline-flex items-center rounded-md border border-danger-200 bg-danger-50 px-2 py-0.5 text-micro font-medium uppercase text-danger-700">
      Source missing
    </span>
  );
}

function recordMeta(row: {
  State: string;
  Version: number;
  Revisions: number[];
  DocumentVersions: number;
  HistoryEntries: number;
}): string {
  const parts: string[] = [];
  if (row.Revisions.length > 0) {
    parts.push(
      `${row.Revisions.length} issued ${row.Revisions.length === 1 ? "revision" : "revisions"} (${row.Revisions.join(", ")})`
    );
  }
  if (row.State !== "") {
    parts.push(row.State === "saved" ? "saved" : row.State);
  }
  if (row.Version > 0) parts.push(`version ${row.Version}`);
  if (row.DocumentVersions > 0) {
    parts.push(
      `${row.DocumentVersions} ${row.DocumentVersions === 1 ? "attachment" : "attachments"}`
    );
  }
  if (row.HistoryEntries > 0) {
    parts.push(
      `${row.HistoryEntries} history ${row.HistoryEntries === 1 ? "version" : "versions"}`
    );
  }
  return parts.join(" · ");
}

/** One connected document in the list the admin actually reads. */
function DeletionRecordRowView({
  row,
  fate,
}: {
  row: {
    Label: string;
    Type: string;
    DocumentNumber: string;
    Revisions?: number[];
    State?: string;
    Version?: number;
    DocumentVersions?: number;
    HistoryEntries?: number;
  };
  fate: "removed" | "preserved";
}) {
  const meta = recordMeta({
    State: row.State ?? "",
    Version: row.Version ?? 0,
    Revisions: row.Revisions ?? [],
    DocumentVersions: row.DocumentVersions ?? 0,
    HistoryEntries: row.HistoryEntries ?? 0,
  });
  return (
    <li className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 border-b border-rule/70 py-2 last:border-b-0">
      <div className="min-w-0">
        <p className="break-words text-small font-medium text-ink-900">{row.Label}</p>
        {meta !== "" ? (
          <p className="text-caption text-ink-500">{meta}</p>
        ) : null}
      </div>
      <Badge tone={fate === "removed" ? "neutral" : "success"}>
        {fate === "removed"
          ? "Removed"
          : row.Type === "purchase_order"
            ? "Survives as Source missing"
            : "Stays"}
      </Badge>
    </li>
  );
}

const SCOPE_COPY: Record<DeletionScope, { title: string; hint: string }> = {
  record: {
    title: "Delete this record only",
    hint: "The connected document on the other side of the chain stays.",
  },
  chain: {
    title: "Delete the entire chain",
    hint: "The confirmed quotation and its purchase order are removed together.",
  },
};

/** The whole-chain second confirmation. Exported so the snapshot harness can
 *  show the issued letters without the dialog's scroll position hiding them. */
export function DeletionChallengePanel({
  state,
  challengeError,
  onChallengeInput,
  onRequestChallenge,
}: {
  state: DeletionState;
  challengeError?: string;
  onChallengeInput: (value: string) => void;
  onRequestChallenge: () => void;
}) {
  return (
    <Card className="border-danger-200 p-4" data-deletion-challenge>
      <SectionHeading title="Ten-letter challenge" hint="Whole-chain deletion" />
      {state.challenge === null ? (
        <div className="flex flex-col gap-2">
          <p className="text-small text-ink-700">
            Whole-chain deletion needs a second confirmation. Vera issues ten
            random uppercase letters, excluding I, L and O, and accepts them in
            any case.
          </p>
          <div>
            <Button
              size="sm"
              onClick={onRequestChallenge}
              disabled={state.challengeLoading || state.phase === "submitting"}
            >
              {state.challengeLoading
                ? "Generating…"
                : "Generate the ten-letter challenge"}
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <p className="tnum break-all text-h2 font-semibold tracking-[0.18em] text-ink-950">
            {state.challenge.Letters}
          </p>
          <p className="text-caption text-ink-500">
            Type these ten letters below. They expire at{" "}
            {formatChallengeExpiry(state.challenge.ExpiresAt)}.
          </p>
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              The ten letters
            </span>
            <input
              className={`${fieldClass} tnum uppercase`}
              aria-label="Ten-letter challenge"
              autoComplete="off"
              spellCheck={false}
              value={state.challengeInput}
              disabled={state.phase === "submitting"}
              onChange={(event) =>
                onChallengeInput(normalizeChallengeLetters(event.target.value))
              }
            />
            {challengeError !== undefined ? (
              <span className="text-caption text-danger-700">{challengeError}</span>
            ) : null}
          </label>
        </div>
      )}
      {state.challengeError !== null ? (
        <div className="mt-2">
          <StatusBanner tone="danger">{state.challengeError}</StatusBanner>
        </div>
      ) : null}
    </Card>
  );
}

export function DeletionDialogView({
  state,
  targetLabel,
  onClose,
  onScope,
  onReason,
  onTypedNumber,
  onChallengeInput,
  onRequestChallenge,
  onChangeReviewed,
  onConfirm,
}: DeletionDialogViewProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    dialogRef.current?.focus();
  }, []);
  const preview = state.preview;
  const plan = selectedDeletionScope(state);
  const errors = deletionConfirmErrors(state);
  const confirmable = canConfirmDeletion(state);
  const submitting = state.phase === "submitting";
  const consequences = deletionScopeConsequences(state);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-ink-950/45 p-4 sm:p-5"
      role="presentation"
      onClick={submitting ? undefined : onClose}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="deletion-dialog-heading"
        tabIndex={-1}
        onKeyDown={(event) => {
          if (event.key === "Escape" && !submitting) onClose();
        }}
        className="flex max-h-[90vh] w-full max-w-2xl flex-col rounded-xl border border-rule bg-white shadow-card outline-none"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-3 border-b border-rule px-5 py-4 sm:px-6">
          <div className="min-w-0">
            <h2
              id="deletion-dialog-heading"
              className="break-words text-title font-semibold text-ink-950"
            >
              {deletionTitle(preview)}
            </h2>
            <p className="mt-0.5 break-words text-small text-ink-500">
              Permanently deletes a record. Admin only.
            </p>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose} disabled={submitting}>
            Close
          </Button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4 sm:px-6">
          {preview === null ? (
            <StatusBanner tone="info">
              {state.error === null
                ? `Loading ${targetLabel}…`
                : state.error}
            </StatusBanner>
          ) : (
            <div className="flex flex-col gap-4">
              <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                <span className="tnum text-h2 font-semibold text-ink-950">
                  {preview.DocumentNumber}
                </span>
                <span className="text-small text-ink-500">
                  {preview.DocumentNumberLabel}
                </span>
                {preview.SourceMissing ? <SourceMissingBadge /> : null}
                <span className="text-small text-ink-700">{preview.CustomerName}</span>
              </div>
              <p className="text-caption text-ink-500">
                {preview.TargetType === "quotation"
                  ? `Quotation ${preview.QuoteNumber}`
                  : `Quotation ${preview.QuoteNumber} · revision ${preview.Version}`}
                {preview.State !== "" ? ` · ${preview.State}` : ""}
              </p>

              {state.changes.length > 0 ? (
                <Card className="border-danger-200 p-4" data-deletion-changed>
                  <SectionHeading
                    title="The chain changed"
                    hint="Nothing was deleted"
                  />
                  <p className="text-small text-ink-700">
                    The documents moved after you confirmed them. Deletion
                    stopped, and both confirmations must be repeated against the
                    new chain.
                  </p>
                  <ul className="mt-2 grid gap-1 text-small text-ink-900">
                    {state.changes.map((change) => (
                      <li key={change.Field}>
                        <span className="font-medium">{change.Label}: </span>
                        <span className="tnum">
                          {change.Before || "—"} → {change.After || "—"}
                        </span>
                      </li>
                    ))}
                  </ul>
                  <div className="mt-3">
                    <Button size="sm" onClick={onChangeReviewed}>
                      Review the new chain
                    </Button>
                  </div>
                </Card>
              ) : null}

              <fieldset className="grid gap-2">
                <legend className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
                  Scope
                </legend>
                {preview.Scopes.map((option) => (
                  <label
                    key={option.Scope}
                    className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 ${
                      state.scope === option.Scope
                        ? "border-accent-500 bg-accent-50/40"
                        : "border-rule bg-white"
                    } ${option.Available ? "" : "cursor-not-allowed opacity-60"}`}
                  >
                    <input
                      type="radio"
                      name="deletion-scope"
                      className="mt-1 h-4 w-4 accent-accent-700"
                      value={option.Scope}
                      checked={state.scope === option.Scope}
                      disabled={!option.Available || submitting}
                      onChange={() =>
                        onScope(option.Scope === "chain" ? "chain" : "record")
                      }
                    />
                    <span className="min-w-0">
                      <span className="block text-small font-medium text-ink-900">
                        {SCOPE_COPY[option.Scope === "chain" ? "chain" : "record"].title}
                      </span>
                      <span className="block text-caption text-ink-500">
                        {option.Available
                          ? SCOPE_COPY[option.Scope === "chain" ? "chain" : "record"].hint
                          : option.UnavailableReason}
                      </span>
                    </span>
                  </label>
                ))}
              </fieldset>

              {plan !== null && plan.Available ? (
                <Card className="p-4">
                  <SectionHeading
                    title="Connected records"
                    hint={`${plan.Removes.length} removed · ${plan.Preserves.length} kept`}
                  />
                  <ul className="grid">
                    {plan.Removes.map((row) => (
                      <DeletionRecordRowView
                        key={`remove-${row.Type}-${row.ID}`}
                        row={row}
                        fate="removed"
                      />
                    ))}
                    {plan.Preserves.map((row) => (
                      <DeletionRecordRowView
                        key={`keep-${row.Type}-${row.ID}`}
                        row={row}
                        fate="preserved"
                      />
                    ))}
                  </ul>
                </Card>
              ) : null}

              {consequences.length > 0 ? (
                <ul className="grid list-disc gap-1 pl-5 text-small text-ink-700">
                  {consequences.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              ) : null}

              <label className="grid gap-1.5">
                <span className="text-small font-medium text-ink-700">
                  Reason
                </span>
                <textarea
                  className={`${fieldClass} min-h-20`}
                  aria-label="Deletion reason"
                  aria-invalid={errors.reason !== undefined}
                  value={state.reason}
                  maxLength={500}
                  disabled={submitting}
                  onChange={(event) => onReason(event.target.value)}
                />
                {errors.reason !== undefined ? (
                  <span className="text-caption text-danger-700">{errors.reason}</span>
                ) : null}
              </label>

              <label className="grid gap-1.5">
                <span className="text-small font-medium text-ink-700">
                  Type{" "}
                  <span className="tnum font-semibold text-ink-950">
                    {preview.DocumentNumber}
                  </span>{" "}
                  to confirm
                </span>
                <input
                  className={fieldClass}
                  aria-label={`Type ${preview.DocumentNumber} to confirm`}
                  aria-invalid={errors.typedNumber !== undefined}
                  value={state.typedNumber}
                  autoComplete="off"
                  spellCheck={false}
                  disabled={submitting}
                  onChange={(event) => onTypedNumber(event.target.value)}
                />
                {errors.typedNumber !== undefined ? (
                  <span className="text-caption text-danger-700">
                    {errors.typedNumber}
                  </span>
                ) : null}
              </label>

              {plan?.RequiresChallenge ? (
                <DeletionChallengePanel
                  state={state}
                  challengeError={errors.challenge}
                  onChallengeInput={onChallengeInput}
                  onRequestChallenge={onRequestChallenge}
                />
              ) : null}

              {state.error !== null && preview !== null ? (
                <StatusBanner tone="danger">{state.error}</StatusBanner>
              ) : null}

              <StatusBanner tone="info">{state.backupNotice}</StatusBanner>
            </div>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule px-5 py-4 sm:px-6">
          <p className="text-caption text-ink-500">
            {plan?.RequiresChallenge
              ? "Reason, document number and the ten-letter challenge are required."
              : "Reason and the typed document number are required."}
          </p>
          <div className="flex items-center gap-2">
            <Button onClick={onClose} disabled={submitting}>
              Cancel
            </Button>
            <Button
              variant="danger"
              aria-label="Delete permanently"
              onClick={onConfirm}
              disabled={!confirmable || submitting}
            >
              {submitting ? "Deleting…" : "Delete permanently"}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

export interface DeletionDialogProps {
  targetType: DeletionTargetType;
  targetId: number;
  targetLabel: string;
  onClose: () => void;
  onDeleted: (preview: DeletionPreviewDTO) => void;
}

/**
 * The container: three calls and the transitions between them. A changed chain
 * or a dead challenge is a value, never an error string, so the modal can show
 * the changes and renew the confirmation without guessing.
 */
export function DeletionDialog({
  targetType,
  targetId,
  targetLabel,
  onClose,
  onDeleted,
}: DeletionDialogProps) {
  const [state, dispatch] = useReducer(deletionReducer, initialDeletionState);
  const [lastPreview, setLastPreview] = useState<DeletionPreviewDTO | null>(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => {
    dispatch({ type: "previewLoading" });
    PreviewDeletion({ TargetType: targetType, TargetID: targetId })
      .then((preview) => {
        if (!mounted.current) return;
        dispatch({ type: "previewLoaded", preview });
        setLastPreview(preview);
      })
      .catch((error: unknown) => {
        if (!mounted.current) return;
        dispatch({ type: "previewFailed", message: errorMessage(error) });
      });
  }, [targetType, targetId]);

  const adoptChanged = useCallback((changed: DeletionChangedDTO) => {
    dispatch({ type: "chainChanged", changed });
    if (changed.Preview != null) setLastPreview(changed.Preview);
  }, []);

  const requestChallenge = useCallback(async () => {
    const input = buildDeletionChallengeInput(state);
    if (input === null) return;
    dispatch({ type: "challengeRequested" });
    try {
      const result = await CreateDeletionChallenge(input);
      if (!mounted.current) return;
      if (result.Changed !== undefined && result.Changed !== null) {
        adoptChanged(result.Changed);
        return;
      }
      if (result.Challenge !== undefined && result.Challenge !== null) {
        dispatch({ type: "challengeLoaded", challenge: result.Challenge });
      }
    } catch (error) {
      if (!mounted.current) return;
      dispatch({ type: "challengeRequestFailed", message: errorMessage(error) });
    }
  }, [state, adoptChanged]);

  const confirm = useCallback(async () => {
    if (!canConfirmDeletion(state)) return;
    dispatch({ type: "submitRequested" });
    try {
      const result = await ConfirmDeletion(buildDeletionConfirmInput(state));
      if (!mounted.current) return;
      if (result.Changed !== undefined && result.Changed !== null) {
        adoptChanged(result.Changed);
        return;
      }
      if (result.ChallengeProblem !== undefined && result.ChallengeProblem !== "") {
        dispatch({
          type: "challengeProblem",
          problem: result.ChallengeProblem as "required" | "invalid" | "expired",
        });
        return;
      }
      if (result.Outcome !== undefined && result.Outcome !== null) {
        dispatch({ type: "succeeded", outcome: result.Outcome });
        const preview = state.preview ?? lastPreview;
        if (preview !== null) onDeleted(preview);
        return;
      }
      dispatch({ type: "failed", message: "The deletion did not complete." });
    } catch (error) {
      if (!mounted.current) return;
      dispatch({ type: "failed", message: errorMessage(error) });
    }
  }, [state, lastPreview, adoptChanged, onDeleted]);

  return (
    <DeletionDialogView
      state={state}
      targetLabel={targetLabel}
      onClose={onClose}
      onScope={(scope) => dispatch({ type: "scopeChanged", scope })}
      onReason={(value) => dispatch({ type: "reasonChanged", value })}
      onTypedNumber={(value) => dispatch({ type: "typedNumberChanged", value })}
      onChallengeInput={(value) =>
        dispatch({ type: "challengeInputChanged", value })
      }
      onRequestChallenge={() => void requestChallenge()}
      onChangeReviewed={() => dispatch({ type: "changesReviewed" })}
      onConfirm={() => void confirm()}
    />
  );
}

function errorMessage(error: unknown): string {
  if (typeof error === "string" && error.trim() !== "") return error;
  if (error instanceof Error && error.message.trim() !== "") return error.message;
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message.trim() !== "") return message;
  }
  return "The deletion could not be completed. Try again.";
}
