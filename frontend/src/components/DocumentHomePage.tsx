import type { POSourceSelection } from "../poSelector";
import { Badge, Button, Card, SectionHeading } from "./ui";

// Issue #466: the post-login document home. Quotation and Purchase Order open
// their pages; Delivery Order and Invoice are visible but disabled with
// "Coming later", so unavailable features are not misleading. No data is
// fetched here: this is a routing surface, not a document list.

export interface DocumentHomePageProps {
  /** Opens the existing Quotation create/archive workflow, as today. */
  onOpenQuotation: () => void;
  /** Opens the Purchase Order entry surface (a placeholder until #467+). */
  onOpenPurchaseOrder: () => void;
}

function DestinationCard({
  title,
  description,
  action,
  comingLater = false,
}: {
  title: string;
  description: string;
  action: React.ReactNode;
  comingLater?: boolean;
}) {
  return (
    <Card className="flex flex-col gap-3 p-5">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-lead font-medium text-ink-900">{title}</h3>
        {comingLater ? <Badge tone="neutral">Coming later</Badge> : null}
      </div>
      <p className="flex-1 text-body leading-relaxed text-ink-500">
        {description}
      </p>
      <div>{action}</div>
    </Card>
  );
}

export function DocumentHomePage({
  onOpenQuotation,
  onOpenPurchaseOrder,
}: DocumentHomePageProps) {
  return (
    <div className="flex flex-col gap-6">
      <SectionHeading title="Documents" />
      <div className="grid gap-6 sm:grid-cols-2">
        <DestinationCard
          title="Quotation"
          description="Create quotations, resume drafts, and browse the archive."
          action={
            <Button
              variant="primary"
              aria-label="Open Quotation"
              onClick={onOpenQuotation}
            >
              Open Quotation
            </Button>
          }
        />
        <DestinationCard
          title="Purchase Order"
          description="Enter a customer purchase order against an issued quotation."
          action={
            <Button
              variant="primary"
              aria-label="Open Purchase Order"
              onClick={onOpenPurchaseOrder}
            >
              Open Purchase Order
            </Button>
          }
        />
        <DestinationCard
          title="Delivery Order"
          description="Delivery orders are not part of this release."
          comingLater
          action={
            <Button
              variant="secondary"
              aria-label="Delivery Order (coming later)"
              disabled
              aria-disabled="true"
            >
              Delivery Order
            </Button>
          }
        />
        <DestinationCard
          title="Invoice"
          description="Invoices are not part of this release."
          comingLater
          action={
            <Button
              variant="secondary"
              aria-label="Invoice (coming later)"
              disabled
              aria-disabled="true"
            >
              Invoice
            </Button>
          }
        />
      </div>
    </div>
  );
}

// The honest Purchase Order surface: entry fields arrive with #468, but the
// source selector (#467) is real now. The action below is the "Add Purchase
// Order from a Quotation" path; a confirmed selection is displayed so the
// hand-off is visible rather than implied.
export interface PurchaseOrderPlaceholderPageProps {
  onAddFromQuotation: () => void;
  selectedSource?: POSourceSelection | null;
}

export function PurchaseOrderPlaceholderPage({
  onAddFromQuotation,
  selectedSource = null,
}: PurchaseOrderPlaceholderPageProps) {
  return (
    <div className="flex flex-col gap-6">
      <SectionHeading title="Purchase Order" />
      <Card className="overflow-hidden">
        <div className="flex flex-col items-center gap-4 px-6 py-16 text-center">
          <p className="text-lead font-medium text-ink-900">
            Purchase order entry is not yet available
          </p>
          <p className="max-w-md text-body text-ink-500">
            Start from an issued quotation revision. The selection is recorded
            now; the entry fields arrive in a later step.
          </p>
          <Button
            variant="primary"
            aria-label="Add Purchase Order from a Quotation"
            onClick={onAddFromQuotation}
          >
            Add Purchase Order from a Quotation
          </Button>
          {selectedSource !== null ? (
            <div className="mt-2 w-full max-w-md rounded-lg border border-rule bg-paper-muted px-4 py-3 text-left">
              <p className="text-small font-medium text-ink-900">
                Source selected
              </p>
              <p className="tnum text-small text-ink-700">
                Quotation {selectedSource.quotationId} · revision{" "}
                {selectedSource.revisionNumber}
              </p>
            </div>
          ) : null}
        </div>
      </Card>
    </div>
  );
}
