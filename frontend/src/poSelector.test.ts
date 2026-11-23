import { describe, expect, it } from "vitest";
import {
  canConfirm,
  canContinue,
  draftOf,
  initialPOSourceSelectorState,
  PO_SOURCE_SEARCH_PAUSE_MS,
  poSourceQueryDelay,
  poSourceSelectorReducer,
  selectionOf,
  tabStopIndex,
  visibleRows,
} from "./poSelector";
import type {
  POSourceAvailability,
  POSourceQuotation,
  POSourceRevision,
  POSourceSelectorState,
} from "./poSelector";

// Issue #467: the three-step selector's decisions are pure. The modal renders
// them, the API answers them, and this suite pins the parts a later ticket
// (#468) will build on: guarded confirm, back navigation, sole-revision
// auto-select, disabled used rows, the optional filter, and the exact payload.
//
// What this suite cannot prove: that a click reaches these transitions, that
// focus stays inside the dialog, or that the assembled modal reads well at
// 360/1280. Those are the view test's markup plus the snapshot case.

function quotation(overrides: Partial<POSourceQuotation> = {}): POSourceQuotation {
  return {
    id: 12,
    quoteNumber: "1502260001",
    subject: "Electrical installation",
    customerName: "PT. Infineon Technologies Batam",
    latestRevision: 1,
    issuedRevisionCount: 2,
    activePO: null,
    allBought: false,
    leafCount: 2,
    remainingLeafCount: 2,
    savedPoCount: 0,
    draftPoCount: 0,
    legacyHeaderDiscount: false,
    closed: false,
    closedAt: null,
    closedReason: "",
    validUntil: null,
    expiredUnclosed: false,
    ...overrides,
  };
}

function revisions(...numbers: number[]): POSourceRevision[] {
  return numbers.map((revision) => ({
    revision,
    issuedAt: "2026-10-20T10:00:00+07:00",
  }));
}

/** Drives the reducer through actions, returning the final state. */
function run(
  actions: Parameters<typeof poSourceSelectorReducer>[1][],
  start: POSourceSelectorState = initialPOSourceSelectorState
): POSourceSelectorState {
  return actions.reduce(poSourceSelectorReducer, start);
}

/** A state sitting on step 2 with the given revisions already answered. */
function atRevisionStep(
  revisionList: POSourceRevision[],
  overrides: Partial<POSourceSelectorState> = {}
): POSourceSelectorState {
  return run(
    [
      { type: "rowsLoaded", page: pageOf(quotation()) },
      { type: "quotationSelected", quotation: quotation() },
      {
        type: "revisionsLoaded",
        quotationId: quotation().id,
        revisions: revisionList,
      },
    ],
    { ...initialPOSourceSelectorState, ...overrides }
  );
}

function pageOf(...rows: POSourceQuotation[]) {
  return {
    quotations: rows,
    page: 1,
    pageSize: 15,
    total: rows.length,
    totalPages: 1,
  };
}

