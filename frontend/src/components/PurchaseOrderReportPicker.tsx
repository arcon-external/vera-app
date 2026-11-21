import {
  useEffect,
  useId,
  useRef,
  type JSX,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";

// Issue #642: the persistent, fully controlled report picker.
//
// Selection lives outside this module. An item or All click calls back
// immediately and never closes; only the visible X closes. While it stands the
// modal owns focus: the background is inert, pointer input is blocked by the
// scrim, Tab and Shift+Tab wrap inside the dialog, Escape is swallowed rather
// than dismissing, and closing returns focus to the stable trigger node named
// by `returnFocusRef`.
//
// There is no internal open/selection/scroll state. A parent re-render with new
// props therefore updates labels and marks in place instead of remounting the
// dialog, which keeps the focused option focused and the option scroll position
// where the person left it.

export interface ReportPickerItem {
  key: string;
  label: string;
}

export interface PurchaseOrderReportPickerProps {
  title: string;
  open: boolean;
  items: readonly ReportPickerItem[];
  selectedKeys: readonly string[];
  allLabel: string;
  allSelected: boolean;
  onSelectAll: () => void;
  onToggle: (key: string) => void;
  onClose: () => void;
  returnFocusRef: RefObject<HTMLButtonElement | null>;
  pageLabel?: string;
  onPreviousPage?: () => void;
  onNextPage?: () => void;
}

/**
 * The static open-state view. It renders the same surface and controls as the
 * live controller, without a portal or effects, so a render-to-string harness
 * can screenshot the open picker. Interactive behavior always comes from
 * `PurchaseOrderReportPicker`.
 */
export type PurchaseOrderReportPickerViewProps = Omit<
  PurchaseOrderReportPickerProps,
  "open" | "returnFocusRef"
>;

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusableWithin(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
}

function pickerOptionClass(selected: boolean): string {
  return `flex min-h-11 w-full min-w-0 cursor-pointer items-center justify-between gap-2 rounded-lg border px-3 py-2 text-left text-small font-medium transition-colors duration-150 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-500/40 ${
    selected
      ? "border-accent-300 bg-accent-50 text-accent-900 hover:bg-accent-100"
      : "border-rule bg-white text-ink-700 hover:border-ink-300 hover:bg-paper-subtle hover:text-ink-900"
  }`;
}

function CheckMark() {
  return (
    <svg
      data-picker-check
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="shrink-0 text-accent-700"
      aria-hidden="true"
    >
      <path d="M3 8.5l3.5 3.5L13 4.5" />
    </svg>
  );
}

function EmptyCheckSlot() {
  return <span className="h-4 w-4 shrink-0" aria-hidden="true" />;
}

function Chevron({ direction }: { direction: "left" | "right" }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {direction === "left" ? (
        <path d="m15 18-6-6 6-6" />
      ) : (
        <path d="m9 18 6-6-6-6" />
      )}
    </svg>
  );
}

const PAGE_BUTTON_CLASS =
  "flex min-h-11 min-w-11 cursor-pointer items-center justify-center rounded-lg text-ink-600 transition-colors duration-150 ease-out-quart hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40";

