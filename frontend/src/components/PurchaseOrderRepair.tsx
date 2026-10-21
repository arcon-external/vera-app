import { useReducer, useState } from "react";
import { RepairPurchaseOrderSource } from "@backend/backend/Quotation";
import { backend } from "@backend/models";
import { formatMoney, type CurrencyInfo } from "../format";
import { writeOutcome } from "../poCorrection";
import {
  evaluateReview,
  type POReviewChoice,
  type POReviewDecisions,
  type PurchaseOrderReviewDTO,
} from "../poFinalize";
import {
  buildSourceRepairInput,
  repairConflictWarning,
  repairReasonError,
  sourceRepairChoiceLabel,
} from "../poRepair";
import {
  initialPurchaseOrderEditorState,
  purchaseOrderEditorReducer,
  type PurchaseOrderDTO,
} from "../purchaseOrder";
import { PurchaseOrderDifferenceReviewView } from "./PurchaseOrderFinalize";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";

// Issue #475: the admin source-link repair.
//
// The saved record's own values are the starting point. The admin picks a new
// issued revision through the same three-step selector (repair mode), reviews
// every difference against it with the #471 rules (no choice preselected),
// corrects any line match, and must type the reason the history keeps. Only
// then does the repair write; success returns to the saved view, which shows
// the new source. Nothing here writes a confirmation number: the server keeps
// it for a same-customer repair and refuses a cross-customer move once one is
// assigned.

/** One readable side of the source change. */
function SourceSide({
  label,
  quoteNumber,
  revision,
  customerName,
}: {
  label: string;
  quoteNumber: string;
  revision: number;
  customerName: string;
}) {
  return (
    <div className="min-w-0">
      <p className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
        {label}
      </p>
      <p className="tnum text-small font-semibold text-ink-900">
        Quotation {quoteNumber} · revision {revision}
      </p>
      <p className="break-words text-small text-ink-700">{customerName}</p>
    </div>
  );
}

/** The old link, the new link and the rules this repair is bound by. */
export function PurchaseOrderSourceChange({
  review,
  confirmationNumber,
}: {
  review: PurchaseOrderReviewDTO;
  confirmationNumber: string | null;
}) {
  const customerChanged = review.Order.CustomerID !== review.Source.CustomerID;
  return (
    <Card className="p-4" data-source-repair>
      <SectionHeading title="Source link change" hint="Old link → new link" />
      <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] sm:items-center">
        <SourceSide
          label="Current source"
          quoteNumber={review.Order.QuoteNumber}
          revision={review.Order.SourceRevision}
          customerName={review.Order.CustomerName}
        />
        <span aria-hidden="true" className="hidden text-h2 text-ink-400 sm:block">
          →
        </span>
        <SourceSide
          label="New source"
          quoteNumber={review.Source.QuoteNumber}
          revision={review.Source.RevisionNumber}
          customerName={review.Source.CustomerName}
        />
      </div>
      <div className="mt-3 grid gap-1 text-small text-ink-700">
        <p>
          <span className="font-medium text-ink-900">Confirmation number: </span>
          {confirmationNumber === null || confirmationNumber === ""
            ? "Not assigned"
            : `${confirmationNumber} — kept unchanged by a same-customer repair.`}
        </p>
        {customerChanged ? (
          <p>
            <span className="font-medium text-ink-900">Customer change: </span>
            the record follows another customer's revision. The new customer name
            and billing address are taken from that revision; every other
            recorded detail stays as it is.
          </p>
        ) : null}
      </div>
      {customerChanged && confirmationNumber !== null && confirmationNumber !== "" ? (
        <div className="mt-3">
          <StatusBanner tone="danger">
            This record has an assigned confirmation number, which carries the
            current customer's code. Moving it to another customer is refused;
            choose a revision of the same customer.
          </StatusBanner>
        </div>
      ) : null}
    </Card>
  );
}

export interface PurchaseOrderRepairConfirmationViewProps {
  review: PurchaseOrderReviewDTO;
  evaluation: ReturnType<typeof evaluateReview>;
  currency: CurrencyInfo;
  reason: string;
  onReasonChange: (value: string) => void;
  onBack: () => void;
  onSave: () => void;
  saving: boolean;
  /** A named version conflict: the values must be re-read before retrying. */
  stale?: boolean;
  error: string | null;
}

