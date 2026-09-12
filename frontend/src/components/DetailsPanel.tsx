import type { ReactNode } from "react";
import { Card, SectionHeading } from "./ui";
import { Select } from "./Select";
import { taxRateLabel } from "../tax";

export interface DetailsState {
  subject: string;
  recipientName: string;
  validity: number;
  paymentTerm: number;
  leadTime: number;
  discountPercent: number;
  incoterm: string;
  taxZone: string;
  taxCategory: string;
}

interface DetailsPanelProps {
  value: DetailsState;
  customerOptions: readonly { value: string | number; label: string }[];
  customerId: number;
  incotermOptions: readonly { value: string | number; label: string }[];
  taxZoneOptions: readonly { value: string; label: string }[];
  taxCategoryOptions: readonly { value: string; label: string }[];
  onChange: (patch: Partial<DetailsState>) => void;
  onCustomerChange: (value: number) => void;
  onPaste: (event: React.ClipboardEvent<HTMLInputElement>) => void;
}

const fieldClass =
  "w-full rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-none transition duration-150 ease-out-quart placeholder:text-ink-400 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20";

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="flex items-baseline justify-between gap-2">
        <span className="text-caption font-medium text-ink-600">{label}</span>
        {hint ? <span className="text-caption text-ink-400">{hint}</span> : null}
      </span>
      {children}
    </div>
  );
}

function NumberField({
  label,
  hint,
  value,
  suffix = "days",
  max,
  onChange,
}: {
  label: string;
  hint?: string;
  value: number;
  suffix?: string;
  max?: number;
  onChange: (value: number) => void;
}) {
  return (
    <Field label={label} hint={hint}>
      <div className="flex items-center rounded-md border border-rule bg-white pr-3 transition duration-150 focus-within:border-accent-400 focus-within:ring-2 focus-within:ring-accent-500/20">
        <input
          type="text"
          inputMode="numeric"
          value={value}
          onChange={(event) => {
            const parsed = Number.parseInt(event.target.value.replace(/\D/g, ""), 10);
            const next = Number.isFinite(parsed) ? parsed : 0;
            onChange(max !== undefined ? Math.min(next, max) : next);
          }}
          className="w-full bg-transparent px-3 py-2 text-right text-body tnum text-ink-900 outline-none"
        />
        <span className="text-caption text-ink-400">{suffix}</span>
      </div>
    </Field>
  );
}

export function DetailsPanel({
  value,
  customerOptions,
  customerId,
  incotermOptions,
  taxZoneOptions,
  taxCategoryOptions,
  onChange,
  onCustomerChange,
  onPaste,
}: DetailsPanelProps) {
  const localizedTax =
    value.taxZone === "FTZ"
      ? "0%"
      : taxRateLabel(value.taxZone, value.taxCategory);

  return (
    <Card className="p-5">
      <SectionHeading title="Quotation details" />

      <div className="grid gap-5 lg:grid-cols-2">
        <div className="lg:col-span-2">
          <Field label="Subject" hint="paste from Excel to fill all details">
            <input
              type="text"
              placeholder="Supply and installation of electrical and mechanical systems"
              value={value.subject}
              onChange={(event) => onChange({ subject: event.target.value })}
              onPaste={onPaste}
              className={fieldClass}
            />
          </Field>
        </div>

        <Field label="Customer">
          <Select
            value={customerId}
            options={customerOptions}
            ariaLabel="Customer"
            onChange={(next) => onCustomerChange(Number(next))}
          />
        </Field>

        <Field label="Recipient name">
          <input
            type="text"
            placeholder="Full name of the recipient"
            value={value.recipientName}
            onChange={(event) => onChange({ recipientName: event.target.value })}
            className={fieldClass}
          />
        </Field>

        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:col-span-2 lg:grid-cols-5">
          <NumberField
            label="Validity"
            value={value.validity}
            onChange={(validity) => onChange({ validity })}
          />
          <NumberField
            label="Payment"
            value={value.paymentTerm}
            onChange={(paymentTerm) => onChange({ paymentTerm })}
          />
          <NumberField
            label="Lead time"
            value={value.leadTime}
            onChange={(leadTime) => onChange({ leadTime })}
          />
          <NumberField
            label="Discount"
            suffix="%"
            max={100}
            value={value.discountPercent}
            onChange={(discountPercent) => onChange({ discountPercent })}
          />
          <Field label="Incoterm">
            <Select
              value={value.incoterm}
              options={incotermOptions}
              ariaLabel="Incoterm"
              onChange={(next) => onChange({ incoterm: String(next) })}
            />
          </Field>
        </div>

        <div className="lg:col-span-2">
          <div className="grid items-start gap-5 border-t border-rule pt-5 lg:grid-cols-2">
            <div>
              <span className="mb-1.5 block text-caption font-medium text-ink-600">
                Tax zone
              </span>
              <div
                role="radiogroup"
                aria-label="Tax zone"
                className="inline-flex rounded-lg border border-rule bg-paper-muted p-0.5"
              >
                {taxZoneOptions.map((option) => {
                  const selected = value.taxZone === option.value;
                  return (
                    <button
                      key={option.value}
                      type="button"
                      role="radio"
                      aria-checked={selected}
                      onClick={() =>
                        onChange({
                          taxZone: option.value,
                          taxCategory: option.value === "FTZ" ? "" : value.taxCategory,
                        })
                      }
                      className={`min-h-8 rounded-md px-3 py-1.5 text-small font-medium transition-colors duration-150 ease-out-quart focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
                        selected
                          ? "bg-white text-ink-950 shadow-card"
                          : "text-ink-500 hover:text-ink-800"
                      }`}
                    >
                      {option.label}
                    </button>
                  );
                })}
              </div>
              <p className="mt-2 text-caption text-ink-500">
                Withholding tax applies to the grand total, not individual
                lines. Current rate {localizedTax}.
              </p>
            </div>

            {value.taxZone === "NonFTZ" ? (
              <Field label="Tax category">
                <Select
                  value={value.taxCategory}
                  ariaLabel="Tax category"
                  options={[
                    { value: "", label: "Select category" },
                    ...taxCategoryOptions.map((option) => ({
                      value: option.value,
                      label: `${option.label} — ${taxRateLabel("NonFTZ", option.value)}`,
                    })),
                  ]}
                  onChange={(next) => onChange({ taxCategory: String(next) })}
                />
              </Field>
            ) : (
              <div className="rounded-lg border border-rule bg-paper-subtle px-4 py-3.5">
                <p className="text-small font-medium text-ink-700">
                  Free trade zone (FTZ)
                </p>
                <p className="mt-1 text-caption leading-relaxed text-ink-500">
                  No withholding tax is deducted, so the grand total is also
                  the nett amount payable.
                </p>
                <p className="mt-2 border-t border-rule pt-2 text-caption leading-relaxed text-ink-500">
                  Use Non-FTZ for work outside the free trade zone.
                </p>
              </div>
            )}
          </div>
        </div>
      </div>
    </Card>
  );
}
