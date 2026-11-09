// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DeletionPreviewDTO, DeletionScopeDTO } from "../deletion";
import { DeletionDialog } from "./DeletionDialog";

// Issue #480: the admin deletion flow in a real DOM.
//
// The SSR suite proves the markup; this suite drives the decisions the ticket is
// about: whole-chain deletion cannot be confirmed without the issued letters,
// the typed number and reason are sent exactly, a changed chain stops the
// deletion and forces both confirmations to be repeated against the fresh
// preview, and a dead challenge is cleared rather than silently retried.

const { previewMock, challengeMock, confirmMock } = vi.hoisted(() => ({
  previewMock: vi.fn(),
  challengeMock: vi.fn(),
  confirmMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  PreviewDeletion: (...args: unknown[]) => previewMock(...args),
  CreateDeletionChallenge: (...args: unknown[]) => challengeMock(...args),
  ConfirmDeletion: (...args: unknown[]) => confirmMock(...args),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

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
    Preserves: [record({ Type: "quotation", ID: 4, Label: "Quotation 1502260007" }) as never],
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
        Removes: [record({ Type: "quotation", ID: 4, Label: "Quotation 1502260007" }), record()] as never,
        Preserves: [] as never[],
      }),
    ],
    ...overrides,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  previewMock.mockReset();
  challengeMock.mockReset();
  confirmMock.mockReset();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function mount(onDeleted = vi.fn()) {
  await act(async () => {
    root.render(
      <DeletionDialog
        targetType="purchase_order"
        targetId={7}
        targetLabel="purchase order PO-480"
        onClose={() => undefined}
        onDeleted={onDeleted}
      />
    );
  });
  return onDeleted;
}

function inputByLabel(label: string): HTMLInputElement & HTMLTextAreaElement {
  const element = container.querySelector(`[aria-label="${label}"]`);
  if (element === null) throw new Error(`no control labelled ${label}`);
  return element as HTMLInputElement & HTMLTextAreaElement;
}

