// Issue #467: the PO source selector's decisions, separated from its markup.
//
// Both PO entry paths (the document home's Purchase Order surface and, later,
// an "Add Purchase Order from a Quotation" action) use this same three-step
// machine: choose a Quotation, choose one of its issued revisions, confirm.
// Keeping the transitions pure is what lets the state machine be tested
// without a DOM, and it is the contract #468 resumes work against.

/** The active Purchase Order occupying a Quotation (issue #468). */
export interface POSourceActivePO {
  id: number;
  state: "draft" | "saved";
}

/** One selectable Quotation, as GET /v1/po-sources answers it. */
export interface POSourceQuotation {
  id: number;
  quoteNumber: string;
  subject: string;
  customerName: string;
  latestRevision: number;
  issuedRevisionCount: number;
  /**
   * The newest PO for this Quotation, or null when none exists. A draft may be
   * reopened; a saved record may be viewed. A quotation may have many POs, so
   * this is not an occupancy flag.
   */
  activePO: POSourceActivePO | null;
  /** Bought-state badges (#651). AllBought disables a new PO. */
  allBought: boolean;
  leafCount: number;
  remainingLeafCount: number;
  savedPoCount: number;
  draftPoCount: number;
  legacyHeaderDiscount: boolean;
}

/** One priced leaf of the selected revision, from the availability read. */
export interface POSourceAvailabilityItem {
  quotationItemNo: number;
  quotationSubNo: number | null;
  label: string;
  description: string;
  parentDescription?: string;
  groupKey?: string;
  descriptionLines?: string[];
  uom: string;
  quotedQuantityScaled: number;
  boughtQuantityScaled: number;
  draftQuantityScaled: number;
  remainingQuantityScaled: number;
  overBoughtQuantityScaled: number;
  quotedUnitPriceMinor: number;
}

/** Server-side bought state for one quotation revision (#651). */
export interface POSourceAvailability {
  quotationId: number;
  revisionNumber: number;
  latestRevision: number;
  allBought: boolean;
  legacyHeaderDiscount: boolean;
  savedPoCount: number;
  draftPoCount: number;
  items: POSourceAvailabilityItem[];
}

/** One leaf the caller wants in the new draft. */
export interface POSourceSelectionItem {
  quotationItemNo: number;
  quotationSubNo: number | null;
}

/** Stable identity of a quotation leaf inside one revision. */
export function availabilityLeafKey(item: {
  quotationItemNo: number;
  quotationSubNo: number | null;
}): string {
  return item.quotationSubNo === null
    ? `${item.quotationItemNo}.`
    : `${item.quotationItemNo}.${item.quotationSubNo}`;
}

