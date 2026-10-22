import {
  MAX_PO_LIST_PAGE_SIZE,
  MIN_PO_LIST_PAGE_SIZE,
  SETTINGS_INCOTERMS,
  todayWIB,
  type PurchaseOrderSettings,
} from "../workspaceSettings";
import { MAX_PAYMENT_TERMS_LENGTH } from "../purchaseOrder";
import { Select } from "./Select";
import { Button, Card, SectionHeading, fieldClass } from "./ui";
import { NumberSetting, SettingsField } from "./WorkspaceSettingsFields";

// Issue #492: the Purchase Order defaults surface. A profile surface like
// Account settings: no workspace tabs, and the shell's back arrow returns Home.
//
// A PO draft is always born from an issued quotation revision, so these are
// fallbacks, not replacements: payment terms and Incoterms only fill a field
// the revision left empty, and the received date is a PO fact the quotation
// never supplies. Every change is written as it is made.

const INCOTERM_OPTIONS = SETTINGS_INCOTERMS.map((value) => ({
  value,
  label: value,
}));

export function PurchaseOrderSettingsPage({
  settings,
  onChange,
  onClear,
}: {
  settings: PurchaseOrderSettings;
  onChange: (settings: PurchaseOrderSettings) => void;
  onClear: () => void;
}) {
  const patch = (next: Partial<PurchaseOrderSettings>) =>
    onChange({ ...settings, ...next });

  return (
    <section aria-labelledby="po-settings-heading" className="space-y-6">
      <div>
        <h1
          id="po-settings-heading"
          className="text-display font-semibold text-ink-950"
        >
          Purchase Order settings
        </h1>
        <p className="mt-1.5 max-w-3xl text-body text-ink-500">
          Defaults for new purchase orders on this device. A value the quotation
          revision supplies is always kept; existing records are unchanged.
        </p>
      </div>

      <Card className="p-5">
        <SectionHeading title="New purchase order defaults" />
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <SettingsField
              label="Payment terms"
              hint={`Free text, up to ${MAX_PAYMENT_TERMS_LENGTH} characters. Used only when the quotation carries no payment terms.`}
            >
              <textarea
                aria-label="Default payment terms"
                value={settings.paymentTerms}
                maxLength={MAX_PAYMENT_TERMS_LENGTH}
                placeholder="e.g. 50% advance, balance 30 days after delivery"
                onChange={(event) => patch({ paymentTerms: event.target.value })}
                className={`${fieldClass} min-h-20 resize-y`}
              />
            </SettingsField>
          </div>
          <SettingsField
            label="Incoterms"
            hint="Used only when the quotation carries none."
          >
            <Select
              ariaLabel="Default Incoterms"
              value={settings.incoterms}
              options={INCOTERM_OPTIONS}
              onChange={(value) => patch({ incoterms: String(value) })}
            />
          </SettingsField>
          <SettingsField
            label="Received date"
            hint="Leave blank to use today in WIB when a draft starts."
          >
            <input
              type="date"
              aria-label="Default received date"
              value={settings.receivedDate}
              onChange={(event) => patch({ receivedDate: event.target.value })}
              className={fieldClass}
            />
            {settings.receivedDate === "" ? (
              <span className="text-caption text-ink-400">
                Today in WIB is <span className="tnum">{todayWIB()}</span>.
              </span>
            ) : null}
          </SettingsField>
          <NumberSetting
            label="List page size"
            ariaLabel="Default PO list page size"
            value={settings.listPageSize}
            min={MIN_PO_LIST_PAGE_SIZE}
            max={MAX_PO_LIST_PAGE_SIZE}
            suffix="rows"
            hint="Applies to the Purchase Order Drafts and Archive lists. Default 15, from 1 to 100."
            onChange={(listPageSize) => patch({ listPageSize })}
          />
        </div>

        <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-rule pt-4">
          <p className="text-caption text-ink-500">
            Changes save as you edit. They are kept on this device for this
            account only.
          </p>
          <Button aria-label="Clear purchase order settings" onClick={onClear}>
            Clear
          </Button>
        </div>
      </Card>
    </section>
  );
}
