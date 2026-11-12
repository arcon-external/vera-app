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
import type { CurrencyInfo } from "../format";
import {
  conflictNeedsCorrection,
  correctionReasonError,
  trimCorrectionReason,
  writeOutcome,
  type PurchaseOrderConflictDTO,
} from "../poCorrection";
import {
  buildFinalizeInput,
  comparePurchaseOrder,
  decisionsForCurrentValues,
  evaluateReview,
  type PurchaseOrderReviewDTO,
  type PurchaseOrderSourceLineDTO,
} from "../poFinalize";
import {
  buildPurchaseOrderUpdate,
  calculatePurchaseOrder,
  listValidationBlockers,
  initialPurchaseOrderEditorState,
  isPurchaseOrderDirty,
  latestPersistedVersion,
  newPurchaseOrderLineKey,
  purchaseOrderEditorReducer,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorAction,
  type PurchaseOrderEditorState,
  type PurchaseOrderLine,
  type PurchaseOrderUpdateInput,
} from "../purchaseOrder";
import {
  draftLinkConflictWarning,
  draftLinkCurrencyRemedy,
  planDraftLink,
} from "../poDraftLink";
import {
  InlineQuoteField,
  InlineQuoteSelect,
  type InlineQuoteMode,
  type InlineQuoteValue,
} from "./QuoteInlineField";
import { QuotationGhost } from "./QuotationGhost";
import {
  groupSourceItems,
  linesFromSourceItem,
  sourceItemApplied,
  type SourceItem,
} from "../poSourceItems";
import { PurchaseOrderDocumentPanel } from "./PurchaseOrderDocumentPanel";
import { PurchaseOrderConflictPrompt } from "./PurchaseOrderConflictPrompt";
import { PurchaseOrderFinalConfirmationView } from "./PurchaseOrderFinalize";
import { PurchaseOrderSourceLinkPanel } from "./PurchaseOrderSourceLinkPanel";
import { PurchaseOrderTotals } from "./PurchaseOrderTotals";
import {
  ItemEditor,
  type EditorItem,
  type EditorLineField,
  type EditorSubItem,
} from "./ItemEditor";
import {
  addSubLine,
  editorItemsFromGroups,
  groupPurchaseOrderLines,
  moveSubLine,
  patchGroupParentDescription,
  patchLine,
  patchSubLine,
  removeGroup,
  splitPlainLine,
} from "../purchaseOrderLines";
import { Select } from "./Select";
import {
  SERVICE_CATEGORY_OPTIONS,
  TAX_TYPE_OPTIONS,
  TAX_ZONE_OPTIONS,
} from "../tax";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";
import type {
  HeaderFieldKey,
  ReconciliationSourceValues,
} from "../poReconciliation";
import {
  hasAckRecording,
  sourceLineRef,
  type LineField,
} from "../poReconciliation.lines";
import {
  wailsReconciliationTransport,
  type ReconciliationTransport,
} from "../poReconciliationAdapter";
import { useReconciliationSession } from "../useReconciliationSession";

// Issue #468: the PO draft workspace. The header keeps the customer snapshot
// read-only (a directory edit must not rewrite this order) while the agreed
// commercial details stay editable; the lines are the customer's order, with
// quantity as a 4-decimal string and money in the PO's own currency.
//
// The calculation (#470) is shown beside the entry: row discounts,
// customer-facing PPN, and the grand total. The source
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
  /** Exact quotation values for the inline field bars and ghost items. */
  quoteSource?: ReconciliationSourceValues | null;
  quoteLines?: PurchaseOrderSourceLineDTO[] | null;
  /** Safe deferred linking for #497; it never replaces or creates a record. */
  onLinkQuotation?: () => void;
  onSave: () => void;
  onCancel: () => void;
  /** Opens the final-save confirmation. */
  onFinalSave: () => void;
  /** True while the confirmation's exact source is being loaded. */
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
  onCommitField?: (field: HeaderFieldKey, explicitValue?: string) => void;
  onFocusField?: (field: HeaderFieldKey) => void;
  onFocusLine?: (lineKey: string, field: LineField) => void;
  /** U1: which line's match select is expanded (only one at a time). */
  openMatchLineKey?: string | null;
  onToggleMatch?: (lineKey: string) => void;
  /** U1: removal that moves focus and announces; falls back to plain dispatch. */
  onRemoveLine?: (lineKey: string) => void;
  /** U1: adding a line focuses the new description and announces it. */
  onAddLine?: () => void;
  /** Records an ignored quotation item as an explicit omission. */
  onIgnoreQuotationItem?: (item: SourceItem) => void;
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
   * The editor persists its working copy, links immediately and clears the
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
   * that stays reachable while the form scrolls. The confirmation view keeps
   * its own buttons and reports null.
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
  onCancel: () => void;
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

/** A YYYY-MM-DD field reads as "10 Nov 2026" in the hero; the form keeps the
 *  ISO value because that is what the date input and the payload use. */
function displayPurchaseOrderDate(raw: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw.trim());
  if (match === null) return raw.trim() === "" ? "—" : raw;
  const date = new Date(
    Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]))
  );
  return date.toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  });
}

/** One empty editor line, with the stable alias every write submits. */
function newEditorLine(key: number, uom: string): PurchaseOrderLine {
  return {
    key,
    clientKey: newPurchaseOrderLineKey(),
    persistedId: undefined,
    persistedClientKey: null,
    description: "",
    quantity: "1",
    uom,
    unitPrice: "0",
    discountEnabled: false,
    discountPercent: "0",
    quotationItemNo: null,
    quotationSubNo: null,
    groupKey: "",
    parentDescription: "",
    descriptionLines: [],
  };
}

/** A split-line identity. Source groups use `q:<itemNo>`; an extra group
 *  needs a value no other group can claim, so it gets a UUID. */
function newGroupKey(): string {
  const cryptoObject = globalThis.crypto;
  const uuid =
    cryptoObject !== undefined && typeof cryptoObject.randomUUID === "function"
      ? cryptoObject.randomUUID()
      : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  return `g:${uuid}`;
}

/**
 * One line's entry controls: the customer's description, quantity, UoM and
 * unit price, then the row's own discount and the exact line total it
 * produces. Exported so the snapshot harness can show this row at 360/1280 --
 * the full editor page pushes it below the fold.
 */
