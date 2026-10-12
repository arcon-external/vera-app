import { useRef } from "react";
import type { JSX } from "react";
import type { CompanyForm } from "../company";
import { Button, Card, SectionHeading, fieldClass } from "./ui";

// Presentational slice of the company form: the parent (CompanyProfileCard)
// owns all state and the whole-record save; see profilePayload in
// frontend/src/company.ts for why a slice must never save on its own.
// This panel holds no state — the file-input ref below is a DOM detail,
// not state.
export function CompanyGeneralPanel(props: {
  /** The two edited profile fields. The parent owns this state. */
  form: CompanyForm;
  /** Called with the field name and its new value. The parent applies it
   *  with the setForm updater at the CompanyGeneralPanel call site in
   *  CompanyProfileCard. */
  onFieldChange: (field: keyof CompanyForm, value: string) => void;
  /** Data-URL preview of the current logo, or null. */
  logoPreview: string | null;
  /** True when a custom (non-built-in) logo is stored. */
  hasLogo: boolean;
  /** True while an upload or removal is in flight. */
  logoBusy: boolean;
  /** The caption under the logo copy, e.g. "PNG · 12.3 KB · 240 × 120 px". */
  logoMeta: string | null;
  /** The file chosen in the hidden picker. The parent validates and uploads it. */
  onPickLogo: (file: File | undefined) => void;
  onRemoveLogo: () => void;
}): JSX.Element {
  const { form, onFieldChange, logoPreview, hasLogo, logoBusy, logoMeta, onPickLogo, onRemoveLogo } = props;
  const fileInputRef = useRef<HTMLInputElement>(null);
  return (
    <div className="grid items-stretch gap-6 lg:grid-cols-2">
      <Card className="p-5">
        <SectionHeading title="Company profile" />
        <div className="grid gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">Legal name</span>
            <input
              type="text"
              required
              maxLength={160}
              value={form.legalName}
              onChange={(event) => onFieldChange("legalName", event.target.value)}
              placeholder="PT Arcon Penta Persada"
              className={fieldClass}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">Address</span>
            <input
              type="text"
              value={form.address}
              onChange={(event) => onFieldChange("address", event.target.value)}
              placeholder="Street, city, province"
              className={fieldClass}
            />
          </label>
          {/* No save button here by design: the page keeps a single shared
              footer so the record can be saved from any of the three sub-tabs.
              A sub-tab that cannot save is a dead end. */}
        </div>
        <p className="mt-3 text-caption leading-relaxed text-ink-400">
          Only admins see this section. The quotation header prints this name
          and address on every new quotation.
        </p>
      </Card>
      <Card className="flex h-full flex-col p-5">
        <SectionHeading title="Logo" />
        <div className="flex flex-1 flex-col justify-center gap-4 sm:flex-row sm:items-start">
          <span className="flex h-28 w-44 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-rule bg-white p-2" aria-hidden="true">
            {logoPreview ? (
              <img src={logoPreview} alt="" className="h-full w-full object-contain" />
            ) : (
              <span className="px-2 text-center text-caption text-ink-400">Built-in logo</span>
            )}
          </span>
          <div className="grid flex-1 content-start gap-2">
            <p className="max-w-xl text-body text-ink-600">
              {hasLogo
                ? "Custom logo prints on new quotations and previews."
                : "Built-in logo prints on new quotations and previews."}
            </p>
            <p className="text-caption text-ink-500">
              Shows in the quotation header.
              {logoMeta ? <span className="tnum"> {logoMeta}</span> : null}
            </p>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              {/* Explicit type: a bare button inside the shared profile form
                  would submit it, firing validation and a spurious save. */}
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => fileInputRef.current?.click()}
                disabled={logoBusy}
              >
                {logoBusy ? "Uploading…" : hasLogo ? "Replace logo" : "Upload logo"}
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/gif,image/webp"
                className="sr-only"
                tabIndex={-1}
                aria-hidden="true"
                disabled={logoBusy}
                onChange={(event) => {
                  onPickLogo(event.target.files?.[0]);
                  event.target.value = "";
                }}
              />
              {hasLogo ? (
                <button
                  type="button"
                  onClick={() => onRemoveLogo()}
                  disabled={logoBusy}
                  className="text-caption text-ink-400 hover:text-danger-600 hover:underline disabled:opacity-50"
                >
                  Remove
                </button>
              ) : null}
            </div>
          </div>
        </div>
        <p className="mt-3 text-caption leading-relaxed text-ink-400">
          PNG, JPEG, GIF, or WebP under 512KB. Past revisions keep whatever printed.
        </p>
      </Card>
    </div>
  );
}
