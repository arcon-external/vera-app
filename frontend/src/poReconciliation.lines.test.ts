import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import type { PurchaseOrderSourceLineDTO } from "./poFinalize";
import {
  canonicalLineFieldValue,
  canonicalSourceLineValue,
  commitLineField,
  countBlockingItems,
  deriveLineSet,
  explainTotalsDifference,
  sessionLineKey,
  prefillLineDecisions,
  reconcileLineSession,
  acknowledgeLineMatch,
  hasAckRecording,
  resolveLineRemoved,
  sessionBlockers,
  type LineCommitInput,
} from "./poReconciliation.lines";
import type { ReconciliationItem, ReconciliationSession } from "./poReconciliation";

// Ticket 05: the line commit state machine, stable identity, the line-set
// derivation, removal with retained history, the totals reason and the named
// Final Save blockers. Every case drives literals from the contract, never
// recomputed expectations. Money is IDR minor units; the stable key never
// uses the displayed line number.

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

function sourceLine(overrides: Partial<PurchaseOrderSourceLineDTO> = {}): PurchaseOrderSourceLineDTO {
  return {
    QuotationItemNo: 1,
    QuotationSubNo: null,
    LineLabel: "1",
    Description: "Widget",
    UoM: "Pc",
    QuantityScaled: 20000,
    UnitPriceMinor: 10000,
    DiscountEnabled: false,
    DiscountPercent: 0,
    ...overrides,
  };
}

function lineItem(overrides: Partial<ReconciliationItem> = {}): ReconciliationItem {
  return {
    Key: "line:client:aaa:quantity",
    Field: "line:quantity",
    Kind: "difference",
    LineKey: "client:aaa",
    SourceLineRef: "1",
    Label: "Line 1 quantity",
    POValue: 20000,
    SourceValue: 20000,
    Status: "unresolved",
    Resolution: null,
    ActorID: 0,
    UpdatedAt: "2026-10-28T00:00:01Z",
    History: [],
    ...overrides,
  };
}

function commitInput(overrides: Partial<LineCommitInput> = {}): LineCommitInput {
  return {
    lineKey: "client:aaa",
    displayNumber: 1,
    field: "quantity",
    poValue: "2",
    source: sourceLine(),
    existingItem: null,
    actorId: 7,
    now: "2026-10-28T00:00:01Z",
    currency: IDR,
    ...overrides,
  };
}

describe("sessionLineKey (first-seen wins, never the position)", () => {
  it("uses the stored identity when the session already saw the row", () => {
    const identities = [{ Key: "client:aaa", ID: 501 }];
    // The row later gains an id, but the existing client decision is never
    // re-keyed to id:<row>.
    expect(
      sessionLineKey(identities, { persistedId: 501, dtoAlias: "client:aaa", clientKey: "client:aaa" })
    ).toBe("client:aaa");
    expect(
      sessionLineKey(identities, { persistedId: 501, clientKey: "client:aaa" })
    ).toBe("client:aaa");
  });

  it("seeds id:<row> for a persisted row with no alias and no identity", () => {
    expect(
      sessionLineKey([], { persistedId: 501, dtoAlias: null, clientKey: "client:fresh" })
    ).toBe("id:501");
  });

  it("prefers the row-carried alias for a persisted row the session never saw", () => {
    expect(
      sessionLineKey([], { persistedId: 501, dtoAlias: "client:kept", clientKey: "client:kept" })
    ).toBe("client:kept");
  });

  it("keys a new editor line by its alias", () => {
    expect(
      sessionLineKey([], { persistedId: null, dtoAlias: null, clientKey: "client:new" })
    ).toBe("client:new");
  });

  it("reordering never re-keys a decision", () => {
    // Same line, swapped display number: identical keys both times. The
    // value differs from the source so the first commit really creates.
    const first = commitLineField(
      commitInput({ poValue: "3", existingItem: null })
    );
    expect(first.item?.Key).toBe("line:client:aaa:quantity");
    const reordered = commitLineField(
      commitInput({
        displayNumber: 2,
        poValue: "3",
        existingItem: first.item,
      })
    );
    expect(reordered.item?.Key).toBe("line:client:aaa:quantity");
    expect(reordered.itemChanged).toBe(false);
  });
});