function ReportPickerSurface({
  title,
  items,
  selectedKeys,
  allLabel,
  allSelected,
  onSelectAll,
  onToggle,
  onClose,
  pageLabel,
  onPreviousPage,
  onNextPage,
  dialogRef,
}: PurchaseOrderReportPickerViewProps & {
  dialogRef?: RefObject<HTMLDivElement | null>;
}): JSX.Element {
  const titleId = useId();
  const selected = new Set(selectedKeys);
  const showPaging =
    pageLabel !== undefined ||
    onPreviousPage !== undefined ||
    onNextPage !== undefined;

  return (
    <div
      data-report-picker-backdrop
      role="presentation"
      onMouseDown={(event) => {
        // Keep focus inside the dialog when the scrim itself is pressed; the
        // click still does nothing (there is no dismissal on backdrop).
        if (event.target === event.currentTarget) event.preventDefault();
      }}
      className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto overscroll-contain bg-ink-950/45 p-4 sm:p-5"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        data-report-picker
        tabIndex={-1}
        className="my-auto flex max-h-[calc(100dvh-2rem)] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-rule bg-white p-4 shadow-card outline-hidden sm:p-6"
      >
        <div className="flex shrink-0 items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 id={titleId} className="text-title font-semibold text-ink-950">
              {title}
            </h2>
            <p
              data-picker-count
              role="status"
              aria-live="polite"
              className="tnum mt-1 text-caption text-ink-500"
            >
              {selectedKeys.length} selected
            </p>
          </div>
          <button
            type="button"
            data-picker-close
            aria-label="Close picker"
            onClick={onClose}
            className="flex min-h-11 min-w-11 shrink-0 cursor-pointer items-center justify-center rounded-lg border border-rule bg-white text-ink-600 transition-colors duration-150 ease-out-quart hover:border-ink-300 hover:bg-paper-subtle hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <path d="M6 6l12 12M18 6 6 18" />
            </svg>
          </button>
        </div>

        {showPaging ? (
          <div className="mt-4 flex shrink-0 items-center justify-between gap-2 rounded-lg border border-rule bg-paper-subtle px-2 py-1">
            {onPreviousPage !== undefined ? (
              <button
                type="button"
                aria-label="Previous page"
                onClick={onPreviousPage}
                className={PAGE_BUTTON_CLASS}
              >
                <Chevron direction="left" />
              </button>
            ) : (
              <span className="min-h-11 min-w-11" aria-hidden="true" />
            )}
            <span className="min-w-0 break-words text-center text-small font-medium text-ink-700">
              {pageLabel ?? ""}
            </span>
            {onNextPage !== undefined ? (
              <button
                type="button"
                aria-label="Next page"
                onClick={onNextPage}
                className={PAGE_BUTTON_CLASS}
              >
                <Chevron direction="right" />
              </button>
            ) : (
              <span className="min-h-11 min-w-11" aria-hidden="true" />
            )}
          </div>
        ) : null}

        <div
          data-picker-options
          className="mt-4 grid min-h-0 flex-1 content-start grid-cols-1 gap-2 overflow-y-auto overscroll-contain scroll-thin pb-1 sm:grid-cols-2 md:grid-cols-3"
        >
          <button
            type="button"
            data-picker-all
            aria-pressed={allSelected}
            onClick={onSelectAll}
            className={pickerOptionClass(allSelected)}
          >
            <span className="min-w-0 break-words">{allLabel}</span>
            {allSelected ? <CheckMark /> : <EmptyCheckSlot />}
          </button>
          {items.map((item) => {
            const isSelected = selected.has(item.key);
            return (
              <button
                key={item.key}
                type="button"
                data-picker-key={item.key}
                aria-pressed={isSelected}
                onClick={() => onToggle(item.key)}
                className={pickerOptionClass(isSelected)}
              >
                <span className="min-w-0 break-words">{item.label}</span>
                {isSelected ? <CheckMark /> : <EmptyCheckSlot />}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}

export function PurchaseOrderReportPickerView(
  props: PurchaseOrderReportPickerViewProps
): JSX.Element {
  return <ReportPickerSurface {...props} />;
}

function PurchaseOrderReportPickerModal(
  props: PurchaseOrderReportPickerProps
): JSX.Element {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  // Props may change while the dialog is open; the restoration target is only
  // read when it closes, so a re-render cannot re-run the open effect.
  const returnFocus = useRef(props.returnFocusRef);
  returnFocus.current = props.returnFocusRef;

  useEffect(() => {
    const dialog = dialogRef.current;
    if (dialog === null) return;
    const layer = dialog.closest("[data-report-picker-layer]");
    const openedFrom =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    dialog.focus();

    const inerted: HTMLElement[] = [];
    for (const child of Array.from(document.body.children)) {
      if (!(child instanceof HTMLElement)) continue;
      if (child === layer) continue;
      if (child.hasAttribute("inert")) continue;
      child.setAttribute("inert", "");
      inerted.push(child);
    }

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        // X-only dismissal: consume Escape so neither the browser nor a page
        // shortcut treats it as closing this modal.
        event.preventDefault();
        event.stopPropagation();
        return;
      }
      if (event.key !== "Tab") return;
      const focusable = focusableWithin(dialog);
      if (focusable.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      const inside = active instanceof Node && dialog.contains(active);
      if (active === dialog) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
        return;
      }
      if (event.shiftKey && (active === first || !inside)) {
        event.preventDefault();
        last.focus();
        return;
      }
      if (!event.shiftKey && (active === last || !inside)) {
        event.preventDefault();
        first.focus();
      }
    };

    const onFocusIn = (event: FocusEvent) => {
      const target = event.target;
      if (target instanceof Node && dialog.contains(target)) return;
      dialog.focus();
    };

    document.addEventListener("keydown", onKeyDown, true);
    document.addEventListener("focusin", onFocusIn, true);
    const onKeyDownBubble = (event: KeyboardEvent) => {
      // The modal owns the keyboard while it stands. Target/React handlers
      // inside the dialog have already run; stop the event before it reaches
      // window-level page shortcuts (the App registers those on window).
      event.stopPropagation();
    };
    document.addEventListener("keydown", onKeyDownBubble);
    return () => {
      document.removeEventListener("keydown", onKeyDown, true);
      document.removeEventListener("focusin", onFocusIn, true);
      document.removeEventListener("keydown", onKeyDownBubble);
      // Restore the background before restoring focus: an inert trigger cannot
      // take focus back.
      for (const element of inerted) element.removeAttribute("inert");
      const target = returnFocus.current.current ?? openedFrom;
      if (target !== null && target.isConnected) target.focus();
    };
  }, []);

  return <ReportPickerSurface {...props} dialogRef={dialogRef} />;
}

export function PurchaseOrderReportPicker(
  props: PurchaseOrderReportPickerProps
): JSX.Element | null {
  if (!props.open || typeof document === "undefined") return null;
  return createPortal(
    <div data-report-picker-layer>
      <PurchaseOrderReportPickerModal {...props} />
    </div>,
    document.body
  );
}