describe("PO source selector state machine (issue #467)", () => {
  it("starts on step 1 with nothing selected and confirm impossible", () => {
    const state = initialPOSourceSelectorState;
    expect(state.step).toBe(1);
    expect(state.selected).toBeNull();
    expect(state.selectedRevision).toBeNull();
    expect(selectionOf(state)).toBeNull();
    expect(canContinue(state)).toBe(false);
    expect(canConfirm(state)).toBe(false);
  });

  it("moves to step 2 when a Quotation is chosen and forgets stale revisions", () => {
    const state = run([
      { type: "rowsLoaded", page: pageOf(quotation()) },
      { type: "quotationSelected", quotation: quotation() },
    ]);
    expect(state.step).toBe(2);
    expect(state.selected?.id).toBe(12);
    expect(state.revisions).toEqual([]);
    expect(state.selectedRevision).toBeNull();
    // No revision answered yet: Continue stays disabled.
    expect(canContinue(state)).toBe(false);
  });

  it("auto-selects the sole issued revision and enables Continue", () => {
    const state = atRevisionStep(revisions(0));
    expect(state.revisionsLoading).toBe(false);
    expect(state.selectedRevision).toBe(0);
    expect(canContinue(state)).toBe(true);
    expect(state.step).toBe(2);
  });

  it("does not guess when several revisions exist", () => {
    const state = atRevisionStep(revisions(1, 0));
    expect(state.selectedRevision).toBeNull();
    expect(canContinue(state)).toBe(false);

    const chosen = poSourceSelectorReducer(state, {
      type: "revisionSelected",
      revision: 0,
    });
    expect(chosen.selectedRevision).toBe(0);
    expect(canContinue(chosen)).toBe(true);
  });

  it("only Continue reaches step 3, and only with a complete pair", () => {
    const incomplete = atRevisionStep(revisions(1, 0));
    const ignored = poSourceSelectorReducer(incomplete, { type: "continue" });
    expect(ignored.step).toBe(2);

    const complete = poSourceSelectorReducer(incomplete, {
      type: "revisionSelected",
      revision: 1,
    });
    const confirmed = poSourceSelectorReducer(complete, { type: "continue" });
    expect(confirmed.step).toBe(3);
    expect(selectionOf(confirmed)).toEqual({ quotationId: 12, revisionNumber: 1 });
  });

  it("keeps Confirm disabled until the selection is complete", () => {
    const complete = run([
      { type: "rowsLoaded", page: pageOf(quotation()) },
      { type: "quotationSelected", quotation: quotation() },
      { type: "revisionsLoaded", quotationId: 12, revisions: revisions(0) },
      { type: "continue" },
    ]);
    expect(complete.step).toBe(3);
    expect(canConfirm(complete)).toBe(true);

    // The guard, stated directly: step 3 alone is not enough. A state that
    // reached the screen without a revision must not confirm.
    const broken: POSourceSelectorState = { ...complete, selectedRevision: null };
    expect(canConfirm(broken)).toBe(false);
    expect(selectionOf(broken)).toBeNull();

    const noQuotation: POSourceSelectorState = { ...complete, selected: null };
    expect(canConfirm(noQuotation)).toBe(false);

    // And while a confirm is in flight, a second click cannot dispatch again.
    const confirming = poSourceSelectorReducer(complete, { type: "confirmStarted" });
    expect(canConfirm(confirming)).toBe(false);
  });

  it("navigates back without losing a complete choice, and clears when leaving step 2", () => {
    const complete = run([
      { type: "rowsLoaded", page: pageOf(quotation()) },
      { type: "quotationSelected", quotation: quotation() },
      { type: "revisionsLoaded", quotationId: 12, revisions: revisions(1, 0) },
      { type: "revisionSelected", revision: 0 },
      { type: "continue" },
    ]);

    const backToRevisions = poSourceSelectorReducer(complete, { type: "back" });
    expect(backToRevisions.step).toBe(2);
    expect(backToRevisions.selected?.id).toBe(12);
    expect(backToRevisions.selectedRevision).toBe(0);
    expect(canContinue(backToRevisions)).toBe(true);

    const backToQuotations = poSourceSelectorReducer(backToRevisions, { type: "back" });
    expect(backToQuotations.step).toBe(1);
    expect(backToQuotations.selected).toBeNull();
    expect(backToQuotations.selectedRevision).toBeNull();
    expect(backToQuotations.revisions).toEqual([]);
  });

  it("selects a partly bought Quotation and refuses an All bought one", () => {
    const partial = quotation({
      activePO: { id: 21, state: "saved" },
      savedPoCount: 1,
      remainingLeafCount: 1,
    });
    const selected = run([
      { type: "rowsLoaded", page: pageOf(partial) },
      { type: "quotationSelected", quotation: partial },
    ]);
    expect(selected.step).toBe(2);
    expect(selected.selected).toEqual(partial);

    const allBought = quotation({
      activePO: { id: 22, state: "saved" },
      allBought: true,
      remainingLeafCount: 0,
      savedPoCount: 2,
    });
    const refused = run([
      { type: "rowsLoaded", page: pageOf(allBought) },
      { type: "quotationSelected", quotation: allBought },
    ]);
    expect(refused.step).toBe(1);
    expect(refused.selected).toBeNull();
  });

  it("lets repair and link select an All bought quotation", () => {
    const allBought = quotation({
      allBought: true,
      remainingLeafCount: 0,
      savedPoCount: 2,
    });
    const entry = run([
      { type: "rowsLoaded", page: pageOf(allBought) },
      { type: "quotationSelected", quotation: allBought },
    ]);
    expect(entry.step).toBe(1);
    expect(entry.selected).toBeNull();

    for (const allowAllBought of [true]) {
      const repair = run([
        { type: "rowsLoaded", page: pageOf(allBought) },
        { type: "quotationSelected", quotation: allBought, allowAllBought },
      ]);
      expect(repair.step).toBe(2);
      expect(repair.selected).toEqual(allBought);
    }
  });

  it("offers the draft action but never for a saved record", () => {
    expect(draftOf(quotation({ activePO: { id: 20, state: "draft" } }))).toEqual({
      id: 20,
      state: "draft",
    });
    expect(draftOf(quotation({ activePO: { id: 21, state: "saved" } }))).toBeNull();
    expect(draftOf(quotation())).toBeNull();
  });

  it("filters All bought rows when the filter is on and keeps partial rows visible", () => {
    const available = quotation({ id: 1, quoteNumber: "1502260001" });
    const partial = quotation({
      id: 2,
      quoteNumber: "1502260002",
      activePO: { id: 20, state: "saved" },
      savedPoCount: 1,
      remainingLeafCount: 1,
    });
    const allBought = quotation({
      id: 3,
      quoteNumber: "1502260003",
      activePO: { id: 21, state: "saved" },
      allBought: true,
      savedPoCount: 2,
      remainingLeafCount: 0,
    });
    expect(visibleRows([available, partial, allBought], false)).toEqual([
      available,
      partial,
      allBought,
    ]);
    expect(visibleRows([available, partial, allBought], true)).toEqual([
      available,
      partial,
    ]);
    const state = run([
      { type: "rowsLoaded", page: pageOf(available, partial) },
      { type: "hideUsedChanged", hideUsed: true },
    ]);
    expect(state.hideUsed).toBe(true);
    expect(state.page).toBe(1);
  });

  it("resets to page 1 when search or the filter changes", () => {
    const onPageTwo = run(
      [{ type: "pageChanged", page: 2 }],
      { ...initialPOSourceSelectorState, page: 2 }
    );
    const searched = poSourceSelectorReducer(onPageTwo, {
      type: "searchChanged",
      search: "infineon",
    });
    expect(searched.search).toBe("infineon");
    expect(searched.page).toBe(1);

    const filtered = poSourceSelectorReducer(onPageTwo, {
      type: "hideUsedChanged",
      hideUsed: true,
    });
    expect(filtered.page).toBe(1);
  });

  it("ignores a revision answer for a Quotation that is no longer selected", () => {
    const state = run([
      { type: "rowsLoaded", page: pageOf(quotation()) },
      { type: "quotationSelected", quotation: quotation() },
      { type: "revisionsLoaded", quotationId: 99, revisions: revisions(0) },
    ]);
    expect(state.revisions).toEqual([]);
    expect(state.selectedRevision).toBeNull();
  });

  it("surfaces a failed revalidation and keeps Back available", () => {
    const complete = run([
      { type: "rowsLoaded", page: pageOf(quotation()) },
      { type: "quotationSelected", quotation: quotation() },
      { type: "revisionsLoaded", quotationId: 12, revisions: revisions(0) },
      { type: "continue" },
      { type: "confirmStarted" },
      {
        type: "confirmFailed",
        error: "This quotation is no longer available. Choose another source.",
      },
    ]);
    expect(complete.step).toBe(3);
    expect(complete.confirming).toBe(false);
    expect(complete.confirmError).toContain("no longer available");
    expect(canConfirm(complete)).toBe(true);
  });

  it("resets everything when the modal closes", () => {
    const dirty = run([
      { type: "searchChanged", search: "infineon" },
      { type: "rowsLoaded", page: pageOf(quotation()) },
      { type: "quotationSelected", quotation: quotation() },
      { type: "hideUsedChanged", hideUsed: true },
    ]);
    const closed = poSourceSelectorReducer(dirty, { type: "reset" });
    expect(closed).toEqual(initialPOSourceSelectorState);
  });
});

