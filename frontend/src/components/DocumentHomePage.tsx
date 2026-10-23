import type { ReactNode } from "react";
import { Badge, Button, Card, SectionHeading } from "./ui";

// Issue #466: the post-login home. Issue #491: it is now a global workspace
// dashboard, not a tab inside Quotation. Each card opens a workspace; Delivery
// Order and Invoice are visible but disabled with "Coming later", so
// unavailable features are not misleading. No data is fetched here: this is a
// routing surface, not a document list.

export interface DocumentHomePageProps {
  /** Opens the Quotation workspace's Editor tab. */
  onOpenQuotation: () => void;
  /** Opens the Purchase Order workspace's Editor tab. */
  onOpenPurchaseOrder: () => void;
  /** Opens the Admin Dashboard. Hidden for non-admins. */
  onOpenAdmin?: () => void;
  /** Whether the signed-in user may see the Admin Dashboard card. */
  isAdmin?: boolean;
}

function DestinationCard({
  title,
  description,
  action,
  icon,
  comingLater = false,
  wide = false,
}: {
  title: string;
  description: string;
  action: ReactNode;
  icon: ReactNode;
  comingLater?: boolean;
  /** Admin spans both columns at wide widths; at 360 it is one column. */
  wide?: boolean;
}) {
  return (
    <Card
      className={`flex flex-col gap-4 p-5 ${
        wide ? "sm:col-span-2 sm:flex-row sm:items-center" : ""
      }`}
    >
      <div className="flex items-center gap-3">
        <span
          className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border ${
            comingLater
              ? "border-rule bg-paper-subtle text-ink-300"
              : "border-accent-200 bg-accent-50 text-accent-700"
          }`}
          aria-hidden="true"
        >
          {icon}
        </span>
        <h3 className="text-lead font-medium text-ink-900">{title}</h3>
        {comingLater ? <Badge tone="neutral">Coming later</Badge> : null}
      </div>
      <p className="flex-1 text-body leading-relaxed text-ink-500">
        {description}
      </p>
      <div className={wide ? "sm:ml-auto" : ""}>{action}</div>
    </Card>
  );
}

const iconProps = {
  width: 20,
  height: 20,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.7,
  strokeLinecap: "round",
  strokeLinejoin: "round",
} as const;

function QuotationIcon() {
  return (
    <svg {...iconProps}>
      <path d="M6 3h9l4 4v14H6z" />
      <path d="M15 3v4h4M9 12h6M9 16h6" />
    </svg>
  );
}

function PurchaseOrderIcon() {
  return (
    <svg {...iconProps}>
      <path d="M3 6h2l2 10h11l2-7H6" />
      <circle cx="9" cy="19" r="1.5" />
      <circle cx="17" cy="19" r="1.5" />
      <path d="M9 10h9" />
    </svg>
  );
}

function DeliveryOrderIcon() {
  return (
    <svg {...iconProps}>
      <path d="M3 7h11v9H3zM14 10h4l3 3v3h-7" />
      <circle cx="7" cy="18" r="1.5" />
      <circle cx="18" cy="18" r="1.5" />
    </svg>
  );
}

function InvoiceIcon() {
  return (
    <svg {...iconProps}>
      <path d="M6 3h12v18l-3-2-3 2-3-2-3 2z" />
      <path d="M9 8h6M9 12h6M9 16h3" />
    </svg>
  );
}

function AdminIcon() {
  return (
    <svg {...iconProps}>
      <path d="M12 3l7 3v6c0 4.2-2.8 7.3-7 9-4.2-1.7-7-4.8-7-9V6z" />
      <path d="M9 12l2 2 4-4" />
    </svg>
  );
}

export function DocumentHomePage({
  onOpenQuotation,
  onOpenPurchaseOrder,
  onOpenAdmin,
  isAdmin = false,
}: DocumentHomePageProps) {
  return (
    <div className="flex flex-col gap-6">
      <SectionHeading title="Workspaces" />
      <div className="grid gap-6 sm:grid-cols-2">
        <DestinationCard
          title="Quotation"
          description="Create quotations, resume drafts, and browse the archive."
          icon={<QuotationIcon />}
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
          description="Enter a purchase order, resume drafts, and browse the archive."
          icon={<PurchaseOrderIcon />}
          action={
            <Button
              variant="primary"
              aria-label="Open Purchase Orders"
              onClick={onOpenPurchaseOrder}
            >
              Open Purchase Orders
            </Button>
          }
        />
        <DestinationCard
          title="Delivery Order"
          description="Delivery orders are not part of this release."
          icon={<DeliveryOrderIcon />}
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
          icon={<InvoiceIcon />}
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
        {isAdmin && onOpenAdmin ? (
          <DestinationCard
            title="Admin Dashboard"
            description="Clients, the team, company settings and database backups."
            icon={<AdminIcon />}
            wide
            action={
              <Button
                variant="secondary"
                aria-label="Open Admin Dashboard"
                onClick={onOpenAdmin}
              >
                Open Admin Dashboard
              </Button>
            }
          />
        ) : null}
      </div>
    </div>
  );
}
