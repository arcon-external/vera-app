import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import {
  CorrectPurchaseOrder,
  CreatePurchaseOrderBlankDraft,
  FinalizePurchaseOrder,
  GetPurchaseOrderReview,
  LinkPurchaseOrderDraftSource,
  PreviewPurchaseOrderDraftLink,
  UpdatePurchaseOrderDraft,
} from "@backend/backend/Quotation";
import { backend } from "@backend/models";
import type { CompanyUnitOption } from "../companyUnits";
import { errorMessage } from "../errors";
import { formatMoney, type CurrencyInfo } from "../format";
import {
  conflictNeedsCorrection,
  correctionReasonError,
  trimCorrectionReason,
  writeOutcome,
  type PurchaseOrderConflictDTO,
} from "../poCorrection";
import {
  buildFinalizeInput,
  evaluateReview,
  initialFinalizeReviewState,
  type POReviewChoice,
  type POReviewDecisions,
  type PurchaseOrderReviewDTO,
  type PurchaseOrderSourceLineDTO,
} from "../poFinalize";
import {
  buildPurchaseOrderUpdate,
  calculatePurchaseOrder,
  listValidationBlockers,
  initialPurchaseOrderEditorState,
  isPurchaseOrderDirty,
  parseDiscountPercent,
  purchaseOrderEditorReducer,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorAction,
  type PurchaseOrderEditorState,
  type PurchaseOrderLineCalculation,
  type PurchaseOrderUpdateInput,
} from "../purchaseOrder";
import {
  draftLinkConflictWarning,
  planDraftLink,
  type PurchaseOrderDraftLinkReviewDTO,
  type PurchaseOrderDraftTaxChoice,
} from "../poDraftLink";
import { PurchaseOrderDocumentPanel } from "./PurchaseOrderDocumentPanel";
import { PurchaseOrderConflictPrompt } from "./PurchaseOrderConflictPrompt";
import {
  PurchaseOrderDifferenceReviewView,
  PurchaseOrderFinalConfirmationView,
} from "./PurchaseOrderFinalize";
import {
  PurchaseOrderDraftLinkReviewPanel,
  PurchaseOrderSourceLinkPanel,
} from "./PurchaseOrderSourceLinkPanel";
import { PurchaseOrderTotals } from "./PurchaseOrderTotals";
import { ReconciliationTotalsStrip } from "./ReconciliationTotalsStrip";
import { Select } from "./Select";
import {
  SERVICE_CATEGORY_OPTIONS,
  TAX_TYPE_OPTIONS,
  TAX_ZONE_OPTIONS,
} from "../tax";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";
import {
  HEADER_FIELDS,
  prefillDecisionsFromSession,
  type CommitNotice,
  type HeaderFieldKey,
  type ReconciliationItem,
  type ReconciliationSession,
  type ReconciliationSourceValues,
} from "../poReconciliation";
import {
  explainTotalsDifference,
  headerDiscountDifference,
  lineMoneyDifferences,
  sourceLineRef,
  type LineField,
} from "../poReconciliation.lines";
import {
  wailsReconciliationTransport,
  type ReconciliationTransport,
} from "../poReconciliationAdapter";
import { useReconciliationSession } from "../useReconciliationSession";
import {
  ReconciliationPaneContent,
  ReconciliationSheetTrigger,
  type ReconciliationPaneHandle,
} from "./ReconciliationPane";

// Issue #468: the PO draft workspace. The header keeps the customer snapshot
// read-only (a directory edit must not rewrite this order) while the agreed
// commercial details stay editable; the lines are the customer's order, with
// quantity as a 4-decimal string and money in the PO's own currency.
//
// The calculation (#470) is shown beside the entry: row and whole-PO discounts,
// customer-facing PPN, one explained adjustment, and the grand total. The source
// quotation's printed figures stay in their own group. Differences, PDF and
// confirmation number belong to #471/#472/#474. A draft is deliberately
// unfinished, so Save draft only stores what was entered.

export interface PurchaseOrderEditorViewProps {
  state: PurchaseOrderEditorState;
  dispatch: (action: PurchaseOrderEditorAction) => void;
  currency: CurrencyInfo;
  /** The loaded currency catalogue; the basis is editable before linking. */
  currencyOptions?: CurrencyInfo[];
  uomOptions: CompanyUnitOption[];
  incotermOptions: { value: string; label: string }[];
  /** Safe deferred linking for #497; it never replaces or creates a record. */
  onLinkQuotation?: () => void;
  onSave: () => void;
  onCancel: () => void;
  /** Opens the final-save difference review (#471). */
  onFinalSave: () => void;
  /** True while the review's exact source is being loaded. */
  finalSaving: boolean;
  /** "correction" replaces a saved record and requires the reason (#473). */
  mode?: "draft" | "correction";
  correctionReason?: string;
  onCorrectionReasonChange?: (value: string) => void;
  /** Shown read-only in correction mode; there is no input for it. */
  confirmationNumber?: string;
  /**
   * Ticket 02: the Save draft / Final Save buttons live in the shell action
   * bar instead of this footer. When true the footer keeps only Cancel and
   * the status line, so the two primary actions never render twice. The
   * standalone harnesses leave this unset and keep the footer buttons.
   */
  actionsInShell?: boolean;
  /**
   * Ticket 04: live reconciliation wiring. Notices render under their fields,
   * the unresolved anchors open the pane, and blur commits the field. Absent
   * in the standalone harnesses, which keep the static form.
   */
  notices?: Record<HeaderFieldKey, CommitNotice>;
  validationErrors?: Record<HeaderFieldKey, string | null>;
  unresolvedByField?: Record<HeaderFieldKey, ReconciliationItem | null>;
  onCommitField?: (field: HeaderFieldKey, explicitValue?: string) => void;
  onFocusField?: (field: HeaderFieldKey) => void;
  onFocusLine?: (lineKey: string, field: LineField) => void;
  /** F2: resolves an editor alias to its first-seen session key. */
  sessionKeyFor?: (clientKey: string) => string;
  onShowItem?: (key: string) => void;
  /** U1: which line's match select is expanded (only one at a time). */
  openMatchLineKey?: string | null;
  onToggleMatch?: (lineKey: string) => void;
  /** U1: removal that moves focus and announces; falls back to plain dispatch. */
  onRemoveLine?: (lineKey: string) => void;
  /** U1: adding a line focuses the new description and announces it. */
  onAddLine?: () => void;
  /**
   * Ticket 05: line reconciliation wiring. Match choices come from the
   * linked source; blur commits each line field; feedback keys are
   * `clientKey:field`. Absent in harnesses without a source.
   */
  matchOptions?: { value: string; label: string }[];
  onMatchChange?: (lineKey: string, quotationItemNo: number | null, quotationSubNo: number | null) => void;
  onCommitLine?: (
    lineKey: string,
    field: LineField,
    explicitRaw?: string | { enabled: boolean; percent: string | number }
  ) => void;
  lineNotices?: Record<string, CommitNotice | null>;
  lineErrors?: Record<string, string | null>;
  unresolvedByLineField?: Record<string, ReconciliationItem | null>;
  /**
   * Ticket 04: the Final Save gate also applies to the standalone footer.
   * When unresolved items block, the button stays focusable with
   * aria-disabled and activates the same blocked path as the shell slot.
   */
  gateUnresolvedCount?: number;
  gateBlockedMessage?: string | null;
  onBlockedActivate?: () => void;
}

export interface PurchaseOrderEditorPageProps {
  order: PurchaseOrderDTO;
  currency: CurrencyInfo;
  currencyOptions?: CurrencyInfo[];
  uomOptions: CompanyUnitOption[];
  incotermOptions: { value: string; label: string }[];
  /** Safe deferred linking for #497; App currently explains the next step. */
  onLinkQuotation?: () => void;
  /**
   * Reports the stored record. `corrected` is true when this save went through
   * the correction flow, including a draft that was transitioned into one after
   * an intervening finalization (wave 2 of #473), so the page can word its
   * success message truthfully.
   */
  onSaved: (order: PurchaseOrderDTO, corrected?: boolean) => void;
  onCancel: () => void;
  /**
   * Reports whether the draft holds unsaved changes, so the page that owns
   * navigation (App) can ask before unmounting this editor. The editor stays
   * the only place that decides dirtiness; App never re-derives it.
   */
  onDirtyChange?: (dirty: boolean) => void;
  /**
   * Wave 2 of #473: the editor moved a draft write into the correction flow
   * after an intervening finalization. App should treat this record as a
   * correction from now on, so Cancel returns to the saved view instead of the
   * archive.
   */
  onCorrectionStart?: () => void;
  /** "correction" edits a saved record; every save then needs a reason. */
  mode?: "draft" | "correction";
  /**
   * #497: the exact source App's shared selector confirmed for this draft.
   * The editor persists its working copy, loads the review and clears the
   * target through onLinkTargetHandled.
   */
  pendingLinkTarget?: { quotationId: number; revisionNumber: number } | null;
  onLinkTargetHandled?: () => void;
  /** #497: the link committed; App words its success message. */
  onLinked?: (order: PurchaseOrderDTO) => void;
  /**
   * Ticket 02: renders Save draft / Final Save in the shell action bar. When
   * true the footer keeps only Cancel; the entry view reports its actions
   * through onShellActions so App can render them in the sticky shell slot
   * that stays reachable while the form scrolls. Review, confirm and link
   * views keep their own buttons and report null.
   */
  actionsInShell?: boolean;
  onShellActions?: (actions: POEditorShellActions | null) => void;
  /**
   * Ticket 04: the reconciliation backend. Defaults to the Wails bindings;
   * component tests pass a fake. The pane never touches bindings directly.
   */
  reconciliationTransport?: ReconciliationTransport;
  /** Attribution for local keep/use decisions; the server stamps its own. */
  actorId?: number;
  actorName?: string;
}

/**
 * Ticket 02: the PO editor's primary actions as App's shell renders them.
 * The editor owns the enabled rules (saving, source-less) and the callbacks;
 * App only places the buttons in the sticky action-bar slot. Kept separate
 * from the footer markup so Jessy's visual follow-up touches one place.
 *
 * Ticket 04 extends the bridge with the reconciliation gate: the unresolved
 * header count, the blocked message, and the blocked-activation path that
 * focuses the first unresolved item instead of saving.
 */
export interface POEditorShellActions {
  mode: "draft" | "correction";
  dirty: boolean;
  saving: boolean;
  reviewLoading: boolean;
  sourceLess: boolean;
  unresolvedCount: number;
  blockedMessage: string | null;
  onSave: () => void;
  onFinalSave: () => void;
  onBlockedActivate: () => void;
}

/**
 * The one discard confirmation. Both paths that can lose a draft's unsaved
 * state -- the editor's Cancel button and App's navigation guard -- open this
 * same component from one pending decision, so two confirmations can never
 * stand at once.
 *
 * It is modal: focus enters on Keep editing, Tab and Shift+Tab wrap inside the
 * dialog, Escape means Keep editing, and closing restores focus to the control
 * that opened it. The caller marks the background inert (AppShell's `inert`
 * prop); keeping that explicit rather than reaching outside this component
 * makes the ownership of the page visible where the page is composed.
 */
const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusableWithin(dialog: HTMLElement): HTMLElement[] {
  return Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
}

export function PurchaseOrderDiscardPrompt({
  onKeepEditing,
  onDiscard,
  returnFocus,
  description = "The draft was saved earlier. Changes since then will be lost.",
}: {
  onKeepEditing: () => void;
  onDiscard: () => void;
  /**
   * The control focused when the decision opened, captured before the shell
   * became inert. Null falls back to whatever holds focus at mount.
   */
  returnFocus?: HTMLElement | null;
  /**
   * Issue #491: the same modal now covers Quotation and Company edits, so the
   * sentence names what is actually being discarded. The default keeps the PO
   * draft wording for every existing call site.
   */
  description?: string;
}) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  // The effect owns the mounting decision; callbacks stay in refs so a parent
  // re-render cannot re-run it and steal focus a second time.
  const keepEditing = useRef(onKeepEditing);
  const discard = useRef(onDiscard);
  keepEditing.current = onKeepEditing;
  discard.current = onDiscard;
  const restoreTo = useRef<HTMLElement | null>(returnFocus ?? null);
  useEffect(() => {
    // A second navigation while the dialog stands (a tab click replacing the
    // Cancel action) moves the restoration target to the newer control.
    if (returnFocus !== null && returnFocus !== undefined) {
      restoreTo.current = returnFocus;
    }
  }, [returnFocus]);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (
      restoreTo.current === null &&
      typeof document !== "undefined" &&
      document.activeElement instanceof HTMLElement
    ) {
      restoreTo.current = document.activeElement;
    }
    const initial = dialog === null ? null : focusableWithin(dialog)[0] ?? dialog;
    initial?.focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        // The dialog owns Escape; App's global shortcut listener must not
        // treat the same key as a page action.
        event.preventDefault();
        event.stopPropagation();
        keepEditing.current();
        return;
      }
      if (event.key !== "Tab" || dialog === null) return;
      const focusable = focusableWithin(dialog);
      if (focusable.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      const inside = dialog.contains(active);
      if (event.shiftKey && (active === first || !inside)) {
        event.preventDefault();
        last.focus();
        return;
      }
      if (!event.shiftKey && (active === last || !inside)) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", onKeyDown, true);
    return () => {
      document.removeEventListener("keydown", onKeyDown, true);
      const target = restoreTo.current;
      if (target !== null && target.isConnected) {
        target.focus();
      }
    };
  }, []);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/45 p-5"
      role="presentation"
    >
      <div className="w-full max-w-md">
        <Card className="border-danger-200 p-4">
          <div
            ref={dialogRef}
            role="alertdialog"
            aria-modal="true"
            aria-label="Discard unsaved changes?"
            tabIndex={-1}
          >
            <p className="text-lead font-medium text-ink-900">
              Discard unsaved changes?
            </p>
            <p className="mt-1 text-small text-ink-500">{description}</p>
            <div className="mt-3 flex flex-wrap gap-2">
              <Button aria-label="Keep editing" onClick={onKeepEditing}>
                Keep editing
              </Button>
              <Button
                variant="danger"
                aria-label="Discard changes"
                onClick={onDiscard}
              >
                Discard changes
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
}

/**
 * The correction payload must not carry the source-basis fields, even when a
 * never-linked draft happens to hold them: a saved-record correction has no
 * editable currency or tax basis and the server refuses those fields by name.
 */
function withoutSourceBasis(input: PurchaseOrderUpdateInput): Record<string, unknown> {
  const { Currency, TaxZone, TaxType, ServiceCategory, ...rest } = input;
  void Currency;
  void TaxZone;
  void TaxType;
  void ServiceCategory;
  return rest;
}