async function type(label: string, value: string) {
  const input = inputByLabel(label);
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(
      input instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype,
      "value"
    )?.set;
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function click(label: string) {
  const element = container.querySelector(`[aria-label="${label}"]`);
  if (element === null) throw new Error(`no control labelled ${label}`);
  await act(async () => {
    (element as HTMLElement).click();
  });
}

async function selectChain() {
  const radio = container.querySelector<HTMLInputElement>(
    'input[name="deletion-scope"][value="chain"]'
  );
  if (radio === null) throw new Error("no chain scope control");
  await act(async () => {
    radio.click();
  });
}

describe("DeletionDialog", () => {
  it("loads the preview and confirms a single record with the typed number", async () => {
    previewMock.mockResolvedValue(preview());
    confirmMock.mockResolvedValue({
      Outcome: { Removed: [record()], AuditIDs: [1] },
    });
    const onDeleted = await mount();

    expect(container.textContent).toContain("Purchase order PO-480");
    expect(
      container.querySelector<HTMLButtonElement>('[aria-label="Delete permanently"]')?.disabled
    ).toBe(true);

    await type("Deletion reason", "  duplicate entry  ");
    await type("Type PO-480 to confirm", " po-480 ");
    expect(confirmMock).not.toHaveBeenCalled();

    await click("Delete permanently");
    expect(confirmMock).toHaveBeenCalledTimes(1);
    expect(confirmMock.mock.calls[0][0]).toMatchObject({
      TargetType: "purchase_order",
      TargetID: 7,
      Scope: "record",
      Reason: "duplicate entry",
      TypedNumber: "po-480",
      ExpectedFingerprint: "v1;target=purchase_order;purchase_order_id=7",
      ChallengeID: "",
      ChallengeLetters: "",
    });
    expect(onDeleted).toHaveBeenCalledTimes(1);
  });

  it("requires the issued ten letters before a chain deletion can be confirmed", async () => {
    previewMock.mockResolvedValue(preview());
    challengeMock.mockResolvedValue({
      Challenge: {
        ChallengeID: "challenge-1",
        Letters: "ABCDEFGHJK",
        ExpiresAt: "2026-10-21T10:15:00Z",
        Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
      },
    });
    confirmMock.mockResolvedValue({
      Outcome: { Removed: [record(), record({ Type: "quotation", ID: 4 })], AuditIDs: [1, 2] },
    });
    await mount();

    await selectChain();
    const generate = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Generate the ten-letter challenge")
    );
    if (generate === undefined) throw new Error("no challenge button");
    await act(async () => generate.click());

    expect(challengeMock).toHaveBeenCalledWith({
      TargetType: "purchase_order",
      TargetID: 7,
      Scope: "chain",
      ExpectedFingerprint: "v1;target=purchase_order;purchase_order_id=7",
    });
    expect(container.textContent).toContain("ABCDEFGHJK");

    await type("Deletion reason", "entered by mistake");
    await type("Type PO-480 to confirm", "PO-480");
    expect(
      container.querySelector<HTMLButtonElement>('[aria-label="Delete permanently"]')?.disabled
    ).toBe(true);

    await type("Ten-letter challenge", "abcdefghjk");
    expect(inputByLabel("Ten-letter challenge").value).toBe("ABCDEFGHJK");
    expect(
      container.querySelector<HTMLButtonElement>('[aria-label="Delete permanently"]')?.disabled
    ).toBe(false);

    await click("Delete permanently");
    expect(confirmMock.mock.calls[0][0]).toMatchObject({
      Scope: "chain",
      ChallengeID: "challenge-1",
      ChallengeLetters: "ABCDEFGHJK",
    });
  });

  it("stops on a changed chain, explains it, and renews both confirmations", async () => {
    const fresh = preview({
      Version: 4,
      Fingerprint: "v1;target=purchase_order;purchase_order_id=7;v=4",
    });
    previewMock.mockResolvedValue(preview());
    confirmMock.mockResolvedValueOnce({
      Changed: {
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
    confirmMock.mockResolvedValueOnce({
      Outcome: { Removed: [record()], AuditIDs: [3] },
    });
    const onDeleted = await mount();

    await type("Deletion reason", "duplicate entry");
    await type("Type PO-480 to confirm", "PO-480");
    await click("Delete permanently");

    expect(container.textContent).toContain("The chain changed");
    expect(container.textContent).toContain("Purchase order version");
    expect(inputByLabel("Type PO-480 to confirm").value).toBe("");
    expect(
      container.querySelector<HTMLButtonElement>('[aria-label="Delete permanently"]')?.disabled
    ).toBe(true);
    expect(onDeleted).not.toHaveBeenCalled();

    const review = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Review the new chain")
    );
    if (review === undefined) throw new Error("no review button");
    await act(async () => review.click());

    await type("Type PO-480 to confirm", "PO-480");
    await click("Delete permanently");
    expect(confirmMock).toHaveBeenCalledTimes(2);
    expect(confirmMock.mock.calls[1][0]).toMatchObject({
      ExpectedFingerprint: "v1;target=purchase_order;purchase_order_id=7;v=4",
    });
    expect(onDeleted).toHaveBeenCalledTimes(1);
  });

  it("clears an expired challenge instead of retrying it", async () => {
    previewMock.mockResolvedValue(preview());
    challengeMock.mockResolvedValue({
      Challenge: {
        ChallengeID: "challenge-1",
        Letters: "ABCDEFGHJK",
        ExpiresAt: "2026-10-21T10:15:00Z",
        Fingerprint: "v1;target=purchase_order;purchase_order_id=7",
      },
    });
    confirmMock.mockResolvedValue({ ChallengeProblem: "expired" });
    await mount();

    await selectChain();
    const generate = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Generate the ten-letter challenge")
    );
    if (generate === undefined) throw new Error("no challenge button");
    await act(async () => generate.click());
    await type("Deletion reason", "entered by mistake");
    await type("Type PO-480 to confirm", "PO-480");
    await type("Ten-letter challenge", "ABCDEFGHJK");
    await click("Delete permanently");

    expect(container.textContent).toContain("expired");
    expect(container.textContent).toContain("Generate the ten-letter challenge");
    expect(container.querySelector('[aria-label="Ten-letter challenge"]')).toBeNull();
  });

  it("shows the server's 403 to a non-admin caller as an error, with nothing to delete", async () => {
    previewMock.mockRejectedValue(
      new Error("permanently deleting a record requires an administrator")
    );
    await mount();

    expect(container.textContent).toContain("requires an administrator");
    expect(container.querySelector('[aria-label="Delete permanently"]')).not.toBeNull();
    expect(container.textContent).not.toContain("Connected records");
  });
});
