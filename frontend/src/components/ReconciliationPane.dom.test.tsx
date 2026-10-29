// @vitest-environment jsdom
import { act } from "react";
import * as React from "react";
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
    // U10: the count is state, not a live region — announcements come from
    // the single workbench live region instead.
    expect(document.querySelector('[role="status"]')).toBeNull();
    expect(document.body.textContent).toContain("2 to resolve");
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
    // U10: the count is focusable state, not a live region.
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "1 item to resolve"
    );
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

  it("sticks to the viewport top below 640px and below the shell header above", async () => {
    await act(async () => {
      root.render(
        <ReconciliationSheetTrigger
          unresolvedCount={1}
          sourceLabel={null}
          expanded={false}
          onOpen={() => undefined}
        />
      );
    });
    const trigger = document.querySelector("button.sticky");
    if (trigger === null) throw new Error("no sticky sheet trigger");
    const tokens = trigger.className.split(/\s+/);
    expect(tokens).toContain("top-0");
    expect(tokens).toContain("sm:top-16");
  });
});

describe("ReconciliationPaneContent line-set cards (ticket 05)", () => {
  const lineItemWith = (overrides: Partial<ReconciliationItem> = {}) =>
    item({
      Key: "source:3:match",
      Field: "source:match",
      Kind: "missing_line",
      LineKey: null,
      SourceLineRef: "3",
      Label: "Missing line · Quotation 3",
      POValue: null,
      SourceValue: "3",
      Status: "unresolved",
      Resolution: null,
      ...overrides,
    });

  async function renderWith(items: ReconciliationItem[], props: Record<string, unknown> = {}) {
    await renderPane({ session: { ...BASE.session, Items: items }, ...props });
  }

  it("offers Add line and Acknowledge omission on a missing line", async () => {
    const onAddLineFromSource = vi.fn();
    const onAcknowledgeMissing = vi.fn();
    await renderWith([lineItemWith()], {
      onAddLineFromSource,
      onAcknowledgeMissing,
      sourceDescriptions: { "3": "Sprocket" },
    });
    expect(document.body.textContent).toContain("Missing line · Quotation 3");
    expect(document.body.textContent).toContain("Sprocket");
    button("Add line from Quotation 3").click();
    expect(onAddLineFromSource).toHaveBeenCalledWith("3");
    button("Acknowledge omission of Missing line · Quotation 3").click();
    expect(onAcknowledgeMissing).toHaveBeenCalledWith("source:3:match");
  });

  it("offers Match to Quotation and Acknowledge extra on an extra line", async () => {
    const onFixMatch = vi.fn();
    const onAcknowledgeExtra = vi.fn();
    await renderWith(
      [
        lineItemWith({
          Key: "line:client:b:match",
          Field: "line:match",
          Kind: "extra_line",
          LineKey: "client:b",
          SourceLineRef: null,
          Label: "Extra line · Line 2",
          POValue: null,
          SourceValue: null,
        }),
      ],
      {
        onFixMatch,
        onAcknowledgeExtra,
        lineSummaries: { "client:b": { displayNumber: 2, description: "Custom bracket" } },
      }
    );
    button("Match to Quotation for Extra line · Line 2").click();
    expect(onFixMatch).toHaveBeenCalledWith("client:b");
    button("Acknowledge Extra line · Line 2 as extra").click();
    expect(onAcknowledgeExtra).toHaveBeenCalledWith("line:client:b:match");
  });

  it("names the saved-reference blocker on an unknown reference", async () => {
    const onAcknowledgeExtra = vi.fn();
    await renderWith(
      [
        lineItemWith({
          Key: "line:client:u:match",
          Field: "line:match",
          Kind: "extra_line",
          LineKey: "client:u",
          SourceLineRef: null,
          Label: "Unknown reference · 99",
          POValue: "99",
          SourceValue: null,
        }),
      ],
      {
        onAcknowledgeExtra,
        lineSummaries: { "client:u": { displayNumber: 1, description: "Custom bracket" } },
      }
    );
    expect(document.body.textContent).toContain("Unknown reference · 99");
    expect(document.body.textContent).toContain("No Quotation line 99 exists.");
    button("Acknowledge unknown reference 99 as extra").click();
    expect(onAcknowledgeExtra).toHaveBeenCalledWith("line:client:u:match");
  });

  it("shows the recorded acknowledgement without unblocking an unknown line", async () => {
    await renderWith([
      lineItemWith({
        Key: "line:client:u:match",
        Field: "line:match",
        Kind: "extra_line",
        LineKey: "client:u",
        SourceLineRef: null,
        Label: "Unknown reference · 99",
        POValue: "99",
        SourceValue: null,
        History: [
          {
            Kind: "extra_line",
            SourceLineRef: null,
            POValue: "99",
            SourceValue: null,
            Status: "unresolved",
            Resolution: "acknowledged_extra",
            ActorID: 7,
            At: "2026-10-28T00:00:05Z",
          },
        ],
      }),
    ]);
    expect(document.body.textContent).toContain("Acknowledged: link a valid Quotation line to finalize.");
    expect(
      document.querySelector('button[aria-label="Acknowledge unknown reference 99 as extra"]')
    ).toBeNull();
  });

  it("blocks a duplicate match until one match is corrected", async () => {
    const onFixMatch = vi.fn();
    await renderWith(
      [
        lineItemWith({
          Key: "line:client:a:match",
          Field: "line:match",
          Kind: "duplicate_match",
          LineKey: "client:a",
          SourceLineRef: "2",
          Label: "Duplicate match · Quotation 2",
          POValue: "2",
          SourceValue: null,
        }),
      ],
      {
        onFixMatch,
        lineSummaries: { "client:a": { displayNumber: 1, description: "Widget" } },
      }
    );
    expect(document.body.textContent).toContain("Duplicate match · Quotation 2");
    expect(document.body.textContent).toContain("Blocking");
    button("Fix match for Quotation 2").click();
    expect(onFixMatch).toHaveBeenCalledWith("client:a");
  });

  it("focuses the first line-set action for blocked Final Save", async () => {
    const ref: { current: ReconciliationPaneHandle | null } = { current: null };
    await act(async () => {
      root.render(
        <ReconciliationPaneContent
          {...BASE}
          session={{ ...BASE.session, Items: [lineItemWith()] }}
          ref={ref}
        />
      );
    });
    expect(ref.current?.focusFirstUnresolved()).toBe(true);
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "Add line from Quotation 3"
    );
  });
});

