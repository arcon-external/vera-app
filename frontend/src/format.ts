import type { currency } from "@backend/models";

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

/**
 * The ceiling on a per-row discount. 100% is a free line, which is a real
 * thing to quote; above that it is a price rather than a discount. Mirrors
 * MaxDiscountPercent in the backend, and shares the name rather than repeating
 * the number -- a limit written down twice is a limit that eventually
 * disagrees with itself.
 */
export const MAX_DISCOUNT_PERCENT = 100;

/** Parses a numeric input string, treating anything unparseable as zero. */
export function parseIntOrZero(value: string): number {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : 0;
}

/**
 * One entry of the company-currency catalogue. The single source of truth
 * lives in Go (`backend/currency/currency.go`); this shape arrives over the
 * `GetCurrencyCatalogue()` binding as `currency.CurrencyInfo` and is
 * re-exported here so callers name one type. There is deliberately no
 * hardcoded catalogue on this side -- a second list of exponents is a second
 * list that eventually disagrees with the first.
 *
 * This module takes the currency as a parameter and knows nothing of where
 * it came from: archive/detail components pass the REVISION currency while
 * editor components pass the LIVE profile currency (issue #336 Wave B).
 * Do not bake a default currency into these functions.
 */
export type CurrencyInfo = currency.CurrencyInfo;

/**
 * The sanitised display fields of one entry. A hand-built entry whose
 * separators are missing or identical cannot group (grouping with the decimal
 * separator never round-trips -- see Go's TestSeparatorsDiffer), so it falls
 * back to the IDR separators, mirroring Go's Normalize-to-IDR rule for
 * unknown codes. A negative or non-finite exponent means no fraction.
 */
function displayFields(entry: CurrencyInfo): {
  exponent: number;
  groupSep: string;
  decimalSep: string;
} {
  const exponent =
    Number.isFinite(entry.exponent) && entry.exponent > 0
      ? Math.trunc(entry.exponent)
      : 0;
  if (!entry.groupSep || !entry.decimalSep || entry.groupSep === entry.decimalSep) {
    return { exponent, groupSep: ".", decimalSep: "," };
  }
  return { exponent, groupSep: entry.groupSep, decimalSep: entry.decimalSep };
}

function pow10(exp: number): number {
  let multiplier = 1;
  for (let i = 0; i < exp; i++) multiplier *= 10;
  return multiplier;
}

/** Groups a digit string in threes with the separator, exactly as Go's groupThousands. */
function groupThousands(digits: string, sep: string): string {
  if (digits.length <= 3) return digits;
  const parts: string[] = [];
  let rest = digits;
  while (rest.length > 3) {
    parts.unshift(rest.slice(-3));
    rest = rest.slice(0, -3);
  }
  parts.unshift(rest);
  return parts.join(sep);
}

/**
 * Renders minor units as a decorated amount in the entry's home convention:
 * exponent-aware grouping via the entry's separators, the fraction padded to
 * `exponent` digits, the symbol placed per `position` and spaced per
 * `symbolSpaced`.
 *
 * This must agree character-for-character with Go's
 * `Decorate(code, FormatAmount(code, minor))` -- the strings are pinned by
 * `TestFormatAmount`, `TestDecorate` and `TestFormatFromMinorUnits` in
 * `backend/currency/currency_test.go`, and `format.test.ts` asserts this
 * side against those same literals. In particular a negative prints with the
 * symbol OUTSIDE the minus (`$-1,500.00`), because Go decorates the already
 * signed bare figure; do not "fix" the order.
 *
 * Non-finite input renders as zero (the old `formatIdr` rule); fractional
 * input truncates toward zero (the `applyMargin` rule). Money travels as
 * integers -- a float reaching here is already a bug, and truncation keeps
 * both sides on the same integer.
 */
export function formatMoney(entry: CurrencyInfo, minor: number): string {
  const { exponent, groupSep, decimalSep } = displayFields(entry);
  let amount = Number.isFinite(minor) ? Math.trunc(minor) : 0;
  const negative = amount < 0;
  if (negative) amount = -amount;
  let bare: string;
  if (exponent === 0) {
    bare = groupThousands(String(amount), groupSep);
  } else {
    const divisor = pow10(exponent);
    const major = Math.trunc(amount / divisor);
    let fraction = String(amount % divisor);
    while (fraction.length < exponent) fraction = "0" + fraction;
    bare = groupThousands(String(major), groupSep) + decimalSep + fraction;
  }
  if (negative) bare = "-" + bare;
  if (entry.position === "after") {
    return bare + (entry.symbolSpaced ? " " : "") + entry.symbol;
  }
  return entry.symbol + (entry.symbolSpaced ? " " : "") + bare;
}

/**
 * Wraps an already-formatted bare figure with the entry's position/spacing
 * affixes -- the TypeScript half of Go's `Decorate(code, bare)` in
 * `backend/currency/currency.go`. The server's `Display*` strings are bare
 * numbers (no symbol: `FormatAmount`, never decorated), so components pass
 * them straight through here rather than re-rendering from integers.
 *
 * The signed bare figure keeps Go's symbol-outside-minus order
 * (`decorateMoney(USD, "-1,500.00")` is `"$-1,500.00"`): the minus stays
 * inside the affixes, exactly as the server prints it. Never swap-prefix --
 * EUR/VND trail (`"1.234,56 €"`), so a leading `$`-style template prints
 * the wrong order.
 */
export function decorateMoney(entry: CurrencyInfo, bare: string): string {
  if (entry.position === "after") {
    return bare + (entry.symbolSpaced ? " " : "") + entry.symbol;
  }
  return entry.symbol + (entry.symbolSpaced ? " " : "") + bare;
}

