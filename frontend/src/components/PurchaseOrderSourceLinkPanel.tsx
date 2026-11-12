// Issue #497: the Source link panel at the top of the Purchase Order editor.
//
// The idle panel replaces the deferred #496 affordance: a never-linked or
// formerly-linked draft offers "Link a Quotation", and a live draft shows a
// read-only linked-source line with no re-link action. The inline editor owns
// the comparison: linking keeps the entered values and shows the quotation
// values beside each field.

import type { PurchaseOrderSourceState } from "../purchaseOrder";
import { Button, Card, SectionHeading } from "./ui";

export interface PurchaseOrderSourceLinkPanelProps {
  sourceState: PurchaseOrderSourceState;
  quoteNumber: string;
  sourceRevision: number;
  /** A correction must not offer a link; the saved record has its own repair. */
  correcting: boolean;
  /** Opens the shared source selector. Undefined leaves the action disabled. */
  onChooseSource?: () => void;
  /**
   * The page's sticky rail is narrow. The compact card drops the horizontal
   * heading rule and stacks the action, matching the customer-PDF card below.
   */
  compact?: boolean;
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
  compact = false,
}: PurchaseOrderSourceLinkPanelProps) {
  if (correcting) return null;

  if (sourceState === "live") {
    if (compact) {
      return (
        <Card className="p-4" data-tutorial="po-linked-source">
          <div className="flex items-center justify-between gap-2">
            <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-ink-500">
              Linked source
            </p>
            <span className="inline-flex items-center gap-1.5 rounded-full border border-success-200 bg-success-50 px-2 py-0.5 text-[9px] font-semibold uppercase tracking-[0.1em] text-success-800">
              <span
                aria-hidden="true"
                className="size-1.5 rounded-full bg-success-500"
              />
              Live
            </span>
          </div>
          <p className="mt-1.5 text-small font-semibold text-ink-950">
            {quoteNumber}
          </p>
          <p className="mt-0.5 text-caption text-ink-500">
            Revision <span className="tnum">{sourceRevision}</span>. Your values
            stay; quotation values appear beside each field.
          </p>
        </Card>
      );
    }
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
  if (compact) {
    return (
      <Card
        className="border-accent-200 bg-accent-50/40 p-4"
        data-tutorial="po-link"
      >
        <div className="flex items-start justify-between gap-2">
          <div className="grid gap-1">
            <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-accent-700">
              Source quotation
            </p>
            <h2 className="text-[15px] font-semibold leading-snug tracking-[-0.01em] text-ink-950">
              Link a quotation
            </h2>
          </div>
          <span
            aria-hidden="true"
            className="grid size-7 shrink-0 place-items-center rounded-full bg-white text-accent-700 ring-1 ring-accent-200"
          >
            <svg
              width="13"
              height="13"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
            >
              <path d="M6.5 9.5l3-3" />
              <path d="M7 4.5l1.2-1.2a2.5 2.5 0 013.5 3.5L10.5 8" />
              <path d="M9 11.5l-1.2 1.2a2.5 2.5 0 01-3.5-3.5L5.5 8" />
            </svg>
          </span>
        </div>
        <p className="mt-2 text-caption leading-relaxed text-ink-600">
          {neverLinked
            ? "Every value you enter is kept. The quotation values appear beside your fields."
            : "The previous source was removed. Your values stay; link a replacement to compare inline."}
        </p>
        <Button
          className="mt-3 w-full"
          aria-label="Link a Quotation"
          data-tutorial="po-link-button"
          onClick={onChooseSource}
          disabled={onChooseSource === undefined}
          title="Linking a quotation is the next step; your entries are kept."
        >
          Link a Quotation
        </Button>
      </Card>
    );
  }

  return (
    <Card className="p-4" data-tutorial="po-link">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <SectionHeading
            title="Link a Quotation"
            hint="Linking keeps every value you enter and shows the quotation values inline"
          />
          <p className="mt-1 max-w-2xl text-small leading-relaxed text-ink-500">
            {neverLinked
              ? "This draft has no source yet. Enter the customer's order now; linking a quotation is the next step and will not replace these fields."
              : "The quotation this draft followed was deleted. The values below are kept; link a replacement to compare its values inline."}
          </p>
        </div>
        <Button
          aria-label="Link a Quotation"
          data-tutorial="po-link-button"
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
