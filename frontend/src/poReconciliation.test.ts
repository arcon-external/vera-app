import { describe, expect, it } from "vitest";
import {
  HEADER_FIELDS,
  applyKeepResolution,
  applyUseResolution,
  commitHeaderField,
  countUnresolvedHeaderItems,
  normalizeSession,
  orderPaneItems,
  paneFilterCounts,
  prefillDecisionsFromSession,
  type ReconciliationItem,
} from "./poReconciliation";

// Ticket 04: the header commit state machine (contract section 4, header
// rows). Each case drives the pure transition with literals from the contract
// table, never recomputed expectations. R-01 pins required=false for all three
// header fields: an empty header value is a normal comparison, never a notice.

const PAYMENT_TERMS = HEADER_FIELDS["header:paymentTerms"];
const INCOTERMS = HEADER_FIELDS["header:incoterms"];
const DISCOUNT = HEADER_FIELDS["header:discountPercent"];

function item(overrides: Partial<ReconciliationItem> = {}): ReconciliationItem {
  return {
    Key: "header:paymentTerms",
    Field: "header:paymentTerms",
    Kind: "difference",
    LineKey: null,
    SourceLineRef: null,
    Label: "Payment terms",
    POValue: "50% advance",
    SourceValue: "30 days",
    Status: "unresolved",
    Resolution: null,
    ActorID: 0,
    UpdatedAt: "2026-10-28T00:00:01Z",
    History: [],
    ...overrides,
  };
}

