import { describe, expect, it } from "vitest";
import {
  canConfirm,
  canContinue,
  initialPOSourceSelectorState,
  poSourceSelectorReducer,
  selectionOf,
  visibleRows,
} from "./poSelector";
import type {
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
    hasActivePO: false,
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

  it("refuses to select a Quotation that already has an active PO", () => {
    const used = quotation({ hasActivePO: true });
    const state = run([
      { type: "rowsLoaded", page: pageOf(used) },
      { type: "quotationSelected", quotation: used },
    ]);
    expect(state.step).toBe(1);
    expect(state.selected).toBeNull();
  });

  it("filters used rows only when the filter is on, and always hides them from selection", () => {
    const available = quotation({ id: 1, quoteNumber: "1502260001" });
    const used = quotation({ id: 2, quoteNumber: "1502260002", hasActivePO: true });
    expect(visibleRows([available, used], false)).toEqual([available, used]);
    expect(visibleRows([available, used], true)).toEqual([available]);
    // The filter is a view convenience; eligibility is still enforced on the
    // row itself, so a used row that slips back into view stays unselectable.
    const state = run([
      { type: "rowsLoaded", page: pageOf(available, used) },
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
