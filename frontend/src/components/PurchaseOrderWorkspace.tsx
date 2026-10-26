// Issue #473 wave 3: the Purchase Order page's composition.
//
// This is the one branch App used to hold inline: the archive, the saved
// record, the editor, and the short transition while a cancelled correction
// re-reads the saved record. Keeping it here means the real composition -- not
// a copied condition -- is what the workspace DOM regression renders, and App
// only supplies the page-level callbacks it owns (opening a row, toasting a
// save, the leave guard).

import type { CompanyUnitOption } from "../companyUnits";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import { canRepairSource } from "../poRepair";
import type { PurchaseOrderWorkspace } from "../poWorkspace";
import { PurchaseOrderEditorPage } from "./PurchaseOrderEditor";
import { PurchaseOrderSavedView } from "./PurchaseOrderFinalize";
import { PurchaseOrderListPage } from "./PurchaseOrderListPage";
import { PurchaseOrderRepairPage } from "./PurchaseOrderRepair";
import { Button, Card, SectionHeading, StatusBanner } from "./ui";

/** Which tab of the PO workspace a page belongs to (issue #491). */
export type PurchaseOrderTab = "editor" | "drafts" | "archive";

export interface PurchaseOrderWorkspacePageProps {
  workspace: PurchaseOrderWorkspace;
  /** The record's own currency, or null while the catalogue cannot name it. */
  currency: CurrencyInfo | null;
  uomOptions: CompanyUnitOption[];
  incotermOptions: { value: string; label: string }[];
  /** Issue #491: the active workspace tab when no record is open. */
  tab: PurchaseOrderTab;
  /**
   * #496: the transient source-less draft the Editor tab shows before any row
   * exists. Null keeps the old entry page (used by tests and as a fallback).
   */
  blankDraft?: PurchaseOrderDTO | null;
  /** The loaded currency catalogue, for editing the basis before linking. */
  currencyOptions?: CurrencyInfo[];
  /** #497: opens the shared selector for this draft's source link. */
  onLinkQuotation?: () => void;
  /** #497: the selector's confirmed pair, consumed by the editor. */
  pendingLinkTarget?: { quotationId: number; revisionNumber: number } | null;
  onLinkTargetHandled?: () => void;
  /** #497: the link committed; App words its success message. */
  onLinked?: (order: PurchaseOrderDTO) => void;
  /**
   * Issue #492: the account's remembered archive page size. Only the initial
   * list state uses it; the server's own page size takes over once a page
   * loads, because the archive is server-paged.
   */
  pageSize?: number;
  /** Starts a new PO through the shared source selector. */
  onAdd: () => void;
  /** Whether the signed-in user may repair a saved record's source link. */
  isAdmin: boolean;
  /**
   * Opens the shared selector in repair mode for the open saved record. App
   * loads the candidate review and hands it to the workspace.
   */
  onRepairSource: () => void;
  /** Opens one archive row by id (App fetches the record). */
  onOpen: (id: number) => void;
  /** A save succeeded; App words its success message. */
  onSaved?: (order: PurchaseOrderDTO, corrected?: boolean) => void;
  /** A source repair committed; App words its success message. */
  onRepaired?: (order: PurchaseOrderDTO) => void;
  /** App's discard guard on the way out of the saved view. */
  onBack: () => void;
  /** The editor's Cancel, delegating to the workspace controller. */
  onCancel: () => void;
  onDirtyChange: (dirty: boolean) => void;
}

/**
 * The Editor tab before a record is open: the entry point to the shared
 * quotation selector. Kept separate from the archive so "Editor" never reads
 * as a record list.
 */
export function PurchaseOrderEntryPage({ onAdd }: { onAdd: () => void }) {
  return (
    <section aria-labelledby="po-entry-heading" className="flex flex-col gap-5">
      <div>
        <h1 id="po-entry-heading" className="text-title font-semibold text-ink-950">
          New purchase order
        </h1>
        <p className="mt-1 max-w-2xl text-body text-ink-500">
          Start a purchase order from an issued quotation revision. The
          quotation supplies the customer, currency and line items; the
          customer PO number and dates are entered here.
        </p>
      </div>
      <Card className="p-6">
        <SectionHeading title="From a quotation" />
        <p className="max-w-2xl text-body leading-relaxed text-ink-500">
          Choose an issued quotation and revision. A draft is created so the
          entry can be saved and continued later; finalizing it stores the
          record in the Archive.
        </p>
        <div className="mt-4">
          <Button
            variant="primary"
            aria-label="Add Purchase Order from a Quotation"
            onClick={onAdd}
          >
            Add Purchase Order from a Quotation
          </Button>
        </div>
      </Card>
    </section>
  );
}