describe("commitHeaderField", () => {
  it("an empty non-required field compares as a value, never an empty notice", () => {
    // Both sides empty is a real match, so the matched notice applies; per
    // R-01 there is never an empty notice for a field the existing validation
    // does not require.
    const bothEmpty = commitHeaderField({
      field: PAYMENT_TERMS,
      poValue: "   ",
      sourceValue: "",
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(bothEmpty.notice).toBe("matched");
    expect(bothEmpty.item).toBeNull();
    expect(bothEmpty.validationError).toBeNull();

    // Empty against a source value is a normal difference item.
    const emptyVsSource = commitHeaderField({
      field: PAYMENT_TERMS,
      poValue: "",
      sourceValue: "30 days",
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(emptyVsSource.notice).toBeNull();
    expect(emptyVsSource.item?.Status).toBe("unresolved");
    expect(emptyVsSource.item?.POValue).toBe("");
    expect(emptyVsSource.item?.SourceValue).toBe("30 days");
  });

  it("an empty required field shows an empty notice and creates no item", () => {
    // The required branch exists for fields validation requires (R-01 keeps
    // the header set non-required; lines own the required cases in ticket 05).
    const result = commitHeaderField({
      field: { ...PAYMENT_TERMS, required: true },
      poValue: "  ",
      sourceValue: "30 days",
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(result.notice).toBe("empty");
    expect(result.item).toBeNull();
    expect(result.itemChanged).toBe(false);
  });

  it("a matching commit shows a matched notice only when no item exists", () => {
    const fresh = commitHeaderField({
      field: INCOTERMS,
      poValue: "CIF",
      sourceValue: "CIF",
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(fresh.notice).toBe("matched");
    expect(fresh.item).toBeNull();

    // A resolved item never gains a matched notice, even when the values
    // still match: the resolved row is the record.
    const retained = commitHeaderField({
      field: INCOTERMS,
      poValue: "CIF",
      sourceValue: "CIF",
      existingItem: item({
        Key: "header:incoterms",
        Field: "header:incoterms",
        Label: "Incoterms",
        POValue: "CIF",
        SourceValue: "CIF",
        Status: "resolved",
        Resolution: "kept_po",
      }),
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(retained.notice).toBeNull();
    expect(retained.item?.Status).toBe("resolved");
    expect(retained.item?.Resolution).toBe("kept_po");
    expect(retained.item?.History).toHaveLength(0);
  });

  it("text comparison trims the ends and keeps case and inner spaces", () => {
    const padded = commitHeaderField({
      field: PAYMENT_TERMS,
      poValue: "  30 days  ",
      sourceValue: "30 days",
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(padded.notice).toBe("matched");

    const cased = commitHeaderField({
      field: PAYMENT_TERMS,
      poValue: "30 Days",
      sourceValue: "30 days",
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(cased.item?.Status).toBe("unresolved");
  });

  it("a differing commit creates one unresolved item with both values", () => {
    const result = commitHeaderField({
      field: PAYMENT_TERMS,
      poValue: "50% advance, balance 30 days",
      sourceValue: "30 days",
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:02Z",
    });
    expect(result.notice).toBeNull();
    expect(result.itemChanged).toBe(true);
    expect(result.item).toMatchObject({
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      POValue: "50% advance, balance 30 days",
      SourceValue: "30 days",
      Status: "unresolved",
      Resolution: null,
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:02Z",
    });
  });

  it("re-committing the same unresolved value is idempotent", () => {
    const again = commitHeaderField({
      field: PAYMENT_TERMS,
      poValue: "50% advance",
      sourceValue: "30 days",
      existingItem: item(),
      actorId: 7,
      now: "2026-10-28T00:00:09Z",
    });
    expect(again.item?.Status).toBe("unresolved");
    expect(again.itemChanged).toBe(false);
  });

  it("a new differing value reopens the same item and retains history", () => {
    const reopened = commitHeaderField({
      field: PAYMENT_TERMS,
      poValue: "60% advance",
      sourceValue: "30 days",
      existingItem: item({
        Status: "resolved",
        Resolution: "kept_po",
        History: [
          {
            Kind: "difference",
            SourceLineRef: null,
            POValue: "50% advance",
            SourceValue: "30 days",
            Status: "resolved",
            Resolution: "kept_po",
            ActorID: 7,
            At: "2026-10-28T00:00:05Z",
          },
        ],
      }),
      actorId: 7,
      now: "2026-10-28T00:00:09Z",
    });
    expect(reopened.item?.Status).toBe("unresolved");
    expect(reopened.item?.Resolution).toBeNull();
    expect(reopened.item?.POValue).toBe("60% advance");
    expect(reopened.itemChanged).toBe(true);
    // The earlier kept decision stays in the item's history.
    expect(reopened.item?.History).toHaveLength(1);
    expect(reopened.item?.History[0]).toMatchObject({ Resolution: "kept_po" });
  });

  it("editing back to the source value resolves as values-now-match", () => {
    const matched = commitHeaderField({
      field: PAYMENT_TERMS,
      poValue: "30 days",
      sourceValue: "30 days",
      existingItem: item(),
      actorId: 7,
      now: "2026-10-28T00:00:09Z",
    });
    expect(matched.notice).toBeNull();
    expect(matched.item?.Status).toBe("resolved");
    expect(matched.item?.Resolution).toBe("values_now_match");
    expect(matched.itemChanged).toBe(true);
  });

  it("the whole-PO discount compares as an integer percent, empty means 0", () => {
    // Empty input is 0%, so empty against a 0% source is a match.
    const emptyBoth = commitHeaderField({
      field: DISCOUNT,
      poValue: "",
      sourceValue: 0,
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(emptyBoth.notice).toBe("matched");
    expect(emptyBoth.item).toBeNull();

    const differ = commitHeaderField({
      field: DISCOUNT,
      poValue: "2",
      sourceValue: 0,
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(differ.item).toMatchObject({
      Key: "header:discountPercent",
      Label: "Whole-PO discount",
      POValue: 2,
      SourceValue: 0,
      Status: "unresolved",
    });
  });

  it("an invalid discount is a validation error, never an item", () => {
    const invalid = commitHeaderField({
      field: DISCOUNT,
      poValue: "ten",
      sourceValue: 0,
      existingItem: null,
      actorId: 7,
      now: "2026-10-28T00:00:00Z",
    });
    expect(invalid.validationError).toBe(
      "Whole-PO discount must be between 0 and 100."
    );
    expect(invalid.item).toBeNull();
    expect(invalid.notice).toBeNull();
  });
});

describe("keep and use resolutions", () => {
  it("keep resolves the same item without touching the field value", () => {
    const kept = applyKeepResolution(item(), 7, "2026-10-28T00:00:05Z");
    expect(kept).toMatchObject({
      Status: "resolved",
      Resolution: "kept_po",
      POValue: "50% advance",
      ActorID: 7,
      UpdatedAt: "2026-10-28T00:00:05Z",
    });
    expect(kept.History).toHaveLength(1);
    expect(kept.History[0]).toMatchObject({
      Status: "resolved",
      Resolution: "kept_po",
    });
  });

  it("use resolves and carries the Quotation value for the field update", () => {
    const { item: used, value } = applyUseResolution(item(), 7, "2026-10-28T00:00:05Z");
    expect(used.Status).toBe("resolved");
    expect(used.Resolution).toBe("used_quotation");
    expect(value).toBe("30 days");
  });
});

describe("pane view model", () => {
  const unresolvedOld = item({ Key: "header:paymentTerms", UpdatedAt: "2026-10-28T00:00:01Z" });
  const unresolvedNew = item({
    Key: "header:incoterms",
    Field: "header:incoterms",
    Label: "Incoterms",
    UpdatedAt: "2026-10-28T00:00:03Z",
  });
  const resolvedRow = item({
    Key: "header:discountPercent",
    Field: "header:discountPercent",
    Label: "Whole-PO discount",
    Status: "resolved",
    Resolution: "kept_po",
    UpdatedAt: "2026-10-28T00:00:02Z",
  });

  it("orders unresolved newest first, then resolved newest first", () => {
    const ordered = orderPaneItems([unresolvedOld, resolvedRow, unresolvedNew], "all");
    expect(ordered.map((entry) => entry.Key)).toEqual([
      "header:incoterms",
      "header:paymentTerms",
      "header:discountPercent",
    ]);
  });

  it("filters to unresolved or resolved without losing rows", () => {
    expect(orderPaneItems([unresolvedOld, resolvedRow], "unresolved")).toHaveLength(1);
    expect(orderPaneItems([unresolvedOld, resolvedRow], "resolved")).toHaveLength(1);
    expect(orderPaneItems([unresolvedOld, resolvedRow], "resolved")[0].Status).toBe(
      "resolved"
    );
  });

  it("counts unresolved header items for the gate and the sticky count", () => {
    expect(countUnresolvedHeaderItems([unresolvedOld, unresolvedNew, resolvedRow])).toBe(2);
    expect(countUnresolvedHeaderItems([resolvedRow])).toBe(0);
    const counts = paneFilterCounts([unresolvedOld, unresolvedNew, resolvedRow]);
    expect(counts).toEqual({ all: 3, unresolved: 2, resolved: 1 });
  });
});

describe("prefillDecisionsFromSession", () => {
  const source = { PaymentTerms: "30 days", Incoterms: "CIF", DiscountPercent: 0 };
  const current = { paymentTerms: "50% advance", incoterms: "CIF", discountPercent: "0" };

  it("pre-fills keep for a kept value that still differs", () => {
    const decisions = prefillDecisionsFromSession(
      [
        item({
          Status: "resolved",
          Resolution: "kept_po",
          POValue: "50% advance",
          SourceValue: "30 days",
        }),
      ],
      current,
      source
    );
    expect(decisions).toEqual({ "header:paymentTerms": "po" });
  });

  it("does not pre-fill when the value changed after the decision", () => {
    const decisions = prefillDecisionsFromSession(
      [
        item({
          Status: "resolved",
          Resolution: "kept_po",
          POValue: "40% advance",
          SourceValue: "30 days",
        }),
      ],
      current,
      source
    );
    expect(decisions).toEqual({});
  });

  it("does not pre-fill used or matched resolutions, and ignores unknown keys", () => {
    const decisions = prefillDecisionsFromSession(
      [
        item({
          Key: "header:incoterms",
          Field: "header:incoterms",
          Label: "Incoterms",
          POValue: "CIF",
          SourceValue: "CIF",
          Status: "resolved",
          Resolution: "used_quotation",
        }),
        item({ Key: "line:1:quantity", Field: "line:quantity" }),
      ],
      current,
      source
    );
    expect(decisions).toEqual({});
  });
});

describe("normalizeSession", () => {
  it("turns generated-binding undefineds into contract nulls", () => {
    const normalized = normalizeSession({
      ID: 1,
      PurchaseOrderID: 7,
      SourceQuotationID: 12,
      SourceRevision: 1,
      POVersion: 3,
      Active: true,
      ReadOnly: false,
      CreatedBy: 7,
      CreatedAt: "2026-10-28T00:00:00Z",
      UpdatedAt: "2026-10-28T00:00:01Z",
      LineIdentities: [{ Key: "client:x", ID: undefined, Removed: false }],
      Items: [
        {
          Key: "header:paymentTerms",
          Field: "header:paymentTerms",
          Kind: "difference",
          LineKey: undefined,
          SourceLineRef: undefined,
          Label: "Payment terms",
          POValue: "50% advance",
          SourceValue: "30 days",
          Status: "unresolved",
          Resolution: undefined,
          ActorID: 7,
          UpdatedAt: "2026-10-28T00:00:01Z",
          History: [
            {
              Kind: "difference",
              SourceLineRef: undefined,
              POValue: "50% advance",
              SourceValue: "30 days",
              Status: "unresolved",
              Resolution: undefined,
              ActorID: 7,
              At: "2026-10-28T00:00:01Z",
            },
          ],
        },
      ],
    });
    expect(normalized.LineIdentities[0].ID).toBeNull();
    expect(normalized.Items[0].LineKey).toBeNull();
    expect(normalized.Items[0].Resolution).toBeNull();
    expect(normalized.Items[0].History[0].SourceLineRef).toBeNull();
  });
});

describe("prefill used_quotation decisions", () => {
  it("pre-fills the quotation choice when the field still holds the recorded PO value", () => {
    const used = {
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      POValue: "50% advance",
      SourceValue: "30 days",
      Status: "resolved",
      Resolution: "used_quotation",
      ActorID: 7,
      UpdatedAt: "2026-10-29T00:00:05Z",
      History: [],
    };
    const decisions = prefillDecisionsFromSession(
      [used],
      { paymentTerms: "50% advance", incoterms: "CIF", discountPercent: "0" },
      { PaymentTerms: "30 days", Incoterms: "CIF", DiscountPercent: 0 }
    );
    expect(decisions).toEqual({ "header:paymentTerms": "quotation" });
  });

  it("does not pre-fill a used choice whose value moved away from the recorded one", () => {
    const used = {
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      POValue: "30 days",
      SourceValue: "30 days",
      Status: "resolved",
      Resolution: "used_quotation",
      ActorID: 7,
      UpdatedAt: "2026-10-29T00:00:05Z",
      History: [],
    };
    const decisions = prefillDecisionsFromSession(
      [used],
      { paymentTerms: "50% advance", incoterms: "CIF", discountPercent: "0" },
      { PaymentTerms: "30 days", Incoterms: "CIF", DiscountPercent: 0 }
    );
    expect(decisions).toEqual({});
  });
});
