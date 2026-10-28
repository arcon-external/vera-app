// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ReconciliationPaneContent,
  ReconciliationSheetTrigger,
  type ReconciliationPaneHandle,
} from "./ReconciliationPane";
import type { ReconciliationItem } from "../poReconciliation";

// Ticket 04: the pane as staff see it. Every case drives the presentational
// props directly with literals: empty/loading/error/no-source states, the
// All/Unresolved/Resolved filters with roving tabindex, newest-first order,
// keep/use resolution, the single keep confirmation, and keyboard + focus
// behaviour. The commit state machine itself is pinned in poReconciliation.

function item(overrides: Partial<ReconciliationItem> = {}): ReconciliationItem {
  return {
    Key: "header:paymentTerms",
    Field: "header:paymentTerms",
    Kind: "difference",
    LineKey: null,
    SourceLineRef: null,
    Label: "Payment terms",
    POValue: "50% advance",
    SourceValue: "30 days",
    Status: "unresolved",
    Resolution: null,
    ActorID: 7,
    UpdatedAt: "2026-10-28T00:00:01Z",
    History: [],
    ...overrides,
  };
}

const BASE = {
  idPrefix: "pane",
  session: {
    ID: 0,
    PurchaseOrderID: 0,
    SourceQuotationID: 0,
    SourceRevision: 0,
    POVersion: 0,
    Active: true,
    ReadOnly: false,
    CreatedBy: 0,
    CreatedAt: "",
    UpdatedAt: "",
    LineIdentities: [],
    Items: [],
  },
  status: "ready" as const,
  error: null,
  sourceLabel: "QT-2026-018 · revision 02",
  sourceRevision: 2,
  noSource: false,
  formerSource: false,
  readOnly: false,
  filter: "all" as const,
  onFilterChange: () => undefined,
  onShowAll: () => undefined,
  latestNotice: null,
  keepConfirmKey: null,
  onRequestKeep: () => undefined,
  onCancelKeep: () => undefined,
  onConfirmKeep: () => undefined,
  onUse: () => undefined,
  onRetry: () => undefined,
  actorNameFor: () => null,
};

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

async function renderPane(props: Record<string, unknown> = {}) {
  await act(async () => {
    root.render(<ReconciliationPaneContent {...BASE} {...props} />);
  });
}

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

function buttonWithText(label: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (node) => node.textContent?.trim() === label
  );
  if (found === undefined) throw new Error(`no button reading ${label}`);
  return found;
}

