// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import { usePurchaseOrderLeaveGuard, type AppPage } from "../poLeaveGuard";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import type { PurchaseOrderDocumentStateDTO } from "../purchaseOrderDocument";
import { AppShell } from "./AppShell";
import {
  PurchaseOrderDiscardPrompt,
  PurchaseOrderEditorPage,
} from "./PurchaseOrderEditor";

// Issue #468, wave 2: the tab-navigation loss the Judge reproduced. The editor
// owns its reducer state, so leaving the PO page unmounts it; the guard must
// ask before that happens, and Keep editing must leave the typed value alive.
//
// Wave 3: the Judge found two dialogs and no modal behaviour. Cancel used to
// open the editor's inline panel while a tab opened App's overlay, so both
// could stand at once. There is now one pending discard decision and one
// dialog; this suite holds that line.
//
// This is the runtime regression the SSR suite cannot reach. The harness is
// App's own composition: AppShell + the editor + the shared leave-guard hook
// and the shared discard prompt. App supplies only the one-line wiring this
// harness copies.

const { extractionMock, startExtractionMock, documentStateMock, attachMock } = vi.hoisted(() => ({
  extractionMock: vi.fn(),
  startExtractionMock: vi.fn(),
  documentStateMock: vi.fn(),
  attachMock: vi.fn(),
}));

