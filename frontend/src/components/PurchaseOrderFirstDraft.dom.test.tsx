// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderDTO, PurchaseOrderUpdateInput } from "../purchaseOrder";
import { blankPurchaseOrderDraft } from "../purchaseOrder";
import { usePurchaseOrderWorkspace } from "../poWorkspace";
import { PurchaseOrderWorkspacePage } from "./PurchaseOrderWorkspace";

// Issue #496: PO-first entry in a real DOM. The workspace controller, the real
// editor, the real document panel and the real reducer are all production
// code; only the desktop bindings are replaced. These cover the behaviours the
// SSR suite cannot: an untouched visit creating no row, the first Save creating
// exactly one row and then updating it, upload create-then-attach with a retry
// on the same id, the deferred Link affordance and the hidden source evidence.

const {
  createMock,
  updateMock,
  attachMock,
  documentStateMock,
} = vi.hoisted(() => ({
  createMock: vi.fn(),
  updateMock: vi.fn(),
  attachMock: vi.fn(),
  documentStateMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  CorrectPurchaseOrder: vi.fn(),
  CreatePurchaseOrderBlankDraft: (...args: unknown[]) => createMock(...args),
  FinalizePurchaseOrder: vi.fn(),
  GetActivePurchaseOrderDocumentContent: vi.fn(),
  GetPurchaseOrderDocumentState: (...args: unknown[]) => documentStateMock(...args),
  GetPurchaseOrderHistory: vi.fn(async () => []),
  GetPurchaseOrderReview: vi.fn(),
  AttachPurchaseOrderDocument: (...args: unknown[]) => attachMock(...args),
  RemovePurchaseOrderDocument: vi.fn(),
  UpdatePurchaseOrderDraft: (...args: unknown[]) => updateMock(...args),
}));

vi.mock("@backend/models", () => {
  class Input {
    constructor(source: Record<string, unknown>) {
      Object.assign(this, source);
    }
  }
  return {
    backend: {
      PurchaseOrderBlankCreateInput: Input,
      PurchaseOrderCorrectionInput: Input,
      PurchaseOrderFinalizeInput: Input,
      PurchaseOrderItemInput: Input,
      PurchaseOrderUpdateInput: Input,
      PurchaseOrderDocumentUploadInput: Input,
    },
  };
});

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const UOMS = [{ value: "Pc", label: "Pc" }];
const INCOTERMS = [{ value: "EXW", label: "EXW" }];

function blank(): PurchaseOrderDTO {
  return blankPurchaseOrderDraft({
    currency: "IDR",
    taxZone: "FTZ",
    taxType: "Goods",
    serviceCategory: "",
    receivedDate: "2026-10-22",
    incoterms: "EXW",
  });
}

function createdRow(): PurchaseOrderDTO {
  return {
    ...blank(),
    ID: 41,
    UUID: "uuid-496",
    Version: 1,
  };
}

