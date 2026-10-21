// @vitest-environment jsdom
import { act, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import { usePurchaseOrderLeaveGuard } from "../poLeaveGuard";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import type { PurchaseOrderReviewDTO } from "../poFinalize";
import { usePurchaseOrderWorkspace } from "../poWorkspace";
import { PurchaseOrderDiscardPrompt } from "./PurchaseOrderEditor";
import { PurchaseOrderWorkspacePage } from "./PurchaseOrderWorkspace";

// Issue #473 wave 3: Cancel from the saved-record correction transition must
// show the saved view, not the stale draft editor.
//
// The regression renders the real page composition -- the workspace
// controller, the real archive/saved-view/editor branch, App's real leave
// guard and its real discard prompt -- and never replaces `onCancel` with a
// spy. The two conflict paths are the ones the Judge probed: a draft write
// whose conflict names `saved`, and Overwrite racing into `po_not_draft`.
//
// Bindings are replaced at their module boundary, which is what the desktop
// provides; everything above it is production code.

const { getMock, updateMock, correctMock, documentStateMock, repairMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  updateMock: vi.fn(),
  correctMock: vi.fn(),
  documentStateMock: vi.fn(),
  repairMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  GetPurchaseOrder: (...args: unknown[]) => getMock(...args),
  UpdatePurchaseOrderDraft: (...args: unknown[]) => updateMock(...args),
  CorrectPurchaseOrder: (...args: unknown[]) => correctMock(...args),
  RepairPurchaseOrderSource: (...args: unknown[]) => repairMock(...args),
  FinalizePurchaseOrder: vi.fn(),
  GetPurchaseOrderReview: vi.fn(),
  GetPurchaseOrderHistory: vi.fn(async () => []),
  AssignPurchaseOrderConfirmationNumber: vi.fn(),
  GetPurchaseOrderDocumentState: (...args: unknown[]) => documentStateMock(...args),
  AttachPurchaseOrderDocument: vi.fn(),
  GetActivePurchaseOrderDocumentContent: vi.fn(),
  GetPurchaseOrderDocumentVersionContent: vi.fn(),
  RemovePurchaseOrderDocument: vi.fn(),
  ListPurchaseOrders: vi.fn(async () => ({
    Orders: [],
    Total: 0,
    Page: 1,
    PageSize: 15,
    Customers: [],
  })),
}));

vi.mock("@backend/models", () => ({
  backend: {
    PurchaseOrderCorrectionInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderUpdateInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderItemInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderFinalizeInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderSourceRepairInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
  },
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

function order(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    PODate: "2026-10-01",
    RequestedDeliveryDate: null,
    UUID: "uuid-473",
    State: "saved",
    QuoteNumber: "1502260012",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing",
    DeliveryAddress: "Jl. Old Delivery",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "+62 21 555",
    PONumber: "PO-473",
    ReceivedDate: "2026-10-02",
    Version: 4,
    Incoterms: "CIF",
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-02T10:00:00Z",
    UpdatedAt: "2026-10-02T10:00:00Z",
    Items: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Widget",
        QuantityScaled: 20000,
        UoM: "Pc",
        UnitPriceMinor: 10000,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 20000,
        DiscountMinor: 0,
        NetMinor: 20000,
      },
    ],
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    CustomerID: 2,
    PaymentTerms: "30 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 20000,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 20000,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    ConfirmationNumber: "2302260001",
    ConfirmationWarn: false,
    ...overrides,
  };
}

const CONFLICT = {
  CurrentState: "saved",
  CurrentVersion: 5,
  UpdatedBy: 9,
  UpdatedByName: "Budi",
  UpdatedAt: "2026-10-21T10:15:00+07:00",
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  getMock.mockReset();
  updateMock.mockReset();
  correctMock.mockReset();
  documentStateMock.mockReset();
  documentStateMock.mockResolvedValue({ Document: null, Versions: [] });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (!found) throw new Error(`button ${label} not found`);
  return found;
}

function queryButton(label: string): HTMLButtonElement | null {
  return document.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
}

