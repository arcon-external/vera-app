import { Children, isValidElement, type JSX, type ReactNode } from "react";

// Issue #642: the individual-comparison grid.
//
// It is presentation only. Callers supply cards in canonical selection order;
// this module never sorts, sums or reads report data. One column below the
// existing `lg` breakpoint and two above, with an odd final card spanning both
// columns only when the two-column layout is active.

export function PurchaseOrderReportComparisonGrid({
  children,
}: {
  children: ReactNode;
}): JSX.Element {
  const cards = Children.toArray(children);
  return (
    <div
      data-comparison-grid
      className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2"
    >
      {cards.map((card, index) => {
        const oddFinal = index === cards.length - 1 && cards.length % 2 === 1;
        const key =
          isValidElement(card) && card.key !== null
            ? String(card.key)
            : `comparison-card-${index}`;
        return (
          <div
            key={key}
            data-comparison-card
            {...(oddFinal ? { "data-comparison-last-odd": "" } : {})}
            className={oddFinal ? "min-w-0 lg:col-span-2" : "min-w-0"}
          >
            {card}
          </div>
        );
      })}
    </div>
  );
}