// The editor page now mounts the document panel, which reads the one optional
// customer PO PDF through the desktop binding (#472), and the extraction
// review, which reads the latest run (#477). This suite is about tab
// navigation, so both reads default to their empty state; individual tests
// override the extraction answer where they exercise it.
vi.mock("@backend/backend/Quotation", () => ({
  GetPurchaseOrderDocumentState: (...args: unknown[]) => documentStateMock(...args),
  AttachPurchaseOrderDocument: (...args: unknown[]) => attachMock(...args),
  GetActivePurchaseOrderDocumentContent: vi.fn(),
  RemovePurchaseOrderDocument: vi.fn(),
  GetPurchaseOrderExtraction: (...args: unknown[]) => extractionMock(...args),
  StartPurchaseOrderExtraction: (...args: unknown[]) => startExtractionMock(...args),
  UpdatePurchaseOrderDraft: vi.fn(),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const ORDER: PurchaseOrderDTO = {
  PODate: null,
  RequestedDeliveryDate: null,
  UUID: "uuid-1",
  State: "draft",
  QuoteNumber: "1502260012",
  CustomerName: "PT. Buyer",
  BillingAddress: "Jl. Billing",
  DeliveryAddress: "Jl. Delivery",
  ShipToCode: "SHIP-1",
  ContactEmail: "buyer@example.com",
  ContactPhone: "+62 21 555",
  PONumber: "PO-1",
  ReceivedDate: "2026-10-20",
  Version: 1,
  Incoterms: "CIF",
  Currency: "USD",
  TaxZone: "NonFTZ",
  TaxCategory: "SupplyInstall",
  CreatedAt: "2026-10-20T10:00:00Z",
  UpdatedAt: "2026-10-20T10:00:00Z",
  Items: [
    {
      QuotationItemNo: 1,
      QuotationSubNo: null,
      LineNumber: 1,
      Description: "Widget",
      QuantityScaled: 25000,
      UoM: "Pc",
      UnitPriceMinor: 125050,
      DiscountEnabled: false,
      DiscountPercent: 0,
      GrossMinor: 312625,
      DiscountMinor: 0,
      NetMinor: 312625,
    },
  ],
  ID: 7,
  QuotationID: 12,
  SourceRevision: 1,
  CustomerID: 2,
  PaymentTerms: "45 days",
  AdjustmentReason: "",
  DiscountPercent: 0,
  PpnBps: 0,
  SubtotalMinor: 312625,
  DiscountMinor: 0,
  PpnMinor: 0,
  AdjustmentMinor: 0,
  GrandTotalMinor: 312625,
  PrintedSubtotalMinor: 312625,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 312625,
};

const INCOTERMS = [{ value: "CIF", label: "CIF" }];
const UOMS = [{ value: "Pc", label: "Pc" }];

function Workspace() {
  const [page, setPage] = useState<AppPage>("po");
  const leave = usePurchaseOrderLeaveGuard(page, setPage);
  const open = leave.pendingPage !== null;
  return (
    <>
      <AppShell
        page={page}
        inert={open}
        onOpenHome={() => leave.request("home")}
        onOpenEditor={() => leave.request("create")}
        onOpenDrafts={() => leave.request("drafts")}
        onOpenArchive={() => leave.request("archive")}
        companyName="PT Arcon Penta Persada"
        user={null}
      >
        {page === "po" ? (
          <PurchaseOrderEditorPage
            order={ORDER}
            currency={USD}
            uomOptions={UOMS}
            incotermOptions={INCOTERMS}
            onSaved={() => {}}
            onCancel={() => leave.request("home", () => setPage("home"))}
            onDirtyChange={leave.setDirty}
          />
        ) : (
          <p data-testid={`page-${page}`}>Destination {page}</p>
        )}
      </AppShell>
      {open ? (
        <PurchaseOrderDiscardPrompt
          onKeepEditing={leave.keepEditing}
          onDiscard={leave.discard}
          returnFocus={leave.pendingReturnFocus}
        />
      ) : null}
    </>
  );
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  extractionMock.mockReset();
  extractionMock.mockResolvedValue({ Run: null, Document: null });
  startExtractionMock.mockReset();
  attachMock.mockReset();
  documentStateMock.mockReset();
  documentStateMock.mockResolvedValue(extractionDocumentState());
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

function tab(label: string): HTMLButtonElement {
  const nav = document.querySelector<HTMLElement>('nav[aria-label="Primary"]');
  const found = Array.from(nav?.querySelectorAll<HTMLButtonElement>("button") ?? []).find(
    (button) => button.textContent?.trim() === label
  );
  if (!found) throw new Error(`tab ${label} not found`);
  return found;
}

function click(element: HTMLElement): void {
  act(() => element.click());
}

function typeInto(label: string, value: string): void {
  const input = document.querySelector<HTMLInputElement>(
    `input[aria-label="${label}"]`
  );
  if (!input) throw new Error(`input ${label} not found`);
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value"
  )?.set;
  act(() => {
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function prompt(): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    '[role="alertdialog"][aria-label="Discard unsaved changes?"]'
  );
}

async function flush(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function extractionCell(value: string, status: string, page: number | null = 1) {
  return {
    Value: value,
    Raw: value,
    Status: status,
    Page: page,
    BBox: page === null ? null : { X: 86, Y: 82, Width: 56, Height: 9 },
    Confidence: null,
    Note: "",
  };
}

function extractionState() {
  return {
    Run: {
      UUID: "run-1",
      Status: "succeeded",
      Engine: "poppler",
      EngineVersion: "25.03.0",
      DocumentSHA256: "doc-sha",
      ErrorCode: null,
      ErrorMessage: null,
      Result: {
        Engine: "poppler",
        EngineVersion: "25.03.0",
        PageCount: 1,
        PagesWithText: 1,
        PagesWithoutText: 0,
        DurationMs: 20,
        SchemaVersion: 1,
        Pages: [{ Number: 1, Width: 595, Height: 842 }],
        Fields: [
          { Key: "poNumber", Cell: extractionCell("4500123456", "extracted") },
          { Key: "poDate", Cell: extractionCell("2026-10-12", "extracted") },
          { Key: "requestedDeliveryDate", Cell: extractionCell("", "absent", null) },
          { Key: "currency", Cell: extractionCell("USD", "extracted") },
          { Key: "paymentTerms", Cell: extractionCell("30 days after delivery", "needs_review") },
          { Key: "incoterms", Cell: extractionCell("CIF", "extracted") },
        ],
        Rows: [
          {
            LineNumber: extractionCell("1", "extracted"),
            Description: extractionCell("Extracted line", "extracted"),
            Quantity: extractionCell("2", "extracted"),
            UoM: extractionCell("Pc", "extracted"),
            UnitPrice: extractionCell("1250.00", "extracted"),
            Amount: extractionCell("2500.00", "extracted"),
            Status: "extracted",
            Page: 1,
            BBox: { X: 50, Y: 225, Width: 490, Height: 10 },
            Note: "",
          },
        ],
        Abstentions: { Fields: 2, Rows: 0 },
      },
      ID: 9,
      PurchaseOrderID: 7,
      DocumentID: 4,
      DocumentVersion: 1,
      PageCount: 1,
      RequestedBy: 3,
      CreatedAt: "2026-10-21T10:00:00Z",
      StartedAt: "2026-10-21T10:00:01Z",
      FinishedAt: "2026-10-21T10:00:02Z",
      DurationMS: 20,
    },
    Document: { ID: 4, Version: 1, SHA256: "doc-sha" },
  };
}

function extractionDocumentState(
  id = 4,
  version = 1,
  sha256 = "doc-sha"
): PurchaseOrderDocumentStateDTO {
  const document = {
    UUID: `doc-${id}-${version}`,
    SHA256: sha256,
    ContentType: "application/pdf",
    OriginalFilename: "Customer PO.pdf",
    UploadedAt: "2026-10-21T10:00:00Z",
    ID: id,
    PurchaseOrderID: 7,
    Version: version,
    SizeBytes: 1536,
    UploadedBy: 4,
    Active: true,
  };
  return { Document: document, Versions: [document] };
}

function extractionCheckbox(label: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`);
  if (!found) throw new Error(`checkbox ${label} not found`);
  return found;
}

function applySelectedButtonOrNull(): HTMLButtonElement | null {
  return (
    Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find((candidate) =>
      candidate.textContent?.startsWith("Apply selected")
    ) ?? null
  );
}

function applySelectedButton(): HTMLButtonElement {
  const found = applySelectedButtonOrNull();
  if (!found) throw new Error("Apply selected button not found");
  return found;
}

function chooseDocumentFile(name = "Replacement.pdf", content = "%PDF-1.7\n"): void {
  const input = document.querySelector<HTMLInputElement>(
    'input[aria-label="Customer PO PDF file"]'
  );
  if (!input) throw new Error("file input not found");
  const file = new File([content], name, { type: "application/pdf" });
  Object.defineProperty(input, "files", {
    configurable: true,
    value: [file],
  });
  act(() => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

function inputValue(label: string): string | undefined {
  return document.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`)?.value;
}

function dialogCount(): number {
  return document.querySelectorAll('[role="alertdialog"]').length;
}

function paymentTermsValue(): string | undefined {
  return document.querySelector<HTMLInputElement>(
    'input[aria-label="Payment terms"]'
  )?.value;
}

function pressKey(target: EventTarget, key: string, shiftKey = false): void {
  act(() => {
    target.dispatchEvent(
      new KeyboardEvent("keydown", {
        key,
        shiftKey,
        bubbles: true,
        cancelable: true,
      })
    );
  });
}

function button(label: string, scope: ParentNode = document): HTMLButtonElement {
  const found = Array.from(
    scope.querySelectorAll<HTMLButtonElement>(`button[aria-label="${label}"]`)
  )[0];
  if (!found) throw new Error(`button ${label} not found`);
  return found;
}

describe("PO tab navigation leaves unsaved edits (issue #468)", () => {
  it("asks before a tab unmounts an edited draft, and Keep editing keeps it", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "50% advance");
    expect(
      document.querySelector<HTMLInputElement>('input[aria-label="Payment terms"]')
        ?.value
    ).toBe("50% advance");

    click(tab("Editor"));
    expect(prompt()).not.toBeNull();
    // Still on the PO page: the editor is mounted underneath the prompt.
    expect(document.querySelector('[data-testid="page-create"]')).toBeNull();

    click(button("Keep editing"));
    expect(prompt()).toBeNull();
    expect(
      document.querySelector<HTMLInputElement>('input[aria-label="Payment terms"]')
        ?.value
    ).toBe("50% advance");
  });

  it("Discard changes navigates to the requested tab", () => {
    act(() => root.render(<Workspace />));

    typeInto("Ship-to code", "SHIP-EDITED");
    click(tab("Archive"));
    expect(prompt()).not.toBeNull();

    click(button("Discard changes"));
    expect(prompt()).toBeNull();
    expect(document.querySelector('[data-testid="page-archive"]')).not.toBeNull();
    expect(
      document.querySelector('input[aria-label="Payment terms"]')
    ).toBeNull();
  });

  it("navigates without a prompt when nothing changed", () => {
    act(() => root.render(<Workspace />));

    click(tab("Drafts"));
    expect(prompt()).toBeNull();
    expect(document.querySelector('[data-testid="page-drafts"]')).not.toBeNull();
  });
});

describe("PO discard is one modal decision (issue #468, wave 3)", () => {
  it("Cancel opens one dialog, a tab click replaces its action, Keep editing stays", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "50% advance");
    const cancel = button("Cancel draft");
    act(() => cancel.focus());
    click(cancel);

    expect(dialogCount()).toBe(1);
    const dialog = prompt();
    expect(dialog).not.toBeNull();
    expect(dialog?.contains(document.activeElement)).toBe(true);

    const shell = document.querySelector<HTMLElement>("[data-app-shell]");
    expect(shell).not.toBeNull();
    expect(shell?.getAttribute("aria-hidden")).toBe("true");
    expect(shell?.hasAttribute("inert")).toBe(true);

    // A tab click while the Cancel confirmation stands must replace the
    // pending action, never stack a second dialog.
    click(tab("Archive"));
    expect(dialogCount()).toBe(1);
    expect(prompt()?.contains(document.activeElement)).toBe(true);

    click(button("Keep editing"));
    expect(dialogCount()).toBe(0);
    expect(document.querySelector('[data-testid="page-archive"]')).toBeNull();
    expect(paymentTermsValue()).toBe("50% advance");
    expect(document.activeElement).toBe(cancel);
    expect(shell?.getAttribute("aria-hidden")).toBeNull();
    expect(shell?.hasAttribute("inert")).toBe(false);
  });

  it("Discard from a Cancel-then-tab prompt performs the tab action", () => {
    act(() => root.render(<Workspace />));

    typeInto("Ship-to code", "SHIP-EDITED");
    click(button("Cancel draft"));
    expect(dialogCount()).toBe(1);

    click(tab("Archive"));
    expect(dialogCount()).toBe(1);

    click(button("Discard changes"));
    expect(dialogCount()).toBe(0);
    expect(document.querySelector('[data-testid="page-archive"]')).not.toBeNull();
    expect(paymentTermsValue()).toBeUndefined();
  });

  it("Discard from Cancel leaves the PO workspace", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "50% advance");
    click(button("Cancel draft"));
    expect(dialogCount()).toBe(1);

    click(button("Discard changes"));
    expect(dialogCount()).toBe(0);
    expect(document.querySelector('[data-testid="page-home"]')).not.toBeNull();
    expect(paymentTermsValue()).toBeUndefined();
  });

  it("Escape keeps editing and restores focus to the button that asked", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "30 days after delivery");
    const cancel = button("Cancel draft");
    act(() => cancel.focus());
    click(cancel);

    expect(dialogCount()).toBe(1);
    expect(document.activeElement).toBe(button("Keep editing"));

    pressKey(document, "Escape");

    expect(dialogCount()).toBe(0);
    expect(paymentTermsValue()).toBe("30 days after delivery");
    expect(document.activeElement).toBe(cancel);
  });

  it("contains Tab and Shift+Tab inside the dialog", () => {
    act(() => root.render(<Workspace />));

    typeInto("Payment terms", "50% advance");
    click(button("Cancel draft"));

    const keep = button("Keep editing");
    const discard = button("Discard changes");
    expect(dialogCount()).toBe(1);
    expect(document.activeElement).toBe(keep);

    pressKey(keep, "Tab", true);
    expect(document.activeElement).toBe(discard);

    pressKey(discard, "Tab");
    expect(document.activeElement).toBe(keep);
    expect(dialogCount()).toBe(1);
    expect(prompt()?.contains(document.activeElement)).toBe(true);
  });
});