function matchLabel(
  quotationItemNo: number | null,
  quotationSubNo: number | null
): string {
  if (quotationItemNo === null) return "Extra line (no quotation match)";
  if (quotationSubNo === null) return `Quotation item ${quotationItemNo}`;
  return `Quotation item ${quotationItemNo}.${quotationSubNo}`;
}

function Field({
  label,
  value,
  onChange,
  type = "text",
  placeholder,
  onBlur,
  onFocus,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: "text" | "date" | "email" | "number";
  placeholder?: string;
  onBlur?: () => void;
  onFocus?: () => void;
}) {
  return (
    <label className="grid gap-1.5">
      <span className="text-small font-medium text-ink-700">{label}</span>
      <input
        type={type}
        aria-label={label}
        className={fieldClass}
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        onBlur={onBlur}
        onFocus={onFocus}
      />
    </label>
  );
}

function TextAreaField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="grid gap-1.5">
      <span className="text-small font-medium text-ink-700">{label}</span>
      <textarea
        aria-label={label}
        className={`${fieldClass} min-h-20 resize-y`}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

/**
 * One line's entry controls: the customer's description, quantity, UoM and
 * unit price, then the row's own discount and the exact line total it
 * produces. Exported so the snapshot harness can show this row at 360/1280 --
 * the full editor page pushes it below the fold.
 */
export function PurchaseOrderLineRow({
  line,
  index,
  currency,
  uomOptions,
  calculation,
  dispatch,
  matchOptions,
  onMatchChange,
  sessionKey,
  onCommitLine,
  onFocusLine,
  lineNotices,
  lineErrors,
  unresolvedByLineField,
  onShowItem,
  matchOpen = false,
  onToggleMatch,
  onRemoveLine,
}: {
  line: PurchaseOrderEditorState["lines"][number];
  index: number;
  currency: CurrencyInfo;
  uomOptions: CompanyUnitOption[];
  calculation: PurchaseOrderLineCalculation | null;
  dispatch: (action: PurchaseOrderEditorAction) => void;
  /** Quotation match choices; absent when no source is linked. */
  matchOptions?: { value: string; label: string }[];
  onMatchChange?: (quotationItemNo: number | null, quotationSubNo: number | null) => void;
  /**
   * F2: the stable session key for this row's feedback and item lookups,
   * resolved by the editor from the session identities (first-seen wins).
   */
  sessionKey?: string;
  /**
   * Blur-commit of one line field; absent when reconciliation is off.
   * Selects and checkboxes pass their NEW value explicitly (F3) instead of
   * reading React's previous render.
   */
  onCommitLine?: (
    field: LineField,
    explicitRaw?: string | { enabled: boolean; percent: string | number }
  ) => void;
  /** U8: tracks the focused line field so Final Save can commit it first. */
  onFocusLine?: (field: LineField) => void;
  lineNotices?: Record<string, CommitNotice | null>;
  lineErrors?: Record<string, string | null>;
  unresolvedByLineField?: Record<string, ReconciliationItem | null>;
  onShowItem?: (key: string) => void;
  /** U1: whether this row's match select is expanded (only one at a time). */
  matchOpen?: boolean;
  onToggleMatch?: () => void;
  /** U1: removal that also moves focus and announces; falls back to plain dispatch. */
  onRemoveLine?: (lineKey: string) => void;
}) {
  const feedbackKey = (field: LineField) => `${sessionKey ?? line.clientKey}:${field}`;
  const feedback = (field: LineField, label: string) => (
    <HeaderFieldFeedback
      fieldKey={feedbackKey(field)}
      label={label}
      notice={lineNotices?.[feedbackKey(field)] ?? null}
      validationError={lineErrors?.[feedbackKey(field)] ?? null}
      unresolvedItem={unresolvedByLineField?.[feedbackKey(field)] ?? null}
      onShowItem={onShowItem}
    />
  );
  const commit = (field: LineField) => () => onCommitLine?.(field);
  const focus = (field: LineField) => () => onFocusLine?.(field);
  const commitValue = (
    field: LineField,
    explicitRaw: string | { enabled: boolean; percent: string | number }
  ) => onCommitLine?.(field, explicitRaw);
  const matchRef =
    line.quotationItemNo === null
      ? null
      : line.quotationSubNo === null
        ? String(line.quotationItemNo)
        : `${line.quotationItemNo}.${line.quotationSubNo}`;
  const matchValue =
    line.quotationItemNo === null
      ? ""
      : line.quotationSubNo === null
        ? String(line.quotationItemNo)
        : `${line.quotationItemNo}.${line.quotationSubNo}`;
  const rowRef = useRef<HTMLDivElement | null>(null);
  // U1: focusing the revealed match select is part of opening it. The pane's
  // Fix match and the header Change button share this path.
  const wasOpen = useRef(matchOpen);
  useEffect(() => {
    if (matchOpen && !wasOpen.current) {
      rowRef.current
        ?.querySelector<HTMLElement>(`[aria-label="Line ${index + 1} quotation match"]`)
        ?.focus();
    }
    wasOpen.current = matchOpen;
  }, [matchOpen, index]);
  return (
    <div
      ref={rowRef}
      className="grid gap-3 rounded-lg border border-rule bg-white p-3"
      data-po-line={index + 1}
    >
      <div className="flex items-center gap-2">
        <span className="text-small font-medium text-ink-900">
          Line {index + 1}
        </span>
        {matchOptions !== undefined && onMatchChange !== undefined && onToggleMatch !== undefined ? (
          <>
            <span className="inline-flex shrink-0 items-center whitespace-nowrap rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
              {matchRef === null ? "Extra line" : `Quotation ${matchRef}`}
            </span>
            <button
              type="button"
              aria-label={`Change quotation match for line ${index + 1}`}
              aria-expanded={matchOpen}
              onClick={onToggleMatch}
              className="cursor-pointer rounded-md px-2 py-1 text-small font-medium text-accent-700 underline-offset-2 hover:underline focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
            >
              Change
            </button>
          </>
        ) : (
          <span className="text-caption text-ink-400">
            {matchLabel(line.quotationItemNo, line.quotationSubNo)}
          </span>
        )}
        <span className="ml-auto">
          <Button
            size="sm"
            aria-label={`Remove line ${index + 1}`}
            onClick={() => {
              if (onRemoveLine !== undefined) onRemoveLine(line.clientKey);
              else dispatch({ type: "lineRemoved", key: line.key });
            }}
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              aria-hidden="true"
              className="@min-[560px]:hidden"
            >
              <path d="M18 6 6 18M6 6l12 12" />
            </svg>
            <span className="hidden @min-[560px]:inline">Remove</span>
          </Button>
        </span>
      </div>
      {matchOptions !== undefined && onMatchChange !== undefined && matchOpen ? (
        <div className="grid gap-1.5">
          <span className="text-small font-medium text-ink-700" id={`line-${index + 1}-match-heading`}>
            Quotation match
          </span>
          <Select
            ariaLabel={`Line ${index + 1} quotation match`}
            value={matchValue}
            options={matchOptions}
            onChange={(value) => {
              const text = String(value);
              if (text === "") {
                onMatchChange(null, null);
                return;
              }
              const [head, tail] = text.split(".");
              const itemNo = Number(head);
              if (!Number.isInteger(itemNo) || itemNo < 1) return;
              onMatchChange(itemNo, tail === undefined ? null : Number(tail));
            }}
          />
        </div>
      ) : null}
      <div className="grid gap-3 @min-[560px]:grid-cols-[1fr_1fr_1.2fr] @min-[700px]:grid-cols-[minmax(0,1fr)_104px_120px_150px]">
        <label className="grid gap-1.5 @min-[560px]:col-span-full @min-[700px]:col-span-1">
          <span className="text-small font-medium text-ink-700">
            Description
          </span>
          <input
            aria-label={`Line ${index + 1} description`}
            className={fieldClass}
            value={line.description}
            onChange={(event) =>
              dispatch({
                type: "lineChanged",
                key: line.key,
                patch: { description: event.target.value },
              })
            }
            onBlur={commit("description")}
          onFocus={focus("description")}
          />
          {feedback("description", `Line ${index + 1} description`)}
        </label>
        <label className="grid gap-1.5">
          <span className="text-small font-medium text-ink-700">Quantity</span>
          <input
            inputMode="decimal"
            aria-label={`Line ${index + 1} quantity`}
            className={fieldClass}
            value={line.quantity}
            onChange={(event) =>
              dispatch({
                type: "lineChanged",
                key: line.key,
                patch: { quantity: event.target.value },
              })
            }
            onBlur={commit("quantity")}
          onFocus={focus("quantity")}
          />
          {feedback("quantity", `Line ${index + 1} quantity`)}
        </label>
        <div className="grid gap-1.5">
          <span className="text-small font-medium text-ink-700">UoM</span>
          <Select
            ariaLabel={`Line ${index + 1} unit of measure`}
            value={line.uom}
            options={uomOptions.length > 0 ? uomOptions : [{ value: line.uom, label: line.uom || "—" }]}
            onChange={(value) => {
              const next = String(value);
              dispatch({
                type: "lineChanged",
                key: line.key,
                patch: { uom: next },
              });
              commitValue("uom", next);
            }}
          />
          {feedback("uom", `Line ${index + 1} unit of measure`)}
        </div>
        <label className="grid gap-1.5 @min-[560px]:col-span-full @min-[700px]:col-span-1">
          <span className="text-small font-medium text-ink-700">
            Unit price ({currency.code})
          </span>
          <input
            inputMode="decimal"
            aria-label={`Line ${index + 1} unit price`}
            className={fieldClass}
            value={line.unitPrice}
            onChange={(event) =>
              dispatch({
                type: "lineChanged",
                key: line.key,
                patch: { unitPrice: event.target.value },
              })
            }
            onBlur={commit("unitPrice")}
          onFocus={focus("unitPrice")}
          />
          {feedback("unitPrice", `Line ${index + 1} unit price`)}
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-rule pt-2">
        <label className="flex items-center gap-2 text-small text-ink-700">
          <input
            type="checkbox"
            aria-label={`Line ${index + 1} discount enabled`}
            checked={line.discountEnabled}
            onChange={(event) => {
              const enabled = event.target.checked;
              dispatch({
                type: "lineChanged",
                key: line.key,
                patch: { discountEnabled: enabled },
              });
              commitValue("discount", { enabled, percent: line.discountPercent });
            }}
            onBlur={commit("discount")}
            onFocus={focus("discount")}
          />
          Row discount
        </label>
        <div className="flex items-center gap-1">
          <input
            inputMode="decimal"
            aria-label={`Line ${index + 1} discount percent`}
            className={`${fieldClass} w-20`}
            value={line.discountPercent}
            disabled={!line.discountEnabled}
            onChange={(event) =>
              dispatch({
                type: "lineChanged",
                key: line.key,
                patch: { discountPercent: event.target.value },
              })
            }
            onBlur={commit("discount")}
            onFocus={focus("discount")}
          />
          <span className="text-small text-ink-500">%</span>
        </div>
        {feedback("discount", `Line ${index + 1} discount`)}
        <span className="ml-auto tnum whitespace-nowrap text-small text-ink-700">
          {calculation === null
            ? "Line total —"
            : `Line total ${formatMoney(currency, calculation.netMinor)}`}
        </span>
        {calculation !== null && calculation.discountMinor > 0 ? (
          <span className="tnum whitespace-nowrap text-caption text-ink-500">
            Discount {formatMoney(currency, -calculation.discountMinor)}
          </span>
        ) : null}
      </div>
    </div>
  );
}
/**
 * Ticket 04: the inline feedback under a committed header field. A notice is
 * inline feedback and never a review item; the pane owns decisions. An
 * unresolved item instead shows the quiet anchor that focuses it in the pane.
 */
function HeaderFieldFeedback({
  fieldKey,
  label,
  notice,
  validationError,
  unresolvedItem,
  onShowItem,
}: {
  fieldKey: string;
  label: string;
  notice: CommitNotice | undefined;
  validationError: string | null | undefined;
  unresolvedItem: ReconciliationItem | null | undefined;
  onShowItem: ((key: string) => void) | undefined;
}) {
  if (validationError !== null && validationError !== undefined) {
    return (
      <p className="text-caption leading-relaxed text-danger-700">{validationError}</p>
    );
  }
  if (unresolvedItem !== null && unresolvedItem !== undefined && onShowItem !== undefined) {
    return (
      <p className="text-caption leading-relaxed text-accent-700">
        Review · differs from the Quotation{" "}
        <button
          type="button"
          aria-label={`Show review item for ${label}`}
          onClick={() => onShowItem(unresolvedItem.Key)}
          className="cursor-pointer font-medium underline underline-offset-2 hover:text-accent-800 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
        >
          Show
        </button>
      </p>
    );
  }
  if (notice === "empty") {
    return (
      <p className="flex items-center gap-1.5 text-caption leading-relaxed text-ink-500">
        <span aria-hidden="true" className="inline-block h-1.5 w-1.5 rounded-full border border-ink-300" />
        {label} is empty
      </p>
    );
  }
  if (notice === "matched") {
    return (
      <p className="flex items-center gap-1.5 text-caption leading-relaxed text-success-700">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
          <path d="M2.5 8.5l3.5 3.5 7-8" />
        </svg>
        Matches the Quotation
      </p>
    );
  }
  return null;
}

