// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  POSourcePage,
  POSourceQuery,
  POSourceRevision,
  POSourceSelection,
  POSourceValidation,
} from "../poSelector";
import { POQuotationSelector } from "./POQuotationSelector";

// Issue #467, wave 3: the runtime interaction regression the SSR suite cannot
// reach. jsdom drives the real container -- effects, focus and keyboard -- so
// the two defects the Judge reproduced in Chrome have a committed test.
//
// jsdom does not move focus on programmatic pointer input the way a browser
// does. These tests focus a control before clicking it, which is the browser's
// order (mousedown focuses, click activates) and is what makes "the focused
// control was removed" reproducible.

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const QUOTATION = {
  id: 12,
  quoteNumber: "1502260001",
  subject: "Electrical installation",
  customerName: "PT. Infineon Technologies Batam",
  latestRevision: 1,
  issuedRevisionCount: 2,
  hasActivePO: false,
};

const OTHER = {
  id: 13,
  quoteNumber: "1502260002",
  subject: "Mechanical works",
  customerName: "PT. H-Tech Oilfield Equipment",
  latestRevision: 0,
  issuedRevisionCount: 1,
  hasActivePO: false,
};

const TWO_REVISIONS: POSourceRevision[] = [
  { revision: 1, issuedAt: "2026-10-20T10:00:00+07:00" },
  { revision: 0, issuedAt: "2026-10-18T10:00:00+07:00" },
];

function page(
  quotations = [QUOTATION, OTHER],
  pageNumber = 1,
  totalPages = 1
): POSourcePage {
  return {
    quotations,
    page: pageNumber,
    pageSize: 15,
    total: quotations.length,
    totalPages,
  };
}

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
}

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((fulfil) => {
    resolve = fulfil;
  });
  return { promise, resolve };
}

interface HarnessProps {
  initialOpen?: boolean;
  loadPage?: (query: POSourceQuery) => Promise<POSourcePage>;
  loadRevisions?: (quotationId: number) => Promise<POSourceRevision[]>;
  validate?: (selection: POSourceSelection) => Promise<POSourceValidation>;
  onConfirm?: (selection: POSourceSelection) => void;
}

function Harness({
  initialOpen = true,
  loadPage = async () => page(),
  loadRevisions = async () => TWO_REVISIONS,
  validate = async () => ({ valid: true }),
  onConfirm = () => {},
}: HarnessProps) {
  const [open, setOpen] = useState(initialOpen);
  return (
    <>
      <button type="button" data-testid="opener" onClick={() => setOpen(true)}>
        Open selector
      </button>
      <button type="button" data-testid="external-close" onClick={() => setOpen(false)}>
        Close selector externally
      </button>
      <POQuotationSelector
        open={open}
        onClose={() => setOpen(false)}
        onConfirm={onConfirm}
        loadPage={loadPage}
        loadRevisions={loadRevisions}
        validate={validate}
      />
    </>
  );
}

let container: HTMLDivElement;
let root: Root;
let mounted: boolean;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  mounted = true;
});