describe("PO source selector request timing and focus order (issue #467)", () => {
  it("queries immediately for paging and filtering, and pauses only while typing", () => {
    // First render and page/filter changes keep the search unchanged: there
    // is nothing to debounce, so the request is immediate.
    expect(poSourceQueryDelay("", "")).toBe(0);
    expect(poSourceQueryDelay("infineon", "infineon")).toBe(0);
    // Typing a filter term pauses once, so each keystroke does not query.
    expect(poSourceQueryDelay("inf", "infi")).toBe(PO_SOURCE_SEARCH_PAUSE_MS);
    // Clearing, or typing only whitespace, is an immediate query as well.
    expect(poSourceQueryDelay("infi", "")).toBe(0);
    expect(poSourceQueryDelay("", "   ")).toBe(0);
  });

  it("wraps Tab and Shift+Tab through the dialog's stops", () => {
    expect(tabStopIndex(0, -1, false)).toBe(-1);
    expect(tabStopIndex(3, -1, false)).toBe(0);
    expect(tabStopIndex(3, -1, true)).toBe(2);
    expect(tabStopIndex(3, 0, false)).toBe(1);
    expect(tabStopIndex(3, 2, false)).toBe(0);
    expect(tabStopIndex(3, 0, true)).toBe(2);
    expect(tabStopIndex(3, 2, true)).toBe(1);
    // An active element the list no longer contains falls back to the edge.
    expect(tabStopIndex(3, 7, false)).toBe(0);
  });
});

