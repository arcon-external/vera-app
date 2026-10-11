import type { ButtonHTMLAttributes, ReactNode } from "react";

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

export function Button({
  variant = "secondary",
  size = "md",
  leading,
  className = "",
  children,
  ...rest
}: ButtonProps) {
  return (
    <button className={`${base} ${variants[variant]} ${sizes[size]} ${className}`} {...rest}>
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