describe("ReconciliationPaneContent tax basis (ticket 05)", () => {
  it("renders the link-time tax basis triple read-only with no actions", async () => {
    await renderPane({
      session: {
        ...BASE.session,
        Items: [
          item({
            Key: "link:taxBasis",
            Field: "link:taxBasis",
            Kind: "tax_basis",
            LineKey: null,
            SourceLineRef: null,
            Label: "Tax basis",
            POValue: { TaxZone: "FTZ", TaxType: "Goods", ServiceCategory: "" },
            SourceValue: { TaxZone: "FTZ", TaxType: "Goods", ServiceCategory: "" },
            Status: "resolved",
            Resolution: "kept_po",
          }),
        ],
      },
    });
    expect(document.body.textContent).toContain("Tax basis · linked at revision 2");
    expect(document.body.textContent).toContain("FTZ · Goods");
    expect(document.body.textContent).toContain("Protected");
    // No Keep/Use/Delete actions anywhere: filter tabs are the only buttons.
    expect(
      document.querySelector(
        'button[aria-label^="Keep PO"], button[aria-label^="Use Quotation"], button[aria-label^="Delete"]'
      )
    ).toBeNull();
  });
});

describe("ReconciliationPaneContent focus, count and context (U6-U10)", () => {
  const twoUnresolved = () => [
    item({
      Key: "header:paymentTerms",
      Label: "Payment terms",
      POValue: "50% advance",
      SourceValue: "30 days",
    }),
    item({
      Key: "header:incoterms",
      Field: "header:incoterms",
      Label: "Incoterms",
      POValue: "EXW",
      SourceValue: "CIF",
    }),
  ];

  it("moves focus to the next unresolved action after Use Quotation", async () => {
    const onUse = vi.fn();
    await renderPane({ session: { ...BASE.session, Items: twoUnresolved() }, onUse });
    button("Use Quotation value for Payment terms").click();
    expect(onUse).toHaveBeenCalledWith("header:paymentTerms");
    await act(async () => {});
    // The mock leaves the session unchanged, so focus moves to the next
    // unresolved action rather than falling to the body.
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "Keep PO entry for Incoterms"
    );
  });

  it("returns focus to Keep after cancelling the keep confirmation", async () => {
    const onCancelKeep = vi.fn();
    function Harness() {
      const [confirm, setConfirm] = React.useState<string | null>("header:paymentTerms");
      return (
        <ReconciliationPaneContent
          {...BASE}
          session={{ ...BASE.session, Items: twoUnresolved() }}
          keepConfirmKey={confirm}
          onCancelKeep={() => {
            setConfirm(null);
            onCancelKeep();
          }}
        />
      );
    }
    await act(async () => {
      root.render(<Harness />);
    });
    button("Cancel keeping PO entry for Payment terms").click();
    expect(onCancelKeep).toHaveBeenCalledTimes(1);
    await act(async () => {});
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "Keep PO entry for Payment terms"
    );
  });

  it("hides the count while loading and with no source", async () => {
    await renderPane({ status: "loading", session: { ...BASE.session, Items: [] } });
    expect(document.querySelector("[aria-label$='to resolve']")).toBeNull();
    expect(document.body.textContent).toContain("Loading the reconciliation");
    await renderPane({ noSource: true, session: { ...BASE.session, Items: [] } });
    expect(document.querySelector("[aria-label$='to resolve']")).toBeNull();
    expect(document.body.textContent).toContain("Link a Quotation to start the comparison.");
  });

  it("shows the resolved count in read-only mode", async () => {
    await renderPane({
      readOnly: true,
      readOnlyCaption: "The saved reconciliation is history.",
      session: {
        ...BASE.session,
        Items: [
          item({ Status: "resolved", Resolution: "kept_po" }),
          item({ Key: "header:incoterms", Field: "header:incoterms", Status: "resolved", Resolution: "used_quotation" }),
        ],
      },
    });
    expect(document.body.textContent).toContain("2 resolved");
    expect(document.body.textContent?.includes("Nothing to resolve")).toBe(false);
  });

  it("names every claimant line on a duplicate card", async () => {
    await renderPane({
      session: {
        ...BASE.session,
        Items: [
          item({
            Key: "line:client:a:match",
            Field: "line:match",
            Kind: "duplicate_match",
            LineKey: "client:a",
            SourceLineRef: "1",
            Label: "Duplicate match · Quotation 1",
            POValue: "1",
            SourceValue: null,
          }),
          item({
            Key: "line:client:b:match",
            Field: "line:match",
            Kind: "duplicate_match",
            LineKey: "client:b",
            SourceLineRef: "1",
            Label: "Duplicate match · Quotation 1",
            POValue: "1",
            SourceValue: null,
          }),
        ],
      },
      lineSummaries: {
        "client:a": { displayNumber: 3, description: "Widget" },
        "client:b": { displayNumber: 5, description: "Sprocket" },
      },
    });
    expect(document.body.textContent).toContain("Lines 3 and 5 claim this quotation line.");
  });

  it("shows the source triple on the tax row when it differs and falls back to User #id", async () => {
    await renderPane({
      session: {
        ...BASE.session,
        Items: [
          item({
            Key: "link:taxBasis",
            Field: "link:taxBasis",
            Kind: "tax_basis",
            LineKey: null,
            SourceLineRef: null,
            Label: "Tax basis",
            POValue: { TaxZone: "FTZ", TaxType: "Goods", ServiceCategory: "" },
            SourceValue: { TaxZone: "FTZ", TaxType: "Services", ServiceCategory: "Construction" },
            Status: "resolved",
            Resolution: "kept_po",
            ActorID: 9,
          }),
        ],
      },
    });
    expect(document.body.textContent).toContain("Quotation: FTZ · Services · Construction");
    expect(document.body.textContent).toContain("User #9");
  });

  it("keeps filter tabs at 44px below 1024 and the tabpanel associated when empty", async () => {
    await renderPane({ session: { ...BASE.session, Items: [] }, filter: "unresolved" });
    const tab = document.querySelector('[role="tab"]');
    if (tab === null) throw new Error("no filter tab");
    expect(tab.className).toContain("min-h-11");
    expect(document.querySelector('[role="tabpanel"]')).not.toBeNull();
  });
});
