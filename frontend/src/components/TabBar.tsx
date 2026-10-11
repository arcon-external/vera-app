export type TabItem = { id: string; label: string };

/** The id a panel should carry so the matching tab points at it. */
export function tabPanelId(idPrefix: string, id: string): string {
  return `${idPrefix}-panel-${id}`;
}

/** The id of the tab that controls that panel. */
export function tabId(idPrefix: string, id: string): string {
  return `${idPrefix}-tab-${id}`;
}

const TABLIST_CLASS = "inline-flex rounded-lg border border-rule bg-paper-muted p-0.5";

/**
 * One tab bar for both the top-level Settings tabs and the Company sub-tabs.
 * The wrapper class is fixed on purpose: every tab bar in Settings must look
 * identical, so there is deliberately no per-caller `className` override.
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
  return (
    <div role="tablist" aria-label={label} className={TABLIST_CLASS}>
      {items.map((tab) => {
        const selected = active === tab.id;
        return (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={selected}
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