/** Leaves the workspace may order: remaining quantity only. */
export function selectableAvailabilityItems(
  availability: POSourceAvailability | null
): POSourceAvailabilityItem[] {
  if (availability === null) return [];
  return availability.items.filter(
    (item) => item.remainingQuantityScaled > 0
  );
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
  /**
   * Repair mode (#475): the one saved record being repaired, whose own
   * Quotation must stay selectable. Absent for ordinary PO entry.
   */
  excludePOID?: number;
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
  availability: POSourceAvailability | null;
  availabilityLoading: boolean;
  availabilityError: string | null;
  /** Leaf keys the new draft will contain; defaults to every remaining leaf. */
  selectedLeaves: string[];
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
  availability: null,
  availabilityLoading: false,
  availabilityError: null,
  selectedLeaves: [],
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
  | { type: "availabilityLoading" }
  | {
      type: "availabilityLoaded";
      quotationId: number;
      revision: number;
      availability: POSourceAvailability;
    }
  | { type: "availabilityFailed"; quotationId: number; error: string }
  | { type: "leafToggled"; key: string }
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
 * incomplete pair must never become a committed link. In the ordinary entry
 * flow the availability read must also have answered with at least one leaf
 * selected; repair and link modes keep their existing pair-only rule.
 */
export function canConfirm(
  state: POSourceSelectorState,
  requireAvailability = false
): boolean {
  if (
    state.step !== 3 ||
    state.selected === null ||
    state.selectedRevision === null ||
    state.confirming
  ) {
    return false;
  }
  if (!requireAvailability) return true;
  return (
    !state.availabilityLoading &&
    state.availabilityError === null &&
    state.availability !== null &&
    state.selectedLeaves.length > 0
  );
}

/** The leaves the confirmed draft will contain, or null before they load. */
export function selectionItemsOf(
  state: POSourceSelectorState
): POSourceSelectionItem[] | null {
  if (state.availability === null) return null;
  const selected = new Set(state.selectedLeaves);
  return selectableAvailabilityItems(state.availability)
    .filter((item) => selected.has(availabilityLeafKey(item)))
    .map((item) => ({
      quotationItemNo: item.quotationItemNo,
      quotationSubNo: item.quotationSubNo,
    }));
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

/** Rows the first step draws: occupied Quotations are hidden only when asked. */
export function visibleRows(
  rows: POSourceQuotation[],
  hideUsed: boolean
): POSourceQuotation[] {
  return hideUsed ? rows.filter((row) => !row.allBought) : rows;
}

/** A draft already occupies the Quotation and can be reopened. */
export function draftOf(row: POSourceQuotation): POSourceActivePO | null {
  return row.activePO !== null && row.activePO.state === "draft"
    ? row.activePO
    : null;
}

/** The first step's typing pause, in milliseconds. */
export const PO_SOURCE_SEARCH_PAUSE_MS = 250;

/**
 * How long the first step waits before querying. Only a changed, non-empty
 * search is paused: typing would otherwise send one request per keystroke.
 * Paging, the used filter, the first render and clearing the box are explicit
 * actions and query immediately. `previousSearch` is the term behind the last
 * request (or "" on first render).
 */
export function poSourceQueryDelay(
  previousSearch: string,
  search: string
): number {
  if (previousSearch === search) return 0;
  return search.trim() === "" ? 0 : PO_SOURCE_SEARCH_PAUSE_MS;
}

/**
 * The focus stop a Tab press lands on, wrapping at the dialog's edges.
 * Returns -1 when the dialog has no focusable control. `current` is -1 when
 * the dialog container itself holds focus (or the active element left the
 * list), which sends forward Tab to the first stop and Shift+Tab to the last.
 */
export function tabStopIndex(
  count: number,
  current: number,
  shift: boolean
): number {
  if (count < 1) return -1;
  if (current < 0 || current >= count) return shift ? count - 1 : 0;
  return shift ? (current - 1 + count) % count : (current + 1) % count;
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
      // Only a fully bought quotation is non-interactive (#651). A partial
      // quotation stays selectable and may produce another PO.
      if (state.step !== 1 || action.quotation.allBought) return state;
      return {
        ...state,
        step: 2,
        selected: action.quotation,
        revisions: [],
        revisionsLoading: true,
        revisionsError: null,
        selectedRevision: null,
        availability: null,
        availabilityLoading: false,
        availabilityError: null,
        selectedLeaves: [],
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
      return {
        ...state,
        selectedRevision: action.revision,
        availability: null,
        availabilityLoading: false,
        availabilityError: null,
        selectedLeaves: [],
      };
    case "availabilityLoading":
      return {
        ...state,
        availabilityLoading: true,
        availabilityError: null,
      };
    case "availabilityLoaded":
      if (
        state.selected === null ||
        state.selected.id !== action.quotationId ||
        state.selectedRevision !== action.revision
      ) {
        return state;
      }
      return {
        ...state,
        availability: action.availability,
        availabilityLoading: false,
        availabilityError: null,
        selectedLeaves: selectableAvailabilityItems(action.availability).map(
          availabilityLeafKey
        ),
      };
    case "availabilityFailed":
      if (state.selected === null || state.selected.id !== action.quotationId) {
        return state;
      }
      return {
        ...state,
        availability: null,
        availabilityLoading: false,
        availabilityError: action.error,
        selectedLeaves: [],
      };
    case "leafToggled": {
      const available = new Set(
        selectableAvailabilityItems(state.availability).map(availabilityLeafKey)
      );
      if (!available.has(action.key)) return state;
      const selected = new Set(state.selectedLeaves);
      if (selected.has(action.key)) {
        selected.delete(action.key);
      } else {
        selected.add(action.key);
      }
      return { ...state, selectedLeaves: Array.from(selected) };
    }
    case "continue":
      if (!canContinue(state)) return state;
      return {
        ...state,
        step: 3,
        availability: null,
        availabilityLoading: false,
        availabilityError: null,
        selectedLeaves: [],
        confirmError: null,
      };
    case "back":
      if (state.step === 3) {
        return {
          ...state,
          step: 2,
          availability: null,
          availabilityLoading: false,
          availabilityError: null,
          selectedLeaves: [],
          confirmError: null,
        };
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
          availability: null,
          availabilityLoading: false,
          availabilityError: null,
          selectedLeaves: [],
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
