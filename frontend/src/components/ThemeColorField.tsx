// Extracted from CompanyProfileCard's inline colour controls: the eight
// colour inputs share this exact markup, so CompanyThemePanel renders both
// the Quotation and Portal panels through this component instead of
// duplicating thirty lines of JSX.
import { isCompleteHex } from "../company";
import { fieldClass } from "./ui";

export function ThemeColorField(props: {
  /** The panel this field belongs to — used in the accessible names, e.g. "Quotation". */
  section: string;
  /** The human label, e.g. "Deep". */
  label: string;
  /** The helper sentence under the control. */
  hint: string;
  /** The raw stored value; may be empty or invalid. Shown verbatim in the
   *  text input — the picker trims it before testing (see below). */
  value: string;
  /** The baked default: the picker shows it when the trimmed value is empty
   *  or not a complete hex, and the text input uses it as its placeholder. */
  fallback: string;
  onChange: (next: string) => void;
}): JSX.Element {
  const { section, label, hint, value, fallback, onChange } = props;
  // As the inline controls did: trim before testing, and substitute
  // the default when the trimmed value is empty.
  const trimmed = value.trim();
  const effective = trimmed === "" ? fallback : trimmed;
  const pickerValue = isCompleteHex(effective) ? effective : fallback;
  return (
    <label className="flex flex-col gap-1.5" title={`${label}: ${hint}`}>
      <span className="text-caption font-medium text-ink-600">{label}</span>
      <span className="flex items-center gap-2">
        <input
          type="color"
          value={pickerValue}
          onChange={(event) => onChange(event.target.value)}
          aria-label={`${section} ${label} color picker`}
          className="h-9 w-10 shrink-0 cursor-pointer rounded-md border border-rule bg-white p-1"
        />
        <input
          type="text"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={fallback}
          spellCheck={false}
          autoComplete="off"
          aria-label={`${section} ${label} hex value`}
          className={`${fieldClass} font-mono`}
        />
      </span>
      <span className="text-caption leading-snug text-ink-400">{hint}</span>
    </label>
  );
}
