import { useEffect, useState } from "react";
import {
  AssignPurchaseOrderConfirmationNumber,
  GetPurchaseOrderHistory,
} from "@backend/backend/Quotation";
import { formatMoney, type CurrencyInfo } from "../format";
import type { PurchaseOrderHistoryEntry } from "../poCorrection";
import {
  evaluateReview,
  sourceLineLabel,
  type POReviewChoice,
  type POReviewDecisions,
  type PurchaseOrderReviewDTO,
  type PurchaseOrderSourceDTO,
  type PurchaseOrderSourceLineDTO,
} from "../poFinalize";
import {
  canAssignConfirmationNumber,
  confirmationNumberLabel,
  confirmationNumberOf,
  initialPurchaseOrderEditorState,
  MAX_EXACT_MINOR,
  parseDiscountPercent,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorState,
  type PurchaseOrderLine,
  type PurchaseOrderUpdateInput,
} from "../purchaseOrder";
import type {
  PurchaseOrderDocumentState,
  PurchaseOrderDocumentStateDTO,
} from "../purchaseOrderDocument";
import { sourceReferenceLabel, sourceRepairActionLabel } from "../poRepair";
import { PurchaseOrderDocumentPanel } from "./PurchaseOrderDocumentPanel";
import { PurchaseOrderHistoryPanel } from "./PurchaseOrderHistoryPanel";
import { PurchaseOrderTotals } from "./PurchaseOrderTotals";
import { Select } from "./Select";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

/** The server's message when it has one, so the panel names the real refusal. */
function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== "") {
    return error.message;
  }
  if (typeof error === "string" && error.trim() !== "") {
    return error;
  }
  return "The confirmation number could not be assigned.";
}

// Issue #471: the final-save review. Three assembled views live here:
//
//   1. the difference review, where every differing value gets an explicit
//      Use PO entry / Use Quotation choice (none preselected), missing and
//      extra lines are acknowledged, matches can be corrected, and the totals
//      are recalculated after every choice;
//   2. the final confirmation, which asks for the deliberate no-PDF
//      acknowledgement only when no attachment exists; an attached save needs
//      no checkbox and never records an absence (#472);
//   3. the saved record, shown read-only until the correction workflow (#473)
//      ships.
//
// The copy distinguishes finalized data readiness from business approval: a
// customer PO is already accepted commercially before it is entered, so saving
// here never claims an approval.
//
// The saved totals are the purchase order's own. The quotation revision's
// printed figures are shown as document evidence and never as a target: an
// accepted price difference or an extra line is explained by the review's
// explicit choices and acknowledgements, and the optional signed adjustment
// exists to reconcile what the customer's PO document actually printed.

export interface PurchaseOrderDifferenceReviewViewProps {
  state: PurchaseOrderEditorState;
  source: PurchaseOrderSourceDTO;
  currency: CurrencyInfo;
  quoteNumber: string;
  /**
   * "finalize" is the draft -> saved review; "repair" (#475) is the admin
   * source-link review. The same matching and choice rules apply; only the
   * wording and the no-PDF finalization note differ.
   */
  mode?: "finalize" | "repair";
  /** "none" or "attached"; the final confirmation follows this branch. */
  documentState: PurchaseOrderDocumentState;
  decisions: POReviewDecisions;
  acknowledgedMissing: string[];
  acknowledgedExtra: number[];
  onDecision: (key: string, choice: POReviewChoice) => void;
  onToggleMissing: (label: string) => void;
  onToggleExtra: (lineNumber: number) => void;
  onMatchChange: (lineKey: number, source: PurchaseOrderSourceLineDTO | null) => void;
  onBack: () => void;
  onContinue: () => void;
}

function differenceValue(value: string): string {
  return value === "" ? "(empty)" : value;
}