export function PurchaseOrderEditorView({
  state,
  dispatch,
  currency,
  currencyOptions = [],
  uomOptions,
  incotermOptions,
  onLinkQuotation,
  onSave,
  onCancel,
  onFinalSave,
  finalSaving,
  mode = "draft",
  correctionReason = "",
  onCorrectionReasonChange,
  confirmationNumber = "",
  actionsInShell = false,
  notices,
  validationErrors,
  unresolvedByField,
  onCommitField,
  onFocusField,
  onFocusLine,
  onShowItem,
  sessionKeyFor,
  openMatchLineKey,
  onToggleMatch,
  onRemoveLine,
  onAddLine,
  matchOptions,
  onMatchChange,
  onCommitLine,
  lineNotices,
  lineErrors,
  unresolvedByLineField,
  gateUnresolvedCount = 0,
  gateBlockedMessage = null,
  onBlockedActivate,
}: PurchaseOrderEditorViewProps) {
  const dirty = isPurchaseOrderDirty(state);
  const correcting = mode === "correction";
  const activeCurrency =
    currencyOptions.find((option) => option.code === state.currency) ?? currency;
  // The panel and the save guard read the same pure calculation, so the number
  // on screen is the number the payload will carry.
  const calculation = calculatePurchaseOrder(state, activeCurrency);
  const wholeDiscountPercent = parseDiscountPercent(state.header.discountPercent);
  const neverLinked = state.sourceState === "never";
  const sourceLess = state.sourceState !== "live";
  const transient = state.orderId === 0;
  const basisLabel =
    state.taxType !== ""
      ? `${state.taxType}${state.serviceCategory !== "" ? ` / ${state.serviceCategory}` : ""}`
      : state.taxCategory;
  return (
    <div className="animate-step-in flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading
            title="Purchase Order"
            hint={
              correcting
                ? `Saved record ${state.orderId} · correction`
                : transient
                  ? "New purchase order"
                  : `Draft ${state.orderId}`
            }
          />
          <h1 className="tnum text-display font-semibold text-ink-950">
            {neverLinked ? "New purchase order" : state.quoteNumber}
          </h1>
          <p className="mt-1 text-body text-ink-500">
            {neverLinked ? (
              <>
                Source missing · {state.currency.toUpperCase()}
                {state.taxZone !== "" ? ` · ${state.taxZone}` : ""}
                {basisLabel !== "" ? ` / ${basisLabel}` : ""}
              </>
            ) : (
              <>
                {state.customerName} ·{" "}
                {state.sourceState === "former"
                  ? `former quotation revision ${state.sourceRevision}`
                  : `revision ${state.sourceRevision}`}{" "}
                · {state.currency.toUpperCase()}
                {state.taxZone !== ""
                  ? ` · ${state.taxZone}${
                      state.taxCategory !== ""
                        ? ` / ${state.taxCategory}`
                        : ""
                    }`
                  : ""}
              </>
            )}
          </p>
          <p className="mt-1 text-caption text-ink-500">
            {correcting
              ? `Editing version ${state.version}. The previous values are kept in the correction history.`
              : transient
                ? "Not saved yet. Save draft stores it so you can continue later."
                : `Loaded at version ${state.version}.`}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {sourceLess ? <Badge tone="danger">Source missing</Badge> : null}
          <Badge tone="accent">{correcting ? "Correction" : "Draft"}</Badge>
        </div>
      </div>

      {!correcting ? (
        <PurchaseOrderSourceLinkPanel
          sourceState={state.sourceState}
          quoteNumber={state.quoteNumber}
          sourceRevision={state.sourceRevision}
          correcting={correcting}
          onChooseSource={onLinkQuotation}
        />
      ) : null}

      {state.error !== null ? (
        <StatusBanner tone="danger">{state.error}</StatusBanner>
      ) : null}

      {correcting ? (
        <Card className="p-4">
          <SectionHeading
            title="Correction"
            hint="A reason is required and kept with the replaced values"
          />
          <label className="mt-2 grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              Reason for this correction
            </span>
            <textarea
              aria-label="Correction reason"
              className={`${fieldClass} min-h-20 resize-y`}
              placeholder="What changed and why, for the people who will read this record later"
              value={correctionReason}
              onChange={(event) => onCorrectionReasonChange?.(event.target.value)}
            />
          </label>
          <div className="mt-3 grid gap-1 text-caption text-ink-500">
            <p>
              <span className="font-medium text-ink-700">Customer PO number:</span>{" "}
              <span className="tnum">{state.header.poNumber || "—"}</span>
            </p>
            <p>
              <span className="font-medium text-ink-700">Confirmation number:</span>{" "}
              <span className="tnum">{confirmationNumber || "Not assigned"}</span>
            </p>
            <p>
              The customer, quotation link, currency, tax basis, UUID and
              confirmation number are protected. They cannot be changed here;
              repairing a quotation link is its own permission (#475).
            </p>
          </div>
        </Card>
      ) : null}

      {/* A never- or formerly-linked draft is still before a link, so the
          source basis stays editable until one commits. */}
      {sourceLess && !correcting ? (
        <Card className="p-4">
          <SectionHeading
            title="Source basis"
            hint={
              state.sourceState === "former"
                ? "Editable until a new quotation is linked; the former source remains the reference and the server computes PPN"
                : "Editable until a quotation is linked; the server computes PPN"
            }
          />
          <div className="mt-3 grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <span className="text-small font-medium text-ink-700">Currency</span>
              <Select
                ariaLabel="Currency"
                value={state.currency}
                options={currencyOptions.map((option) => ({
                  value: option.code,
                  label: `${option.code} (${option.symbol})`,
                }))}
                onChange={(value) => {
                  const next = currencyOptions.find(
                    (option) => option.code === value
                  );
                  if (next === undefined || next.code === state.currency) return;
                  dispatch({
                    type: "currencyChanged",
                    code: next.code,
                    from: activeCurrency,
                    to: next,
                  });
                }}
              />
            </div>
            <div className="grid gap-1.5">
              <span className="text-small font-medium text-ink-700">Tax zone</span>
              <Select
                ariaLabel="Tax zone"
                value={state.taxZone}
                options={TAX_ZONE_OPTIONS.map((option) => ({
                  value: option.value,
                  label: option.label,
                }))}
                onChange={(value) =>
                  dispatch({
                    type: "sourceBasisChanged",
                    patch: { taxZone: String(value) },
                  })
                }
              />
            </div>
            <div className="grid gap-1.5">
              <span className="text-small font-medium text-ink-700">Tax type</span>
              <Select
                ariaLabel="Tax type"
                value={state.taxType}
                options={TAX_TYPE_OPTIONS.map((option) => ({
                  value: option.value,
                  label: option.label,
                }))}
                onChange={(value) =>
                  dispatch({
                    type: "sourceBasisChanged",
                    patch: { taxType: String(value) },
                  })
                }
              />
            </div>
            {state.taxType === "Services" ? (
              <div className="grid gap-1.5">
                <span className="text-small font-medium text-ink-700">
                  Service category
                </span>
                <Select
                  ariaLabel="Service category"
                  value={state.serviceCategory || "NonConstruction"}
                  options={SERVICE_CATEGORY_OPTIONS.map((option) => ({
                    value: option.value,
                    label: option.label,
                  }))}
                  onChange={(value) =>
                    dispatch({
                      type: "sourceBasisChanged",
                      patch: { serviceCategory: String(value) },
                    })
                  }
                />
              </div>
            ) : null}
          </div>
          <p className="mt-3 text-caption text-ink-500">
            Currency and tax basis are PO-owned until a quotation is linked.
            Choosing a different currency never converts an amount; amounts are
            entered in the selected currency. PPN is derived by the server from
            this basis, and linking a quotation replaces it only through an
            explicit review.
          </p>
        </Card>
      ) : null}

      <Card className="p-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <TextAreaField
            label="Billing address"
            value={state.header.billingAddress}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { billingAddress: value } })
            }
          />
          <TextAreaField
            label="Delivery address"
            value={state.header.deliveryAddress}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { deliveryAddress: value } })
            }
          />
          <Field
            label="Ship-to code"
            value={state.header.shipToCode}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { shipToCode: value } })
            }
          />
          <Field
            label="Contact email"
            type="email"
            value={state.header.contactEmail}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { contactEmail: value } })
            }
          />
          <Field
            label="Contact phone"
            value={state.header.contactPhone}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { contactPhone: value } })
            }
          />
          <Field
            label="Customer PO number"
            value={state.header.poNumber}
            placeholder="Optional"
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { poNumber: value } })
            }
          />
          <Field
            label="PO date"
            type="date"
            value={state.header.poDate}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { poDate: value } })
            }
          />
          <Field
            label="Received date"
            type="date"
            value={state.header.receivedDate}
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { receivedDate: value } })
            }
          />
          <Field
            label="Requested delivery date"
            type="date"
            value={state.header.requestedDeliveryDate}
            onChange={(value) =>
              dispatch({
                type: "headerChanged",
                patch: { requestedDeliveryDate: value },
              })
            }
          />
          <Field
            label="Payment terms"
            value={state.header.paymentTerms}
            placeholder="e.g. 50% advance, balance 30 days after delivery"
            onChange={(value) =>
              dispatch({ type: "headerChanged", patch: { paymentTerms: value } })
            }
            onBlur={() => onCommitField?.("header:paymentTerms")}
            onFocus={() => onFocusField?.("header:paymentTerms")}
          />
          <HeaderFieldFeedback
            fieldKey="header:paymentTerms"
            label={HEADER_FIELDS["header:paymentTerms"].label}
            notice={notices?.["header:paymentTerms"]}
            validationError={validationErrors?.["header:paymentTerms"]}
            unresolvedItem={unresolvedByField?.["header:paymentTerms"]}
            onShowItem={onShowItem}
          />
          <div className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">Incoterms</span>
            <Select
              ariaLabel="Incoterms"
              value={state.header.incoterms}
              options={incotermOptions}
              onChange={(value) => {
                const next = String(value);
                dispatch({ type: "headerChanged", patch: { incoterms: next } });
                onCommitField?.("header:incoterms", next);
              }}
            />
            <HeaderFieldFeedback
              fieldKey="header:incoterms"
              label={HEADER_FIELDS["header:incoterms"].label}
              notice={notices?.["header:incoterms"]}
              validationError={validationErrors?.["header:incoterms"]}
              unresolvedItem={unresolvedByField?.["header:incoterms"]}
              onShowItem={onShowItem}
            />
          </div>
        </div>
        <p className="mt-4 text-caption text-ink-500">
          {state.sourceState === "never"
            ? "Customer, quotation link and printed totals are filled when a quotation is linked. Currency and tax basis above are editable until then."
            : state.sourceState === "former"
              ? "The former quotation remains the reference for the printed totals and lines. Currency and tax basis above are editable until a new link."
              : "Customer, currency and tax basis are snapshots of the issued quotation revision and cannot be edited here."}
        </p>
      </Card>

      <div>
        <SectionHeading
          title="Lines"
          hint={`${state.lines.length} ${state.lines.length === 1 ? "line" : "lines"}`}
          action={
            <Button
              size="sm"
              aria-label="Add line"
              onClick={() => {
                if (onAddLine !== undefined) onAddLine();
                else dispatch({ type: "lineAdded", uom: uomOptions[0]?.value ?? "" });
              }}
            >
              Add line
            </Button>
          }
        />
        <div className="grid gap-3">
          {state.lines.length === 0 ? (
            <p className="rounded-lg border border-dashed border-rule px-4 py-8 text-center text-small text-ink-500">
              No lines yet. Add the customer's first line.
            </p>
          ) : (
            state.lines.map((line, index) => (
              <PurchaseOrderLineRow
                key={line.key}
                line={line}
                index={index}
                currency={activeCurrency}
                uomOptions={uomOptions}
                calculation={calculation.lines[index] ?? null}
                dispatch={dispatch}
                matchOpen={openMatchLineKey !== undefined && openMatchLineKey === line.clientKey}
                onToggleMatch={
                  onToggleMatch === undefined
                    ? undefined
                    : () => onToggleMatch(line.clientKey)
                }
                onRemoveLine={onRemoveLine}
                matchOptions={matchOptions}
                onMatchChange={
                  onMatchChange === undefined
                    ? undefined
                    : (quotationItemNo, quotationSubNo) =>
                        onMatchChange(line.clientKey, quotationItemNo, quotationSubNo)
                }
                sessionKey={sessionKeyFor?.(line.clientKey)}
                onCommitLine={
                  onCommitLine === undefined
                    ? undefined
                    : (field, explicitRaw) =>
                        onCommitLine(line.clientKey, field, explicitRaw)
                }
                onFocusLine={
                  onFocusLine === undefined
                    ? undefined
                    : (field) => onFocusLine(line.clientKey, field)
                }
                lineNotices={lineNotices}
                lineErrors={lineErrors}
                unresolvedByLineField={unresolvedByLineField}
                onShowItem={onShowItem}
              />
            ))
          )}
        </div>
      </div>

      <Card className="p-4">
        <SectionHeading
          title="Discounts and adjustment"
          hint="Explained separately from the line prices"
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              Whole-PO discount (%)
            </span>
            <input
              inputMode="decimal"
              aria-label="Whole-PO discount"
              className={fieldClass}
              value={state.header.discountPercent}
              onChange={(event) =>
                dispatch({
                  type: "headerChanged",
                  patch: { discountPercent: event.target.value },
                })
              }
              onBlur={() => onCommitField?.("header:discountPercent")}
              onFocus={() => onFocusField?.("header:discountPercent")}
            />
            <HeaderFieldFeedback
              fieldKey="header:discountPercent"
              label={HEADER_FIELDS["header:discountPercent"].label}
              notice={notices?.["header:discountPercent"]}
              validationError={validationErrors?.["header:discountPercent"]}
              unresolvedItem={unresolvedByField?.["header:discountPercent"]}
              onShowItem={onShowItem}
            />
          </label>
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              Adjustment ({activeCurrency.code})
            </span>
            <input
              inputMode="decimal"
              aria-label="Adjustment"
              className={fieldClass}
              placeholder="Signed, e.g. -12.50"
              value={state.header.adjustment}
              onChange={(event) =>
                dispatch({
                  type: "headerChanged",
                  patch: { adjustment: event.target.value },
                })
              }
            />
          </label>
          <label className="grid gap-1.5 sm:col-span-2">
            <span className="text-small font-medium text-ink-700">
              Adjustment reason
            </span>
            <input
              aria-label="Adjustment reason"
              className={fieldClass}
              placeholder="Required when the adjustment is not zero"
              value={state.header.adjustmentReason}
              onChange={(event) =>
                dispatch({
                  type: "headerChanged",
                  patch: { adjustmentReason: event.target.value },
                })
              }
            />
          </label>
        </div>
        <p className="mt-3 text-caption text-ink-500">
          Row discounts come off each line first, then the whole-PO discount.
          PPN is charged on the discounted subtotal and printed separately. The
          signed adjustment is added last and never changes a row price or
          disguises a discount or tax entry.
        </p>
      </Card>

      {calculation.error !== null ? (
        <StatusBanner tone="danger">{calculation.error}</StatusBanner>
      ) : null}

      <PurchaseOrderTotals
        totals={calculation.totals}
        currency={activeCurrency}
        discountPercent={wholeDiscountPercent}
        ppnBps={state.ppnBps}
        adjustmentReason={state.header.adjustmentReason.trim()}
        printedTotals={state.printedTotals}
        sourceRevision={state.sourceRevision}
        printedTotalsKnown={state.printedTotalsKnown}
      />

      {state.error !== null ? (
        <StatusBanner tone="danger">{state.error}</StatusBanner>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
        <span className="text-caption text-ink-500">
          {dirty ? "Unsaved changes" : "All changes saved"} ·{" "}
          {correcting
            ? "Saving keeps the replaced values and reason in the correction history."
            : sourceLess
              ? "Link a Quotation before Final Save; this draft stays editable and keeps its values."
              : "Final Save reviews every difference against the exact quotation revision."}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          <Button aria-label={correcting ? "Cancel correction" : "Cancel draft"} onClick={onCancel}>
            Cancel
          </Button>
          {actionsInShell ? null : (
            <>
              <Button
                aria-label={correcting ? "Save correction" : "Save draft"}
                disabled={state.saving || finalSaving}
                onClick={onSave}
              >
                {state.saving
                  ? "Saving…"
                  : correcting
                    ? "Save correction"
                    : "Save draft"}
              </Button>
              {correcting ? null : (
                <Button
                  variant="primary"
                  aria-label="Final Save"
                  aria-disabled={gateUnresolvedCount > 0 ? true : undefined}
                  disabled={
                    gateUnresolvedCount === 0 &&
                    (state.saving || finalSaving || sourceLess)
                  }
                  title={
                    gateUnresolvedCount > 0
                      ? gateBlockedMessage ?? undefined
                      : sourceLess
                        ? "Link a Quotation before finalizing; the server also refuses a source-less finalize."
                        : undefined
                  }
                  onClick={() => {
                    if (gateUnresolvedCount > 0) onBlockedActivate?.();
                    else onFinalSave();
                  }}
                >
                  {finalSaving ? "Loading review…" : "Final Save"}
                </Button>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Live focus targets for a §5.5 entry-blocked difference. Comparison keys
 * choose the control: every matched-line field and header maps to its entry
 * control by label, except the row discount whose label ("Line N discount")
 * names neither the percent input nor the enabled checkbox. The enabled
 * percent input commits the shown value unchanged on blur; when the row
 * discount is off its percent input is disabled, so the checkbox is the
 * usable target (toggling it commits for Keep/Use review). Candidates are
 * tried in order; the caller focuses the first existing enabled match.
 */
function gateFocusCandidates(
  key: string,
  label: string,
  lines: { discountEnabled: boolean }[]
): string[] {
  const line = /^line:(\d+):(.+)$/.exec(key);
  if (line !== null && line[2] === "discount") {
    const row = lines[Number(line[1]) - 1];
    const name = `Line ${line[1]}`;
    if (row !== undefined && row.discountEnabled) {
      return [`${name} discount percent`, `${name} discount enabled`];
    }
    return [`${name} discount enabled`, `${name} discount percent`];
  }
  return [label];
}

/**
 * Reveal a live blocker target between the actual sticky chrome and focus
 * it without letting native focus scrolling undo the reveal. The header
 * height comes from the shell's live measurement, and the bottom save bar
 * is measured in place, so the target rests clear of both at the current
 * viewport instead of at a fixed offset. Shared by entry-gate and
 * validation focus routing.
 */
function revealIntoView(target: HTMLElement): void {
  const shell = document.querySelector<HTMLElement>("[data-app-shell]");
  const declared =
    shell === null ? "" : getComputedStyle(shell).getPropertyValue("--shell-header-h");
  const headerH = Number.parseFloat(declared) || 0;
  const bar = document.querySelector<HTMLElement>("div.sticky.bottom-0");
  const barH = bar === null ? 0 : bar.getBoundingClientRect().height;
  const freeTop = headerH + 8;
  const freeBottom = window.innerHeight - barH - 8;
  const rect = target.getBoundingClientRect();
  if (freeBottom > freeTop && (rect.top < freeTop || rect.bottom > freeBottom)) {
    const visibleH = Math.min(rect.height, freeBottom - freeTop);
    const offset = freeTop + Math.max(0, (freeBottom - freeTop - visibleH) / 2);
    window.scrollTo({ top: window.scrollY + rect.top - offset, behavior: "auto" });
  } else if (freeBottom <= freeTop) {
    target.scrollIntoView({ block: "center" });
  }
  target.focus({ preventScroll: true });
}

export function PurchaseOrderEditorPage({
  order,
  currency,
  currencyOptions = [],
  uomOptions,
  incotermOptions,
  onLinkQuotation,
  pendingLinkTarget = null,
  onLinkTargetHandled,
  onLinked,
  onSaved,
  onCancel,
  onDirtyChange,
  onCorrectionStart,
  mode = "draft",
  actionsInShell = false,
  onShellActions,
  reconciliationTransport,
  actorId = 0,
  actorName = "You",
}: PurchaseOrderEditorPageProps) {
  // #473 wave 2: a draft write that meets an already-saved record keeps the
  // working copy and continues as a correction. The transition is local to this
  // editor; App still owns whether the record opened in correction mode.
  const [savedCorrection, setSavedCorrection] = useState(false);
  const correcting = mode === "correction" || savedCorrection;
  const [state, dispatch] = useReducer(
    purchaseOrderEditorReducer,
    undefined,
    () => initialPurchaseOrderEditorState(order, currency)
  );
  const activeCurrency =
    currencyOptions.find((option) => option.code === state.currency) ?? currency;
  // The final-save flow is a view of this page, not a separate route: the
  // review reads the exact source from the server, and every choice edits the
  // one editor state, so the totals on screen are the totals that will be sent.
  const [view, setView] = useState<"entry" | "review" | "confirm">("entry");
  const [review, setReview] = useState<PurchaseOrderReviewDTO | null>(null);
  const [reviewLoading, setReviewLoading] = useState(false);
  const [decisions, setDecisions] = useState<POReviewDecisions>({});
  const [acknowledgedMissing, setAcknowledgedMissing] = useState<string[]>([]);
  const [acknowledgedExtra, setAcknowledgedExtra] = useState<number[]>([]);
  const [documentConfirmed, setDocumentConfirmed] = useState(false);
  // #474: optional and unchecked until staff ask for it on the final save.
  const [assignConfirmation, setAssignConfirmation] = useState(false);
  // The applied payload the confirmation displays and Final save submits. It is
  // built once when Continue is pressed, from the applied choices -- never from
  // the stored record, which may still hold an identifier the editor replaced.
  const [confirmPayload, setConfirmPayload] = useState<PurchaseOrderUpdateInput | null>(
    null
  );
  const [finalSaving, setFinalSaving] = useState(false);
  // #473: the required reason a correction carries, and the one warning a
  // stale save gets. The working copy is never touched by either.
  const [reason, setReason] = useState("");
  const [conflict, setConflict] = useState<PurchaseOrderConflictDTO | null>(null);
  // Which write the warning belongs to: Overwrite resumes the same one.
  const [pendingWrite, setPendingWrite] = useState<"save" | "finalize">("save");

  // #497: the Staff draft link. The review is loaded from the server after the
  // working copy is persisted, so the review compares the values the link will
  // actually keep. Decisions are separate from the final-save review: a link
  // decision is not a finalize decision.
  const [linkReview, setLinkReview] = useState<PurchaseOrderDraftLinkReviewDTO | null>(null);
  const [linkLoading, setLinkLoading] = useState(false);
  const [linkError, setLinkError] = useState<string | null>(null);
  const [linkTaxChoice, setLinkTaxChoice] = useState<PurchaseOrderDraftTaxChoice>("");
  const [linking, setLinking] = useState(false);

  // The async save and upload paths must see the editor as it is now, not as
  // it was when the request was scheduled.
  const editorStateRef = useRef(state);
  editorStateRef.current = state;

  // Ticket 04: live reconciliation. The session the server created for the
  // linked revision, the blur-commit state machine, notices, filters, the
  // keep step and announcements. The editor owns the form values; the hook
  // only reads them at commit time and writes them back on Use Quotation.
  const [compareSource, setCompareSource] = useState<ReconciliationSourceValues | null>(null);
  // Ticket 05: source lines for line matching; loaded with the scalars.
  const [compareLines, setCompareLines] = useState<PurchaseOrderSourceLineDTO[] | null>(null);
  const [openMatchLineKey, setOpenMatchLineKey] = useState<string | null>(null);
  const [pendingFocus, setPendingFocus] = useState<string | null>(null);
  const addLine = () => {
    const at = state.lines.length + 1;
    dispatch({ type: "lineAdded", uom: uomOptions[0]?.value ?? "" });
    setOpenMatchLineKey(null);
    setPendingFocus(`Line ${at} description`);
    reconciliation.announce(`Line ${at} added.`);
  };
  useEffect(() => {
    if (pendingFocus === null) return;
    setPendingFocus(null);
    const target = document.querySelector<HTMLElement>(
      `[aria-label="${pendingFocus.replace(/"/g, "")}"]`
    );
    if (target !== null) {
      revealIntoView(target);
    }
  }, [pendingFocus, state.lines.length]);
  const [sessionBannerError, setSessionBannerError] = useState<string | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const sheetTriggerRef = useRef<HTMLButtonElement | null>(null);
  const paneRailRef = useRef<ReconciliationPaneHandle | null>(null);
  const paneSheetRef = useRef<ReconciliationPaneHandle | null>(null);
  const sheetFocusRef = useRef<
    { kind: "heading" } | { kind: "first" } | { kind: "item"; key: string } | null
  >(null);
  const focusedFieldRef = useRef<HeaderFieldKey | null>(null);
  const focusedLineRef = useRef<{ clientKey: string; field: LineField } | null>(null);
  // U8: validation readiness uses the same rules as the payload builder
  // (no new requiredness): named per-field blockers with focus targets.
  // Defined early so the shell gate and blocked activation can name every
  // validation blocker alongside every session blocker.
  const validationBlockers = useMemo(
    () => listValidationBlockers(state, activeCurrency),
    [state, activeCurrency]
  );
  // N4/U2: receipt provenance for the review. An authoritative persisted
  // session (stored ID before the pass, or a first save of already-recorded
  // items) makes the review a read-only receipt of the session-recorded
  // choices and acknowledgements, with needs-review rows and Change routes
  // for anything unrecorded. Identity-only local and no-session reviews keep
  // the legacy choice workbench.
  const [receiptResolvedKeys, setReceiptResolvedKeys] = useState<string[]>([]);
  const [receiptAckedMissing, setReceiptAckedMissing] = useState<string[]>([]);
  const [receiptAckedExtra, setReceiptAckedExtra] = useState<number[]>([]);
  const [receiptHasSession, setReceiptHasSession] = useState(false);
  const loadedSourceForRef = useRef<number | null>(null);

  const reconciliation = useReconciliationSession({
    transport: reconciliationTransport ?? wailsReconciliationTransport,
    orderId: state.orderId,
    sourceState: state.sourceState,
    sourceQuotationId: order.QuotationID,
    sourceRevision: order.SourceRevision,
    poVersion: state.version,
    readOnly: correcting,
    source: compareSource,
    sourceLines: compareLines,
    lines: state.lines.map((line, index) => ({
      clientKey: line.clientKey,
      persistedId: line.persistedId,
      dtoAlias: line.persistedClientKey,
      displayNumber: index + 1,
      quotationItemNo: line.quotationItemNo,
      quotationSubNo: line.quotationSubNo,
      values: {
        description: line.description,
        uom: line.uom,
        quantity: line.quantity,
        unitPrice: line.unitPrice,
        discountEnabled: line.discountEnabled,
        discountPercent: line.discountPercent,
      },
    })),
    currency: activeCurrency,
    actor: { id: actorId, name: actorName },
    getFieldValue: (field) =>
      field === "header:paymentTerms"
        ? editorStateRef.current.header.paymentTerms
        : field === "header:incoterms"
          ? editorStateRef.current.header.incoterms
          : editorStateRef.current.header.discountPercent,
    applyFieldValue: (field, value) => {
      if (field === "header:paymentTerms") {
        dispatch({ type: "headerChanged", patch: { paymentTerms: value } });
      } else if (field === "header:incoterms") {
        dispatch({ type: "headerChanged", patch: { incoterms: String(value) } });
      } else {
        dispatch({ type: "headerChanged", patch: { discountPercent: String(value) } });
      }
    },
    applyLineFieldValue: (lineKey, field, value) => {
      const target = editorStateRef.current.lines.find((line) => line.clientKey === lineKey);
      if (target === undefined) return;
      if (field === "discount" && typeof value === "object") {
        dispatch({
          type: "lineChanged",
          key: target.key,
          patch: { discountEnabled: value.enabled, discountPercent: value.percent },
        });
        return;
      }
      if (typeof value === "string") {
        if (field === "description") dispatch({ type: "lineChanged", key: target.key, patch: { description: value } });
        else if (field === "uom") dispatch({ type: "lineChanged", key: target.key, patch: { uom: value } });
        else if (field === "quantity") dispatch({ type: "lineChanged", key: target.key, patch: { quantity: value } });
        else if (field === "unitPrice") dispatch({ type: "lineChanged", key: target.key, patch: { unitPrice: value } });
      }
    },
  });

  // Loads the exact source once per persisted live record: the commit handler
  // compares against it, never against typed input. Transient drafts have no
  // server row to review, so nothing loads until the first persist.
  useEffect(() => {
    if (state.orderId < 1 || state.sourceState !== "live") {
      loadedSourceForRef.current = null;
      return;
    }
    if (loadedSourceForRef.current === state.orderId) return;
    loadedSourceForRef.current = state.orderId;
    let cancelled = false;
    // The binding throws synchronously with no Wails runtime (plain-browser
    // dev, partial harness mocks); the async wrapper turns that into a
    // rejection the catch below absorbs.
    void (async () => {
      try {
        const loaded = await GetPurchaseOrderReview(state.orderId);
        if (cancelled) return;
        setCompareSource({
          paymentTerms: loaded.Source.PaymentTerms,
          incoterms: loaded.Source.Incoterms,
          discountPercent: loaded.Source.DiscountPercent,
        });
        setCompareLines(loaded.Source.Lines);
      } catch {
        if (cancelled) return;
        setCompareSource(null);
        setCompareLines(null);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [state.orderId, state.sourceState]);

  /**
   * Wave 2 of #473: carry the working copy into the correction flow at the
   * version the server named. Nothing is written here; the reason panel appears
   * and the next Save correction submits against `version`.
   */
  const beginSavedCorrection = (version: number) => {
    // A finalize review holds "Use Quotation" choices that exist only as
    // decisions. Apply them to the working copy before the review closes, so
    // the correction carries what the reviewer had confirmed. From the entry
    // view there is nothing to apply.
    if (view !== "entry" && review !== null) {
      const applied = evaluateReview(
        state,
        review.Source,
        activeCurrency,
        decisions,
        acknowledgedMissing,
        acknowledgedExtra
      );
      dispatch({
        type: "valuesReplaced",
        header: applied.applied.header,
        lines: applied.applied.lines,
      });
    }
    dispatch({ type: "versionAdopted", version });
    setSavedCorrection(true);
    onCorrectionStart?.();
    setConflict(null);
    setPendingWrite("save");
    setReview(null);
    setConfirmPayload(null);
    setView("entry");
  };

  /**
   * Ticket 04: a draft save persists the session with the row version the
   * draft save just produced (R-04). A session failure never fails the draft
   * save: the record is stored, the banner names the sync problem, and the
   * next save retries with the same items. A session save that bumps the row
   * version is adopted, so the next submit names the version the server now
   * holds rather than the draft-save one (round-2 N3).
   */
  const adoptSessionVersion = (
    orderId: number,
    draftVersion: number,
    sessionVersion: number | undefined
  ) => {
    if (sessionVersion === undefined || sessionVersion === draftVersion) return;
    dispatch({ type: "versionAdopted", version: sessionVersion });
    persistedRef.current = { id: orderId, version: sessionVersion };
  };

  const handleSaved = async (savedOrder: PurchaseOrderDTO, corrected?: boolean) => {
    const outcome = await reconciliation.persistAfterDraftSave(savedOrder.Version);
    if (!outcome.ok) {
      setSessionBannerError(
        "The reconciliation could not be saved. Your draft is stored; saving again retries the sync."
      );
    } else {
      setSessionBannerError(null);
      adoptSessionVersion(savedOrder.ID, savedOrder.Version, outcome.version);
    }
    onSaved(savedOrder, corrected);
  };

  /**
   * The answer to a draft write that met a saved record. The earlier warning's
   * actor and time are kept when there is one; a bare po_not_draft has only the
   * version, because the server validated it before its state rule refused.
   */
  const savedRecordAnswer = (version: number): PurchaseOrderConflictDTO => ({
    CurrentState: "saved",
    CurrentVersion: version,
    UpdatedBy: conflict?.UpdatedBy ?? 0,
    UpdatedByName: conflict?.UpdatedByName ?? "",
    UpdatedAt: conflict?.UpdatedAt ?? "",
  });

  // The dirty flag is lifted, not duplicated: the editor still owns the
  // baseline comparison, and App only consumes the answer so it can ask
  // before a tab navigation unmounts this component's reducer state.
  useEffect(() => {
    onDirtyChange?.(isPurchaseOrderDirty(state));
  }, [state, onDirtyChange]);

  // Ticket 02: the shell action-bar bridge. The report depends only on
  // primitives; the callbacks ride a ref updated every render, so a fresh
  // function identity never retriggers the report and App state never
  // ping-pongs. Entry view only: review, confirm and link views keep their
  // own buttons and report null so the shell bar stands down.
  const shellDirty = isPurchaseOrderDirty(state);
  const shellSourceLess = state.sourceState !== "live";
  const shellActionsRef = useRef<{
    save: (expectedVersion: number, overwrite: boolean) => Promise<void>;
    startReview: () => Promise<void>;
    blockedActivate: () => void;
  } | null>(null);

  // U8: a blocked Final Save never saves. It names every validation and
  // session blocker (no silent slicing after the third), announces them
  // assertively, and focuses the first blocker so its error becomes visible.
  // Validation blockers focus their field; session blockers reveal the pane
  // (switching the filter to All so a Resolved-filter view cannot hide the
  // blocker) and focus the item.
  const focusValidationBlocker = useCallback((fieldLabel: string) => {
    const target = document.querySelector<HTMLElement>(
      `[aria-label="${fieldLabel.replace(/"/g, "")}"]`
    );
    if (target !== null) {
      revealIntoView(target);
      return true;
    }
    return false;
  }, []);
  const blockedActivate = useCallback(() => {
    const validationMessages = validationBlockers.map((entry) => entry.message);
    const sessionLabels = reconciliation.blockers.map((entry) => entry.label);
    const all = [...validationMessages, ...sessionLabels];
    const count = all.length;
    reconciliation.setBlockedAnnouncement(
      count === 0
        ? "Final Save is ready."
        : `Final Save is blocked. ${count} ${count === 1 ? "blocker" : "blockers"}: ${all.join("; ")}.`
    );
    // Focus the first blocker: a validation failure focuses its field,
    // otherwise the first unresolved pane item is revealed and focused.
    if (validationBlockers.length > 0) {
      if (focusValidationBlocker(validationBlockers[0]?.fieldLabel ?? "")) return;
    }
    if (reconciliation.blockingCount > 0) {
      reconciliation.setFilter("all");
      const smallViewport =
        typeof window !== "undefined" &&
        typeof window.matchMedia === "function" &&
        window.matchMedia("(max-width: 1023px)").matches;
      if (smallViewport || sheetOpen) {
        sheetFocusRef.current = { kind: "first" };
        setSheetOpen(true);
        return;
      }
      paneRailRef.current?.focusFirstUnresolved();
    }
  }, [
    validationBlockers,
    reconciliation.blockingCount,
    reconciliation.blockers,
    reconciliation.setBlockedAnnouncement,
    reconciliation.setFilter,
    focusValidationBlocker,
    sheetOpen,
  ]);

  // Ticket 04: focuses a pane item from the inline anchor under a field.
  const showPaneItem = useCallback(
    (key: string) => {
      const smallViewport =
        typeof window !== "undefined" &&
        typeof window.matchMedia === "function" &&
        window.matchMedia("(max-width: 1023px)").matches;
      if (smallViewport || sheetOpen) {
        sheetFocusRef.current = { kind: "item", key };
        setSheetOpen(true);
        return;
      }
      paneRailRef.current?.focusItem(key);
    },
    [sheetOpen]
  );

  const closeSheet = useCallback(() => {
    sheetFocusRef.current = null;
    setSheetOpen(false);
    sheetTriggerRef.current?.focus();
  }, []);

  // Ticket 04: sheet focus contract. A manual open lands on the sheet
  // heading; a blocked save or Show action lands on the requested item.
  useEffect(() => {
    if (!sheetOpen) return;
    const target = sheetFocusRef.current;
    sheetFocusRef.current = null;
    if (target !== null && target.kind !== "heading") {
      const timer = window.setTimeout(() => {
        if (target.kind === "first") paneSheetRef.current?.focusFirstUnresolved();
        else paneSheetRef.current?.focusItem(target.key);
      }, 0);
      return () => window.clearTimeout(timer);
    }
    document.getElementById("sheet-pane-heading")?.focus();
  }, [sheetOpen]);
  useEffect(() => {
    if (!actionsInShell) {
      onShellActions?.(null);
      return;
    }
    if (view !== "entry" || linkReview !== null) {
      onShellActions?.(null);
      return;
    }
    const callbacks = shellActionsRef.current;
    if (callbacks === null) return;
    const version = state.version;
    // U8/F11: the shell names every validation and session blocker, never a
    // silent first-three slice. The count covers both so an invalid field
    // blocks exactly like an unresolved pane item.
    const allBlockerLabels = [
      ...validationBlockers.map((entry) => entry.label),
      ...reconciliation.blockers.map((entry) => entry.label),
    ];
    const unresolved = allBlockerLabels.length;
    onShellActions?.({
      mode: correcting ? "correction" : "draft",
      dirty: shellDirty,
      saving: state.saving,
      reviewLoading,
      sourceLess: shellSourceLess,
      unresolvedCount: unresolved,
      blockedMessage:
        unresolved > 0
          ? `Resolve ${unresolved} ${unresolved === 1 ? "blocker" : "blockers"} to enable Final Save: ${allBlockerLabels.join("; ")}`
          : null,
      onSave: () => void callbacks.save(version, false),
      onFinalSave: () => void callbacks.startReview(),
      onBlockedActivate: () => callbacks.blockedActivate(),
    });
  }, [
    actionsInShell,
    view,
    linkReview,
    correcting,
    shellDirty,
    state.saving,
    state.version,
    reviewLoading,
    shellSourceLess,
    validationBlockers,
    reconciliation.blockers,
    blockedActivate,
    onShellActions,
  ]);
  useEffect(() => {
    if (!actionsInShell) return;
    return () => {
      onShellActions?.(null);
    };
  }, [actionsInShell, onShellActions]);

  /**
   * Persists the transient row the first time an action needs an id (Save
   * draft or PDF upload). The created id is adopted immediately, so
   * a failed attach or update retries against the same row instead of creating
   * a second one.
   */
  type PersistedDraft = { id: number; version: number };
  const persisting = useRef<Promise<PersistedDraft> | null>(null);
  // Retained beyond the in-flight promise so a trigger that arrives after the
  // create resolves but before React renders the `created` action still sees
  // the one row.
  const persistedRef = useRef<PersistedDraft | null>(null);
  const ensurePersisted = async (): Promise<PersistedDraft> => {
    if (persistedRef.current !== null) return persistedRef.current;
    const current = editorStateRef.current;
    if (current.orderId > 0) {
      const existing = { id: current.orderId, version: current.version };
      persistedRef.current = existing;
      return existing;
    }
    if (persisting.current !== null) return persisting.current;
    const promise = (async () => {
      const created = await CreatePurchaseOrderBlankDraft(
        new backend.PurchaseOrderBlankCreateInput({
          ReceivedDate: current.header.receivedDate,
          TaxZone: current.taxZone,
          TaxType: current.taxType,
          ServiceCategory: current.serviceCategory,
        })
      );
      dispatch({ type: "created", orderId: created.ID, version: created.Version });
      const result = { id: created.ID, version: created.Version };
      persistedRef.current = result;
      return result;
    })();
    persisting.current = promise;
    try {
      return await promise;
    } finally {
      persisting.current = null;
    }
  };

  /**
   * #493: one persistence of the working copy, shared by Save, the
   * upload's id resolver and the link review. It creates the row once and then
   * writes the entered values with a single Update, so a concurrent Save and
   * upload produce one create and one successful update -- never a second row
   * or a version conflict. The retained version advances after the update, so a
   * failed attach retries at the latest version. The notification is delivered
   * once even when two callers await the same save: a Save and an upload never
   * notify twice. The upload itself passes `false`, so its save cannot remount
   * the editor and discard a failed attach's error.
   */
  type WorkingCopyOutcome =
    | { kind: "saved"; order: PurchaseOrderDTO }
    | { kind: "conflict"; conflict: PurchaseOrderConflictDTO }
    | { kind: "savedRecord" }
    | { kind: "error"; message: string };
  const workingCopyPersisting = useRef<{
    promise: Promise<WorkingCopyOutcome>;
    notified: boolean;
  } | null>(null);

  const persistWorkingCopy = async (notifySaved: boolean): Promise<WorkingCopyOutcome> => {
    // Validate the working copy before creating anything. An incomplete
    // transient draft must not leave a blank stored row behind just because an
    // upload or Link was clicked; the same rule Save follows.
    const preflight = buildPurchaseOrderUpdate(editorStateRef.current, activeCurrency);
    if (preflight.input === null) {
      return { kind: "error", message: preflight.error ?? "The draft is incomplete." };
    }
    let shared = workingCopyPersisting.current;
    if (shared === null) {
      const promise = (async (): Promise<WorkingCopyOutcome> => {
        const persisted = await ensurePersisted();
        const working = {
          ...editorStateRef.current,
          orderId: persisted.id,
          version: persisted.version,
        };
        const result = buildPurchaseOrderUpdate(working, activeCurrency);
        if (result.input === null) {
          return { kind: "error", message: result.error ?? "The draft is incomplete." };
        }
        const answer = await UpdatePurchaseOrderDraft(
          new backend.PurchaseOrderUpdateInput({
            ...result.input,
            ID: persisted.id,
            ExpectedVersion: persisted.version,
            Overwrite: false,
            Items: result.input.Items.map((item) => new backend.PurchaseOrderItemInput(item)),
          })
        );
        const outcome = writeOutcome(answer);
        if (outcome.kind === "saved") {
          // Advance the retained version: an attach retry after a failed
          // upload must write against the row as it now stands.
          persistedRef.current = { id: outcome.order.ID, version: outcome.order.Version };
          dispatch({ type: "saveSucceeded", order: outcome.order, currency: activeCurrency });
          return { kind: "saved", order: outcome.order };
        }
        if (outcome.kind === "conflict") {
          return { kind: "conflict", conflict: outcome.conflict };
        }
        if (outcome.kind === "savedRecord") {
          return { kind: "savedRecord" };
        }
        return { kind: "error", message: "The server answered without a saved record. Try again." };
      })();
      const record = { promise, notified: false };
      workingCopyPersisting.current = record;
      const clear = () => {
        if (workingCopyPersisting.current === record) workingCopyPersisting.current = null;
      };
      void promise.then(clear, clear);
      shared = record;
    }
    const outcome = await shared.promise;
    if (outcome.kind === "saved" && notifySaved && !shared.notified) {
      shared.notified = true;
      void handleSaved(outcome.order, correcting);
    }
    return outcome;
  };

  /** Loads the link review for the selector's confirmed exact pair. */
  async function startDraftLinkReview(target: {
    quotationId: number;
    revisionNumber: number;
  }) {
    setLinkLoading(true);
    setLinkError(null);
    try {
      // The review and the link compare the stored values, so an unsaved edit
      // must be saved first or the review would describe values the link does
      // not keep. This path does not add a second "draft saved" toast.
      const persistence = await persistWorkingCopy(false);
      if (persistence.kind === "conflict") {
        throw new Error(draftLinkConflictWarning(persistence.conflict));
      }
      if (persistence.kind === "savedRecord") {
        throw new Error("This purchase order was saved by someone else; reopen it before linking.");
      }
      if (persistence.kind === "error") {
        throw new Error(persistence.message);
      }
      const loaded = await PreviewPurchaseOrderDraftLink(
        persistence.order.ID,
        target.quotationId,
        target.revisionNumber
      );
      setLinkReview(loaded);
      setDecisions({});
      setAcknowledgedMissing([]);
      setAcknowledgedExtra([]);
      setLinkTaxChoice(loaded.TaxChoiceRequired ? "" : "keep");
    } catch (error) {
      setLinkError(errorMessage(error, "The purchase order could not be saved. Try again."));
    } finally {
      setLinkLoading(false);
    }
  }

  // App confirms the shared selector's exact pair; the editor owns persistence
  // and the review because it owns the working copy.
  const handledLinkTarget = useRef<object | null>(null);
  useEffect(() => {
    if (pendingLinkTarget === null) return;
    if (handledLinkTarget.current === pendingLinkTarget) return;
    handledLinkTarget.current = pendingLinkTarget;
    onLinkTargetHandled?.();
    void startDraftLinkReview(pendingLinkTarget);
    // The target object is the trigger; the callbacks read current values.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingLinkTarget]);

  /**
   * #497: commits the link. The applied review is saved as an ordinary draft
   * first -- so every "Use Quotation" choice and corrected line match is what
   * the server compares -- and then the link request commits identity and the
   * chosen tax basis against that saved version.
   */
  const commitDraftLink = async () => {
    if (linkReview === null || linking) return;
    const plan = planDraftLink(
      state,
      linkReview,
      activeCurrency,
      decisions,
      acknowledgedMissing,
      acknowledgedExtra,
      linkTaxChoice
    );
    if (plan.input === null || plan.update === null) {
      setLinkError(plan.error ?? "Resolve the review before linking.");
      return;
    }
    setLinking(true);
    setLinkError(null);
    try {
      const answer = await UpdatePurchaseOrderDraft(
        new backend.PurchaseOrderUpdateInput({
          ...plan.update,
          ID: state.orderId,
          ExpectedVersion: state.version,
          Overwrite: false,
          Items: plan.update.Items.map((item) => new backend.PurchaseOrderItemInput(item)),
        })
      );
      const outcome = writeOutcome(answer);
      if (outcome.kind === "conflict") {
        setLinkError(draftLinkConflictWarning(outcome.conflict));
        return;
      }
      if (outcome.kind === "savedRecord") {
        setLinkError("This purchase order was saved by someone else; reopen it before linking.");
        return;
      }
      if (outcome.kind !== "saved") {
        setLinkError("The reviewed values could not be saved before linking.");
        return;
      }
      dispatch({ type: "saveSucceeded", order: outcome.order, currency: activeCurrency });

      const linkedAnswer = await LinkPurchaseOrderDraftSource(
        new backend.PurchaseOrderDraftLinkInput({
          ID: outcome.order.ID,
          ExpectedVersion: outcome.order.Version,
          TargetQuotationID: plan.input.TargetQuotationID,
          TargetRevisionNumber: plan.input.TargetRevisionNumber,
          Decisions: plan.input.Decisions,
          AcknowledgedMissing: plan.input.AcknowledgedMissing,
          AcknowledgedExtra: plan.input.AcknowledgedExtra,
          TaxChoice: plan.input.TaxChoice,
        })
      );
      const linked = writeOutcome(linkedAnswer);
      if (linked.kind === "conflict") {
        setLinkError(draftLinkConflictWarning(linked.conflict));
        return;
      }
      if (linked.kind === "savedRecord") {
        setLinkError("This purchase order was saved by someone else; reopen it before linking.");
        return;
      }
      if (linked.kind !== "saved") {
        setLinkError("The server answered without a linked purchase order.");
        return;
      }
      dispatch({ type: "saveSucceeded", order: linked.order, currency: activeCurrency });
      setLinkReview(null);
      setDecisions({});
      setAcknowledgedMissing([]);
      setAcknowledgedExtra([]);
      setLinkTaxChoice("");
      onLinked?.(linked.order);
    } catch (error) {
      setLinkError(errorMessage(error, "The purchase order could not be saved. Try again."));
    } finally {
      setLinking(false);
    }
  };

  const save = async (expectedVersion: number, overwrite: boolean) => {
    if (correcting) {
      const reasonProblem = correctionReasonError(reason);
      if (reasonProblem !== null) {
        dispatch({ type: "saveFailed", error: reasonProblem });
        return;
      }
    }
    // Validate before creating anything: an invalid form must not leave an
    // empty row behind.
    const preview = buildPurchaseOrderUpdate(state, activeCurrency);
    if (preview.input === null) {
      dispatch({ type: "saveFailed", error: preview.error ?? "The draft is incomplete." });
      return;
    }
    dispatch({ type: "saveStarted" });
    // The first Save of a transient draft goes through the same shared
    // persistence an upload uses. A concurrent Save + upload therefore makes
    // one create and one successful update, rather than two writes racing on
    // the same version or a second row.
    if (!correcting && !overwrite && expectedVersion < 1) {
      try {
        const outcome = await persistWorkingCopy(true);
        if (outcome.kind === "saved") {
          setConflict(null);
          return;
        }
        if (outcome.kind === "conflict") {
          dispatch({ type: "saveConflict" });
          setPendingWrite("save");
          setConflict(outcome.conflict);
          return;
        }
        if (outcome.kind === "savedRecord") {
          dispatch({ type: "saveConflict" });
          setPendingWrite("save");
          setConflict(savedRecordAnswer(expectedVersion));
          return;
        }
        dispatch({ type: "saveFailed", error: outcome.message });
      } catch (error) {
        dispatch({ type: "saveFailed", error: errorMessage(error, "The purchase order could not be saved. Try again.") });
      }
      return;
    }
    try {
      // Save and upload share one in-flight create. A Save whose
      // create is still running and a file choice made at the same time must
      // await the same row, never create a second one.
      let working = editorStateRef.current;
      if (!correcting) {
        const persisted = await ensurePersisted();
        working = { ...working, orderId: persisted.id, version: persisted.version };
      }
      const result = buildPurchaseOrderUpdate(working, activeCurrency);
      if (result.input === null) {
        dispatch({ type: "saveFailed", error: result.error ?? "The draft is incomplete." });
        return;
      }
      const payload = {
        ...result.input,
        ID: working.orderId,
        ExpectedVersion: expectedVersion < 1 ? working.version : expectedVersion,
        Overwrite: overwrite,
      };
      const items = payload.Items.map(
        (item) => new backend.PurchaseOrderItemInput(item)
      );
      const answer = correcting
        ? await CorrectPurchaseOrder(
            new backend.PurchaseOrderCorrectionInput({
              ...withoutSourceBasis(payload),
              Reason: trimCorrectionReason(reason),
              Items: items,
            })
          )
        : await UpdatePurchaseOrderDraft(
            new backend.PurchaseOrderUpdateInput({ ...payload, Items: items })
          );
      const outcome = writeOutcome(answer);
      if (outcome.kind === "saved") {
        dispatch({ type: "saveSucceeded", order: outcome.order, currency: activeCurrency });
        setConflict(null);
        void handleSaved(outcome.order, correcting);
        return;
      }
      if (outcome.kind === "conflict") {
        dispatch({ type: "saveConflict" });
        setPendingWrite("save");
        setConflict(outcome.conflict);
        return;
      }
      if (outcome.kind === "savedRecord") {
        // The record is saved: a draft write cannot replace it. Keep the work
        // and offer the correction transition rather than a dead end.
        dispatch({ type: "saveConflict" });
        setPendingWrite("save");
        setConflict(savedRecordAnswer(expectedVersion));
        return;
      }
      dispatch({
        type: "saveFailed",
        error: "The server answered without a saved record. Try again.",
      });
    } catch (error) {
      dispatch({ type: "saveFailed", error: errorMessage(error, "The purchase order could not be saved. Try again.") });
    }
  };

  const startReview = async () => {
    // Contract section 7: commit the focused input first, before readiness.
    // The commit result is the readiness truth: a focused field that still
    // fails validation blocks with its message and focus.
    let focusedCommitFailed = false;
    if (focusedLineRef.current !== null) {
      const target = focusedLineRef.current;
      focusedLineRef.current = null;
      const result = reconciliation.commitLine(
        target.clientKey,
        target.field
      );
      if (result !== null && result !== undefined) {
        const failed =
          (result as { validationError?: string | null }).validationError != null ||
          ((result as { item?: unknown }).item !== null &&
            (result as { itemChanged?: boolean }).itemChanged === true &&
            (result as unknown as { item: { Status: string } }).item.Status === "unresolved");
        if (failed) focusedCommitFailed = true;
      }
    } else if (focusedFieldRef.current !== null) {
      const field = focusedFieldRef.current;
      focusedFieldRef.current = null;
      const result = reconciliation.commitField(field);
      if (result !== null && result !== undefined) {
        const failed =
          (result as { validationError?: string | null }).validationError != null ||
          ((result as { item?: unknown }).item !== null &&
            (result as { itemChanged?: boolean }).itemChanged === true &&
            (result as unknown as { item: { Status: string } }).item.Status === "unresolved");
        if (failed) focusedCommitFailed = true;
      }
    }
    // U8: named readiness before any save. Validation blockers use the same
    // rules as the payload builder (no new requiredness); session blockers
    // come from the live line set. The first blocker is focused so its error
    // becomes visible; the review never opens while blocked.
    const liveValidation = listValidationBlockers(state, activeCurrency);
    const liveSessionCount = reconciliation.blockingCount;
    if (liveValidation.length > 0 || liveSessionCount > 0 || focusedCommitFailed) {
      const firstValidation = liveValidation[0];
      if (firstValidation !== undefined) {
        dispatch({ type: "saveFailed", error: firstValidation.message });
        reconciliation.setBlockedAnnouncement(
          `Final Save is blocked. ${liveValidation.length + liveSessionCount} ${liveValidation.length + liveSessionCount === 1 ? "blocker" : "blockers"}: ${[...liveValidation.map((entry) => entry.message), ...reconciliation.blockers.map((entry) => entry.label)].join("; ")}.`
        );
        const target = document.querySelector<HTMLElement>(
          `[aria-label="${firstValidation.fieldLabel.replace(/"/g, "")}"]`
        );
        if (target !== null) {
          target.scrollIntoView({ block: "nearest" });
          target.focus();
        }
        return;
      }
      blockedActivate();
      return;
    }
    const result = buildPurchaseOrderUpdate(state, activeCurrency);
    if (result.input === null) {
      dispatch({ type: "saveFailed", error: result.error ?? "The draft is incomplete." });
      return;
    }
    setReviewLoading(true);
    try {
      // Round-2 N1: a Final Save without a manual Save Draft must still
      // persist the working copy and the session BEFORE the review loads.
      // The backend gate reads the persisted session, so opening the review
      // on unsaved decisions would finalize against stale items. In
      // correction mode the session is read-only history and the working
      // copy is written by Save correction, so the legacy path is kept.
      let persisted = { id: state.orderId, version: state.version };
      // N4/U2: receipt-provenance inputs. The pre-save session is captured
      // before the persist; the post-save truth is read from the persist
      // answer, never from the stale render closure, so the first save can
      // flip provenance in this same pass.
      const preSaveSession = reconciliation.session ?? null;
      let persistedSession: ReconciliationSession | null = preSaveSession;
      const preSaveId = preSaveSession?.ID ?? 0;
      const preSaveItems = preSaveSession?.Items.length ?? 0;
      if (!correcting) {
        const saved = await persistWorkingCopy(false);
        if (saved.kind === 'conflict') {
          dispatch({ type: 'saveConflict' });
          setPendingWrite('save');
          setConflict(saved.conflict);
          return;
        }
        if (saved.kind === 'savedRecord') {
          dispatch({ type: 'saveConflict' });
          setPendingWrite('save');
          setConflict(savedRecordAnswer(state.version));
          return;
        }
        if (saved.kind === 'error') {
          dispatch({ type: 'saveFailed', error: saved.message });
          return;
        }
        const synced = await reconciliation.persistAfterDraftSave(saved.order.Version);
        if (!synced.ok) {
          setSessionBannerError(
            'The reconciliation could not be saved. Your draft is stored; saving again retries the sync.'
          );
          dispatch({ type: 'saveFailed', error: synced.message ?? 'The reconciliation decisions could not be saved yet.' });
          return;
        }
        setSessionBannerError(null);
        const finalVersion = synced.version ?? saved.order.Version;
        adoptSessionVersion(saved.order.ID, saved.order.Version, synced.version);
        persisted = { id: saved.order.ID, version: finalVersion };
        persistedSession = synced.session ?? persistedSession;
      } else {
        persisted = await ensurePersisted();
      }
      const loaded = await GetPurchaseOrderReview(persisted.id);
      setCompareSource({
        paymentTerms: loaded.Source.PaymentTerms,
        incoterms: loaded.Source.Incoterms,
        discountPercent: loaded.Source.DiscountPercent,
      });
      setCompareLines(loaded.Source.Lines);
      const initial = initialFinalizeReviewState();
      setReview(loaded);
      // U2: the finalize review is a receipt of the recorded session, not a
      // second workbench. Kept/used choices pre-fill from unchanged session
      // resolutions; missing/extra acknowledgements pre-fill from the
      // session's recorded acks. Everything else stays undecided and blocks.
      const prefilled = prefillDecisionsFromSession(
        reconciliation.items,
        {
          paymentTerms: state.header.paymentTerms,
          incoterms: state.header.incoterms,
          discountPercent: state.header.discountPercent,
        },
        {
          PaymentTerms: loaded.Source.PaymentTerms,
          Incoterms: loaded.Source.Incoterms,
          DiscountPercent: loaded.Source.DiscountPercent,
        }
      );
      const prefilledLines = reconciliation.prefillLines();
      const mergedDecisions = { ...initial.decisions, ...prefilled, ...prefilledLines };
      setDecisions(mergedDecisions);
      // Session-recorded acks survive into the review; without a session the
      // legacy empty initial state is preserved (no invented history).
      const sessionMissing =
        reconciliation.hasSession === true
          ? reconciliation.prefillMissing(loaded.Source.Lines)
          : [];
      const sessionExtra =
        reconciliation.hasSession === true ? reconciliation.prefillExtra() : [];
      const mergedMissing =
        reconciliation.hasSession === true
          ? Array.from(new Set([...initial.acknowledgedMissing, ...sessionMissing]))
          : initial.acknowledgedMissing;
      const mergedExtra =
        reconciliation.hasSession === true
          ? Array.from(new Set([...initial.acknowledgedExtra, ...sessionExtra]))
          : initial.acknowledgedExtra;
      setAcknowledgedMissing(mergedMissing);
      setAcknowledgedExtra(mergedExtra);
      // N4/U2: receipt provenance is the authoritative persisted session,
      // not decision counts. An already-stored session (positive ID before
      // this pass, even empty/tax-only/all-unresolved) is recorded
      // provenance by definition. A first save in this same pass counts only
      // when the pre-save session already recorded something (items), so a
      // genuinely local identity-only shell or a no-session draft keeps the
      // legacy choice workbench instead of a dead-end receipt.
      const authoritativeSession =
        preSaveId > 0 || ((persistedSession?.ID ?? 0) > 0 && preSaveItems > 0);
      setReceiptResolvedKeys(Object.keys(mergedDecisions));
      setReceiptAckedMissing(sessionMissing);
      setReceiptAckedExtra(sessionExtra);
      setReceiptHasSession(!correcting && authoritativeSession);
      // §5.5 entry readiness (authoritative sessions only): the whole
      // current form is compared with the actual loaded source using the
      // same pure comparison as the review. A zero item count never implies
      // ready: an unreviewed difference keeps the editor in entry with the
      // first blocker named and focused, instead of opening the review.
      if (!correcting && authoritativeSession) {
        const gate = evaluateReview(
          state,
          loaded.Source,
          activeCurrency,
          mergedDecisions,
          mergedMissing,
          mergedExtra
        );
        if (!gate.canContinue) {
          reconciliation.setBlockedAnnouncement(
            `Final Save is blocked. ${gate.blockers.length} ${gate.blockers.length === 1 ? "blocker" : "blockers"}: ${gate.blockers.join("; ")}.`
          );
          // Focus the first unreviewed difference's live field so it can be
          // committed and resolved in the pane; anything without a live
          // field (missing/extra/duplicate/unknown) reveals the pane.
          // Semantic comparison keys choose the control, not display-label
          // coincidence: the row-discount label ("Line N discount") names
          // neither the percent input nor the enabled checkbox.
          const firstOpen = gate.comparison.differences.find(
            (entry) => mergedDecisions[entry.key] === undefined
          );
          const candidates =
            firstOpen === undefined
              ? []
              : gateFocusCandidates(firstOpen.key, firstOpen.label, state.lines);
          let focused = false;
          for (const candidate of candidates) {
            const target = document.querySelector<HTMLElement>(
              `[aria-label="${candidate.replace(/"/g, "")}"]`
            );
            if (target !== null && !(target as HTMLInputElement).disabled) {
              setPendingFocus(candidate);
              focused = true;
              break;
            }
          }
          if (focused) return;
          // No live field (missing/extra/duplicate/unknown): reveal the
          // pane. Focus the first unresolved action when one exists;
          // otherwise focus the pane heading so the block never strands
          // focus on BODY with no target.
          reconciliation.setFilter("all");
          const hasUnresolvedAction = (persistedSession?.Items ?? []).some(
            (entry) => entry.Status === "unresolved"
          );
          const smallViewport =
            typeof window !== "undefined" &&
            typeof window.matchMedia === "function" &&
            window.matchMedia("(max-width: 1023px)").matches;
          if (smallViewport || sheetOpen) {
            sheetFocusRef.current = hasUnresolvedAction ? { kind: "first" } : { kind: "heading" };
            setSheetOpen(true);
            return;
          }
          if (
            !hasUnresolvedAction ||
            !paneRailRef.current?.focusFirstUnresolved()
          ) {
            document.getElementById("rail-pane-heading")?.focus();
          }
          return;
        }
      }
      setDocumentConfirmed(initial.documentConfirmed);
      setAssignConfirmation(initial.assignConfirmation);
      setView("review");
    } catch (error) {
      dispatch({ type: "saveFailed", error: errorMessage(error, "The purchase order could not be saved. Try again.") });
    } finally {
      setReviewLoading(false);
    }
  };

  // Ticket 02: keeps the shell callbacks fresh without retriggering the
  // report effect above. Assigned every render; the effect reads it.
  shellActionsRef.current = { save, startReview, blockedActivate };

  const finalSave = async (expectedVersion: number, overwrite: boolean) => {
    if (review === null) return;
    const result = buildFinalizeInput(
      state,
      review.Source,
      activeCurrency,
      review.DocumentState,
      decisions,
      acknowledgedMissing,
      acknowledgedExtra,
      documentConfirmed,
      assignConfirmation
    );
    if (result.input === null) {
      dispatch({ type: "saveFailed", error: result.error ?? "The purchase order cannot be saved yet." });
      setView("confirm");
      return;
    }
    const progressStarted = Date.now();
    const waitForProgress = async () => {
      const remaining = 300 - (Date.now() - progressStarted);
      if (remaining > 0) {
        await new Promise((resolve) => setTimeout(resolve, remaining));
      }
    };
    setFinalSaving(true);
    try {
      const saved = await FinalizePurchaseOrder(
        new backend.PurchaseOrderFinalizeInput({
          ...result.input,
          ExpectedVersion: expectedVersion,
          Overwrite: overwrite,
        })
      );
      const outcome = writeOutcome(saved);
      if (outcome.kind === "saved") {
        await waitForProgress();
        dispatch({ type: "saveSucceeded", order: outcome.order, currency: activeCurrency });
        setConflict(null);
        // Round-2 N2: the saved record (and its session) is read-only. Never
        // attempt a session write after success: the server refuses it and
        // the UI would show a false sync banner. Just clear any earlier sync
        // error and report the saved order.
        setSessionBannerError(null);
        onSaved(outcome.order, correcting);
        setReview(null);
        setView("entry");
        return;
      }
      if (outcome.kind === "conflict") {
        setPendingWrite("finalize");
        setConflict(outcome.conflict);
        return;
      }
      if (outcome.kind === "savedRecord") {
        // Someone finalized the record while the review was open. The final
        // save cannot become an overwrite; keep the reviewed working copy and
        // continue as a correction against the version just submitted, because
        // the server's version check passed before its state rule refused.
        dispatch({ type: "saveConflict" });
        setPendingWrite("save");
        setConflict(savedRecordAnswer(expectedVersion));
        return;
      }
      dispatch({
        type: "saveFailed",
        error: "The server answered without a saved record. Try again.",
      });
    } catch (error) {
      dispatch({ type: "saveFailed", error: errorMessage(error, "The purchase order could not be saved. Try again.") });
    } finally {
      setFinalSaving(false);
    }
  };

  const conflictPrompt =
    conflict === null ? null : (
      <PurchaseOrderConflictPrompt
        conflict={conflict}
        overwriting={state.saving || finalSaving}
        savedTransition={!correcting && conflictNeedsCorrection(conflict)}
        onCancel={() => setConflict(null)}
        onCorrectSaved={() => beginSavedCorrection(conflict.CurrentVersion)}
        onOverwrite={() => {
          if (pendingWrite === "finalize") {
            void finalSave(conflict.CurrentVersion, true);
          } else {
            void save(conflict.CurrentVersion, true);
          }
        }}
      />
    );

  const evaluation =
    review === null
      ? null
      : evaluateReview(
          state,
          review.Source,
          currency,
          decisions,
          acknowledgedMissing,
          acknowledgedExtra
        );

  // #497: the link review replaces the editor view while it is open, exactly as
  // the final-save review does. The working copy is already saved, so Back
  // returns to the same values without an unsaved-change prompt.
  // U2: receipt Change links leave the review for the entry form and focus
  // the relevant control after render (fields by accessible name, line
  // matches by opening the row's match select).
  const editReviewDifference = (key: string, label: string) => {
    setView("entry");
    setPendingFocus(label);
  };
  const editReviewLine = (lineNumber: number) => {
    const line = state.lines[lineNumber - 1];
    setView("entry");
    if (line !== undefined) {
      setOpenMatchLineKey(line.clientKey);
      setPendingFocus(`Line ${lineNumber} quotation match`);
    } else {
      setPendingFocus(`Line ${lineNumber} description`);
    }
  };
  if (linkReview !== null) {
    return (
      <PurchaseOrderDraftLinkReviewPanel
        state={state}
        review={linkReview}
        currency={activeCurrency}
        decisions={decisions}
        acknowledgedMissing={acknowledgedMissing}
        acknowledgedExtra={acknowledgedExtra}
        taxChoice={linkTaxChoice}
        linking={linking}
        error={linkError ?? state.error}
        onDecision={(key: string, choice: POReviewChoice) =>
          setDecisions((current) => ({ ...current, [key]: choice }))
        }
        onToggleMissing={(label) =>
          setAcknowledgedMissing((current) =>
            current.includes(label)
              ? current.filter((value) => value !== label)
              : [...current, label]
          )
        }
        onToggleExtra={(lineNumber) =>
          setAcknowledgedExtra((current) =>
            current.includes(lineNumber)
              ? current.filter((value) => value !== lineNumber)
              : [...current, lineNumber]
          )
        }
        onMatchChange={(lineKey, source: PurchaseOrderSourceLineDTO | null) =>
          dispatch({
            type: "lineChanged",
            key: lineKey,
            patch: {
              quotationItemNo: source === null ? null : source.QuotationItemNo,
              quotationSubNo: source === null ? null : (source.QuotationSubNo ?? null),
            },
          })
        }
        onTaxChoice={setLinkTaxChoice}
        onBack={() => {
          setLinkReview(null);
          setLinkError(null);
        }}
        onLink={() => void commitDraftLink()}
      />
    );
  }

  // Cancel hands the whole decision to App's guard: there is no separate
  // editor confirmation, so Cancel followed by a tab click replaces one
  // pending action instead of opening a second dialog. Leaving the review
  // with Back to entry keeps the entry values, including any Use Quotation
  // choice already applied.
  if (view === "review" && review !== null) {
    return (
      <>
      <PurchaseOrderDifferenceReviewView
        state={state}
        source={review.Source}
        currency={activeCurrency}
        quoteNumber={order.QuoteNumber}
        documentState={review.DocumentState === "attached" ? "attached" : "none"}
        decisions={decisions}
        acknowledgedMissing={acknowledgedMissing}
        acknowledgedExtra={acknowledgedExtra}
        receiptResolvedKeys={receiptResolvedKeys}
        receiptAckedMissing={receiptAckedMissing}
        receiptAckedExtra={receiptAckedExtra}
        hasSession={receiptHasSession}
        onEditDifference={editReviewDifference}
        onEditLine={editReviewLine}
        onDecision={(key: string, choice: POReviewChoice) =>
          setDecisions((current) => ({ ...current, [key]: choice }))
        }
        onToggleMissing={(label) =>
          setAcknowledgedMissing((current) =>
            current.includes(label)
              ? current.filter((value) => value !== label)
              : [...current, label]
          )
        }
        onToggleExtra={(lineNumber) =>
          setAcknowledgedExtra((current) =>
            current.includes(lineNumber)
              ? current.filter((value) => value !== lineNumber)
              : [...current, lineNumber]
          )
        }
        onMatchChange={(lineKey, source: PurchaseOrderSourceLineDTO | null) =>
          dispatch({
            type: "lineChanged",
            key: lineKey,
            patch: {
              quotationItemNo: source === null ? null : source.QuotationItemNo,
              quotationSubNo:
                source === null ? null : (source.QuotationSubNo ?? null),
            },
          })
        }
        onBack={() => setView("entry")}
        onContinue={() => {
          if (evaluation === null) return;
          const applied = buildPurchaseOrderUpdate(evaluation.applied, currency);
          if (applied.input === null) {
            dispatch({
              type: "saveFailed",
              error: applied.error ?? "The purchase order is incomplete.",
            });
            return;
          }
          setConfirmPayload(applied.input);
          setView("confirm");
        }}
      />
      {conflictPrompt}
      </>
    );
  }

  if (
    view === "confirm" &&
    review !== null &&
    evaluation !== null &&
    confirmPayload !== null
  ) {
    return (
      <>
      <PurchaseOrderFinalConfirmationView
        review={review}
        evaluation={evaluation}
        payload={confirmPayload}
        currency={activeCurrency}
        documentConfirmed={documentConfirmed}
        onToggleDocumentConfirmed={() => setDocumentConfirmed((current) => !current)}
        assignConfirmation={assignConfirmation}
        onToggleAssignConfirmation={() =>
          setAssignConfirmation((current) => !current)
        }
        onBack={() => setView("review")}
        onFinalSave={() => void finalSave(state.version, false)}
        saving={finalSaving}
        error={state.error}
      />
      {conflictPrompt}
      </>
    );
  }

  // Ticket 04: pane view-model for the entry layout. Notices render under
  // their fields; the inline anchor appears only for an unresolved item.
  const reconciliationNotices = reconciliation.notices;
  const unresolvedHeaderItems: Record<HeaderFieldKey, ReconciliationItem | null> = {
    "header:paymentTerms": null,
    "header:incoterms": null,
    "header:discountPercent": null,
  };
  for (const item of reconciliation.items) {
    if (
      item.Status === "unresolved" &&
      (item.Field === "header:paymentTerms" ||
        item.Field === "header:incoterms" ||
        item.Field === "header:discountPercent")
    ) {
      unresolvedHeaderItems[item.Field as HeaderFieldKey] = item;
    }
  }
  // Ticket 05: line feedback keys are `clientKey:field`, from the stable
  // identity — never the displayed line number.
  const unresolvedByLineField: Record<string, ReconciliationItem | null> = {};
  for (const item of reconciliation.items) {
    if (item.Status !== "unresolved" || typeof item.LineKey !== "string") continue;
    if (
      item.Field === "line:description" ||
      item.Field === "line:uom" ||
      item.Field === "line:quantity" ||
      item.Field === "line:unitPrice" ||
      item.Field === "line:discount"
    ) {
      const suffix = item.Field.slice("line:".length);
      unresolvedByLineField[`${item.LineKey}:${suffix}`] = item;
    }
  }
  // U1: focusing a line's match select from a pane card. The pane hands
  // the stable session key; the editor works in aliases. Opening the select
  // is part of focusing it; the pending-focus effect focuses after render.
  const focusLineMatch = (sessionKey: string) => {
    const clientKey = reconciliation.clientKeyForSessionKey(sessionKey);
    if (clientKey === null) return;
    const at = state.lines.findIndex((line) => line.clientKey === clientKey);
    if (at === -1) return;
    setOpenMatchLineKey(clientKey);
    setPendingFocus(`Line ${at + 1} quotation match`);
  };
  // U1: immediate removal keeps review history server-side; focus moves to
  // the next line's description (or Add line) and never falls to body.
  const removeLine = (clientKey: string) => {
    const at = state.lines.findIndex((line) => line.clientKey === clientKey);
    if (at === -1) return;
    const target = editorStateRef.current.lines[at + 1];
    dispatch({ type: "lineRemoved", key: state.lines[at]?.key ?? -1 });
    setOpenMatchLineKey(null);
    if (target !== undefined) {
      const nextAt = state.lines.findIndex((line) => line.clientKey === target.clientKey);
      setPendingFocus(`Line ${nextAt === -1 ? at + 1 : nextAt + 1} description`);
    } else {
      setPendingFocus("Add line");
    }
    reconciliation.announce(`Line ${at + 1} removed. Its review items are kept as history.`);
  };
  const addLineFromSource = (reference: string) => {
    const [head, tail] = reference.split(".");
    const itemNo = Number(head);
    if (!Number.isInteger(itemNo) || itemNo < 1) return;
    // R2-U3: like addLine, the new row takes focus and announces once, so a
    // pointer or keyboard user lands on the new description instead of the
    // body. The match reference, count and history are untouched.
    const at = state.lines.length + 1;
    dispatch({
      type: "lineAddedFromSource",
      uom: uomOptions[0]?.value ?? "",
      quotationItemNo: itemNo,
      quotationSubNo: tail === undefined ? null : Number(tail),
    });
    setOpenMatchLineKey(null);
    setPendingFocus(`Line ${at} description`);
    reconciliation.announce(`Line ${at} added.`);
  };
  // U6/R-09: the reason follows the visible comparable-input order. The
  // form renders the Lines section before Discounts and adjustment, so
  // matched lines come first in display order (quantity, unit price, row
  // discount per line) and the whole-PO discount last. No duplicate discount
  // input, no pricing change; the helper itself is order-agnostic and the
  // R-09 unit test covers first-differs-wins.
  const totalsStripReason: string | null =
    compareSource === null || compareLines === null
      ? null
      : explainTotalsDifference([
          ...lineMoneyDifferences(
            state.lines.map((line, index) => ({
              displayNumber: index + 1,
              values: {
                description: line.description,
                uom: line.uom,
                quantity: line.quantity,
                unitPrice: line.unitPrice,
                discountEnabled: line.discountEnabled,
                discountPercent: line.discountPercent,
              },
              source:
                compareLines.find(
                  (entry) =>
                    sourceLineRef(entry.QuotationItemNo, entry.QuotationSubNo) ===
                    (line.quotationItemNo === null
                      ? ""
                      : sourceLineRef(line.quotationItemNo, line.quotationSubNo))
                ) ?? null,
            })),
            activeCurrency
          ),
          headerDiscountDifference(state.header.discountPercent, compareSource.discountPercent),
        ]);
  const calculatedTotals = calculatePurchaseOrder(state, activeCurrency).totals;
  const totalsStrip =
    state.sourceState !== "live" || !state.printedTotalsKnown ? null : (
      <ReconciliationTotalsStrip
        calculated={
          calculatedTotals === null || calculatedTotals === undefined
            ? null
            : {
                subtotalMinor: calculatedTotals.subtotalMinor,
                discountMinor: calculatedTotals.discountMinor,
                ppnMinor: calculatedTotals.ppnMinor,
                adjustmentMinor: calculatedTotals.adjustmentMinor,
                grandTotalMinor: calculatedTotals.grandTotalMinor,
              }
        }
        printed={state.printedTotals}
        reason={totalsStripReason}
        currency={activeCurrency}
      />
    );
  const matchOptions: { value: string; label: string }[] | undefined =
    compareLines === null
      ? undefined
      : [
          { value: "", label: "Extra line (no quotation match)" },
          ...compareLines.map((entry) => ({
            value: sourceLineRef(entry.QuotationItemNo, entry.QuotationSubNo),
            label: `Quotation item ${sourceLineRef(entry.QuotationItemNo, entry.QuotationSubNo)} · ${entry.Description}`,
          })),
          ...state.lines
            .filter(
              (line) =>
                line.quotationItemNo !== null &&
                !compareLines.some(
                  (entry) =>
                    sourceLineRef(entry.QuotationItemNo, entry.QuotationSubNo) ===
                    sourceLineRef(line.quotationItemNo ?? 0, line.quotationSubNo)
                )
            )
            .map((line) => ({
              value: sourceLineRef(line.quotationItemNo ?? 0, line.quotationSubNo),
              label: `Quotation item ${sourceLineRef(line.quotationItemNo ?? 0, line.quotationSubNo)} (not in source)`,
            })),
        ];
  const paneStatus =
    state.sourceState === "live"
      ? reconciliation.status === "idle"
        ? "loading"
        : reconciliation.status
      : "ready";
  // F1: display context for line-set cards, keyed by stable session key.
  // Wire values stay canonical; human descriptions travel here.
  const lineSummaries: Record<string, { displayNumber: number; description: string }> =
    Object.fromEntries(
      state.lines.map((line, index) => [
        reconciliation.sessionKeyFor(line.clientKey),
        { displayNumber: index + 1, description: line.description },
      ])
    );
  const sourceDescriptions: Record<string, string> = Object.fromEntries(
    (compareLines ?? []).map((entry) => [
      sourceLineRef(entry.QuotationItemNo, entry.QuotationSubNo),
      entry.Description,
    ])
  );
  const paneSourceLabel =
    state.sourceState === "never"
      ? null
      : `QT-${order.QuoteNumber} · revision ${order.SourceRevision}`;
  const paneRailListTop = () => {
    document.getElementById("rail-pane-items")?.scrollTo({ top: 0 });
  };
  const paneSheetListTop = () => {
    document.getElementById("sheet-pane-items")?.scrollTo({ top: 0 });
  };

  return (
    <div className="flex flex-col gap-6">
      {linkLoading ? (
        <StatusBanner tone="info">
          <span role="status">Saving the draft and loading the source review…</span>
        </StatusBanner>
      ) : null}
      {linkError !== null && linkReview === null ? (
        <StatusBanner tone="danger">{linkError}</StatusBanner>
      ) : null}
      {sessionBannerError !== null ? (
        <StatusBanner tone="danger">{sessionBannerError}</StatusBanner>
      ) : null}
      <div className="hidden lg:block">
        <a
          href="#rail-pane-heading"
          className="sr-only focus-visible:not-sr-only focus-visible:inline-flex focus-visible:rounded-lg focus-visible:bg-ink-900 focus-visible:px-4 focus-visible:py-2.5 focus-visible:text-small focus-visible:font-medium focus-visible:text-white"
        >
          Skip to reconciliation
        </a>
      </div>
      <ReconciliationSheetTrigger
        ref={sheetTriggerRef}
        unresolvedCount={reconciliation.unresolvedCount}
        status={paneStatus}
        noSource={state.sourceState === "never"}
        readOnly={correcting}
        resolvedCount={reconciliation.counts.resolved}
        sourceLabel={
          state.sourceState === "never"
            ? null
            : `QT-${order.QuoteNumber} · revision ${order.SourceRevision}`
        }
        expanded={sheetOpen}
        onOpen={() => {
          sheetFocusRef.current = { kind: "heading" };
          setSheetOpen(true);
        }}
      />
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="@container min-w-0" inert={sheetOpen ? true : undefined}>
          <div className="flex flex-col gap-6">
      <PurchaseOrderEditorView
        state={state}
        dispatch={dispatch}
        currency={activeCurrency}
        currencyOptions={currencyOptions}
        uomOptions={uomOptions}
        incotermOptions={incotermOptions}
        onLinkQuotation={onLinkQuotation}
        onSave={() => void save(state.version, false)}
        onCancel={onCancel}
        onFinalSave={() => void startReview()}
        finalSaving={reviewLoading}
        mode={correcting ? "correction" : "draft"}
        correctionReason={reason}
        onCorrectionReasonChange={setReason}
        confirmationNumber={order.ConfirmationNumber ?? ""}
        actionsInShell={actionsInShell}
        notices={reconciliationNotices}
        validationErrors={reconciliation.validationErrors}
        unresolvedByField={unresolvedHeaderItems}
        onCommitField={(field, explicitValue) => {
          focusedFieldRef.current = field;
          reconciliation.commitField(field, explicitValue);
        }}
        sessionKeyFor={reconciliation.sessionKeyFor}
        onFocusField={(field) => {
          focusedFieldRef.current = field;
          focusedLineRef.current = null;
        }}
        onFocusLine={(lineKey, field) => {
          focusedFieldRef.current = null;
          focusedLineRef.current = { clientKey: lineKey, field };
        }}
        onShowItem={(key) => showPaneItem(key)}
        openMatchLineKey={openMatchLineKey}
        onToggleMatch={(lineKey) =>
          setOpenMatchLineKey((current) => (current === lineKey ? null : lineKey))
        }
        onRemoveLine={removeLine}
        onAddLine={addLine}
        matchOptions={matchOptions}
        onMatchChange={(lineKey, quotationItemNo, quotationSubNo) => {
          const target = state.lines.find((line) => line.clientKey === lineKey);
          if (target === undefined) return;
          dispatch({
            type: "lineChanged",
            key: target.key,
            patch: { quotationItemNo, quotationSubNo },
          });
        }}
        onCommitLine={(lineKey, field, explicitRaw) =>
          reconciliation.commitLine(lineKey, field, explicitRaw)
        }
        lineNotices={reconciliation.lineNotices}
        lineErrors={reconciliation.lineValidationErrors}
        unresolvedByLineField={unresolvedByLineField}
        gateUnresolvedCount={
          validationBlockers.length + reconciliation.blockingCount
        }
        gateBlockedMessage={
          validationBlockers.length + reconciliation.blockingCount > 0
            ? `Resolve ${validationBlockers.length + reconciliation.blockingCount} ${validationBlockers.length + reconciliation.blockingCount === 1 ? "blocker" : "blockers"} to enable Final Save: ${[...validationBlockers.map((entry) => entry.label), ...reconciliation.blockers.map((entry) => entry.label)].join("; ")}`
            : null
        }
        onBlockedActivate={blockedActivate}
      />
      {conflictPrompt}
          </div>
        </div>
        <aside
          aria-label="Reconciliation"
          className="animate-rail-in hidden min-h-0 lg:sticky lg:top-[88px] lg:flex lg:max-h-[calc(100dvh-176px)] lg:flex-col lg:gap-3 [@media(max-height:719px)]:lg:max-h-none"
        >
          <div className="min-h-0 flex-1 overflow-hidden [@media(max-height:719px)]:overflow-visible">
          <ReconciliationPaneContent
            idPrefix="rail"
            ref={paneRailRef}
            session={reconciliation.session}
            status={paneStatus}
            error={reconciliation.error}
            sourceLabel={paneSourceLabel}
            sourceRevision={order.SourceRevision}
            noSource={state.sourceState === "never"}
            formerSource={state.sourceState === "former"}
            readOnly={correcting}
            readOnlyCaption={
              correcting
                ? "The saved reconciliation is history. This correction does not change it."
                : undefined
            }
            filter={reconciliation.filter}
            onFilterChange={(next) => {
              reconciliation.setFilter(next);
              paneRailListTop();
            }}
            onShowAll={reconciliation.showAll}
            latestNotice={reconciliation.latestNotice}
            keepConfirmKey={reconciliation.keepConfirmKey}
            onRequestKeep={reconciliation.requestKeep}
            onCancelKeep={reconciliation.cancelKeep}
            onConfirmKeep={reconciliation.confirmKeep}
            onUse={reconciliation.useQuotation}
            onLinkQuotation={onLinkQuotation}
            onRetry={reconciliation.reload}
            onAddLineFromSource={addLineFromSource}
            onAcknowledgeMissing={reconciliation.acknowledgeMissing}
            onAcknowledgeExtra={reconciliation.acknowledgeExtra}
            onFixMatch={focusLineMatch}
            lineSummaries={lineSummaries}
            sourceDescriptions={sourceDescriptions}
            currency={activeCurrency}
            actorNameFor={(id) => (id === actorId ? actorName : null)}
            fillHeight
          />
          </div>
          <div className="shrink-0">
            {/* U4: the rail owns the customer PO document; the form must not
                repeat it. An upload persists the working copy first so a
                reload cannot lose the values staff saw when they chose the
                file (#493). */}
            <PurchaseOrderDocumentPanel
              purchaseOrderId={state.orderId}
              disabled={state.saving}
              resolvePurchaseOrderId={async () => {
                const outcome = await persistWorkingCopy(false);
                if (outcome.kind === "saved") return outcome.order.ID;
                if (outcome.kind === "conflict") {
                  throw new Error(draftLinkConflictWarning(outcome.conflict));
                }
                if (outcome.kind === "savedRecord") {
                  throw new Error(
                    "This purchase order was saved by someone else; reopen it before attaching the customer PO PDF."
                  );
                }
                throw new Error(outcome.message);
              }}
            />
          </div>
          {totalsStrip === null ? null : (
            <div className="shrink-0 rounded-lg border border-rule bg-white p-3">{totalsStrip}</div>
          )}
        </aside>
      </div>
      {sheetOpen ? (
        <div
          role="region"
          aria-label="Reconciliation"
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              closeSheet();
            }
          }}
          className="animate-sheet-in fixed inset-x-0 bottom-0 top-[var(--shell-header-h,64px)] z-20 flex flex-col gap-3 overflow-y-auto border-t border-rule bg-paper px-5 pb-24 pt-4 lg:hidden"
        >
          <ReconciliationPaneContent
            idPrefix="sheet"
            ref={paneSheetRef}
            session={reconciliation.session}
            status={paneStatus}
            error={reconciliation.error}
            sourceLabel={paneSourceLabel}
            sourceRevision={order.SourceRevision}
            noSource={state.sourceState === "never"}
            formerSource={state.sourceState === "former"}
            readOnly={correcting}
            readOnlyCaption={
              correcting
                ? "The saved reconciliation is history. This correction does not change it."
                : undefined
            }
            filter={reconciliation.filter}
            onFilterChange={(next) => {
              reconciliation.setFilter(next);
              paneSheetListTop();
            }}
            onShowAll={reconciliation.showAll}
            latestNotice={reconciliation.latestNotice}
            keepConfirmKey={reconciliation.keepConfirmKey}
            onRequestKeep={reconciliation.requestKeep}
            onCancelKeep={reconciliation.cancelKeep}
            onConfirmKeep={reconciliation.confirmKeep}
            onUse={reconciliation.useQuotation}
            onLinkQuotation={onLinkQuotation}
            onRetry={reconciliation.reload}
            onAddLineFromSource={addLineFromSource}
            onAcknowledgeMissing={reconciliation.acknowledgeMissing}
            onAcknowledgeExtra={reconciliation.acknowledgeExtra}
            onFixMatch={focusLineMatch}
            lineSummaries={lineSummaries}
            sourceDescriptions={sourceDescriptions}
            currency={activeCurrency}
            actorNameFor={(id) => (id === actorId ? actorName : null)}
            headerActions={
              <button
                type="button"
                aria-label="Close reconciliation"
                onClick={closeSheet}
                className="inline-flex min-h-11 shrink-0 cursor-pointer items-center justify-center gap-2 rounded-lg border border-rule-strong bg-white px-3.5 text-small font-medium text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
              >
                Close
              </button>
            }
          />
          <PurchaseOrderDocumentPanel
            purchaseOrderId={state.orderId}
            disabled={state.saving}
            resolvePurchaseOrderId={async () => {
              const outcome = await persistWorkingCopy(false);
              if (outcome.kind === "saved") return outcome.order.ID;
              if (outcome.kind === "conflict") {
                throw new Error(draftLinkConflictWarning(outcome.conflict));
              }
              if (outcome.kind === "savedRecord") {
                throw new Error(
                  "This purchase order was saved by someone else; reopen it before attaching the customer PO PDF."
                );
              }
              throw new Error(outcome.message);
            }}
          />
          {totalsStrip === null ? null : (
            <div className="rounded-lg border border-rule bg-white p-3">{totalsStrip}</div>
          )}
        </div>
      ) : null}
      <div role="status" aria-live="polite" className="sr-only">
        {reconciliation.announcement}
      </div>
      {reconciliation.blockedAnnouncement !== "" ? (
        <div role="alert" className="sr-only">
          {reconciliation.blockedAnnouncement}
        </div>
      ) : null}
    </div>
  );
}
