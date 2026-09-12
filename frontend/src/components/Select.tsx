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
  ariaLabel?: string;
}

const MENU_MAX_HEIGHT = 256; // matches max-h-64
const MENU_GAP = 4;
const VIEWPORT_MARGIN = 12;

export function Select({
  value,
  options,
  onChange,
  className = "",
  ariaLabel,
}: SelectProps) {
  const [open, setOpen] = useState(false);
  const [highlighted, setHighlighted] = useState(0);
  const [pos, setPos] = useState({ top: 0, left: 0, width: 0 });
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const listId = useRef(`select-${Math.random().toString(36).slice(2, 9)}`);

  const selectedIndex = options.findIndex((option) => option.value === value);
  const selectedLabel = selectedIndex >= 0 ? options[selectedIndex].label : "";

  const calcPos = useCallback(() => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    const spaceBelow = window.innerHeight - rect.bottom;
    const flip = spaceBelow < MENU_MAX_HEIGHT + MENU_GAP + VIEWPORT_MARGIN;
    setPos({
      top: flip
        ? Math.max(VIEWPORT_MARGIN, rect.top - MENU_MAX_HEIGHT - MENU_GAP)
        : rect.bottom + MENU_GAP,
      left: rect.left,
      width: rect.width,
    });
  }, []);

  const openMenu = useCallback(() => {
    calcPos();
    setHighlighted(selectedIndex >= 0 ? selectedIndex : 0);
    setOpen(true);
  }, [calcPos, selectedIndex]);

  const close = useCallback(() => setOpen(false), []);

  const commit = useCallback(
    (index: number) => {
      const option = options[index];
      if (!option) return;
      onChange(option.value);
      close();
      triggerRef.current?.focus();
    },
    [options, onChange, close]
  );

  const onTriggerKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLButtonElement>) => {
      if (!open) {
        if (event.key === "ArrowDown" || event.key === "ArrowUp" || event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          openMenu();
        }
        return;
      }
      switch (event.key) {
        case "ArrowDown":
          event.preventDefault();
          setHighlighted((current) => Math.min(current + 1, options.length - 1));
          break;
        case "ArrowUp":
          event.preventDefault();
          setHighlighted((current) => Math.max(current - 1, 0));
          break;
        case "Home":
          event.preventDefault();
          setHighlighted(0);
          break;
        case "End":
          event.preventDefault();
          setHighlighted(options.length - 1);
          break;
        case "Enter":
        case " ":
          event.preventDefault();
          commit(highlighted);
          break;
        case "Escape":
        case "Tab":
          close();
          break;
        default:
          break;
      }
    },
    [open, openMenu, options.length, commit, highlighted, close]
  );

  useEffect(() => {
    if (!open) return;

    const onClick = (event: MouseEvent) => {
      if (
        menuRef.current &&
        !menuRef.current.contains(event.target as Node) &&
        triggerRef.current &&
        !triggerRef.current.contains(event.target as Node)
      ) {
        close();
      }
    };

    const onResize = () => calcPos();

    document.addEventListener("mousedown", onClick);
    window.addEventListener("resize", onResize);
    window.addEventListener("scroll", calcPos, true);

    return () => {
      document.removeEventListener("mousedown", onClick);
      window.removeEventListener("resize", onResize);
      window.removeEventListener("scroll", calcPos, true);
    };
  }, [open, close, calcPos]);

  // Keep the highlighted option visible while navigating with the keyboard.
  useEffect(() => {
    if (!open) return;
    const node = menuRef.current?.querySelector<HTMLElement>(
      `[data-index="${highlighted}"]`
    );
    node?.scrollIntoView({ block: "nearest" });
  }, [open, highlighted]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        aria-label={ariaLabel}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId.current : undefined}
        aria-activedescendant={open ? `${listId.current}-${highlighted}` : undefined}
        onClick={() => (open ? close() : openMenu())}
        onKeyDown={onTriggerKeyDown}
        className={`flex w-full items-center justify-between gap-2 rounded-md border border-rule bg-white px-3 py-2 text-left text-body text-ink-900 outline-none transition duration-150 ease-out-quart hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20 ${className}`}
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
          className={`flex-shrink-0 text-ink-400 transition-transform duration-150 ease-out-quart ${
            open ? "rotate-180" : ""
          }`}
        >
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>

      {open
        ? createPortal(
            <div
              ref={menuRef}
              id={listId.current}
              role="listbox"
              tabIndex={-1}
              style={{
                position: "fixed",
                top: pos.top,
                left: pos.left,
                width: pos.width,
                maxHeight: MENU_MAX_HEIGHT,
                zIndex: 60,
              }}
              className="animate-toast-in overflow-auto scroll-thin rounded-lg border border-rule bg-white py-1 shadow-pop"
            >
              {options.map((option, index) => {
                const selected = option.value === value;
                const active = index === highlighted;
                return (
                  <button
                    key={String(option.value)}
                    id={`${listId.current}-${index}`}
                    data-index={index}
                    type="button"
                    role="option"
                    aria-selected={selected}
                    onMouseEnter={() => setHighlighted(index)}
                    onClick={() => commit(index)}
                    className={`flex w-full items-center justify-between gap-2 px-3 py-2.5 text-left text-body transition-colors duration-100 ${
                      active
                        ? "bg-accent-50 font-medium text-accent-800"
                        : "text-ink-700"
                    }`}
                  >
                    <span className="truncate">{option.label}</span>
                    {selected ? (
                      <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                        <path d="M3 8.5l3.5 3.5L13 4.5" />
                      </svg>
                    ) : null}
                  </button>
                );
              })}
            </div>,
            document.body
          )
        : null}
    </>
  );
}