export function PurchaseOrderWorkspacePage({
  workspace,
  currency,
  uomOptions,
  incotermOptions,
  tab,
  pageSize,
  blankDraft = null,
  currencyOptions = [],
  onLinkQuotation,
  pendingLinkTarget = null,
  onLinkTargetHandled,
  onLinked,
  onAdd,
  onOpen,
  isAdmin,
  onRepairSource,
  onSaved,
  onRepaired,
  onBack,
  onCancel,
  onDirtyChange,
}: PurchaseOrderWorkspacePageProps) {
  const { draft, phase } = workspace;

  if (phase === "list" || draft === null) {
    if (tab === "editor" && blankDraft !== null) {
      const blankCurrency =
        currencyOptions.find((option) => option.code === blankDraft.Currency) ??
        currency;
      if (blankCurrency === null) {
        return (
          <StatusBanner tone="danger">
            The purchase order's currency ({blankDraft.Currency}) is not in the
            loaded currency catalogue, so money cannot be edited safely yet.
          </StatusBanner>
        );
      }
      // #496: the editor opens on the full blank manual-entry form. It is
      // transient until Save draft or an upload asks for a row; merely opening
      // it creates nothing.
      return (
        <PurchaseOrderEditorPage
          key={0}
          order={blankDraft}
          currency={blankCurrency}
          currencyOptions={currencyOptions}
          uomOptions={uomOptions}
          incotermOptions={incotermOptions}
          onLinkQuotation={onLinkQuotation}
          pendingLinkTarget={pendingLinkTarget}
          onLinkTargetHandled={onLinkTargetHandled}
          onLinked={onLinked}
          onDirtyChange={onDirtyChange}
          onSaved={(updated, corrected) => {
            workspace.saved(updated);
            onSaved?.(updated, corrected);
          }}
          onCancel={onCancel}
        />
      );
    }
    if (tab === "drafts") {
      return (
        <PurchaseOrderListPage
          key="drafts"
          mode="drafts"
          pageSize={pageSize}
          onAdd={onAdd}
          onOpen={onOpen}
          isAdmin={isAdmin}
        />
      );
    }
    if (tab === "archive") {
      return (
        <PurchaseOrderListPage
          key="archive"
          mode="archive"
          pageSize={pageSize}
          onAdd={onAdd}
          onOpen={onOpen}
          isAdmin={isAdmin}
        />
      );
    }
    return <PurchaseOrderEntryPage onAdd={onAdd} />;
  }

  // Neither the discarded working copy nor a stale saved snapshot is shown
  // while the server's record is being read back.
  if (phase === "refreshing") {
    return (
      <StatusBanner tone="info">
        <span role="status">Loading the saved purchase order…</span>
      </StatusBanner>
    );
  }

  if (currency === null) {
    return (
      <StatusBanner tone="danger">
        The purchase order's currency ({draft.Currency}) is not in the loaded
        currency catalogue, so money cannot be edited safely yet.
      </StatusBanner>
    );
  }

  if (phase === "repair" && workspace.repairing !== null) {
    // #475: an admin source repair. The saved record is unchanged until the
    // repair commits; Cancel returns to the saved view with the old link.
    return (
      <PurchaseOrderRepairPage
        review={workspace.repairing}
        currency={currency}
        onCancel={() => {
          onDirtyChange(false);
          workspace.cancelRepair();
        }}
        onRepaired={(order) => {
          workspace.repaired(order);
          onRepaired?.(order);
        }}
        onDirtyChange={onDirtyChange}
      />
    );
  }

  if (phase === "saved") {
    // #471/#473: a saved record opens read-only; Correct is the one entry into
    // the correction editor, which asks for a reason and records the replaced
    // values. Repairing the source link is a separate admin action (#475).
    return (
      <PurchaseOrderSavedView
        order={draft}
        currency={currency}
        onCorrect={workspace.correct}
        canRepairSource={canRepairSource(draft, isAdmin)}
        onRepairSource={onRepairSource}
        onAssigned={workspace.assigned}
        onBack={onBack}
      />
    );
  }

  return (
    <PurchaseOrderEditorPage
      key={draft.ID}
      order={draft}
      currency={currency}
      currencyOptions={currencyOptions}
      uomOptions={uomOptions}
      incotermOptions={incotermOptions}
      onLinkQuotation={onLinkQuotation}
      pendingLinkTarget={pendingLinkTarget}
      onLinkTargetHandled={onLinkTargetHandled}
      onLinked={onLinked}
      mode={workspace.correcting ? "correction" : "draft"}
      onCorrectionStart={workspace.correctionStarted}
      onDirtyChange={onDirtyChange}
      onSaved={(updated, corrected) => {
        workspace.saved(updated);
        onSaved?.(updated, corrected);
      }}
      onCancel={onCancel}
    />
  );
}
