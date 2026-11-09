import { LoadPOReconciliation, SavePOReconciliation } from "@backend/backend/Quotation";
import type { backend } from "@backend/models";
import { errorMessage } from "./errors";
import {
  normalizeSession,
  type ReconciliationSession,
} from "./poReconciliation";

// Ticket 04: the Wails seam for the reconciliation session. Component tests
// drive a fake transport through this adapter; production passes the Wails
// bindings. The transport takes and returns the production session shape; the
// one cast below is where the generated class meets plain JSON, which drops
// class methods on the wire anyway.

/** The backend the pane talks to: load returns the stored session or refuses. */
export interface ReconciliationTransport {
  load(poID: string): Promise<ReconciliationSession>;
  save(
    poID: string,
    session: ReconciliationSession
  ): Promise<ReconciliationSession>;
}

export const wailsReconciliationTransport: ReconciliationTransport = {
  load: (poID: string) =>
    LoadPOReconciliation(poID).then((session) =>
      normalizeSession(session as ReconciliationSession)
    ),
  save: (poID: string, session: ReconciliationSession) =>
    SavePOReconciliation(
      poID,
      session as unknown as backend.POReconciliationSession
    ).then((updated) => normalizeSession(updated as ReconciliationSession)),
};

/**
 * The desktop binding surfaces a refusal as a message string, so absence is
 * matched by the server's sentence. Anything else stays an error.
 */
export function isReconciliationNotFound(error: unknown): boolean {
  if (error === null || error === undefined) return false;
  return /no reconciliation recorded/i.test(errorMessage(error, "The reconciliation could not be loaded."));
}

export type LoadSessionResult =
  | { status: "ready"; session: ReconciliationSession }
  | { status: "absent" }
  | { status: "error"; message: string };

/** A missing session is absent (fresh local shell), never a failure. */
export async function loadSession(
  transport: ReconciliationTransport,
  poID: string
): Promise<LoadSessionResult> {
  try {
    const session = await transport.load(poID);
    return { status: "ready", session: normalizeSession(session) };
  } catch (error: unknown) {
    if (isReconciliationNotFound(error)) return { status: "absent" };
    return { status: "error", message: errorMessage(error, "The reconciliation could not be loaded.") };
  }
}

export type SaveSessionResult =
  | { status: "saved"; session: ReconciliationSession }
  | { status: "error"; message: string };

export async function saveSession(
  transport: ReconciliationTransport,
  poID: string,
  session: ReconciliationSession
): Promise<SaveSessionResult> {
  try {
    const updated = await transport.save(poID, session);
    return { status: "saved", session: normalizeSession(updated) };
  } catch (error: unknown) {
    return { status: "error", message: errorMessage(error, "The reconciliation could not be loaded.") };
  }
}