describe("commitLineField", () => {
  it("compares quantity in stored units: 2 and 2.0000 match", () => {
    const result = commitLineField(commitInput({ poValue: "2.0000" }));
    expect(result.notice).toBe("matched");
    expect(result.item).toBeNull();
  });

  it("an invalid quantity is a validation error, never an item", () => {
    for (const raw of ["", "0", "-2", "ten", "1.00001"]) {
      const result = commitLineField(commitInput({ poValue: raw }));
      expect(result.validationError).toBe(
        "Quantity must be greater than zero."
      );
      expect(result.item).toBeNull();
    }
  });

  it("a differing quantity creates one unresolved item with both values", () => {
    const result = commitLineField(commitInput({ poValue: "3" }));
    expect(result.notice).toBeNull();
    expect(result.item).toMatchObject({
      Key: "line:client:aaa:quantity",
      Field: "line:quantity",
      Kind: "difference",
      LineKey: "client:aaa",
      SourceLineRef: "1",
      Label: "Line 1 quantity",
      POValue: 30000,
      SourceValue: 20000,
      Status: "unresolved",
      Resolution: null,
    });
    expect(result.itemChanged).toBe(true);
  });

  it("unit price compares minor units and accepts a genuine zero", () => {
    const zero = commitLineField(
      commitInput({
        field: "unitPrice",
        poValue: "0",
        source: sourceLine({ UnitPriceMinor: 0 }),
      })
    );
    expect(zero.notice).toBe("matched");

    const bad = commitLineField(
      commitInput({ field: "unitPrice", poValue: "free" })
    );
    expect(bad.validationError).toBe("Unit price must be a valid amount.");
    expect(bad.item).toBeNull();
  });

  it("discount keeps Enabled in the value; disabled differs from enabled 0%", () => {
    const kept = commitLineField(
      commitInput({
        field: "discount",
        poValue: { enabled: false, percent: 0 },
        source: sourceLine({ DiscountEnabled: false, DiscountPercent: 0 }),
      })
    );
    expect(kept.item).toBeNull();

    const changed = commitLineField(
      commitInput({
        field: "discount",
        poValue: { enabled: true, percent: 0 },
        source: sourceLine({ DiscountEnabled: false, DiscountPercent: 0 }),
      })
    );
    expect(changed.item).toMatchObject({
      Key: "line:client:aaa:discount",
      POValue: { Enabled: true, Percent: 0 },
      Status: "unresolved",
    });

    const bad = commitLineField(
      commitInput({
        field: "discount",
        poValue: { enabled: true, percent: 101 },
      })
    );
    expect(bad.validationError).toBe("Discount must be between 0 and 100.");
    expect(bad.item).toBeNull();
  });

  it("description and unit compare trimmed text, case-sensitive", () => {
    const match = commitLineField(
      commitInput({
        field: "description",
        poValue: "  Widget  ",
        source: sourceLine({ Description: "Widget" }),
      })
    );
    expect(match.notice).toBe("matched");

    const differ = commitLineField(
      commitInput({
        field: "uom",
        poValue: "PCS",
        source: sourceLine({ UoM: "Pc" }),
      })
    );
    expect(differ.item?.Status).toBe("unresolved");
  });

  it("an unmatched line never compares: no notice and no item", () => {
    const result = commitLineField(
      commitInput({ poValue: "3", source: null })
    );
    expect(result.notice).toBeNull();
    expect(result.item).toBeNull();
    expect(result.validationError).toBeNull();
  });

  it("re-committing the same kept value retains the decision", () => {
    const kept = lineItem({
      Status: "resolved",
      Resolution: "kept_po",
      POValue: 30000,
    });
    const again = commitLineField(
      commitInput({ poValue: "3", existingItem: kept })
    );
    expect(again.item?.Status).toBe("resolved");
    expect(again.item?.Resolution).toBe("kept_po");
    expect(again.itemChanged).toBe(false);
    expect(again.notice).toBeNull();
  });
});