/**
 * The last step before the repair writes: the old and new link, the values the
 * review settled, and the required reason. The Save button is a local decision
 * only; the server repeats every rule inside its transaction.
 */
export function PurchaseOrderRepairConfirmationView({
  review,
  evaluation,
  currency,
  reason,
  onReasonChange,
  onBack,
  onSave,
  saving,
  stale = false,
  error,
}: PurchaseOrderRepairConfirmationViewProps) {
  const reasonError = repairReasonError(reason);
  const confirmationNumber = review.Order.ConfirmationNumber ?? null;
  const customerChanged = review.Order.CustomerID !== review.Source.CustomerID;
  const customerLocked =
    customerChanged && confirmationNumber !== null && confirmationNumber !== "";
  const total = evaluation.totals === null ? null : evaluation.totals.grandTotalMinor;
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading title="Confirm the source repair" hint="Admin action" />
          <p className="mt-1 max-w-3xl text-body text-ink-500">
            This replaces the saved record's source link. The previous values,
            the old and new link and every choice below are kept in the
            correction history.
          </p>
        </div>
        <Badge tone="accent">Source repair</Badge>
      </div>

      <PurchaseOrderSourceChange review={review} confirmationNumber={confirmationNumber} />

      <Card className="p-4">
        <SectionHeading
          title="Values this repair stores"
          hint={`${evaluation.comparison.differences.length} ${
            evaluation.comparison.differences.length === 1 ? "choice" : "choices"
          }`}
        />
        <ul className="mt-2 grid gap-1 text-small text-ink-700">
          {evaluation.comparison.differences.map((difference) => (
            <li key={difference.key}>
              <span className="font-medium text-ink-900">{difference.label}: </span>
              {sourceRepairChoiceLabel(evaluation.decisions[difference.key] ?? "po")}
              {evaluation.decisions[difference.key] === "quotation"
                ? ` (${difference.sourceValue === "" ? "(empty)" : difference.sourceValue})`
                : ""}
            </li>
          ))}
          {evaluation.comparison.differences.length === 0 ? (
            <li>Every compared value matches the new revision.</li>
          ) : null}
        </ul>
        <p className="mt-2 text-small text-ink-700">
          <span className="font-medium text-ink-900">Calculated total: </span>
          <span className="tnum whitespace-nowrap">
            {total === null ? "—" : formatMoney(currency, total)}
          </span>
        </p>
      </Card>

      <Card className="p-4">
        <label className="grid gap-1.5">
          <span className="text-small font-medium text-ink-700">
            Reason for repairing the source link
          </span>
          <textarea
            aria-label="Reason for repairing the source link"
            className={`${fieldClass} min-h-24`}
            value={reason}
            maxLength={500}
            onChange={(event) => onReasonChange(event.target.value)}
          />
        </label>
        <p className="mt-1 text-caption text-ink-500">
          Required. The operation, who performed it, and the old and new source
          are shown in the correction history.
        </p>
        {reasonError !== null ? (
          <p role="alert" className="mt-1 text-caption text-danger-600">
            {reasonError}
          </p>
        ) : null}
      </Card>

      {error !== null ? <StatusBanner tone="danger">{error}</StatusBanner> : null}

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
        <Button aria-label="Back to the differences" onClick={onBack} disabled={saving}>
          Back
        </Button>
        <Button
          variant="primary"
          aria-label="Repair source link"
          disabled={saving || stale || reasonError !== null || customerLocked}
          onClick={onSave}
        >
          {saving ? "Repairing…" : "Repair source link"}
        </Button>
      </div>
    </div>
  );
}

export interface PurchaseOrderRepairPageProps {
  review: PurchaseOrderReviewDTO;
  currency: CurrencyInfo;
  /** Leaves without writing; the saved view still shows the old source. */
  onCancel: () => void;
  /** The server's record after a successful repair. */
  onRepaired: (order: PurchaseOrderDTO) => void;
  /** Reports unsaved choices/reason so the page guard can ask before leaving. */
  onDirtyChange?: (dirty: boolean) => void;
}

/**
 * The repair flow's container: the review, the reason and the versioned write.
 * A stale version is a named conflict, never an overwrite: the repair path is
 * the admin operation, and an ordinary overwrite cannot become one by racing.
 */
