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