describe("deriveLineSet", () => {
  const lines = [
    { sessionKey: "client:a", lineNumber: 1, quotationItemNo: 1, quotationSubNo: null },
    { sessionKey: "client:b", lineNumber: 2, quotationItemNo: null, quotationSubNo: null },
  ];
  const sources = [sourceLine(), sourceLine({ QuotationItemNo: 2, LineLabel: "2", Description: "Sprocket" })];

  it("partitions matched, missing and extra lines", () => {
    const set = deriveLineSet(lines, sources);
    expect(set.matched.map((entry) => entry.line.sessionKey)).toEqual(["client:a"]);
    expect(set.extra.map((entry) => entry.sessionKey)).toEqual(["client:b"]);
    expect(set.missing.map((entry) => entry.QuotationItemNo)).toEqual([2]);
    expect(set.duplicates).toEqual([]);
    expect(set.unknown).toEqual([]);
  });

  it("flags an unknown non-null reference instead of a plain extra", () => {
    const set = deriveLineSet(
      [{ sessionKey: "client:u", lineNumber: 1, quotationItemNo: 99, quotationSubNo: null }],
      sources
    );
    expect(set.extra).toEqual([]);
    expect(set.unknown.map((entry) => entry.sessionKey)).toEqual(["client:u"]);
    expect(set.unknown[0]?.reference).toBe("99");
  });

  it("flags a duplicate only until one match is corrected", () => {
    const claimed = deriveLineSet(
      [
        { sessionKey: "client:a", lineNumber: 1, quotationItemNo: 1, quotationSubNo: null },
        { sessionKey: "client:c", lineNumber: 2, quotationItemNo: 1, quotationSubNo: null },
      ],
      sources
    );
    expect(claimed.duplicates).toHaveLength(1);
    expect(claimed.duplicates[0]?.lineKeys).toEqual(["client:a", "client:c"]);

    // Correcting the second claim to a free reference dissolves the group.
    const fixed = deriveLineSet(
      [
        { sessionKey: "client:a", lineNumber: 1, quotationItemNo: 1, quotationSubNo: null },
        { sessionKey: "client:c", lineNumber: 2, quotationItemNo: 2, quotationSubNo: null },
      ],
      sources
    );
    expect(fixed.duplicates).toEqual([]);
    expect(fixed.matched).toHaveLength(2);
  });
});

describe("resolveLineRemoved", () => {
  it("resolves the removed line's items as line_removed with history retained", () => {
    const kept = lineItem({
      Status: "resolved",
      Resolution: "kept_po",
      History: [
        {
          Kind: "difference",
          SourceLineRef: "1",
          POValue: 30000,
          SourceValue: 20000,
          Status: "resolved",
          Resolution: "kept_po",
          ActorID: 7,
          At: "2026-10-28T00:00:02Z",
        },
      ],
    });
    const other = lineItem({ Key: "line:client:zzz:quantity", LineKey: "client:zzz" });
    const { items, removed } = resolveLineRemoved(
      [kept, other],
      "client:aaa",
      7,
      "2026-10-28T00:00:09Z"
    );
    expect(removed).toEqual(["client:aaa"]);
    const resolved = items.find((entry) => entry.Key === kept.Key);
    expect(resolved).toMatchObject({ Status: "resolved", Resolution: "line_removed" });
    expect(resolved?.History).toHaveLength(2);
    // The surviving line is untouched.
    expect(items.find((entry) => entry.Key === other.Key)).toBe(other);
  });
});

describe("explainTotalsDifference (R-09)", () => {
  it("names the first differing input in document order", () => {
    const reason = explainTotalsDifference([
      { key: "header:discountPercent", label: "Whole-PO discount", poDisplay: "2%", sourceDisplay: "0%", differs: true },
      { key: "line:client:a:quantity", label: "Line 1 quantity", poDisplay: "12", sourceDisplay: "10", differs: true },
    ]);
    expect(reason).toBe("Whole-PO discount 2% vs 0%");
  });

  it("says so when no input difference explains the printed total", () => {
    expect(explainTotalsDifference([])).toBe(
      "no input difference explains the printed total"
    );
  });
});