export function PurchaseOrderDifferenceReviewView({
  state,
  source,
  currency,
  quoteNumber,
  mode = "finalize",
  documentState,
  decisions,
  acknowledgedMissing,
  acknowledgedExtra,
  onDecision,
  onToggleMissing,
  onToggleExtra,
  onMatchChange,
  onBack,
  onContinue,
}: PurchaseOrderDifferenceReviewViewProps) {
  const repairing = mode === "repair";
  const evaluation = evaluateReview(
    state,
    source,
    currency,
    decisions,
    acknowledgedMissing,
    acknowledgedExtra
  );
  const comparison = evaluation.comparison;
  const matchedByLineKey = new Map<number, PurchaseOrderSourceLineDTO>();
  for (const pair of comparison.matched) {
    matchedByLineKey.set(pair.line.key, pair.source);
  }
  const sourceOptions = [
    { value: "", label: "Extra line (no quotation match)" },
    ...source.Lines.map((line) => ({
      value: line.LineLabel,
      label: sourceLineLabel(line),
    })),
  ];

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading
            title={
              repairing
                ? "Review differences against the new source"
                : "Review differences before saving"
            }
            hint={`${quoteNumber} · revision ${source.RevisionNumber}`}
          />
          <p className="mt-1 max-w-3xl text-body text-ink-500">
            {repairing
              ? "Repairing the link moves this saved purchase order to another issued revision. Its current values are the starting point; every value that differs needs an explicit choice before the repair can be saved."
              : "Finalizing records this purchase order's data so later documents can use it. It is not a business approval — the customer's order was already accepted before entry."}
          </p>
        </div>
        <Badge tone="accent">{repairing ? "Source repair" : "Final save"}</Badge>
      </div>

      <StatusBanner tone="info">
        The purchase order is kept in {currency.code}, the currency of quotation{" "}
        {quoteNumber} revision {source.RevisionNumber}. Amounts are compared only
        inside that currency; Vera does not convert between currencies.
      </StatusBanner>

      <Card className="p-4">
        <SectionHeading
          title="Differing values"
          hint={`${comparison.differences.length} ${
            comparison.differences.length === 1 ? "difference" : "differences"
          }`}
        />
        {comparison.differences.length === 0 ? (
          <p className="mt-2 rounded-lg border border-dashed border-rule px-3 py-4 text-small text-ink-500">
            Every compared value matches the quotation revision.
          </p>
        ) : (
          <div className="mt-2 grid gap-3">
            {comparison.differences.map((difference) => {
              const choice = decisions[difference.key];
              return (
                <div
                  key={difference.key}
                  role="group"
                  aria-label={difference.label}
                  className="grid gap-3 rounded-lg border border-rule bg-paper-subtle/40 p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
                >
                  <div className="grid gap-1">
                    <p className="text-small font-medium text-ink-900">
                      {difference.label}
                    </p>
                    <p className="text-caption text-ink-600">
                      PO entry:{" "}
                      <span className="tnum whitespace-nowrap font-medium text-ink-900">
                        {differenceValue(difference.poValue)}
                      </span>{" "}
                      · Quotation:{" "}
                      <span className="tnum whitespace-nowrap font-medium text-ink-900">
                        {differenceValue(difference.sourceValue)}
                      </span>
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant={choice === "po" ? "primary" : "secondary"}
                      aria-pressed={choice === "po"}
                      aria-label={`Use PO entry for ${difference.label}`}
                      onClick={() => onDecision(difference.key, "po")}
                    >
                      Use PO entry
                    </Button>
                    <Button
                      size="sm"
                      variant={choice === "quotation" ? "primary" : "secondary"}
                      aria-pressed={choice === "quotation"}
                      aria-label={`Use Quotation for ${difference.label}`}
                      onClick={() => onDecision(difference.key, "quotation")}
                    >
                      Use Quotation
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
        <p className="mt-3 text-caption text-ink-500">
          No choice is selected for you. Use Quotation takes the quotation's
          value when you continue; Use PO entry keeps what you entered.{" "}
          {repairing
            ? "Back without repairing leaves the saved purchase order and its source link unchanged."
            : "Back to entry leaves the purchase order as entered."}
        </p>
      </Card>

      <Card className="p-4">
        <SectionHeading
          title="Line matches"
          hint="Matched by reference, not by position"
        />
        <p className="mt-2 text-caption text-ink-500">
          Every line keeps its customer line number. Correct any match that
          follows a different quotation line; a corrected match is saved on the
          purchase order line.
        </p>
        <div className="mt-3 grid gap-2">
          {state.lines.map((line, index) => {
            const lineNumber = index + 1;
            const current = matchedByLineKey.get(line.key);
            const suggestion = comparison.suggestions.find(
              (candidate) => candidate.lineNumber === lineNumber
            );
            return (
              <div
                key={line.key}
                className="grid gap-2 rounded-lg border border-rule p-3 sm:grid-cols-2 sm:items-end"
              >
                <div className="grid gap-1">
                  <p className="text-small font-medium text-ink-900">
                    Line {lineNumber} — {line.description || "(no description)"}
                  </p>
                  <p className="text-caption text-ink-500">
                    {current === undefined
                      ? "Currently extra (no quotation match)"
                      : `Currently quotation line ${current.LineLabel}`}
                  </p>
                  {suggestion !== undefined ? (
                    <p className="text-caption text-ink-500">
                      Suggested: quotation line {suggestion.source.LineLabel}.{" "}
                      <button
                        type="button"
                        className="font-medium text-accent-700 underline"
                        aria-label={`Use suggested match for line ${lineNumber}`}
                        onClick={() => onMatchChange(line.key, suggestion.source)}
                      >
                        Use suggestion
                      </button>
                    </p>
                  ) : null}
                </div>
                <div className="grid gap-1.5">
                  <span className="text-small font-medium text-ink-700">
                    Quotation match
                  </span>
                  <Select
                    ariaLabel={`Line ${lineNumber} quotation match`}
                    value={current?.LineLabel ?? ""}
                    options={sourceOptions}
                    onChange={(value) => {
                      const label = String(value);
                      const chosen =
                        source.Lines.find((candidate) => candidate.LineLabel === label) ?? null;
                      onMatchChange(line.key, chosen);
                    }}
                  />
                </div>
              </div>
            );
          })}
        </div>

        {comparison.missing.length > 0 ? (
          <fieldset className="mt-4 grid gap-2 border-t border-rule pt-3">
            <legend className="text-small font-medium text-ink-700">
              On the quotation, not on this purchase order
            </legend>
            {comparison.missing.map((line) => (
              <label
                key={line.LineLabel}
                className="flex items-start gap-2 text-small text-ink-700"
              >
                <input
                  type="checkbox"
                  className="mt-0.5 h-4 w-4 accent-accent-700"
                  aria-label={`Acknowledge omission of quotation line ${line.LineLabel}`}
                  checked={acknowledgedMissing.includes(line.LineLabel)}
                  onChange={() => onToggleMissing(line.LineLabel)}
                />
                <span>
                  Quotation line {line.LineLabel} — {line.Description || "(no description)"}
                </span>
              </label>
            ))}
          </fieldset>
        ) : null}

        {comparison.extraLineNumbers.length > 0 ? (
          <fieldset className="mt-4 grid gap-2 border-t border-rule pt-3">
            <legend className="text-small font-medium text-ink-700">
              On this purchase order, not on the quotation
            </legend>
            {comparison.extraLineNumbers.map((lineNumber) => {
              const line = state.lines[lineNumber - 1];
              return (
                <label
                  key={lineNumber}
                  className="flex items-start gap-2 text-small text-ink-700"
                >
                  <input
                    type="checkbox"
                    className="mt-0.5 h-4 w-4 accent-accent-700"
                    aria-label={`Acknowledge line ${lineNumber} as extra`}
                    checked={acknowledgedExtra.includes(lineNumber)}
                    onChange={() => onToggleExtra(lineNumber)}
                  />
                  <span>
                    Line {lineNumber} — {line.description || "(no description)"}
                  </span>
                </label>
              );
            })}
          </fieldset>
        ) : null}
      </Card>

      {comparison.duplicateLineNumbers.length > 0 ? (
        <StatusBanner tone="danger">
          Lines {comparison.duplicateLineNumbers.join(", ")} match the same
          quotation line. Correct one of the matches before saving.
        </StatusBanner>
      ) : null}

      <PurchaseOrderTotals
        totals={evaluation.totals}
        currency={currency}
        discountPercent={parseDiscountPercent(evaluation.applied.header.discountPercent)}
        ppnBps={state.ppnBps}
        adjustmentReason={evaluation.applied.header.adjustmentReason.trim()}
        printedTotals={{
          subtotalMinor: source.PrintedSubtotalMinor,
          discountMinor: source.PrintedDiscountMinor,
          ppnMinor: source.PrintedPpnMinor,
          grandTotalMinor: source.PrintedGrandTotalMinor,
        }}
        sourceRevision={source.RevisionNumber}
      />

      <Card className="p-4">
        <SectionHeading title="Totals check" hint="Reference only" />
        <div className="mt-2 grid gap-1 text-small text-ink-700">
          <p className="flex flex-wrap items-baseline justify-between gap-2">
            <span>Quotation printed total</span>
            <span className="tnum whitespace-nowrap font-medium text-ink-900">
              {formatMoney(currency, source.PrintedGrandTotalMinor)}
            </span>
          </p>
          <p className="flex flex-wrap items-baseline justify-between gap-2">
            <span>This purchase order</span>
            <span className="tnum whitespace-nowrap font-medium text-ink-900">
              {evaluation.totals === null
                ? "—"
                : formatMoney(currency, evaluation.totals.grandTotalMinor)}
            </span>
          </p>
          <p className="flex flex-wrap items-baseline justify-between gap-2">
            <span>Difference to the printed document</span>
            <span className="tnum whitespace-nowrap font-medium text-ink-900">
              {evaluation.printedDifferenceMinor === null
                ? "—"
                : formatMoney(currency, evaluation.printedDifferenceMinor)}
            </span>
          </p>
        </div>
        <p className="mt-2 text-caption text-ink-500">
          The saved total is calculated from this purchase order's own lines,
          row and whole-order discounts, PPN and optional adjustment. The
          quotation's printed total stays here as document evidence, not as a
          target: an accepted price difference or an extra line is explained by
          the choices above and the acknowledgements, and never needs an
          adjustment to reverse it. Record a signed adjustment with a reason
          only to reconcile what the customer's PO document actually printed.
        </p>
        {repairing ? null : documentState === "attached" ? (
          <p className="mt-1 text-caption text-ink-500">
            The customer PO PDF is attached and stays with this record. This
            save does not change it.
          </p>
        ) : (
          <p className="mt-1 text-caption text-ink-500">
            No customer PO PDF is attached. The final confirmation after this
            review records that deliberately.
          </p>
        )}
      </Card>

      {evaluation.blockers.length > 0 ? (
        <StatusBanner tone="danger">
          <span className="block font-medium">Resolve before saving:</span>
          <ul className="mt-1 list-disc pl-5">
            {evaluation.blockers.map((blocker) => (
              <li key={blocker}>{blocker}</li>
            ))}
          </ul>
        </StatusBanner>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
        <Button
          aria-label={repairing ? "Back without repairing" : "Back to entry"}
          onClick={onBack}
        >
          {repairing ? "Back without repairing" : "Back to entry"}
        </Button>
        <Button
          variant="primary"
          aria-label={repairing ? "Continue to repair" : "Continue final save"}
          disabled={!evaluation.canContinue}
          onClick={onContinue}
        >
          {repairing ? "Continue to repair" : "Continue"}
        </Button>
      </div>
    </div>
  );
}

export interface PurchaseOrderFinalConfirmationViewProps {
  review: PurchaseOrderReviewDTO;
  evaluation: ReturnType<typeof evaluateReview>;
  /**
   * The applied editor payload: the exact values Final save submits. It is
   * built from the applied review choices, never from the stored record, so
   * the confirmation cannot show a stale identifier the save will not store.
   */
  payload: PurchaseOrderUpdateInput;
  currency: CurrencyInfo;
  documentConfirmed: boolean;
  onToggleDocumentConfirmed: () => void;
  /** #474: whether this save should allocate the optional reference. */
  assignConfirmation: boolean;
  onToggleAssignConfirmation: () => void;
  onBack: () => void;
  onFinalSave: () => void;
  saving: boolean;
  error: string | null;
}

export function PurchaseOrderFinalConfirmationView({
  review,
  evaluation,
  payload,
  currency,
  documentConfirmed,
  onToggleDocumentConfirmed,
  assignConfirmation,
  onToggleAssignConfirmation,
  onBack,
  onFinalSave,
  saving,
  error,
}: PurchaseOrderFinalConfirmationViewProps) {
  const { Order: order, DocumentState: documentState } = review;
  const noDocument = documentState !== "attached";
  const ready = !saving && (!noDocument || documentConfirmed);
  return (
    <div className="flex flex-col gap-5">
      <div>
        <SectionHeading title="Final save" hint="One-way: a saved record is read-only" />
        <p className="mt-1 max-w-3xl text-body text-ink-500">
          Saving marks this purchase order as finalized entry data for later
          documents. It is not a business approval. The details below are the
          values this save will store.
        </p>
      </div>

      <Card className="p-4">
        <dl className="grid gap-2 text-small sm:grid-cols-2">
          <div>
            <dt className="text-caption text-ink-500">Customer PO number</dt>
            <dd className="tnum font-medium text-ink-900">{payload.PONumber}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Confirmation number</dt>
            <dd className="tnum font-medium text-ink-900">
              {assignConfirmation ? "Assigned when saved" : "Not assigned"}
            </dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Customer</dt>
            <dd className="font-medium text-ink-900">{order.CustomerName}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Quotation</dt>
            <dd className="tnum font-medium text-ink-900">
              {order.QuoteNumber} · revision {order.SourceRevision}
            </dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Customer PO date</dt>
            <dd className="tnum font-medium text-ink-900">{payload.PODate || "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Received date</dt>
            <dd className="tnum font-medium text-ink-900">{payload.ReceivedDate}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Requested delivery</dt>
            <dd className="tnum font-medium text-ink-900">
              {payload.RequestedDeliveryDate || "—"}
            </dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Payment terms</dt>
            <dd className="font-medium text-ink-900">{payload.PaymentTerms || "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Incoterms</dt>
            <dd className="font-medium text-ink-900">{payload.Incoterms || "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Lines</dt>
            <dd className="tnum font-medium text-ink-900">{payload.Items.length}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Whole-PO discount</dt>
            <dd className="tnum font-medium text-ink-900">
              {payload.DiscountPercent > 0 ? `${payload.DiscountPercent}%` : "None"}
            </dd>
          </div>
          {payload.AdjustmentMinor !== 0 ? (
            <div>
              <dt className="text-caption text-ink-500">Adjustment</dt>
              <dd className="tnum font-medium text-ink-900">
                {formatMoney(currency, payload.AdjustmentMinor)} ·{" "}
                {payload.AdjustmentReason}
              </dd>
            </div>
          ) : null}
          <div>
            <dt className="text-caption text-ink-500">Total</dt>
            <dd className="tnum font-medium text-ink-900">
              {evaluation.totals === null
                ? "—"
                : formatMoney(currency, evaluation.totals.grandTotalMinor)}
            </dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Currency</dt>
            <dd className="font-medium text-ink-900">{currency.code}</dd>
          </div>
        </dl>
      </Card>

      <Card className="border-accent-200 bg-accent-50/40 p-4">
        <SectionHeading title="Customer PO PDF" hint="Evidence for this record" />
        {noDocument ? (
          <label className="mt-2 flex items-start gap-2 text-small text-ink-800">
            <input
              type="checkbox"
              className="mt-0.5 h-4 w-4 accent-accent-700"
              aria-label="Confirm saving without the customer PDF"
              checked={documentConfirmed}
              onChange={onToggleDocumentConfirmed}
            />
            <span>
              I confirm this purchase order is saved without its customer PO PDF.
              You can attach one before saving; this confirmation records that
              the missing evidence is deliberate rather than silent.
            </span>
          </label>
        ) : (
          <p className="mt-2 text-small text-ink-700">
            The attached customer PO PDF stays with this record.
          </p>
        )}
      </Card>

      <Card className="p-4">
        <SectionHeading title="Confirmation number" hint="Optional" />
        <label className="mt-2 flex items-start gap-2 text-small text-ink-800">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4 accent-accent-700"
            aria-label="Assign confirmation number"
            checked={assignConfirmation}
            onChange={onToggleAssignConfirmation}
          />
          <span>
            Assign a confirmation number when this purchase order is saved.
          </span>
        </label>
        <p className="mt-2 text-caption text-ink-500">
          This number is a reference for manual submission to the customer
          system. Vera does not submit it, and this record is not marked as
          submitted. Leave the option unchecked when the customer does not
          require one: an unassigned record consumes no number and shows “Not
          assigned”.
        </p>
      </Card>

      {error !== null ? <StatusBanner tone="danger">{error}</StatusBanner> : null}

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
        <Button aria-label="Back to review" onClick={onBack} disabled={saving}>
          Back
        </Button>
        <Button
          variant="primary"
          aria-label="Final save"
          disabled={!ready}
          onClick={onFinalSave}
        >
          {saving ? "Saving…" : "Final save"}
        </Button>
      </div>
    </div>
  );
}

export interface PurchaseOrderSavedViewProps {
  order: PurchaseOrderDTO;
  currency: CurrencyInfo;
  onBack: () => void;
  /**
   * Opens the correction editor (#473). Omitted by a surface that only reads a
   * record (the SSR snapshot), so the action never exists without a handler.
   */
  onCorrect?: () => void;
  /**
   * #475: opens the source selector in repair mode. The action exists only for
   * an admin on a saved record; both facts are stated here so a staff view can
   * never render it by accident.
   */
  canRepairSource?: boolean;
  onRepairSource?: () => void;
  /**
   * Reports a later confirmation-number assignment so the page that owns the
   * record can keep showing it. The view also keeps its own copy, so it is
   * correct even when a parent does not provide this.
   */
  onAssigned?: (order: PurchaseOrderDTO) => void;
  /**
   * The server's document state when the parent already has it. Omitted in the
   * app so the panel fetches; supplied by the SSR snapshot so the attached and
   * empty branches are both visible without a browser effect.
   */
  documentInitialState?: PurchaseOrderDocumentStateDTO | null;
  onDocumentStateChange?: (state: PurchaseOrderDocumentStateDTO) => void;
  /** The history the parent already has; omitted in the app so it fetches. */
  historyInitial?: PurchaseOrderHistoryEntry[] | null;
  historyInitialError?: string | null;
}

/**
 * A saved record, read-only. This is the behaviour chosen for #471: #473 owns
 * corrections, so this view exposes no editable control and no save action. It
 * shows the stored values, the exact source link, the calculated totals and the
 * printed document figures side by side.
 */
export function PurchaseOrderSavedView({
  order,
  currency,
  onBack,
  onCorrect,
  canRepairSource = false,
  onRepairSource,
  onAssigned,
  documentInitialState,
  onDocumentStateChange,
  historyInitial,
  historyInitialError = null,
}: PurchaseOrderSavedViewProps) {
  // #474: a later assignment is the one write this read-only view performs.
  // It keeps the server's answer locally and reports it upward, so the number
  // appears whether or not the parent re-renders.
  const [assignedOrder, setAssignedOrder] = useState<PurchaseOrderDTO | null>(null);
  const [assigning, setAssigning] = useState(false);
  const [assignError, setAssignError] = useState<string | null>(null);
  const [copyStatus, setCopyStatus] = useState<string | null>(null);
  // #473: the retained correction snapshots. The parent may supply them (SSR),
  // otherwise this is the one read the saved view performs.
  const [history, setHistory] = useState<PurchaseOrderHistoryEntry[]>(
    historyInitial ?? []
  );
  const [historyLoading, setHistoryLoading] = useState(historyInitial == null);
  const [historyError, setHistoryError] = useState<string | null>(historyInitialError);
  // A later write (a confirmation assignment, or #475's source repair) makes
  // the record prop newer than the local assignment answer. Prefer whichever
  // carries the higher version so a repair cannot leave the old source on
  // screen behind a stale assigned copy.
  const current =
    assignedOrder !== null && assignedOrder.Version >= order.Version
      ? assignedOrder
      : order;
  const confirmationNumber = confirmationNumberOf(current);
  const canAssign = canAssignConfirmationNumber(current);
  const state = initialPurchaseOrderEditorState(order, currency);

  useEffect(() => {
    if (historyInitial !== undefined && historyInitial !== null) {
      setHistory(historyInitial);
      setHistoryLoading(false);
      setHistoryError(historyInitialError);
      return;
    }
    let cancelled = false;
    setHistoryLoading(true);
    GetPurchaseOrderHistory(order.ID)
      .then((entries) => {
        if (cancelled) return;
        setHistory(entries ?? []);
        setHistoryError(null);
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setHistory([]);
        setHistoryError(errorMessage(error));
      })
      .finally(() => {
        if (!cancelled) setHistoryLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // A correction changes the version, so reopening the same record refetches
    // rather than showing the history from before the write.
  }, [order.ID, order.Version]);
  // A saved record displays its stored figures. Recalculating would let a
  // later calculator change rewrite what an already-saved record says, which
  // is exactly what the stored calculation columns exist to prevent.
  let gross = 0n;
  let rowDiscount = 0n;
  for (const item of order.Items) {
    gross += BigInt(item.GrossMinor);
    rowDiscount += BigInt(item.DiscountMinor);
  }
  const safe = (value: bigint): number | null =>
    value >= 0n && value <= BigInt(MAX_EXACT_MINOR) ? Number(value) : null;
  const totals = {
    grossMinor: safe(gross),
    rowDiscountMinor: safe(rowDiscount),
    subtotalMinor: order.SubtotalMinor,
    discountMinor: order.DiscountMinor,
    afterDiscountMinor: order.SubtotalMinor - order.DiscountMinor,
    ppnMinor: order.PpnMinor,
    adjustmentMinor: order.AdjustmentMinor,
    grandTotalMinor: order.GrandTotalMinor,
  };

  const assignConfirmation = async () => {
    setAssigning(true);
    setAssignError(null);
    setCopyStatus(null);
    try {
      const assigned = await AssignPurchaseOrderConfirmationNumber(current.ID);
      setAssignedOrder(assigned);
      onAssigned?.(assigned);
    } catch (error) {
      setAssignError(errorMessage(error));
    } finally {
      setAssigning(false);
    }
  };

  const copyConfirmation = async () => {
    if (confirmationNumber === "") return;
    setCopyStatus(null);
    try {
      await navigator.clipboard.writeText(confirmationNumber);
      setCopyStatus("Confirmation number copied.");
    } catch {
      setCopyStatus(
        "The confirmation number could not be copied. Select it and copy it manually."
      );
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading title="Purchase Order" hint={`Saved record ${order.ID}`} />
          <h1 className="tnum text-display font-semibold text-ink-950">
            {order.PONumber}
          </h1>
          <p className="mt-1 text-body text-ink-500">
            {order.CustomerName} · {sourceReferenceLabel(current)} ·{" "}
            {order.Currency.toUpperCase()}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone="success">Saved</Badge>
          {current.SourceMissing ? (
            <Badge tone="danger">Source missing</Badge>
          ) : null}
          {/*
            #481: a correction needs the frozen source revision to compare
            against, and the server refuses one while the source is missing
            (po_source_missing). Offering the action would be a dead end; the
            admin's Link Quotation is the one repair path in this state.
          */}
          {onCorrect !== undefined && !current.SourceMissing ? (
            <Button size="sm" aria-label="Correct purchase order" onClick={onCorrect}>
              Correct
            </Button>
          ) : null}
          {canRepairSource && onRepairSource !== undefined ? (
            <Button
              size="sm"
              aria-label={sourceRepairActionLabel(current)}
              onClick={onRepairSource}
            >
              {sourceRepairActionLabel(current)}
            </Button>
          ) : null}
        </div>
      </div>

      <StatusBanner tone="info">
        Finalized entry data for later documents. This is not a business
        approval.
      </StatusBanner>
      {current.SourceMissing ? (
        <>
          <StatusBanner tone="danger">
            <span className="font-medium">Source missing. </span>
            The quotation this purchase order followed was permanently deleted.
            The purchase order, its recorded values and its customer PO PDF are
            kept. No new Delivery Order or Invoice can be created from this
            purchase order until the source is linked. This release does not
            create Delivery Orders or Invoices.
          </StatusBanner>
          <p className="text-caption text-ink-500">
            {canRepairSource
              ? "Link a quotation to repair the source; the values above stay the starting point and the difference review is repeated."
              : "An administrator can link a quotation to repair the source."}
          </p>
        </>
      ) : null}
      <StatusBanner tone="info">
        Corrections need a reason and keep the values they replace in the
        correction history. The customer PO PDF can still be attached, replaced
        or removed; the customer, quotation link, currency and confirmation
        number stay protected. An admin can repair a wrong source link; that
        repeats the difference review and is recorded with the old and new
        source.
      </StatusBanner>

      <Card className="p-4">
        <SectionHeading title="Confirmation number" hint="Customer-system reference" />
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <span
            className={
              confirmationNumber === ""
                ? "text-small text-ink-600"
                : "tnum whitespace-nowrap text-h2 font-semibold text-ink-950"
            }
          >
            {confirmationNumberLabel(current)}
          </span>
          {confirmationNumber !== "" ? (
            <Button
              size="sm"
              aria-label="Copy confirmation number"
              onClick={() => void copyConfirmation()}
            >
              Copy
            </Button>
          ) : canAssign ? (
            <Button
              size="sm"
              variant="primary"
              aria-label="Assign confirmation number"
              disabled={assigning}
              onClick={() => void assignConfirmation()}
            >
              {assigning ? "Assigning…" : "Assign confirmation number"}
            </Button>
          ) : null}
        </div>
        <p className="mt-2 text-caption text-ink-500">
          This number is a reference for manual submission to the customer
          system. Vera does not submit it, and this record is not marked as
          submitted. Once assigned, the number is fixed: it cannot be edited,
          cleared or regenerated.
        </p>
        {current.ConfirmationWarn ? (
          <div className="mt-2">
            <StatusBanner tone="info">
              <span className="font-medium">Confirmation sequence warning: </span>
              this customer's cycle has reached the 8,000 warning threshold.
              Numbers keep issuing; past 9,999 the sequence widens to five
              digits instead of wrapping.
            </StatusBanner>
          </div>
        ) : null}
        {copyStatus !== null ? (
          <p role="status" className="mt-2 text-caption text-ink-600">
            {copyStatus}
          </p>
        ) : null}
        {assignError !== null ? (
          <div className="mt-2">
            <StatusBanner tone="danger">{assignError}</StatusBanner>
          </div>
        ) : null}
      </Card>

      <Card className="p-4">
        <SectionHeading title="Recorded details" hint="Snapshots kept as entered" />
        <dl className="mt-2 grid gap-3 text-small sm:grid-cols-2">
          <div>
            <dt className="text-caption text-ink-500">Customer</dt>
            <dd className="text-ink-900">{order.CustomerName}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Customer PO date</dt>
            <dd className="tnum text-ink-900">{order.PODate ?? "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Received date</dt>
            <dd className="tnum text-ink-900">{order.ReceivedDate}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Requested delivery</dt>
            <dd className="tnum text-ink-900">{order.RequestedDeliveryDate ?? "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Payment terms</dt>
            <dd className="text-ink-900">{order.PaymentTerms || "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Incoterms</dt>
            <dd className="text-ink-900">{order.Incoterms || "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Billing address</dt>
            <dd className="text-ink-900">{order.BillingAddress || "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Delivery address</dt>
            <dd className="text-ink-900">{order.DeliveryAddress || "—"}</dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Contact</dt>
            <dd className="text-ink-900">
              {[order.ContactEmail, order.ContactPhone].filter((value) => value !== "").join(" · ") || "—"}
            </dd>
          </div>
          <div>
            <dt className="text-caption text-ink-500">Ship-to code</dt>
            <dd className="text-ink-900">{order.ShipToCode || "—"}</dd>
          </div>
        </dl>
      </Card>

      <Card className="overflow-hidden">
        <div className="overflow-x-auto scroll-thin">
          <table className="w-full min-w-[720px] text-left text-body">
            <caption className="sr-only">Saved purchase order lines</caption>
            <thead>
              <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                <th className="px-4 py-3 font-medium">Line</th>
                <th className="px-4 py-3 font-medium">Description</th>
                <th className="px-4 py-3 font-medium">Match</th>
                <th className="px-4 py-3 text-right font-medium">Quantity</th>
                <th className="px-4 py-3 text-right font-medium">Unit price</th>
                <th className="px-4 py-3 text-right font-medium">Discount</th>
                <th className="px-4 py-3 text-right font-medium">Line total</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-rule">
              {state.lines.map((line: PurchaseOrderLine, index) => (
                <tr key={line.key}>
                  <td className="tnum px-4 py-3 align-middle text-ink-700">
                    {index + 1}
                  </td>
                  <td className="px-4 py-3 align-middle text-ink-900">
                    {line.description || "—"}
                  </td>
                  <td className="px-4 py-3 align-middle text-ink-700">
                    {line.quotationItemNo === null
                      ? "Extra line"
                      : `Quotation ${line.quotationItemNo}${
                          line.quotationSubNo === null ? "" : `.${line.quotationSubNo}`
                        }`}
                  </td>
                  <td className="tnum px-4 py-3 text-right align-middle text-ink-700">
                    {line.quantity}
                  </td>
                  <td className="tnum whitespace-nowrap px-4 py-3 text-right align-middle text-ink-700">
                    {line.unitPrice}
                  </td>
                  <td className="tnum px-4 py-3 text-right align-middle text-ink-700">
                    {line.discountEnabled ? `${line.discountPercent}%` : "—"}
                  </td>
                  <td className="tnum whitespace-nowrap px-4 py-3 text-right align-middle text-ink-900">
                    {formatMoney(currency, order.Items[index]?.NetMinor ?? 0)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <PurchaseOrderTotals
        totals={totals}
        currency={currency}
        discountPercent={order.DiscountPercent}
        ppnBps={order.PpnBps}
        adjustmentReason={order.AdjustmentReason.trim()}
        printedTotals={state.printedTotals}
        sourceRevision={order.SourceRevision}
      />

      <PurchaseOrderDocumentPanel
        purchaseOrderId={order.ID}
        initialState={documentInitialState}
        absenceConfirmed={order.PdfAbsentConfirmed === true}
        onStateChange={onDocumentStateChange}
      />

      <PurchaseOrderHistoryPanel
        entries={history}
        currency={currency}
        loading={historyLoading}
        error={historyError}
      />

      <div className="border-t border-rule pt-4">
        <Button aria-label="Back to purchase orders" onClick={onBack}>
          Back to purchase orders
        </Button>
      </div>
    </div>
  );
}
