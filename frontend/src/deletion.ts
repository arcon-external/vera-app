// Issue #480: the deletion dialog's decisions, kept out of the DOM.
//
// The rules live here so they are checkable without a browser: which scope is
// selected, when the typed number and the ten-letter challenge are complete,
// what a changed chain does to the confirmation, and the exact payload the
// server receives. The JSX only renders this state and forwards events.

export type DeletionTargetType = "quotation" | "purchase_order";
export type DeletionScope = "record" | "chain";

/** A-Z without I, L and O, case-insensitive. */
export const DELETION_CHALLENGE_ALPHABET = "ABCDEFGHJKMNPQRSTUVWXYZ";
export const DELETION_CHALLENGE_LENGTH = 10;

export interface DeletionRecordDTO {
  Type: string;
  UUID: string;
  DocumentNumber: string;
  SourceReference: string;
  Label: string;
  State: string;
  Revisions: number[];
  ID: number;
  Version: number;
  DocumentVersions: number;
  HistoryEntries: number;
}

export interface DeletionScopeDTO {
  Scope: string;
  UnavailableReason: string;
  Removes: DeletionRecordDTO[];
  Preserves: DeletionRecordDTO[];
  Available: boolean;
  RequiresChallenge: boolean;
}

export interface DeletionPreviewDTO {
  TargetType: string;
  TargetUUID: string;
  DocumentNumber: string;
  DocumentNumberLabel: string;
  QuoteNumber: string;
  PONumber: string;
  CustomerName: string;
  State: string;
  Fingerprint: string;
  Scopes: DeletionScopeDTO[];
  TargetID: number;
  Version: number;
  SourceMissing: boolean;
}

export interface DeletionChallengeDTO {
  ChallengeID: string;
  Letters: string;
  ExpiresAt: string;
  Fingerprint: string;
}

export interface DeletionChangeDTO {
  Field: string;
  Label: string;
  Before: string;
  After: string;
}

export interface DeletionChangedDTO {
  Preview?: DeletionPreviewDTO | null;
  Changes: DeletionChangeDTO[];
}

export interface DeletionOutcomeDTO {
  Removed: DeletionRecordDTO[];
  AuditIDs: number[];
}

export interface DeletionConfirmInputPayload {
  TargetType: string;
  TargetID: number;
  Scope: string;
  Reason: string;
  TypedNumber: string;
  ExpectedFingerprint: string;
  ChallengeID: string;
  ChallengeLetters: string;
}

export interface DeletionChallengeInputPayload {
  TargetType: string;
  TargetID: number;
  Scope: string;
  ExpectedFingerprint: string;
}

export type DeletionPhase = "idle" | "loading" | "ready" | "submitting" | "done";

export interface DeletionState {
  phase: DeletionPhase;
  preview: DeletionPreviewDTO | null;
  scope: DeletionScope;
  reason: string;
  typedNumber: string;
  challenge: DeletionChallengeDTO | null;
  challengeInput: string;
  challengeLoading: boolean;
  changes: DeletionChangeDTO[];
  error: string | null;
  challengeError: string | null;
  outcome: DeletionOutcomeDTO | null;
  backupNotice: string;
}

export type DeletionAction =
  | { type: "previewLoading" }
  | { type: "previewLoaded"; preview: DeletionPreviewDTO }
  | { type: "previewFailed"; message: string }
  | { type: "scopeChanged"; scope: DeletionScope }
  | { type: "reasonChanged"; value: string }
  | { type: "typedNumberChanged"; value: string }
  | { type: "challengeRequested" }
  | { type: "challengeLoaded"; challenge: DeletionChallengeDTO }
  | { type: "challengeRequestFailed"; message: string }
  | { type: "challengeInputChanged"; value: string }
  | { type: "challengeProblem"; problem: "required" | "invalid" | "expired" }
  | { type: "submitRequested" }
  | { type: "chainChanged"; changed: DeletionChangedDTO }
  | { type: "changesReviewed" }
  | { type: "succeeded"; outcome: DeletionOutcomeDTO }
  | { type: "failed"; message: string };

/** What deletion keeps in backups, stated once so every surface says it. */
export const DELETION_BACKUP_NOTICE =
  "This removes the record from the live database immediately. Vera keeps only a minimal audit entry: who deleted it, when, which document, which source it followed, and your reason. Backups follow the operator's retention schedule, so a copy may exist in a backup until it expires; backups are never used to reinstate a deleted record automatically.";

export const emptyDeletionState: DeletionState = {
  phase: "idle",
  preview: null,
  scope: "record",
  reason: "",
  typedNumber: "",
  challenge: null,
  challengeInput: "",
  challengeLoading: false,
  changes: [],
  error: null,
  challengeError: null,
  outcome: null,
  backupNotice: DELETION_BACKUP_NOTICE,
};

export const initialDeletionState: DeletionState = emptyDeletionState;