function typeInto(label: string, value: string) {
  const found = document.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    `input[aria-label="${label}"], textarea[aria-label="${label}"]`
  );
  if (!found) throw new Error(`field ${label} not found`);
  const prototype =
    found instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  act(() => {
    Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(found, value);
    found.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

/**
 * The production composition: App's PO branch (the extracted page), the
 * controller that owns its transitions, and App's real leave guard with its
 * real discard prompt. Only the desktop bindings are mocked.
 */
function Harness({
  initialId,
  onRefreshError,
  isAdmin = false,
  onRepairSource = () => undefined,
  repairReview,
}: {
  initialId: number;
  onRefreshError?: (error: unknown) => void;
  isAdmin?: boolean;
  onRepairSource?: () => void;
  repairReview?: PurchaseOrderReviewDTO;
}) {
  const poLeave = usePurchaseOrderLeaveGuard("po", () => undefined);
  const workspace = usePurchaseOrderWorkspace({ onRefreshError });
  useEffect(() => {
    // App's open path: the row id is fetched, then the controller opens it.
    void getMock(initialId).then((record: PurchaseOrderDTO) => {
      workspace.open(record);
      // App's repair path: the candidate review is read after the record is
      // open, then the workspace replaces the saved view with the panel.
      if (repairReview !== undefined) workspace.repair(repairReview);
    });
    // The open path runs once, as it does when a row is clicked.
  }, []);
  return (
    <>
      <PurchaseOrderWorkspacePage
        workspace={workspace}
        currency={USD}
        uomOptions={[{ value: "Pc", label: "Pc" }]}
        incotermOptions={[{ value: "CIF", label: "CIF" }]}
        onAdd={() => undefined}
        isAdmin={isAdmin}
        onRepairSource={onRepairSource}
        onOpen={() => undefined}
        onBack={() => {
          poLeave.setDirty(false);
          workspace.close();
        }}
        onCancel={() => workspace.cancel(poLeave.request, poLeave.setDirty)}
        onDirtyChange={poLeave.setDirty}
      />
      {poLeave.pendingPage !== null ? (
        <PurchaseOrderDiscardPrompt
          onKeepEditing={poLeave.keepEditing}
          onDiscard={poLeave.discard}
          returnFocus={poLeave.pendingReturnFocus}
        />
      ) : null}
    </>
  );
}

/** Every indication that the read-only saved record is the page on screen. */
function expectSavedView() {
  expect(document.body.textContent).toContain(
    "Finalized entry data for later documents"
  );
  expect(queryButton("Correct purchase order")).not.toBeNull();
  expect(queryButton("Save draft")).toBeNull();
  expect(queryButton("Save correction")).toBeNull();
  expect(queryButton("Cancel correction")).toBeNull();
}

/** Cancel, keep the discard guard real, and let its Discard run. */
async function cancelCorrectionAndDiscard() {
  await act(async () => {
    button("Cancel correction").click();
  });
  expect(document.body.textContent).toContain("Discard unsaved changes?");
  await act(async () => {
    button("Discard changes").click();
  });
}

describe("cancel from a saved-record correction (issue #473)", () => {
  it("shows the saved view when the conflict named a saved record", async () => {
    updateMock.mockResolvedValue({
      Order: null,
      Conflict: { ...CONFLICT, CurrentState: "saved" },
    });
    getMock.mockResolvedValueOnce(
      order({ State: "draft", Version: 3, DeliveryAddress: "Jl. Old Delivery" })
    );
    getMock.mockResolvedValueOnce(
      order({ State: "saved", Version: 5, DeliveryAddress: "Jl. Server Record 9" })
    );

    await act(async () => {
      root.render(<Harness initialId={7} />);
    });
    expect(queryButton("Save draft")).not.toBeNull();

    typeInto("Delivery address", "Jl. Working Copy 9");
    await act(async () => {
      button("Save draft").click();
    });
    expect(document.body.textContent).toContain("Correct saved record");

    // The transition keeps the working copy and adopts the server's version.
    await act(async () => {
      button("Correct saved record").click();
    });
    expect(queryButton("Save correction")).not.toBeNull();
    expect(
      document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Delivery address"]')
        ?.value
    ).toBe("Jl. Working Copy 9");

    await cancelCorrectionAndDiscard();

    // The saved view, loaded from the server's record: not the stale draft
    // editor, and not the working copy Cancel discarded.
    expectSavedView();
    expect(getMock).toHaveBeenCalledTimes(2);
    expect(getMock).toHaveBeenLastCalledWith(7);
    expect(document.body.textContent).toContain("Jl. Server Record 9");
    expect(document.body.textContent).not.toContain("Jl. Working Copy 9");
  });

  it("shows the saved view when Overwrite raced into po_not_draft", async () => {
    updateMock
      .mockResolvedValueOnce({
        Order: null,
        Conflict: { ...CONFLICT, CurrentState: "draft" },
      })
      .mockResolvedValueOnce({ Order: null, Conflict: null, SavedRecord: true });
    getMock.mockResolvedValueOnce(
      order({ State: "draft", Version: 4, DeliveryAddress: "Jl. Old Delivery" })
    );
    getMock.mockResolvedValueOnce(
      order({ State: "saved", Version: 5, DeliveryAddress: "Jl. Server Record 9" })
    );

    await act(async () => {
      root.render(<Harness initialId={7} />);
    });
    expect(queryButton("Save draft")).not.toBeNull();

    typeInto("Delivery address", "Jl. Working Copy 9");
    await act(async () => {
      button("Save draft").click();
    });
    expect(queryButton("Overwrite")).not.toBeNull();

    // The draft was finalized between load and Overwrite; the refusal is the
    // SavedRecord answer, and the correction transition keeps the work.
    await act(async () => {
      button("Overwrite").click();
    });
    expect(document.body.textContent).toContain("cannot be overwritten as a draft");
    expect(queryButton("Correct saved record")).not.toBeNull();

    await act(async () => {
      button("Correct saved record").click();
    });
    expect(queryButton("Save correction")).not.toBeNull();
    expect(
      document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Delivery address"]')
        ?.value
    ).toBe("Jl. Working Copy 9");

    await cancelCorrectionAndDiscard();

    expectSavedView();
    expect(getMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("Jl. Server Record 9");
    expect(document.body.textContent).not.toContain("Jl. Working Copy 9");
  });

  it("reloads a corrected saved record instead of showing the loaded snapshot", async () => {
    getMock.mockResolvedValueOnce(
      order({ State: "saved", Version: 4, DeliveryAddress: "Jl. Loaded Snapshot 1" })
    );
    getMock.mockResolvedValueOnce(
      order({ State: "saved", Version: 6, DeliveryAddress: "Jl. Someone Else 6" })
    );

    await act(async () => {
      root.render(<Harness initialId={7} />);
    });
    expectSavedView();
    expect(document.body.textContent).toContain("Jl. Loaded Snapshot 1");

    await act(async () => {
      button("Correct purchase order").click();
    });
    typeInto("Delivery address", "Jl. Working Copy 9");
    await cancelCorrectionAndDiscard();

    expectSavedView();
    // The other writer's version is what the saved view must show.
    expect(document.body.textContent).toContain("Jl. Someone Else 6");
    expect(document.body.textContent).not.toContain("Jl. Loaded Snapshot 1");
    expect(document.body.textContent).not.toContain("Jl. Working Copy 9");
    expect(getMock).toHaveBeenCalledTimes(2);
  });

  it("returns to the archive instead of the stale editor when the reload fails", async () => {
    updateMock.mockResolvedValue({
      Order: null,
      Conflict: { ...CONFLICT, CurrentState: "saved" },
    });
    getMock.mockResolvedValueOnce(
      order({ State: "draft", Version: 3, DeliveryAddress: "Jl. Old Delivery" })
    );
    getMock.mockRejectedValueOnce(new Error("record unavailable"));
    const onRefreshError = vi.fn();

    await act(async () => {
      root.render(<Harness initialId={7} onRefreshError={onRefreshError} />);
    });
    typeInto("Delivery address", "Jl. Working Copy 9");
    await act(async () => {
      button("Save draft").click();
    });
    await act(async () => {
      button("Correct saved record").click();
    });
    await cancelCorrectionAndDiscard();

    // There is no truthful record to show, so the archive is shown rather than
    // the stale correction editor.
    expect(onRefreshError).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("Purchase orders");
    expect(queryButton("Save correction")).toBeNull();
    expect(queryButton("Correct purchase order")).toBeNull();
  });
});

describe("source-link repair from the saved view (issue #475)", () => {
  async function flush(): Promise<void> {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }

  function repairReview(): PurchaseOrderReviewDTO {
    const record = order();
    return {
      Order: record,
      Source: {
        QuotationID: 19,
        RevisionNumber: 1,
        QuoteNumber: "1502260019",
        CustomerID: record.CustomerID,
        CustomerName: record.CustomerName,
        Currency: "USD",
        PaymentTerms: "30 days",
        Incoterms: "CIF",
        DiscountPercent: 0,
        Lines: [
          {
            QuotationItemNo: 1,
            QuotationSubNo: null,
            LineLabel: "1",
            Description: "Widget",
            UoM: "Pc",
            QuantityScaled: 20000,
            UnitPriceMinor: 10000,
            DiscountEnabled: false,
            DiscountPercent: 0,
          },
        ],
        PrintedSubtotalMinor: 20000,
        PrintedDiscountMinor: 0,
        PrintedPpnMinor: 0,
        PrintedGrandTotalMinor: 20000,
      },
      DocumentState: "none",
    };
  }

  it("shows the action only to an admin", async () => {
    getMock.mockResolvedValue(order());
    await act(async () => {
      root.render(<Harness initialId={7} isAdmin={false} />);
    });
    await flush();
    expectSavedView();
    expect(queryButton("Repair source link")).toBeNull();

    await act(async () => {
      root.render(<Harness initialId={7} isAdmin />);
    });
    await flush();
    expect(queryButton("Repair source link")).not.toBeNull();
  });

  it("opens the repair view and Cancel returns to the saved record", async () => {
    getMock.mockResolvedValue(order());
    const opened: number[] = [];
    await act(async () => {
      root.render(
        <Harness
          initialId={7}
          isAdmin
          onRepairSource={() => opened.push(1)}
          repairReview={repairReview()}
        />
      );
    });
    await flush();
    expect(document.body.textContent).toContain("Source link change");
    expect(document.body.textContent).toContain("1502260019");

    await act(async () => {
      button("Back without repairing").click();
    });
    await flush();
    expectSavedView();
    expect(document.body.textContent).toContain("1502260012");
  });
});

describe("source-missing record repair from the saved view (issue #481)", () => {
  async function flush(): Promise<void> {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }

  function missingOrder(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
    return order({
      SourceMissing: true,
      QuoteNumber: "1502260012",
      ConfirmationNumber: "2302260001",
      ...overrides,
    });
  }

  function repairReview(record: PurchaseOrderDTO): PurchaseOrderReviewDTO {
    return {
      Order: record,
      Source: {
        QuotationID: 19,
        RevisionNumber: 1,
        QuoteNumber: "1502260019",
        CustomerID: record.CustomerID,
        CustomerName: record.CustomerName,
        Currency: "USD",
        PaymentTerms: record.PaymentTerms,
        Incoterms: record.Incoterms,
        DiscountPercent: 0,
        Lines: [
          {
            QuotationItemNo: 1,
            QuotationSubNo: null,
            LineLabel: "1",
            Description: "Widget",
            UoM: "Pc",
            QuantityScaled: 20000,
            UnitPriceMinor: 10000,
            DiscountEnabled: false,
            DiscountPercent: 0,
          },
        ],
        PrintedSubtotalMinor: 20000,
        PrintedDiscountMinor: 0,
        PrintedPpnMinor: 0,
        PrintedGrandTotalMinor: 20000,
      },
      DocumentState: "none",
    };
  }

  it("shows the banner and Link Quotation only to an admin, and no action to staff", async () => {
    getMock.mockResolvedValue(missingOrder());
    await act(async () => {
      root.render(<Harness initialId={7} isAdmin={false} />);
    });
    await flush();
    expect(document.body.textContent).toContain(
      "The quotation this purchase order followed was permanently deleted"
    );
    expect(queryButton("Link Quotation")).toBeNull();
    expect(queryButton("Repair source link")).toBeNull();
    expect(document.body.textContent).toContain(
      "An administrator can link a quotation to repair the source."
    );

    await act(async () => {
      root.render(<Harness initialId={7} isAdmin />);
    });
    await flush();
    expect(queryButton("Link Quotation")).not.toBeNull();
  });

  it("keeps the current values, records the reason and returns to a linked saved view", async () => {
    const record = missingOrder({ DeliveryAddress: "Jl. Deleted Source 1" });
    getMock.mockResolvedValue(record);
    repairMock.mockResolvedValue({
      Order: missingOrder({
        SourceMissing: false,
        QuoteNumber: "1502260019",
        SourceRevision: 1,
        DeliveryAddress: "Jl. Deleted Source 1",
        Version: 5,
      }),
      Conflict: null,
    });

    await act(async () => {
      root.render(
        <Harness initialId={7} isAdmin repairReview={repairReview(record)} />
      );
    });
    await flush();
    expect(document.body.textContent).toContain("Former source (deleted)");

    // The review starts from the record's own values with nothing preselected,
    // and continuing is allowed because every compared value matches.
    await act(async () => {
      button("Continue to repair").click();
    });
    typeInto("Reason for repairing the source link", "recover the deleted source");
    await act(async () => {
      button("Link Quotation").click();
    });
    await flush();

    expect(repairMock).toHaveBeenCalledTimes(1);
    const input = repairMock.mock.calls[0][0] as Record<string, unknown>;
    expect(input.TargetQuotationID).toBe(19);
    expect(input.TargetRevisionNumber).toBe(1);
    expect(input.ExpectedVersion).toBe(record.Version);
    expect(input.Reason).toBe("recover the deleted source");
    expect(input.DeliveryAddress).toBe("Jl. Deleted Source 1");

    // The server's repaired record replaces the saved view: the missing-source
    // banner is gone and the new link is what the page names.
    expectSavedView();
    expect(document.body.textContent).not.toContain(
      "The quotation this purchase order followed was permanently deleted"
    );
    expect(document.body.textContent).toContain("quotation 1502260019 revision 1");
  });
});
