import type { SignedInUser } from "./components/LoginPage";

/** The typed outcomes of the desktop session check (issue #531). The Go
 *  binding returns one of these as data; it never rejects for an expected
 *  outcome, so the poll never has to parse an error message. */
export type SessionState =
  | "active"
  | "expired"
  | "revoked"
  | "inactive"
  | "invalid"
  | "unreachable"
  | "server_error"
  | "rate_limited"
  | "incompatible";

export interface SessionStatus {
  State: SessionState | string;
  User?: SignedInUser;
  Code: string;
  Message: string;
  ExpiresAt: string;
  CanRenew: boolean;
}

/** Only these four outcomes end the session. A transport failure, a 5xx, a
 *  429 or an unreadable response is a connection state: the laptop is offline
 *  or the server is unwell, not signed out. */
const SESSION_END_STATES: ReadonlySet<string> = new Set([
  "expired",
  "revoked",
  "inactive",
  "invalid",
]);

export function isSessionEnd(state: string): boolean {
  return SESSION_END_STATES.has(state);
}

/** Re-login restores the preserved workspace only for the same account. A
 *  different user signing in on the same device starts clean. */
export function sameUser(
  previous: SignedInUser | null,
  next: SignedInUser
): boolean {
  return previous !== null && previous.ID === next.ID;
}
