// Issue #467: the PO source selector's decisions, separated from its markup.
//
// Both PO entry paths (the document home's Purchase Order surface and, later,
// an "Add Purchase Order from a Quotation" action) use this same three-step
// machine: choose a Quotation, choose one of its issued revisions, confirm.
// Keeping the transitions pure is what lets the state machine be tested
// without a DOM, and it is the contract #468 resumes work against.

/** One selectable Quotation, as GET /v1/po-sources answers it. */
export interface POSourceQuotation {
  id: number;
  quoteNumber: string;
  subject: string;
  customerName: string;
  latestRevision: number;
  issuedRevisionCount: number;
  /** Eligibility: true once a PO occupies this Quotation (#468). */
  hasActivePO: boolean;
}

/** One page of the first step. PageSize is the server's fixed 15. */
export interface POSourcePage {
  quotations: POSourceQuotation[];
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
}

/** One issued revision of the selected Quotation. */
export interface POSourceRevision {
  revision: number;
  issuedAt: string;
}

/** The exact pair the selector returns and later tickets link a PO to. */
export interface POSourceSelection {
  quotationId: number;
  revisionNumber: number;
}

/** The confirm-time revalidation answer. */
export interface POSourceValidation {
  valid: boolean;
  reason?: string;
}

/** The first step's request. */
export interface POSourceQuery {
  page: number;
  search: string;
  hideUsed: boolean;
}

export type POSourceStep = 1 | 2 | 3;

export interface POSourceSelectorState {
  step: POSourceStep;
  search: string;
  page: number;
  hideUsed: boolean;
  rows: POSourceQuotation[];
  pageSize: number;
  total: number;
  totalPages: number;
  rowsLoading: boolean;
  rowsError: string | null;
  selected: POSourceQuotation | null;
  revisions: POSourceRevision[];
  revisionsLoading: boolean;
  revisionsError: string | null;
  selectedRevision: number | null;
  confirming: boolean;
  confirmError: string | null;
}

export const initialPOSourceSelectorState: POSourceSelectorState = {
  step: 1,
  search: "",
  page: 1,
  hideUsed: false,
  rows: [],
  pageSize: 15,
  total: 0,
  totalPages: 1,
  rowsLoading: false,
  rowsError: null,
  selected: null,
  revisions: [],
  revisionsLoading: false,
  revisionsError: null,
  selectedRevision: null,
  confirming: false,
  confirmError: null,
};

export type POSourceSelectorAction =
  | { type: "rowsLoading" }
  | { type: "rowsLoaded"; page: POSourcePage }
  | { type: "rowsFailed"; error: string }
  | { type: "searchChanged"; search: string }
  | { type: "pageChanged"; page: number }
  | { type: "hideUsedChanged"; hideUsed: boolean }
  | { type: "quotationSelected"; quotation: POSourceQuotation }
  | { type: "revisionsLoading" }
  | { type: "revisionsLoaded"; quotationId: number; revisions: POSourceRevision[] }
  | { type: "revisionsFailed"; quotationId: number; error: string }
  | { type: "revisionSelected"; revision: number }
  | { type: "continue" }
  | { type: "back" }
  | { type: "confirmStarted" }
  | { type: "confirmFailed"; error: string }
  | { type: "reset" };

/** Whether step 2 may advance to confirmation. */
export function canContinue(state: POSourceSelectorState): boolean {
  return (
    state.step === 2 &&
    state.selected !== null &&
    state.selectedRevision !== null &&
    !state.revisionsLoading
  );
}

/**
 * Whether the confirm button may run. Step 3 alone is not enough: an
 * incomplete pair must never become a committed link.
 */
export function canConfirm(state: POSourceSelectorState): boolean {
  return (
    state.step === 3 &&
    state.selected !== null &&
    state.selectedRevision !== null &&
    !state.confirming
  );
}

/** The exact payload the caller receives, or null while incomplete. */
export function selectionOf(
  state: POSourceSelectorState
): POSourceSelection | null {
  if (state.step < 2 || state.selected === null || state.selectedRevision === null) {
    return null;
  }
  return {
    quotationId: state.selected.id,
    revisionNumber: state.selectedRevision,
  };
}

/** Rows the first step draws: used Quotations are hidden only when asked. */
export function visibleRows(
  rows: POSourceQuotation[],
  hideUsed: boolean
): POSourceQuotation[] {
  return hideUsed ? rows.filter((row) => !row.hasActivePO) : rows;
}

export function poSourceSelectorReducer(
  state: POSourceSelectorState,
  action: POSourceSelectorAction
): POSourceSelectorState {
  switch (action.type) {
    case "reset":
      return initialPOSourceSelectorState;
    case "rowsLoading":
      return { ...state, rowsLoading: true, rowsError: null };
    case "rowsLoaded":
      return {
        ...state,
        rows: action.page.quotations,
        page: action.page.page,
        pageSize: action.page.pageSize,
        total: action.page.total,
        totalPages: action.page.totalPages,
        rowsLoading: false,
        rowsError: null,
      };
    case "rowsFailed":
      return {
        ...state,
        rows: [],
        rowsLoading: false,
        rowsError: action.error,
      };
    case "searchChanged":
      return { ...state, search: action.search, page: 1 };
    case "pageChanged":
      return action.page < 1 ? state : { ...state, page: action.page };
    case "hideUsedChanged":
      return { ...state, hideUsed: action.hideUsed, page: 1 };
    case "quotationSelected":
      // A used row is non-interactive in the view; the reducer refuses it too,
      // so a mis-dispatched click cannot create a second active PO.
      if (state.step !== 1 || action.quotation.hasActivePO) return state;
      return {
        ...state,
        step: 2,
        selected: action.quotation,
        revisions: [],
        revisionsLoading: true,
        revisionsError: null,
        selectedRevision: null,
        confirmError: null,
      };
    case "revisionsLoading":
      return { ...state, revisionsLoading: true, revisionsError: null };
    case "revisionsLoaded": {
      // A late answer for a Quotation the user has left must not replace the
      // current step's revisions.
      if (state.selected === null || state.selected.id !== action.quotationId) {
        return state;
      }
      const sole = action.revisions.length === 1 ? action.revisions[0].revision : null;
      const stillListed =
        state.selectedRevision !== null &&
        action.revisions.some((revision) => revision.revision === state.selectedRevision);
      return {
        ...state,
        revisions: action.revisions,
        revisionsLoading: false,
        revisionsError: null,
        selectedRevision: stillListed ? state.selectedRevision : sole,
      };
    }
    case "revisionsFailed":
      if (state.selected === null || state.selected.id !== action.quotationId) {
        return state;
      }
      return {
        ...state,
        revisions: [],
        revisionsLoading: false,
        revisionsError: action.error,
        selectedRevision: null,
      };
    case "revisionSelected":
      if (state.step !== 2) return state;
      return { ...state, selectedRevision: action.revision };
    case "continue":
      if (!canContinue(state)) return state;
      return { ...state, step: 3, confirmError: null };
    case "back":
      if (state.step === 3) {
        return { ...state, step: 2, confirmError: null };
      }
      if (state.step === 2) {
        return {
          ...state,
          step: 1,
          selected: null,
          revisions: [],
          revisionsLoading: false,
          revisionsError: null,
          selectedRevision: null,
          confirmError: null,
        };
      }
      return state;
    case "confirmStarted":
      if (!canConfirm(state)) return state;
      return { ...state, confirming: true, confirmError: null };
    case "confirmFailed":
      return { ...state, confirming: false, confirmError: action.error };
    default:
      return state;
  }
}
