import type { JSX } from "react";
import {
  UOM_LABEL_MAX,
  UOM_VALUE_MAX,
  addCompanyUnit,
  moveCompanyUnit,
  patchCompanyUnit,
  removeCompanyUnit,
  validateCompanyUnits,
} from "../companyUnits";
import type { CompanyUnitOption } from "../companyUnits";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";

// Presentational slice of the Company settings Units tab: the canonical-unit
// picker and the unit-list editor. Like the General and Theme panels, this
// holds no state — the parent (CompanyProfileCard) owns the list and the save.
// The canonical unit rides the whole-record profile save; the list has its
// own UpdateCompanyUnits save. See companyUnits.ts for the edit-time rules and
// frontend/src/company.ts for why a slice never saves on its own.

const moveButtonClass =
  "flex h-9 w-7 cursor-pointer items-center justify-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent";

/** Enter inside a field must not submit the surrounding profile form: it would
 *  save the profile (name, currency, canonical) while leaving the list unsaved —
 *  a partial save that reads as success. Bullets in the editor do the same. */
function blockEnter(event: React.KeyboardEvent): void {
  if (event.key === "Enter") event.preventDefault();
}

export function CompanyUnitsPanel(props: {
  /** The served unit list in display order, or null while it loads. */
  units: CompanyUnitOption[] | null;
  /** The current canonical unit (a value on the list). */
  canonical: string;
  onUnitsChange: (units: CompanyUnitOption[]) => void;
  onCanonicalChange: (value: string) => void;
  /** True while a units save is in flight. */
  busy: boolean;
  /** A units-save error to show, or null. */
  error: string | null;
  onSaveUnits: () => void;
}): JSX.Element {
  const { units, canonical, onUnitsChange, onCanonicalChange, busy, error, onSaveUnits } = props;

  if (units === null) {
    return (
      <Card className="p-5">
        <SectionHeading title="Units of measure" />
        <div className="h-48 animate-pulse rounded-xl bg-paper-muted" aria-label="Loading units" />
      </Card>
    );
  }

  const canonicalTrimmed = canonical.trim();
  // Live edit-time validation, mirroring the store boundary: it disables Save
  // and shows the reason, so a list that the server would refuse is never
  // offered as saved.
  const invalid = validateCompanyUnits(units, canonical);
  // The canonical picker must always name its current value, even one that has
  // slipped off the list, so saving the profile cannot silently clear it.
  const canonicalOptions: CompanyUnitOption[] =
    canonicalTrimmed !== "" && !units.some((unit) => unit.value === canonicalTrimmed)
      ? [{ value: canonicalTrimmed, label: canonicalTrimmed }, ...units]
      : units;

  return (
    <Card className="p-5">
      <SectionHeading title="Units of measure" />
      <p className="max-w-2xl text-body text-ink-600">
        These are the units staff pick in the line-item editor. The{" "}
        <span className="font-medium text-ink-800">value</span> is what a
        quotation stores and signs; the{" "}
        <span className="font-medium text-ink-800">label</span> is what prints —
        so square meters can store as <span className="tnum">m2</span> and print
        as <span className="tnum">m²</span>. Units already on saved quotations
        keep whatever they were issued with.
      </p>

      <label className="mt-4 flex max-w-sm flex-col gap-1.5">
        <span className="text-caption font-medium text-ink-600">Canonical unit</span>
        <select
          value={canonicalTrimmed}
          onChange={(event) => onCanonicalChange(event.target.value)}
          className={fieldClass}
          aria-label="Canonical unit"
        >
          {canonicalOptions.map((unit) => (
            <option key={unit.value} value={unit.value}>
              {unit.label && unit.label !== unit.value
                ? `${unit.value} — ${unit.label}`
                : unit.value}
            </option>
          ))}
        </select>
      </label>
      <p className="mt-1 max-w-2xl text-caption leading-relaxed text-ink-400">
        The unit a new line, sub-item and a package (sub-item) group default to.
        Saved with <span className="font-medium text-ink-500">Save company
        profile</span> below, not with the list. It must be one of the units on
        this list.
      </p>

      <div className="mt-6">
        <div className="mb-2 flex items-center justify-between gap-3">
          <h3 className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
            Units ({units.length})
          </h3>
          <Button
            variant="secondary"
            size="sm"
            leading={
              <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M8 3.333v9.334M3.333 8h9.334" />
              </svg>
            }
            onClick={() => onUnitsChange(addCompanyUnit(units))}
          >
            Add unit
          </Button>
        </div>

        <ul className="divide-y divide-rule rounded-lg border border-rule">
          {units.map((unit, index) => {
            const isCanonical = unit.value.trim() === canonicalTrimmed && unit.value.trim() !== "";
            return (
              <li key={index} className="flex flex-wrap items-center gap-2 px-3 py-2.5">
                <div className="grid min-w-0 flex-1 grid-cols-1 items-center gap-2 sm:grid-cols-2">
                  <input
                    type="text"
                    aria-label={`Value for unit ${index + 1}`}
                    placeholder="Value (stored)"
                    value={unit.value}
                    maxLength={UOM_VALUE_MAX}
                    spellCheck={false}
                    autoComplete="off"
                    onKeyDown={blockEnter}
                    onChange={(event) =>
                      onUnitsChange(patchCompanyUnit(units, index, { value: event.target.value }))
                    }
                    className={`${fieldClass} font-mono`}
                  />
                  <input
                    type="text"
                    aria-label={`Label for unit ${index + 1}`}
                    placeholder="Label (printed)"
                    value={unit.label}
                    maxLength={UOM_LABEL_MAX}
                    onKeyDown={blockEnter}
                    onChange={(event) =>
                      onUnitsChange(patchCompanyUnit(units, index, { label: event.target.value }))
                    }
                    className={fieldClass}
                  />
                </div>
                {isCanonical ? (
                  <Badge tone="accent">Canonical</Badge>
                ) : (
                  <span className="w-[68px]" aria-hidden="true" />
                )}
                <div className="flex shrink-0 items-center gap-0.5">
                  <button
                    type="button"
                    aria-label={`Move unit ${index + 1} up`}
                    disabled={index === 0}
                    onClick={() => onUnitsChange(moveCompanyUnit(units, index, -1))}
                    className={moveButtonClass}
                  >
                    <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.75">
                      <path d="M4 10l4-4 4 4" />
                    </svg>
                  </button>
                  <button
                    type="button"
                    aria-label={`Move unit ${index + 1} down`}
                    disabled={index === units.length - 1}
                    onClick={() => onUnitsChange(moveCompanyUnit(units, index, 1))}
                    className={moveButtonClass}
                  >
                    <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.75">
                      <path d="M4 6l4 4 4-4" />
                    </svg>
                  </button>
                  <button
                    type="button"
                    aria-label={`Remove unit ${index + 1}`}
                    // The canonical unit may not be removed while it is the
                    // default: the store refuses a replacement that drops it
                    // (the profile FK). Disabling it here prevents a save that
                    // could only bounce.
                    disabled={isCanonical}
                    onClick={() => onUnitsChange(removeCompanyUnit(units, index))}
                    className={`${moveButtonClass} hover:bg-danger-50 hover:text-danger-500`}
                  >
                    <svg width="9" height="9" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M4 4l8 8M12 4l-8 8" />
                    </svg>
                  </button>
                </div>
              </li>
            );
          })}
        </ul>

        <div className="mt-3 flex flex-wrap items-center gap-3">
          <Button
            variant="primary"
            onClick={onSaveUnits}
            disabled={busy || invalid !== null}
          >
            {busy ? "Saving…" : "Save units"}
          </Button>
          {invalid ? (
            <span className="text-caption text-ink-500">{invalid}</span>
          ) : (
            <span className="text-caption text-ink-400">
              Order is the order the editor dropdown shows.
            </span>
          )}
        </div>
        {error ? (
          <div className="mt-3">
            <StatusBanner tone="danger">{error}</StatusBanner>
          </div>
        ) : null}
      </div>
    </Card>
  );
}
