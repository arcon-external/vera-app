// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import {
  blankPurchaseOrderDraft,
  PURCHASE_ORDER_SOURCE_REQUIRED_COPY,
  type PurchaseOrderDTO,
} from "../purchaseOrder";
import { usePurchaseOrderWorkspace } from "../poWorkspace";
import { PurchaseOrderEditorPage } from "./PurchaseOrderEditor";
import { PurchaseOrderWorkspacePage } from "./PurchaseOrderWorkspace";

// Issue #655 retires the #496 transient source-less draft. The Editor tab opens
// on the quotation entry page, the shared selector creates the one stored row,
// and a source-less editor cannot create a row even if a stale caller mounts
// one. These replace the former first-draft suite, whose subject was the
// retired path itself.

const { createMock, updateMock, documentStateMock } = vi.hoisted(() => ({
  createMock: vi.fn(),
  updateMock: vi.fn(),
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
  AttachPurchaseOrderDocument: vi.fn(),
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

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  Element.prototype.scrollIntoView = () => undefined;
  createMock.mockReset();
  updateMock.mockReset();
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

describe("the Editor tab offers no source-less draft (#655)", () => {
  it("shows the quotation entry page and creates nothing on an untouched visit", async () => {
    let addClicks = 0;
    function EntryHarness() {
      const workspace = usePurchaseOrderWorkspace();
      return (
        <PurchaseOrderWorkspacePage
          workspace={workspace}
          currency={IDR}
          currencyOptions={[IDR]}
          uomOptions={UOMS}
          incotermOptions={INCOTERMS}
          tab="editor"
          onAdd={() => {
            addClicks += 1;
          }}
          onOpen={() => undefined}
          isAdmin={false}
          onRepairSource={() => undefined}
          onSaved={() => undefined}
          onBack={() => undefined}
          onCancel={() => undefined}
          onDirtyChange={() => undefined}
        />
      );
    }
    await act(async () => {
      root.render(<EntryHarness />);
    });
    await flush();

    // The entry page, not a blank manual-entry form.
    expect(document.getElementById("po-entry-heading")?.textContent).toBe(
      "New purchase order"
    );
    expect(document.body.textContent).toContain(
      PURCHASE_ORDER_SOURCE_REQUIRED_COPY
    );
    expect(
      document.querySelector('input[aria-label="Customer PO number"]')
    ).toBeNull();
    expect(createMock).not.toHaveBeenCalled();
    expect(updateMock).not.toHaveBeenCalled();

    // The one creation path is the shared selector.
    act(() => button("Add Purchase Order from a Quotation").click());
    expect(addClicks).toBe(1);
  });

  it("refuses a mounted source-less editor instead of creating a row", async () => {
    await act(async () => {
      root.render(
        <PurchaseOrderEditorPage
          order={blank()}
          currency={IDR}
          uomOptions={UOMS}
          incotermOptions={INCOTERMS}
          onSaved={() => undefined}
          onCancel={() => undefined}
          onDirtyChange={() => undefined}
        />
      );
    });
    await flush();

    act(() => button("Save draft").click());
    await flush();

    // The refusal is the entry page's own sentence, and nothing reached the
    // blank-create or update bindings.
    expect(document.body.textContent).toContain(
      PURCHASE_ORDER_SOURCE_REQUIRED_COPY
    );
    expect(createMock).not.toHaveBeenCalled();
    expect(updateMock).not.toHaveBeenCalled();
  });
});
