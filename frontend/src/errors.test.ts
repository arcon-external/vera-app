import { describe, expect, it } from "vitest";
import { errorMessage } from "./errors";

// Issue #531: Wails rejects with more than a bare string. The poll used to read
// only a string and treated every other shape as a sign-out. One helper now
// owns the shapes so no call site can silently fall back to a generic message.
describe("errorMessage", () => {
  it("reads a bare string", () => {
    expect(errorMessage("the server said no", "fallback")).toBe(
      "the server said no"
    );
  });

  it("reads an Error", () => {
    expect(errorMessage(new Error("reach the quotation server: EOF"), "fallback")).toBe(
      "reach the quotation server: EOF"
    );
  });

  it("reads the Wails Error object shape", () => {
    expect(
      errorMessage(
        { name: "Error", message: "invalid or expired session" },
        "fallback"
      )
    ).toBe("invalid or expired session");
  });

  it("reads a {code} refusal when the server sent no message", () => {
    expect(errorMessage({ code: "session_check_failed" }, "fallback")).toBe(
      "Session check failed"
    );
  });

  it("prefers a message over a code", () => {
    expect(
      errorMessage(
        { message: "The current password is incorrect.", code: "current_password_incorrect" },
        "fallback"
      )
    ).toBe("The current password is incorrect.");
  });

  it("keeps the fallback for an empty or unknown rejection", () => {
    expect(errorMessage("", "fallback")).toBe("fallback");
    expect(errorMessage("   ", "fallback")).toBe("fallback");
    expect(errorMessage(new Error(""), "fallback")).toBe("fallback");
    expect(errorMessage(undefined, "fallback")).toBe("fallback");
    expect(errorMessage({ code: "   " }, "fallback")).toBe("fallback");
    expect(errorMessage({ code: "unknown" }, "fallback")).toBe("fallback");
    expect(errorMessage(42, "fallback")).toBe("fallback");
  });
});
