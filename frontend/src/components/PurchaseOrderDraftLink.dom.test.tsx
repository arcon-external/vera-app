// @vitest-environment jsdom
import { act, useEffect, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type {
  PurchaseOrderSourceDTO,
  PurchaseOrderSourceLineDTO,
} from "../poFinalize";
import type {
  PurchaseOrderDraftLinkInput,
  PurchaseOrderDraftLinkPreviewDTO,
} from "../poDraftLink";
import type { PurchaseOrderDTO, PurchaseOrderUpdateInput } from "../purchaseOrder";
import { blankPurchaseOrderDraft } from "../purchaseOrder";
import { usePurchaseOrderWorkspace } from "../poWorkspace";
import { PurchaseOrderWorkspacePage } from "./PurchaseOrderWorkspace";

// Issue #497: the Staff draft link in a real DOM. The workspace controller, the
// real editor, the real Source link panel, the real difference review and the
// real reducers are production code; only the desktop bindings are replaced.
// These cover the selector -> review -> link path, a currency mismatch with the
// edit-and-retry remedy, the required tax choice, value preservation, and ten
// manual lines explicitly matched one by one.

const {
  createMock,
  updateMock,
  previewMock,
  linkMock,
  documentStateMock,
  attachMock,
} = vi.hoisted(() => ({
  createMock: vi.fn(),
  updateMock: vi.fn(),
  previewMock: vi.fn(),
  linkMock: vi.fn(),
  documentStateMock: vi.fn(),
  attachMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  AttachPurchaseOrderDocument: (...args: unknown[]) => attachMock(...args),
  CorrectPurchaseOrder: vi.fn(),
  CreatePurchaseOrderBlankDraft: (...args: unknown[]) => createMock(...args),
  FinalizePurchaseOrder: vi.fn(),
  GetActivePurchaseOrderDocumentContent: vi.fn(),
  GetPurchaseOrder: vi.fn(),
  GetPurchaseOrderDocumentState: (...args: unknown[]) => documentStateMock(...args),
  GetPurchaseOrderHistory: vi.fn(async () => []),
  GetPurchaseOrderReview: async () => ({
    Order: latestOrder,
    Source: reviewForTest.Source,
    DocumentState: "none",
  }),
  LinkPurchaseOrderDraftSource: (...args: unknown[]) => linkMock(...args),
  PreviewPurchaseOrderDraftLink: (...args: unknown[]) => previewMock(...args),
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
      PurchaseOrderDocumentUploadInput: Input,
      PurchaseOrderDraftLinkInput: Input,
      PurchaseOrderFinalizeInput: Input,
      PurchaseOrderItemInput: Input,
      PurchaseOrderUpdateInput: Input,
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

let latestOrder: PurchaseOrderDTO;
let reviewForTest: PurchaseOrderDraftLinkPreviewDTO;

function blank(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    ...blankPurchaseOrderDraft({
      currency: "IDR",
      taxZone: "FTZ",
      taxType: "Goods",
      serviceCategory: "",
      receivedDate: "2026-10-22",
      paymentTerms: "50% advance",
      incoterms: "EXW",
    }),
    ...overrides,
  };
}

function createdRow(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    ...blank(),
    ID: 41,
    UUID: "uuid-497",
    Version: 1,
    ...overrides,
  };
}

function itemsFrom(input: PurchaseOrderUpdateInput) {
  return input.Items.map((item, index) => ({
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
  }));
}

function orderFromInput(input: PurchaseOrderUpdateInput, version: number): PurchaseOrderDTO {
  return {
    ...latestOrder,
    ID: input.ID,
    Version: version,
    PONumber: input.PONumber,
    BillingAddress: input.BillingAddress,
    DeliveryAddress: input.DeliveryAddress,
    ShipToCode: input.ShipToCode,
    ContactEmail: input.ContactEmail,
    ContactPhone: input.ContactPhone,
    PODate: input.PODate === "" ? null : input.PODate,
    ReceivedDate: input.ReceivedDate,
    RequestedDeliveryDate:
      input.RequestedDeliveryDate === "" ? null : input.RequestedDeliveryDate,
    PaymentTerms: input.PaymentTerms,
    Incoterms: input.Incoterms,
    DiscountPercent: input.DiscountPercent,
    AdjustmentMinor: input.AdjustmentMinor,
    AdjustmentReason: input.AdjustmentReason,
    Currency: input.Currency === "" ? latestOrder.Currency : input.Currency,
    TaxZone: input.TaxZone === "" ? latestOrder.TaxZone : input.TaxZone,
    TaxType: input.TaxType === "" ? latestOrder.TaxType : input.TaxType,
    ServiceCategory:
      input.ServiceCategory === "" ? latestOrder.ServiceCategory : input.ServiceCategory,
    Items: itemsFrom(input),
  };
}

function sourceLine(
  index: number,
  overrides: Partial<PurchaseOrderSourceLineDTO> = {}
): PurchaseOrderSourceLineDTO {
  return {
    QuotationItemNo: index,
    QuotationSubNo: null,
    LineLabel: String(index),
    Description: `Source line ${index}`,
    UoM: "Pc",
    QuantityScaled: 10000,
    UnitPriceMinor: 100,
    DiscountEnabled: false,
    DiscountPercent: 0,
    ...overrides,
  };
}

function source(
  overrides: Partial<PurchaseOrderSourceDTO> = {}
): PurchaseOrderSourceDTO {
  return {
    QuotationID: 12,
    RevisionNumber: 0,
    QuoteNumber: "1502260001",
    CustomerID: 2,
    CustomerName: "PT. Source",
    Currency: "IDR",
    PaymentTerms: "50% advance",
    Incoterms: "EXW",
    DiscountPercent: 0,
    Lines: [sourceLine(1)],
    PrintedSubtotalMinor: 100,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 100,
    ...overrides,
  };
}

function review(
  overrides: Partial<PurchaseOrderDraftLinkPreviewDTO> = {}
): PurchaseOrderDraftLinkPreviewDTO {
  return {
    Order: createdRow(),
    Source: source(),
    DocumentState: "none",
    DraftTax: {
      TaxZone: "FTZ",
      TaxType: "Goods",
      ServiceCategory: "",
      TaxCategory: "",
      PpnBps: 0,
    },
    SourceTax: {
      TaxZone: "FTZ",
      TaxType: "Goods",
      ServiceCategory: "",
      TaxCategory: "",
      PpnBps: 0,
    },
    TaxChoiceRequired: false,
    ...overrides,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  createMock.mockReset();
  updateMock.mockReset();
  previewMock.mockReset();
  linkMock.mockReset();
  documentStateMock.mockReset();
  attachMock.mockReset();

  latestOrder = createdRow();
  createMock.mockImplementation(async () => {
    latestOrder = createdRow();
    return latestOrder;
  });
  updateMock.mockImplementation(async (input: PurchaseOrderUpdateInput) => {
    latestOrder = orderFromInput(input, input.ExpectedVersion + 1);
    return { Order: latestOrder };
  });
  previewMock.mockImplementation(async () => reviewForTest);
  linkMock.mockImplementation(async (input: PurchaseOrderDraftLinkInput) => {
    const linked: PurchaseOrderDTO = {
      ...latestOrder,
      ID: input.ID,
      Version: input.ExpectedVersion + 1,
      SourceState: "live",
      SourceMissing: false,
      PrintedTotalsKnown: true,
      QuotationID: reviewForTest.Source.QuotationID,
      SourceRevision: reviewForTest.Source.RevisionNumber,
      QuoteNumber: reviewForTest.Source.QuoteNumber,
      CustomerID: reviewForTest.Source.CustomerID,
      CustomerName: reviewForTest.Source.CustomerName,
      TaxZone:
        input.TaxChoice === "source"
          ? reviewForTest.SourceTax.TaxZone
          : latestOrder.TaxZone,
      TaxType:
        input.TaxChoice === "source"
          ? reviewForTest.SourceTax.TaxType
          : latestOrder.TaxType,
      PpnBps:
        input.TaxChoice === "source"
          ? reviewForTest.SourceTax.PpnBps
          : reviewForTest.DraftTax.PpnBps,
      PrintedSubtotalMinor: reviewForTest.Source.PrintedSubtotalMinor,
      PrintedDiscountMinor: reviewForTest.Source.PrintedDiscountMinor,
      PrintedPpnMinor: reviewForTest.Source.PrintedPpnMinor,
      PrintedGrandTotalMinor: reviewForTest.Source.PrintedGrandTotalMinor,
    };
    latestOrder = linked;
    return { Order: linked };
  });
  documentStateMock.mockResolvedValue({ Document: null, Versions: [] });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
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

function chooseFile(name = "Customer PO.pdf"): void {
  const fileInput = document.querySelector<HTMLInputElement>(
    'input[aria-label="Customer PO PDF file"]'
  );
  if (fileInput === null) throw new Error("file input not found");
  const file = new File(["%PDF-1.7\n"], name, { type: "application/pdf" });
  Object.defineProperty(fileInput, "files", {
    configurable: true,
    value: [file],
  });
  act(() => {
    fileInput.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

function Harness({ draft, opened }: { draft?: PurchaseOrderDTO; opened?: PurchaseOrderDTO }) {
  const workspace = usePurchaseOrderWorkspace();
  // A saved draft opens through the workspace controller, exactly as a row
  // from the archive does. The blank fixture path mounts the transient editor,
  // which remounts on its first save and so cannot reproduce #584.
  useEffect(() => {
    if (opened !== undefined) workspace.open(opened);
  }, [opened, workspace.open]);
  const [target, setTarget] = useState<{
    quotationId: number;
    revisionNumber: number;
  } | null>(null);
  const [linked, setLinked] = useState<PurchaseOrderDTO | null>(null);
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
        onLinkQuotation={() =>
          setTarget({
            quotationId: reviewForTest.Source.QuotationID,
            revisionNumber: reviewForTest.Source.RevisionNumber,
          })
        }
        pendingLinkTarget={target}
        onLinkTargetHandled={() => setTarget(null)}
        onLinked={(order) => {
          workspace.saved(order);
          setLinked(order);
        }}
        onAdd={() => {}}
        onOpen={() => {}}
        isAdmin={false}
        onRepairSource={() => {}}
        onSaved={() => {}}
        onBack={() => {}}
        onCancel={() => {}}
        onDirtyChange={() => {}}
        reconciliationTransport={{
          load: async () => {
            throw new Error("no reconciliation recorded");
          },
          save: async (_id, session) => session,
        }}
      />
      <span data-testid="linked">
        {linked === null ? "none" : `${linked.QuoteNumber}:${linked.SourceRevision}`}
      </span>
    </>
  );
}

async function render(draft?: PurchaseOrderDTO, opened?: PurchaseOrderDTO): Promise<void> {
  latestOrder = opened ?? draft ?? createdRow();
  await act(async () => {
    root.render(<Harness draft={draft} opened={opened} />);
  });
  await flush();
  await flush();
}

/** Clicks the top affordance and waits for the persisted draft + review. */
async function openReview(): Promise<void> {
  act(() => button("Link a Quotation").click());
  await flush();
  await flush();
}

describe("draft source link (issue #497)", () => {
  it("links a never-linked draft immediately, without a review step", async () => {
    reviewForTest = review();
    await render();
    type("Customer PO number", "KEEP-497");
    await flush();

    await openReview();
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    const preSave = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    expect(preSave.ID).toBe(41);
    expect(preSave.PONumber).toBe("KEEP-497");
    expect(previewMock).toHaveBeenCalledWith(41, 12, 0);

    // The source is previewed and linked in the same action; the editor shows
    // the quotation values inline instead of a review screen.
    expect(linkMock).toHaveBeenCalledTimes(1);
    const linkInput = linkMock.mock.calls[0][0] as PurchaseOrderDraftLinkInput;
    expect(linkInput.ID).toBe(41);
    expect(linkInput.ExpectedVersion).toBe(2);
    expect(linkInput.TargetQuotationID).toBe(12);
    expect(linkInput.TargetRevisionNumber).toBe(0);
    // The empty order keeps no value and leaves the quotation line off; the
    // ghost below the line is where staff decide to add it.
    expect(linkInput.AcknowledgedMissing).toEqual(["1"]);
    expect(linkInput.AcknowledgedExtra).toEqual([]);
    expect(linkInput.TaxChoice).toBe("keep");
    expect(document.body.textContent).not.toContain(
      "Link this draft to a quotation"
    );
    expect(document.body.textContent).toContain("Quotation line 1");

    expect(document.querySelector('[data-testid="linked"]')?.textContent).toBe(
      "1502260001:0"
    );
    expect(document.body.textContent).toContain("Linked source");
    expect(
      document.querySelector('button[aria-label="Link a Quotation"]')
    ).toBeNull();
  });

  it("keeps values typed before the link and offers the quotation values instead of applying them", async () => {
    reviewForTest = review({
      Source: source({ PaymentTerms: "30 days", Incoterms: "DAP" }),
    });
    await render();

    type("Customer PO number", "KEEP-BEFORE-LINK");
    type("Payment terms", "45 days");
    await flush();

    await openReview();
    await flush();

    // The link stores what staff typed; the quotation's differing values are
    // offered in blue, not adopted behind their back.
    expect(input("Customer PO number").value).toBe("KEEP-BEFORE-LINK");
    expect(input("Payment terms").value).toBe("45 days");
    expect(input("Payment terms").getAttribute("style")).toMatch(
      /59, 130, 246|3b82f6/i
    );
    expect(button("Incoterms").textContent).toContain("EXW");
    // The old link review is gone: linking never opens a difference page.
    expect(document.body.textContent).not.toContain(
      "Choose a side for each difference"
    );
    expect(document.body.textContent).not.toContain("Back without linking");
    const linkInput = linkMock.mock.calls[0][0] as PurchaseOrderDraftLinkInput;
    expect(linkInput.Decisions).toEqual(
      expect.arrayContaining([
        { Key: "header:paymentTerms", Choice: "po" },
        { Key: "header:incoterms", Choice: "po" },
      ])
    );
    expect(input("Customer PO number").value).not.toBe("");
  });

  it("links after an upload at the version the upload saved", async () => {
    reviewForTest = review();
    attachMock.mockResolvedValue({
      ID: 4,
      UUID: "doc-493",
      PurchaseOrderID: 41,
      Version: 1,
      Active: true,
    });
    await render();
    type("Customer PO number", "UPLOAD-THEN-LINK");
    await flush();
    chooseFile();
    await flush();

    expect(createMock).toHaveBeenCalledTimes(1);
    expect(updateMock).toHaveBeenCalledTimes(1);
    const uploadSave = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    expect(uploadSave.ExpectedVersion).toBe(1);
    expect(uploadSave.PONumber).toBe("UPLOAD-THEN-LINK");
    expect(attachMock).toHaveBeenCalledTimes(1);

    // The link preview persists the working copy against the version the upload
    // advanced to, not the transient version.
    await openReview();
    expect(updateMock).toHaveBeenCalledTimes(2);
    const linkSave = updateMock.mock.calls[1][0] as PurchaseOrderUpdateInput;
    expect(linkSave.ID).toBe(41);
    expect(linkSave.ExpectedVersion).toBe(2);
    expect(linkSave.PONumber).toBe("UPLOAD-THEN-LINK");
    expect(previewMock).toHaveBeenCalledWith(41, 12, 0);
  });

  it("links after a save at the version the save returned (#584)", async () => {
    reviewForTest = review();
    await render(undefined, createdRow({ Version: 4 }));

    // Save the open draft: the server returns the next version.
    act(() => button("Save draft").click());
    await flush();
    expect(updateMock).toHaveBeenCalledTimes(1);
    const save = updateMock.mock.calls[0][0] as PurchaseOrderUpdateInput;
    expect(save.ID).toBe(41);
    expect(save.ExpectedVersion).toBe(4);

    // The link's pre-save must write against the version the save produced,
    // not the one the editor opened with.
    await openReview();
    expect(updateMock).toHaveBeenCalledTimes(2);
    const linkSave = updateMock.mock.calls[1][0] as PurchaseOrderUpdateInput;
    expect(linkSave.ID).toBe(41);
    expect(linkSave.ExpectedVersion).toBe(5);
    expect(previewMock).toHaveBeenCalledWith(41, 12, 0);
  });

  it("blocks a currency mismatch without conversion and explains the remedy", async () => {
    reviewForTest = review({ Source: source({ Currency: "USD" }) });
    await render();
    await openReview();

    expect(document.body.textContent).toContain(
      "The purchase order is in IDR but its quotation revision is in USD."
    );
    expect(document.body.textContent).toContain("Source basis");
    expect(linkMock).not.toHaveBeenCalled();
  });

  it("keeps the draft tax basis automatically when the bases differ", async () => {
    reviewForTest = review({
      Source: source({ Lines: [] }),
      SourceTax: {
        TaxZone: "NonFTZ",
        TaxType: "Goods",
        ServiceCategory: "",
        TaxCategory: "",
        PpnBps: 1100,
      },
      TaxChoiceRequired: true,
    });
    await render();
    await openReview();

    // There is no second question in the inline model: the PO-owned basis is
    // kept, exactly as the old default did, and the source can still be
    // adopted on a later link through the source-basis card.
    expect(linkMock).toHaveBeenCalledTimes(1);
    const linkInput = linkMock.mock.calls[0][0] as PurchaseOrderDraftLinkInput;
    expect(linkInput.TaxChoice).toBe("keep");
  });

  it("edits the currency and retries the link after the mismatch", async () => {
    reviewForTest = review({ Source: source({ Currency: "USD" }) });
    await render();
    await openReview();

    // The editor stays open on the mismatch, so the currency is editable
    // without leaving the page. No amount is converted; the same amounts are
    // relabelled USD.
    chooseSelect("Currency", "USD");
    await flush();
    act(() => button("Link a Quotation").click());
    await flush();
    await flush();

    const preSave = updateMock.mock.calls[1][0] as PurchaseOrderUpdateInput;
    expect(preSave.Currency).toBe("USD");
    expect(linkMock).toHaveBeenCalledTimes(1);
    const linked = document.querySelector('[data-testid="linked"]')?.textContent;
    expect(linked).toBe("1502260001:0");
  });

  it("links a formerly-linked draft immediately", async () => {
    reviewForTest = review();
    await render(
      blank({
        ID: 7,
        UUID: "former-497",
        Version: 2,
        SourceState: "former",
        SourceMissing: true,
        PrintedTotalsKnown: true,
        QuotationID: 0,
        QuoteNumber: "1502260099",
        SourceRevision: 1,
        CustomerID: 2,
        CustomerName: "PT. Former",
      })
    );

    expect(document.body.textContent).toContain(
      "The previous source was removed."
    );
    await openReview();
    expect(previewMock).toHaveBeenCalledWith(7, 12, 0);

    expect(linkMock).toHaveBeenCalledTimes(1);
    const linkInput = linkMock.mock.calls[0][0] as PurchaseOrderDraftLinkInput;
    expect(linkInput.ID).toBe(7);
    expect(linkInput.TargetQuotationID).toBe(12);
    expect(linkInput.AcknowledgedMissing).toEqual(["1"]);
    expect(document.querySelector('[data-testid="linked"]')?.textContent).toBe(
      "1502260001:0"
    );
    expect(document.body.textContent).toContain("Linked source");
  });

  it("edits a former draft's currency and basis before relinking", async () => {
    reviewForTest = review({ Source: source({ Currency: "USD" }) });
    await render(
      blank({
        ID: 7,
        UUID: "former-493",
        Version: 2,
        SourceState: "former",
        SourceMissing: true,
        PrintedTotalsKnown: true,
        QuotationID: 0,
        QuoteNumber: "1502260099",
        SourceRevision: 1,
        CustomerID: 2,
        CustomerName: "PT. Former",
        Currency: "IDR",
        TaxZone: "FTZ",
        TaxType: "Goods",
        ServiceCategory: "",
      })
    );

    // A formerly-linked draft is before a new link, so the basis card is
    // editable and says the former source is only a reference.
    const editorText = document.body.textContent ?? "";
    expect(editorText).toContain("Source basis");
    expect(editorText).toContain("former quotation remains the reference");

    await openReview();
    expect(linkMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("quotation revision is in USD");

    chooseSelect("Currency", "USD");
    await flush();
    act(() => button("Link a Quotation").click());
    await flush();
    await flush();

    expect(linkMock).toHaveBeenCalledTimes(1);
    const preSave = updateMock.mock.calls[
      updateMock.mock.calls.length - 1
    ][0] as PurchaseOrderUpdateInput;
    expect(preSave.Currency).toBe("USD");
    expect(preSave.TaxZone).toBe("FTZ");
    expect(document.querySelector('[data-testid="linked"]')?.textContent).toBe(
      "1502260001:0"
    );
  });

  it("validates the working copy before creating a stored draft", async () => {
    reviewForTest = review();
    await render();
    type("Received date", "");
    await flush();

    act(() => button("Link a Quotation").click());
    await flush();
    await flush();

    // An incomplete transient draft must not leave a blank stored row behind:
    // the builder refuses it before ensurePersisted can create one.
    expect(createMock).not.toHaveBeenCalled();
    expect(previewMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("Received date must be YYYY-MM-DD.");
  });

});