/**
 * Parses numeric money text into minor units, mirroring Go's ParseMinor in
 * `backend/currency/currency.go` (same grouping validation, same fraction
 * padding, same refusals): the entry's group separator is stripped, its
 * decimal separator opens the fraction, at most `exponent` fraction digits.
 *
 * Two deliberate differences from Go, both for the input field:
 *
 * - The entry's own symbol and surrounding spaces are stripped first, so a
 *   formatted figure pasted or fed back (`$1,500.00`, `1.234,56 €`) parses
 *   to its value. Go refuses symbols because its inputs hold bare numbers;
 *   this one must never silently return 0 for formatted money -- today's
 *   `parseIntOrZero("$1,500.00")` is 0, and that is the bug being fixed.
 * - Failure returns 0 instead of an error (the `parseIntOrZero` convention:
 *   inputs hold numbers, and there is no error channel). A figure from
 *   another currency's convention still fails -- cross-currency paste is
 *   refused, not converted, because there is no rate to convert with.
 */
export function parseMoneyInput(entry: CurrencyInfo, raw: string): number {
  const parsed = tryParseMinor(entry, raw);
  return parsed === null ? 0 : parsed;
}

/** Nullable core of parseMoneyInput; null means "not a number". */
function tryParseMinor(entry: CurrencyInfo, raw: string): number | null {
  const { exponent, groupSep, decimalSep } = displayFields(entry);
  let text = raw.replace(/\u00a0/g, " ").trim();
  if (text === "") return null;
  if (entry.symbol !== "") {
    text = text.split(entry.symbol).join("").replace(/\u00a0/g, " ").trim();
  }
  let negative = false;
  if (text.startsWith("-")) {
    negative = true;
    text = text.slice(1).trim();
  }
  if (text === "") return null;

  let intPart = text;
  let fracPart = "";
  if (exponent > 0) {
    const i = text.lastIndexOf(decimalSep);
    if (i >= 0) {
      intPart = text.slice(0, i);
      fracPart = text.slice(i + decimalSep.length);
    }
  } else if (decimalSep !== groupSep && text.includes(decimalSep)) {
    // An exponent-0 currency has no fraction: a decimal separator here is a
    // misplaced grouping from another convention, refused rather than
    // silently truncated -- exactly as Go refuses it.
    return null;
  }
  if (fracPart.includes(groupSep) || intPart.includes(decimalSep)) return null;
  if (fracPart.length > exponent) return null;
  while (fracPart.length < exponent) fracPart += "0";

  let digits = intPart;
  if (groupSep !== "" && intPart.includes(groupSep)) {
    const groups = intPart.split(groupSep);
    if (groups[0].length < 1 || groups[0].length > 3) return null;
    for (const group of groups.slice(1)) {
      if (group.length !== 3) return null;
    }
    digits = groups.join("");
  }
  if (digits === "") return null;
  if (!/^[0-9]+$/.test(digits + fracPart)) return null;
  const major = Number(digits);
  const fraction = fracPart === "" ? 0 : Number(fracPart);
  if (!Number.isSafeInteger(major) || !Number.isSafeInteger(fraction)) return null;
  const minor = major * pow10(exponent) + fraction;
  if (!Number.isSafeInteger(minor)) return null;
  if (negative && minor !== 0) return -minor;
  return minor;
}

/**
 * Display form for money inputs: empty stays empty so untouched fields never
 * read as zero, anything else renders through formatMoney.
 *
 * The stored string is minor-unit digits (`onChange` strips the display
 * back to digits), so a pure digit string means minor units -- a stored
 * string never carries separators. Text carrying either separator is
 * major-unit typing, pasted text or fed-back formatted output, parsed with
 * Go's major-unit semantics so a half-typed `15.5` reads `$15.50` (not
 * `$1.55`) and a pasted `15,000` reads `$15,000.00` (not `$150.00`).
 * Unparseable separator text falls back to its leading integer (the old
 * `parseIntOrZero` rule) rather than zeroing what the user typed.
 */
export function formatMoneyInput(entry: CurrencyInfo, raw: string): string {
  if (raw === "") return "";
  const { groupSep, decimalSep } = displayFields(entry);
  const body = raw.replace(/\u00a0/g, " ").trim();
  const stripped =
    entry.symbol !== ""
      ? body.split(entry.symbol).join("").replace(/\u00a0/g, " ").trim()
      : body;
  if (stripped.includes(decimalSep) || stripped.includes(groupSep)) {
    const parsed = tryParseMinor(entry, raw);
    if (parsed !== null) return formatMoney(entry, parsed);
    return formatMoney(entry, parseIntOrZero(stripped));
  }
  const negative = stripped.startsWith("-");
  const digits = stripped.replace(/[^0-9]/g, "");
  if (digits === "") return formatMoney(entry, 0);
  return formatMoney(entry, (negative ? -1 : 1) * parseIntOrZero(digits));
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
 * A row's own discount, taken off the already-marginalised price.
 *
 * Mirrors applyDiscount in backend/builder.go exactly, for the same reason
 * applyMargin mirrors applyMarkup: the two must never disagree about a number
 * the customer is quoted. Margin first, then discount, so the printed list
 * price stays the figure staff see.
 *
 * The cost side is untouched by this. What we pay suppliers does not move when
 * the customer is given a lower price -- see issue #270.
 */
export function applyDiscount(price: number, percent: number, enabled: boolean): number {
  if (!enabled) return price;
  return price - Math.trunc((price * percent) / 100);
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