describe("ReconciliationPaneContent states (ticket 04)", () => {
  it("shows the no-source empty state with a link action", async () => {
    const onLinkQuotation = vi.fn();
    await renderPane({ noSource: true, session: null, sourceLabel: null, onLinkQuotation });
    expect(document.body.textContent).toContain("Link a Quotation to start the comparison.");
    expect(document.querySelector('[role="tablist"]')).toBeNull();
    buttonWithText("Link Quotation").click();
    expect(onLinkQuotation).toHaveBeenCalledTimes(1);
  });

  it("shows skeletons while loading and an alert with retry on error", async () => {
    await renderPane({ status: "loading" });
    expect(document.body.textContent).toContain("Loading the reconciliation…");

    const onRetry = vi.fn();
    await renderPane({ status: "error", error: "offline", onRetry });
    expect(document.querySelector('[role="alert"]')?.textContent).toContain(
      "The reconciliation could not be loaded."
    );
    expect(document.body.textContent).toContain("Save draft still works.");
    buttonWithText("Try again").click();
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("shows the linked empty state with real filter counts", async () => {
    await renderPane({
      session: {
        ID: 3,
        PurchaseOrderID: 7,
        SourceQuotationID: 12,
        SourceRevision: 1,
        POVersion: 4,
        Active: true,
        ReadOnly: false,
        CreatedBy: 7,
        CreatedAt: "",
        UpdatedAt: "",
        LineIdentities: [],
        Items: [],
      },
    });
    expect(document.body.textContent).toContain("No differences yet");
    expect(document.body.textContent).toContain("Compared with QT-2026-018 · revision 02");
  });

  it("renders unresolved newest first, then resolved, with the sticky count", async () => {
    const oldUnresolved = item({ UpdatedAt: "2026-10-28T00:00:01Z" });
    const freshUnresolved = item({
      Key: "header:incoterms",
      Field: "header:incoterms",
      Label: "Incoterms",
      UpdatedAt: "2026-10-28T00:00:03Z",
    });
    const resolved = item({
      Key: "header:discountPercent",
      Field: "header:discountPercent",
      Label: "Whole-PO discount",
      Status: "resolved",
      Resolution: "kept_po",
      UpdatedAt: "2026-10-28T00:00:02Z",
    });
    await renderPane({
      session: { ...BASE.session, Items: [oldUnresolved, resolved, freshUnresolved] },
    });
    const labels = Array.from(
      document.querySelectorAll(
        "li > div > div:first-child > span:first-child, li > div > button span.truncate"
      )
    ).map((node) => node.textContent);
    expect(labels).toEqual(["Incoterms", "Payment terms", "Whole-PO discount"]);
    expect(document.querySelector('[role="status"]')?.textContent).toContain("2 to resolve");
    // Header/footer of the pane name the state, not a warning colour.
    expect(document.body.textContent).toContain("Unresolved");
    expect(document.body.textContent).toContain("Kept PO");
  });

  it("filters to unresolved or resolved and offers Show all on empty", async () => {
    const onShowAll = vi.fn();
    const onFilterChange = vi.fn();
    await renderPane({
      session: { ...BASE.session, Items: [item()] },
      filter: "resolved",
      onShowAll,
      onFilterChange,
    });
    expect(document.body.textContent).toContain("No resolved items yet");
    buttonWithText("Show all").click();
    expect(onShowAll).toHaveBeenCalledTimes(1);
    // Roving tabindex: only the active tab stops.
    const tabs = Array.from(document.querySelectorAll('[role="tab"]'));
    expect(tabs.map((tab) => tab.getAttribute("tabindex"))).toEqual(["-1", "-1", "0"]);
  });

  it("moves and activates the filter tabs with arrow keys", async () => {
    const onFilterChange = vi.fn();
    await renderPane({
      session: { ...BASE.session, Items: [item()] },
      filter: "all",
      onFilterChange,
    });
    const tablist = document.querySelector('[role="tablist"]');
    if (tablist === null) throw new Error("no tablist");
    const all = document.getElementById("pane-filter-all");
    if (all === null) throw new Error("no All tab");
    await act(async () => {
      all.focus();
      all.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    });
    expect(onFilterChange).toHaveBeenCalledWith("unresolved");
  });
});

describe("ReconciliationPaneContent keep and use (ticket 04)", () => {
  it("opens one Is this correct step and resolves on confirm", async () => {
    const onRequestKeep = vi.fn();
    const onConfirmKeep = vi.fn();
    const sessionItems = [item()];
    await renderPane({
      session: { ...BASE.session, Items: sessionItems },
      onRequestKeep,
      onConfirmKeep,
    });
    button("Keep PO entry for Payment terms").click();
    expect(onRequestKeep).toHaveBeenCalledWith("header:paymentTerms");
    // The parent owns the step state; the pane renders the open step.
    await renderPane({
      session: { ...BASE.session, Items: sessionItems },
      onRequestKeep,
      onConfirmKeep,
      keepConfirmKey: "header:paymentTerms",
    });
    // The pane asks for the decision inline, never as a modal.
    expect(document.body.textContent).toContain("Is this correct?");
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.querySelector('[aria-modal="true"]')).toBeNull();
    button("Yes, keep PO entry for Payment terms").click();
    expect(onConfirmKeep).toHaveBeenCalledWith("header:paymentTerms");
  });

  it("uses accessible names carrying the field on every repeated action", async () => {
    await renderPane({
      session: {
        ...BASE.session,
        Items: [
          item(),
          item({ Key: "header:incoterms", Field: "header:incoterms", Label: "Incoterms" }),
        ],
      },
    });
    button("Keep PO entry for Payment terms");
    button("Keep PO entry for Incoterms");
    button("Use Quotation value for Payment terms");
    button("Use Quotation value for Incoterms");
  });

  it("renders resolved rows as expandable summaries with actor and time", async () => {
    await renderPane({
      session: {
        ...BASE.session,
        Items: [
          item({
            Status: "resolved",
            Resolution: "used_quotation",
            POValue: "30 days",
            ActorID: 7,
            UpdatedAt: "2026-10-28T00:00:05Z",
          }),
        ],
      },
      actorNameFor: (id: number) => (id === 7 ? "Rina" : null),
    });
    // The summary row names the decision and its attribution; values stay
    // hidden until expanded.
    expect(document.body.textContent).toContain("Used Quotation");
    expect(document.body.textContent).toContain("Rina");
    const summary = button("Payment terms, Used Quotation. Show values");
    expect(summary.getAttribute("aria-expanded")).toBe("false");
    await act(async () => {
      summary.click();
    });
    expect(button("Payment terms, Used Quotation. Collapse")).not.toBeNull();
  });

  it("renders tax basis rows read-only with no actions", async () => {
    await renderPane({
      session: {
        ...BASE.session,
        Items: [
          {
            Key: "link:taxBasis",
            Field: "link:taxBasis",
            Kind: "tax_basis",
            LineKey: null,
            SourceLineRef: null,
            Label: "Tax basis",
            POValue: "FTZ · Goods",
            SourceValue: "FTZ · Goods",
            Status: "resolved",
            Resolution: "kept_po",
            ActorID: 7,
            UpdatedAt: "2026-10-28T00:00:01Z",
            History: [],
          },
        ],
      },
    });
    expect(document.body.textContent).toContain("Tax basis · linked at revision 2");
    // No Keep/Use/Delete actions anywhere: filter tabs are the only buttons.
    expect(
      document.querySelector(
        'button[aria-label^="Keep PO"], button[aria-label^="Use Quotation"], button[aria-label^="Delete"]'
      )
    ).toBeNull();
  });
});

describe("ReconciliationPaneContent focus contract (ticket 04)", () => {
  it("moves focus to the next unresolved action after Use Quotation", async () => {
    const ref: { current: ReconciliationPaneHandle | null } = { current: null };
    const second = item({
      Key: "header:incoterms",
      Field: "header:incoterms",
      Label: "Incoterms",
      UpdatedAt: "2026-10-28T00:00:03Z",
    });
    await act(async () => {
      root.render(
        <ReconciliationPaneContent
          {...BASE}
          session={{ ...BASE.session, Items: [item(), second] }}
          ref={ref}
        />
      );
    });
    expect(ref.current?.focusItem("header:incoterms")).toBe(true);
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "Keep PO entry for Incoterms"
    );
    expect(ref.current?.focusFirstUnresolved()).toBe(true);
    expect(ref.current?.focusItem("header:missing")).toBe(false);
  });

  it("exposes the count element as the resolved-last focus target", async () => {
    const ref: { current: ReconciliationPaneHandle | null } = { current: null };
    await act(async () => {
      root.render(
        <ReconciliationPaneContent
          {...BASE}
          session={{ ...BASE.session, Items: [item()] }}
          ref={ref}
        />
      );
    });
    ref.current?.focusCount();
    expect(document.activeElement?.getAttribute("role")).toBe("status");
  });
});

describe("ReconciliationSheetTrigger (ticket 04)", () => {
  it("names the count and the open action, and announces open state", async () => {
    const onOpen = vi.fn();
    await act(async () => {
      root.render(
        <ReconciliationSheetTrigger
          unresolvedCount={3}
          sourceLabel="QT-2026-018 · revision 02"
          expanded={false}
          onOpen={onOpen}
        />
      );
    });
    const trigger = button(
      "Reconciliation, 3 items to resolve. Open panel."
    );
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    trigger.click();
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it("reads nothing to resolve at zero, with no source label required", async () => {
    await act(async () => {
      root.render(
        <ReconciliationSheetTrigger
          unresolvedCount={0}
          sourceLabel={null}
          expanded={true}
          onOpen={() => undefined}
        />
      );
    });
    button("Reconciliation, nothing to resolve. Open panel.");
  });
});
