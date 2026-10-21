import { describe, expect, it } from "vitest";
import {
  buildDeletionChallengeInput,
  formatChallengeExpiry,
  buildDeletionConfirmInput,
  challengeLettersAreValid,
  deletionConfirmErrors,
  deletionReducer,
  deletionScopeConsequences,
  emptyDeletionState,
  initialDeletionState,
  canConfirmDeletion,
  normalizeChallengeLetters,
  type DeletionPreviewDTO,
  type DeletionScopeDTO,
  type DeletionState,
} from "./deletion";

// Issue #480: the deletion dialog's decisions, with no DOM. These are the rules
// the ticket is about: scope, the typed number, the ten-letter challenge, the
// changed chain that restarts confirmation, and what the copy promises about
// backups.

function record(overrides: Record<string, unknown> = {}) {
  return {
    Type: "purchase_order",
    ID: 7,
    UUID: "uuid-7",
    DocumentNumber: "PO-480",
    SourceReference: "1502260007",
    Label: "Purchase order PO-480",
    State: "saved",
    Version: 3,
    Revisions: [] as number[],
    DocumentVersions: 1,
    HistoryEntries: 2,
    ...overrides,
  };
}

function scope(overrides: Partial<DeletionScopeDTO> = {}): DeletionScopeDTO {
  return {
    Scope: "record",
    Available: true,
    UnavailableReason: "",
    RequiresChallenge: false,
    Removes: [record() as never],
    Preserves: [] as never[],
    ...overrides,
  };
}

function preview(overrides: Partial<DeletionPreviewDTO> = {}): DeletionPreviewDTO {
  return {
    TargetType: "purchase_order",
    TargetID: 7,
    TargetUUID: "uuid-7",
    DocumentNumber: "PO-480",
    DocumentNumberLabel: "Customer PO number",
    QuoteNumber: "1502260007",
    PONumber: "PO-480",
    CustomerName: "PT. Deletion",
    State: "saved",
    SourceMissing: false,
    Version: 3,
    Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
    Scopes: [
      scope(),
      scope({
        Scope: "chain",
        RequiresChallenge: true,
        Removes: [record({ Type: "quotation", Label: "Quotation 1502260007" }), record()] as never,
      }),
    ],
    ...overrides,
  };
}

function loaded(overrides: Partial<DeletionState> = {}): DeletionState {
  return {
    ...deletionReducer(initialDeletionState, {
      type: "previewLoaded",
      preview: preview(),
    }),
    ...overrides,
  };
}

describe("deletion scope", () => {
  it("starts on the single record and lists the actual connected documents", () => {
    const state = deletionReducer(initialDeletionState, {
      type: "previewLoaded",
      preview: preview(),
    });
    expect(state.phase).toBe("ready");
    expect(state.scope).toBe("record");
    const chain = state.preview?.Scopes.find((plan) => plan.Scope === "chain");
    expect(chain?.Removes.map((row) => row.Label)).toEqual([
      "Quotation 1502260007",
      "Purchase order PO-480",
    ]);
  });

  it("explains an unavailable chain scope instead of hiding it", () => {
    const state = deletionReducer(initialDeletionState, {
      type: "previewLoaded",
      preview: preview({
        Scopes: [
          scope(),
          scope({
            Scope: "chain",
            Available: false,
            UnavailableReason: "This purchase order already has no source quotation.",
          }),
        ],
      }),
    });
    const consequences = deletionScopeConsequences(state);
    expect(consequences.join(" ")).toContain("no source quotation");
  });

  it("falling back to record when a chain scope becomes unavailable", () => {
    const chainState = deletionReducer(loaded(), { type: "scopeChanged", scope: "chain" });
    expect(chainState.scope).toBe("chain");
    const reduced = deletionReducer(chainState, {
      type: "previewLoaded",
      preview: preview({
        Scopes: [scope(), scope({ Scope: "chain", Available: false, UnavailableReason: "gone" })],
      }),
    });
    expect(reduced.scope).toBe("record");
  });
});