describe("sessionBlockers", () => {
  it("names unresolved items, missing, extra, duplicates and unknown refs", () => {
    const blockers = sessionBlockers({
      items: [lineItem({ Status: "unresolved" })],
      missing: [{ quotationItemNo: 2, quotationSubNo: null, acknowledged: false }],
      extra: [
        { sessionKey: "client:b", label: "Extra line 2", acknowledged: true },
        { sessionKey: "client:e", label: "Extra line 3", acknowledged: false },
      ],
      duplicates: [{ reference: "1", lineKeys: ["client:a", "client:c"] }],
      unknown: [{ sessionKey: "client:u", reference: "99", acknowledged: false }],
    });
    expect(blockers.map((entry) => entry.kind)).toEqual([
      "unresolved",
      "missing",
      "extra",
      "duplicate",
      "unknown",
    ]);
    expect(blockers.find((entry) => entry.kind === "unknown")?.label).toContain("99");
    expect(blockers.find((entry) => entry.kind === "extra")?.label).toContain(
      "Extra line 3"
    );
    // An acknowledged plain extra is not a blocker.
    expect(blockers.filter((entry) => entry.kind === "extra")).toHaveLength(1);
    expect(countBlockingItems(blockers)).toBe(5);
  });

  it("an acknowledged unknown reference still blocks", () => {
    const blockers = sessionBlockers({
      items: [],
      missing: [],
      extra: [],
      duplicates: [],
      unknown: [{ sessionKey: "client:u", reference: "99", acknowledged: true }],
    });
    expect(blockers).toHaveLength(1);
    expect(blockers[0]?.kind).toBe("unknown");
  });
});

describe("prefillLineDecisions", () => {
  const keptQuantity = lineItem({
    Key: "line:client:a:quantity",
    Field: "line:quantity",
    LineKey: "client:a",
    Status: "resolved",
    Resolution: "kept_po",
    POValue: 120000,
    SourceValue: 20000,
    SourceLineRef: "1",
  });
  const sources = [sourceLine()];
  const rawValues = {
    description: "Widget",
    uom: "Pc",
    quantity: "12",
    unitPrice: "1",
    discountEnabled: false,
    discountPercent: "0",
  };

  it("pre-fills keep for an unchanged kept line value", () => {
    const decisions = prefillLineDecisions(
      [keptQuantity],
      [{ sessionKey: "client:a", displayNumber: 1, values: rawValues }],
      sources,
      IDR
    );
    expect(decisions).toEqual({ "line:1:quantity": "po" });
  });

  it("does not pre-fill a changed value or an unused key", () => {
    const changed = prefillLineDecisions(
      [keptQuantity],
      [
        {
          sessionKey: "client:a",
          displayNumber: 3,
          values: { ...rawValues, quantity: "13" },
        },
      ],
      sources,
      IDR
    );
    expect(changed).toEqual({});

    const unknownKey = prefillLineDecisions(
      [{ ...keptQuantity, LineKey: "client:gone" }],
      [{ sessionKey: "client:a", displayNumber: 1, values: rawValues }],
      sources,
      IDR
    );
    expect(unknownKey).toEqual({});
  });
});

