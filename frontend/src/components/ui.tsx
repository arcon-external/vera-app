import type { ButtonHTMLAttributes, JSX, ReactNode } from "react";

type ButtonVariant = "primary" | "dark" | "secondary" | "ghost" | "danger";
type ButtonSize = "xs" | "sm" | "md";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  leading?: ReactNode;
}

const base =
  "inline-flex cursor-pointer items-center justify-center gap-2 rounded-lg font-medium transition-[transform,background-color,border-color,color,opacity] duration-150 ease-out-quart active:scale-[0.98] focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50 disabled:cursor-not-allowed disabled:opacity-45 disabled:active:scale-100";

const variants: Record<ButtonVariant, string> = {
  primary:
    "bg-accent-700 text-white hover:bg-accent-600 disabled:bg-ink-200 disabled:text-ink-500",
  dark:
    "bg-ink-900 text-white hover:bg-ink-800 disabled:bg-ink-200 disabled:text-ink-500",
  secondary:
    "border border-rule-strong bg-white text-ink-700 hover:border-ink-300 hover:text-ink-900",
  ghost:
    "text-ink-500 hover:bg-paper-muted hover:text-ink-900",
  danger:
    "border border-danger-200 bg-white text-danger-600 hover:bg-danger-50",
};

const sizes: Record<ButtonSize, string> = {
  xs: "min-h-8 px-3 py-1.5 text-small",
  sm: "min-h-9 px-3.5 py-2.5 text-small",
  md: "min-h-10 px-4 py-3 text-small",
};

/** The class for a standard text input in the settings forms. Shared so the
 *  panels and the page cannot drift apart. */
export const fieldClass =
  "w-full rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-hidden transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20";

// The default is `type="button"`, not the HTML default. A bare <button> is a
// submit button, so inside a form the common case -- a button that just does
// something -- submits by accident: the logo picker never opened with a
// required field empty (#338), and two Cancel buttons created the record on
// the way out (#342). A button that means to submit says type="submit".
export function Button({
  variant = "secondary",
  size = "md",
  leading,
  className = "",
  children,
  type = "button",
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      className={`${base} ${variants[variant]} ${sizes[size]} ${className}`}
      {...rest}
    >
      {leading}
      {children}
    </button>
  );
}

export function Card({
  className = "",
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={`rounded-xl border border-rule bg-white shadow-card ${className}`}>
      {children}
    </div>
  );
}

export function SectionHeading({
  title,
  hint,
  action,
}: {
  title: string;
  hint?: string;
  action?: ReactNode;
}) {
  return (
    <div className="mb-3 flex items-center gap-3">
      <h2 className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
        {title}
      </h2>
      <div className="h-px flex-1 bg-rule" />
      {hint ? <span className="tnum text-caption text-ink-500">{hint}</span> : null}
      {action}
    </div>
  );
}

export type SaveState = "saved" | "dirty" | "saving";

/** The live save readout, shared by the four Company settings save rows and
 *  the wording panel's heading. Wrapped in role="status" so the state change is
 *  announced rather than only repainted. */
export function SaveStateChip({ state }: { state: SaveState }): JSX.Element {
  if (state === "saving") {
    return (
      <span role="status" className="flex items-center">
        <Badge tone="internal">Saving…</Badge>
      </span>
    );
  }
  if (state === "dirty") {
    return (
      <span role="status" className="flex items-center">
        <Badge tone="accent">Unsaved changes</Badge>
      </span>
    );
  }
  return (
    <span role="status" className="flex items-center gap-1.5 text-caption text-ink-400">
      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
        <path d="M3 8.5l3.2 3.2L13 5" />
      </svg>
      Saved
    </span>
  );
}

/**
 * The one save row every Company settings tab renders (#432): a primary
 * button with a busy label, the standing reason the save is refused when the
 * store would bounce it (otherwise the live Save chip), and a failed save's
 * reason in a banner under the row.
 *
 * It exists so the four tabs cannot drift into four shapes — the old footer
 * button saved everything from everywhere; now each tab saves itself, and this
 * is what "saving itself" looks like. The Units tab's row is the precedent it
 * was extracted from.
 */
export function SaveRow({
  label,
  busyLabel,
  busy,
  state,
  onSave,
  invalid = null,
  error = null,
  hint = null,
  disabled = false,
}: {
  /** The idle button label, e.g. "Save wording and colours". */
  label: string;
  /** The busy button label, e.g. "Saving…". */
  busyLabel: string;
  /** True while the tab's save is in flight. */
  busy: boolean;
  /** The live save state, shown as the chip beside the button. */
  state: SaveState;
  /** Runs the tab's save. */
  onSave: () => void;
  /** Why the save is unavailable: the store would refuse it. Shown inline
   *  beside the button, in place of the chip, and the button is disabled. */
  invalid?: string | null;
  /** A failed save's reason, shown in a banner under the row. */
  error?: string | null;
  /** A standing note under the row when there is nothing to report. */
  hint?: string | null;
  /** Extra reason to disable the button (a load still in flight). */
  disabled?: boolean;
}): JSX.Element {
  const refused = invalid !== null && invalid !== "";
  return (
    <>
      <div className="mt-3 flex flex-wrap items-center gap-3">
        <Button variant="primary" onClick={onSave} disabled={busy || disabled || refused}>
          {busy ? busyLabel : label}
        </Button>
        {refused ? (
          <span className="text-caption text-ink-500">{invalid}</span>
        ) : (
          <SaveStateChip state={state} />
        )}
      </div>
      {error !== null && error !== "" ? (
        <div className="mt-3">
          <StatusBanner tone="danger">{error}</StatusBanner>
        </div>
      ) : hint !== null && hint !== "" ? (
        <p className="mt-2 text-caption leading-relaxed text-ink-400">{hint}</p>
      ) : null}
    </>
  );
}

type BannerTone = "info" | "danger";

export function StatusBanner({
  tone,
  children,
}: {
  tone: BannerTone;
  children: ReactNode;
}) {
  const tones: Record<BannerTone, string> = {
    info: "border-rule bg-paper-subtle text-ink-700",
    danger: "border-danger-200 bg-danger-50 text-danger-700",
  };
  return (
    <div
      role={tone === "danger" ? "alert" : "status"}
      className={`animate-status-in rounded-lg border px-4 py-2.5 text-small ${tones[tone]}`}
    >
      {children}
    </div>
  );
}

export function Badge({
  tone = "neutral",
  children,
}: {
  tone?: "neutral" | "internal" | "success" | "accent";
  children: ReactNode;
}) {
  const tones = {
    neutral: "border-rule bg-paper-subtle text-ink-500",
    internal: "border-internal-200 bg-internal-50 text-internal-700",
    success: "border-success-200 bg-success-50 text-success-700",
    accent: "border-accent-200 bg-accent-50 text-accent-700",
  } as const;
  return (
    <span
      className={`inline-flex items-center rounded-md border px-2 py-0.5 text-micro font-medium uppercase ${tones[tone]}`}
    >
      {children}
    </span>
  );
}