describe("PO extraction applies only chosen proposals (issue #477)", () => {
  it("shows the evidence and leaves every value unchanged until Apply", async () => {
    extractionMock.mockResolvedValue(extractionState());
    act(() => root.render(<Workspace />));
    await flush();

    expect(document.querySelector('[data-testid="po-extraction-panel"]')).not.toBeNull();
    expect(document.body.textContent).toContain("Page 1 · 86–142 × 82–91 pt");
    expect(inputValue("Customer PO number")).toBe("PO-1");
    expect(inputValue("Payment terms")).toBe("45 days");
  });

  it("applies a ticked header field and leaves an unticked one alone", async () => {
    extractionMock.mockResolvedValue(extractionState());
    act(() => root.render(<Workspace />));
    await flush();

    // #479: the draft already held a different PO number, so it is a conflict
    // with an explicit choice rather than a checkbox that would overwrite it.
    const useExtracted = document.querySelector<HTMLInputElement>(
      'input[type="radio"][aria-label="Use extracted Customer PO number"]'
    );
    expect(useExtracted?.checked).toBe(false);
    act(() => useExtracted?.click());
    act(() => applySelectedButton().click());

    expect(inputValue("Customer PO number")).toBe("4500123456");
    expect(inputValue("Payment terms")).toBe("45 days");
    // The empty PO date was preselected by the conservative default; the
    // nonempty payment terms and quoted Incoterms were not.
    expect(inputValue("PO date")).toBe("2026-10-12");
    expect(document.body.textContent).toContain("Applied 2 fields");
  });

  it("replaces the line set only when a row is ticked", async () => {
    extractionMock.mockResolvedValue(extractionState());
    act(() => root.render(<Workspace />));
    await flush();

    expect(inputValue("Line 1 description")).toBe("Widget");
    const row = extractionCheckbox("Apply extracted line 1");
    expect(row.checked).toBe(false);
    act(() => row.click());
    act(() => applySelectedButton().click());

    expect(inputValue("Line 1 description")).toBe("Extracted line");
    expect(inputValue("Line 1 quantity")).toBe("2");
    expect(inputValue("Line 1 unit price")).toBe("$1,250.00");
  });

  it("removes Apply as soon as the PDF is replaced, even while the document refresh is pending", async () => {
    extractionMock.mockResolvedValue(extractionState());
    let resolveDocumentRefresh!: (state: PurchaseOrderDocumentStateDTO) => void;
    const pendingDocumentRefresh = new Promise<PurchaseOrderDocumentStateDTO>((resolve) => {
      resolveDocumentRefresh = resolve;
    });
    documentStateMock.mockReturnValueOnce(Promise.resolve(extractionDocumentState()));
    documentStateMock.mockReturnValueOnce(pendingDocumentRefresh);
    attachMock.mockResolvedValue(undefined);

    act(() => root.render(<Workspace />));
    await flush();

    expect(document.querySelector('[data-testid="po-extraction-panel"]')).not.toBeNull();
    const row = extractionCheckbox("Apply extracted line 1");
    act(() => row.click());
    expect(applySelectedButtonOrNull()).not.toBeNull();

    // Replacing the PDF succeeds, but the refreshed document state is still in
    // flight: this is exactly the window where old proposals used to stay live.
    chooseDocumentFile();
    await flush();

    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();

    // When the refresh finally resolves to a new generation, the old result is
    // still not actionable even though the extraction read returns it again.
    await act(async () => {
      resolveDocumentRefresh(extractionDocumentState(5, 2, "replacement-sha"));
      await Promise.resolve();
    });
    await flush();
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();
  });

  it("discards a stale result visibly and offers no Apply", async () => {
    const stale = extractionState();
    stale.Run.Status = "stale";
    stale.Run.Result = null as unknown as typeof stale.Run.Result;
    stale.Document.SHA256 = "different-document";
    extractionMock.mockResolvedValue(stale);
    act(() => root.render(<Workspace />));
    await flush();

    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(document.body.textContent).toContain("The PDF changed after this extraction");
    expect(inputValue("Customer PO number")).toBe("PO-1");
  });
});