describe("reconcileLineSession (declarative line-set sync)", () => {
  const base = (overrides: Partial<ReconciliationItem> = {}): ReconciliationItem =>
    lineItem({
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      ...overrides,
    });

  const sessionWith = (items: ReconciliationItem[]): ReconciliationSession => ({
    ID: 3,
    PurchaseOrderID: 11,
    SourceQuotationID: 5,
    SourceRevision: 2,
    POVersion: 9,
    Active: true,
    ReadOnly: false,
    CreatedBy: 7,
    CreatedAt: "2026-10-28T00:00:00Z",
    UpdatedAt: "2026-10-28T00:00:00Z",
    LineIdentities: [],
    Items: items,
  });

  const env = {
    sources: [sourceLine(), sourceLine({ QuotationItemNo: 2, LineLabel: "2", Description: "Sprocket" })],
    actorId: 7,
    now: "2026-10-28T00:01:00Z",
  };

  it("seeds identities and missing/extra items on first run", () => {
    const { session, changed } = reconcileLineSession({
      session: sessionWith([base()]),
      lines: [
        { sessionKey: "client:a", displayNumber: 1, quotationItemNo: 1, quotationSubNo: null },
        { sessionKey: "client:b", displayNumber: 2, quotationItemNo: null, quotationSubNo: null },
      ],
      ...env,
    });
    expect(changed).toBe(true);
    expect(session.LineIdentities).toEqual([
      { Key: "client:a", ID: null, Removed: false },
      { Key: "client:b", ID: null, Removed: false },
    ]);
    const keys = session.Items.map((entry) => entry.Key);
    expect(keys).toContain("source:2:match");
    expect(keys).toContain("line:client:b:match");
    // The header item is untouched.
    expect(session.Items.find((entry) => entry.Key === base().Key)).toEqual(base());
    const missing = session.Items.find((entry) => entry.Key === "source:2:match");
    expect(missing).toMatchObject({ Kind: "missing_line", Status: "unresolved", SourceLineRef: "2" });
    const extra = session.Items.find((entry) => entry.Key === "line:client:b:match");
    expect(extra).toMatchObject({ Kind: "extra_line", Status: "unresolved", LineKey: "client:b" });
  });

  it("is idle when nothing changed", () => {
    const first = reconcileLineSession({
      session: sessionWith([]),
      lines: [{ sessionKey: "client:a", displayNumber: 1, quotationItemNo: 1, quotationSubNo: null }],
      sources: [sourceLine()],
      actorId: 7,
      now: env.now,
    });
    const second = reconcileLineSession({ ...env, session: first.session, lines: [{ sessionKey: "client:a", displayNumber: 1, quotationItemNo: 1, quotationSubNo: null }], sources: [sourceLine()] });
    expect(second.changed).toBe(false);
    expect(second.session).toBe(first.session);
  });

  it("marks removed identities and resolves their items as line_removed", () => {
    const kept = lineItem({ Key: "line:client:a:quantity", LineKey: "client:a", Status: "resolved", Resolution: "kept_po" });
    const first = reconcileLineSession({
      session: sessionWith([kept]),
      lines: [{ sessionKey: "client:a", displayNumber: 1, quotationItemNo: 1, quotationSubNo: null }],
      sources: [sourceLine()],
      actorId: 7,
      now: env.now,
    });
    const second = reconcileLineSession({
      session: first.session,
      lines: [],
      sources: [sourceLine()],
      actorId: 7,
      now: "2026-10-28T00:02:00Z",
    });
    expect(second.changed).toBe(true);
    expect(second.session.LineIdentities).toEqual([{ Key: "client:a", ID: null, Removed: true }]);
    const resolved = second.session.Items.find((entry) => entry.Key === kept.Key);
    expect(resolved).toMatchObject({ Status: "resolved", Resolution: "line_removed" });
    expect(resolved?.History).toHaveLength(1);
  });

  it("creates a blocking duplicate item until one match is corrected", () => {
    const lines = [
      { sessionKey: "client:a", displayNumber: 1, quotationItemNo: 1, quotationSubNo: null },
      { sessionKey: "client:c", displayNumber: 2, quotationItemNo: 1, quotationSubNo: null },
    ];
    const { session } = reconcileLineSession({ session: sessionWith([]), lines, sources: [sourceLine()], actorId: 7, now: env.now });
    for (const claimant of ["client:a", "client:c"]) {
      const dup = session.Items.find((entry) => entry.Key === `line:${claimant}:match`);
      expect(dup).toMatchObject({
        Kind: "duplicate_match",
        Field: "line:match",
        LineKey: claimant,
        SourceLineRef: "1",
        POValue: "1",
        SourceValue: null,
        Status: "unresolved",
      });
    }
    expect(
      session.Items.some((entry) => entry.Key === "source:1:duplicate")
    ).toBe(false);

    const blockers = sessionBlockers({
      items: session.Items,
      missing: [],
      extra: [],
      duplicates: [{ reference: "1", lineKeys: ["client:a", "client:c"] }],
      unknown: [],
    });
    // Each unresolved claimant item names the blocker; the covered set
    // entry adds no third blocker.
    expect(blockers).toHaveLength(2);
    for (const blocker of blockers) {
      expect(blocker).toMatchObject({ kind: "unresolved", label: "Duplicate match · Quotation 1" });
    }

    // Correcting the second claim to a free reference resolves the duplicate.
    const fixed = reconcileLineSession({
      session,
      lines: [
        { sessionKey: "client:a", displayNumber: 1, quotationItemNo: 1, quotationSubNo: null },
        { sessionKey: "client:c", displayNumber: 2, quotationItemNo: 2, quotationSubNo: null },
      ],
      sources: [sourceLine(), sourceLine({ QuotationItemNo: 2, LineLabel: "2" })],
      actorId: 7,
      now: "2026-10-28T00:03:00Z",
    });
    const resolvedDup = fixed.session.Items.find((entry) => entry.Key === "line:client:a:match");
    expect(resolvedDup).toMatchObject({ Status: "resolved", Resolution: "match_corrected" });
    const correctedClaim = fixed.session.Items.find((entry) => entry.Key === "line:client:c:match");
    expect(correctedClaim).toMatchObject({ Status: "resolved", Resolution: "match_corrected" });
  });

  it("flags an unknown reference and keeps it blocking after acknowledgement", () => {
    const lines = [{ sessionKey: "client:u", displayNumber: 1, quotationItemNo: 99, quotationSubNo: null }];
    const { session } = reconcileLineSession({ session: sessionWith([]), lines, sources: [sourceLine()], actorId: 7, now: env.now });
    const unknown = session.Items.find((entry) => entry.Key === "line:client:u:match");
    expect(unknown).toMatchObject({
      Kind: "extra_line",
      Field: "line:match",
      LineKey: "client:u",
      Status: "unresolved",
      SourceLineRef: null,
      POValue: "99",
      SourceValue: null,
    });

    const acked = acknowledgeLineMatch(session, "line:client:u:match", "acknowledged_extra", 7, "2026-10-28T00:04:00Z");
    expect(acked.changed).toBe(true);
    const kept = acked.session.Items.find((entry) => entry.Key === "line:client:u:match");
    expect(kept?.Status).toBe("unresolved");
    expect(hasAckRecording(kept!, "acknowledged_extra")).toBe(true);
  });

  it("acknowledging a plain extra resolves it; correcting the match resolves as match_corrected", () => {
    const lines = [{ sessionKey: "client:b", displayNumber: 1, quotationItemNo: null, quotationSubNo: null }];
    const { session } = reconcileLineSession({ session: sessionWith([]), lines, sources: [sourceLine()], actorId: 7, now: env.now });
    const acked = acknowledgeLineMatch(session, "line:client:b:match", "acknowledged_extra", 7, "2026-10-28T00:04:00Z");
    expect(acked.session.Items.find((entry) => entry.Key === "line:client:b:match")).toMatchObject({
      Status: "resolved",
      Resolution: "acknowledged_extra",
    });

    // Linking the line to a real source resolves the correction, keeping history.
    const corrected = reconcileLineSession({
      session: acked.session,
      lines: [{ sessionKey: "client:b", displayNumber: 1, quotationItemNo: 1, quotationSubNo: null }],
      sources: [sourceLine()],
      actorId: 7,
      now: "2026-10-28T00:05:00Z",
    });
    const done = corrected.session.Items.find((entry) => entry.Key === "line:client:b:match");
    expect(done).toMatchObject({ Status: "resolved", Resolution: "match_corrected" });
    expect(done?.History.length).toBeGreaterThanOrEqual(2);
  });
});

