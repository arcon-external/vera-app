// @vitest-environment jsdom
import { act } from "react";
import type { ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import { purchaseOrderReportYearFixture } from "../poReportFixtures";
import { buildReportChart, layoutReportChart } from "../poReportChart";
import { PurchaseOrderReportChart } from "./PurchaseOrderReportChart";

// Issue #644: the overview embeds the same chart without a second card, heading
// or period toggle. The standalone props stay source-compatible, so this file
// pins only the embedded presentation: one compact forecast status, one native
// Details region, and the full SVG/accessible description mounted outside it.

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

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    right: 960,
    bottom: 300,
    width: 960,
    height: 300,
    toJSON: () => ({}),
  });
  window.matchMedia = ((query: string) => ({
    matches: query.includes("prefers-reduced-motion"),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function render(
  element: ReactElement = (
    <PurchaseOrderReportChart
      report={purchaseOrderReportYearFixture()}
      currency={IDR}
      period="year"
      embedded
    />
  )
): Promise<void> {
  await act(async () => {
    root.render(element);
  });
  await flush();
}

function chart(): HTMLElement {
  const found = container.querySelector<HTMLElement>("[data-report-chart]");
  if (found === null) throw new Error("no chart");
  return found;
}

function svg(): SVGSVGElement {
  const found = container.querySelector<SVGSVGElement>("svg[data-chart-svg]");
  if (found === null) throw new Error("no chart svg");
  return found;
}

function surface(): HTMLElement {
  const found = container.querySelector<HTMLElement>("[data-chart-surface]");
  if (found === null) throw new Error("no chart surface");
  return found;
}

function pointer(type: string, clientX: number): Event {
  const event = new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    clientX,
  });
  Object.defineProperty(event, "pointerType", { value: "mouse" });
  return event;
}

function key(target: Element, value: string): void {
  act(() => {
    target.dispatchEvent(
      new KeyboardEvent("keydown", { key: value, bubbles: true })
    );
  });
}

describe("embedded Purchase order performance chart (issue #644)", () => {
  it("renders no card heading or period toggle, only the chart, one status and Details", async () => {
    await render();

    expect(chart().getAttribute("data-report-chart-embedded")).toBe("");
    expect(container.querySelector("h2")).toBeNull();
    expect(container.querySelector("[data-report-period-toggle]")).toBeNull();
    expect(container.querySelectorAll("[data-report-chart]")).toHaveLength(1);
    expect(container.querySelectorAll("[data-report-forecast-status]")).toHaveLength(
      1
    );
    expect(container.querySelector('[aria-label="Chart legend"]')).not.toBeNull();
    expect(svg()).not.toBeNull();
    const details = container.querySelector<HTMLDetailsElement>("details");
    expect(details).not.toBeNull();
    expect(details?.open).toBe(false);
    expect(details?.querySelector("summary")?.textContent).toContain(
      "Method and chart details"
    );
    expect(
      container.querySelector('a[href="#report-period-table"]')
    ).not.toBeNull();
  });

  it("does not render its own toggle even when a callback is supplied", async () => {
    const onPeriodChange = vi.fn();
    await render(
      <PurchaseOrderReportChart
        report={purchaseOrderReportYearFixture()}
        currency={IDR}
        period="year"
        onPeriodChange={onPeriodChange}
        embedded
      />
    );

    expect(container.querySelector("[data-report-period-toggle]")).toBeNull();
    expect(onPeriodChange).not.toHaveBeenCalled();
  });

  it("keeps the full accessible title, caption and description outside the collapsed Details", async () => {
    await render();

    const details = container.querySelector<HTMLDetailsElement>("details");
    const labelledBy = svg().getAttribute("aria-labelledby") ?? "";
    const describedBy = svg().getAttribute("aria-describedby") ?? "";
    expect(labelledBy.trim()).not.toBe("");
    expect(describedBy.trim()).not.toBe("");

    for (const id of [...labelledBy.split(/\s+/), ...describedBy.split(/\s+/)]) {
      const target = document.getElementById(id);
      expect(target, `missing accessible reference ${id}`).not.toBeNull();
      expect(target?.closest("details")).toBeNull();
      expect(target?.closest("[hidden]")).toBeNull();
      expect(target?.getAttribute("aria-hidden")).not.toBe("true");
    }

    const caption = document.getElementById(labelledBy.split(/\s+/)[1] ?? "");
    expect(caption?.className).toContain("sr-only");
    expect(caption?.textContent).toContain("12 POs across 11 actual monthly periods");
    expect(svg().querySelector("title")?.textContent).toBe(
      "Purchase order performance chart"
    );
    const description = svg().querySelector("desc");
    expect(description?.textContent).toContain(
      "Grouped columns show amount, capital and margin"
    );
    expect(description?.textContent).toContain(
      "never included in actual totals"
    );
    expect(details?.contains(description ?? null)).toBe(false);
  });

  it("opens Details to the method, notes and full ranges without rebuilding the chart", async () => {
    await render();
    const marks = svg().querySelector("[data-chart-marks]");
    expect(marks).not.toBeNull();

    const details = container.querySelector<HTMLDetailsElement>("details");
    const summary = details?.querySelector("summary");
    await act(async () => {
      summary?.click();
    });
    await flush();

    expect(details?.open).toBe(true);
    const text = details?.textContent ?? "";
    expect(text).toContain("Money columns use the left axis");
    expect(text).toContain("Basis:");
    expect(text).toContain("Method:");
    expect(text).toContain("Updates as data arrives");
    expect(text).toContain("never enter the KPIs");
    expect(text).toContain("Rest of this month");
    expect(text).toContain("Per remaining month");
    expect(text).toContain("The hatched part of an amount column is uncosted");
    expect(text).toContain("A dot under the axis");
    expect(text).toContain("category margin lines are omitted");
    expect(svg().querySelector("[data-chart-marks]")).toBe(marks);

    await act(async () => {
      root.render(
        <PurchaseOrderReportChart
          report={purchaseOrderReportYearFixture()}
          currency={IDR}
          period={null}
          embedded
        />
      );
    });
    await flush();
    expect(svg().querySelector("[data-chart-marks]")).toBe(marks);
  });

  it("keeps pointer and keyboard inspection and the exact table link in embedded mode", async () => {
    const report = purchaseOrderReportYearFixture();
    await render(
      <PurchaseOrderReportChart
        report={report}
        currency={IDR}
        period="year"
        embedded
      />
    );

    expect(
      svg().querySelector("[data-chart-marks]")?.getAttribute("data-chart-motion")
    ).toBe("static");

    const model = buildReportChart(report, []);
    const layout = layoutReportChart(model, 960);
    act(() => {
      surface().dispatchEvent(pointer("pointermove", layout.centers[0]));
    });
    const tooltip = container.querySelector("[data-chart-tooltip]");
    expect(tooltip?.textContent).toContain("Jan 2026");
    expect(tooltip?.textContent).toContain("PO count");
    expect(tooltip?.textContent).toContain("Amount");
    expect(tooltip?.textContent).toContain("Margin %");

    act(() => {
      svg().focus();
    });
    key(svg(), "End");
    expect(
      container.querySelector("[data-chart-tooltip]")?.textContent
    ).toContain("Dec 2026");
    expect(
      container.querySelector('a[href="#report-period-table"]')
    ).not.toBeNull();
  });

  it("keeps the honest unresolved-currency message without inventing axes", async () => {
    await render(
      <PurchaseOrderReportChart
        report={purchaseOrderReportYearFixture()}
        currency={null}
        embedded
      />
    );

    expect(chart().getAttribute("data-report-chart-embedded")).toBe("");
    expect(chart().textContent).toContain(
      "not in this app's currency catalogue"
    );
    expect(container.querySelector("[data-chart-axis]")).toBeNull();
    expect(container.querySelector("[data-report-forecast-status]")).toBeNull();
  });
});