function availableScope(
  preview: DeletionPreviewDTO | null,
  preferred: DeletionScope
): DeletionScope {
  if (preview === null) return preferred;
  const plan = preview.Scopes.find((entry) => entry.Scope === preferred);
  if (plan?.Available) return preferred;
  const record = preview.Scopes.find(
    (entry) => entry.Scope === "record" && entry.Available
  );
  return record ? "record" : preferred;
}

/** The selected scope's plan, or null before the preview arrives. */
export function selectedDeletionScope(
  state: DeletionState
): DeletionScopeDTO | null {
  return (
    state.preview?.Scopes.find((plan) => plan.Scope === state.scope) ?? null
  );
}

export function deletionReducer(
  state: DeletionState,
  action: DeletionAction
): DeletionState {
  switch (action.type) {
    case "previewLoading":
      return {
        ...emptyDeletionState,
        phase: "loading",
        reason: state.reason,
      };
    case "previewLoaded":
      return {
        ...state,
        phase: "ready",
        preview: action.preview,
        scope: availableScope(action.preview, state.scope),
        challenge: null,
        challengeInput: "",
        challengeLoading: false,
        error: null,
        challengeError: null,
        outcome: null,
      };
    case "previewFailed":
      return { ...state, phase: "ready", error: action.message };
    case "scopeChanged":
      return {
        ...state,
        scope: action.scope,
        challenge: null,
        challengeInput: "",
        challengeLoading: false,
        challengeError: null,
        error: null,
      };
    case "reasonChanged":
      return { ...state, reason: action.value, error: null };
    case "typedNumberChanged":
      return { ...state, typedNumber: action.value, error: null };
    case "challengeRequested":
      return { ...state, challengeLoading: true, challengeError: null };
    case "challengeLoaded":
      return {
        ...state,
        challengeLoading: false,
        challengeError: null,
        challenge: action.challenge,
        challengeInput: "",
        preview:
          state.preview === null
            ? null
            : { ...state.preview, Fingerprint: action.challenge.Fingerprint },
      };
    case "challengeRequestFailed":
      return {
        ...state,
        challengeLoading: false,
        challengeError: action.message,
      };
    case "challengeInputChanged":
      return { ...state, challengeInput: action.value, error: null };
    case "challengeProblem":
      return {
        ...state,
        challenge: null,
        challengeInput: "",
        challengeLoading: false,
        error:
          action.problem === "expired"
            ? "The ten-letter challenge expired. Generate a fresh one and type it again."
            : action.problem === "required"
              ? "Whole-chain deletion needs the ten-letter challenge."
              : "Those letters did not match the challenge. Check them and try again.",
      };
    case "submitRequested":
      return { ...state, phase: "submitting", error: null };
    case "chainChanged": {
      const preview = action.changed.Preview ?? state.preview;
      return {
        ...state,
        phase: "ready",
        preview,
        scope: availableScope(preview, state.scope),
        changes: action.changed.Changes,
        challenge: null,
        challengeInput: "",
        typedNumber: "",
        error: null,
        challengeError: null,
      };
    }
    case "changesReviewed":
      return { ...state, changes: [], error: null };
    case "succeeded":
      return { ...state, phase: "done", outcome: action.outcome, error: null };
    case "failed":
      return { ...state, phase: "ready", error: action.message };
    default:
      return state;
  }
}

export function normalizeChallengeLetters(value: string): string {
  return value.replace(/\s+/g, "").toUpperCase();
}

export function challengeLettersAreValid(value: string): boolean {
  const normalized = normalizeChallengeLetters(value);
  if (normalized.length !== DELETION_CHALLENGE_LENGTH) return false;
  for (const letter of normalized) {
    if (!DELETION_CHALLENGE_ALPHABET.includes(letter)) return false;
  }
  return true;
}

function sameNumber(left: string, right: string): boolean {
  return (
    left.trim().toLocaleLowerCase() === right.trim().toLocaleLowerCase()
  );
}

export interface DeletionConfirmErrors {
  reason?: string;
  typedNumber?: string;
  challenge?: string;
}

/** Everything still missing before a confirmation may be submitted. */
export function deletionConfirmErrors(
  state: DeletionState
): DeletionConfirmErrors {
  const errors: DeletionConfirmErrors = {};
  if (state.reason.trim() === "") {
    errors.reason = "State why this record is being permanently deleted.";
  } else if (state.reason.trim().length > 500) {
    errors.reason = "Keep the reason to 500 characters or fewer.";
  }
  const plan = selectedDeletionScope(state);
  if (state.preview === null) {
    errors.typedNumber = "Load the record before confirming.";
    return errors;
  }
  if (state.typedNumber.trim() === "") {
    errors.typedNumber = `Type ${state.preview.DocumentNumber} to confirm.`;
  } else if (!sameNumber(state.typedNumber, state.preview.DocumentNumber)) {
    errors.typedNumber = `That does not match ${state.preview.DocumentNumber}.`;
  }
  if (plan?.RequiresChallenge) {
    if (state.challenge === null) {
      errors.challenge =
        "Generate the ten-letter challenge and type it to confirm whole-chain deletion.";
    } else if (state.challengeInput.trim() === "") {
      errors.challenge = "Type the ten letters shown for this chain.";
    } else if (
      !sameNumber(state.challengeInput, state.challenge.Letters)
    ) {
      errors.challenge = "Those letters do not match the challenge.";
    }
  }
  return errors;
}

