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

/** Ticket #536: the "Stay signed in" window opens ten minutes before the
 *  session's idle deadline. The server owns the deadline; this is only the
 *  threshold the desktop offers renewal at. */
export const RENEW_WINDOW_MS = 10 * 60 * 1000;

/** True when the banner should offer a renewal: the session is live, the
 *  server says renewal can still extend it, and the idle deadline is within
 *  the window. A passed or unreadable deadline is not renewable — the poll
 *  owns the session-end path. */
export function renewalDue(
  expiresAt: string | null | undefined,
  now: number,
  canRenew: boolean
): boolean {
  if (!canRenew || !expiresAt) return false;
  const deadline = Date.parse(expiresAt);
  if (Number.isNaN(deadline)) return false;
  const remaining = deadline - now;
  return remaining > 0 && remaining <= RENEW_WINDOW_MS;
}

/** Whole minutes left before the idle deadline, rounded up so the banner
 *  never says "0 minutes" while time remains. Zero for a passed or
 *  unreadable deadline. */
export function minutesUntil(
  expiresAt: string | null | undefined,
  now: number
): number {
  if (!expiresAt) return 0;
  const deadline = Date.parse(expiresAt);
  if (Number.isNaN(deadline)) return 0;
  const remaining = deadline - now;
  if (remaining <= 0) return 0;
  return Math.ceil(remaining / 60000);
}

/** Re-login restores the preserved workspace only for the same account. A
 *  different user signing in on the same device starts clean. */
export function sameUser(
  previous: SignedInUser | null,
  next: SignedInUser
): boolean {
  return previous !== null && previous.ID === next.ID;
}
