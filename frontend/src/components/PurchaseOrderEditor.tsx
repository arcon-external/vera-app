import { useEffect, useReducer, useRef, useState } from "react";
import {
  CorrectPurchaseOrder,
  FinalizePurchaseOrder,
  GetPurchaseOrderExtraction,
  GetPurchaseOrderReview,
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
import { PurchaseOrderDocumentPanel } from "./PurchaseOrderDocumentPanel";
import { PurchaseOrderExtractionPanelView } from "./PurchaseOrderExtractionPanel";
import { PurchaseOrderConflictPrompt } from "./PurchaseOrderConflictPrompt";
import {
  PurchaseOrderDifferenceReviewView,
  PurchaseOrderFinalConfirmationView,
} from "./PurchaseOrderFinalize";
import { PurchaseOrderTotals } from "./PurchaseOrderTotals";
import { Select } from "./Select";
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
  uomOptions: CompanyUnitOption[];
  incotermOptions: { value: string; label: string }[];
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
  uomOptions: CompanyUnitOption[];
  incotermOptions: { value: string; label: string }[];
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
  uomOptions,
  incotermOptions,
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
  // The panel and the save guard read the same pure calculation, so the number
  // on screen is the number the payload will carry.
  const calculation = calculatePurchaseOrder(state, currency);
  const wholeDiscountPercent = parseDiscountPercent(state.header.discountPercent);
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading
            title="Purchase Order"
            hint={correcting ? `Saved record ${state.orderId} · correction` : `Draft ${state.orderId}`}
          />
          <h1 className="tnum text-display font-semibold text-ink-950">
            {state.quoteNumber}
          </h1>
          <p className="mt-1 text-body text-ink-500">
            {state.customerName} · revision {state.sourceRevision} ·{" "}
            {state.currency.toUpperCase()}
            {state.taxZone !== ""
              ? ` · ${state.taxZone}${
                  state.taxCategory !== "" ? ` / ${state.taxCategory}` : ""
                }`
              : ""}
          </p>
          <p className="mt-1 text-caption text-ink-500">
            {correcting
              ? `Editing version ${state.version}. The previous values are kept in the correction history.`
              : `Loaded at version ${state.version}.`}
          </p>
        </div>
        <Badge tone="accent">{correcting ? "Correction" : "Draft"}</Badge>
      </div>

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
          Customer, currency and tax basis are snapshots of the issued
          quotation revision and cannot be edited here.
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
                currency={currency}
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
              Adjustment ({currency.code})
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
        currency={currency}
        discountPercent={wholeDiscountPercent}
        ppnBps={state.ppnBps}
        adjustmentReason={state.header.adjustmentReason.trim()}
        printedTotals={state.printedTotals}
        sourceRevision={state.sourceRevision}
      />

      {state.error !== null ? (
        <StatusBanner tone="danger">{state.error}</StatusBanner>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
        <span className="text-caption text-ink-500">
          {dirty ? "Unsaved changes" : "All changes saved"} ·{" "}
          {correcting
            ? "Saving keeps the replaced values and reason in the correction history."
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
              disabled={state.saving || finalSaving}
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
  uomOptions,
  incotermOptions,
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
          const next = await GetPurchaseOrderExtraction(order.ID);
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
    if (run != null && run.Status === "succeeded" && run.Result != null) {
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
          currency
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
        const next = await GetPurchaseOrderExtraction(order.ID);
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
        const next = await StartPurchaseOrderExtraction(order.ID);
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

  const applyExtraction = () => {
    const run = extractionState?.Run;
    const active =
      activeDocumentRef.current !== undefined
        ? activeDocumentRef.current
        : extractionState?.Document ?? null;
    if (run == null || run.Result == null || extractionIsStale(run, active)) return;
    const plan = resolveExtractionConflicts(
      run.Result,
      state.header,
      state.lines,
      ensureExtractionTracker(),
      currency
    );
    const selection = resolvedExtractionSelection(
      plan,
      extractionChoices,
      extractionSelection
    );
    const applied = applyExtractionSelection(
      run.Result,
      selection,
      state.header,
      state.lines,
      state.nextKey,
      currency
    );
    if (applied.appliedFields === 0 && applied.appliedRows === 0) return;
    dispatch({
      type: "extractionApplied",
      header: applied.headerPatch,
      lines: applied.lines,
      nextKey: state.nextKey + applied.appliedRows,
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
        currency,
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

  const save = async (expectedVersion: number, overwrite: boolean) => {
    if (correcting) {
      const reasonProblem = correctionReasonError(reason);
      if (reasonProblem !== null) {
        dispatch({ type: "saveFailed", error: reasonProblem });
        return;
      }
    }
    const result = buildPurchaseOrderUpdate(state, currency);
    if (result.input === null) {
      dispatch({ type: "saveFailed", error: result.error ?? "The draft is incomplete." });
      return;
    }
    const payload = {
      ...result.input,
      ExpectedVersion: expectedVersion,
      Overwrite: overwrite,
    };
    dispatch({ type: "saveStarted" });
    try {
      const items = payload.Items.map(
        (item) => new backend.PurchaseOrderItemInput(item)
      );
      const answer = correcting
        ? await CorrectPurchaseOrder(
            new backend.PurchaseOrderCorrectionInput({
              ...payload,
              Reason: trimCorrectionReason(reason),
              Items: items,
            })
          )
        : await UpdatePurchaseOrderDraft(
            new backend.PurchaseOrderUpdateInput({ ...payload, Items: items })
          );
      const outcome = writeOutcome(answer);
      if (outcome.kind === "saved") {
        dispatch({ type: "saveSucceeded", order: outcome.order, currency });
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
    const result = buildPurchaseOrderUpdate(state, currency);
    if (result.input === null) {
      dispatch({ type: "saveFailed", error: result.error ?? "The draft is incomplete." });
      return;
    }
    setReviewLoading(true);
    try {
      const loaded = await GetPurchaseOrderReview(order.ID);
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
      currency,
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
        dispatch({ type: "saveSucceeded", order: outcome.order, currency });
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
        currency={currency}
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
        currency={currency}
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
    dismissedExtractionId !== extractionRun?.ID;
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
          currency
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
      <PurchaseOrderDocumentPanel
        purchaseOrderId={order.ID}
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
          currency={currency}
          selection={extractionResolvedSelection}
          onToggle={toggleExtraction}
          onApply={applyExtraction}
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
        currency={currency}
        uomOptions={uomOptions}
        incotermOptions={incotermOptions}
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
