import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { PurchaseOrderWorkspacePage } from "./PurchaseOrderWorkspace";
import type { PurchaseOrderWorkspace } from "../poWorkspace";
import type { CurrencyInfo } from "../format";

// Issue #491: the PO workspace's Editor tab is the entry surface, while Drafts
// and Archive are the same list pinned to one stored state. The suite runs on
// node with no DOM; the list's fetch effect never runs here, so this owns the
// composition, not the query (poList and the DOM suite own that).

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function listWorkspace(): PurchaseOrderWorkspace {
  return {
    draft: null,
    correcting: false,
    repairing: null,
    phase: "list",
    open: () => undefined,
    close: () => undefined,
    correct: () => undefined,
    correctionStarted: () => undefined,
    saved: () => undefined,
    assigned: () => undefined,
    repair: () => undefined,
    repaired: () => undefined,
    cancelRepair: () => undefined,
    cancel: () => undefined,
  };
}

function render(tab: "editor" | "drafts" | "archive"): string {
  return renderToString(
    <PurchaseOrderWorkspacePage
      workspace={listWorkspace()}
      currency={USD}
      uomOptions={[]}
      incotermOptions={[]}
      tab={tab}
      onAdd={() => undefined}
      isAdmin={false}
      onRepairSource={() => undefined}
      onOpen={() => undefined}
      onBack={() => undefined}
      onCancel={() => undefined}
      onDirtyChange={() => undefined}
    />
  ).replace(/<!-- -->/g, "");
}

describe("PurchaseOrderWorkspacePage tabs (issue #491)", () => {
  it("Editor shows the PO entry, not a record list", () => {
    const html = render("editor");
    expect(html).toContain(">New purchase order<");
    expect(html).toContain('aria-label="Add Purchase Order from a Quotation"');
    expect(html).not.toContain(">Draft purchase orders<");
    expect(html).not.toContain(">Purchase order archive<");
  });

  it("Drafts lists draft purchase orders with the state pinned", () => {
    const html = render("drafts");
    expect(html).toContain(">Draft purchase orders<");
    expect(html).not.toContain('aria-label="State"');
    expect(html).toContain('aria-label="Search purchase orders"');
    expect(html).toContain('aria-label="Source"');
  });

  it("Archive lists saved purchase orders with the state pinned", () => {
    const html = render("archive");
    expect(html).toContain(">Purchase order archive<");
    expect(html).not.toContain('aria-label="State"');
    expect(html).toContain('aria-label="Search purchase orders"');
  });
});
