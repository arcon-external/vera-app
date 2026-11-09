import {
  MAX_QUOTATION_DAYS,
  MAX_SUBJECT_LENGTH,
  MIN_QUOTATION_DAYS,
  type QuotationSettings,
} from "../workspaceSettings";
import { SERVICE_CATEGORY_OPTIONS, TAX_TYPE_OPTIONS, TAX_ZONE_OPTIONS } from "../tax";
import { Select } from "./Select";
import { Button, Card, SectionHeading, fieldClass } from "./ui";
import { NumberSetting, SettingsField } from "./WorkspaceSettingsFields";

// Issue #492: the Quotation defaults surface. It is a profile surface like
// Account settings: no workspace tabs, and the shell's back arrow returns Home.
//
// Every change is written as it is made, so there is no unsaved state to lose
// and no second leave guard to add. Clear removes this account's overlay and
// the controls fall back to the built-in defaults.

export function QuotationSettingsPage({
  settings,
  onChange,
  onClear,
}: {
  settings: QuotationSettings;
  onChange: (settings: QuotationSettings) => void;
  onClear: () => void;
}) {
  const patch = (next: Partial<QuotationSettings>) =>
    onChange({ ...settings, ...next });

  return (
    <section aria-labelledby="quotation-settings-heading" className="space-y-6">
      <div>
        <h1
          id="quotation-settings-heading"
          className="text-display font-semibold text-ink-950"
        >
          Quotation settings
        </h1>
        <p className="mt-1.5 max-w-3xl text-body text-ink-500">
          Defaults for each new quotation on this device. Existing drafts and
          issued revisions keep their own values.
        </p>
      </div>

      <Card className="p-5">
        <SectionHeading title="New quotation defaults" />
        <div className="grid gap-4 sm:grid-cols-2">
          <NumberSetting
            label="Validity"
            ariaLabel="Default validity days"
            value={settings.validityDays}
            min={MIN_QUOTATION_DAYS}
            max={MAX_QUOTATION_DAYS}
            suffix="days"
            onChange={(validityDays) => patch({ validityDays })}
          />
          <NumberSetting
            label="Payment terms"
            ariaLabel="Default payment term days"
            value={settings.paymentTermDays}
            min={MIN_QUOTATION_DAYS}
            max={MAX_QUOTATION_DAYS}
            suffix="days"
            onChange={(paymentTermDays) => patch({ paymentTermDays })}
          />
          <SettingsField label="Tax zone">
            <Select
              ariaLabel="Default tax zone"
              value={settings.taxZone}
              options={TAX_ZONE_OPTIONS}
              onChange={(value) =>
                patch({ taxZone: value === "NonFTZ" ? "NonFTZ" : "FTZ" })
              }
            />
          </SettingsField>
          <SettingsField label="Tax type">
            <Select
              ariaLabel="Default tax type"
              value={settings.taxType}
              options={TAX_TYPE_OPTIONS}
              onChange={(value) => {
                if (value === "Services") {
                  patch({
                    taxType: "Services",
                    // The server refuses a service without a category, so the
                    // switch supplies one rather than leaving a default that
                    // cannot be Prepared.
                    serviceCategory: settings.serviceCategory || "NonConstruction",
                  });
                  return;
                }
                patch({ taxType: "Goods", serviceCategory: "" });
              }}
            />
          </SettingsField>
          {settings.taxType === "Services" ? (
            <SettingsField
              label="Service category"
              hint="Sets the internal PPh22 rate; it never prints on the quotation."
            >
              <Select
                ariaLabel="Default service category"
                value={settings.serviceCategory}
                options={SERVICE_CATEGORY_OPTIONS}
                onChange={(value) =>
                  patch({
                    serviceCategory:
                      value === "Construction" ? "Construction" : "NonConstruction",
                  })
                }
              />
            </SettingsField>
          ) : null}
          <div className="sm:col-span-2">
            <SettingsField
              label="Subject"
              hint={`Optional. Up to ${MAX_SUBJECT_LENGTH} characters.`}
            >
              <input
                type="text"
                aria-label="Default subject"
                value={settings.subject}
                maxLength={MAX_SUBJECT_LENGTH}
                placeholder="Supply and installation of electrical and mechanical systems"
                onChange={(event) => patch({ subject: event.target.value })}
                className={fieldClass}
              />
            </SettingsField>
          </div>
        </div>

        <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
          <p className="text-caption text-ink-500">
            Changes save as you edit. They are kept on this device for this
            account only.
          </p>
          <Button aria-label="Clear quotation settings" onClick={onClear}>
            Clear
          </Button>
        </div>
      </Card>
    </section>
  );
}
