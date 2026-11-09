import { describe, expect, it } from "vitest";
import type { PurchaseOrderHistoryEntry } from "./poCorrection";
import {
  normalizeReconciliationHistory,
  savedHistoryRow,
  savedReconciliationContext,
  savedReconciliationCounts,
  savedReconciliationView,
  savedRepairEntry,
  savedSourceDeletedLabel,
  savedSourceLabel,
  savedTriggerState,
  type ReconciliationHistoryRecord,
} from "./poReconciliationHistory";
import type { ReconciliationItem, ReconciliationSession } from "./poReconciliation";

// #518: the saved reconciliation read, as pure rules.
//
// The wire normalisation, the current/history split, the saved-only counts and
// every sentence the saved surface shows are pinned here. The DOM suite proves
// the surface renders them; this suite proves the decisions.

function session(overrides: Partial<ReconciliationSession> = {}): ReconciliationSession {
  return {
    ID: 1,
    PurchaseOrderID: 7,
    SourceQuotationID: 12,
    SourceRevision: 2,
    POVersion: 4,
    Active: true,
    ReadOnly: true,
    CreatedBy: 3,
    CreatedAt: "2026-10-21T09:00:00Z",
    UpdatedAt: "2026-10-21T09:05:00Z",
    LineIdentities: [],
    Items: [],
    ...overrides,
  };
}

function item(overrides: Partial<ReconciliationItem> = {}): ReconciliationItem {
  return {
    Key: "header:paymentTerms",
    Field: "header:paymentTerms",
    Kind: "difference",
    LineKey: null,
    SourceLineRef: null,
    Label: "Payment terms",
    POValue: "30 days",
    SourceValue: "14 days",
    Status: "resolved",
    Resolution: "kept_po",
    ActorID: 3,
    UpdatedAt: "2026-10-21T09:05:00Z",
    History: [],
    ...overrides,
  };
}

function record(
  overrides: Partial<ReconciliationHistoryRecord> = {}
): ReconciliationHistoryRecord {
  return {
    Generation: 1,
    SourceAvailable: true,
    RepairCorrectionID: null,
    Session: session(),
    ...overrides,
  };
}

const ORDER = { QuotationID: 12, QuoteNumber: "1502260044", SourceRevision: 2 };

describe("normalizeReconciliationHistory", () => {
  it("keeps every generation, newest first, and never depends on array order", () => {
    const read = normalizeReconciliationHistory({
      PurchaseOrderID: 7,
      POVersion: 4,
      Sessions: [
        { Generation: 0, SourceAvailable: true, Session: session({ ID: 9, Active: false }) },
        { Generation: 2, SourceAvailable: true, Session: session({ ID: 30 }) },
        { Generation: 1, SourceAvailable: true, Session: session({ ID: 20, Active: false }) },
      ],
    });
    expect(read.Sessions.map((entry) => entry.Generation)).toEqual([2, 1, 0]);
    expect(read.POVersion).toBe(4);
  });

  it("normalizes absent values to contract nulls and never returns a null list", () => {
    const read = normalizeReconciliationHistory({
      PurchaseOrderID: 7,
      POVersion: 4,
      Sessions: [
        {
          Generation: 1,
          SourceAvailable: true,
          Session: session({
            LineIdentities: [],
            Items: [
              item({
                LineKey: undefined,
                SourceLineRef: undefined,
                Resolution: undefined,
                History: [
                  {
                    Kind: "difference",
                    POValue: "a",
                    SourceValue: "b",
                    Status: "resolved",
                    ActorID: 3,
                    At: "2026-10-21T09:00:00Z",
                  },
                ],
              }),
            ],
          }),
        },
      ],
    });
    const first = read.Sessions[0];
    expect(first?.RepairCorrectionID).toBeNull();
    const normalized = first?.Session.Items[0];
    expect(normalized?.LineKey).toBeNull();
    expect(normalized?.SourceLineRef).toBeNull();
    expect(normalized?.Resolution).toBeNull();
    expect(normalized?.History[0]?.Resolution).toBeNull();

    const empty = normalizeReconciliationHistory({});
    expect(empty.Sessions).toEqual([]);
    expect(empty.PurchaseOrderID).toBe(0);
  });
});

describe("savedReconciliationView", () => {
  it("is none for a record with no sessions, not an error", () => {
    expect(
      savedReconciliationView(
        { PurchaseOrderID: 7, POVersion: 4, Sessions: [] },
        4
      )
    ).toEqual({ kind: "none" });
  });

  it("is stale when the outer version is not the displayed record version", () => {
    const read = normalizeReconciliationHistory({
      PurchaseOrderID: 7,
      POVersion: 3,
      Sessions: [record()],
    });
    expect(savedReconciliationView(read, 4)).toEqual({ kind: "stale" });
  });

  it("takes the stored Active generation, not the highest number", () => {
    const read = normalizeReconciliationHistory({
      PurchaseOrderID: 7,
      POVersion: 4,
      Sessions: [
        record({ Generation: 2, Session: session({ ID: 30, Active: false }) }),
        record({ Generation: 1, Session: session({ ID: 20, Active: true }) }),
      ],
    });
    const view = savedReconciliationView(read, 4);
    expect(view.kind).toBe("ready");
    if (view.kind !== "ready") return;
    expect(view.current.Generation).toBe(1);
    expect(view.previous.map((entry) => entry.Generation)).toEqual([2]);
  });

  it("uses the stored audit POVersion, not the outer one, for the split", () => {
    // A correction advanced the parent version; the nested session keeps the
    // version it recorded. The split must still be ready.
    const read = normalizeReconciliationHistory({
      PurchaseOrderID: 7,
      POVersion: 6,
      Sessions: [record({ Session: session({ POVersion: 4 }) })],
    });
    expect(savedReconciliationView(read, 6).kind).toBe("ready");
  });
});