afterEach(() => {
  if (mounted) {
    act(() => root.unmount());
  }
  container.remove();
  document.body.innerHTML = "";
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function renderHarness(props: HarnessProps = {}): Promise<void> {
  await act(async () => {
    root.render(<Harness {...props} />);
  });
  await flush();
}

function dialog(): HTMLElement {
  const element = document.querySelector<HTMLElement>('[role="dialog"]');
  if (element === null) throw new Error("the selector dialog is not rendered");
  return element;
}

function button(label: string): HTMLButtonElement {
  const element = Array.from(document.querySelectorAll("button")).find(
    (candidate) =>
      candidate.getAttribute("aria-label") === label ||
      candidate.textContent?.trim() === label
  );
  if (element === undefined) throw new Error(`no button labelled "${label}"`);
  return element;
}

async function click(element: HTMLElement): Promise<void> {
  await act(async () => {
    element.focus();
    element.click();
  });
  await flush();
}

async function press(key: string, shiftKey = false): Promise<void> {
  const target = document.activeElement;
  if (!(target instanceof HTMLElement)) {
    throw new Error("no active element to dispatch the key from");
  }
  await act(async () => {
    target.dispatchEvent(
      new KeyboardEvent("keydown", {
        key,
        shiftKey,
        bubbles: true,
        cancelable: true,
      })
    );
  });
}

/** Step 1 -> step 2 with the first revision chosen, leaving Continue enabled. */
async function chooseQuotationAndRevision(): Promise<void> {
  await click(button(`Quotation ${QUOTATION.quoteNumber}, ${QUOTATION.customerName}`));
  await click(button("Revision 0"));
}

/** The three-step path every confirmation test starts from. */
async function reachConfirmation(): Promise<void> {
  await chooseQuotationAndRevision();
  await click(button("Continue to confirmation"));
  expect(document.body.textContent).toContain("Step 3 of 3");
}

describe("POQuotationSelector interaction (issue #467 wave 3)", () => {
  it("moves focus into the dialog when it opens", async () => {
    await renderHarness();
    expect(document.activeElement).toBe(dialog());
  });

  it("restores focus to the opener when the dialog closes", async () => {
    await renderHarness({ initialOpen: false });
    const opener = button("Open selector");
    await click(opener);
    expect(document.activeElement).toBe(dialog());

    await click(button("Cancel"));
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it("keeps focus inside the dialog after selecting a quotation", async () => {
    await renderHarness();
    await click(button(`Quotation ${QUOTATION.quoteNumber}, ${QUOTATION.customerName}`));

    expect(document.body.textContent).toContain("Step 2 of 3");
    // The clicked row is gone; the container must have taken focus rather
    // than leaving it on <body>, where Escape cannot reach the dialog.
    expect(document.activeElement).not.toBe(document.body);
    expect(dialog().contains(document.activeElement)).toBe(true);
  });

  it("closes on Escape after a quotation selection", async () => {
    await renderHarness();
    await click(button(`Quotation ${QUOTATION.quoteNumber}, ${QUOTATION.customerName}`));

    await press("Escape");
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it("keeps focus inside the dialog after continuing to confirmation", async () => {
    await renderHarness();
    await chooseQuotationAndRevision();
    await click(button("Continue to confirmation"));

    expect(document.body.textContent).toContain("Step 3 of 3");
    expect(document.activeElement).not.toBe(document.body);
    expect(dialog().contains(document.activeElement)).toBe(true);

    await press("Escape");
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it("wraps Tab and Shift+Tab inside the dialog", async () => {
    await renderHarness({ loadPage: async () => page([QUOTATION, OTHER], 1, 2) });
    expect(document.activeElement).toBe(dialog());

    // Forward from the container lands on the first stop (header Cancel).
    await press("Tab");
    expect(document.activeElement).toBe(button("Cancel"));

    // Shift+Tab from the first stop wraps to the last (Next page).
    await press("Tab", true);
    expect(document.activeElement).toBe(button("Next page"));

    // Forward from the last stop wraps back to the first.
    await press("Tab");
    expect(document.activeElement).toBe(button("Cancel"));
  });

  it("does not deliver a selection confirmed after Cancel", async () => {
    const validation = deferred<POSourceValidation>();
    const onConfirm = vi.fn();
    await renderHarness({
      validate: () => validation.promise,
      onConfirm,
    });
    await reachConfirmation();

    await click(button("Confirm source"));
    expect(onConfirm).not.toHaveBeenCalled();
    // The busy Confirm button becomes disabled while it holds focus. Focus
    // must move back to the dialog rather than sit on a dead control.
    expect(document.activeElement).toBe(dialog());

    await click(button("Cancel"));
    expect(document.querySelector('[role="dialog"]')).toBeNull();

    await act(async () => {
      validation.resolve({ valid: true });
      await validation.promise;
    });
    await flush();

    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("does not deliver a stale selection after Cancel and reopen", async () => {
    const validation = deferred<POSourceValidation>();
    const onConfirm = vi.fn();
    await renderHarness({
      validate: () => validation.promise,
      onConfirm,
    });
    await reachConfirmation();

    await click(button("Confirm source"));
    await click(button("Cancel"));
    await click(button("Open selector"));
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();

    await act(async () => {
      validation.resolve({ valid: true });
      await validation.promise;
    });
    await flush();

    // The modal is open again, so "still open" alone would accept this
    // answer; only the request token can tell it belongs to the closed pass.
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("does not deliver a selection when the dialog closes externally while confirming", async () => {
    const validation = deferred<POSourceValidation>();
    const onConfirm = vi.fn();
    await renderHarness({
      validate: () => validation.promise,
      onConfirm,
    });
    await reachConfirmation();

    await click(button("Confirm source"));
    await click(button("Close selector externally"));

    await act(async () => {
      validation.resolve({ valid: true });
      await validation.promise;
    });
    await flush();

    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("does not deliver a selection confirmed after unmount", async () => {
    const validation = deferred<POSourceValidation>();
    const onConfirm = vi.fn();
    await renderHarness({
      validate: () => validation.promise,
      onConfirm,
    });
    await reachConfirmation();

    await click(button("Confirm source"));
    act(() => root.unmount());
    mounted = false;

    await act(async () => {
      validation.resolve({ valid: true });
      await validation.promise;
    });

    expect(onConfirm).not.toHaveBeenCalled();
  });
});