// Issue #477, wave 3. The Judge replaced the PDF while a read that started
// against the previous document was still in flight, then released that read:
// the old proposals (and its PO date) came back. Every asynchronous acquisition
// path captures the active-document generation when its request starts, and a
// response from a generation that has ended is discarded instead of adopted.
describe("PO extraction late responses cannot resurrect a replaced document (#477)", () => {
  it("discards a delayed initial read after the PDF is replaced", async () => {
    const delayedInitialRead = deferred<ReturnType<typeof extractionState>>();
    extractionMock.mockReturnValueOnce(delayedInitialRead.promise);
    // Every read after the initial one returns the old result, as a server that
    // has not observed the replacement yet would.
    extractionMock.mockResolvedValue(extractionState());
    const pendingDocumentRefresh = deferred<PurchaseOrderDocumentStateDTO>();
    documentStateMock.mockResolvedValueOnce(extractionDocumentState());
    documentStateMock.mockReturnValueOnce(pendingDocumentRefresh.promise);
    attachMock.mockResolvedValue(undefined);

    act(() => root.render(<Workspace />));
    await flush();

    // The refresh the document panel triggered has already made the old result
    // actionable, so this is the state the delayed response could restore.
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).not.toBeNull();
    expect(inputValue("PO date")).toBe("");

    chooseDocumentFile();
    await flush();
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();

    await act(async () => {
      delayedInitialRead.resolve(extractionState());
      await Promise.resolve();
    });
    await flush();

    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();
    // The late response is discarded, not adopted as inert stale evidence: an
    // adopted response would render its empty stale banner here.
    expect(document.querySelector('[role="status"]')).toBeNull();
    expect(inputValue("PO date")).toBe("");
    expect(inputValue("Customer PO number")).toBe("PO-1");
  });

  it("discards a delayed start response after the PDF is replaced", async () => {
    extractionMock.mockResolvedValue({ Run: null, Document: null });
    const delayedStart = deferred<ReturnType<typeof extractionState>>();
    startExtractionMock.mockReturnValueOnce(delayedStart.promise);
    const pendingDocumentRefresh = deferred<PurchaseOrderDocumentStateDTO>();
    documentStateMock.mockResolvedValueOnce(extractionDocumentState());
    documentStateMock.mockReturnValueOnce(pendingDocumentRefresh.promise);
    attachMock.mockResolvedValue(undefined);

    act(() => root.render(<Workspace />));
    await flush();
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();

    act(() => button("Extract and compare").click());
    await flush();
    expect(startExtractionMock).toHaveBeenCalledTimes(1);

    chooseDocumentFile();
    await flush();
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();

    await act(async () => {
      delayedStart.resolve(extractionState());
      await Promise.resolve();
    });
    await flush();

    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();
    // Discarded, not adopted as inert stale evidence (an adopted response would
    // render its empty stale banner here).
    expect(document.querySelector('[role="status"]')).toBeNull();
    expect(inputValue("PO date")).toBe("");
    expect(inputValue("Customer PO number")).toBe("PO-1");
  });

  it("discards a delayed poll response after the PDF is replaced", async () => {
    const running: any = extractionState();
    running.Run.Status = "running";
    running.Run.Result = null;
    extractionMock.mockResolvedValueOnce(running);
    extractionMock.mockResolvedValueOnce(running);
    const delayedPoll = deferred<ReturnType<typeof extractionState>>();
    extractionMock.mockReturnValueOnce(delayedPoll.promise);
    const pendingDocumentRefresh = deferred<PurchaseOrderDocumentStateDTO>();
    documentStateMock.mockResolvedValueOnce(extractionDocumentState());
    documentStateMock.mockReturnValueOnce(pendingDocumentRefresh.promise);
    attachMock.mockResolvedValue(undefined);

    act(() => root.render(<Workspace />));
    await flush();
    expect(document.body.textContent).toContain("Reading the PDF text");

    // Let the one-second poll fire; its response stays in flight.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 1100));
    });
    expect(extractionMock.mock.calls.length).toBeGreaterThanOrEqual(3);

    chooseDocumentFile();
    await flush();
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();

    await act(async () => {
      delayedPoll.resolve(extractionState());
      await Promise.resolve();
    });
    await flush();

    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();
    // Discarded, not adopted as inert stale evidence (an adopted response would
    // render its empty stale banner here).
    expect(document.querySelector('[role="status"]')).toBeNull();
    expect(inputValue("PO date")).toBe("");
    expect(inputValue("Customer PO number")).toBe("PO-1");
  });

  it("ignores a document panel read that lands after the replacement was read", async () => {
    const delayedDocumentRead = deferred<PurchaseOrderDocumentStateDTO>();
    documentStateMock.mockReturnValueOnce(delayedDocumentRead.promise);
    documentStateMock.mockResolvedValueOnce(extractionDocumentState(5, 2, "replacement-sha"));
    attachMock.mockResolvedValue(undefined);
    extractionMock.mockResolvedValue(extractionState());

    act(() => root.render(<Workspace />));
    await flush();

    // The replacement has been read as version 2; its old extraction result is
    // already discarded as stale.
    chooseDocumentFile();
    await flush();
    expect(document.body.textContent).toContain("Version 2 of 1");
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();

    // The panel's first read now lands with the replaced version 1 state. It
    // must not restore the old document generation, its result, or Apply.
    await act(async () => {
      delayedDocumentRead.resolve(extractionDocumentState());
      await Promise.resolve();
    });
    await flush();

    expect(document.body.textContent).toContain("Version 2 of 1");
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(applySelectedButtonOrNull()).toBeNull();
    expect(inputValue("PO date")).toBe("");
    expect(inputValue("Customer PO number")).toBe("PO-1");
  });
});