describe("the typed confirmation", () => {
  it("requires a reason and the exact document number, case-insensitively", () => {
    let state = loaded();
    expect(deletionConfirmErrors(state).reason).toBeDefined();
    expect(deletionConfirmErrors(state).typedNumber).toBeDefined();

    state = deletionReducer(state, { type: "reasonChanged", value: "  duplicate  " });
    state = deletionReducer(state, { type: "typedNumberChanged", value: " po-480 " });
    expect(deletionConfirmErrors(state)).toEqual({});
    expect(canConfirmDeletion(state)).toBe(true);
    expect(buildDeletionConfirmInput(state)).toMatchObject({
      TargetType: "purchase_order",
      TargetID: 7,
      Scope: "record",
      Reason: "duplicate",
      TypedNumber: "po-480",
      ExpectedFingerprint: "v1;target=purchase_order;purchase_order_id=7",
      ChallengeID: "",
      ChallengeLetters: "",
    });
  });

  it("refuses a number for a different record", () => {
    const state = deletionReducer(loaded(), {
      type: "typedNumberChanged",
      value: "1502260007",
    });
    expect(deletionConfirmErrors(state).typedNumber).toContain("PO-480");
    expect(canConfirmDeletion(state)).toBe(false);
  });
});

describe("the ten-letter challenge", () => {
  it("accepts exactly ten letters from A-Z minus I, L and O, case-insensitively", () => {
    expect(challengeLettersAreValid("ABCDEFGHJK")).toBe(true);
    expect(challengeLettersAreValid("abcdefghjk")).toBe(true);
    expect(challengeLettersAreValid("ABCDEFGHIJ")).toBe(false);
    expect(challengeLettersAreValid("ABCDEFGHJL")).toBe(false);
    expect(challengeLettersAreValid("ABCDEFGHJO")).toBe(false);
    expect(challengeLettersAreValid("ABCDEFGHJ")).toBe(false);
    expect(challengeLettersAreValid("ABCDEFGH1K")).toBe(false);
    expect(normalizeChallengeLetters(" ab cdef ghjk ")).toBe("ABCDEFGHJK");
  });

  it("blocks a chain confirmation until the issued letters are entered", () => {
    let state = deletionReducer(loaded(), { type: "scopeChanged", scope: "chain" });
    state = deletionReducer(state, { type: "reasonChanged", value: "entered by mistake" });
    state = deletionReducer(state, { type: "typedNumberChanged", value: "PO-480" });
    expect(deletionConfirmErrors(state).challenge).toContain("ten-letter");
    expect(canConfirmDeletion(state)).toBe(false);

    state = deletionReducer(state, {
      type: "challengeLoaded",
      challenge: {
        ChallengeID: "challenge-1",
        Letters: "ABCDEFGHJK",
        ExpiresAt: "2026-10-21T10:15:00Z",
        Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
      },
    });
    state = deletionReducer(state, { type: "challengeInputChanged", value: "abcdefghjk" });
    expect(deletionConfirmErrors(state)).toEqual({});
    expect(canConfirmDeletion(state)).toBe(true);
    expect(buildDeletionConfirmInput(state)).toMatchObject({
      Scope: "chain",
      ChallengeID: "challenge-1",
      ChallengeLetters: "abcdefghjk",
    });
  });

  it("binds the challenge request to the preview fingerprint", () => {
    const state = deletionReducer(loaded(), { type: "scopeChanged", scope: "chain" });
    expect(buildDeletionChallengeInput(state)).toEqual({
      TargetType: "purchase_order",
      TargetID: 7,
      Scope: "chain",
      ExpectedFingerprint: "v1;target=purchase_order;purchase_order_id=7",
    });
  });

  it("never asks for a challenge on a single-record deletion", () => {
    const state = deletionReducer(loaded(), { type: "reasonChanged", value: "cleanup" });
    expect(buildDeletionChallengeInput(state)).toBeNull();
  });

  it("clears a dead challenge and says which problem it was", () => {
    let state = deletionReducer(
      deletionReducer(loaded(), { type: "scopeChanged", scope: "chain" }),
      {
        type: "challengeLoaded",
        challenge: {
          ChallengeID: "challenge-1",
          Letters: "ABCDEFGHJK",
          ExpiresAt: "2026-10-21T10:15:00Z",
          Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
        },
      }
    );
    state = deletionReducer(state, { type: "challengeInputChanged", value: "ABCDEFGHJK" });
    state = deletionReducer(state, { type: "challengeProblem", problem: "expired" });
    expect(state.challenge).toBeNull();
    expect(state.challengeInput).toBe("");
    expect(state.error).toContain("expired");
  });
});

