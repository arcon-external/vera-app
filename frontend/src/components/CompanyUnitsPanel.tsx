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
import { COMPANY_TAB_SAVES } from "../companySaves";
import { Badge, Button, Card, SaveRow, SectionHeading, type SaveState, fieldClass } from "./ui";

// Presentational slice of the Company settings Units tab: the canonical-unit
// picker and the unit-list editor. Like the General and Theme panels, this
// holds no state — the parent (CompanyProfileCard) owns the list, the canonical
// unit and the save. The canonical unit moved to this tab's save (#432): it is
// written in the same transaction as the list, because a list that drops the
// canonical must be refused as one write, not corrected afterwards. See
// companyUnits.ts for the edit-time rules and frontend/src/companySaves.ts for
// why a slice never saves on its own.

// The native-select chevron idiom, the one UoMSelect in ItemEditor.tsx uses:
// `appearance-none` strips the platform arrow, and this inline SVG is drawn as
// the background at 10px, right 0.6rem from the centre — the same chevron the
// house Select draws. The house Select itself is deliberately not extended here
// (it has no disabled prop, and the picker is disabled while the catalogue
// loads); copying the idiom costs one constant.
//
// The xmlns attribute is load-bearing, not decoration: a data-URI SVG used as
// a background image is parsed in image mode, which has no HTML parser to
// imply the SVG namespace — without the attribute Chrome paints nothing at all,
// silently (verified in the ui-snapshot container: the same SVG with and
// without it). ItemEditor's copy now carries it too (#430).
const canonicalChevronStyle = {
  backgroundImage:
    "url(\"data:image/svg+xml;charset=UTF-8,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23767469' stroke-width='2' stroke-linecap='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")",
} as const;

const moveButtonClass =
  "flex h-10 w-10 cursor-pointer items-center justify-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent";

export function CompanyUnitsPanel(props: {
  /** The served unit list in display order, or null while it loads. */
  units: CompanyUnitOption[] | null;
  /** The current canonical unit (a value on the list). */
  canonical: string;
  onUnitsChange: (units: CompanyUnitOption[]) => void;
  onCanonicalChange: (value: string) => void;
  /** The live save state, shown on the save row's chip. */
  status: SaveState;
  /** True while a units save is in flight. */
  busy: boolean;
  /** A units-save error to show, or null. */
  error: string | null;
  onSaveUnits: () => void;
}): JSX.Element {
  const { units, canonical, onUnitsChange, onCanonicalChange, status, busy, error, onSaveUnits } = props;

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
        These are the units staff pick in the line-item editor — square meters
        can store as <span className="tnum">m2</span> and print as{" "}
        <span className="tnum">m²</span>. Units already on saved quotations
        keep whatever they were issued with.
      </p>

      <label className="mt-4 flex max-w-sm flex-col gap-1.5">
        <span className="text-caption font-medium text-ink-600">Canonical unit</span>
        <select
          value={canonicalTrimmed}
          onChange={(event) => onCanonicalChange(event.target.value)}
          className={`${fieldClass} cursor-pointer appearance-none bg-[length:10px] bg-[right_0.6rem_center] bg-no-repeat pr-7`}
          style={canonicalChevronStyle}
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
        It saves with{" "}
        <span className="font-medium text-ink-500">
          {COMPANY_TAB_SAVES.units.button}
        </span>{" "}
        below, in the same write as the list, and it must be one of the units on
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
                  <label className="flex flex-col gap-1.5">
                    <span className="text-caption font-medium text-ink-600">
                      Value — stored
                    </span>
                    <input
                      type="text"
                      aria-label={`Value for unit ${index + 1}`}
                      placeholder="Value (stored)"
                      value={unit.value}
                      maxLength={UOM_VALUE_MAX}
                      spellCheck={false}
                      autoComplete="off"
                      onChange={(event) =>
                        onUnitsChange(patchCompanyUnit(units, index, { value: event.target.value }))
                      }
                      className={`${fieldClass} font-mono`}
                    />
                  </label>
                  <label className="flex flex-col gap-1.5">
                    <span className="text-caption font-medium text-ink-600">
                      Label — printed
                    </span>
                    <input
                      type="text"
                      aria-label={`Label for unit ${index + 1}`}
                      placeholder="Label (printed)"
                      value={unit.label}
                      maxLength={UOM_LABEL_MAX}
                      onChange={(event) =>
                        onUnitsChange(patchCompanyUnit(units, index, { label: event.target.value }))
                      }
                      className={fieldClass}
                    />
                  </label>
                </div>
                {/* The badge and the controls read as the row's footer: full
                    width and justified apart below the inputs at 360 (where the
                    two captioned fields stack), inline beside them from sm up. */}
                <div className="flex w-full items-center justify-between sm:w-auto">
                  {isCanonical ? (
                    <Badge tone="accent">Canonical</Badge>
                  ) : (
                    <span className="w-[68px]" aria-hidden="true" />
                  )}
                  <div className="flex shrink-0 items-center gap-1">
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
                </div>
              </li>
            );
          })}
        </ul>

        {/* The tab's own save row (#432), the shared shape all four Company
            tabs now render: the button, the chip, the store's reason when it
            would refuse the list, and the save's error in a banner. */}
        <SaveRow
          label={COMPANY_TAB_SAVES.units.button}
          busyLabel={COMPANY_TAB_SAVES.units.busy}
          busy={busy}
          state={status}
          onSave={onSaveUnits}
          invalid={invalid}
          error={error}
          hint="Order is the order the editor dropdown shows."
        />
      </div>
    </Card>
  );
}
