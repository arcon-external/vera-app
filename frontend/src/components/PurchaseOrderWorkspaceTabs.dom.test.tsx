// @vitest-environment jsdom
import { useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PurchaseOrderWorkspacePage } from "./PurchaseOrderWorkspace";
import type { PurchaseOrderWorkspace } from "../poWorkspace";
import type { CurrencyInfo } from "../format";

// Issue #491: Drafts and Archive are the same list component with a different
// pinned state. Switching tabs must re-initialize that list; reusing the Drafts
// reducer would leave the Archive tab querying drafts. The binding is replaced
// at its module boundary, so the assertion is the request the server receives.

const { listMock } = vi.hoisted(() => ({
  listMock: vi.fn(async (..._args: unknown[]) => ({
    PurchaseOrders: [],
    Customers: [],
    Page: 1,
    PageSize: 15,
    Total: 0,
    TotalPages: 1,
  })),
}));

vi.mock("@backend/backend/Quotation", () => ({
  ListPurchaseOrders: (...args: unknown[]) => listMock(...args),
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

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

function Harness() {
  const [tab, setTab] = useState<"editor" | "drafts" | "archive">("drafts");
  return (
    <>
      <button type="button" onClick={() => setTab("archive")}>
        Switch to Archive
      </button>
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
    </>
  );
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  listMock.mockClear();
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

function stateOfCall(index: number): string {
  const call = listMock.mock.calls[index] as unknown as [{ State: string }];
  return call[0].State;
}

describe("PO workspace tabs pin their own stored state (issue #491)", () => {
  it("queries drafts, then saved after switching to Archive", async () => {
    await act(async () => {
      root.render(<Harness />);
    });
    await flush();
    expect(listMock).toHaveBeenCalled();
    expect(stateOfCall(0)).toBe("draft");

    await act(async () => {
      document.querySelector<HTMLButtonElement>("button")?.click();
    });
    await flush();
    expect(stateOfCall(listMock.mock.calls.length - 1)).toBe("saved");
  });
});