describe("PO extraction failure keeps the manual path (issue #477)", () => {
  it("shows the typed reason and changes no field", async () => {
    const failed: any = extractionState();
    failed.Run.Status = "failed";
    failed.Run.ErrorCode = "page_limit";
    failed.Run.Result = null as unknown as typeof failed.Run.Result;
    extractionMock.mockResolvedValue(failed);
    act(() => root.render(<Workspace />));
    await flush();

    expect(document.body.textContent).toContain("more pages");
    expect(document.body.textContent).toContain("manual");
    expect(inputValue("Customer PO number")).toBe("PO-1");
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
  });
});

// Issue #479: staff keep entering values while extraction runs. A late result
// is reconciled field by field: untouched empty fields fill in, a field the
// draft already held (or that staff cleared) waits for an explicit choice, and
// nothing reaches the editor until Apply. Retry keeps the working copy; a
// replaced PDF discards the run and its tracked edits.
describe("PO extraction reconciles manual edits (issue #479)", () => {
  function radio(label: string): HTMLInputElement {
    const found = document.querySelector<HTMLInputElement>(
      `input[type="radio"][aria-label="${label}"]`
    );
    if (!found) throw new Error(`radio ${label} not found`);
    return found;
  }

  function conflict(key: string): HTMLElement | null {
    return document.querySelector<HTMLElement>(
      `[data-testid="po-extraction-conflict-${key}"]`
    );
  }

  it("asks only about values changed while extraction ran, and applies the chosen sides", async () => {
    extractionMock.mockResolvedValue({ Run: null, Document: null });
    const delayedStart = deferred<ReturnType<typeof extractionState>>();
    startExtractionMock.mockReturnValueOnce(delayedStart.promise);

    act(() => root.render(<Workspace />));
    await flush();

    // Staff edit one field and deliberately clear another while the run is
    // still in flight.
    typeInto("Customer PO number", "MANUAL-1");
    typeInto("Payment terms", "");
    act(() => button("Extract and compare").click());
    await flush();
    expect(startExtractionMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      delayedStart.resolve(extractionState());
      await Promise.resolve();
    });
    await flush();

    // Only the two differing values get a choice. The untouched empty PO date
    // is preselected without a prompt; an equal Incoterms value and an absent
    // requested date are not conflicts.
    expect(conflict("poNumber")).not.toBeNull();
    expect(conflict("paymentTerms")).not.toBeNull();
    expect(conflict("poDate")).toBeNull();
    expect(conflict("incoterms")).toBeNull();
    expect(conflict("requestedDeliveryDate")).toBeNull();
    expect(extractionCheckbox("Apply extracted Customer PO date").checked).toBe(true);
    expect(radio("Keep draft Customer PO number").checked).toBe(true);
    expect(radio("Use extracted Payment terms").checked).toBe(false);

    // Nothing has moved yet: the choices are a decision, not an edit.
    expect(inputValue("Customer PO number")).toBe("MANUAL-1");
    expect(inputValue("Payment terms")).toBe("");

    // Keep the edited PO number, use the extracted payment terms.
    act(() => radio("Use extracted Payment terms").click());
    act(() => applySelectedButton().click());

    expect(inputValue("Customer PO number")).toBe("MANUAL-1");
    expect(inputValue("Payment terms")).toBe("30 days after delivery");
    expect(inputValue("PO date")).toBe("2026-10-12");
    expect(document.body.textContent).toContain("Applied 2 fields");
  });

  it("keeps manual edits and the reviewed choice through Extract again", async () => {
    extractionMock.mockResolvedValue(extractionState());
    act(() => root.render(<Workspace />));
    await flush();

    expect(conflict("paymentTerms")).not.toBeNull();
    act(() => radio("Use extracted Payment terms").click());
    typeInto("Customer PO number", "MANUAL-1");

    const delayedRetry = deferred<ReturnType<typeof extractionState>>();
    startExtractionMock.mockReturnValueOnce(delayedRetry.promise);
    act(() => button("Extract and compare").click());
    await flush();

    // The retry is only a request: the editor keeps both values and the
    // reviewed choice while the new run is pending.
    expect(inputValue("Customer PO number")).toBe("MANUAL-1");
    expect(inputValue("Payment terms")).toBe("45 days");
    expect(radio("Use extracted Payment terms").checked).toBe(true);

    await act(async () => {
      delayedRetry.resolve(extractionState());
      await Promise.resolve();
    });
    await flush();

    expect(inputValue("Customer PO number")).toBe("MANUAL-1");
    expect(radio("Use extracted Payment terms").checked).toBe(true);
    act(() => applySelectedButton().click());
    expect(inputValue("Payment terms")).toBe("30 days after delivery");
    expect(inputValue("Customer PO number")).toBe("MANUAL-1");
  });

  it("keeps unresolved conflicts when only an auto field is applied", async () => {
    extractionMock.mockResolvedValue({ Run: null, Document: null });
    const delayedStart = deferred<ReturnType<typeof extractionState>>();
    startExtractionMock.mockReturnValueOnce(delayedStart.promise);

    act(() => root.render(<Workspace />));
    await flush();

    // One cleared header and one edited line while the run is in flight.
    typeInto("Payment terms", "");
    typeInto("Line 1 description", "Typed by hand");
    act(() => button("Extract and compare").click());
    await flush();
    await act(async () => {
      delayedStart.resolve(extractionState());
      await Promise.resolve();
    });
    await flush();

    expect(conflict("paymentTerms")).not.toBeNull();
    expect(document.querySelector('[data-testid="po-extraction-line-conflict"]')).not.toBeNull();

    // Applying the untouched empty PO date must consume only that proposal.
    act(() => applySelectedButton().click());
    expect(inputValue("PO date")).toBe("2026-10-12");
    expect(inputValue("Payment terms")).toBe("");
    expect(inputValue("Line 1 description")).toBe("Typed by hand");

    expect(conflict("paymentTerms")).not.toBeNull();
    expect(document.querySelector('[data-testid="po-extraction-line-conflict"]')).not.toBeNull();
    expect(extractionCheckbox("Apply extracted line 1").disabled).toBe(true);
  });

  it("discards a late result, its conflicts and its choices after the PDF is replaced", async () => {
    extractionMock.mockResolvedValue({ Run: null, Document: null });
    const delayedStart = deferred<ReturnType<typeof extractionState>>();
    startExtractionMock.mockReturnValueOnce(delayedStart.promise);
    documentStateMock.mockResolvedValueOnce(extractionDocumentState());
    documentStateMock.mockResolvedValue(
      extractionDocumentState(5, 2, "replacement-sha")
    );
    attachMock.mockResolvedValue(undefined);

    act(() => root.render(<Workspace />));
    await flush();

    typeInto("Customer PO number", "MANUAL-1");
    act(() => button("Extract and compare").click());
    await flush();
    chooseDocumentFile();
    await flush();

    expect(conflict("poNumber")).toBeNull();
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();

    await act(async () => {
      delayedStart.resolve(extractionState());
      await Promise.resolve();
    });
    await flush();

    // The result belonged to the replaced generation: it cannot bring back a
    // conflict, a choice, or a proposal for the new document.
    expect(document.querySelector('[data-testid="po-extraction-panel"]')).toBeNull();
    expect(conflict("poNumber")).toBeNull();
    expect(
      document.querySelector('input[type="radio"][aria-label="Use extracted Customer PO number"]')
    ).toBeNull();
    expect(inputValue("Customer PO number")).toBe("MANUAL-1");
  });
});
