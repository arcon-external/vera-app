import { useRef, type JSX, type KeyboardEvent } from "react";

export type TabItem = { id: string; label: string };

/** The id a panel should carry so the matching tab points at it. */
export function tabPanelId(idPrefix: string, id: string): string {
  return `${idPrefix}-panel-${id}`;
}

/** The id of the tab that controls that panel. */
export function tabId(idPrefix: string, id: string): string {
  return `${idPrefix}-tab-${id}`;
}

/**
 * Which tab a keypress selects, per the WAI-ARIA tabs pattern this bar
 * implements: ArrowLeft/ArrowRight move to the previous/next tab wrapping
 * around the ends, Home/End jump to the first/last. Returns null for any
 * other key so the caller lets the browser keep it.
 *
 * Kept a pure function of (items, active, key) on purpose: it is the whole
 * keyboard contract, and the node test suite can run it without a DOM. The
 * handler wiring around it (onKeyDown -> onSelect -> focus()) still needs the
 * browser and stays hand-driven.
 *
 * An `active` that matches no item starts from the first tab, matching where
 * the roving tabindex parks its single tab stop.
 */
export function tabAfterKey(items: TabItem[], active: string, key: string): string | null {
  const count = items.length;
  if (count === 0) return null;
  const current = items.findIndex((tab) => tab.id === active);
  const from = current === -1 ? 0 : current;
  let next: number;
  switch (key) {
    case "ArrowLeft":
      next = (from + count - 1) % count;
      break;
    case "ArrowRight":
      next = (from + 1) % count;
      break;
    case "Home":
      next = 0;
      break;
    case "End":
      next = count - 1;
      break;
    default:
      return null;
  }
  return items[next].id;
}

const TABLIST_CLASS = "inline-flex rounded-lg border border-rule bg-paper-muted p-0.5";

/**
 * One tab bar for both the top-level Settings tabs and the Company sub-tabs.
 * The wrapper class is fixed on purpose: every tab bar in Settings must look
 * identical, so there is deliberately no per-caller `className` override.
 *
 * It follows the full WAI-ARIA tabs pattern with automatic activation: a
 * roving tabindex (only the active tab is in the tab order) and
 * ArrowLeft/ArrowRight with wrapping plus Home/End on the tablist, with
 * selection following focus. Both call sites get that from here, so the
 * keyboard cannot work in one bar and not the other.
 */
export function TabBar(props: {
  /** The tabs to render, in order. Already filtered by the caller. */
  items: TabItem[];
  /** The id of the selected tab. */
  active: string;
  onSelect: (id: string) => void;
  /** The accessible name for the tablist, e.g. "Settings sections". */
  label: string;
  /**
   * When set, each tab gets `id={idPrefix + "-tab-" + item.id}` and
   * `aria-controls={idPrefix + "-panel-" + item.id}`, so the panel it controls
   * can be wired with aria-labelledby. Omit it and the markup is exactly what
   * the top-level bar renders today.
   */
  idPrefix?: string;
}): JSX.Element {
  const { items, active, onSelect, label, idPrefix } = props;
  const tablistRef = useRef<HTMLDivElement>(null);

  // Where the selection sits. A caller whose `active` matches no item (a tab
  // it filtered away, a stale state) still needs exactly one tab stop --
  // zero would strand keyboard users outside the tablist entirely.
  const activeIndex = items.findIndex((tab) => tab.id === active);
  const stopIndex = activeIndex === -1 ? 0 : activeIndex;

  // Selection follows focus: the selected tab moves, then focus moves to the
  // node that is now selected. The buttons stay mounted while `active`
  // changes -- only their tabIndex/aria-selected move -- so the node exists
  // now and keeps DOM focus through the re-render.
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const nextId = tabAfterKey(items, active, event.key);
    if (nextId === null) return;
    // Otherwise arrows/Home/End scroll the page from inside the tablist.
    event.preventDefault();
    const tabs = tablistRef.current?.querySelectorAll<HTMLButtonElement>('[role="tab"]');
    tabs?.[items.findIndex((tab) => tab.id === nextId)]?.focus();
    onSelect(nextId);
  };

  return (
    <div
      ref={tablistRef}
      role="tablist"
      aria-label={label}
      className={TABLIST_CLASS}
      onKeyDown={onKeyDown}
    >
      {items.map((tab, index) => {
        const selected = active === tab.id;
        return (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={selected}
            tabIndex={index === stopIndex ? 0 : -1}
            {...(idPrefix !== undefined
              ? { id: tabId(idPrefix, tab.id), "aria-controls": tabPanelId(idPrefix, tab.id) }
              : {})}
            onClick={() => onSelect(tab.id)}
            className={`min-h-8 rounded-md px-4 py-1.5 text-small font-medium transition-colors duration-150 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 ${
              selected
                ? "bg-white text-ink-950 shadow-card"
                : "text-ink-500 hover:text-ink-800"
            }`}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}
