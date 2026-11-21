// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ArchivePage } from "./ArchivePage";
import { ToastProvider } from "../toast";

// Issue #645: a confirmed quotation deletion can change report evidence. A
// whole-chain deletion removes the linked purchase order; a record deletion
// leaves it as Source missing, which changes its cost and margin. The page must
// tell its caller, and only after the server actually confirmed the deletion --
// never on cancel, on a failed confirmation, or while the dialog is open.
//
// The list, preview and confirm bindings are replaced at their module boundary,
// which is what the desktop provides; the page and the real DeletionDialog above
// them are production code.

const { listMock, previewMock, challengeMock, confirmMock, catalogueMock } =
  vi.hoisted(() => ({
    listMock: vi.fn(),
    previewMock: vi.fn(),
    challengeMock: vi.fn(),
    confirmMock: vi.fn(),
    catalogueMock: vi.fn(),
  }));

vi.mock("@backend/backend/Quotation", () => ({
  ListQuotations: (...args: unknown[]) => listMock(...args),
  PreviewDeletion: (...args: unknown[]) => previewMock(...args),
  CreateDeletionChallenge: (...args: unknown[]) => challengeMock(...args),
  ConfirmDeletion: (...args: unknown[]) => confirmMock(...args),
}));

vi.mock("@backend/backend/AdminAPI", () => ({
  GetCurrencyCatalogue: (...args: unknown[]) => catalogueMock(...args),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  listMock.mockReset();
  previewMock.mockReset();
  challengeMock.mockReset();
  confirmMock.mockReset();
  catalogueMock.mockReset();
  listMock.mockResolvedValue([quotationRow()]);
  catalogueMock.mockResolvedValue([]);
  previewMock.mockResolvedValue(deletionPreview());
  confirmMock.mockResolvedValue({
    Outcome: {
      Removed: [deletionRecord({ Type: "quotation" })],
      AuditIDs: [1],
    },
  });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

function quotationRow(overrides: Record<string, unknown> = {}) {
  return {
    ID: 12,
    QuoteNumber: "1502260012",
    RevisionNumber: 1,
    CustomerName: "PT. Buyer",
    Subject: "AC maintenance",
    Currency: "IDR",
    CreatedAt: "2026-10-02T10:00:00Z",
    GrandTotal: 1500000,
    DisplayGrand: "1.500.000",
    ...overrides,
  };
}

function deletionRecord(overrides: Record<string, unknown> = {}) {
  return {
    Type: "quotation",
    ID: 12,
    UUID: "uuid-12",
    DocumentNumber: "1502260012",
    SourceReference: "PO-645",
    Label: "Quotation 1502260012",
    State: "saved",
    Version: 2,
    Revisions: [0, 1],
    DocumentVersions: 0,
    HistoryEntries: 0,
    ...overrides,
  };
}

function deletionPreview(overrides: Record<string, unknown> = {}) {
  return {
    TargetType: "quotation",
    TargetID: 12,
    TargetUUID: "uuid-12",
    DocumentNumber: "1502260012",
    DocumentNumberLabel: "Quotation number",
    QuoteNumber: "1502260012",
    PONumber: "PO-645",
    CustomerName: "PT. Buyer",
    State: "saved",
    SourceMissing: false,
    Version: 2,
    Fingerprint: "v1;target=quotation;quotation_id=12",
    Scopes: [
      {
        Scope: "record",
        Available: true,
        UnavailableReason: "",
        RequiresChallenge: false,
        Removes: [deletionRecord()],
        Preserves: [
          deletionRecord({
            Type: "purchase_order",
            ID: 7,
            Label: "Purchase order PO-645",
          }),
        ],
      },
      {
        Scope: "chain",
        Available: true,
        UnavailableReason: "",
        RequiresChallenge: true,
        Removes: [
          deletionRecord(),
          deletionRecord({
            Type: "purchase_order",
            ID: 7,
            DocumentNumber: "PO-645",
            Label: "Purchase order PO-645",
          }),
        ],
        Preserves: [],
      },
    ],
    ...overrides,
  };
}

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function button(label: string): HTMLButtonElement {
  const node = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (node === null) throw new Error(`no button labelled ${label}`);
  return node;
}

function buttonWithText(text: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll("button")).find(
    (candidate) => candidate.textContent?.trim() === text
  );
  if (found === undefined) throw new Error(`no button reading ${text}`);
  return found;
}

function click(element: Element): void {
  act(() => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

function typeField(label: string, value: string) {
  const field = document.querySelector<
    HTMLInputElement | HTMLTextAreaElement
  >(`input[aria-label="${label}"], textarea[aria-label="${label}"]`);
  if (field === null) throw new Error(`no field labelled ${label}`);
  const prototype =
    field instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  act(() => {
    Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(field, value);
    field.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function renderArchive(onDeleted?: () => void) {
  await act(async () => {
    root.render(
      <ToastProvider>
        <ArchivePage
          onNewQuotation={() => undefined}
          onOpenQuotation={() => undefined}
          isAdmin
          onDeleted={onDeleted}
        />
      </ToastProvider>
    );
  });
  await flush();
}

async function openDeletionDialog() {
  click(button("Delete quotation 1502260012"));
  await flush();
}

async function confirmRecordDeletion() {
  typeField("Deletion reason", "duplicate entry");
  typeField("Type 1502260012 to confirm", "1502260012");
  click(button("Delete permanently"));
  await flush();
}

describe("quotation deletion success callback (issue #645)", () => {
  it("fires once after a confirmed record deletion", async () => {
    const onDeleted = vi.fn();
    await renderArchive(onDeleted);

    await openDeletionDialog();
    expect(onDeleted).not.toHaveBeenCalled();

    await confirmRecordDeletion();
    expect(confirmMock).toHaveBeenCalledTimes(1);
    expect(onDeleted).toHaveBeenCalledTimes(1);
  });

  it("fires once after a confirmed whole-chain deletion", async () => {
    challengeMock.mockResolvedValue({
      Challenge: {
        ChallengeID: "challenge-1",
        Letters: "ABCDEFGHJK",
        ExpiresAt: "2026-10-21T10:15:00Z",
        Fingerprint: "v1;target=quotation;quotation_id=12",
      },
    });
    confirmMock.mockResolvedValue({
      Outcome: {
        Removed: [
          deletionRecord({ Type: "quotation" }),
          deletionRecord({ Type: "purchase_order", ID: 7 }),
        ],
        AuditIDs: [1, 2],
      },
    });
    const onDeleted = vi.fn();
    await renderArchive(onDeleted);

    await openDeletionDialog();
    const chain = document.querySelector<HTMLInputElement>(
      'input[name="deletion-scope"][value="chain"]'
    );
    if (chain === null) throw new Error("no chain scope control");
    act(() => chain.click());

    const generate = Array.from(document.querySelectorAll("button")).find(
      (candidate) =>
        candidate.textContent?.includes("Generate the ten-letter challenge")
    );
    if (generate === undefined) throw new Error("no challenge button");
    act(() => generate.click());
    await flush();
    expect(challengeMock).toHaveBeenCalledTimes(1);

    typeField("Deletion reason", "whole chain entered by mistake");
    typeField("Type 1502260012 to confirm", "1502260012");
    typeField("Ten-letter challenge", "ABCDEFGHJK");
    click(button("Delete permanently"));
    await flush();

    expect(confirmMock.mock.calls[0][0]).toMatchObject({ Scope: "chain" });
    expect(onDeleted).toHaveBeenCalledTimes(1);
  });

  it("stays silent when the dialog is cancelled", async () => {
    const onDeleted = vi.fn();
    await renderArchive(onDeleted);

    await openDeletionDialog();
    act(() => buttonWithText("Cancel").click());
    await flush();

    expect(confirmMock).not.toHaveBeenCalled();
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("stays silent when the confirmation fails", async () => {
    confirmMock.mockRejectedValue(new Error("the quotation is locked"));
    const onDeleted = vi.fn();
    await renderArchive(onDeleted);

    await openDeletionDialog();
    await confirmRecordDeletion();

    expect(confirmMock).toHaveBeenCalledTimes(1);
    expect(onDeleted).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("the quotation is locked");
  });
});