describe("PO source selector closure and expiry (issue #656)", () => {
  const CLOSED = quotation({
    id: 21,
    quoteNumber: "1502260021",
    closed: true,
    closedAt: "2026-11-20T03:04:05Z",
    closedReason: "customer cancelled the remainder",
    validUntil: "2026-12-01",
  });

  function availability(
    overrides: Partial<POSourceAvailability> = {}
  ): POSourceAvailability {
    return {
      quotationId: 12,
      revisionNumber: 1,
      latestRevision: 1,
      allBought: false,
      legacyHeaderDiscount: false,
      savedPoCount: 0,
      draftPoCount: 0,
      closed: false,
      closedAt: null,
      closedReason: "",
      validUntil: "2026-12-01",
      expiredUnclosed: false,
      items: [
        {
          quotationItemNo: 1,
          quotationSubNo: null,
          label: "1",
          description: "Electrical installation",
          uom: "Pc",
          quotedQuantityScaled: 100000,
          boughtQuantityScaled: 0,
          draftQuantityScaled: 0,
          remainingQuantityScaled: 100000,
          overBoughtQuantityScaled: 0,
          quotedUnitPriceMinor: 5000000,
        },
      ],
      ...overrides,
    };
  }

  function atConfirmStep(): POSourceSelectorState {
    return run([
      { type: "rowsLoaded", page: pageOf(quotation()) },
      { type: "quotationSelected", quotation: quotation() },
      { type: "revisionsLoaded", quotationId: 12, revisions: revisions(1) },
      { type: "continue" },
    ]);
  }

  it("refuses a closed quotation in entry mode but lets repair and link select it", () => {
    const entry = run([
      { type: "rowsLoaded", page: pageOf(CLOSED) },
      { type: "quotationSelected", quotation: CLOSED },
    ]);
    expect(entry.step).toBe(1);
    expect(entry.selected).toBeNull();

    const repair = run([
      { type: "rowsLoaded", page: pageOf(CLOSED) },
      { type: "quotationSelected", quotation: CLOSED, allowAllBought: true },
    ]);
    expect(repair.step).toBe(2);
    expect(repair.selected).toEqual(CLOSED);
  });

  it("keeps a closed row under the used filter even when it is fully bought", () => {
    const closedAndBought = quotation({ ...CLOSED, allBought: true, remainingLeafCount: 0 });
    const plainBought = quotation({
      id: 22,
      quoteNumber: "1502260022",
      allBought: true,
      remainingLeafCount: 0,
    });
    expect(visibleRows([closedAndBought], true)).toEqual([closedAndBought]);
    expect(visibleRows([plainBought], true)).toEqual([]);
  });

  it("allows an expired-unclosed quotation: expiry is a badge, never a block", () => {
    const expired = quotation({
      id: 23,
      quoteNumber: "1502260023",
      validUntil: "2026-01-01",
      expiredUnclosed: true,
    });
    const selected = run([
      { type: "rowsLoaded", page: pageOf(expired) },
      { type: "quotationSelected", quotation: expired },
      { type: "revisionsLoaded", quotationId: 23, revisions: revisions(1) },
    ]);
    expect(selected.step).toBe(2);
    expect(selected.selected).toEqual(expired);
    expect(canContinue(selected)).toBe(true);
  });

  it("refuses to confirm entry when the availability read says closed", () => {
    const complete = atConfirmStep();
    const loaded = poSourceSelectorReducer(complete, {
      type: "availabilityLoaded",
      quotationId: 12,
      revision: 1,
      availability: availability({
        closed: true,
        closedAt: "2026-11-20T03:04:05Z",
        closedReason: "customer cancelled",
      }),
    });
    expect(loaded.step).toBe(3);
    expect(loaded.selectedLeaves.length).toBeGreaterThan(0);
    // Entry must not hand a closed source to Create PO...
    expect(canConfirm(loaded, true)).toBe(false);
    // ...while repair/link keep their existing pair-only rule.
    expect(canConfirm(loaded, false)).toBe(true);

    const open = poSourceSelectorReducer(complete, {
      type: "availabilityLoaded",
      quotationId: 12,
      revision: 1,
      availability: availability(),
    });
    expect(canConfirm(open, true)).toBe(true);
  });
});
