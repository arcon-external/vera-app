import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
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
import type { ReconciliationHistoryInput } from "../poReconciliationHistory";
import { PurchaseOrderDocumentPanel } from "./PurchaseOrderDocumentPanel";
import { PurchaseOrderHistoryPanel } from "./PurchaseOrderHistoryPanel";
import {
  SavedReconciliationPanel,
  SavedReconciliationTrigger,
  useSavedReconciliation,
  type ReconciliationHistoryTransport,
  type SavedReconciliationState,
} from "./PurchaseOrderReconciliationHistory";
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
   * source-link review; "link" (#497) is the Staff draft link. The same matching
   * and choice rules apply; only the wording and the link/tax controls differ.
   */
  mode?: "finalize" | "repair" | "link";
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
  /**
   * #497: the PPN rate the link's chosen tax basis would produce. The link's
   * totals must show the number the server will recompute, not the draft's old
   * rate. Finalize/repair leave it unset.
   */
  ppnBpsOverride?: number;
  /** #497: the explicit currency-mismatch remedy shown in link mode. */
  currencyRemedy?: string;
  /** The link write is in flight: the continue action is disabled. */
  busy?: boolean;
  /** A required choice (the tax basis) is still unmade. */
  blocked?: boolean;
  /**
   * #497: content rendered after the currency banner and before the differences.
   * The link's tax decision lives here so there is one page header, not two.
   */
  beforeDifferences?: ReactNode;
  /**
   * U2/N4: receipt provenance from the recorded reconciliation session.
   * hasSession is true only for an authoritative persisted session
   * (positive stored ID, even when empty, tax-only or all-unresolved, and
   * including the session just created by the first save). The review is
   * then a read-only receipt: resolved choices keep their provenance,
   * unreviewed differences show needs-review provenance with a Change
   * route, and matches/acks render without editors. Identity-only local
   * (ID 0) and no-session drafts pass false and keep the legacy choice
   * workbench with no invented history.
   */
  receiptResolvedKeys?: string[];
  receiptAckedMissing?: string[];
  receiptAckedExtra?: number[];
  hasSession?: boolean;
  /** U2: Change links return focus to the relevant form or pane field. */
  onEditDifference?: (key: string, label: string) => void;
  onEditLine?: (lineNumber: number) => void;
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
  ppnBpsOverride,
  currencyRemedy,
  busy = false,
  blocked = false,
  beforeDifferences,
  receiptResolvedKeys = [],
  receiptAckedMissing = [],
  receiptAckedExtra = [],
  hasSession = false,
  onEditDifference,
  onEditLine,
}: PurchaseOrderDifferenceReviewViewProps) {
  const isReceipt = hasSession === true;
  const resolvedSet = new Set(receiptResolvedKeys);
  const ackedMissingSet = new Set(receiptAckedMissing);
  const ackedExtraSet = new Set(receiptAckedExtra);
  // N4/U2: receipt mode is keyed on authoritative persisted-session
  // provenance, not on recorded comparison choices. The caller passes
  // hasSession only for a stored session (positive persisted ID, including
  // after the first save): an empty, tax-only or all-unresolved persisted
  // session is still a recorded session, so every difference, match and ack
  // renders read-only with a Change route back. Genuinely local
  // identity-only (ID 0) and no-session drafts keep the legacy choice
  // workbench: a never-reviewed draft has no recorded provenance, so the
  // workbench is its first decision point, not a second one, and invents no
  // audit history.
  const showReceipt = isReceipt;
  const repairing = mode === "repair";
  const linking = mode === "link";
  const currencyMismatch = source.Currency !== state.currency;
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
      {linking ? null : (
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
      )}

      {currencyMismatch ? (
        <StatusBanner tone="danger">
          <span className="block font-medium">
            This purchase order is in {state.currency.toUpperCase()}; quotation{" "}
            {quoteNumber} revision {source.RevisionNumber} is in{" "}
            {source.Currency.toUpperCase()}.
          </span>
          <span className="mt-1 block">
            {currencyRemedy ??
              "Amounts in different currencies are never compared or converted."}
          </span>
        </StatusBanner>
      ) : (
        <StatusBanner tone="info">
          The purchase order is kept in {currency.code}, the currency of quotation{" "}
          {quoteNumber} revision {source.RevisionNumber}. Amounts are compared only
          inside that currency; Vera does not convert between currencies.
        </StatusBanner>
      )}

      {beforeDifferences}

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
              // U2/R2-U2: in receipt mode EVERY difference is read-only, not
              // a second decision workbench. A recorded choice shows its
              // provenance; an unreviewed one shows needs-review provenance
              // with a Change route back to the live pane. Either way there
              // are no Use buttons; readiness stays blocked for undecided
              // rows through the evaluation blockers.
              if (showReceipt) {
                const recorded =
                  resolvedSet.has(difference.key) &&
                  (choice === "po" || choice === "quotation");
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
                      <p className="text-caption text-ink-500">
                        {recorded
                          ? choice === "po"
                            ? "PO entry kept in the reconciliation pane."
                            : "Quotation value used from the reconciliation pane."
                          : "Needs review in the reconciliation pane."}
                      </p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        size="sm"
                        aria-label={`Change ${difference.label}`}
                        onClick={() => onEditDifference?.(difference.key, difference.label)}
                      >
                        Change
                      </Button>
                    </div>
                  </div>
                );
              }
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
        {/* R2-U2: the receipt sentence is written whole under showReceipt so
            the legacy choice tail can never append to receipt copy. */}
        <p className="mt-3 text-caption text-ink-500">
          {showReceipt ? (
            <>
              These are the choices recorded in the reconciliation pane.
              Values that still need review appear below without choices;
              resolve them in the pane.{" "}
              {repairing
                ? "Back without repairing leaves the saved purchase order and its source link unchanged."
                : linking
                  ? "Back without linking leaves the draft unlinked and its entered values unchanged."
                  : "Back to entry leaves the purchase order as entered."}
            </>
          ) : (
            <>
              Choose a side for each difference. Use Quotation takes the
              quotation&apos;s value when you continue; Use PO entry keeps
              what you entered.{" "}
              {repairing
                ? "Back without repairing leaves the saved purchase order and its source link unchanged."
                : linking
                  ? "Back without linking leaves the draft unlinked and its entered values unchanged."
                  : "Back to entry leaves the purchase order as entered."}
            </>
          )}
        </p>
      </Card>

      <Card className="p-4">
        <SectionHeading
          title="Line matches"
          hint="Matched by reference, not by position"
        />
        <p className="mt-2 text-caption text-ink-500">
          {showReceipt
            ? "Matches are recorded in the reconciliation pane; Change returns to the entry line."
            : "Every line keeps its customer line number. Correct any match that follows a different quotation line; a corrected match is saved on the purchase order line."}
        </p>
        <div className="mt-3 grid gap-2">
          {state.lines.map((line, index) => {
            const lineNumber = index + 1;
            const current = matchedByLineKey.get(line.key);
            const suggestion = comparison.suggestions.find(
              (candidate) => candidate.lineNumber === lineNumber
            );
            // U2: in receipt mode the match is recorded evidence, not an
            // editor. Change returns to the entry line's match control.
            if (showReceipt) {
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
                        ? "Currently extra (no quotation match) · recorded in the pane."
                        : `Currently quotation line ${current.LineLabel} · recorded in the pane.`}
                    </p>
                  </div>
                  <div className="flex flex-wrap items-end justify-end gap-2">
                    <Button
                      size="sm"
                      aria-label={`Change match for line ${lineNumber}`}
                      onClick={() => onEditLine?.(lineNumber)}
                    >
                      Change
                    </Button>
                  </div>
                </div>
              );
            }
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
            {comparison.missing.map((line) => {
              const acked = showReceipt && ackedMissingSet.has(line.LineLabel);
              if (acked) {
                return (
                  <p key={line.LineLabel} className="flex items-start gap-2 text-small text-ink-700">
                    <span aria-hidden="true" className="mt-0.5 inline-flex h-4 w-4 items-center justify-center rounded border border-rule bg-paper-subtle text-caption text-ink-500">✓</span>
                    <span>
                      Quotation line {line.LineLabel} — {line.Description || "(no description)"} · omission acknowledged in the pane.
                    </span>
                  </p>
                );
              }
              if (showReceipt) {
                return (
                  <p key={line.LineLabel} className="flex items-start gap-2 text-small text-ink-700">
                    <span>
                      Quotation line {line.LineLabel} — {line.Description || "(no description)"} · needs acknowledgement in the reconciliation pane. Return to entry and resolve it there.
                    </span>
                  </p>
                );
              }
              return (
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
              );
            })}
          </fieldset>
        ) : null}

        {comparison.extraLineNumbers.length > 0 ? (
          <fieldset className="mt-4 grid gap-2 border-t border-rule pt-3">
            <legend className="text-small font-medium text-ink-700">
              On this purchase order, not on the quotation
            </legend>
            {comparison.extraLineNumbers.map((lineNumber) => {
              const line = state.lines[lineNumber - 1];
              const acked = showReceipt && ackedExtraSet.has(lineNumber);
              if (acked) {
                return (
                  <p key={lineNumber} className="flex items-start gap-2 text-small text-ink-700">
                    <span aria-hidden="true" className="mt-0.5 inline-flex h-4 w-4 items-center justify-center rounded border border-rule bg-paper-subtle text-caption text-ink-500">✓</span>
                    <span>
                      Line {lineNumber} — {line.description || "(no description)"} · acknowledged as extra in the pane.
                    </span>
                  </p>
                );
              }
              if (showReceipt) {
                return (
                  <p key={lineNumber} className="flex items-start gap-2 text-small text-ink-700">
                    <span>
                      Line {lineNumber} — {line.description || "(no description)"} · needs acknowledgement in the reconciliation pane. Return to entry and resolve it there.
                    </span>
                  </p>
                );
              }
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
        ppnBps={ppnBpsOverride ?? state.ppnBps}
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
        {repairing || linking ? null : documentState === "attached" ? (
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
          <span className="block font-medium">
            {linking ? "Resolve before linking:" : "Resolve before saving:"}
          </span>
          <ul className="mt-1 list-disc pl-5">
            {evaluation.blockers.map((blocker) => (
              <li key={blocker}>{blocker}</li>
            ))}
          </ul>
        </StatusBanner>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
        <Button
          aria-label={
            repairing
              ? "Back without repairing"
              : linking
                ? "Back without linking"
                : "Back to entry"
          }
          onClick={onBack}
        >
          {repairing
            ? "Back without repairing"
            : linking
              ? "Back without linking"
              : "Back to entry"}
        </Button>
        <Button
          variant="primary"
          aria-label={
            repairing
              ? "Continue to repair"
              : linking
                ? "Link source"
                : "Continue final save"
          }
          disabled={!evaluation.canContinue || busy || blocked}
          onClick={onContinue}
        >
          {repairing
            ? "Continue to repair"
            : linking
              ? busy
                ? "Linking…"
                : "Link source"
              : "Continue"}
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
  /**
   * #518: the saved reconciliation generations. Omitted in the app so the
   * saved view performs the read; supplied by SSR and tests.
   */
  reconciliationInitial?: ReconciliationHistoryInput | null;
  reconciliationInitialError?: string | null;
  /**
   * Solves the snapshot's one unrenderable state: the harness can capture the
   * history-selected layout only if the generation is chosen up front. The app
   * never sets it and always opens on the current session.
   */
  reconciliationInitialSelection?: number | null;
  /** The read seam; tests substitute a fake, the app uses Wails. */
  reconciliationTransport?: ReconciliationHistoryTransport;
  /**
   * #518: the signed-in actor, so an item's resolution can name the person
   * only when this view knows that person. An unknown id renders "User #id".
   */
  actorId?: number;
  actorName?: string;
  /**
   * #518: refetches the record after a stale reconciliation read. The saved
   * view cannot reload the purchase order itself.
   */
  onReload?: () => void;
  /**
   * #518: how the saved record was re-entered from the source-repair flow.
   * A successful repair focuses the new generation and announces it; a cancel
   * returns focus to the repair action. The parent clears it after one run.
   */
  repairReturn?: RepairReturn | null;
  onRepairReturnHandled?: () => void;
}

/** What just happened in the repair flow, for the saved record's focus. */
export type RepairReturn =
  | { kind: "repaired"; revision: number }
  | { kind: "cancelled" };

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
  reconciliationInitial,
  reconciliationInitialError = null,
  reconciliationInitialSelection,
  reconciliationTransport,
  actorId,
  actorName,
  onReload,
  repairReturn = null,
  onRepairReturnHandled,
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
  // #518: one read feeds the >=1024 rail, the <1024 trigger and the sheet, so
  // the three cannot disagree about the current generation or the filter.
  const reconciliation = useSavedReconciliation({
    orderId: order.ID,
    orderVersion: current.Version,
    initial: reconciliationInitial,
    initialError: reconciliationInitialError,
    initialSelection: reconciliationInitialSelection,
    transport: reconciliationTransport,
  });
  const [sheetOpen, setSheetOpen] = useState(false);
  const savedTriggerRef = useRef<HTMLButtonElement | null>(null);
  const repairActionRef = useRef<HTMLButtonElement | null>(null);
  const handledRepairReturn = useRef<RepairReturn | null>(null);
  const actorNameFor = (id: number): string | null =>
    actorId !== undefined &&
    id === actorId &&
    actorName !== undefined &&
    actorName !== ""
      ? actorName
      : null;
  const closeSheet = () => {
    setSheetOpen(false);
    savedTriggerRef.current?.focus();
  };
  // A manual open lands on the sheet heading; Close/Escape returns to the
  // trigger, never the body.
  useEffect(() => {
    if (!sheetOpen) return;
    document.getElementById("saved-sheet-pane-heading")?.focus();
  }, [sheetOpen]);
  // #518: returning from the repair flow is one focus move. A successful
  // repair lands on the new current generation and says so; a cancel returns
  // to the action that opened the repair. The parent clears the signal after
  // this run, and the ref guards a re-render before it does.
  useEffect(() => {
    if (repairReturn === null || handledRepairReturn.current === repairReturn) return;
    handledRepairReturn.current = repairReturn;
    if (repairReturn.kind === "repaired") {
      const small =
        typeof window !== "undefined" &&
        typeof window.matchMedia === "function" &&
        window.matchMedia("(max-width: 1023px)").matches;
      // The rail heading only exists visually from 1024; below that the saved
      // surface's own entry control is the honest focus target.
      const heading = small ? null : document.getElementById("saved-rail-pane-heading");
      (heading ?? savedTriggerRef.current)?.focus();
      reconciliation.announce(
        `Source repaired. Showing saved session revision ${repairReturn.revision}.`
      );
    } else {
      repairActionRef.current?.focus();
    }
    onRepairReturnHandled?.();
  }, [repairReturn, onRepairReturnHandled, reconciliation.announce]);

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
      <div className="hidden lg:block">
        <a
          href="#saved-rail-pane-heading"
          className="sr-only focus-visible:not-sr-only focus-visible:inline-flex focus-visible:rounded-lg focus-visible:bg-ink-900 focus-visible:px-4 focus-visible:py-2.5 focus-visible:text-small focus-visible:font-medium focus-visible:text-white"
        >
          Skip to reconciliation
        </a>
      </div>
      <SavedReconciliationTrigger
        ref={savedTriggerRef}
        status={reconciliation.status}
        resolvedCount={reconciliation.counts.resolved}
        open={sheetOpen}
        onOpen={() => setSheetOpen(true)}
      />
      {/* #518: the record keeps its single column below 1024; from 1024 the
          reconciliation history takes the rail column. */}
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px] lg:gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="@container flex min-w-0 flex-col gap-5" inert={sheetOpen ? true : undefined}>
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
                  ref={repairActionRef}
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

          {/*
            #518 layout B: below a 720px container the saved lines read as a
            stacked list; at 720 or wider the table returns. The query is on
            the record's main column (container), not the viewport, so the 1024
            rail never forces a sideways scroll.
          */}
          <Card className="overflow-hidden">
            <ul className="divide-y divide-rule @min-[720px]:hidden">
              {state.lines.map((line: PurchaseOrderLine, index) => (
                <li key={line.key} className="px-4 py-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-small font-medium text-ink-900">
                        {line.description || "—"}
                      </p>
                      <p className="mt-0.5 text-caption text-ink-500">
                        Line {index + 1} ·{" "}
                        {line.quotationItemNo === null
                          ? "Extra line"
                          : `Quotation ${line.quotationItemNo}${
                              line.quotationSubNo === null ? "" : `.${line.quotationSubNo}`
                            }`}
                      </p>
                    </div>
                    <span className="tnum shrink-0 text-small font-medium text-ink-900">
                      {formatMoney(currency, order.Items[index]?.NetMinor ?? 0)}
                    </span>
                  </div>
                  <dl className="mt-2 grid grid-cols-3 gap-2 text-caption">
                    <div className="min-w-0">
                      <dt className="text-ink-500">Quantity</dt>
                      <dd className="tnum truncate text-ink-700">{line.quantity}</dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-ink-500">Unit price</dt>
                      <dd className="tnum truncate text-ink-700">{line.unitPrice}</dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-ink-500">Discount</dt>
                      <dd className="tnum truncate text-ink-700">
                        {line.discountEnabled ? `${line.discountPercent}%` : "—"}
                      </dd>
                    </div>
                  </dl>
                </li>
              ))}
            </ul>
            <div className="hidden @min-[720px]:block">
              <table className="w-full text-left text-body">
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
            <Button aria-label="Back to archive" onClick={onBack}>
              Back to archive
            </Button>
          </div>
        </div>
        <aside
          aria-label="Reconciliation"
          className="hidden min-h-0 lg:sticky lg:top-[88px] lg:flex lg:max-h-[calc(100dvh-112px)] lg:flex-col"
        >
          <SavedReconciliationPanel
            idPrefix="saved-rail"
            state={reconciliation}
            order={current}
            history={history}
            currency={currency}
            actorNameFor={actorNameFor}
            onReload={onReload}
            takesFocus={!sheetOpen}
            fillHeight
          />
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
          className="fixed inset-x-0 bottom-0 top-[var(--shell-header-h,64px)] z-20 flex flex-col gap-3 overflow-y-auto border-t border-rule bg-paper px-5 pb-8 pt-4 lg:hidden"
        >
          <SavedReconciliationPanel
            idPrefix="saved-sheet"
            state={reconciliation}
            order={current}
            history={history}
            currency={currency}
            actorNameFor={actorNameFor}
            onReload={onReload}
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
        </div>
      ) : null}
      <div role="status" aria-live="polite" className="sr-only">
        {reconciliation.announcement}
      </div>
    </div>
  );
}
