// @vitest-environment jsdom
import { readFileSync } from "node:fs";
import path from "node:path";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import { normalizePurchaseOrderReport } from "../poReport";
import type { PurchaseOrderReportData } from "../poReport";
import { purchaseOrderReportFixture, purchaseOrderReportMonthFixture, purchaseOrderReportYearFixture } from "../poReportFixtures";
import type { ReportPeriodMode } from "../poReportPage";
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
  currency: CurrencyInfo | null = IDR,
  period: ReportPeriodMode | null = null,
  onPeriodChange: (period: ReportPeriodMode) => void = () => undefined
): Promise<void> {
  await act(async () => {
    root.render(
      <PurchaseOrderReportChart
        report={report}
        currency={currency}
        period={period}
        onPeriodChange={onPeriodChange}
      />
    );
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

  it("leaves a real gap where a period's margin percentage is null", async () => {
    const report = purchaseOrderReportFixture();
    report.periods[3] = { ...report.periods[3], marginBps: null };
    await render(report);
    const line = svg().querySelector<SVGPathElement>(
      "[data-chart-margin-line]"
    );
    expect(line).not.toBeNull();
    const path = line?.getAttribute("d") ?? "";
    expect(path.match(/M/g)).toHaveLength(2);
    expect(path).not.toContain("C");
    expect(path).not.toContain("Q");
    expect(svg().querySelector("[data-chart-latest-margin]")).not.toBeNull();
  });

  it("places the latest margin label beside its real point, not at the clipped top", async () => {
    const report = purchaseOrderReportFixture();
    await render(report);
    const points = svg().querySelectorAll("[data-chart-margin-point]");
    const lastPoint = points[points.length - 1];
    const label = svg().querySelector("[data-chart-latest-margin]");
    if (lastPoint === undefined || label === null) {
      throw new Error("expected the margin line and its latest label");
    }
    const cy = Number(lastPoint.getAttribute("cy"));
    const y = Number(label.getAttribute("y"));
    expect(Number.isFinite(cy)).toBe(true);
    // 10.77% sits inside a 0-30% plot; a basis-point/percent mix put it at
    // cy=-8148 and clamped the label to the top edge.
    expect(cy).toBeGreaterThan(0);
    expect(Math.abs(y - (cy - 6))).toBeLessThan(1);
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
    expect(surface().style.touchAction).toBe("pan-x pan-y");
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

  it("draws month ghost days distinctly after the last actual day, with a range whisker and divider", async () => {
    const report = purchaseOrderReportMonthFixture();
    await render(report, IDR, "month");

    const actual = svg().querySelectorAll(
      '[data-chart-bucket][data-projected="false"]'
    );
    expect(actual).toHaveLength(19);
    const projected = svg().querySelectorAll(
      '[data-chart-bucket][data-projected="true"]'
    );
    expect(projected).toHaveLength(11);
    for (const bucket of Array.from(projected)) {
      expect(bucket.querySelector('[data-series="amount"]')).not.toBeNull();
      for (const bar of Array.from(
        bucket.querySelectorAll<SVGRectElement>("[data-series]")
      )) {
        expect(bar.getAttribute("fill")).toContain("url(#");
        expect(bar.getAttribute("stroke-dasharray")).toBe("3 2");
      }
    }
    const ranges = svg().querySelectorAll("[data-chart-band]");
    expect(ranges).toHaveLength(11);
    for (const range of Array.from(ranges)) {
      expect(range.querySelectorAll("line")).toHaveLength(3);
      expect(range.getAttribute("data-bucket-index")).not.toBeNull();
    }
    expect(svg().querySelector("[data-chart-divider]")).not.toBeNull();
    expect(svg().querySelector("[data-chart-divider-label]")).not.toBeNull();
    expect(figure().textContent).toContain("never included in actual totals");
    const labels = Array.from(
      svg().querySelectorAll('[data-chart-axis="x"] text')
    ).map((node) => node.textContent);
    expect(labels[0]).toBe("1");
    expect(labels.at(-1)).toBe("30");
  });

  it("keeps the ghost tooltip to low, base, high, sample and basis, never a PO count", async () => {
    const report = purchaseOrderReportMonthFixture();
    await render(report, IDR, "month");
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

  it("shows an unavailable current-period reason and its real sample, with no flat line", async () => {
    const report = purchaseOrderReportMonthFixture();
    report.currentProjection.status = "unavailable";
    report.currentProjection.reason = "insufficient_days";
    report.currentProjection.buckets = [];
    await render(report, IDR, "month");

    expect(figure().textContent).toContain("Not enough elapsed days");
    expect(figure().textContent).toContain("19 elapsed days");
    expect(figure().textContent).toContain("12 POs observed");
    expect(svg().querySelectorAll('[data-projected="true"]')).toHaveLength(0);
    expect(svg().querySelector("[data-chart-band]")).toBeNull();
    expect(
      container.querySelector("[data-report-projection-summary]")
        ?.getAttribute("data-projection-status")
    ).toBe("unavailable");
  });

  it("names the current-period boundary instead of inventing a quarterly series", async () => {
    const report = purchaseOrderReportFixture();
    report.filter.bucket = "quarter";
    await render(report);

    expect(figure().textContent).toContain(
      "Current-period projections appear in the Month and Year views"
    );
    expect(svg().querySelectorAll('[data-projected="true"]')).toHaveLength(0);
    expect(svg().querySelector("[data-chart-divider]")).toBeNull();
  });

  it("stacks the current month's remainder as a translucent ghost segment and ghosts the later month", async () => {
    const report = purchaseOrderReportYearFixture();
    await render(report, IDR, "year");

    const november = svg().querySelector(
      '[data-chart-bucket][data-bucket-index="10"]'
    );
    expect(november).not.toBeNull();
    const remainder = november?.querySelector("[data-chart-remainder]");
    expect(remainder).not.toBeNull();
    expect(remainder?.querySelector('[data-series-remainder="amount"]')).not.toBeNull();
    expect(remainder?.querySelector('[data-series-remainder="capital"]')).not.toBeNull();
    expect(remainder?.querySelector('[data-series-remainder="margin"]')).not.toBeNull();
    expect(remainder?.getAttribute("opacity")).toBe("0.8");
    expect(
      remainder
        ?.querySelector('[data-series-remainder="amount"]')
        ?.getAttribute("fill")
    ).toContain("url(#");
    expect(svg().querySelector("[data-chart-remainder-band]")).not.toBeNull();
    expect(
      svg().querySelector('[data-chart-bucket][data-projected="true"]')
        ?.getAttribute("data-bucket-index")
    ).toBe("11");

    const summary = container.querySelector(
      "[data-report-projection-summary]"
    );
    expect(summary?.textContent).toContain("Rest of this month");
    expect(summary?.textContent).toContain("Low");
    expect(summary?.textContent).toContain("Base");
    expect(summary?.textContent).toContain("High");
    expect(summary?.textContent).toContain("Per remaining month");
    expect(summary?.textContent).toContain("Updates as data arrives");
  });

  it("renders a Month/Year toggle that reports the chosen period", async () => {
    const onPeriodChange = vi.fn();
    const report = purchaseOrderReportMonthFixture();
    await render(report, IDR, "month", onPeriodChange);

    const monthButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Month view"]'
    );
    const yearButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Year view"]'
    );
    expect(monthButton?.getAttribute("aria-pressed")).toBe("true");
    expect(yearButton?.getAttribute("aria-pressed")).toBe("false");

    await act(async () => {
      yearButton?.click();
    });
    expect(onPeriodChange).toHaveBeenCalledWith("year");
  });

  it("shows the projection summary's low, base, high and basis for Month mode", async () => {
    const report = purchaseOrderReportMonthFixture();
    await render(report, IDR, "month");

    const summary = container.querySelector(
      "[data-report-projection-summary]"
    );
    const text = summary?.textContent ?? "";
    expect(text).toContain("Month view, daily buckets");
    expect(text).toContain("Per remaining day");
    expect(text).toContain("Low");
    expect(text).toContain("Base");
    expect(text).toContain("High");
    expect(text).toContain("19 elapsed days");
    expect(text).toContain("12 POs observed");
    expect(text).toContain("as of 19 Nov 2026");
    expect(text).toContain("Updates as data arrives");
    expect(text).toContain("never enter the KPIs");
  });

  it("states the reason instead of low, base or high when the projection is unavailable", async () => {
    const report = purchaseOrderReportMonthFixture();
    report.currentProjection.status = "unavailable";
    report.currentProjection.reason = "insufficient_days";
    report.currentProjection.buckets = [];
    await render(report, IDR, "month");

    const summary = container.querySelector(
      "[data-report-projection-summary]"
    );
    expect(summary?.getAttribute("data-projection-reason")).toBe(
      "insufficient_days"
    );
    expect(summary?.textContent).toContain("Not enough elapsed days");
    expect(summary?.textContent).not.toContain("Per remaining day");
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
    expect(tooltip()?.textContent).toContain("Nov 2026");
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
    expect(surface().style.touchAction).toBe("pan-x pan-y");
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
    // A real zero is a measured value, not a missing one: no column rect is
    // drawn for a zero bar, and the money axis still prints its zero tick.
    expect(svg().querySelectorAll("rect[data-series]")).toHaveLength(0);
    const moneyLabels = [...svg().querySelectorAll('[data-chart-axis="money"] text')].map(
      (node) => node.textContent
    );
    expect(moneyLabels).toContain("Rp0");
    // No capital means no defined margin percentage: the line is absent rather
    // than drawn on the zero baseline.
    expect(svg().querySelector("[data-chart-margin-line]")).toBeNull();
  });
});
