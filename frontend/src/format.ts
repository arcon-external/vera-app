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
