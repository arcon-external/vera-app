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
import type { PurchaseOrderWorkspace } from "../poWorkspace";
import { PurchaseOrderEditorPage } from "./PurchaseOrderEditor";
import { PurchaseOrderSavedView } from "./PurchaseOrderFinalize";
import { PurchaseOrderListPage } from "./PurchaseOrderListPage";
import { StatusBanner } from "./ui";

export interface PurchaseOrderWorkspacePageProps {
  workspace: PurchaseOrderWorkspace;
  /** The record's own currency, or null while the catalogue cannot name it. */
  currency: CurrencyInfo | null;
  uomOptions: CompanyUnitOption[];
  incotermOptions: { value: string; label: string }[];
  /** Starts a new PO through the shared source selector. */
  onAdd: () => void;
  /** Opens one archive row by id (App fetches the record). */
  onOpen: (id: number) => void;
  /** A save succeeded; App words its success message. */
  onSaved?: (order: PurchaseOrderDTO, corrected?: boolean) => void;
  /** App's discard guard on the way out of the saved view. */
  onBack: () => void;
  /** The editor's Cancel, delegating to the workspace controller. */
  onCancel: () => void;
  onDirtyChange: (dirty: boolean) => void;
}

export function PurchaseOrderWorkspacePage({
  workspace,
  currency,
  uomOptions,
  incotermOptions,
  onAdd,
  onOpen,
  onSaved,
  onBack,
  onCancel,
  onDirtyChange,
}: PurchaseOrderWorkspacePageProps) {
  const { draft, phase } = workspace;

  if (phase === "list" || draft === null) {
    return <PurchaseOrderListPage onAdd={onAdd} onOpen={onOpen} />;
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

  if (phase === "saved") {
    // #471/#473: a saved record opens read-only; Correct is the one entry into
    // the correction editor, which asks for a reason and records the replaced
    // values.
    return (
      <PurchaseOrderSavedView
        order={draft}
        currency={currency}
        onCorrect={workspace.correct}
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
      uomOptions={uomOptions}
      incotermOptions={incotermOptions}
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
