import { formatMoney, type CurrencyInfo } from "../format";
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
  calculatePurchaseOrder,
  initialPurchaseOrderEditorState,
  parseDiscountPercent,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorState,
  type PurchaseOrderLine,
} from "../purchaseOrder";
import { PurchaseOrderTotals } from "./PurchaseOrderTotals";
import { Select } from "./Select";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

// Issue #471: the final-save review. Three assembled views live here:
//
//   1. the difference review, where every differing value gets an explicit
//      Use PO entry / Use Quotation choice (none preselected), missing and
//      extra lines are acknowledged, matches can be corrected, and the totals
//      are recalculated after every choice;
//   2. the final confirmation, which is the deliberate no-PDF acknowledgement
//      while #472 has not shipped (the attached branch needs no checkbox);
//   3. the saved record, shown read-only until the correction workflow (#473)
//      ships.
//
// The copy distinguishes finalized data readiness from business approval: a
// customer PO is already accepted commercially before it is entered, so saving
// here never claims an approval.

export interface PurchaseOrderDifferenceReviewViewProps {
  state: PurchaseOrderEditorState;
  source: PurchaseOrderSourceDTO;
  currency: CurrencyInfo;
  quoteNumber: string;
  /** "none" today; #472 adds "attached" without changing this flow. */
  documentState: string;
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
            title="Review differences before saving"
            hint={`${quoteNumber} · revision ${source.RevisionNumber}`}
          />
          <p className="mt-1 max-w-3xl text-body text-ink-500">
            Finalizing records this purchase order's data so later documents can
            use it. It is not a business approval — the customer's order was
            already accepted before entry.
          </p>
        </div>
        <Badge tone="accent">Final save</Badge>
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
          No choice is selected for you. Choosing Use Quotation writes the
          quotation's value into this purchase order; choosing Use PO entry keeps
          what you entered.
        </p>
      </Card>

