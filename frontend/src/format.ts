/**
 * The highest profit margin a line may carry, in percent.
 *
 * 1000% means selling at eleven times cost, which is ordinary for a low-cost
 * fitting on a service visit. The previous ceiling of 100% capped those at
 * double cost, which is not enough to build a quote around.
 *
 * This mirrors backend.MaxMarginPercent. Both the typing clamp and the paste
 * clamp use it so the two cannot drift apart and reject in different places.
 *
 * Discount percentage is a different setting and keeps its own ceiling of 100.
 */
export const MAX_MARGIN_PERCENT = 1000;

/** Formats a numeric IDR amount using Indonesian thousands separators. */
export function formatIdr(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return Math.trunc(value).toLocaleString("id-ID");
}

/** Parses a numeric input string, treating anything unparseable as zero. */
export function parseIntOrZero(value: string): number {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : 0;
}

/**
 * Formats a raw digit string for money inputs with thousand delimiters.
 * Empty stays empty so untouched fields never read "0".
 */
export function formatMoneyInput(raw: string): string {
  if (raw === "") return "";
  return formatIdr(parseIntOrZero(raw));
}

/**
 * Applies a percentage margin to a unit cost using integer IDR arithmetic,
 * matching the backend's truncation: cost + trunc(cost * percent / 100).
 */
export function applyMargin(cost: number, percent: number, enabled: boolean): number {
  if (!enabled) return cost;
  return cost + Math.trunc((cost * percent) / 100);
}

/**
 * The overall margin across priced sub-items: total profit over total cost.
 *
 * The money is summed first and the division happens once -- the blend of the
 * rates is not the rate of the blend, and the backend divides the money. A
 * whole percentage is shown as-is; anything else is rounded half-up and
 * flagged so the caller can prefix "≈".
 *
 * This mirrors the backend's internal breakdown; both sides use integer math
 * so they cannot disagree about which percentages are whole.
 */
export function blendedMargin(
  totalCost: number,
  totalSell: number
): { percent: number; approx: boolean } {
  if (totalCost <= 0) return { percent: 0, approx: false };
  const numerator = (totalSell - totalCost) * 100;
  const approx = numerator % totalCost !== 0;
  // Half-up on non-negative values: floor((2a + b) / 2b).
  const percent = Math.floor((2 * numerator + totalCost) / (2 * totalCost));
  return { percent, approx };
}
