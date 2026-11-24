// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  QUOTATION_CLOSE_REASON_MAX,
  QuotationCloseDialog,
  closeReasonError,
} from "./QuotationCloseDialog";

// Issue #656: closing is an admin decision with a mandatory, bounded reason.
// The dialog must not close optimistically: a server refusal has to leave the
// typed reason and the dialog on screen so the admin can read the message and
// retry (or cancel).

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

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function dialog(): HTMLElement | null {
  return document.querySelector<HTMLElement>('[role="dialog"]');
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

function textarea(): HTMLTextAreaElement {
  const element = document.querySelector<HTMLTextAreaElement>(
    'textarea[aria-label="Close reason"]'
  );
  if (element === null) throw new Error("the close reason textarea is not rendered");
  return element;
}

function type(value: string): void {
  const field = textarea();
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value"
  )?.set;
  setter?.call(field, value);
  act(() => {
    field.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

interface HarnessProps {
  isAdmin?: boolean;
  pending?: boolean;
  error?: string | null;
  onConfirm?: (reason: string) => Promise<void> | void;
}

function Harness({
  isAdmin = true,
  pending = false,
  error = null,
  onConfirm = () => {},
}: HarnessProps) {
  const [open, setOpen] = useState(true);
  // The server round trip is the caller's, so the harness owns the busy flag
  // exactly as the page does: it turns on at submit and off when the request
  // settles, while the dialog stays open throughout.
  const [busy, setBusy] = useState(false);
  return (
    <QuotationCloseDialog
      open={open}
      isAdmin={isAdmin}
      quotationNumber="1502260001"
      pending={pending || busy}
      error={error}
      onCancel={() => setOpen(false)}
      onConfirm={(reason) => {
        setBusy(true);
        void (async () => {
          try {
            await onConfirm(reason);
          } finally {
            setBusy(false);
          }
        })();
      }}
    />
  );
}

describe("QuotationCloseDialog (issue #656)", () => {
  it("requires a non-blank reason before confirming", () => {
    const onConfirm = vi.fn();
    act(() => root.render(<Harness onConfirm={onConfirm} />));

    expect(button("Confirm close quotation").disabled).toBe(true);
    type("   \n  ");
    expect(button("Confirm close quotation").disabled).toBe(true);
    type("customer cancelled the remainder");
    expect(button("Confirm close quotation").disabled).toBe(false);
    act(() => button("Confirm close quotation").click());
    expect(onConfirm).toHaveBeenCalledWith("customer cancelled the remainder");
  });

  it("bounds the reason at 500 characters, counting characters not bytes", () => {
    act(() => root.render(<Harness />));
    type("界".repeat(QUOTATION_CLOSE_REASON_MAX));
    expect(button("Confirm close quotation").disabled).toBe(false);

    type("界".repeat(QUOTATION_CLOSE_REASON_MAX + 1));
    expect(button("Confirm close quotation").disabled).toBe(true);
    expect(container.textContent).toContain("500");
  });

  it("never renders for a non-admin, even if the parent asks", () => {
    act(() => root.render(<Harness isAdmin={false} />));
    expect(dialog()).toBeNull();
    expect(container.textContent).toBe("");
  });

  it("stays open and keeps the reason while the server answers", async () => {
    let settle: (() => void) | null = null;
    const onConfirm = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          settle = resolve;
        })
    );
    act(() => root.render(<Harness onConfirm={onConfirm} />));
    type("customer cancelled");
    act(() => button("Confirm close quotation").click());

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(dialog()).not.toBeNull();
    expect(textarea().value).toBe("customer cancelled");
    const submit = button("Confirm close quotation");
    expect(submit.disabled).toBe(true);
    expect(submit.textContent).toContain("Closing…");

    await act(async () => {
      settle?.();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    // The parent owns closing; a resolved promise alone does not dismiss it.
    expect(dialog()).not.toBeNull();
  });

  it("shows the server's refusal without losing the typed reason", () => {
    act(() =>
      root.render(
        <Harness error="This quotation is already closed." pending={false} />
      )
    );
    type("customer cancelled");
    expect(container.textContent).toContain("This quotation is already closed.");
    expect(textarea().value).toBe("customer cancelled");
    expect(dialog()).not.toBeNull();
  });
});

describe("closeReasonError (issue #656)", () => {
  it("names the blank reason and the 500-character bound", () => {
    expect(closeReasonError("")).toBe("Give a reason for closing this quotation.");
    expect(closeReasonError("   ")).toBe("Give a reason for closing this quotation.");
    expect(closeReasonError("ok")).toBeNull();
    expect(closeReasonError("界".repeat(QUOTATION_CLOSE_REASON_MAX))).toBeNull();
    expect(closeReasonError("界".repeat(QUOTATION_CLOSE_REASON_MAX + 1))).toContain(
      "500"
    );
  });
});