describe("sessionLineKey across save and reopen (first-seen wins)", () => {
  it("keeps id:<row> after the row gains an adopted alias", () => {
    // A pre-042 row enters the session at link time as id:5: no alias yet.
    const identities = [{ Key: "id:5", ID: 5, Removed: false }];
    expect(
      sessionLineKey(identities, { persistedId: 5, dtoAlias: null, clientKey: "client:fresh" })
    ).toBe("id:5");

    // The draft save adopts the editor alias onto the row, but the stored
    // identity does not move: reopen still resolves the same key even though
    // the DTO now carries the alias.
    expect(
      sessionLineKey(identities, {
        persistedId: 5,
        dtoAlias: "client:fresh",
        clientKey: "client:fresh",
      })
    ).toBe("id:5");
  });

  it("keeps the client key after the row gains its id", () => {
    // An editor-entered line is first seen by its alias; the save assigns
    // the row id and binds it, without re-keying the decision.
    const identities = [{ Key: "client:n", ID: 109, Removed: false }];
    expect(
      sessionLineKey(identities, {
        persistedId: 109,
        dtoAlias: "client:n",
        clientKey: "client:n",
      })
    ).toBe("client:n");
  });

  it("distinguishes identical-valued rows by id, never by value", () => {
    const identities = [
      { Key: "client:aaa", ID: 101, Removed: false },
      { Key: "client:bbb", ID: 102, Removed: false },
    ];
    expect(
      sessionLineKey(identities, { persistedId: 102, dtoAlias: "client:bbb", clientKey: "client:bbb" })
    ).toBe("client:bbb");
    expect(
      sessionLineKey(identities, { persistedId: 101, dtoAlias: "client:aaa", clientKey: "client:aaa" })
    ).toBe("client:aaa");
  });
});