      {comparison.missing.length > 0 || comparison.extraLineNumbers.length > 0 ? (
        <Card className="p-4">
          <SectionHeading
            title="Lines that do not pair"
            hint="Reviewed explicitly, never inferred from position"
          />
          <div className="mt-3 grid gap-4">
            {comparison.missing.length > 0 ? (
              <fieldset className="grid gap-2">
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
              <fieldset className="grid gap-2">
                <legend className="text-small font-medium text-ink-700">
                  On this purchase order, not on the quotation
                </legend>
                {comparison.extraLineNumbers.map((lineNumber) => {
                  const line = state.lines[lineNumber - 1];
                  const suggestion = comparison.suggestions.find(
                    (candidate) => candidate.lineNumber === lineNumber
                  );
                  return (
                    <div
                      key={lineNumber}
                      className="grid gap-2 rounded-lg border border-rule p-3 sm:grid-cols-2"
                    >
                      <div className="grid gap-1">
                        <label className="flex items-start gap-2 text-small text-ink-700">
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
                        {suggestion !== undefined ? (
                          <p className="text-caption text-ink-500">
                            Suggested match: quotation line{" "}
                            {suggestion.source.LineLabel}.{" "}
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
                          value={matchedByLineKey.get(line.key)?.LineLabel ?? ""}
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
              </fieldset>
            ) : null}
          </div>
        </Card>
      ) : null}

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
        printedTotals={state.printedTotals}
        sourceRevision={source.RevisionNumber}
      />

      <Card className="p-4">
        <SectionHeading title="Printed total check" hint="Explained, not overwritten" />
        <div className="mt-2 grid gap-1 text-small text-ink-700">
          <p className="flex flex-wrap items-baseline justify-between gap-2">
            <span>Quotation printed total</span>
            <span className="tnum whitespace-nowrap font-medium text-ink-900">
              {formatMoney(currency, source.PrintedGrandTotalMinor)}
            </span>
          </p>
          <p className="flex flex-wrap items-baseline justify-between gap-2">
            <span>Remaining difference</span>
            <span className="tnum whitespace-nowrap font-medium text-ink-900">
              {evaluation.differenceMinor === null
                ? "—"
                : formatMoney(currency, evaluation.differenceMinor)}
            </span>
          </p>
          <p className="flex flex-wrap items-baseline justify-between gap-2">
            <span>Adjustment that covers it</span>
            <span className="tnum whitespace-nowrap font-medium text-ink-900">
              {evaluation.requiredAdjustmentMinor === null
                ? "—"
                : formatMoney(currency, evaluation.requiredAdjustmentMinor)}
            </span>
          </p>
        </div>
        <p className="mt-2 text-caption text-ink-500">
          {evaluation.covered
            ? "The signed adjustment explains the printed total, so the saved order reproduces the customer document."
            : "Correct the wrong fields, or return to entry and record the signed adjustment above with a reason. An adjustment never replaces correcting a wrongly entered value."}
        </p>
        {documentState === "none" ? (
          <p className="mt-1 text-caption text-ink-500">
            No customer PO PDF is attached. The final confirmation after this
            review records that deliberately.
          </p>
        ) : null}
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
        <Button aria-label="Back to entry" onClick={onBack}>
          Back to entry
        </Button>
        <Button
          variant="primary"
          aria-label="Continue final save"
          disabled={!evaluation.canContinue}
          onClick={onContinue}
        >
          Continue
        </Button>
      </div>
    </div>
  );
}

export interface PurchaseOrderFinalConfirmationViewProps {
  review: PurchaseOrderReviewDTO;
  evaluation: ReturnType<typeof evaluateReview>;
  currency: CurrencyInfo;
  documentConfirmed: boolean;
  onToggleDocumentConfirmed: () => void;
  onBack: () => void;
  onFinalSave: () => void;
  saving: boolean;
  error: string | null;
}

export function PurchaseOrderFinalConfirmationView({
  review,
  evaluation,
  currency,
  documentConfirmed,
  onToggleDocumentConfirmed,
  onBack,
  onFinalSave,
  saving,
  error,
}: PurchaseOrderFinalConfirmationViewProps) {
  const { Order: order, DocumentState: documentState } = review;
  const noDocument = documentState === "none";
  const ready = !saving && (!noDocument || documentConfirmed);
  return (
    <div className="flex flex-col gap-5">
      <div>
        <SectionHeading title="Final save" hint="One-way: a saved record is read-only" />
        <p className="mt-1 max-w-3xl text-body text-ink-500">
          Saving marks this purchase order as finalized entry data for later
          documents. It is not a business approval.
        </p>
      </div>

      <Card className="p-4">
        <dl className="grid gap-2 text-small sm:grid-cols-2">
          <div>
            <dt className="text-caption text-ink-500">Customer PO number</dt>
            <dd className="tnum font-medium text-ink-900">{order.PONumber.trim()}</dd>
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
            <dt className="text-caption text-ink-500">Received date</dt>
            <dd className="tnum font-medium text-ink-900">
              {evaluation.applied.header.receivedDate}
            </dd>
          </div>
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
              No PDF can be attached yet; the requirement to attach one arrives
              with that feature, and this confirmation is recorded so the missing
              evidence is deliberate rather than silent.
            </span>
          </label>
        ) : (
          <p className="mt-2 text-small text-ink-700">
            The attached customer PO PDF stays with this record.
          </p>
        )}
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
}: PurchaseOrderSavedViewProps) {
  const state = initialPurchaseOrderEditorState(order, currency);
  const totals = calculatePurchaseOrder(state, currency).totals;
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading title="Purchase Order" hint={`Saved record ${order.ID}`} />
          <h1 className="tnum text-display font-semibold text-ink-950">
            {order.PONumber}
          </h1>
          <p className="mt-1 text-body text-ink-500">
            {order.CustomerName} · quotation {order.QuoteNumber} revision{" "}
            {order.SourceRevision} · {order.Currency.toUpperCase()}
          </p>
        </div>
        <Badge tone="success">Saved</Badge>
      </div>

      <StatusBanner tone="info">
        Finalized entry data for later documents. This is not a business
        approval.
      </StatusBanner>
      <StatusBanner tone="info">
        This saved purchase order is read-only in this release. Corrections
        arrive with the correction workflow.
      </StatusBanner>

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

      <Card className="p-4">
        <SectionHeading title="Customer PO PDF" hint="Evidence for this record" />
        <p className="mt-2 text-small text-ink-700">
          {order.PdfAbsentConfirmed === true
            ? "No customer PO PDF attached. The final save recorded that confirmation."
            : "No customer PO PDF attached."}
        </p>
      </Card>

      <div className="border-t border-rule pt-4">
        <Button aria-label="Back to purchase orders" onClick={onBack}>
          Back to purchase orders
        </Button>
      </div>
    </div>
  );
}
