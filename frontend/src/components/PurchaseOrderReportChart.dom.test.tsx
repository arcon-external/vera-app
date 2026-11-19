// @vitest-environment jsdom
import { readFileSync } from "node:fs";
import path from "node:path";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import { normalizePurchaseOrderReport } from "../poReport";
import type { PurchaseOrderReportData } from "../poReport";
import { purchaseOrderReportFixture } from "../poReportFixtures";
import {
  buildReportChart,
  layoutReportChart,
} from "../poReportChart";
import { PurchaseOrderReportChart } from "./PurchaseOrderReportChart";

// Issue #631: the chart is a single SVG with one hover overlay, one tooltip and
// one keyboard cursor. These tests drive the real component in jsdom: pointer,
// touch and keyboard share the same nearest-bucket selection, null is a gap,
// projected marks are unmistakably not actual, and motion is only on a fresh
// data identity.

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

// jsdom gives import.meta.url an http scheme, so resolve from the frontend
// working directory (the Vitest root) as the snapshot parity test does.
const SHARED = JSON.parse(
  readFileSync(
    path.resolve(
      process.cwd(),
      "../backend/render/report/testdata/report_snapshot.json"
    ),
    "utf8"
  )
) as { report: Parameters<typeof normalizePurchaseOrderReport>[0] };

const originalMatchMedia = window.matchMedia;
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
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  window.matchMedia = originalMatchMedia;
  vi.restoreAllMocks();
});