describe("canonical disabled discount (F4: dormant percent is 0, Enabled stays distinct)", () => {
  it("canonicalizes a dormant disabled percent to 0 on the PO side", () => {
    expect(
      canonicalLineFieldValue("discount", { enabled: false, percent: 25 }, USD)
    ).toEqual({ ok: true, value: { Enabled: false, Percent: 0 } });
  });

  it("canonicalizes a dormant disabled percent to 0 on the source side", () => {
    expect(
      canonicalSourceLineValue("discount", { ...sourceLine(), DiscountPercent: 25 })
    ).toEqual({ Enabled: false, Percent: 0 });
  });

  it("does not invent a difference for two disabled discounts with different dormant percentages", () => {
    const result = commitLineField({
      lineKey: "client:probe-first",
      displayNumber: 1,
      field: "discount",
      poValue: { enabled: false, percent: 25 },
      source: sourceLine({ DiscountEnabled: false, DiscountPercent: 0 }),
      existingItem: null,
      actorId: 7,
      now: "2026-10-29T00:00:00Z",
      currency: USD,
    });
    expect(result.notice).toBe("matched");
    expect(result.item).toBeNull();
  });

  it("still treats enabled 0% as different from disabled 0%", () => {
    const result = commitLineField({
      lineKey: "client:probe-first",
      displayNumber: 1,
      field: "discount",
      poValue: { enabled: true, percent: 0 },
      source: sourceLine({ DiscountEnabled: false, DiscountPercent: 0 }),
      existingItem: null,
      actorId: 7,
      now: "2026-10-29T00:00:00Z",
      currency: USD,
    });
    expect(result.item?.Status).toBe("unresolved");
  });
});

