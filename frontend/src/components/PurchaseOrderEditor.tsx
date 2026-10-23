import { useEffect, useReducer, useRef, useState } from "react";
import {
  CorrectPurchaseOrder,
  CreatePurchaseOrderBlankDraft,
  FinalizePurchaseOrder,
  GetPurchaseOrderExtraction,
  GetPurchaseOrderReview,
  LinkPurchaseOrderDraftSource,
  PreviewPurchaseOrderDraftLink,
  StartPurchaseOrderExtraction,
  UpdatePurchaseOrderDraft,
} from "@backend/backend/Quotation";
import { backend } from "@backend/models";
import type { CompanyUnitOption } from "../companyUnits";
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
  initialPurchaseOrderEditorState,
  isPurchaseOrderDirty,
  parseDiscountPercent,
  purchaseOrderEditorReducer,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorAction,
  type PurchaseOrderEditorState,
  type PurchaseOrderHeader,
  type PurchaseOrderLine,
  type PurchaseOrderLineCalculation,
  type PurchaseOrderUpdateInput,
} from "../purchaseOrder";
import {
  applyExtractionSelection,
  createExtractionEditTracker,
  extractionErrorMessage,
  extractionIsStale,
  extractionProgressText,
  extractionResultActionable,
  isTerminalExtractionStatus,
  resolveExtractionConflicts,
  resolvedExtractionSelection,
  type ExtractionChoice,
  type ExtractionEditTracker,
  type PurchaseOrderExtractionDocumentDTO,
  type PurchaseOrderExtractionStateDTO,
} from "../poExtraction";
import type { PurchaseOrderDocumentStateDTO } from "../purchaseOrderDocument";
import {
  draftLinkConflictWarning,
  planDraftLink,
  type PurchaseOrderDraftLinkReviewDTO,
  type PurchaseOrderDraftTaxChoice,
} from "../poDraftLink";
import { PurchaseOrderDocumentPanel } from "./PurchaseOrderDocumentPanel";
import { PurchaseOrderExtractionPanelView } from "./PurchaseOrderExtractionPanel";
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
import { Select } from "./Select";
import {
  SERVICE_CATEGORY_OPTIONS,
  TAX_TYPE_OPTIONS,
  TAX_ZONE_OPTIONS,
} from "../tax";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";

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
  /** #498: a currency change invalidates an actionable extraction result. */
  onCurrencyChange?: (next: CurrencyInfo) => void;
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

