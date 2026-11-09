import { parseDraftPayload, type DraftPayload } from "./drafts";

/**
 * Issue #537: the local recovery snapshot.
 *
 * The envelope is deliberately the frontend's own shape. Go stores it as an
 * opaque string under the user's config directory; nothing about it enters the
 * API contract, the database or a signed revision. That is what lets the
 * draft payload keep evolving without a server migration.
 */
export const RECOVERY_VERSION = 1;

/** How long the editor must stay quiet before a snapshot is written. Long
 *  enough not to hit the disk on every keystroke, short enough that a crash
 *  loses only the last moment of typing. */
export const RECOVERY_DEBOUNCE_MS = 1500;

export interface RecoverySnapshot {
  version: 1;
  /** ISO-8601 time the snapshot was taken; the banner's label is a hint. */
  savedAt: string;
  /** The same `{ items, details, customerId }` a draft stores. */
  draft: DraftPayload;
}

export function buildRecoverySnapshot(
  draft: DraftPayload,
  savedAt: string
): RecoverySnapshot {
  return { version: RECOVERY_VERSION, savedAt, draft };
}

/**
 * Parses a stored snapshot, tolerating an older draft shape and refusing
 * anything it cannot safely apply.
 *
 * A wrong/absent version is refused: a future envelope may mean something
 * different, and guessing is how a restore overwrites work with nonsense. The
 * draft itself delegates to `parseDraftPayload`, so a snapshot written before
 * the quotation note existed opens with an empty note rather than failing.
 */
export function parseRecoverySnapshot(raw: string): RecoverySnapshot | null {
  let decoded: unknown;
  try {
    decoded = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof decoded !== "object" || decoded === null || Array.isArray(decoded)) {
    return null;
  }
  const record = decoded as Record<string, unknown>;
  if (record.version !== RECOVERY_VERSION) return null;
  if (typeof record.savedAt !== "string" || record.savedAt.trim() === "") {
    return null;
  }
  if (
    typeof record.draft !== "object" ||
    record.draft === null ||
    Array.isArray(record.draft)
  ) {
    return null;
  }
  const draft = parseDraftPayload(JSON.stringify(record.draft));
  if (draft === null) return null;
  return {
    version: RECOVERY_VERSION,
    savedAt: record.savedAt,
    draft,
  };
}

/** "2:14 PM" — the time the banner names. A hint only: an unreadable stamp
 *  still offers the restore, it just cannot say when. Formatted by hand rather
 *  than through the locale, so the label is the same string in every browser
 *  and on every Node the tests run under. */
export function recoveryLabel(snapshot: RecoverySnapshot): string {
  const savedAt = new Date(snapshot.savedAt);
  if (Number.isNaN(savedAt.getTime())) return "earlier";
  const hours = savedAt.getHours();
  const minutes = String(savedAt.getMinutes()).padStart(2, "0");
  const hour12 = hours % 12 === 0 ? 12 : hours % 12;
  return `${hour12}:${minutes} ${hours < 12 ? "AM" : "PM"}`;
}