describe("canonical line-set wire values (F1)", () => {
  const env = {
    sources: [
      sourceLine(),
      sourceLine({ QuotationItemNo: 2, LineLabel: "2", Description: "Sprocket" }),
      sourceLine({ QuotationItemNo: 2, QuotationSubNo: 1, LineLabel: "2.1", Description: "Sub widget" }),
    ],
    actorId: 7,
    now: "2026-10-29T00:00:00Z",
  };
  const emptySession = (): ReconciliationSession => ({
    ID: 1,
    PurchaseOrderID: 7,
    SourceQuotationID: 12,
    SourceRevision: 1,
    POVersion: 4,
    Active: true,
    ReadOnly: false,
    CreatedBy: 7,
    CreatedAt: "2026-10-29T00:00:00Z",
    UpdatedAt: "2026-10-29T00:00:00Z",
    LineIdentities: [],
    Items: [],
  });
  const line = (
    sessionKey: string,
    displayNumber: number,
    quotationItemNo: number | null,
    quotationSubNo: number | null = null
  ) => ({ sessionKey, persistedId: null, displayNumber, quotationItemNo, quotationSubNo });

  it("emits a plain extra with null wire values", () => {
    const { session } = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:b", 2, null), line("client:a", 1, 1)],
      ...env,
    });
    const extra = session.Items.find((entry) => entry.Key === "line:client:b:match");
    expect(extra).toMatchObject({
      Key: "line:client:b:match",
      Field: "line:match",
      Kind: "extra_line",
      LineKey: "client:b",
      SourceLineRef: null,
      POValue: null,
      SourceValue: null,
      Status: "unresolved",
    });
  });

  it("emits an unknown reference with the reference string and no source ref", () => {
    const { session } = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:u", 1, 99)],
      ...env,
    });
    const unknown = session.Items.find((entry) => entry.Key === "line:client:u:match");
    expect(unknown).toMatchObject({
      Key: "line:client:u:match",
      Field: "line:match",
      Kind: "extra_line",
      LineKey: "client:u",
      SourceLineRef: null,
      POValue: "99",
      SourceValue: null,
      Status: "unresolved",
    });
  });

  it("emits a missing sub-line with the canonical reference value", () => {
    const { session } = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:a", 1, 1)],
      ...env,
    });
    const missing = session.Items.find((entry) => entry.Key === "source:2.1:match");
    expect(missing).toMatchObject({
      Key: "source:2.1:match",
      Field: "source:match",
      Kind: "missing_line",
      LineKey: null,
      SourceLineRef: "2.1",
      POValue: null,
      SourceValue: "2.1",
      Status: "unresolved",
    });
  });

  it("emits one duplicate item per claimant, never a source-side duplicate", () => {
    const { session } = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:a", 1, 1), line("client:c", 2, 1)],
      ...env,
    });
    expect(session.Items.some((entry) => entry.Key === "source:1:duplicate")).toBe(false);
    for (const claimant of ["client:a", "client:c"]) {
      const dup = session.Items.find((entry) => entry.Key === `line:${claimant}:match`);
      expect(dup).toMatchObject({
        Key: `line:${claimant}:match`,
        Field: "line:match",
        Kind: "duplicate_match",
        LineKey: claimant,
        SourceLineRef: "1",
        POValue: "1",
        SourceValue: null,
        Status: "unresolved",
      });
    }
  });

  it("removing one duplicate claimant resolves it as line_removed and the survivor as match_corrected", () => {
    const first = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:a", 1, 1), line("client:c", 2, 1)],
      ...env,
    });
    const second = reconcileLineSession({
      session: first.session,
      lines: [line("client:a", 1, 1)],
      sources: env.sources,
      actorId: 7,
      now: "2026-10-29T00:01:00Z",
    });
    const removed = second.session.Items.find((entry) => entry.Key === "line:client:c:match");
    expect(removed).toMatchObject({ Status: "resolved", Resolution: "line_removed" });
    expect(removed?.History.length).toBeGreaterThan(0);
    const survivor = second.session.Items.find((entry) => entry.Key === "line:client:a:match");
    expect(survivor).toMatchObject({ Status: "resolved", Resolution: "match_corrected" });
  });

  it("correcting an unknown match keeps a valid shape with history", () => {
    const first = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:u", 1, 99)],
      ...env,
    });
    const second = reconcileLineSession({
      session: first.session,
      lines: [line("client:u", 1, 2)],
      sources: env.sources,
      actorId: 7,
      now: "2026-10-29T00:01:00Z",
    });
    const corrected = second.session.Items.find(
      (entry) => entry.Key === "line:client:u:match"
    );
    expect(corrected).toMatchObject({
      Status: "resolved",
      Resolution: "match_corrected",
      SourceLineRef: null,
      POValue: null,
      SourceValue: null,
    });
    expect(corrected?.History.length).toBeGreaterThan(0);
  });

  it("moving an unknown claim to another unknown reference keeps a valid shape", () => {
    const first = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:u", 1, 99)],
      ...env,
    });
    const second = reconcileLineSession({
      session: first.session,
      lines: [line("client:u", 1, 98)],
      sources: env.sources,
      actorId: 7,
      now: "2026-10-29T00:01:00Z",
    });
    const moved = second.session.Items.find((entry) => entry.Key === "line:client:u:match");
    expect(moved).toMatchObject({
      Status: "unresolved",
      Resolution: null,
      SourceLineRef: null,
      POValue: "98",
    });
    expect(moved?.History.length).toBeGreaterThan(0);
  });

  it("an acknowledged unknown stays unresolved with recorded history", () => {
    const first = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:u", 1, 99)],
      ...env,
    });
    const acked = acknowledgeLineMatch(
      first.session,
      "line:client:u:match",
      "acknowledged_extra",
      7,
      "2026-10-29T00:02:00Z"
    );
    expect(acked.changed).toBe(true);
    const kept = acked.session.Items.find((entry) => entry.Key === "line:client:u:match");
    expect(kept?.Status).toBe("unresolved");
    expect(hasAckRecording(kept!, "acknowledged_extra")).toBe(true);
  });

  it("an acknowledged plain extra resolves", () => {
    const first = reconcileLineSession({
      session: emptySession(),
      lines: [line("client:b", 2, null), line("client:a", 1, 1)],
      ...env,
    });
    const acked = acknowledgeLineMatch(
      first.session,
      "line:client:b:match",
      "acknowledged_extra",
      7,
      "2026-10-29T00:02:00Z"
    );
    expect(
      acked.session.Items.find((entry) => entry.Key === "line:client:b:match")
    ).toMatchObject({ Status: "resolved", Resolution: "acknowledged_extra" });
  });
});
