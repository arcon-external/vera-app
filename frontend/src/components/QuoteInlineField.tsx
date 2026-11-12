import { useState } from "react";
import { Select } from "./Select";
import { fieldClass } from "./ui";

export type InlineQuoteMode = "available" | "used" | "original";

export interface InlineQuoteValue {
  value: string;
  mode: InlineQuoteMode;
  onUse: () => void;
  onUseMine: () => void;
}

function borderStyle(mode: InlineQuoteMode) {
  if (mode === "used") {
    return {
      borderColor: "#529873",
      boxShadow: "0 0 0 1px rgba(82, 152, 115, 0.28)",
    };
  }
  if (mode === "available") {
    return {
      borderColor: "#3b82f6",
      boxShadow: "0 0 0 1px rgba(59, 130, 246, 0.22)",
    };
  }
  return undefined;
}

function QuoteBar({ quote }: { quote: InlineQuoteValue }) {
  const using = quote.mode === "used";
  const original = quote.mode === "original";
  return (
    <div
      className="quote-bar-enter absolute bottom-[calc(100%+7px)] left-0 right-0 z-50 rounded-lg border border-accent-200 bg-white shadow-pop"
      onMouseDown={(event) => event.preventDefault()}
    >
      <span
        aria-hidden="true"
        className="absolute -bottom-1 left-8 size-2 rotate-45 border-b border-r border-accent-200 bg-accent-50"
      />
      <div className="flex flex-wrap items-start justify-between gap-2 rounded-t-lg border-b border-accent-100 bg-accent-50/70 px-3 py-2">
        <div>
          <p className="text-[10px] font-semibold uppercase tracking-[0.12em] text-accent-700">
            Value from quotation
          </p>
          <p className="mt-0.5 text-body font-semibold text-ink-950">{quote.value}</p>
        </div>
        <span
          className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[10px] font-medium ${
            using
              ? "border-success-300 bg-success-50 text-success-800"
              : original
                ? "border-rule-strong bg-white text-ink-600"
                : "border-blue-300 bg-blue-50 text-blue-800"
          }`}
        >
          <span className="size-1.5 rounded-full bg-current opacity-70" />
          {using ? "Using quotation value" : original ? "Using your value" : "Not used yet"}
        </span>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
        <p className="text-caption text-ink-500">
          {using
            ? "This field is filled from the quotation."
            : original
              ? "You entered your own value. You can still switch."
              : "Use the quotation’s value for this field."}
        </p>
        {using ? (
          <button
            type="button"
            aria-label="Use my value"
            onClick={quote.onUseMine}
            className="rounded-md border border-rule-strong bg-white px-2.5 py-1.5 text-caption font-medium text-ink-800 hover:bg-paper-subtle"
          >
            Use my value
          </button>
        ) : (
          <button
            type="button"
            aria-label="Use quotation value"
            onClick={quote.onUse}
            className="rounded-md bg-accent-800 px-2.5 py-1.5 text-caption font-medium text-white hover:bg-accent-900"
          >
            Use quotation value
          </button>
        )}
      </div>
    </div>
  );
}

export function InlineQuoteField({
  label,
  value,
  quote,
  type = "text",
  placeholder,
  onChange,
  onFocus,
  onBlur,
  dataTutorial,
}: {
  label: string;
  value: string;
  quote?: InlineQuoteValue;
  type?: "text" | "date" | "email" | "number";
  placeholder?: string;
  onChange: (value: string) => void;
  onFocus?: () => void;
  onBlur?: () => void;
  dataTutorial?: string;
}) {
  const [focused, setFocused] = useState(false);
  return (
    <div className="relative grid gap-1.5">
      <span className="text-small font-medium text-ink-700">{label}</span>
      <input
        type={type}
        aria-label={label}
        data-tutorial={dataTutorial}
        className={fieldClass}
        style={quote === undefined ? undefined : borderStyle(quote.mode)}
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        onFocus={() => {
          setFocused(true);
          onFocus?.();
        }}
        onBlur={() => {
          setFocused(false);
          onBlur?.();
        }}
      />
      {focused && quote !== undefined ? <QuoteBar quote={quote} /> : null}
    </div>
  );
}

export function InlineQuoteSelect({
  label,
  value,
  options,
  quote,
  onChange,
  onFocus,
  onBlur,
}: {
  label: string;
  value: string;
  options: readonly { value: string; label: string }[];
  quote?: InlineQuoteValue;
  onChange: (value: string) => void;
  onFocus?: () => void;
  onBlur?: () => void;
}) {
  const [focused, setFocused] = useState(false);
  return (
    <div className="relative grid gap-1.5">
      <span className="text-small font-medium text-ink-700">{label}</span>
      <div
        className="rounded-md border"
        style={quote === undefined ? undefined : borderStyle(quote.mode)}
        onFocusCapture={() => {
          setFocused(true);
          onFocus?.();
        }}
        onBlurCapture={() => {
          setFocused(false);
          onBlur?.();
        }}
      >
        <Select
          ariaLabel={label}
          value={value}
          options={options}
          onChange={(next) => onChange(String(next))}
          className={quote === undefined ? undefined : "border-transparent shadow-none focus:ring-0"}
        />
      </div>
      {focused && quote !== undefined ? <QuoteBar quote={quote} /> : null}
    </div>
  );
}