export function PurchaseOrderEditorView({
  state,
  dispatch,
  currency,
  currencyOptions = [],
  uomOptions,
  incotermOptions,
  quoteSource = null,
  quoteLines = null,
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
  onCommitField,
  onFocusField,
  onFocusLine,
  openMatchLineKey,
  onToggleMatch,
  onRemoveLine,
  onAddLine,
  onIgnoreQuotationItem,
  matchOptions,
  onMatchChange,
  onCommitLine,
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

  // #573: the flat rows are grouped for the quotation line-item component.
  // Every edit maps back onto the rows that own the value; the parent row of a
  // split line is a presentation, never a row of its own.
  const groups = useMemo(() => groupPurchaseOrderLines(state.lines), [state.lines]);
  const plainGroupCount = groups.filter((group) => group.groupKey === "").length;
  const subItemCount = state.lines.length - plainGroupCount;
  const editorItems = useMemo(
    () => editorItemsFromGroups(groups, activeCurrency),
    [groups, activeCurrency]
  );
  const [inlineUsedFields, setInlineUsedFields] = useState<Set<string>>(new Set());
  const [inlineOriginalFields, setInlineOriginalFields] = useState<Set<string>>(new Set());
  const inlineOriginalValues = useRef<Record<string, string>>({});
  const [ghostAccepted, setGhostAccepted] = useState<Set<number>>(new Set());
  const [ghostRejected, setGhostRejected] = useState<Set<number>>(new Set());

  const sourceItems = useMemo(
    () => groupSourceItems(quoteLines ?? []),
    [quoteLines]
  );
  const ghostAttachments = useMemo(() => {
    const attached: { item: SourceItem; groupKey: string; lineKey: number }[] = [];
    const referenced = new Set<number>();
    const ghosted = new Set<number>();
    for (const group of groups) {
      const itemNo =
        group.groupKey.startsWith("q:")
          ? Number(group.groupKey.slice(2))
          : group.lines[0]?.quotationItemNo ?? null;
      if (itemNo === null) continue;
      referenced.add(itemNo);
      if (ghosted.has(itemNo)) continue;
      const item = sourceItems.find((entry) => entry.itemNo === itemNo);
      if (item === undefined) continue;
      if (ghostAccepted.has(itemNo) || ghostRejected.has(itemNo)) continue;
      if (sourceItemApplied(item, state.lines, activeCurrency)) continue;
      ghosted.add(itemNo);
      attached.push({ item, groupKey: group.groupKey, lineKey: group.lines[0].key });
    }
    const unmatched = sourceItems.filter(
      (item) =>
        !referenced.has(item.itemNo) &&
        !ghosted.has(item.itemNo) &&
        !ghostAccepted.has(item.itemNo) &&
        !ghostRejected.has(item.itemNo)
    );
    return { attached, unmatched };
  }, [groups, sourceItems, state.lines, activeCurrency, ghostAccepted, ghostRejected]);
  const ghostCount = ghostAttachments.attached.length + ghostAttachments.unmatched.length;

  const applyGhost = (
    item: SourceItem,
    target?: { groupKey?: string; lineKey?: number }
  ) => {
    const startKey = Math.max(
      state.nextKey,
      ...state.lines.map((line) => line.key + 1),
      1
    );
    const fresh = linesFromSourceItem(item, activeCurrency, startKey);
    let next: PurchaseOrderLine[] = [];
    let replaced = false;
    for (const line of state.lines) {
      if (target?.groupKey !== undefined && target.groupKey !== "" && line.groupKey === target.groupKey) {
        if (!replaced) {
          next.push(...fresh);
          replaced = true;
        }
        continue;
      }
      if (target?.lineKey !== undefined && line.key === target.lineKey) {
        next.push(...fresh);
        replaced = true;
        continue;
      }
      next.push(line);
    }
    if (!replaced) next = [...state.lines, ...fresh];
    dispatch({ type: "linesReplaced", lines: next });
    setGhostAccepted((current) => new Set(current).add(item.itemNo));
  };

  const rejectGhost = (item: SourceItem) => {
    setGhostRejected((current) => new Set(current).add(item.itemNo));
    onIgnoreQuotationItem?.(item);
  };

  const acceptAllGhosts = () => {
    let next = [...state.lines];
    let startKey = Math.max(state.nextKey, ...state.lines.map((line) => line.key + 1), 1);
    const accepted = new Set(ghostAccepted);
    const apply = (item: SourceItem, target?: { groupKey?: string; lineKey?: number }) => {
      const fresh = linesFromSourceItem(item, activeCurrency, startKey);
      startKey += fresh.length;
      let replaced = false;
      const built: PurchaseOrderLine[] = [];
      for (const line of next) {
        if (target?.groupKey !== undefined && target.groupKey !== "" && line.groupKey === target.groupKey) {
          if (!replaced) {
            built.push(...fresh);
            replaced = true;
          }
          continue;
        }
        if (target?.lineKey !== undefined && line.key === target.lineKey) {
          built.push(...fresh);
          replaced = true;
          continue;
        }
        built.push(line);
      }
      next = replaced ? built : [...next, ...fresh];
      accepted.add(item.itemNo);
    };
    for (const attachment of ghostAttachments.attached) {
      apply(attachment.item, { groupKey: attachment.groupKey, lineKey: attachment.lineKey });
    }
    for (const item of ghostAttachments.unmatched) apply(item);
    dispatch({ type: "linesReplaced", lines: next });
    setGhostAccepted(accepted);
  };

  const rejectAllGhosts = () => {
    setGhostRejected((current) => {
      const next = new Set(current);
      for (const attachment of ghostAttachments.attached) next.add(attachment.item.itemNo);
      for (const item of ghostAttachments.unmatched) next.add(item.itemNo);
      return next;
    });
    for (const attachment of ghostAttachments.attached) {
      onIgnoreQuotationItem?.(attachment.item);
    }
    for (const item of ghostAttachments.unmatched) {
      onIgnoreQuotationItem?.(item);
    }
  };

  const inlineMode = (
    key: string,
    value: string,
    sourceValue: string | undefined
  ): InlineQuoteMode | undefined => {
    if (sourceValue === undefined || sourceValue.trim() === "") return undefined;
    if (inlineUsedFields.has(key)) return "used";
    if (inlineOriginalFields.has(key)) return "original";
    return value.trim() === sourceValue.trim() ? "used" : "available";
  };

  const inlineQuote = (
    key: string,
    value: string,
    sourceValue: string | undefined,
    apply: (next: string) => void
  ): InlineQuoteValue | undefined => {
    const mode = inlineMode(key, value, sourceValue);
    if (mode === undefined || sourceValue === undefined) return undefined;
    return {
      value: sourceValue,
      mode,
      onUse: () => {
        inlineOriginalValues.current[key] = value;
        setInlineUsedFields((current) => new Set(current).add(key));
        setInlineOriginalFields((current) => {
          const next = new Set(current);
          next.delete(key);
          return next;
        });
        apply(sourceValue);
        onCommitField?.(key as HeaderFieldKey, sourceValue);
      },
      onUseMine: () => {
        const original = inlineOriginalValues.current[key] ?? value;
        setInlineUsedFields((current) => {
          const next = new Set(current);
          next.delete(key);
          return next;
        });
        setInlineOriginalFields((current) => new Set(current).add(key));
        apply(original);
        onCommitField?.(key as HeaderFieldKey, original);
      },
    };
  };

  const changeInlineField = (key: string, value: string) => {
    setInlineUsedFields((current) => {
      const next = new Set(current);
      next.delete(key);
      return next;
    });
    setInlineOriginalFields((current) => new Set(current).add(key));
    inlineOriginalValues.current[key] = value;
  };
  const replaceLines = (lines: PurchaseOrderLine[]) =>
    dispatch({ type: "linesReplaced", lines });
  const groupForItem = (key: number) =>
    groups.find((group) => group.key === key);
  const lineForKey = (key: number) =>
    state.lines.find((line) => line.key === key);
  const flatIndexOf = (key: number) =>
    state.lines.findIndex((line) => line.key === key);

  const addLine = () => {
    if (onAddLine !== undefined) onAddLine();
    else dispatch({ type: "lineAdded", uom: uomOptions[0]?.value ?? "" });
  };

  const changeEditorItem = (
    key: number,
    field: keyof EditorItem,
    value: string | string[] | boolean
  ) => {
    const group = groupForItem(key);
    if (group === undefined) return;
    if (group.groupKey === "") {
      replaceLines(patchLine(state.lines, key, field, value, activeCurrency));
      return;
    }
    if (field === "description" && typeof value === "string") {
      replaceLines(
        patchGroupParentDescription(state.lines, group.groupKey, value)
      );
    }
  };

  const removeEditorItem = (key: number) => {
    const group = groupForItem(key);
    if (group === undefined) return;
    if (group.groupKey === "" && onRemoveLine !== undefined) {
      onRemoveLine(group.lines[0].clientKey);
      return;
    }
    replaceLines(removeGroup(state.lines, group.groupKey, key));
  };

  const addBullet = (key: number) => {
    const line = lineForKey(key);
    if (line === undefined) return;
    replaceLines(
      patchLine(
        state.lines,
        key,
        "bullets",
        [...line.descriptionLines, ""],
        activeCurrency
      )
    );
  };
  const removeBullet = (key: number, bulletIndex: number) => {
    const line = lineForKey(key);
    if (line === undefined) return;
    replaceLines(
      patchLine(
        state.lines,
        key,
        "bullets",
        line.descriptionLines.filter((_, index) => index !== bulletIndex),
        activeCurrency
      )
    );
  };
  const changeBullet = (key: number, bulletIndex: number, value: string) => {
    const line = lineForKey(key);
    if (line === undefined) return;
    replaceLines(
      patchLine(
        state.lines,
        key,
        "bullets",
        line.descriptionLines.map((lineText, index) =>
          index === bulletIndex ? value : lineText
        ),
        activeCurrency
      )
    );
  };

  const enableSubItems = (key: number) => {
    const line = lineForKey(key);
    if (line === undefined) return;
    // A line matched to a source item keeps that item's group key, so the
    // split stays tied to the quotation line it came from. A line with no
    // match (or one whose group already exists) gets its own identity.
    const sourceKey =
      line.quotationItemNo !== null ? `q:${line.quotationItemNo}` : "";
    const groupKey =
      sourceKey !== "" && !groups.some((group) => group.groupKey === sourceKey)
        ? sourceKey
        : newGroupKey();
    replaceLines(
      splitPlainLine(
        state.lines,
        key,
        groupKey,
        newEditorLine(state.nextKey, line.uom || uomOptions[0]?.value || "")
      )
    );
  };

  const focusEditorLineField = (
    itemKey: number,
    field: EditorLineField,
    subId?: number
  ) => {
    // A split line's parent title is a PO-only presentation, not a source
    // line; committing it would overwrite the first sub-item's description.
    const group = groupForItem(itemKey);
    if (subId === undefined && group?.groupKey !== "" && field === "description") {
      return;
    }
    const line = subId !== undefined ? lineForKey(subId) : lineForKey(itemKey);
    if (line !== undefined) onFocusLine?.(line.clientKey, field);
  };

  const commitEditorLineField = (
    itemKey: number,
    field: EditorLineField,
    subId?: number
  ) => {
    const group = groupForItem(itemKey);
    if (subId === undefined && group?.groupKey !== "" && field === "description") {
      return;
    }
    const line = subId !== undefined ? lineForKey(subId) : lineForKey(itemKey);
    if (line !== undefined) onCommitLine?.(line.clientKey, field);
  };

  const commitEditorSelect = (
    itemKey: number,
    field: EditorLineField,
    value: string,
    subId?: number
  ) => {
    const line = subId !== undefined ? lineForKey(subId) : lineForKey(itemKey);
    if (line !== undefined) onCommitLine?.(line.clientKey, field, value);
  };

  const commitEditorDiscount = (
    itemKey: number,
    enabled: boolean,
    percent: string,
    subId?: number
  ) => {
    const line = subId !== undefined ? lineForKey(subId) : lineForKey(itemKey);
    if (line !== undefined) {
      onCommitLine?.(line.clientKey, "discount", { enabled, percent });
    }
  };

  const addSubRow = (key: number) => {
    const group = groupForItem(key);
    if (group === undefined || group.groupKey === "") return;
    replaceLines(
      addSubLine(
        state.lines,
        group.groupKey,
        newEditorLine(state.nextKey, group.lines[0]?.uom ?? "")
      )
    );
  };

  const removeSubRow = (_key: number, subId: number) => {
    replaceLines(state.lines.filter((line) => line.key !== subId));
  };

  const patchSubRow = (
    _key: number,
    subId: number,
    patch: Partial<EditorSubItem>
  ) => {
    replaceLines(patchSubLine(state.lines, subId, patch, activeCurrency));
  };

  const moveSubRow = (key: number, subId: number, direction: -1 | 1) => {
    const group = groupForItem(key);
    if (group === undefined) return;
    replaceLines(moveSubLine(state.lines, group.groupKey, subId, direction));
  };

  const addSubBullet = (_key: number, subId: number) => {
    const line = lineForKey(subId);
    if (line === undefined) return;
    replaceLines(
      patchSubLine(
        state.lines,
        subId,
        {
          bullets: [...line.descriptionLines, ""],
        },
        activeCurrency
      )
    );
  };
  const removeSubBullet = (
    _key: number,
    subId: number,
    bulletIndex: number
  ) => {
    const line = lineForKey(subId);
    if (line === undefined) return;
    replaceLines(
      patchSubLine(
        state.lines,
        subId,
        {
          bullets: line.descriptionLines.filter((_, index) => index !== bulletIndex),
        },
        activeCurrency
      )
    );
  };
  const changeSubBullet = (
    _key: number,
    subId: number,
    bulletIndex: number,
    value: string
  ) => {
    const line = lineForKey(subId);
    if (line === undefined) return;
    replaceLines(
      patchSubLine(
        state.lines,
        subId,
        {
          bullets: line.descriptionLines.map((lineText, index) =>
            index === bulletIndex ? value : lineText
          ),
        },
        activeCurrency
      )
    );
  };

  const matchMetaFor = (line: PurchaseOrderLine, label: string) => {
    if (
      matchOptions === undefined ||
      onMatchChange === undefined ||
      onToggleMatch === undefined
    ) {
      return (
        <span className="text-caption text-ink-400">
          {matchLabel(line.quotationItemNo, line.quotationSubNo)}
        </span>
      );
    }
    const reference =
      line.quotationItemNo === null
        ? null
        : line.quotationSubNo === null
          ? String(line.quotationItemNo)
          : `${line.quotationItemNo}.${line.quotationSubNo}`;
    const open = openMatchLineKey === line.clientKey;
    return (
      <div className="flex flex-wrap items-center gap-2">
        <span className="inline-flex shrink-0 items-center whitespace-nowrap rounded-md border border-rule bg-paper-subtle px-2 py-0.5 text-micro font-medium uppercase text-ink-500">
          {reference === null ? "Extra line" : `Quotation ${reference}`}
        </span>
        <button
          type="button"
          aria-label={`Change quotation match for ${label}`}
          aria-expanded={open}
          onClick={() => onToggleMatch(line.clientKey)}
          className="cursor-pointer rounded-md px-2 py-1 text-small font-medium text-accent-700 underline-offset-2 hover:underline focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
        >
          Change
        </button>
        {open ? (
          <Select
            ariaLabel={`Line ${flatIndexOf(line.key) + 1} quotation match`}
            value={reference ?? ""}
            options={matchOptions}
            onChange={(value) => {
              const text = String(value);
              if (text === "") {
                onMatchChange(line.clientKey, null, null);
                return;
              }
              const [head, tail] = text.split(".");
              const itemNo = Number(head);
              if (!Number.isInteger(itemNo) || itemNo < 1) return;
              onMatchChange(
                line.clientKey,
                itemNo,
                tail === undefined ? null : Number(tail)
              );
            }}
          />
        ) : null}
      </div>
    );
  };

  const renderItemMeta = (item: EditorItem, index: number) => {
    const group = groupForItem(item.key);
    if (group === undefined) return null;
    if (group.groupKey === "") {
      return matchMetaFor(group.lines[0], `Line ${index + 1}`);
    }
    const first = group.lines[0];
    return (
      <span className="text-caption text-ink-500">
        {first?.quotationItemNo != null
          ? `Quotation item ${first.quotationItemNo}`
          : "Split line"}{" "}
        · {group.lines.length} sub-item{group.lines.length === 1 ? "" : "s"}
      </span>
    );
  };

  const renderSubItemMeta = (
    _item: EditorItem,
    sub: EditorSubItem,
    index: number,
    subIndex: number
  ) => {
    const line = lineForKey(sub.id);
    if (line === undefined) return null;
    return matchMetaFor(line, `Sub-item ${subIndex + 1} of line ${index + 1}`);
  };

  const neverLinked = state.sourceState === "never";
  const sourceLess = state.sourceState !== "live";
  const transient = state.orderId === 0;
  const basisLabel =
    state.taxType !== ""
      ? `${state.taxType}${state.serviceCategory !== "" ? ` / ${state.serviceCategory}` : ""}`
      : state.taxCategory;
  return (
    <div className="animate-step-in flex flex-col gap-6">
      <section className="relative overflow-hidden rounded-[14px] bg-accent-800 px-5 py-5 text-white shadow-card sm:px-6 sm:py-6">
        <div className="pointer-events-none absolute -bottom-8 right-4 select-none text-[122px] font-bold leading-none tracking-[-0.08em] text-white/5">
          PO
        </div>
        <div className="relative z-10 flex items-center justify-between gap-3">
          <span className="text-micro uppercase tracking-[0.18em] text-accent-200">
            Purchase order
          </span>
          <span className="rounded-md border border-white/25 bg-white/10 px-2 py-0.5 text-micro uppercase tracking-wide">
            {correcting ? "Correction" : "Draft"}
          </span>
        </div>
        <div className="relative z-10 mt-4 flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
          <div className="min-w-0">
            <h1 className="tnum text-[30px] font-semibold leading-none tracking-[-0.035em] sm:text-[34px]">
              {state.header.poNumber.trim() !== ""
                ? state.header.poNumber
                : "New purchase order"}
            </h1>
            <p className="mt-2 truncate text-small text-accent-100">
              {neverLinked ? "No customer yet" : state.customerName}
              {state.header.billingAddress !== "" ? ` · ${state.header.billingAddress}` : ""}
            </p>
          </div>
          <div className="shrink-0 text-micro uppercase tracking-[0.1em] text-accent-300 sm:text-right">
            <span>Source quotation</span>
            <strong className="mt-1 block text-small normal-case tracking-normal text-white">
              {neverLinked
                ? "Not linked"
                : `${state.quoteNumber} · rev ${String(state.sourceRevision).padStart(2, "0")}`}
            </strong>
          </div>
        </div>
        <div className="relative z-10 mt-5 flex flex-wrap gap-x-7 gap-y-3 border-t border-white/15 pt-3 text-small text-accent-100">
          <span>
            <span className="block text-[9px] uppercase tracking-[0.14em] text-accent-300">Issued</span>
            {displayPurchaseOrderDate(state.header.poDate)}
          </span>
          <span>
            <span className="block text-[9px] uppercase tracking-[0.14em] text-accent-300">Delivery</span>
            {displayPurchaseOrderDate(state.header.requestedDeliveryDate)}
          </span>
          <span>
            <span className="block text-[9px] uppercase tracking-[0.14em] text-accent-300">Terms</span>
            {state.header.incoterms || "—"} · {state.currency.toUpperCase()}
          </span>
          <span>
            <span className="block text-[9px] uppercase tracking-[0.14em] text-accent-300">Tax</span>
            {basisLabel || "—"}
          </span>
        </div>
        <p className="relative z-10 mt-3 text-caption text-accent-200">
          {correcting
            ? `Editing version ${state.version}. The previous values are kept in the correction history.`
            : transient
              ? "Not saved yet. Save draft stores it so you can continue later."
              : `Loaded at version ${state.version}.`}
          {sourceLess ? " · Source missing" : ""}
        </p>
      </section>

      {/* #580: the PO-owned details sit directly under the hero, which reads
          the same state and updates as they change. The customer, quotation
          identity, currency and tax basis are source snapshots, not controls
          here. */}
      <Card className="p-4">
        <SectionHeading
          title="Purchase order details"
          hint="Editable on this purchase order"
        />
        <div className="grid gap-4 sm:grid-cols-2">
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
          <InlineQuoteField
            label="Payment terms"
            value={state.header.paymentTerms}
            placeholder="e.g. 50% advance, balance 30 days after delivery"
            quote={inlineQuote(
              "header:paymentTerms",
              state.header.paymentTerms,
              quoteSource?.paymentTerms,
              (next) =>
                dispatch({ type: "headerChanged", patch: { paymentTerms: next } })
            )}
            onChange={(value) => {
              changeInlineField("header:paymentTerms", value);
              dispatch({ type: "headerChanged", patch: { paymentTerms: value } });
            }}
            onFocus={() => onFocusField?.("header:paymentTerms")}
            onBlur={() => onCommitField?.("header:paymentTerms")}
          />
          <InlineQuoteSelect
            label="Incoterms"
            value={state.header.incoterms}
            options={incotermOptions}
            quote={inlineQuote(
              "header:incoterms",
              state.header.incoterms,
              quoteSource?.incoterms,
              (next) =>
                dispatch({ type: "headerChanged", patch: { incoterms: next } })
            )}
            onChange={(value) => {
              changeInlineField("header:incoterms", value);
              dispatch({ type: "headerChanged", patch: { incoterms: value } });
            }}
            onFocus={() => onFocusField?.("header:incoterms")}
            onBlur={() => onCommitField?.("header:incoterms")}
          />
        </div>
        {correcting ? null : (
          <label className="mt-4 flex items-start gap-2 rounded-lg border border-rule bg-paper-subtle p-3 text-small text-ink-800">
            <input
              type="checkbox"
              className="mt-0.5 h-4 w-4 accent-accent-700"
              aria-label="Generate a confirmation number when this purchase order is saved"
              checked={state.header.confirmationRequested}
              onChange={(event) =>
                dispatch({
                  type: "headerChanged",
                  patch: { confirmationRequested: event.target.checked },
                })
              }
            />
            <span>
              <span className="font-medium">
                Generate a confirmation number when this purchase order is saved.
              </span>{" "}
              Optional. The number is issued at Final Save and cannot be edited
              afterwards; saving a draft consumes no number.
            </span>
          </label>
        )}
        <p className="mt-4 text-caption text-ink-500">
          {state.sourceState === "never"
            ? "Customer, quotation link and printed totals are filled when a quotation is linked. Currency and tax basis below are editable until then."
            : state.sourceState === "former"
              ? "The former quotation remains the reference for the printed totals and lines. Currency and tax basis below are editable until a new link."
              : "The customer, quotation number and revision, currency and tax basis are snapshots of the issued quotation revision and cannot be edited here."}
        </p>
      </Card>

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
            this basis; linking a quotation keeps it while the quotation values
            appear inline for comparison.
          </p>
        </Card>
      ) : null}

      <Card className="p-4">
        <SectionHeading
          title="Addresses and contacts"
          hint="Commercial contact details for this order"
        />
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
        </div>
      </Card>

      <div>
        <SectionHeading
          title="Line items"
          hint={`${groups.length} ${groups.length === 1 ? "line" : "lines"}${
            subItemCount > 0
              ? ` · ${subItemCount} sub-item${subItemCount === 1 ? "" : "s"}`
              : ""
          }${
            ghostCount > 0
              ? ` · ${ghostCount} quotation item${ghostCount === 1 ? "" : "s"} available`
              : ""
          }`}
          action={
            <Button size="sm" aria-label="Add line" onClick={addLine}>
              Add line
            </Button>
          }
        />
        {state.lines.length === 0 ? (
          <p className="rounded-lg border border-dashed border-rule px-4 py-8 text-center text-small text-ink-500">
            No lines yet. Add the customer's first line.
          </p>
        ) : (
          <ItemEditor
            items={editorItems}
            currency={activeCurrency}
            uomOptions={uomOptions}
            mode="purchase-order"
            renderAfterItem={(item) => {
              const group = groupForItem(item.key);
              if (group === undefined) return null;
              const attachment = ghostAttachments.attached.find((entry) =>
                group.groupKey !== ""
                  ? entry.groupKey === group.groupKey
                  : entry.lineKey === group.lines[0]?.key
              );
              if (attachment === undefined) return null;
              const filled = group.lines.some(
                (line) =>
                  line.description.trim() !== "" ||
                  line.quantity.trim() !== "" ||
                  line.unitPrice.trim() !== ""
              );
              return (
                <div className="px-3 pb-3 pt-1">
                  <QuotationGhost
                    item={attachment.item}
                    currency={activeCurrency}
                    existing={filled}
                    pointer
                    onAccept={() =>
                      applyGhost(attachment.item, {
                        groupKey: attachment.groupKey,
                        lineKey: attachment.lineKey,
                      })
                    }
                    onReject={() => rejectGhost(attachment.item)}
                  />
                </div>
              );
            }}
            allowMergeSubItems={false}
            allowEmpty
            renderItemMeta={renderItemMeta}
            renderSubItemMeta={renderSubItemMeta}
            onLineFieldFocus={focusEditorLineField}
            onLineFieldBlur={commitEditorLineField}
            onLineDiscountCommit={commitEditorDiscount}
            onLineSelectCommit={commitEditorSelect}
            onAddItem={addLine}
            onRemoveItem={removeEditorItem}
            onChange={changeEditorItem}
            onAddBullet={addBullet}
            onRemoveBullet={removeBullet}
            onBulletChange={changeBullet}
            onEnableSubItems={enableSubItems}
            onDisableSubItems={() => undefined}
            onAddSubRow={addSubRow}
            onRemoveSubRow={removeSubRow}
            onPatchSubRow={patchSubRow}
            onMoveSubRow={moveSubRow}
            onPasteSubRows={() => undefined}
            onAddSubBullet={addSubBullet}
            onRemoveSubBullet={removeSubBullet}
            onSubBulletChange={changeSubBullet}
            onPasteItems={() => undefined}
          />
        )}

        {ghostCount > 0 ? (
          <div className="mt-4 rounded-xl border border-accent-200 bg-accent-50/30 p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <p className="text-small font-semibold text-ink-900">
                  Quotation items · {ghostCount}
                </p>
                <p className="text-caption text-ink-600">
                  Use what the customer ordered; ignore the rest. A matching quotation item
                  appears under its line.
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Button size="sm" onClick={acceptAllGhosts}>
                  Use all
                </Button>
                <Button size="sm" variant="secondary" onClick={rejectAllGhosts}>
                  Ignore all
                </Button>
              </div>
            </div>
            {ghostAttachments.unmatched.length > 0 ? (
              <div className="mt-3 grid gap-2">
                <p className="text-[10px] font-semibold uppercase tracking-[0.12em] text-ink-500">
                  Not on this order yet
                </p>
                {ghostAttachments.unmatched.map((item) => (
                  <QuotationGhost
                    key={item.itemNo}
                    item={item}
                    currency={activeCurrency}
                    onAccept={() => applyGhost(item)}
                    onReject={() => rejectGhost(item)}
                  />
                ))}
              </div>
            ) : null}
            {ghostAttachments.attached.length > 0 ? (
              <p className="mt-3 text-[10px] text-ink-500">
                {ghostAttachments.attached.length} matching item
                {ghostAttachments.attached.length === 1 ? "" : "s"} show under the line itself.
              </p>
            ) : null}
          </div>
        ) : null}
      </div>

      {calculation.error !== null ? (
        <StatusBanner tone="danger">{calculation.error}</StatusBanner>
      ) : null}

      <PurchaseOrderTotals
        totals={calculation.totals}
        currency={activeCurrency}
        ppnBps={state.ppnBps}
        printedTotals={state.printedTotals}
        sourceRevision={state.sourceRevision}
        printedTotalsKnown={state.printedTotalsKnown}
      />

      {state.error !== null ? (
        <StatusBanner tone="danger">{state.error}</StatusBanner>
      ) : null}

      {actionsInShell ? null : (
      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
        <span className="text-caption text-ink-500">
          {dirty ? "Unsaved changes" : "All changes saved"} ·{" "}
          {correcting
            ? "Saving keeps the replaced values and reason in the correction history."
            : sourceLess
              ? "Link a Quotation before Final Save; this draft stays editable and keeps its values."
              : "Final Save confirms the values shown here; the quotation values are already inline."}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          {actionsInShell ? null : (
            <>
              <Button aria-label={correcting ? "Cancel correction" : "Cancel draft"} onClick={onCancel}>
                Cancel
              </Button>
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
                  {finalSaving ? "Preparing…" : "Final Save"}
                </Button>
              )}
            </>
          )}
        </div>
      </div>
      )}
    </div>
  );
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
  // The final save is a confirmation view of this page, not a separate route:
  // it reads the exact source from the server so the confirmation can show the
  // document branch, while the inline editor already holds every choice.
  const [view, setView] = useState<"entry" | "confirm">("entry");
  const [review, setReview] = useState<PurchaseOrderReviewDTO | null>(null);
  const [reviewLoading, setReviewLoading] = useState(false);
  const [documentConfirmed, setDocumentConfirmed] = useState(false);
  // #474: optional and unchecked until staff ask for it on the final save.
  const [assignConfirmation, setAssignConfirmation] = useState(false);
  const [finalSaving, setFinalSaving] = useState(false);
  // #473: the required reason a correction carries, and the one warning a
  // stale save gets. The working copy is never touched by either.
  const [reason, setReason] = useState("");
  const [conflict, setConflict] = useState<PurchaseOrderConflictDTO | null>(null);
  // Which write the warning belongs to: Overwrite resumes the same one.
  const [pendingWrite, setPendingWrite] = useState<"save" | "finalize">("save");

  // #497: the link runs immediately. The exact source is previewed only to
  // learn what is available; every current PO value is kept, so the editor can
  // show the quotation values inline without asking for a second decision.
  const [linkLoading, setLinkLoading] = useState(false);
  const [linkError, setLinkError] = useState<string | null>(null);

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
    setPendingFocus(`Description for line ${at}`);
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
          : "",
    applyFieldValue: (field, value) => {
      if (field === "header:paymentTerms") {
        dispatch({ type: "headerChanged", patch: { paymentTerms: value } });
      } else if (field === "header:incoterms") {
        dispatch({ type: "headerChanged", patch: { incoterms: String(value) } });
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

  // An extra purchase-order line is the user's own entry: the inline editor
  // offers no ghost for it, so keeping it is the recorded default. Unknown
  // references stay unresolved (the match control is the only fix) and keep
  // blocking Final Save; a plain extra line is acknowledged once and stays
  // acknowledged in the session.
  useEffect(() => {
    if (correcting || state.sourceState !== "live") return;
    for (const item of reconciliation.items) {
      if (
        item.Kind === "extra_line" &&
        item.Status === "unresolved" &&
        typeof item.LineKey === "string" &&
        !hasAckRecording(item, "acknowledged_extra")
      ) {
        reconciliation.acknowledgeExtra(item.Key, { silent: true });
      }
    }
  }, [correcting, state.sourceState, reconciliation.items, reconciliation.acknowledgeExtra]);

  // Loads the exact source once per persisted live record: the commit handler
  // compares against it, never against typed input. Transient drafts have no
  // server row to review, so nothing loads until the first persist.
  useEffect(() => {
    if (state.orderId < 1 || state.sourceState !== "live") {
      loadedSourceForRef.current = null;
      return;
    }
    if (loadedSourceForRef.current === state.orderId) return;
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
        });
        setCompareLines(loaded.Source.Lines);
        loadedSourceForRef.current = state.orderId;
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
    dispatch({ type: "versionAdopted", version });
    setSavedCorrection(true);
    onCorrectionStart?.();
    setConflict(null);
    setPendingWrite("save");
    setReview(null);
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
    persistedRef.current = {
      id: orderId,
      version: latestPersistedVersion(orderId, sessionVersion, persistedRef.current),
    };
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
  // ping-pongs. Entry view only: the confirmation view keeps its own buttons
  // and reports null so the shell bar stands down.
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
  // Validation blockers focus their field; a remaining quotation decision
  // focuses the first ghost (or the line-match control for an unknown
  // reference), because the inline editor is the only place to resolve it.
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
    if (validationBlockers.length > 0) {
      if (focusValidationBlocker(validationBlockers[0]?.fieldLabel ?? "")) return;
    }
    if (reconciliation.blockingCount > 0) {
      const ghost = document.querySelector<HTMLElement>(
        "[data-quotation-ghost] button"
      );
      if (ghost !== null) {
        revealIntoView(ghost);
        return;
      }
      const match = document.querySelector<HTMLElement>(
        '[aria-label^="Change quotation match"]'
      );
      if (match !== null) revealIntoView(match);
    }
  }, [
    validationBlockers,
    reconciliation.blockingCount,
    reconciliation.blockers,
    reconciliation.setBlockedAnnouncement,
    focusValidationBlocker,
  ]);
  useEffect(() => {
    if (!actionsInShell) {
      onShellActions?.(null);
      return;
    }
    if (view !== "entry") {
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
      onCancel,
      onBlockedActivate: () => callbacks.blockedActivate(),
    });
  }, [
    actionsInShell,
    view,
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
    reason,
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
  /**
   * #584: every successful write advances the retained version, so the next
   * write names the row as the server now stands. The retained version never
   * moves backwards (a session save may already have adopted a newer one).
   */
  const retainPersistedVersion = (order: { ID: number; Version: number }) => {
    persistedRef.current = {
      id: order.ID,
      version: latestPersistedVersion(
        order.ID,
        order.Version,
        persistedRef.current
      ),
    };
  };
  const ensurePersisted = async (): Promise<PersistedDraft> => {
    const current = editorStateRef.current;
    if (current.orderId > 0) {
      // The editor state is the newer truth after an ordinary save: the
      // retained version must follow it or the next pre-save conflicts.
      const existing = {
        id: current.orderId,
        version: latestPersistedVersion(
          current.orderId,
          current.version,
          persistedRef.current
        ),
      };
      persistedRef.current = existing;
      return existing;
    }
    if (persistedRef.current !== null) return persistedRef.current;
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
          retainPersistedVersion(outcome.order);
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
    let sourceCurrency: string | null = null;
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
      sourceCurrency = loaded.Source.Currency;
      // The inline model links immediately. The quotation becomes the source
      // of truth for available values, but nothing is adopted: every current
      // PO value is kept and every missing/extra line is acknowledged. The
      // editor's blue/green fields and quotation ghosts take over from here.
      const linkedState: PurchaseOrderEditorState = {
        ...editorStateRef.current,
        orderId: persistence.order.ID,
        version: persistence.order.Version,
      };
      const comparison = comparePurchaseOrder(
        linkedState,
        loaded.Source,
        activeCurrency
      );
      const implicit = decisionsForCurrentValues(comparison);
      const plan = planDraftLink(
        linkedState,
        loaded,
        activeCurrency,
        implicit.decisions,
        implicit.acknowledgedMissing,
        implicit.acknowledgedExtra,
        "keep"
      );
      if (plan.input === null) {
        throw new Error(plan.error ?? "The purchase order could not be linked.");
      }
      const linkedAnswer = await LinkPurchaseOrderDraftSource(
        new backend.PurchaseOrderDraftLinkInput({
          ID: persistence.order.ID,
          ExpectedVersion: persistence.order.Version,
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
        throw new Error(draftLinkConflictWarning(linked.conflict));
      }
      if (linked.kind === "savedRecord") {
        throw new Error("This purchase order was saved by someone else; reopen it before linking.");
      }
      if (linked.kind !== "saved") {
        throw new Error("The server answered without a linked purchase order.");
      }
      dispatch({ type: "saveSucceeded", order: linked.order, currency: activeCurrency });
      retainPersistedVersion(linked.order);
      setCompareSource({
        paymentTerms: loaded.Source.PaymentTerms,
        incoterms: loaded.Source.Incoterms,
      });
      setCompareLines(loaded.Source.Lines);
      loadedSourceForRef.current = linked.order.ID;
      reconciliation.reload();
      onLinked?.(linked.order);
    } catch (error) {
      const message = errorMessage(
        error,
        "The purchase order could not be saved. Try again."
      );
      const remedy =
        sourceCurrency === null
          ? ""
          : draftLinkCurrencyRemedy(
              state.sourceState,
              state.currency,
              sourceCurrency
            );
      setLinkError(remedy === "" ? message : `${message} ${remedy}`);
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
        retainPersistedVersion(outcome.order);
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
    // #580: a draft may be saved with zero lines, but a saved record may not
    // be finalized empty. Refuse before persisting or loading the source.
    if (state.lines.length === 0) {
      const message = "Add at least one line before Final Save.";
      dispatch({ type: "saveFailed", error: message });
      reconciliation.setBlockedAnnouncement(`Final Save is blocked. 1 blocker: ${message}.`);
      const addLine = document.querySelector<HTMLElement>('[aria-label="Add line"]');
      if (addLine !== null) addLine.focus();
      return;
    }
    // Contract section 7: commit the focused input first, before readiness.
    // The inline editor records a differing field as a deliberate keep, so
    // the commit resolves the session item instead of leaving it undecided.
    if (focusedLineRef.current !== null) {
      const target = focusedLineRef.current;
      focusedLineRef.current = null;
      const result = reconciliation.commitLine(target.clientKey, target.field, undefined, {
        autoKeep: true,
      });
      if (result !== null && result.validationError !== null) {
        dispatch({ type: "saveFailed", error: result.validationError });
        return;
      }
    } else if (focusedFieldRef.current !== null) {
      const field = focusedFieldRef.current;
      focusedFieldRef.current = null;
      const result = reconciliation.commitField(field, undefined, { autoKeep: true });
      if (result !== null && result.validationError !== null) {
        dispatch({ type: "saveFailed", error: result.validationError });
        return;
      }
    }
    // U8: named readiness before any save. Validation blockers use the same
    // rules as the payload builder; the first blocker is focused so its error
    // becomes visible. The confirmation never opens while blocked.
    const liveValidation = listValidationBlockers(state, activeCurrency);
    if (liveValidation.length > 0) {
      const first = liveValidation[0];
      dispatch({ type: "saveFailed", error: first.message });
      reconciliation.setBlockedAnnouncement(
        `Final Save is blocked. ${liveValidation.length} ${liveValidation.length === 1 ? "blocker" : "blockers"}: ${liveValidation.map((entry) => entry.message).join("; ")}.`
      );
      const target = document.querySelector<HTMLElement>(
        `[aria-label="${first.fieldLabel.replace(/"/g, "")}"]`
      );
      if (target !== null) revealIntoView(target);
      return;
    }
    // A draft may be saved without a customer PO number, but a finalized
    // record may not. Refuse before persisting or opening the confirmation so
    // an empty required field never looks ready to save.
    if (state.header.poNumber.trim() === "") {
      const message = "A customer PO number is required before the PO can be saved.";
      dispatch({ type: "saveFailed", error: message });
      reconciliation.setBlockedAnnouncement(`Final Save is blocked. 1 blocker: ${message}`);
      const target = document.querySelector<HTMLElement>(
        '[aria-label="Customer PO number"]'
      );
      if (target !== null) revealIntoView(target);
      return;
    }
    // A remaining quotation decision (an unignored ghost) is exactly what the
    // inline editor shows, so focus it instead of opening a review.
    if (reconciliation.blockingCount > 0) {
      blockedActivate();
      return;
    }
    const preview = buildPurchaseOrderUpdate(state, activeCurrency);
    if (preview.input === null) {
      dispatch({ type: "saveFailed", error: preview.error ?? "The draft is incomplete." });
      return;
    }
    setReviewLoading(true);
    try {
      // A Final Save without a manual Save Draft must still persist the
      // working copy and the session BEFORE the source is loaded: the backend
      // gate reads the persisted session, so unsaved decisions would finalize
      // against stale items.
      let persisted = { id: state.orderId, version: state.version };
      if (!correcting) {
        const saved = await persistWorkingCopy(false);
        if (saved.kind === "conflict") {
          dispatch({ type: "saveConflict" });
          setPendingWrite("save");
          setConflict(saved.conflict);
          return;
        }
        if (saved.kind === "savedRecord") {
          dispatch({ type: "saveConflict" });
          setPendingWrite("save");
          setConflict(savedRecordAnswer(state.version));
          return;
        }
        if (saved.kind === "error") {
          dispatch({ type: "saveFailed", error: saved.message });
          return;
        }
        const synced = await reconciliation.persistAfterDraftSave(saved.order.Version);
        if (!synced.ok) {
          setSessionBannerError(
            "The reconciliation could not be saved. Your draft is stored; saving again retries the sync."
          );
          dispatch({
            type: "saveFailed",
            error: synced.message ?? "The reconciliation decisions could not be saved yet.",
          });
          return;
        }
        setSessionBannerError(null);
        const finalVersion = synced.version ?? saved.order.Version;
        adoptSessionVersion(saved.order.ID, saved.order.Version, synced.version);
        persisted = { id: saved.order.ID, version: finalVersion };
      } else {
        persisted = await ensurePersisted();
      }
      const loaded = await GetPurchaseOrderReview(persisted.id);
      setCompareSource({
        paymentTerms: loaded.Source.PaymentTerms,
        incoterms: loaded.Source.Incoterms,
      });
      setCompareLines(loaded.Source.Lines);
      loadedSourceForRef.current = persisted.id;
      // The confirmation shows the exact source branch and totals, but the
      // choices are already visible in the editor: every remaining difference
      // is the PO's own value, every missing quotation line was left off, and
      // every extra line was entered deliberately.
      const comparison = comparePurchaseOrder(state, loaded.Source, activeCurrency);
      const implicit = decisionsForCurrentValues(comparison);
      const gate = evaluateReview(
        state,
        loaded.Source,
        activeCurrency,
        implicit.decisions,
        implicit.acknowledgedMissing,
        implicit.acknowledgedExtra
      );
      if (!gate.canContinue) {
        dispatch({
          type: "saveFailed",
          error: gate.blockers[0] ?? "The purchase order cannot be saved yet.",
        });
        reconciliation.setBlockedAnnouncement(
          `Final Save is blocked. ${gate.blockers.length} ${gate.blockers.length === 1 ? "blocker" : "blockers"}: ${gate.blockers.join("; ")}.`
        );
        const match = document.querySelector<HTMLElement>(
          '[aria-label^="Change quotation match"]'
        );
        if (match !== null) revealIntoView(match);
        return;
      }
      const applied = buildPurchaseOrderUpdate(gate.applied, activeCurrency);
      if (applied.input === null) {
        dispatch({
          type: "saveFailed",
          error: applied.error ?? "The purchase order is incomplete.",
        });
        return;
      }
      setReview(loaded);
      setDocumentConfirmed(false);
      setAssignConfirmation(state.header.confirmationRequested);
      setView("confirm");
    } catch (error) {
      dispatch({
        type: "saveFailed",
        error: errorMessage(error, "The purchase order could not be saved. Try again."),
      });
    } finally {
      setReviewLoading(false);
    }
  };

  // Ticket 02: keeps the shell callbacks fresh without retriggering the
  // report effect above. Assigned every render; the effect reads it.
  shellActionsRef.current = { save, startReview, blockedActivate };

  const finalSave = async (expectedVersion: number, overwrite: boolean) => {
    if (review === null) return;
    const comparison = comparePurchaseOrder(state, review.Source, activeCurrency);
    const implicit = decisionsForCurrentValues(comparison);
    const result = buildFinalizeInput(
      state,
      review.Source,
      activeCurrency,
      review.DocumentState,
      implicit.decisions,
      implicit.acknowledgedMissing,
      implicit.acknowledgedExtra,
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
        retainPersistedVersion(outcome.order);
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

  // The final-save confirmation reads the same comparison as the editor:
  // every remaining difference is the purchase order's own value (using the
  // quotation value has already removed it), every missing quotation line was
  // left off deliberately, and every extra line was entered deliberately. The
  // inline editor has already asked the only questions that need asking.
  const currentComparison =
    review === null
      ? null
      : comparePurchaseOrder(state, review.Source, activeCurrency);
  const currentDecisions =
    currentComparison === null
      ? null
      : decisionsForCurrentValues(currentComparison);
  const evaluation =
    review === null || currentDecisions === null
      ? null
      : evaluateReview(
          state,
          review.Source,
          activeCurrency,
          currentDecisions.decisions,
          currentDecisions.acknowledgedMissing,
          currentDecisions.acknowledgedExtra
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
  // A rejected ghost is an explicit omission: record it against every source
  // line the item covers so the persisted session and the shell gate agree.
  const ignoreQuotationItem = (item: SourceItem) => {
    const references =
      item.subItems.length > 0
        ? item.subItems.map((sub) => `${item.itemNo}.${sub.subNo}`)
        : [String(item.itemNo)];
    for (const reference of references) {
      reconciliation.acknowledgeMissing(`source:${reference}:match`);
    }
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
      setPendingFocus(
        `Description for line ${nextAt === -1 ? at + 1 : nextAt + 1}`
      );
    } else {
      setPendingFocus("Add line");
    }
    reconciliation.announce(`Line ${at + 1} removed.`);
  };

  if (view === "confirm" && review !== null && evaluation !== null) {
    // Build the displayed values from the state the confirmation will submit.
    // After the pre-save, that state carries the server's row ids and aliases;
    // a copy built before the save could omit them from the final payload.
    const confirmation = buildPurchaseOrderUpdate(state, activeCurrency);
    if (confirmation.input !== null) {
      return (
        <>
          <PurchaseOrderFinalConfirmationView
            review={review}
            evaluation={evaluation}
            payload={confirmation.input}
            currency={activeCurrency}
            documentConfirmed={documentConfirmed}
            onToggleDocumentConfirmed={() =>
              setDocumentConfirmed((current) => !current)
            }
            assignConfirmation={assignConfirmation}
            onToggleAssignConfirmation={() =>
              setAssignConfirmation((current) => !current)
            }
            onBack={() => setView("entry")}
            onFinalSave={() => void finalSave(state.version, false)}
            saving={finalSaving}
            error={state.error}
          />
          {conflictPrompt}
        </>
      );
    }
  }

  return (
    <div className="flex flex-col gap-6">
      {linkLoading ? (
        <StatusBanner tone="info">
          <span role="status">Saving the draft and linking the quotation…</span>
        </StatusBanner>
      ) : null}
      {linkError !== null ? (
        <StatusBanner tone="danger">{linkError}</StatusBanner>
      ) : null}
      {reconciliation.status === "error" ? (
        <StatusBanner tone="danger">
          The quotation comparison could not be loaded.{" "}
          {reconciliation.error ?? "Try again."}
        </StatusBanner>
      ) : null}
      {sessionBannerError !== null ? (
        <StatusBanner tone="danger">{sessionBannerError}</StatusBanner>
      ) : null}
      <div className="@container min-w-0">
        <PurchaseOrderEditorView
          state={state}
          dispatch={dispatch}
          currency={activeCurrency}
          currencyOptions={currencyOptions}
          uomOptions={uomOptions}
          incotermOptions={incotermOptions}
          quoteSource={compareSource}
          quoteLines={compareLines}
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
          onCommitField={(field, explicitValue) => {
            focusedFieldRef.current = field;
            reconciliation.commitField(field, explicitValue, { autoKeep: true });
          }}
          onFocusField={(field) => {
            focusedFieldRef.current = field;
            focusedLineRef.current = null;
          }}
          onFocusLine={(lineKey, field) => {
            focusedFieldRef.current = null;
            focusedLineRef.current = { clientKey: lineKey, field };
          }}
          onIgnoreQuotationItem={ignoreQuotationItem}
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
            reconciliation.commitLine(lineKey, field, explicitRaw, {
              autoKeep: true,
            })
          }
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
      </div>
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
      {conflictPrompt}
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