describe("saved labels", () => {
  it("counts only the two saved filters the surface offers", () => {
    const counts = savedReconciliationCounts(
      record({
        Session: session({
          Items: [
            item(),
            item({ Key: "link:taxBasis", Kind: "tax_basis", Status: "resolved" }),
            item({ Key: "line:1:quantity", Status: "unresolved", Resolution: null }),
          ],
        }),
      })
    );
    expect(counts).toEqual({ all: 3, resolved: 2, unresolved: 1 });
  });

  it("names the current and previous contexts exactly", () => {
    const entry = record({ Generation: 3, Session: session({ SourceRevision: 2 }) });
    expect(savedReconciliationContext(entry, true)).toBe(
      "Saved session · revision 2 · generation 3"
    );
    expect(savedReconciliationContext(entry, false)).toBe(
      "Previous saved session · revision 2 · generation 3"
    );
  });

  it("uses the live quotation number only for the generation that still matches it", () => {
    expect(savedSourceLabel(record(), ORDER)).toBe("QT-1502260044 · revision 2");
    const older = record({
      Generation: 0,
      Session: session({ SourceRevision: 1, SourceQuotationID: 11 }),
    });
    expect(savedSourceLabel(older, ORDER)).toBe("Quotation 11 · revision 1");
  });

  it("names a deleted source with its stored identifier and revision", () => {
    const gone = record({
      SourceAvailable: false,
      Session: session({ SourceQuotationID: 11, SourceRevision: 1 }),
    });
    expect(savedSourceDeletedLabel(gone)).toBe(
      "Source deleted · Quotation 11 · revision 1"
    );
    // A deleted source is availability context, not an empty session: the
    // stored items still count and render.
    expect(savedReconciliationCounts(gone).all).toBe(0);
  });
});

describe("saved history rows", () => {
  const repair = (id: number): PurchaseOrderHistoryEntry =>
    ({
      ID: id,
      UUID: `corr-${id}`,
      Kind: "source_repair",
      State: "saved",
      Reason: "wrong revision",
      ActorName: "Budi",
      CreatedAt: "2026-10-30T10:04:00Z",
      Repair: {
        PreviousQuoteNumber: "1502260044",
        PreviousSourceRevision: 7,
        NewQuoteNumber: "1502260051",
        NewSourceRevision: 0,
      },
    }) as PurchaseOrderHistoryEntry;

  it("joins the repair correction by ID and names the old/new source", () => {
    const entry = record({
      Generation: 2,
      RepairCorrectionID: 5,
      Session: session({ SourceRevision: 1 }),
    });
    expect(savedRepairEntry(entry, [repair(5)])?.ID).toBe(5);
    const row = savedHistoryRow(entry, [repair(5)]);
    expect(row.primary).toBe("Revision 1 · 0 resolved · 2026-10-21 09:00 UTC");
    expect(row.secondary).toBe(
      "Source repaired · Quotation 1502260044 rev 7 → Quotation 1502260051 rev 0 · generation 2"
    );
  });

  it("never invents a repair it cannot resolve", () => {
    const entry = record({ Generation: 4, RepairCorrectionID: 99 });
    expect(savedRepairEntry(entry, [repair(5)])).toBeNull();
    expect(savedHistoryRow(entry, [repair(5)]).secondary).toBe(
      "Source repaired · details in correction history · generation 4"
    );
  });

  it("names a deleted source and, otherwise, the numeric source", () => {
    expect(
      savedHistoryRow(record({ Generation: 0, SourceAvailable: false }), []).secondary
    ).toBe("Source deleted · Quotation 12 · revision 2 · generation 0");
    expect(savedHistoryRow(record({ Generation: 0 }), []).secondary).toBe(
      "Quotation 12 · generation 0"
    );
  });
});

describe("savedTriggerState", () => {
  it("names each trigger state exactly", () => {
    expect(savedTriggerState("loading")).toEqual({
      text: "Reconciliation · loading",
      pill: false,
      ariaLabel: "Reconciliation, loading. Open panel.",
    });
    expect(savedTriggerState("error").ariaLabel).toBe(
      "Reconciliation, failed to load. Open panel."
    );
    expect(savedTriggerState("stale").ariaLabel).toBe(
      "Reconciliation, failed to load. Open panel."
    );
    expect(savedTriggerState("ready")).toEqual({
      text: "Reconciliation",
      pill: true,
      ariaLabel: "Reconciliation, read-only. Open panel.",
    });
    expect(savedTriggerState("none").ariaLabel).toBe(
      "Reconciliation, read-only. Open panel.",
    );
  });
});
