import type { ReactNode } from "react";
import { fieldClass } from "./ui";

// Issue #492: the two shared settings controls. Both settings pages render the
// same label/hint block and the same clamped number field, so the two surfaces
// cannot drift into different validation or spacing.

/** One labelled settings control, with the reason under it. */
export function SettingsField({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-caption font-medium text-ink-600">{label}</span>
      {children}
      {hint ? <span className="text-caption text-ink-400">{hint}</span> : null}
    </label>
  );
}

/**
 * A whole-number setting that cannot store a value outside its bounds. The
 * digits are taken exactly as typed and clamped on the way in, which is the
 * rule the lead-time segments already use: one guard site covers typing and
 * paste, and no invalid value ever reaches storage.
 */
export function NumberSetting({
  label,
  ariaLabel,
  value,
  min,
  max,
  suffix,
  hint,
  onChange,
}: {
  label: string;
  ariaLabel: string;
  value: number;
  min: number;
  max: number;
  suffix: string;
  hint?: string;
  onChange: (value: number) => void;
}) {
  return (
    <SettingsField label={label} hint={hint}>
      <div className="flex items-center rounded-md border border-rule bg-white pr-3 transition duration-150 focus-within:border-accent-400 focus-within:ring-2 focus-within:ring-accent-500/20">
        <input
          type="text"
          inputMode="numeric"
          aria-label={ariaLabel}
          value={String(value)}
          onChange={(event) => {
            const parsed = Number.parseInt(event.target.value.replace(/\D/g, ""), 10);
            const next = Number.isFinite(parsed) ? parsed : min;
            onChange(Math.min(Math.max(next, min), max));
          }}
          className={`${fieldClass} border-0 bg-transparent text-right tnum focus:ring-0`}
        />
        <span className="text-caption text-ink-400">{suffix}</span>
      </div>
    </SettingsField>
  );
}
