import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  deletionReducer,
  initialDeletionState,
  type DeletionPreviewDTO,
  type DeletionScopeDTO,
  type DeletionState,
} from "../deletion";
import { DeletionDialogView, type DeletionDialogViewProps } from "./DeletionDialog";

// Issue #480: the deletion dialog's visible states. The suite runs on node with
// no DOM (see vitest.config.ts), so the view is rendered to a string and
// asserted on as markup. It proves which scope options, connected records,
// confirmations and warnings can appear; clicks, focus and the three API calls
// are the DOM suite's job.

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
    Preserves: [
      record({
        Type: "quotation",
        ID: 4,
        UUID: "uuid-4",
        Label: "Quotation 1502260007",
        State: "issued",
        Version: 0,
        Revisions: [0, 1],
        DocumentVersions: 1,
        HistoryEntries: 0,
      }) as never,
    ],
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
        Removes: [
          record({
            Type: "quotation",
            ID: 4,
            Label: "Quotation 1502260007",
            State: "issued",
            Version: 0,
            Revisions: [0, 1],
            DocumentVersions: 1,
            HistoryEntries: 0,
          }),
          record(),
        ] as never,
        Preserves: [] as never[],
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

function render(state: DeletionState, overrides: Partial<DeletionDialogViewProps> = {}): string {
  const props: DeletionDialogViewProps = {
    state,
    targetLabel: "purchase order PO-480",
    onClose: () => undefined,
    onScope: () => undefined,
    onReason: () => undefined,
    onTypedNumber: () => undefined,
    onChallengeInput: () => undefined,
    onRequestChallenge: () => undefined,
    onChangeReviewed: () => undefined,
    onConfirm: () => undefined,
    ...overrides,
  };
  return renderToString(<DeletionDialogView {...props} />).replace(/<!-- -->/g, "");
}

function buttonTag(markup: string, label: string): string {
  const index = markup.indexOf(`aria-label="${label}"`);
  if (index < 0) return "";
  const start = markup.lastIndexOf("<button", index);
  const end = markup.indexOf(">", index);
  return markup.slice(start, end + 1);
}

describe("deletion dialog markup", () => {
  it("offers both scopes and lists the actual connected records", () => {
    const markup = render(loaded());
    expect(markup).toContain("Delete this record only");
    expect(markup).toContain("Delete the entire chain");
    expect(markup).toContain("Connected records");
    expect(markup).toContain("Purchase order PO-480");
    expect(markup).toContain("Quotation 1502260007");
    expect(markup).toContain("Stays");
    expect(markup).toContain("2 issued revisions (0, 1)");
  });

  it("names the document number that must be typed and keeps confirm disabled", () => {
    const markup = render(loaded());
    expect(markup).toContain("Type");
    expect(markup).toContain("PO-480");
    expect(markup).toContain('aria-label="Type PO-480 to confirm"');
    expect(buttonTag(markup, "Delete permanently")).toContain("disabled");
    expect(markup).toContain("retention schedule");
  });

  it("requires the ten-letter challenge on a chain deletion", () => {
    const chain = deletionReducer(loaded(), { type: "scopeChanged", scope: "chain" });
    const markup = render(chain);
    expect(markup).toContain("Ten-letter challenge");
    expect(markup).toContain("Generate the ten-letter challenge");
    expect(markup).toContain("excluding I, L and O");
    expect(markup).toContain("Reason, document number and the ten-letter challenge are required.");
    expect(buttonTag(markup, "Delete permanently")).toContain("disabled");
  });

  it("shows the issued challenge and its input", () => {
    let state = deletionReducer(loaded(), { type: "scopeChanged", scope: "chain" });
    state = deletionReducer(state, {
      type: "challengeLoaded",
      challenge: {
        ChallengeID: "challenge-1",
        Letters: "ABCDEFGHJK",
        ExpiresAt: "2026-10-21T10:15:00Z",
        Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
      },
    });
    const markup = render(state);
    expect(markup).toContain("ABCDEFGHJK");
    expect(markup).toContain('aria-label="Ten-letter challenge"');
    expect(markup).not.toContain("Generate the ten-letter challenge");
  });

  it("explains a changed chain without showing a confirmation", () => {
    const state = deletionReducer(loaded(), {
      type: "chainChanged",
      changed: {
        Preview: preview({ Fingerprint: "v1;target=purchase_order;purchase_order_id=7;v=4" }),
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
    const markup = render(state);
    expect(markup).toContain("The chain changed");
    expect(markup).toContain("Nothing was deleted");
    expect(markup).toContain("Purchase order version");
    expect(markup).toContain("3 → 4");
    expect(markup).toContain("Review the new chain");
    expect(buttonTag(markup, "Delete permanently")).toContain("disabled");
  });

  it("explains an unavailable chain scope and disables its control", () => {
    const state = deletionReducer(initialDeletionState, {
      type: "previewLoaded",
      preview: preview({
        SourceMissing: true,
        Scopes: [
          scope({ Preserves: [] as never[] }),
          scope({
            Scope: "chain",
            Available: false,
            UnavailableReason:
              "This purchase order already has no source quotation, so there is no chain to remove.",
          }),
        ],
      }),
    });
    const markup = render(state);
    expect(markup).toContain("Source missing");
    expect(markup).toContain("no source quotation");
    expect(markup).toContain("disabled");
  });

  it("shows a loading state before the preview arrives", () => {
    const markup = render(
      deletionReducer(initialDeletionState, { type: "previewLoading" }),
      { targetLabel: "quotation 1502260007" }
    );
    expect(markup).toContain("Loading quotation 1502260007");
    expect(buttonTag(markup, "Delete permanently")).toContain("disabled");
  });

  it("shows a surviving purchase order as Source missing when its quotation goes", () => {
    const state = deletionReducer(initialDeletionState, {
      type: "previewLoaded",
      preview: preview({
        TargetType: "quotation",
        TargetID: 4,
        TargetUUID: "uuid-4",
        DocumentNumber: "1502260007",
        DocumentNumberLabel: "Quotation number",
        State: "issued",
        Scopes: [
          scope({
            Removes: [record({ Type: "quotation", Label: "Quotation 1502260007" }) as never],
            Preserves: [record() as never],
          }),
          scope({ Scope: "chain", RequiresChallenge: true }),
        ],
      }),
    });
    const markup = render(state, { targetLabel: "quotation 1502260007" });
    expect(markup).toContain("Survives as Source missing");
    expect(markup).toContain("survives as Source missing");
  });
});