describe("a changed chain", () => {
  it("adopts the fresh preview, lists the changes and renews both confirmations", () => {
    let state = loaded();
    state = deletionReducer(state, { type: "scopeChanged", scope: "chain" });
    state = deletionReducer(state, {
      type: "challengeLoaded",
      challenge: {
        ChallengeID: "old",
        Letters: "ABCDEFGHJK",
        ExpiresAt: "2026-10-21T10:15:00Z",
        Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
      },
    });
    state = deletionReducer(state, { type: "typedNumberChanged", value: "PO-480" });
    state = deletionReducer(state, { type: "reasonChanged", value: "entered by mistake" });

    const fresh = preview({
      Version: 4,
      Fingerprint: "v1;target=purchase_order;purchase_order_id=7;version=4",
    });
    state = deletionReducer(state, {
      type: "chainChanged",
      changed: {
        Preview: fresh,
        Changes: [
          {
            Field: "purchase_order_version",
            Label: "Purchase order version",
            Before: "3",
            After: "4",
          },
        ],
      },
    });
    expect(state.phase).toBe("ready");
    expect(state.preview?.Fingerprint).toBe(fresh.Fingerprint);
    expect(state.changes.map((change) => change.Label)).toEqual(["Purchase order version"]);
    expect(state.challenge).toBeNull();
    expect(state.challengeInput).toBe("");
    // The typed number is a confirmation of the record that was shown; it must
    // be typed again against the new chain.
    expect(state.typedNumber).toBe("");
    expect(state.reason).toBe("entered by mistake");
    expect(state.scope).toBe("chain");
    expect(canConfirmDeletion(state)).toBe(false);

    state = deletionReducer(state, { type: "changesReviewed" });
    expect(state.changes).toEqual([]);
  });

  it("falls back to a single record when the changed chain has no chain scope", () => {
    const state = deletionReducer(loaded(), {
      type: "chainChanged",
      changed: {
        Preview: preview({
          Scopes: [
            scope(),
            scope({ Scope: "chain", Available: false, UnavailableReason: "the source is gone" }),
          ],
        }),
        Changes: [],
      },
    });
    expect(state.scope).toBe("record");
  });
});

describe("deletion copy", () => {
  it("states what is removed and that backups expire on the retention schedule", () => {
    const state = loaded();
    const consequences = deletionScopeConsequences(state).join(" ");
    expect(consequences).toContain("attachments");
    expect(consequences).toContain("history");
    expect(consequences).toContain("cannot be undone");
    expect(deletionReducer(state, { type: "previewLoaded", preview: preview() }).backupNotice)
      .toContain("retention schedule");
  });
});

describe("challenge expiry copy", () => {
  it("shows the expiry in the company's time zone, not raw RFC3339", () => {
    expect(formatChallengeExpiry("2026-10-21T10:15:00Z")).toBe("21 Oct 2026, 17:15");
    expect(formatChallengeExpiry("not-a-date")).toBe("not-a-date");
  });
});

describe("the empty state", () => {
  it("starts idle and cannot confirm anything", () => {
    expect(emptyDeletionState.phase).toBe("idle");
    expect(canConfirmDeletion(emptyDeletionState)).toBe(false);
    expect(deletionConfirmErrors(emptyDeletionState).reason).toBeDefined();
  });
});