function sameExtractionDocument(
  left: PurchaseOrderExtractionDocumentDTO | null,
  right: PurchaseOrderExtractionDocumentDTO | null
): boolean {
  if (left === null || right === null) return left === right;
  return (
    left.ID === right.ID &&
    left.Version === right.Version &&
    left.SHA256 === right.SHA256
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

function errorText(error: unknown): string {
  if (typeof error === "string" && error.trim() !== "") return error;
  if (error instanceof Error && error.message.trim() !== "") return error.message;
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message.trim() !== "") return message;
  }
  return "The purchase order could not be saved. Try again.";
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
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: "text" | "date" | "email" | "number";
  placeholder?: string;
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
}: {
  line: PurchaseOrderEditorState["lines"][number];
  index: number;
  currency: CurrencyInfo;
  uomOptions: CompanyUnitOption[];
  calculation: PurchaseOrderLineCalculation | null;
  dispatch: (action: PurchaseOrderEditorAction) => void;
}) {
  return (
    <div
      className="grid gap-3 rounded-lg border border-rule bg-white p-3 sm:grid-cols-[minmax(0,1fr)_120px_140px_170px_auto] sm:items-end"
      data-po-line={index + 1}
    >
      <label className="grid gap-1.5">
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
        />
        <span className="text-caption text-ink-400">
          {matchLabel(line.quotationItemNo, line.quotationSubNo)}
        </span>
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
        />
      </label>
      <div className="grid gap-1.5">
        <span className="text-small font-medium text-ink-700">UoM</span>
        <Select
          ariaLabel={`Line ${index + 1} unit of measure`}
          value={line.uom}
          options={uomOptions.length > 0 ? uomOptions : [{ value: line.uom, label: line.uom || "—" }]}
          onChange={(value) =>
            dispatch({
              type: "lineChanged",
              key: line.key,
              patch: { uom: String(value) },
            })
          }
        />
      </div>
      <label className="grid gap-1.5">
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
        />
      </label>
      <Button
        size="sm"
        aria-label={`Remove line ${index + 1}`}
        onClick={() => dispatch({ type: "lineRemoved", key: line.key })}
      >
        Remove
      </Button>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-rule pt-2 sm:col-span-5">
        <label className="flex items-center gap-2 text-small text-ink-700">
          <input
            type="checkbox"
            aria-label={`Line ${index + 1} discount enabled`}
            checked={line.discountEnabled}
            onChange={(event) =>
              dispatch({
                type: "lineChanged",
                key: line.key,
                patch: { discountEnabled: event.target.checked },
              })
            }
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
          />
          <span className="text-small text-ink-500">%</span>
        </div>
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

export function PurchaseOrderEditorView({
  state,
  dispatch,
  currency,
  currencyOptions = [],
  uomOptions,
  incotermOptions,
  onLinkQuotation,
  onCurrencyChange,
  onSave,
  onCancel,
  onFinalSave,
  finalSaving,
  mode = "draft",
  correctionReason = "",
  onCorrectionReasonChange,
  confirmationNumber = "",
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
    <div className="flex flex-col gap-6">
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
                  if (onCurrencyChange !== undefined) {
                    onCurrencyChange(next);
                    return;
                  }
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
          />
          <div className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">Incoterms</span>
            <Select
              ariaLabel="Incoterms"
              value={state.header.incoterms}
              options={incotermOptions}
              onChange={(value) =>
                dispatch({ type: "headerChanged", patch: { incoterms: String(value) } })
              }
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
              onClick={() =>
                dispatch({ type: "lineAdded", uom: uomOptions[0]?.value ?? "" })
              }
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
              disabled={state.saving || finalSaving || sourceLess}
              title={
                sourceLess
                  ? "Link a Quotation before finalizing; the server also refuses a source-less finalize."
                  : undefined
              }
              onClick={onFinalSave}
            >
              {finalSaving ? "Loading review…" : "Final Save"}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
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

  // #477: the extraction review. The run is the server's, the selection is
  // local, and applying is the one action that moves a proposal into the
  // working copy. A run is never applied automatically and a late response
  // cannot overwrite a value staff has since edited: the selection starts from
  // what the editor held when the result arrived, and Apply re-checks each
  // ticked value against the current header.
  const [extractionState, setExtractionState] =
    useState<PurchaseOrderExtractionStateDTO | null>(null);
  const [extractionSelection, setExtractionSelection] = useState<Record<string, boolean>>({});
  /**
   * #479: the editor as it stood when tracking began, plus the choices staff
   * made for conflicting values. The tracker distinguishes "still empty" from
   * "cleared deliberately"; the choices are kept across a retry and discarded
   * with the document generation.
   */
  const [extractionTracker, setExtractionTracker] = useState<ExtractionEditTracker>(() =>
    createExtractionEditTracker(0, state.header, state.lines)
  );
  const extractionTrackerRef = useRef(extractionTracker);
  extractionTrackerRef.current = extractionTracker;
  const [extractionChoices, setExtractionChoices] = useState<
    Record<string, ExtractionChoice>
  >({});
  const [extractionStarting, setExtractionStarting] = useState(false);
  const [extractionApplying, setExtractionApplying] = useState(false);
  const [extractionError, setExtractionError] = useState<string | null>(null);
  const [extractionApplied, setExtractionApplied] = useState<{
    fields: number;
    rows: number;
  } | null>(null);
  const [dismissedExtractionId, setDismissedExtractionId] = useState<number | null>(null);
  // The active PDF generation as the document panel has reported it. Undefined
  // means "not loaded yet"; null means "no actionable document" -- either the
  // record deliberately has no PDF, or a replacement's state read has not
  // returned yet. The review panel and Apply compare the run against this, not
  // against whatever document the last extraction response happened to mention.
  const [activeDocument, setActiveDocument] = useState<
    PurchaseOrderExtractionDocumentDTO | null | undefined
  >(undefined);
  const activeDocumentRef = useRef<PurchaseOrderExtractionDocumentDTO | null | undefined>(
    undefined
  );
  // Bumped whenever the active document changes. A response that was requested
  // for the previous generation is discarded instead of adopted.
  const extractionGeneration = useRef(0);
  const extractionPoll = useRef<ReturnType<typeof setTimeout> | null>(null);
  const extractionInitialized = useRef<number | null>(null);
  // #498: a run whose result must not become actionable again without a new
  // Extract. A currency change and a stale Apply both add to this set; a new
  // run has a new id, so it is unaffected.
  const extractionRejectedRuns = useRef<Set<number>>(new Set());
  // The draft currency in effect when the result was adopted. Amounts are
  // parsed from the stored result through this currency; changing the draft
  // currency invalidates the result rather than reinterpreting it.
  const extractionCurrencyRef = useRef<string | null>(null);
  // Async poll callbacks must see the editor as it is now, not as it was when
  // the first response was scheduled.
  const editorStateRef = useRef(state);
  editorStateRef.current = state;

  /**
   * Re-bases the tracker on the values now in the editor. It is called when a
   * document generation ends or a result is consumed: from that point the new
   * values are the baseline a later edit is measured against.
   */
  function rebaseExtractionTracker(
    header: PurchaseOrderHeader = editorStateRef.current.header,
    lines: PurchaseOrderLine[] = editorStateRef.current.lines
  ): ExtractionEditTracker {
    const next = createExtractionEditTracker(
      extractionGeneration.current,
      header,
      lines
    );
    extractionTrackerRef.current = next;
    setExtractionTracker(next);
    return next;
  }

  /** The tracker for the active document generation, created on first need. */
  function ensureExtractionTracker(): ExtractionEditTracker {
    const current = extractionTrackerRef.current;
    if (current.generation === extractionGeneration.current) return current;
    return rebaseExtractionTracker();
  }

  function clearExtractionPoll() {
    if (extractionPoll.current !== null) {
      clearTimeout(extractionPoll.current);
      extractionPoll.current = null;
    }
  }

  /**
   * Clears the result and the apply selection now. Called on a document change
   * (and on a successful attach/replace/remove before its refresh returns), so
   * old proposals stop being actionable at the moment the PDF changes rather
   * than one round trip later.
   *
   * The generation bump is what discards responses that were requested for the
   * document this call ends. The retained reference is cleared as well, so a
   * pre-change response cannot compare equal to an old document if a call site
   * ever skips its generation check: until the document panel reports the
   * replacement, no document is actionable.
   */
  const invalidateExtraction = () => {
    extractionGeneration.current += 1;
    extractionInitialized.current = null;
    clearExtractionPoll();
    activeDocumentRef.current = null;
    setActiveDocument(null);
    setExtractionState(null);
    setExtractionSelection({});
    setExtractionApplied(null);
    setDismissedExtractionId(null);
    setExtractionError(null);
    // #479: the old run's baseline and choices describe a PDF that is no
    // longer active. They end with the generation, before any refresh returns.
    setExtractionChoices({});
    rebaseExtractionTracker();
    extractionCurrencyRef.current = null;
  };

  /**
   * #498: ends one result without declaring the active PDF unusable. A
   * currency change (or a stale Apply) discards the run, its selection and its
   * tracker, but the document generation stays known so Extract can start
   * again immediately. The run id stays rejected, so a later refresh cannot
   * quietly restore it.
   */
  const rejectExtraction = (runId: number, message: string) => {
    extractionRejectedRuns.current.add(runId);
    extractionGeneration.current += 1;
    extractionInitialized.current = null;
    clearExtractionPoll();
    setExtractionState(null);
    setExtractionSelection({});
    setExtractionChoices({});
    setExtractionApplied(null);
    setExtractionError(message);
    extractionCurrencyRef.current = null;
    rebaseExtractionTracker();
  };

  // Declared as a function so adoptExtraction and the poll can call each other
  // without a render-time ordering problem; no timer fires during render.
  function scheduleExtractionPoll(runId: number, attempt = 0) {
    if (attempt >= 120) {
      setExtractionError(
        "Extraction has not reported a result after two minutes. Run Extract again, or enter the values manually."
      );
      clearExtractionPoll();
      return;
    }
    clearExtractionPoll();
    extractionPoll.current = setTimeout(() => {
      void (async () => {
        // The generation is captured when this read starts, not when the poll
        // was scheduled: a replacement between the timer firing and the answer
        // arriving must discard the answer.
        const generation = extractionGeneration.current;
        try {
          const next = await GetPurchaseOrderExtraction(editorStateRef.current.orderId);
          if (generation !== extractionGeneration.current) return;
          const run = next.Run;
          if (run !== null && run !== undefined && run.ID === runId && !isTerminalExtractionStatus(run.Status)) {
            setExtractionState(next);
            scheduleExtractionPoll(runId, attempt + 1);
            return;
          }
          adoptExtraction(next);
        } catch (pollError) {
          if (generation !== extractionGeneration.current) return;
          setExtractionError(errorText(pollError));
          clearExtractionPoll();
        }
      })();
    }, 1000);
  }

  const adoptExtraction = (next: PurchaseOrderExtractionStateDTO) => {
    const run = next.Run ?? null;
    const active = activeDocumentRef.current;
    // A response for a different PDF generation is never made actionable: it
    // may be the answer to a request that was already in flight when the
    // document changed.
    if (run != null && active !== undefined && extractionIsStale(run, active)) {
      setExtractionState({ Run: run, Document: active });
      extractionInitialized.current = null;
      setExtractionSelection({});
      setExtractionChoices({});
      rebaseExtractionTracker();
      return;
    }
    setExtractionState(next);
    if (run != null && run.Status === "succeeded" && run.Result != null &&
      !extractionRejectedRuns.current.has(run.ID)) {
      extractionCurrencyRef.current = editorStateRef.current.currency;
      if (extractionInitialized.current !== run.ID) {
        extractionInitialized.current = run.ID;
        const editor = editorStateRef.current;
        // #479: the default selection is the resolved matrix, not a blanket
        // "empty means preselect". A field staff cleared while the run was
        // processing starts unchosen and waits for a decision.
        const plan = resolveExtractionConflicts(
          run.Result,
          editor.header,
          editor.lines,
          ensureExtractionTracker(),
          activeCurrency
        );
        setExtractionSelection(plan.selection);
      }
    } else {
      extractionInitialized.current = null;
      setExtractionSelection({});
      // A queued/running run still needs a baseline: staff can keep typing,
      // and an edit made from here on is protected when the result arrives.
      if (run != null) ensureExtractionTracker();
    }
    if (run != null && !isTerminalExtractionStatus(run.Status)) {
      scheduleExtractionPoll(run.ID);
    }
  };

  useEffect(() => {
    let cancelled = false;
    // A transient editor has no row to read; the first upload/save adopts one
    // and the explicit refresh paths use that id.
    if (order.ID < 1) return;
    // The initial read belongs to the document generation that was active when
    // it started. A replacement while it is in flight ends that generation.
    const generation = extractionGeneration.current;
    void (async () => {
      try {
        const next = await GetPurchaseOrderExtraction(order.ID);
        if (cancelled || generation !== extractionGeneration.current) return;
        adoptExtraction(next);
      } catch (loadError) {
        if (cancelled || generation !== extractionGeneration.current) return;
        setExtractionError(errorText(loadError));
      }
    })();
    return () => {
      cancelled = true;
      clearExtractionPoll();
    };
    // Re-reading is keyed to the record; the callbacks above are intentionally
    // stable for its lifetime and read current values through refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [order.ID]);

  const refreshExtraction = () => {
    const generation = extractionGeneration.current;
    clearExtractionPoll();
    setDismissedExtractionId(null);
    setExtractionApplied(null);
    void (async () => {
      try {
        const next = await GetPurchaseOrderExtraction(editorStateRef.current.orderId);
        if (generation !== extractionGeneration.current) return;
        adoptExtraction(next);
      } catch (refreshError) {
        if (generation !== extractionGeneration.current) return;
        setExtractionError(errorText(refreshError));
      }
    })();
  };

  const handleDocumentStateChange = (next: PurchaseOrderDocumentStateDTO) => {
    const document: PurchaseOrderExtractionDocumentDTO | null = next.Document
      ? {
          ID: next.Document.ID,
          Version: next.Document.Version,
          SHA256: next.Document.SHA256,
        }
      : null;
    const previous = activeDocumentRef.current;
    const run = extractionState?.Run ?? null;
    const changed =
      previous !== undefined
        ? !sameExtractionDocument(previous, document)
        : run != null && extractionIsStale(run, document);
    // Invalidate before reporting the new document: the clear inside ends the
    // old generation, and the value assigned next is the one every response
    // from here on is compared against.
    if (changed) invalidateExtraction();
    activeDocumentRef.current = document;
    setActiveDocument(document);
    refreshExtraction();
  };

  const startExtraction = () => {
    if (extractionStarting) return;
    // The start belongs to the document generation that is active when it is
    // requested. A replacement while it is in flight ends that generation, and
    // its run is never made actionable here.
    const generation = extractionGeneration.current;
    clearExtractionPoll();
    // #479: a retry is a request, never a reset. The working copy and the
    // reviewed choices stay; only the old row ticks are dropped because they
    // name rows of a result that is about to be replaced. The tracker keeps
    // its original baseline, so edits made before and between retries remain
    // protected.
    ensureExtractionTracker();
    setExtractionSelection({});
    setExtractionStarting(true);
    setExtractionError(null);
    setExtractionApplied(null);
    setDismissedExtractionId(null);
    void (async () => {
      try {
        const persisted = await ensurePersisted();
        const next = await StartPurchaseOrderExtraction(persisted.id);
        if (generation !== extractionGeneration.current) return;
        adoptExtraction(next);
      } catch (startError) {
        if (generation !== extractionGeneration.current) return;
        setExtractionError(errorText(startError));
      } finally {
        setExtractionStarting(false);
      }
    })();
  };

  const toggleExtraction = (key: string, selected: boolean) => {
    setExtractionSelection((current) => ({ ...current, [key]: selected }));
  };

  const chooseExtraction = (key: string, choice: ExtractionChoice) => {
    setExtractionChoices((current) => ({ ...current, [key]: choice }));
  };

  const currencyFor = (code: string): CurrencyInfo =>
    currencyOptions.find((option) => option.code === code) ?? currency;

  /**
   * #498: Apply re-reads the latest run and active document before writing.
   * The panel's earlier read can be seconds old, and a PDF replaced in another
   * session is invisible to a tab that never refreshed. A stale, replaced or
   * superseded result discards the selection and writes nothing; the staff
   * member runs Extract again. A currency change invalidates the amounts the
   * same way: the stored decimals are parsed with the draft currency.
   */
  const applyExtraction = async () => {
    if (extractionApplying || extractionStarting) return;
    const run = extractionState?.Run;
    if (run == null || run.Result == null) return;
    const orderId = editorStateRef.current.orderId;
    if (orderId < 1) return;
    // The read belongs to the generation that was active when Apply started.
    const generation = extractionGeneration.current;
    setExtractionApplying(true);
    setExtractionError(null);
    try {
      const latest = await GetPurchaseOrderExtraction(orderId);
      if (generation !== extractionGeneration.current) return;
      const latestRun = latest.Run ?? null;
      const latestDocument = latest.Document ?? null;
      if (
        latestRun == null ||
        latestRun.ID !== run.ID ||
        latestRun.Status !== "succeeded" ||
        latestRun.Result == null ||
        extractionIsStale(latestRun, latestDocument)
      ) {
        if (!sameExtractionDocument(activeDocumentRef.current ?? null, latestDocument)) {
          activeDocumentRef.current = latestDocument;
          setActiveDocument(latestDocument);
        }
        rejectExtraction(
          latestRun?.ID ?? run.ID,
          "The PDF changed after this extraction, so its proposals were discarded. Your entries were not changed. Run Extract again."
        );
        return;
      }
      if (!sameExtractionDocument(activeDocumentRef.current ?? null, latestDocument)) {
        activeDocumentRef.current = latestDocument;
        setActiveDocument(latestDocument);
      }
      const current = editorStateRef.current;
      if (extractionCurrencyRef.current !== null &&
        extractionCurrencyRef.current !== current.currency) {
        rejectExtraction(
          latestRun.ID,
          `The draft currency changed after this extraction, so its proposals were discarded. Run Extract again to read the amounts in ${current.currency}.`
        );
        return;
      }
      const currentCurrency = currencyFor(current.currency);
      const plan = resolveExtractionConflicts(
        latestRun.Result,
        current.header,
        current.lines,
        ensureExtractionTracker(),
        currentCurrency
      );
      const selection = resolvedExtractionSelection(
        plan,
        extractionChoices,
        extractionSelection
      );
      const applied = applyExtractionSelection(
        latestRun.Result,
        selection,
        current.header,
        current.lines,
        current.nextKey,
        currentCurrency
      );
      if (applied.appliedFields === 0 && applied.appliedRows === 0) {
        setExtractionError(
          "No selected proposal could be applied. Choose a value to use, or enter it manually."
        );
        return;
      }
      dispatch({
        type: "extractionApplied",
        header: applied.headerPatch,
        lines: applied.lines,
        nextKey: current.nextKey + applied.appliedRows,
      });
      // The applied proposals are consumed; the rest of the result is not. Only
      // the baseline of what Apply actually wrote moves, so an unresolved
      // cleared field or line conflict stays a conflict until it is decided or
      // the result is discarded.
      const previous = extractionTrackerRef.current;
      const baselineHeader = { ...previous.baseline.header };
      const appliedHeaderKeys = [
        "poNumber",
        "poDate",
        "requestedDeliveryDate",
        "paymentTerms",
        "incoterms",
      ] as const;
      for (const key of appliedHeaderKeys) {
        if (Object.prototype.hasOwnProperty.call(applied.headerPatch, key)) {
          baselineHeader[key] = String(applied.headerPatch[key] ?? "");
        }
      }
      const baselineLines =
        applied.lines !== undefined
          ? applied.lines.map((line) => ({
              key: line.key,
              description: line.description,
              quantity: line.quantity,
              uom: line.uom,
              unitPrice: line.unitPrice,
            }))
          : previous.baseline.lines;
      const nextTracker: ExtractionEditTracker = {
        generation: previous.generation,
        baseline: { header: baselineHeader, lines: baselineLines },
      };
      extractionTrackerRef.current = nextTracker;
      setExtractionTracker(nextTracker);
      setExtractionChoices({});
      setExtractionApplied({ fields: applied.appliedFields, rows: applied.appliedRows });
    } catch (error) {
      if (generation !== extractionGeneration.current) return;
      setExtractionError(errorText(error));
    } finally {
      setExtractionApplying(false);
    }
  };

  /**
   * #498: a currency change while a result is actionable invalidates it. The
   * stored result has no currency of its own; its printed decimals were parsed
   * with the draft currency in effect, so reinterpreting them under another
   * currency would apply a different amount. The run is rejected so a later
   * refresh cannot quietly restore it.
   */
  const changeCurrency = (next: CurrencyInfo) => {
    if (next.code === state.currency) return;
    const run = extractionState?.Run ?? null;
    const active =
      activeDocumentRef.current !== undefined
        ? activeDocumentRef.current
        : extractionState?.Document ?? null;
    if (extractionResultActionable(run, active) && run != null) {
      rejectExtraction(
        run.ID,
        `The draft currency changed to ${next.code} after this extraction, so its proposals were discarded. Run Extract again to read the amounts in ${next.code}.`
      );
    }
    dispatch({
      type: "currencyChanged",
      code: next.code,
      from: activeCurrency,
      to: next,
    });
  };

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

  /**
   * Persists the transient row the first time an action needs an id (Save
   * draft, PDF upload or extraction). The created id is adopted immediately, so
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
   * #493/#498: one persistence of the working copy, shared by Save, the
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
      onSaved(outcome.order, correcting);
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
      setLinkError(errorText(error));
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
      setLinkError(errorText(error));
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
        dispatch({ type: "saveFailed", error: errorText(error) });
      }
      return;
    }
    try {
      // Save, upload and extraction share one in-flight create. A Save whose
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
        onSaved(outcome.order, correcting);
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
      dispatch({ type: "saveFailed", error: errorText(error) });
    }
  };

  const startReview = async () => {
    const result = buildPurchaseOrderUpdate(state, activeCurrency);
    if (result.input === null) {
      dispatch({ type: "saveFailed", error: result.error ?? "The draft is incomplete." });
      return;
    }
    setReviewLoading(true);
    try {
      const persisted = await ensurePersisted();
      const loaded = await GetPurchaseOrderReview(persisted.id);
      const initial = initialFinalizeReviewState();
      setReview(loaded);
      setDecisions(initial.decisions);
      setAcknowledgedMissing(initial.acknowledgedMissing);
      setAcknowledgedExtra(initial.acknowledgedExtra);
      setDocumentConfirmed(initial.documentConfirmed);
      setAssignConfirmation(initial.assignConfirmation);
      setView("review");
    } catch (error) {
      dispatch({ type: "saveFailed", error: errorText(error) });
    } finally {
      setReviewLoading(false);
    }
  };

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
        dispatch({ type: "saveSucceeded", order: outcome.order, currency: activeCurrency });
        setConflict(null);
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
      dispatch({ type: "saveFailed", error: errorText(error) });
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

  const extractionRun = extractionState?.Run ?? null;
  const activeExtractionDocument =
    activeDocument !== undefined
      ? activeDocument
      : extractionState?.Document ?? null;
  const extractionStale =
    extractionRun != null &&
    extractionIsStale(extractionRun, activeExtractionDocument);
  const extractionInFlight =
    extractionRun != null && !isTerminalExtractionStatus(extractionRun.Status);
  const showExtractionPanel =
    extractionResultActionable(extractionRun, activeExtractionDocument) &&
    dismissedExtractionId !== extractionRun?.ID &&
    !extractionRejectedRuns.current.has(extractionRun?.ID ?? -1);
  // #479: the comparison is recomputed from the editor as staff type, not
  // frozen when the result arrived, and every choice is folded into the one
  // selection Apply consumes.
  const extractionPlan =
    showExtractionPanel && extractionRun?.Result != null
      ? resolveExtractionConflicts(
          extractionRun.Result,
          state.header,
          state.lines,
          extractionTracker,
          activeCurrency
        )
      : null;
  const extractionResolvedSelection =
    extractionPlan !== null
      ? resolvedExtractionSelection(
          extractionPlan,
          extractionChoices,
          extractionSelection
        )
      : extractionSelection;

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
      <PurchaseOrderDocumentPanel
        purchaseOrderId={state.orderId}
        // A draft Save remounts this editor through onSaved. Blocking the
        // document controls while it is in flight keeps an attach from starting
        // against a panel that is about to disappear (and losing its error).
        disabled={state.saving}
        // #493/#498: an upload must persist the working copy before attaching,
        // so a reload cannot lose the values staff saw when they chose the file.
        // It notifies no one: onSaved would remount this editor mid-attach and a
        // failed upload's error would be set on the unmounted panel. The shared
        // save still dispatches saveSucceeded, which keeps the editor's local
        // id and version. The panel's interface is unchanged: it still only asks
        // for an id.
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
        onExtract={startExtraction}
        extractionStatus={
          extractionRun?.Status === "queued" || extractionRun?.Status === "running"
            ? extractionRun.Status
            : null
        }
        extractionStarting={extractionStarting}
        onStateChange={handleDocumentStateChange}
        onDocumentChanged={invalidateExtraction}
      />
      {extractionError !== null ? (
        <StatusBanner tone="danger">
          {extractionError} You can still enter the values manually; the PDF stays
          attached.
        </StatusBanner>
      ) : null}
      {extractionInFlight ? (
        <StatusBanner tone="info">
          <span role="status">
            {extractionProgressText(extractionRun?.Status ?? "queued")} You can keep
            entering values while it runs.
          </span>
        </StatusBanner>
      ) : null}
      {extractionStale && !extractionInFlight ? (
        <StatusBanner tone="info">
          {extractionErrorMessage(extractionRun ?? { Status: "stale" })}
        </StatusBanner>
      ) : null}
      {!extractionStale && extractionRun?.Status === "failed" ? (
        <StatusBanner tone="danger">
          {extractionErrorMessage(extractionRun)} The PDF stays attached and the manual
          entry path is unchanged.
        </StatusBanner>
      ) : null}
      {showExtractionPanel && extractionRun?.Result != null ? (
        <PurchaseOrderExtractionPanelView
          result={extractionRun.Result}
          header={state.header}
          lines={state.lines}
          currency={activeCurrency}
          selection={extractionResolvedSelection}
          onToggle={toggleExtraction}
          onApply={() => void applyExtraction()}
          applying={extractionApplying}
          onDiscard={() => {
            setDismissedExtractionId(extractionRun.ID);
            setExtractionChoices({});
            rebaseExtractionTracker();
          }}
          applied={extractionApplied}
          plan={extractionPlan}
          choices={extractionChoices}
          onChoose={chooseExtraction}
        />
      ) : null}
      <PurchaseOrderEditorView
        state={state}
        dispatch={dispatch}
        currency={activeCurrency}
        currencyOptions={currencyOptions}
        uomOptions={uomOptions}
        incotermOptions={incotermOptions}
        onLinkQuotation={onLinkQuotation}
        onCurrencyChange={changeCurrency}
        onSave={() => void save(state.version, false)}
        onCancel={onCancel}
        onFinalSave={() => void startReview()}
        finalSaving={reviewLoading}
        mode={correcting ? "correction" : "draft"}
        correctionReason={reason}
        onCorrectionReasonChange={setReason}
        confirmationNumber={order.ConfirmationNumber ?? ""}
      />
      {conflictPrompt}
    </div>
  );
}
