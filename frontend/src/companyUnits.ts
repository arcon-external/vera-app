/**
 * Unit-of-measure editing helpers and validation for the Company settings
 * units panel, plus the served-shape mapping the editor consumes.
 *
 * The served list is the source of truth: GetCompanyUnits() returns the
 * company's units in display order as `backend.CompanyUnit` ({Value, Label},
 * the array position IS the order), and the editor dropdown, the canonical
 * default and the customer document all draw from it. Issue #337 removed the
 * hardcoded frontend catalog; this module holds the edit-time rules that
 * mirror the server boundary (backend/store/units.go checkCompanyUnits) so a
 * bad list is refused on this side too, with the same shape, before a round
 * trip that would only bounce it back.
 *
 * A unit is a stable value (what quotations store, sign and validate) plus a
 * display label (what the customer document prints). "Lot" is not special
 * here: the canonical unit owns every default, and a company may change it to
 * any unit on the list.
 */

/** One unit of measure as the editor dropdown sees it: value + display label. */
export type CompanyUnitOption = {
  value: string;
  label: string;
};

// Mirror uomMaxLen and labelMaxLen in backend/store/units.go. The value cap is
// a shape bound (a token, not a sentence); the label cap is what the rendered
// UoM column can hold. Both are duplicated here, on purpose: the panel refuses
// an over-long entry the same way the store would, with the same message.
export const UOM_VALUE_MAX = 12;
export const UOM_LABEL_MAX = 20;

/**
 * unitValueShaped reports whether a value could be a unit token: the same
 * alphabet the backend's unitValueShape allows — ASCII letters and digits,
 * plus the separators - / . — and nothing else. So "m2", "PC" and "kg/m3"
 * store; "m²" does not (the superscript is a label concern, so a company
 * relabelling square meters keeps the printed m² as the label while the value
 * stays the shippable m2).
 */
export function unitValueShaped(value: string): boolean {
  return /^[A-Za-z0-9\-/.]+$/.test(value);
}

/**
 * validateCompanyUnits checks a full replacement list exactly as the store
 * boundary does: never empty, every value a shaped non-empty short token,
 * every label a non-empty string, no two values differing only in case, and
 * the canonical unit still present. Returns the message to show, or null when
 * the list may be saved. The wording mirrors the store so a refusal reads the
 * same wherever it surfaces.
 *
 * The canonical check reflects the deferred FK: UpdateCompanyUnits refuses a
 * replacement that drops it (the profile's canonical_uom would fail to commit).
 * An empty canonical means the profile value has not resolved yet and is left
 * to the profile save path, which validates against the list server-side.
 */
export function validateCompanyUnits(
  units: CompanyUnitOption[],
  canonical: string
): string | null {
  if (units.length === 0) {
    return "At least one unit of measure is required.";
  }
  const seen = new Set<string>();
  for (let i = 0; i < units.length; i++) {
    const position = i + 1;
    const value = units[i].value.trim();
    if (value === "") return `Unit ${position}: value is required.`;
    if (value.length > UOM_VALUE_MAX) {
      return `Unit ${position}: value "${value}" is too long (max ${UOM_VALUE_MAX} characters).`;
    }
    if (!unitValueShaped(value)) {
      return `Unit ${position}: value "${value}" is not shaped like a unit label — use letters, digits, and - / . only.`;
    }
    const label = units[i].label.trim();
    if (label === "") return `Unit ${position}: label is required.`;
    if (label.length > UOM_LABEL_MAX) {
      return `Unit ${position}: label "${label}" is too long (max ${UOM_LABEL_MAX} characters).`;
    }
    const folded = value.toLowerCase();
    if (seen.has(folded)) {
      return `Unit ${position}: value "${value}" duplicates an earlier unit (case-insensitively).`;
    }
    seen.add(folded);
  }
  const canonicalTrimmed = canonical.trim();
  if (
    canonicalTrimmed !== "" &&
    !units.some((unit) => unit.value === canonicalTrimmed)
  ) {
    return `The canonical unit "${canonicalTrimmed}" must stay on the list.`;
  }
  return null;
}

/** Appends one blank row the user fills in before Save. */
export function addCompanyUnit(units: CompanyUnitOption[]): CompanyUnitOption[] {
  return [...units, { value: "", label: "" }];
}

/** Drops the row at index. */
export function removeCompanyUnit(
  units: CompanyUnitOption[],
  index: number
): CompanyUnitOption[] {
  return units.filter((_, i) => i !== index);
}

/** Patches one field of the row at index, leaving the others untouched. */
export function patchCompanyUnit(
  units: CompanyUnitOption[],
  index: number,
  patch: Partial<CompanyUnitOption>
): CompanyUnitOption[] {
  return units.map((unit, i) => (i === index ? { ...unit, ...patch } : unit));
}

/** Moves the row at index one step in direction (-1 up, +1 down), clamped. */
export function moveCompanyUnit(
  units: CompanyUnitOption[],
  index: number,
  direction: -1 | 1
): CompanyUnitOption[] {
  const target = index + direction;
  if (target < 0 || target >= units.length) return units;
  const next = [...units];
  [next[index], next[target]] = [next[target], next[index]];
  return next;
}
