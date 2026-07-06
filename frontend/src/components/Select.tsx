import { useState, useRef, useEffect, useCallback } from "react";
import { createPortal } from "react-dom";

interface SelectOption {
  value: string | number;
  label: string;
}

interface SelectProps {
  value: string | number;
  options: readonly SelectOption[];
  onChange: (value: string | number) => void;
  className?: string;
}

export function Select({ value, options, onChange, className = "" }: SelectProps) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState({ top: 0, left: 0, width: 0 });
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  const selectedLabel =
    options.find((o) => o.value === value)?.label ?? "";

  const calcPos = useCallback(() => {
    if (triggerRef.current) {
      const r = triggerRef.current.getBoundingClientRect();
      setPos({ top: r.bottom + 4, left: r.left, width: r.width });
    }
  }, []);

  const toggle = useCallback(() => {
    setOpen((prev) => {
      if (!prev) calcPos();
      return !prev;
    });
  }, [calcPos]);

  const close = useCallback(() => setOpen(false), []);

  const select = useCallback(
    (v: string | number) => {
      onChange(v);
      close();
    },
    [onChange, close]
  );

  useEffect(() => {
    if (!open) return;

    const onClick = (e: MouseEvent) => {
      if (
        menuRef.current &&
        !menuRef.current.contains(e.target as Node) &&
        triggerRef.current &&
        !triggerRef.current.contains(e.target as Node)
      ) {
        close();
      }
    };

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };

    const onResize = () => calcPos();

    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    window.addEventListener("resize", onResize);
    window.addEventListener("scroll", calcPos, true);

    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("resize", onResize);
      window.removeEventListener("scroll", calcPos, true);
    };
  }, [open, close, calcPos]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        onClick={toggle}
        className={`w-full rounded-xl border border-slate-200 bg-white px-4 py-3 text-[14px] text-slate-800 outline-none transition duration-200 hover:border-slate-300 focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/10 flex items-center justify-between gap-2 text-left ${className}`}
      >
        <span className="truncate">{selectedLabel}</span>
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className={`flex-shrink-0 text-slate-400 transition-transform duration-200 ${
            open ? "rotate-180" : ""
          }`}
        >
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>

      {open &&
        createPortal(
          <div
            ref={menuRef}
            style={{
              position: "fixed",
              top: pos.top,
              left: pos.left,
              width: pos.width,
              zIndex: 9999,
            }}
            className="overflow-hidden rounded-xl border border-slate-200/80 bg-white shadow-[0_10px_40px_rgba(0,0,0,0.10),0_2px_8px_rgba(0,0,0,0.06)]"
          >
            <div className="max-h-60 overflow-auto py-1">
              {options.map((option) => (
                <button
                  key={String(option.value)}
                  type="button"
                  onClick={() => select(option.value)}
                  className={`w-full px-4 py-2.5 text-left text-[14px] transition-colors duration-100 ${
                    option.value === value
                      ? "bg-emerald-50 text-emerald-700 font-medium"
                      : "text-slate-700 hover:bg-slate-50 hover:text-slate-900"
                  }`}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>,
          document.body
        )}
    </>
  );
}
