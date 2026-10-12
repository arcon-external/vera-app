import type { JSX } from "react";
import type { CompanyTheme } from "../company";
import { DEFAULT_THEME, THEME_FIELDS } from "../company";
import { ThemeColorField } from "./ThemeColorField";
import { Card, SectionHeading } from "./ui";

// Presentational slice of the company form: the parent (CompanyProfileCard)
// owns the full theme and the whole-record save; see profilePayload in
// frontend/src/company.ts for why a slice must never save on its own.
// This panel holds no state.
export function CompanyThemePanel(props: {
  /** Which half of the theme this panel edits. */
  section: "Quotation" | "Portal";
  /** The full eight-colour theme — the parent owns it, both panels share it. */
  theme: CompanyTheme;
  /** Called with the field key and its new raw value. The parent stores it. */
  onChange: (key: keyof CompanyTheme, next: string) => void;
}): JSX.Element {
  const { section, theme, onChange } = props;
  return (
    <Card className="p-5">
      <SectionHeading title={`${section} colors`} />
      <div className="grid grid-cols-2 gap-3">
        {THEME_FIELDS.filter((field) => field.section === section).map((field) => (
          <ThemeColorField
            key={field.key}
            section={field.section}
            label={field.label}
            hint={field.hint}
            value={theme[field.key]}
            fallback={DEFAULT_THEME[field.key]}
            onChange={(next) => onChange(field.key, next)}
          />
        ))}
      </div>
    </Card>
  );
}
