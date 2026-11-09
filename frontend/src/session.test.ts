import { describe, expect, it } from "vitest";
import { RENEW_WINDOW_MS, isSessionEnd, minutesUntil, renewalDue, sameUser } from "./session";

// Issue #531: only a real session end may sign the desktop out. Transport,
// server and rate-limit failures are connection states, not session states.
describe("isSessionEnd", () => {
  it("ends the session for expired, revoked, inactive and invalid", () => {
    for (const state of ["expired", "revoked", "inactive", "invalid"]) {
      expect(isSessionEnd(state), state).toBe(true);
    }
  });

  it("keeps the session for every operational outcome", () => {
    for (const state of [
      "unreachable",
      "server_error",
      "rate_limited",
      "incompatible",
      "active",
    ]) {
      expect(isSessionEnd(state), state).toBe(false);
    }
  });

  it("keeps the session for an unknown future status", () => {
    expect(isSessionEnd("something_new")).toBe(false);
  });
});

describe("sameUser", () => {
  const ada = { ID: 1, Email: "ada@example.com", DisplayName: "Ada", Role: "admin" };
  const grace = { ID: 2, Email: "grace@example.com", DisplayName: "Grace", Role: "staff" };

  it("restores only for the same account", () => {
    expect(sameUser(ada, ada)).toBe(true);
    expect(sameUser(ada, grace)).toBe(false);
  });

  it("treats a fresh sign-in with no ended session as different", () => {
    expect(sameUser(null, ada)).toBe(false);
  });
});

// Ticket #536: the "Stay signed in" window. The banner is offered only while
// the session is live and the server says renewal is possible, and only in
// the last ten minutes before the idle deadline.
describe("renewalDue", () => {
  const now = Date.parse("2026-11-09T12:00:00Z");
  const at = (offsetMs: number) => new Date(now + offsetMs).toISOString();

  it("opens exactly ten minutes before the idle deadline", () => {
    expect(RENEW_WINDOW_MS).toBe(10 * 60 * 1000);
    expect(renewalDue(at(RENEW_WINDOW_MS), now, true)).toBe(true);
    expect(renewalDue(at(RENEW_WINDOW_MS + 1), now, true)).toBe(false);
  });

  it("stays open inside the window", () => {
    expect(renewalDue(at(60_000), now, true)).toBe(true);
    expect(renewalDue(at(30_000), now, true)).toBe(true);
  });

  it("closes once the deadline has passed", () => {
    expect(renewalDue(at(0), now, true)).toBe(false);
    expect(renewalDue(at(-60_000), now, true)).toBe(false);
  });

  it("stays closed when the server says renewal is impossible", () => {
    expect(renewalDue(at(60_000), now, false)).toBe(false);
  });

  it("stays closed for a missing or unreadable deadline", () => {
    expect(renewalDue(null, now, true)).toBe(false);
    expect(renewalDue(undefined, now, true)).toBe(false);
    expect(renewalDue("", now, true)).toBe(false);
    expect(renewalDue("not-a-date", now, true)).toBe(false);
  });
});

describe("minutesUntil", () => {
  const now = Date.parse("2026-11-09T12:00:00Z");

  it("rounds up so the copy never says zero while time remains", () => {
    expect(minutesUntil(new Date(now + 30_000).toISOString(), now)).toBe(1);
    expect(minutesUntil(new Date(now + 5 * 60_000).toISOString(), now)).toBe(5);
    expect(minutesUntil(new Date(now + 5 * 60_000 + 1).toISOString(), now)).toBe(6);
  });

  it("reports zero for a passed or unreadable deadline", () => {
    expect(minutesUntil(new Date(now).toISOString(), now)).toBe(0);
    expect(minutesUntil("", now)).toBe(0);
    expect(minutesUntil("not-a-date", now)).toBe(0);
  });
});
