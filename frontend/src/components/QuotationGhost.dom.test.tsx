// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { SourceItem } from "../poSourceItems";
import { FEATURES_TUTORIAL_ADVANCE_EVENT } from "./FeaturesTutorial";
import { QuotationGhost } from "./QuotationGhost";

// Issue #606: the ghost's two buttons are the tutorial's action group. In
// tutorial mode they carry the anchor the ghost lesson targets, and either
// action reports the same resolution event; outside the tutorial they are
// plain buttons and must not talk to the guide at all.

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const SPLIT_ITEM: SourceItem = {
  itemNo: 2,
  label: "2",
  description: "Tutorial split package (MOCK)",
  uom: "Pc",
  quantityScaled: 10_000,
  unitPriceMinor: 0,
  discountEnabled: false,
  discountPercent: 0,
  descriptionLines: [],
  subItems: [
    {
      key: "2.1",
      label: "2.1",
      subNo: 1,
      description: "Tutorial split part A (MOCK)",
      uom: "Pc",
      quantityScaled: 20_000,
      unitPriceMinor: 5_500_000,
      discountEnabled: false,
      discountPercent: 0,
      descriptionLines: ["Tutorial scope A1"],
    },
  ],
};

let container: HTMLDivElement;
let root: Root;
let eventKeys: string[];
let stopListening: () => void;

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (found === null) throw new Error(`no button labelled ${label}`);
  return found;
}

beforeEach(() => {
  Element.prototype.scrollIntoView = () => undefined;
  eventKeys = [];
  const listener = (event: Event) => {
    eventKeys.push(
      (event as CustomEvent<{ key: string }>).detail?.key ?? "missing-detail"
    );
  };
  window.addEventListener(FEATURES_TUTORIAL_ADVANCE_EVENT, listener);
  stopListening = () =>
    window.removeEventListener(FEATURES_TUTORIAL_ADVANCE_EVENT, listener);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  stopListening();
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

describe("QuotationGhost actions (issue #606)", () => {
  it("carries the tutorial action anchor and resolves from either choice", () => {
    const onAccept = vi.fn();
    const onReject = vi.fn();
    act(() =>
      root.render(
        <QuotationGhost
          item={SPLIT_ITEM}
          currency={IDR}
          tutorial
          onAccept={onAccept}
          onReject={onReject}
        />
      )
    );

    const actions = document.querySelector('[data-tutorial="po-ghost-actions"]');
    if (actions === null) throw new Error("no ghost action group");
    expect(actions.querySelectorAll("button")).toHaveLength(2);

    act(() => button("Use quotation 2").click());
    expect(onAccept).toHaveBeenCalledTimes(1);
    expect(eventKeys).toEqual(["po-ghost-resolved"]);

    act(() => button("Ignore quotation 2").click());
    expect(onReject).toHaveBeenCalledTimes(1);
    expect(eventKeys).toEqual(["po-ghost-resolved", "po-ghost-resolved"]);
  });

  it("stays silent and unanchored outside the tutorial", () => {
    const onAccept = vi.fn();
    const onReject = vi.fn();
    act(() =>
      root.render(
        <QuotationGhost
          item={SPLIT_ITEM}
          currency={IDR}
          onAccept={onAccept}
          onReject={onReject}
        />
      )
    );

    expect(
      document.querySelector('[data-tutorial="po-ghost-actions"]')
    ).toBeNull();

    act(() => button("Use quotation 2").click());
    act(() => button("Ignore quotation 2").click());

    expect(onAccept).toHaveBeenCalledTimes(1);
    expect(onReject).toHaveBeenCalledTimes(1);
    expect(eventKeys).toEqual([]);
  });
});