export function PurchaseOrderRepairPage({
  review,
  currency,
  onCancel,
  onRepaired,
  onDirtyChange,
}: PurchaseOrderRepairPageProps) {
  const [state, dispatch] = useReducer(
    purchaseOrderEditorReducer,
    review.Order,
    (order) => initialPurchaseOrderEditorState(order, currency)
  );
  const [view, setView] = useState<"review" | "confirm">("review");
  const [decisions, setDecisions] = useState<POReviewDecisions>({});
  const [acknowledgedMissing, setAcknowledgedMissing] = useState<string[]>([]);
  const [acknowledgedExtra, setAcknowledgedExtra] = useState<number[]>([]);
  const [reason, setReason] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A stale version cannot be retried with the same values: the record moved,
  // so the panel asks for a reload instead of offering an overwrite.
  const [stale, setStale] = useState(false);

  const evaluation = evaluateReview(
    state,
    review.Source,
    currency,
    decisions,
    acknowledgedMissing,
    acknowledgedExtra
  );

  const setDecision = (key: string, choice: POReviewChoice) => {
    setDecisions((current) => ({ ...current, [key]: choice }));
    onDirtyChange?.(true);
  };
  const toggleMissing = (label: string) => {
    setAcknowledgedMissing((current) =>
      current.includes(label)
        ? current.filter((value) => value !== label)
        : [...current, label]
    );
    onDirtyChange?.(true);
  };
  const toggleExtra = (lineNumber: number) => {
    setAcknowledgedExtra((current) =>
      current.includes(lineNumber)
        ? current.filter((value) => value !== lineNumber)
        : [...current, lineNumber]
    );
    onDirtyChange?.(true);
  };
  const changeReason = (value: string) => {
    setReason(value);
    onDirtyChange?.(true);
  };

  const save = async () => {
    const built = buildSourceRepairInput(
      review.Order,
      { ...review, Order: { ...review.Order, Version: state.version } },
      currency,
      decisions,
      acknowledgedMissing,
      acknowledgedExtra,
      reason
    );
    if (built.input === null) {
      setError(built.error ?? "The repair could not be prepared.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const result = await RepairPurchaseOrderSource(
        new backend.PurchaseOrderSourceRepairInput(built.input)
      );
      const outcome = writeOutcome(result);
      if (outcome.kind === "saved") {
        onDirtyChange?.(false);
        onRepaired(outcome.order);
        return;
      }
      if (outcome.kind === "conflict") {
        setStale(true);
        setError(repairConflictWarning(outcome.conflict));
        return;
      }
      setError("The repair was not applied. Reload the purchase order and try again.");
    } catch (failure) {
      setError(
        failure instanceof Error && failure.message.trim() !== ""
          ? failure.message
          : "The source repair failed. Try again."
      );
    } finally {
      setSaving(false);
    }
  };

  if (view === "confirm") {
    return (
      <PurchaseOrderRepairConfirmationView
        review={review}
        evaluation={evaluation}
        currency={currency}
        reason={reason}
        onReasonChange={changeReason}
        onBack={() => setView("review")}
        onSave={() => void save()}
        saving={saving}
        stale={stale}
        error={error}
      />
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <PurchaseOrderSourceChange
        review={review}
        confirmationNumber={review.Order.ConfirmationNumber ?? null}
      />
      <PurchaseOrderDifferenceReviewView
        state={state}
        source={review.Source}
        currency={currency}
        quoteNumber={review.Source.QuoteNumber}
        mode="repair"
        documentState={review.DocumentState === "attached" ? "attached" : "none"}
        decisions={decisions}
        acknowledgedMissing={acknowledgedMissing}
        acknowledgedExtra={acknowledgedExtra}
        onDecision={setDecision}
        onToggleMissing={toggleMissing}
        onToggleExtra={toggleExtra}
        onMatchChange={(lineKey, source) =>
          dispatch({
            type: "lineChanged",
            key: lineKey,
            patch: {
              quotationItemNo: source === null ? null : source.QuotationItemNo,
              quotationSubNo: source === null ? null : (source.QuotationSubNo ?? null),
            },
          })
        }
        onBack={onCancel}
        onContinue={() => {
          if (!evaluation.canContinue) return;
          setView("confirm");
        }}
      />
      <p className="text-caption text-ink-500">
        The reason is required on the next step; nothing is written yet.
      </p>
    </div>
  );
}
