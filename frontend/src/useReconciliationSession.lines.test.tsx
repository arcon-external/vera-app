// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import {
  useReconciliationSession,
  type UseReconciliationSessionOptions,
} from "./useReconciliationSession";
import type { ReconciliationSession } from "./poReconciliation";
import type { PurchaseOrderSourceLineDTO } from "./poFinalize";
import type { CurrencyInfo } from "./format";

// F2: the hook resolves the first-seen session key for creation, lookup,
// feedback, prefill and focus, explicitly mapping the editor alias. A kept
// id-keyed decision survives an unchanged re-blur with history intact; a
// re-edit back to the source keeps values_now_match without a second notice.

vi.mock("@backend/backend/Quotation", () => ({
  LoadPOReconciliation: vi.fn(),
  SavePOReconciliation: vi.fn(),
}));

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

const NOW = "2026-10-29T00:00:00Z";

const SOURCE: PurchaseOrderSourceLineDTO = {
  QuotationItemNo: 1,
  QuotationSubNo: null,
  LineLabel: "1",
  Description: "Widget",
  UoM: "Pc",
  QuantityScaled: 20000,
  UnitPriceMinor: 10000,
  DiscountEnabled: false,
  DiscountPercent: 0,
};

function keptDescriptionItem(): ReconciliationSession["Items"][number] {
  return {
    Key: "line:id:501:description",
    Field: "line:description",
    Kind: "difference",
    LineKey: "id:501",
    SourceLineRef: "1",
    Label: "Line 1 description",
    POValue: "Customer label",
    SourceValue: "Widget",
    Status: "resolved",
    Resolution: "kept_po",
    ActorID: 7,
    UpdatedAt: NOW,
    History: [
      {
        Kind: "difference",
        SourceLineRef: "1",
        POValue: "Customer label",
        SourceValue: "Widget",
        Status: "resolved",
        Resolution: "kept_po",
        ActorID: 7,
        At: NOW,
      },
    ],
  };
}

function storedSession(): ReconciliationSession {
  return {
    ID: 1,
    PurchaseOrderID: 1,
    SourceQuotationID: 1,
    SourceRevision: 0,
    POVersion: 1,
    Active: true,
    ReadOnly: false,
    CreatedBy: 7,
    CreatedAt: NOW,
    UpdatedAt: NOW,
    LineIdentities: [{ Key: "id:501", ID: 501, Removed: false }],
    Items: [keptDescriptionItem()],
  };
}

function options(
  stored: ReconciliationSession,
  values: { description: string }
): UseReconciliationSessionOptions {
  return {
    transport: {
      load: async () => structuredClone(stored),
      save: async (_poID: string, session: ReconciliationSession) => session,
    },
    orderId: 1,
    sourceState: "live",
    sourceQuotationId: 1,
    sourceRevision: 0,
    poVersion: 1,
    readOnly: false,
    source: { paymentTerms: "", incoterms: "", discountPercent: 0 },
    sourceLines: [SOURCE],
    lines: [
      {
        // Pre-042 style: the row has an id but no carried alias, so the
        // editor alias differs from the recorded session key.
        clientKey: "client:fresh-editor-alias",
        persistedId: 501,
        dtoAlias: null,
        displayNumber: 1,
        quotationItemNo: 1,
        quotationSubNo: null,
        values: {
          description: values.description,
          uom: "Pc",
          quantity: "2",
          unitPrice: "10000",
          discountEnabled: false,
          discountPercent: "0",
        },
      },
    ],
    currency: IDR,
    actor: { id: 7, name: "Reviewer" },
    getFieldValue: () => "",
    applyFieldValue: () => {},
    applyLineFieldValue: () => {},
  };
}

describe("hook first-seen session key (F2)", () => {
  it("re-blurring an unchanged kept id-keyed value retains resolution and history", async () => {
    let api: ReturnType<typeof useReconciliationSession> | undefined;
    // Stable across renders: a fresh options object per render would
    // re-fire the load effect and never settle.
    const stable = options(storedSession(), { description: "Customer label" });
    function Harness() {
      api = useReconciliationSession(stable);
      return null;
    }
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    try {
      await act(async () => {
        root.render(<Harness />);
      });
      expect(api!.status).toBe("ready");
      expect(api!.items).toHaveLength(1);
      await act(async () => {
        api!.commitLine("client:fresh-editor-alias", "description");
      });
      await act(async () => {
        await Promise.resolve();
      });
      expect(api!.items).toHaveLength(1);
      expect(api!.items[0]).toMatchObject({
        Key: "line:id:501:description",
        Status: "resolved",
        Resolution: "kept_po",
      });
      expect(api!.items[0]?.History).toHaveLength(1);
    } finally {
      await act(async () => root.unmount());
      host.remove();
    }
    expect(api).toBeDefined();
    void api;
  });

  it("re-editing a kept value back to the source keeps values_now_match without a second notice", async () => {
    let api: ReturnType<typeof useReconciliationSession> | undefined;
    const stable = options(storedSession(), { description: "Widget" });
    function Harness() {
      api = useReconciliationSession(stable);
      return null;
    }
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    try {
      await act(async () => {
        root.render(<Harness />);
      });
      await act(async () => {
        api!.commitLine("client:fresh-editor-alias", "description");
      });
      await act(async () => {
        await Promise.resolve();
      });
      expect(api!.items).toHaveLength(1);
      expect(api!.items[0]).toMatchObject({
        Status: "resolved",
        Resolution: "values_now_match",
      });
      expect(api!.lineNotices["id:501:description"] ?? null).toBeNull();
    } finally {
      await act(async () => root.unmount());
      host.remove();
    }
  });
});
