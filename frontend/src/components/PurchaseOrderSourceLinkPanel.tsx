// Issue #497: the Source link panel at the top of the Purchase Order editor.
//
// The idle panel replaces the deferred #496 affordance: a never-linked or
// formerly-linked draft offers "Link a Quotation", and a live draft shows a
// read-only linked-source line with no re-link action. The review panel is the
// same difference review the final save uses, with the link's tax decision and
// currency remedy in front of it; no second comparison or review page exists.

import type { CurrencyInfo } from "../format";
import type {
  POReviewChoice,
  POReviewDecisions,
  PurchaseOrderSourceLineDTO,
} from "../poFinalize";
import type {
  PurchaseOrderEditorState,
  PurchaseOrderSourceState,
} from "../purchaseOrder";
import {
  draftLinkCurrencyRemedy,
  draftTaxBasisLabel,
  type PurchaseOrderDraftLinkReviewDTO,
  type PurchaseOrderDraftTaxChoice,
} from "../poDraftLink";
import { PurchaseOrderDifferenceReviewView } from "./PurchaseOrderFinalize";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

export interface PurchaseOrderSourceLinkPanelProps {
  sourceState: PurchaseOrderSourceState;
  quoteNumber: string;
  sourceRevision: number;
  /** A correction must not offer a link; the saved record has its own repair. */
  correcting: boolean;
  /** Opens the shared source selector. Undefined leaves the action disabled. */
  onChooseSource?: () => void;
}

/**
 * The top-of-editor link affordance. Once a source is live it is replaced by a
 * status line: this slice has no live re-link action.
 */
export function PurchaseOrderSourceLinkPanel({
  sourceState,
  quoteNumber,
  sourceRevision,
  correcting,
  onChooseSource,
}: PurchaseOrderSourceLinkPanelProps) {
  if (correcting) return null;

  if (sourceState === "live") {
    return (
      <Card className="p-4">
        <SectionHeading
          title="Linked source"
          hint="A draft with a live source is not re-linked here"
        />
        <p className="mt-1 max-w-2xl text-small leading-relaxed text-ink-500">
          This draft follows quotation{" "}
          <span className="font-medium text-ink-800">{quoteNumber}</span>, revision{" "}
          <span className="tnum font-medium text-ink-800">{sourceRevision}</span>. The
          customer, quote number, revision and printed totals come from that revision;
          the values entered here stay as they are.
        </p>
      </Card>
    );
  }

  const neverLinked = sourceState === "never";
  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading
            title="Link a Quotation"
            hint="Linking repeats the difference review and keeps every value you enter"
          />
          <p className="mt-1 max-w-2xl text-small leading-relaxed text-ink-500">
            {neverLinked
              ? "This draft has no source yet. Enter the customer's order now; linking a quotation is the next step and will not replace these fields."
              : "The quotation this draft followed was deleted. The values below are kept; link a replacement through the same difference review."}
          </p>
        </div>
        <Button
          aria-label="Link a Quotation"
          onClick={onChooseSource}
          disabled={onChooseSource === undefined}
          title="Linking a quotation is the next step; your entries are kept."
        >
          Link a Quotation
        </Button>
      </div>
    </Card>
  );
}

export interface PurchaseOrderDraftLinkReviewPanelProps {
  state: PurchaseOrderEditorState;
  review: PurchaseOrderDraftLinkReviewDTO;
  currency: CurrencyInfo;
  decisions: POReviewDecisions;
  acknowledgedMissing: string[];
  acknowledgedExtra: number[];
  taxChoice: PurchaseOrderDraftTaxChoice;
  linking: boolean;
  error: string | null;
  onDecision: (key: string, choice: POReviewChoice) => void;
  onToggleMissing: (label: string) => void;
  onToggleExtra: (lineNumber: number) => void;
  onMatchChange: (lineKey: number, source: PurchaseOrderSourceLineDTO | null) => void;
  onTaxChoice: (choice: PurchaseOrderDraftTaxChoice) => void;
  onBack: () => void;
  onLink: () => void;
}

/**
 * The draft-link review. It reuses the final-save difference view in its link
 * mode and adds only the tax decision the server requires, plus an explicit
 * currency-mismatch remedy. The link button is blocked while a required tax
 * choice is unmade, because the server refuses that request by name.
 */
