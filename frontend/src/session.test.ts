import { describe, expect, it } from "vitest";
import { isSessionEnd, sameUser } from "./session";

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
