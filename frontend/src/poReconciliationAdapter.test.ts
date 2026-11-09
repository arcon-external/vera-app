import { describe, expect, it, vi } from "vitest";
import {
  isReconciliationNotFound,
  loadSession,
  saveSession,
  type ReconciliationTransport,
} from "./poReconciliationAdapter";
import type { ReconciliationSession } from "./poReconciliation";

// Ticket 04: the Wails seam. Component tests drive a fake transport through
// this adapter; production passes the Wails bindings. Every case pins the
// mapping the editor relies on: found normalizes, missing means absent,
// anything else is an error, and a save followed by a load rehydrates.

function session(overrides: Partial<ReconciliationSession> = {}): ReconciliationSession {
  return {
    ID: 3,
    PurchaseOrderID: 7,
    SourceQuotationID: 12,
    SourceRevision: 1,
    POVersion: 4,
    Active: true,
    ReadOnly: false,
    CreatedBy: 7,
    CreatedAt: "2026-10-28T00:00:00Z",
    UpdatedAt: "2026-10-28T00:00:01Z",
    LineIdentities: [],
    Items: [
      {
        Key: "header:paymentTerms",
        Field: "header:paymentTerms",
        Kind: "difference",
        Label: "Payment terms",
        POValue: "50% advance",
        SourceValue: "30 days",
        Status: "unresolved",
        Resolution: null,
        ActorID: 7,
        UpdatedAt: "2026-10-28T00:00:01Z",
        History: [],
      },
    ],
    ...overrides,
  };
}

describe("isReconciliationNotFound", () => {
  it("matches the server's absent-session message in any error shape", () => {
    expect(isReconciliationNotFound("no reconciliation recorded")).toBe(true);
    expect(isReconciliationNotFound(new Error("no reconciliation recorded"))).toBe(
      true
    );
    expect(isReconciliationNotFound({ message: "No Reconciliation Recorded" })).toBe(
      true
    );
  });

  it("rejects every other failure", () => {
    expect(isReconciliationNotFound(new Error("offline"))).toBe(false);
    expect(isReconciliationNotFound("po_not_draft")).toBe(false);
    expect(isReconciliationNotFound(null)).toBe(false);
    expect(isReconciliationNotFound(undefined)).toBe(false);
  });
});

describe("loadSession", () => {
  it("normalizes a found session to contract nulls", async () => {
    const transport: ReconciliationTransport = {
      load: vi.fn(async () => ({
        ...session(),
        Items: [
          {
            ...session().Items[0],
            LineKey: undefined,
            Resolution: undefined,
          },
        ],
      })),
      save: vi.fn(),
    };
    const result = await loadSession(transport, "7");
    expect(result.status).toBe("ready");
    if (result.status !== "ready") throw new Error("expected ready");
    expect(result.session.Items[0].LineKey).toBeNull();
    expect(result.session.PurchaseOrderID).toBe(7);
    expect(transport.load).toHaveBeenCalledWith("7");
  });

  it("maps a missing session to absent, never an error", async () => {
    const transport: ReconciliationTransport = {
      load: vi.fn(async () => {
        throw new Error("no reconciliation recorded");
      }),
      save: vi.fn(),
    };
    expect(await loadSession(transport, "7")).toEqual({ status: "absent" });
  });

  it("maps any other failure to an error with the message", async () => {
    const transport: ReconciliationTransport = {
      load: vi.fn(async () => {
        throw new Error("the server is unreachable");
      }),
      save: vi.fn(),
    };
    const result = await loadSession(transport, "7");
    expect(result).toEqual({
      status: "error",
      message: "the server is unreachable",
    });
  });
});

describe("saveSession through a fake backend", () => {
  function fakeBackend(): ReconciliationTransport & { stored: ReconciliationSession | null } {
    const backend: ReconciliationTransport & { stored: ReconciliationSession | null } = {
      stored: null,
      load: vi.fn(async () => {
        if (backend.stored === null) throw new Error("no reconciliation recorded");
        return backend.stored;
      }),
      save: vi.fn(async (_poID: string, next: ReconciliationSession) => {
        backend.stored = {
          ...next,
          ID: 3,
          POVersion: next.POVersion + 1,
          UpdatedAt: "2026-10-28T00:00:02Z",
        };
        return backend.stored;
      }),
    };
    return backend;
  }

  it("saves and reopens the same items and decisions", async () => {
    const transport = fakeBackend();
    const saved = await saveSession(transport, "7", session());
    expect(saved.status).toBe("saved");
    if (saved.status !== "saved") throw new Error("expected saved");
    expect(saved.session.POVersion).toBe(5);

    const reopened = await loadSession(transport, "7");
    expect(reopened.status).toBe("ready");
    if (reopened.status !== "ready") throw new Error("expected ready");
    expect(reopened.session.Items).toHaveLength(1);
    expect(reopened.session.Items[0]).toMatchObject({
      Key: "header:paymentTerms",
      Status: "unresolved",
      POValue: "50% advance",
    });
    expect(transport.save).toHaveBeenCalledWith("7", session());
  });

  it("surfaces a stale write as a conflict error, not a silent overwrite", async () => {
    const transport: ReconciliationTransport = {
      load: vi.fn(),
      save: vi.fn(async () => {
        throw new Error("purchase order version conflict");
      }),
    };
    const result = await saveSession(transport, "7", session());
    expect(result.status).toBe("error");
    if (result.status !== "error") throw new Error("expected error");
    expect(result.message).toBe("purchase order version conflict");
  });
});