function useReducedMotion(matches: boolean): void {
  window.matchMedia = ((query: string) => ({
    matches: matches && query.includes("prefers-reduced-motion"),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

async function render(
  report: PurchaseOrderReportData,
  currency: CurrencyInfo | null = IDR
): Promise<void> {
  await act(async () => {
    root.render(<PurchaseOrderReportChart report={report} currency={currency} />);
  });
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function figure(): HTMLElement {
  const found = container.querySelector<HTMLElement>("[data-report-chart]");
  if (found === null) throw new Error("no chart card");
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

function pointer(
  type: string,
  options: { clientX: number; pointerType?: string }
): Event {
  const event = new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    clientX: options.clientX,
  });
  Object.defineProperty(event, "pointerType", {
    value: options.pointerType ?? "mouse",
  });
  return event;
}

function moveTo(index: number, report: PurchaseOrderReportData): void {
  const model = buildReportChart(report, []);
  const layout = layoutReportChart(model, 960);
  act(() => {
    surface().dispatchEvent(pointer("pointermove", { clientX: layout.centers[index] }));
  });
}

function tooltip(): HTMLElement | null {
  return container.querySelector<HTMLElement>("[data-chart-tooltip]");
}

function key(target: Element, value: string): void {
  act(() => {
    target.dispatchEvent(
      new KeyboardEvent("keydown", { key: value, bubbles: true })
    );
  });
}

function sharedMonthly(): PurchaseOrderReportData {
  return normalizePurchaseOrderReport(SHARED.report);
}

describe("Purchase order performance chart (issue #631)", () => {
  it("renders both labelled axes, grouped money columns and a percentage line", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);

    expect(figure().textContent).toContain("Money");
    expect(figure().textContent).toContain("Margin %");
    const moneyAxis = svg().querySelector('[data-chart-axis="money"]');
    const percentAxis = svg().querySelector('[data-chart-axis="percent"]');
    expect(moneyAxis).not.toBeNull();
    expect(percentAxis).not.toBeNull();
    expect(moneyAxis?.textContent).toContain("Rp");
    expect(percentAxis?.textContent).toContain("%");

    const actual = svg().querySelectorAll('[data-chart-bucket][data-projected="false"]');
    expect(actual).toHaveLength(report.periods.length);
    for (const bucket of Array.from(actual)) {
      expect(bucket.querySelector('[data-series="amount"]')).not.toBeNull();
      expect(bucket.querySelector('[data-series="capital"]')).not.toBeNull();
      expect(bucket.querySelector('[data-series="margin"]')).not.toBeNull();
    }
    const line = svg().querySelector<SVGPathElement>("[data-chart-margin-line]");
    expect(line).not.toBeNull();
    expect(line?.getAttribute("d")).not.toContain("C");
    expect(line?.getAttribute("d")).not.toContain("Q");
  });

  it("draws the uncosted part as a hatch inside the amount column, not above it", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);
    const hatched = svg().querySelector<SVGRectElement>("[data-chart-uncosted]");
    expect(hatched).not.toBeNull();
    const amount = hatched?.parentElement?.querySelector<SVGRectElement>(
      '[data-series="amount"]'
    );
    expect(amount).not.toBeNull();
    const y = Number(hatched?.getAttribute("y"));
    const height = Number(hatched?.getAttribute("height"));
    const amountY = Number(amount?.getAttribute("y"));
    const amountHeight = Number(amount?.getAttribute("height"));
    expect(y).toBeGreaterThanOrEqual(amountY - 0.01);
    expect(y + height).toBeLessThanOrEqual(amountY + amountHeight + 0.01);
    expect(hatched?.getAttribute("fill")).toContain("url(#");
  });

  it("shows one overlay and one tooltip with every actual metric on hover", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);
    moveTo(0, report);

    expect(svg().querySelectorAll("[data-chart-hover-band]")).toHaveLength(1);
    const tip = tooltip();
    expect(tip).not.toBeNull();
    const text = tip?.textContent ?? "";
    for (const label of [
      "PO count",
      "Amount",
      "Costed",
      "Uncosted",
      "Capital",
      "Margin",
      "Margin %",
      "Coverage",
    ]) {
      expect(text).toContain(label);
    }
    expect(text).toContain("Rp");
    expect(text).toContain("Jan 2026");
  });

  it("taps a point on touch, keeps the tooltip and leaves vertical panning alone", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);
    expect(surface().style.touchAction).toBe("pan-y");
    const model = buildReportChart(report, []);
    const layout = layoutReportChart(model, 960);

    act(() => {
      surface().dispatchEvent(
        pointer("pointerdown", {
          clientX: layout.centers[2],
          pointerType: "touch",
        })
      );
    });
    expect(tooltip()?.textContent).toContain("Mar 2026");

    act(() => {
      surface().dispatchEvent(
        pointer("pointerleave", { clientX: layout.centers[2], pointerType: "touch" })
      );
    });
    expect(tooltip()?.textContent).toContain("Mar 2026");
  });

  it("toggles category lines from the legend and rescales only the percentage axis", async () => {
    const report = sharedMonthly();
    report.categorySeries[0].marginBps[0] = 12_000;
    await render(report);

    const moneyBefore = svg().querySelector('[data-chart-axis="money"]')?.textContent;
    const percentBefore = svg().querySelector('[data-chart-axis="percent"]')?.textContent;
    expect(svg().querySelector('[data-category-line="FM"]')).toBeNull();

    const toggle = container.querySelector<HTMLButtonElement>(
      '[data-category-toggle="FM"]'
    );
    expect(toggle).not.toBeNull();
    expect(toggle?.getAttribute("aria-pressed")).toBe("false");
    await act(async () => {
      toggle?.click();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(toggle?.getAttribute("aria-pressed")).toBe("true");
    expect(svg().querySelector('[data-category-line="FM"]')).not.toBeNull();
    expect(svg().querySelector('[data-chart-axis="money"]')?.textContent).toBe(
      moneyBefore
    );
    expect(svg().querySelector('[data-chart-axis="percent"]')?.textContent).not.toBe(
      percentBefore
    );
    // FM is the solid category line; the others carry dash patterns.
    expect(
      svg()
        .querySelector('[data-category-line="FM"]')
        ?.getAttribute("stroke-dasharray")
    ).toBeNull();

    await act(async () => {
      toggle?.click();
    });
    expect(toggle?.getAttribute("aria-pressed")).toBe("false");
    expect(svg().querySelector('[data-category-line="FM"]')).toBeNull();
  });

  it("draws projected columns distinctly, with one amount band and a labelled divider", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);

    const projected = svg().querySelectorAll(
      '[data-chart-bucket][data-projected="true"]'
    );
    expect(projected).toHaveLength(2);
    for (const bucket of Array.from(projected)) {
      expect(bucket.querySelector('[data-series="amount"]')).not.toBeNull();
      for (const bar of Array.from(
        bucket.querySelectorAll<SVGRectElement>("[data-series]")
      )) {
        expect(bar.getAttribute("fill")).toContain("url(#");
        expect(bar.getAttribute("stroke-dasharray")).toBe("3 2");
      }
    }
    expect(svg().querySelectorAll("[data-chart-band]")).toHaveLength(1);
    expect(svg().querySelector("[data-chart-divider]")).not.toBeNull();
    expect(svg().querySelector("[data-chart-divider-label]")).not.toBeNull();
    expect(figure().textContent).toContain("never included in actual totals");
  });

  it("keeps the projected tooltip to low, base, high, sample and basis, never a PO count", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);
    const chart = svg();
    act(() => {
      chart.focus();
    });
    key(chart, "End");
    const text = tooltip()?.textContent ?? "";
    for (const label of [
      "Amount low",
      "Amount base",
      "Amount high",
      "Sample",
      "Basis",
      "Projected",
    ]) {
      expect(text).toContain(label);
    }
    expect(text).not.toContain("PO count");
    expect(text).not.toContain("Coverage");
  });

  it("shows an unavailable projection's reason and real history basis, with no flat line", async () => {
    const report = purchaseOrderReportFixture();
    report.projection.monthly.status = "unavailable";
    report.projection.monthly.reason = "insufficient_history";
    report.projection.monthly.buckets = [];
    await render(report);

    expect(figure().textContent).toContain(
      "Not enough complete-month history"
    );
    expect(figure().textContent).toContain("complete months");
    expect(figure().textContent).toContain("8 POs observed");
    expect(svg().querySelectorAll('[data-projected="true"]')).toHaveLength(0);
    expect(svg().querySelector("[data-chart-band]")).toBeNull();
  });

  it("names the missing quarterly projection instead of inventing a series", async () => {
    const report = purchaseOrderReportFixture();
    report.filter.bucket = "quarter";
    await render(report);

    expect(figure().textContent).toContain("No quarterly projection");
    expect(svg().querySelectorAll('[data-projected="true"]')).toHaveLength(0);
    expect(svg().querySelector("[data-chart-divider]")).toBeNull();
  });

  it("moves one keyboard cursor with the arrow keys and exposes the table link", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);
    const chart = svg();
    expect(chart.getAttribute("tabindex")).toBe("0");
    expect(chart.querySelectorAll("[tabindex]")).toHaveLength(0);

    act(() => {
      chart.focus();
    });
    // Focusing the chart places the one cursor on the first period.
    expect(tooltip()?.textContent).toContain("Jan 2026");
    key(chart, "ArrowRight");
    expect(tooltip()?.textContent).toContain("Feb 2026");
    key(chart, "End");
    expect(tooltip()?.textContent).toContain("Jan 2027");
    key(chart, "Escape");
    expect(tooltip()).toBeNull();

    const link = container.querySelector<HTMLAnchorElement>(
      'a[href="#report-period-table"]'
    );
    expect(link).not.toBeNull();
    expect(link?.textContent).toContain("Period table");
  });

  it("renders the final state immediately under reduced motion", async () => {
    useReducedMotion(true);
    const report = purchaseOrderReportFixture();
    await render(report);

    const marks = svg().querySelector("[data-chart-marks]");
    expect(marks?.getAttribute("data-chart-motion")).toBe("static");
    expect(svg().querySelector(".report-chart-column-in")).toBeNull();
    expect(svg().querySelector(".report-chart-line-in")).toBeNull();
  });

  it("animates a fresh data identity once, not on hover, resize or a parent re-render", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);
    const marks = svg().querySelector("[data-chart-marks]");
    expect(marks?.getAttribute("data-chart-motion")).toBe("run");
    expect(svg().querySelector(".report-chart-column-in")).not.toBeNull();

    moveTo(1, report);
    await act(async () => {
      root.render(<PurchaseOrderReportChart report={report} currency={IDR} />);
    });
    expect(svg().querySelector("[data-chart-marks]")).toBe(marks);

    const changed = purchaseOrderReportFixture();
    changed.filter.bucket = "year";
    await act(async () => {
      root.render(<PurchaseOrderReportChart report={changed} currency={IDR} />);
    });
    expect(svg().querySelector("[data-chart-marks]")).not.toBe(marks);
    expect(svg().querySelector(".report-chart-column-in")).not.toBeNull();
  });

  it("keeps SVG hatch and clip ids unique when two charts are mounted", async () => {
    await act(async () => {
      root.render(
        <div>
          <PurchaseOrderReportChart report={purchaseOrderReportFixture()} currency={IDR} />
          <PurchaseOrderReportChart report={purchaseOrderReportFixture()} currency={IDR} />
        </div>
      );
    });
    const ids = Array.from(
      container.querySelectorAll<SVGElement>("pattern[id], clipPath[id]")
    ).map((node) => node.id);
    expect(ids.length).toBeGreaterThan(0);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("contains a 120-bucket chart in a horizontal scroll container with thinned labels", async () => {
    const report = purchaseOrderReportFixture();
    report.periods = Array.from({ length: 120 }, (_, index) => {
      const year = 2005 + Math.floor(index / 12);
      const month = String((index % 12) + 1).padStart(2, "0");
      return {
        periodStart: `${year}-${month}-01`,
        partial: false,
        inProgress: false,
        poCount: 1,
        amountMinor: 1_000_000 + index,
        costedMinor: 1_000_000 + index,
        uncostedMinor: 0,
        capitalMinor: 800_000,
        marginMinor: 200_000 + index,
        marginBps: 2500,
        coverageBps: 10_000,
      };
    });
    report.categorySeries = [];
    report.projection.monthly.buckets = [];
    report.projection.yearly.buckets = [];
    await render(report);

    expect(surface().className).toContain("overflow-x-auto");
    expect(Number(svg().getAttribute("width"))).toBeGreaterThan(960);
    const labels = svg().querySelectorAll('[data-chart-axis="x"] text');
    expect(labels.length).toBeLessThan(120);
    expect(labels.length).toBeGreaterThan(4);
    expect(labels[labels.length - 1]?.textContent).toBe("Dec 14");
    expect(surface().style.touchAction).toBe("pan-y");
  });

  it("states plainly when the report currency is not in the catalogue", async () => {
    await render(purchaseOrderReportFixture(), null);
    expect(figure().textContent).toContain(
      "not in this app's currency catalogue"
    );
    expect(container.querySelector("[data-chart-axis]")).toBeNull();
    expect(container.querySelector("[data-chart-tooltip]")).toBeNull();
  });

  it("never emits NaN geometry for an empty or all-zero snapshot", async () => {
    const report = purchaseOrderReportFixture();
    report.periods = report.periods.map((period) => ({
      ...period,
      amountMinor: 0,
      costedMinor: 0,
      uncostedMinor: 0,
      capitalMinor: 0,
      marginMinor: 0,
      marginBps: null,
      coverageBps: null,
    }));
    report.projection.monthly.buckets = [];
    report.projection.yearly.buckets = [];
    report.categorySeries = [];
    await render(report);

    expect(svg().innerHTML).not.toContain("NaN");
    expect(svg().innerHTML).not.toContain("Infinity");
  });
});
