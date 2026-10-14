import { useRef } from "react";
import type { JSX } from "react";
import type { CompanyForm } from "../company";
import { normalizeCurrencyCode } from "../drafts";
import type { CurrencyInfo } from "../format";
import { Button, Card, SectionHeading, fieldClass } from "./ui";

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
// without it). ItemEditor's copy of this idiom is missing it; see the report.
const currencyChevronStyle = {
  backgroundImage:
    "url(\"data:image/svg+xml;charset=UTF-8,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23767469' stroke-width='2' stroke-linecap='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")",
} as const;

// Presentational slice of the company form: the parent (CompanyProfileCard)
// owns all state and the whole-record save; see profilePayload in
// frontend/src/company.ts for why a slice must never save on its own.
// This panel holds no state — the file-input ref below is a DOM detail,
// not state. The currency catalogue is fetched by the parent for the same
// reason: a fetch here would give this slice loading state, and a slice
// with loading state is a slice that eventually saves on its own.
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
  /** The stored company currency code, verbatim from StoredCompanyExtras.
   *  Displayed normalized (a never-set profile stores "", which reads IDR). */
  currency: string;
  /** The shipped catalogue from GetCurrencyCatalogue, or null while loading.
   *  A null catalogue disables the picker but still shows the current code. */
  catalogue: CurrencyInfo[] | null;
  /** Called with the picked code. The parent writes it into extras, and the
   *  existing whole-record save round-trips it via profilePayload. */
  onCurrencyChange: (code: string) => void;
}): JSX.Element {
  const { form, onFieldChange, logoPreview, hasLogo, logoBusy, logoMeta, onPickLogo, onRemoveLogo, currency, catalogue, onCurrencyChange } = props;
  const fileInputRef = useRef<HTMLInputElement>(null);
  const displayCode = normalizeCurrencyCode(currency);
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
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">Company currency</span>
            <select
              value={displayCode}
              disabled={catalogue === null}
              onChange={(event) => onCurrencyChange(event.target.value)}
              className={`${fieldClass} cursor-pointer appearance-none bg-[length:10px] bg-[right_0.6rem_center] bg-no-repeat pr-7`}
              style={currencyChevronStyle}
              aria-label="Company currency"
            >
              {catalogue === null ? (
                <option value={displayCode}>{displayCode}</option>
              ) : (
                catalogue.map((entry) => (
                  <option key={entry.code} value={entry.code}>
                    {entry.code} — {entry.symbol}
                  </option>
                ))
              )}
            </select>
          </label>
          {/* No save button here by design: the page keeps a single shared
              footer so the record can be saved from any of the three sub-tabs.
              A sub-tab that cannot save is a dead end. */}
        </div>
        <p className="mt-3 text-caption leading-relaxed text-ink-400">
          Only admins see this section. The quotation header prints this name
          and address on every new quotation. Amounts on new quotations print
          in the company currency; past revisions keep whatever printed.
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