export function PurchaseOrderDraftLinkReviewPanel({
  state,
  review,
  currency,
  decisions,
  acknowledgedMissing,
  acknowledgedExtra,
  taxChoice,
  linking,
  error,
  onDecision,
  onToggleMissing,
  onToggleExtra,
  onMatchChange,
  onTaxChoice,
  onBack,
  onLink,
}: PurchaseOrderDraftLinkReviewPanelProps) {
  const taxUnresolved = review.TaxChoiceRequired && taxChoice === "";
  const displayPpnBps =
    taxChoice === "source" ? review.SourceTax.PpnBps : review.DraftTax.PpnBps;
  const currencyRemedy = draftLinkCurrencyRemedy(
    state.sourceState,
    state.currency,
    review.Source.Currency
  );

  const taxCard = (
    <Card className="p-4">
      <SectionHeading
        title="Tax basis"
        hint={
          review.TaxChoiceRequired
            ? "A choice is required before linking"
            : "Both sides use the same basis"
        }
      />
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <div
          className={`grid content-start gap-2 rounded-lg border p-3 ${
            taxChoice === "keep" ? "border-accent-500 bg-accent-50" : "border-rule"
          }`}
        >
          <p className="text-small font-medium text-ink-900">This purchase order</p>
          <p className="text-caption text-ink-600">{draftTaxBasisLabel(review.DraftTax)}</p>
          {review.TaxChoiceRequired ? (
            <Button
              size="sm"
              variant={taxChoice === "keep" ? "primary" : "secondary"}
              aria-pressed={taxChoice === "keep"}
              aria-label="Keep the draft tax basis"
              onClick={() => onTaxChoice("keep")}
            >
              Keep this basis
            </Button>
          ) : null}
        </div>
        <div
          className={`grid content-start gap-2 rounded-lg border p-3 ${
            taxChoice === "source" ? "border-accent-500 bg-accent-50" : "border-rule"
          }`}
        >
          <p className="text-small font-medium text-ink-900">Quotation revision</p>
          <p className="text-caption text-ink-600">{draftTaxBasisLabel(review.SourceTax)}</p>
          {review.TaxChoiceRequired ? (
            <Button
              size="sm"
              variant={taxChoice === "source" ? "primary" : "secondary"}
              aria-pressed={taxChoice === "source"}
              aria-label="Use the quotation tax basis"
              onClick={() => onTaxChoice("source")}
            >
              Use the quotation's
            </Button>
          ) : null}
        </div>
      </div>
      <p className="mt-3 text-caption text-ink-500">
        PPN is recomputed from the chosen basis when the link commits.{" "}
        {review.TaxChoiceRequired
          ? "Nothing is adopted silently, and the basis is frozen again once the link is saved."
          : "Both bases are the same, so no choice is needed."}
      </p>
    </Card>
  );

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading
            title="Link this draft to a quotation"
            hint={`${review.Source.QuoteNumber} · revision ${review.Source.RevisionNumber}`}
          />
          <p className="mt-1 max-w-3xl text-body text-ink-500">
            Linking makes this draft follow the selected quotation revision. The
            customer, quote number, revision, quotation line references and printed
            totals come from it; every other value stays as you entered it. Each
            difference needs an explicit choice before the link can be saved.
          </p>
        </div>
        <Badge tone="accent">Source link</Badge>
      </div>

      {error !== null ? <StatusBanner tone="danger">{error}</StatusBanner> : null}

      <PurchaseOrderDifferenceReviewView
        state={state}
        source={review.Source}
        currency={currency}
        quoteNumber={review.Source.QuoteNumber}
        mode="link"
        documentState={review.DocumentState === "attached" ? "attached" : "none"}
        decisions={decisions}
        acknowledgedMissing={acknowledgedMissing}
        acknowledgedExtra={acknowledgedExtra}
        onDecision={onDecision}
        onToggleMissing={onToggleMissing}
        onToggleExtra={onToggleExtra}
        onMatchChange={onMatchChange}
        onBack={onBack}
        onContinue={onLink}
        ppnBpsOverride={displayPpnBps}
        currencyRemedy={currencyRemedy}
        busy={linking}
        blocked={taxUnresolved}
        beforeDifferences={taxCard}
      />
    </div>
  );
}