function savedFrom(input: PurchaseOrderUpdateInput, version: number): PurchaseOrderDTO {
  return {
    ...createdRow(),
    ID: input.ID,
    Version: version,
    PONumber: input.PONumber,
    PODate: input.PODate || null,
    RequestedDeliveryDate: input.RequestedDeliveryDate || null,
    BillingAddress: input.BillingAddress,
    DeliveryAddress: input.DeliveryAddress,
    ReceivedDate: input.ReceivedDate,
    PaymentTerms: input.PaymentTerms,
    Incoterms: input.Incoterms,
    Currency: input.Currency,
    TaxZone: input.TaxZone,
    TaxType: input.TaxType,
    ServiceCategory: input.ServiceCategory,
    PpnBps: input.TaxZone === "NonFTZ" && input.TaxType === "Goods" ? 1100 : 0,
    Items: input.Items.map((item, index) => ({
      QuotationItemNo: item.QuotationItemNo ?? null,
      QuotationSubNo: item.QuotationSubNo ?? null,
      LineNumber: index + 1,
      Description: item.Description,
      QuantityScaled: item.QuantityScaled,
      UoM: item.UoM,
      UnitPriceMinor: item.UnitPriceMinor,
      DiscountEnabled: item.DiscountEnabled,
      DiscountPercent: item.DiscountPercent,
      GrossMinor: 0,
      DiscountMinor: 0,
      NetMinor: 0,
    })),
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  createMock.mockReset();
  updateMock.mockReset();
  attachMock.mockReset();
  documentStateMock.mockReset();
  createMock.mockResolvedValue(createdRow());
  updateMock.mockImplementation(async (input: PurchaseOrderUpdateInput) => ({
    Order: savedFrom(input, input.ExpectedVersion + 1),
  }));
  documentStateMock.mockResolvedValue({ Document: null, Versions: [] });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  // jsdom does not implement the scroll helper the Select menu uses.
  Element.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function input(label: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    `input[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no input labelled ${label}`);
  return found;
}

function chooseSelect(label: string, option: string): void {
  act(() => button(label).click());
  const found = Array.from(
    document.querySelectorAll<HTMLElement>('[role="option"]')
  ).find((node) => (node.textContent ?? "").trim().includes(option));
  if (found === undefined) throw new Error(`no option ${option} for ${label}`);
  act(() => found.click());
}

function type(label: string, value: string): void {
  const field = input(label);
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value"
  )?.set;
  act(() => {
    setter?.call(field, value);
    field.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function fileInput(): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    'input[aria-label="Customer PO PDF file"]'
  );
  if (found === null) throw new Error("file input not found");
  return found;
}

function chooseFile(name = "Customer PO.pdf", content = "%PDF-1.7\n"): void {
  const input = fileInput();
  const file = new File([content], name, { type: "application/pdf" });
  Object.defineProperty(input, "files", {
    configurable: true,
    value: [file],
  });
  act(() => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

const PO_DOCUMENT = {
  UUID: "doc-498",
  SHA256: "doc-sha-498",
  ContentType: "application/pdf",
  OriginalFilename: "Customer PO.pdf",
  UploadedAt: "2026-10-23T10:00:00Z",
  ID: 4,
  PurchaseOrderID: 41,
  Version: 1,
  SizeBytes: 1536,
  UploadedBy: 4,
  Active: true,
};

function Harness({
  onLink,
  draft,
}: {
  onLink?: () => void;
  draft?: PurchaseOrderDTO;
}) {
  const workspace = usePurchaseOrderWorkspace();
  const [saved, setSaved] = useState<PurchaseOrderDTO | null>(null);
  return (
    <>
      <PurchaseOrderWorkspacePage
        workspace={workspace}
        currency={IDR}
        currencyOptions={[IDR, USD]}
        uomOptions={UOMS}
        incotermOptions={INCOTERMS}
        tab="editor"
        blankDraft={draft ?? blank()}
        onLinkQuotation={onLink}
        onAdd={() => {}}
        onOpen={() => {}}
        isAdmin={false}
        onRepairSource={() => {}}
        onSaved={(order) => setSaved(order)}
        onBack={() => {}}
        onCancel={() => {}}
        onDirtyChange={() => {}}
      />
      <span data-testid="saved-id">{saved?.ID ?? 0}</span>
    </>
  );
}

async function render(draft?: PurchaseOrderDTO): Promise<void> {
  await act(async () => {
    root.render(<Harness draft={draft} />);
  });
  await flush();
}

async function reopen(draft: PurchaseOrderDTO): Promise<void> {
  act(() => root.unmount());
  container.remove();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => {
    root.render(<Harness draft={draft} />);
  });
  await flush();
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function legacyDraft(state: "live" | "former"): PurchaseOrderDTO {
  return {
    ...blank(),
    ID: 7,
    UUID: "legacy-496",
    Version: 1,
    QuotationID: 12,
    CustomerID: 2,
    SourceRevision: 0,
    SourceState: state,
    SourceMissing: state === "former",
    PrintedTotalsKnown: true,
    QuoteNumber: "1502260001",
    CustomerName: "PT. Legacy Basis",
    TaxZone: "NonFTZ",
    TaxType: undefined,
    ServiceCategory: undefined,
    TaxCategory: "SupplyInstall",
    Items: [],
  };
}

describe("PO-first draft (issue #496)", () => {
  it("creates no row and shows no fake source on an untouched visit", async () => {
    await render();

    expect(createMock).not.toHaveBeenCalled();
    expect(documentStateMock).not.toHaveBeenCalled();
    const text = document.body.textContent ?? "";
    expect(text).toContain("New purchase order");
    expect(text).toContain("Source missing");
    expect(text).toContain("Available after a quotation revision is linked");
    // "Draft 0" and "revision 0" must never describe the transient row.
    expect(text).not.toContain("Draft 0");
    expect(text).not.toContain("revision 0");
    // A never-linked draft cannot finalize.
    expect(button("Final Save").disabled).toBe(true);
  });

  it("creates once on the first Save and updates that row on the next", async () => {
    await render();
    type("Customer PO number", "CUSTOMER-496");
    await flush();
    act(() => button("Save draft").click());
    await flush();

    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    const first = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    expect(first.ID).toBe(41);
    expect(first.ExpectedVersion).toBe(1);
    expect(first.PONumber).toBe("CUSTOMER-496");
    expect(first.Currency).toBe("IDR");
    expect(first.TaxZone).toBe("FTZ");
    expect(first.TaxType).toBe("Goods");

    // The saved answer replaces the transient, so a second Save updates the
    // same row instead of creating another.
    await flush();
    act(() => button("Save draft").click());
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(2);
    const second = updateMock.mock.calls[1][0] as PurchaseOrderUpdateInput;
    expect(second.ID).toBe(41);
    expect(second.ExpectedVersion).toBe(2);
  });

  it("creates once for an upload then retries the same id after a failed attach", async () => {
    attachMock
      .mockRejectedValueOnce(new Error("the upload was interrupted"))
      .mockResolvedValueOnce({
        ID: 1,
        UUID: "doc-1",
        PurchaseOrderID: 41,
        Version: 1,
        Active: true,
      });
    await render();

    chooseFile();
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(attachMock).toHaveBeenCalledTimes(1);
    const firstAttach = attachMock.mock.calls[0][0] as { PurchaseOrderID: number };
    expect(firstAttach.PurchaseOrderID).toBe(41);

    // Retry the upload. The row already exists, so no second create happens.
    chooseFile("Customer PO retry.pdf");
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(attachMock).toHaveBeenCalledTimes(2);
    const retryAttach = attachMock.mock.calls[1][0] as { PurchaseOrderID: number };
    expect(retryAttach.PurchaseOrderID).toBe(41);
  });

  it("keeps entered values when the deferred Link affordance is used", async () => {
    const onLink = vi.fn();
    await act(async () => {
      root.render(<Harness onLink={onLink} />);
    });
    await flush();
    type("Customer PO number", "KEEP-ME");
    await flush();

    act(() => button("Link a Quotation").click());
    await flush();
    expect(onLink).toHaveBeenCalledTimes(1);
    expect(createMock).not.toHaveBeenCalled();
    expect(updateMock).not.toHaveBeenCalled();
    expect(input("Customer PO number").value).toBe("KEEP-ME");
  });

  it("edits currency and the tax triple before linking and sends them", async () => {
    await render();
    chooseSelect("Currency", "USD");
    chooseSelect("Tax zone", "Non-FTZ");
    await flush();

    act(() => button("Save draft").click());
    await flush();
    expect(updateMock).toHaveBeenCalledTimes(1);
    const payload = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    expect(payload.Currency).toBe("USD");
    expect(payload.TaxZone).toBe("NonFTZ");
    expect(payload.TaxType).toBe("Goods");
    expect(payload.ServiceCategory).toBe("");
  });

  it("does not start an upload while a Save is in flight", async () => {
    const gate = deferred<PurchaseOrderDTO>();
    createMock.mockReset();
    createMock.mockReturnValue(gate.promise);
    await render();
    type("Customer PO number", "SAVE-BLOCKS-UPLOAD");
    await flush();

    act(() => button("Save draft").click());
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);

    // A forced file event is captured with the control state so a red run shows
    // both halves: the controls must be blocked and no attach may start.
    const whileSaving = {
      inputDisabled: fileInput().disabled,
      attachButtonDisabled:
        document.querySelector<HTMLButtonElement>(
          'button[aria-label="Attach customer PO PDF"]'
        )?.disabled ?? null,
    };
    chooseFile();
    await flush();

    gate.resolve(createdRow());
    await flush();
    await flush();
    expect({
      whileSaving,
      attachStarted: attachMock.mock.calls.length,
    }).toEqual({
      whileSaving: { inputDisabled: true, attachButtonDisabled: true },
      attachStarted: 0,
    });
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    // The controls return once the draft write settles.
    expect(fileInput().disabled).toBe(false);
  });

  it("shares one persistence when an upload is in flight and Save is pressed", async () => {
    const gate = deferred<PurchaseOrderDTO>();
    createMock.mockReset();
    createMock.mockReturnValue(gate.promise);
    attachMock.mockResolvedValue({
      ID: 1,
      UUID: "doc-upload-first",
      PurchaseOrderID: 41,
      Version: 1,
      Active: true,
    });
    await render();
    type("Customer PO number", "UPLOAD-FIRST");
    await flush();

    chooseFile();
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).not.toHaveBeenCalled();

    // Save joins the same in-flight persistence rather than creating a second
    // row or writing the same version twice.
    act(() => button("Save draft").click());
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);

    gate.resolve(createdRow());
    await flush();
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    expect(attachMock).toHaveBeenCalledTimes(1);
    // The Save path still notifies the page exactly once.
    expect(document.querySelector('[data-testid="saved-id"]')?.textContent).toBe("41");
  });

  it("retries a failed update on the created row without creating again", async () => {
    updateMock.mockRejectedValueOnce(new Error("the update was interrupted"));
    await render();
    type("Customer PO number", "RETRY-496");
    await flush();

    act(() => button("Save draft").click());
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);

    act(() => button("Save draft").click());
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(2);
    const second = updateMock.mock.calls[1][0] as PurchaseOrderUpdateInput;
    expect(second.ID).toBe(41);
  });

  it("does not submit a derived basis for a legacy linked draft", async () => {
    await render(legacyDraft("live"));
    act(() => button("Save draft").click());
    await flush();
    expect(updateMock).toHaveBeenCalledTimes(1);
    const payload = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    // The row's stored type is empty (legacy); the derived Services value is
    // display-only and must not be submitted as a frozen-basis change.
    expect(payload.TaxType).toBe("");
    expect(payload.ServiceCategory).toBe("");
  });

  it("submits the derived basis for a legacy former draft but not for a live one", async () => {
    // A formerly-linked draft may still edit its basis before a new link, so
    // the editor's display-derived legacy basis is submitted. A live row's
    // basis stays server-frozen and must never be submitted as a change.
    await render(legacyDraft("former"));
    act(() => button("Save draft").click());
    await flush();
    expect(updateMock).toHaveBeenCalledTimes(1);
    const formerPayload = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    expect(formerPayload.TaxType).toBe("Services");
    expect(formerPayload.ServiceCategory).toBe("NonConstruction");
    expect(formerPayload.Currency).toBe("IDR");

    await reopen(legacyDraft("live"));
    act(() => button("Save draft").click());
    await flush();
    expect(updateMock).toHaveBeenCalledTimes(2);
    const livePayload = updateMock.mock.calls[1][0] as PurchaseOrderUpdateInput;
    expect(livePayload.TaxType).toBe("");
    expect(livePayload.ServiceCategory).toBe("");
    expect(livePayload.Currency).toBe("");
  });

  it("persists the typed values before a transient upload attaches", async () => {
    documentStateMock.mockResolvedValue({
      Document: PO_DOCUMENT,
      Versions: [PO_DOCUMENT],
    });
    attachMock.mockResolvedValue(PO_DOCUMENT);
    await render();

    type("Customer PO number", "TYPED-BEFORE-UPLOAD");
    await flush();
    chooseFile();
    await flush();

    // One create, then one update carrying what staff typed, and only then the
    // attach: a reload cannot lose the values the user saw when attaching.
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    const payload = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    expect(payload.ID).toBe(41);
    expect(payload.ExpectedVersion).toBe(1);
    expect(payload.PONumber).toBe("TYPED-BEFORE-UPLOAD");
    expect(attachMock).toHaveBeenCalledTimes(1);
    expect(updateMock.mock.invocationCallOrder[0]).toBeLessThan(
      attachMock.mock.invocationCallOrder[0]
    );
    expect(
      (attachMock.mock.calls[0][0] as { PurchaseOrderID: number }).PurchaseOrderID
    ).toBe(41);
    expect(input("Customer PO number").value).toBe("TYPED-BEFORE-UPLOAD");
    // An upload saves the working copy but does not report the page as saved:
    // its own saveSucceeded keeps the editor's id/version, and no onSaved
    // remount can discard a failed attach's error.
    expect(document.querySelector('[data-testid="saved-id"]')?.textContent).toBe("0");
  });

  it("keeps a failed attach error visible without notifying the page", async () => {
    attachMock.mockRejectedValue(new Error("the upload was interrupted"));
    await render();
    type("Customer PO number", "ERROR-STAYS");
    await flush();

    chooseFile();
    await flush();

    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    expect(attachMock).toHaveBeenCalledTimes(1);
    // The upload must not remount the editor through onSaved: the panel that
    // showed the failure is still the one on screen, so the error is readable.
    expect(document.body.textContent).toContain("the upload was interrupted");
    expect(document.querySelector('[data-testid="saved-id"]')?.textContent).toBe("0");
  });

  it("attaches nothing when the transient working copy is invalid", async () => {
    await render();
    type("Received date", "");
    await flush();

    chooseFile();
    await flush();

    // Validation runs before the create, so an incomplete working copy leaves
    // no blank row and no attachment behind.
    expect(createMock).not.toHaveBeenCalled();
    expect(updateMock).not.toHaveBeenCalled();
    expect(attachMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("Received date must be YYYY-MM-DD.");
  });

  it("retries a failed attach on the saved row without writing again", async () => {
    attachMock
      .mockRejectedValueOnce(new Error("the upload was interrupted"))
      .mockResolvedValueOnce({
        ID: 1,
        UUID: "doc-latest",
        PurchaseOrderID: 41,
        Version: 1,
        Active: true,
      });
    await render();
    type("Customer PO number", "RETRY-LATEST");
    await flush();

    chooseFile();
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    expect(attachMock).toHaveBeenCalledTimes(1);
    const first = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    expect(first.ExpectedVersion).toBe(1);
    expect(first.PONumber).toBe("RETRY-LATEST");

    // The failed attach left the row saved at the version the update wrote. The
    // retry attaches to that row; it must not create a second one or rewrite
    // the same values against a stale version.
    expect(document.body.textContent).toContain("Loaded at version 2.");
    chooseFile("Customer PO retry.pdf");
    await flush();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    expect(attachMock).toHaveBeenCalledTimes(2);
    expect(
      (attachMock.mock.calls[1][0] as { PurchaseOrderID: number }).PurchaseOrderID
    ).toBe(41);
  });
});