export function canConfirmDeletion(state: DeletionState): boolean {
  if (state.phase !== "ready") return false;
  const plan = selectedDeletionScope(state);
  if (state.preview === null || plan === null || !plan.Available) return false;
  return Object.keys(deletionConfirmErrors(state)).length === 0;
}

export function buildDeletionChallengeInput(
  state: DeletionState
): DeletionChallengeInputPayload | null {
  const plan = selectedDeletionScope(state);
  if (state.preview === null || plan === null || !plan.RequiresChallenge) {
    return null;
  }
  return {
    TargetType: state.preview.TargetType,
    TargetID: state.preview.TargetID,
    Scope: state.scope,
    ExpectedFingerprint: state.preview.Fingerprint,
  };
}

export function buildDeletionConfirmInput(
  state: DeletionState
): DeletionConfirmInputPayload {
  const preview = state.preview;
  return {
    TargetType: preview?.TargetType ?? "",
    TargetID: preview?.TargetID ?? 0,
    Scope: state.scope,
    Reason: state.reason.trim(),
    TypedNumber: state.typedNumber.trim(),
    ExpectedFingerprint: preview?.Fingerprint ?? "",
    ChallengeID: state.challenge?.ChallengeID ?? "",
    ChallengeLetters: state.challengeInput.trim(),
  };
}

/**
 * The consequences of the selected scope in the words the modal shows. It reads
 * the plan rather than re-deriving the chain: the server is the authority on
 * what a deletion removes.
 */
export function deletionScopeConsequences(state: DeletionState): string[] {
  const plan = selectedDeletionScope(state);
  const preview = state.preview;
  if (preview === null || plan === null) {
    return [];
  }
  const lines: string[] = [];
  if (!plan.Available) {
    if (plan.UnavailableReason !== "") lines.push(plan.UnavailableReason);
    return lines;
  }
  const removed = plan.Removes;
  const attachments = removed.reduce(
    (total, row) => total + (row.DocumentVersions ?? 0),
    0
  );
  const history = removed.reduce(
    (total, row) => total + (row.HistoryEntries ?? 0),
    0
  );
  const revisions = removed.reduce(
    (total, row) => total + (row.Revisions?.length ?? 0),
    0
  );
  lines.push(
    state.scope === "chain"
      ? `This permanently removes ${countLabel(removed.length, "connected document")}, ${countLabel(revisions, "issued revision")}, ${countLabel(attachments, "attachment")} and ${countLabel(history, "history version")}. It cannot be undone.`
      : `This permanently removes ${countLabel(removed.length, "document")} with ${countLabel(revisions, "issued revision")}, ${countLabel(attachments, "attachment")} and ${countLabel(history, "history version")}. It cannot be undone.`
  );
  if (plan.Preserves.length > 0) {
    for (const row of plan.Preserves) {
      if (row.Type === "purchase_order") {
        lines.push(
          `${row.Label} survives as Source missing: its own recorded values stay readable, but it can no longer be saved or corrected until an admin repairs the source.`
        );
      } else {
        lines.push(`${row.Label} stays issued and unchanged.`);
      }
    }
  }
  if (state.scope === "chain") {
    lines.push(
      "Confirmation counters are never rewound: any issued confirmation number stays consumed."
    );
  }
  // A scope that is not offered is explained rather than hidden: the admin must
  // know why whole-chain deletion is missing from the choice.
  for (const other of preview.Scopes) {
    if (!other.Available && other.UnavailableReason !== "") {
      lines.push(other.UnavailableReason);
    }
  }
  return lines;
}

/**
 * The challenge expiry in the company's commercial time (WIB), matching the
 * numbering contract's date context. An unparseable value is shown as-is rather
 * than as "Invalid Date".
 */
export function formatChallengeExpiry(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return new Intl.DateTimeFormat("en-GB", {
    timeZone: "Asia/Jakarta",
    dateStyle: "medium",
    timeStyle: "short",
  }).format(parsed);
}

function countLabel(count: number, singular: string): string {
  return `${count} ${singular}${count === 1 ? "" : "s"}`;
}

/** The preview's document number in the modal's title area. */
export function deletionTitle(preview: DeletionPreviewDTO | null): string {
  if (preview === null) return "Delete a record";
  if (preview.TargetType === "quotation") {
    return `Delete quotation ${preview.DocumentNumber}`;
  }
  return `Delete purchase order ${preview.PONumber || `#${preview.TargetID}`}`;
}
