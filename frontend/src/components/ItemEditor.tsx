import { useMemo, useRef, useState } from "react";
import { loadBindings, matchShortcut } from "../shortcuts";
import { Button } from "./ui";
import {
  applyDiscount,
  applyMargin,
  blendedMargin,
  formatIdr,
  formatMoneyInput,
  MAX_DISCOUNT_PERCENT,
  MAX_MARGIN_PERCENT,
  parseIntOrZero,
} from "../format";

/** One priced sub-item of a main item. */
export interface EditorSubItem {
  id: number;
  description: string;
  qty: string;
  uom: string;
  unitCost: string;
  marginEnabled: boolean;
  marginPercent: string;
  /** The sub-item's own discount, exactly as the parent line's. */
  discountEnabled: boolean;
  discountPercent: string;
  /** Detail lines under the sub-item: plain text with no values of their own. */
  bullets: string[];
}

export interface EditorItem {
  key: number;
  description: string;
  /**
   * Description bullets for a plain line. They convert into priced sub-items
   * when the line is split, which is why both fields exist side by side.
   */
  bullets: string[];
  qty: string;
  uom: string;
  unitPrice: string;
  marginEnabled: boolean;
  marginPercent: string;
  /**
   * The row's own discount, taken off the marginalised price. Same shape as
   * the margin above and for the same reason: a line that needs 15% off and a
   * line that needs none are different lines.
   */
  discountEnabled: boolean;
  discountPercent: string;
  /** The priced composition. Empty means a plain line. */
  subItems: EditorSubItem[];
  /**
   * Customer copy choice: true prices every sub-item beneath a title parent,
   * false leaves the full price on the main item and prints the sub-items as
   * unbilled scope lines. The pricing is identical either way.
   */
  showSubPrices: boolean;
}

/** Builds one empty priced sub-item with its stable id. */
export function newSubItem(id: number, description = ""): EditorSubItem {
  return {
    id,
    description,
    qty: "1",
    uom: "Lot",
    unitCost: "",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    bullets: [],
  };
}

/** The sell price of one sub-item unit, mirroring the backend's truncation. */
export function subItemSellUnit(sub: EditorSubItem): number {
  return applyDiscount(
    applyMargin(
      parseIntOrZero(sub.unitCost),
      parseIntOrZero(sub.marginPercent),
      sub.marginEnabled
    ),
    parseIntOrZero(sub.discountPercent),
    sub.discountEnabled
  );
}

/** The line rollup: cost, sell, and the blended margin between them. */
export function subItemsRollup(subItems: EditorSubItem[]): {
  cost: number;
  sell: number;
  margin: { percent: number; approx: boolean };
} {
  let cost = 0;
  let sell = 0;
  for (const sub of subItems) {
    const qty = parseIntOrZero(sub.qty);
    cost += parseIntOrZero(sub.unitCost) * qty;
    sell += subItemSellUnit(sub) * qty;
  }
  return { cost, sell, margin: blendedMargin(cost, sell) };
}

/**
 * Renders the line's overall margin exactly as the rule wants it: a whole
 * percent as-is, anything else rounded half-up and prefixed with ≈. "Overall"
 * because it is the margin of the summed money, not any single sub-item's.
 */
export function blendedMarginLabel(margin: {
  percent: number;
  approx: boolean;
}): string {
  return `${margin.approx ? "≈" : ""}${margin.percent}% overall margin`;
}

interface ItemEditorProps {
  items: EditorItem[];
  uomOptions: readonly { value: string; label: string }[];
  onAddItem: () => void;
  onRemoveItem: (key: number) => void;
  onChange: (
    key: number,
    field: keyof EditorItem,
    value: string | string[] | boolean
  ) => void;
  onAddBullet: (key: number) => void;
  onRemoveBullet: (key: number, bulletIndex: number) => void;
  onBulletChange: (key: number, bulletIndex: number, value: string) => void;
  /** Converts the line: description bullets become sub-items, Qty 1 / Lot. */
  onEnableSubItems: (key: number) => void;
  /** Reverts the line: sub-item descriptions return as description bullets. */
  onDisableSubItems: (key: number) => void;
  onAddSubRow: (key: number) => void;
  onRemoveSubRow: (key: number, subId: number) => void;
  onPatchSubRow: (
    key: number,
    subId: number,
    patch: Partial<EditorSubItem>
  ) => void;
  onMoveSubRow: (key: number, subId: number, direction: -1 | 1) => void;
  onPasteSubRows: (
    key: number,
    event: React.ClipboardEvent<HTMLTextAreaElement>
  ) => void;
  onAddSubBullet: (key: number, subId: number) => void;
  onRemoveSubBullet: (
    key: number,
    subId: number,
    bulletIndex: number
  ) => void;
  onSubBulletChange: (
    key: number,
    subId: number,
    bulletIndex: number,
    value: string
  ) => void;
  onPasteItems: (key: number, event: React.ClipboardEvent<HTMLInputElement>) => void;
}

export function ItemEditor({
  items,
  uomOptions,
  onAddItem,
  onRemoveItem,
  onChange,
  onAddBullet,
  onRemoveBullet,
  onBulletChange,
  onEnableSubItems,
  onDisableSubItems,
  onAddSubRow,
  onRemoveSubRow,
  onPatchSubRow,
  onMoveSubRow,
  onPasteSubRows,
  onAddSubBullet,
  onRemoveSubBullet,
  onSubBulletChange,
  onPasteItems,
}: ItemEditorProps) {
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});

  const filledLines = useMemo(
    () =>
      items.filter(
        (item) =>
          item.description.trim() !== "" ||
          item.bullets.some((line) => line.trim() !== "") ||
          item.subItems.some((sub) => sub.description.trim() !== "")
      ).length,
    [items]
  );

  const marginedLines = useMemo(
    () => items.filter((item) => item.subItems.length === 0 && item.marginEnabled).length,
    [items]
  );

  const itemizedLines = useMemo(
    () => items.filter((item) => item.subItems.length > 0).length,
    [items]
  );

  // Excel-style grid navigation: arrows move between cells, Tab/Enter
  // advance (Shift reverses), edges clamp. The six main cells take part --
  // description, UoM, qty, unit price, margin, discount;
  // description-bullet and sub-item inputs keep native keys. Bindings
  // reload per keydown so Settings edits apply without a reload.
  const gridRef = useRef<HTMLDivElement>(null);
  const focusCell = (row: number, col: number): boolean => {
    const container = gridRef.current;
    if (!container) return false;
    const order = col <= 2 ? [col] : [col, col - 1, col + 1, col - 2, col + 2];
    for (const candidate of order) {
      if (candidate < 0 || candidate > 5) continue;
      const el = container.querySelector(
        `[data-grid-cell][data-grid-row="${row}"][data-grid-col="${candidate}"]:not([disabled])`
      );
      if (el instanceof HTMLElement) {
        el.focus();
        if (el instanceof HTMLInputElement) el.select();
        return true;
      }
    }
    return false;
  };
  const gridKeyDown = (event: React.KeyboardEvent) => {
    const container = gridRef.current;
    if (!container) return;
    const cell = (event.target as HTMLElement).closest?.(
      "[data-grid-cell]"
    ) as HTMLElement | null;
    if (!cell || !container.contains(cell)) return;
    const row = Number(cell.dataset.gridRow ?? 0);
    const col = Number(cell.dataset.gridCol ?? 0);
    const native = event.nativeEvent;
    let action = matchShortcut(native, loadBindings(), "grid");
    // Fixed Excel keys: Enter moves down, Shift+Enter up. Enter on a
    // dropdown keeps its native open-the-list behavior.
    if (
      !action &&
      native.key === "Enter" &&
      !native.metaKey &&
      !native.ctrlKey &&
      !native.altKey &&
      cell.tagName !== "SELECT"
    ) {
      action = native.shiftKey ? "gridUp" : "gridDown";
    }
    if (!action) {
      // Esc always lands back on the row's description cell.
      if (native.key === "Escape") focusCell(row, 0);
      return;
    }
    const lastRow = items.length - 1;
    switch (action) {
      case "gridUp":
        if (row > 0) {
          event.preventDefault();
          focusCell(row - 1, col);
        }
        return;
      case "gridDown":
        if (row < lastRow) {
          event.preventDefault();
          focusCell(row + 1, col);
        }
        return;
      case "gridLeft":
      case "gridEnterBack":
        event.preventDefault();
        if (col > 0) focusCell(row, col - 1);
        else if (row > 0) focusCell(row - 1, 5);
        return;
      case "gridRight":
      case "gridEnter":
        event.preventDefault();
        if (col < 5) focusCell(row, col + 1);
        else if (row < lastRow) focusCell(row + 1, 0);
        return;
      default:
        return;
    }
  };

  return (
    <div className="overflow-hidden rounded-xl border border-rule bg-white shadow-card">
      {/* Two-line rows: the description takes the whole first line and every
          control sits on the second with a persistent caption, so there is no
          header row to keep aligned and no breakpoint where labels appear or
          vanish. */}
      <div ref={gridRef} onKeyDown={gridKeyDown} className="divide-y divide-rule">
        {items.map((item, index) => (
          <ItemRow
            key={item.key}
            item={item}
            index={index}
            canRemove={items.length > 1}
            uomOptions={uomOptions}
            showPanel={
              expanded[item.key] ??
              (item.subItems.length > 0 || item.bullets.length > 0)
            }
            onTogglePanel={() =>
              setExpanded((prev) => ({
                ...prev,
                [item.key]: !(
                  prev[item.key] ??
                  (item.subItems.length > 0 || item.bullets.length > 0)
                ),
              }))
            }
            onRemoveItem={onRemoveItem}
            onChange={onChange}
            onAddBullet={onAddBullet}
            onRemoveBullet={onRemoveBullet}
            onBulletChange={onBulletChange}
            onEnableSubItems={onEnableSubItems}
            onDisableSubItems={onDisableSubItems}
            onAddSubRow={onAddSubRow}
            onRemoveSubRow={onRemoveSubRow}
            onPatchSubRow={onPatchSubRow}
            onMoveSubRow={onMoveSubRow}
            onPasteSubRows={onPasteSubRows}
            onAddSubBullet={onAddSubBullet}
            onRemoveSubBullet={onRemoveSubBullet}
            onSubBulletChange={onSubBulletChange}
            onPasteItems={onPasteItems}
          />
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-rule bg-paper-subtle px-4 py-2.5">
        <Button
          variant="ghost"
          size="sm"
          onClick={onAddItem}
          leading={
            <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M8 3.333v9.334M3.333 8h9.334" />
            </svg>
          }
        >
          Add line
        </Button>
        <span className="text-caption text-ink-500">
          <span className="tnum">{filledLines}</span> of{" "}
          <span className="tnum">{items.length}</span> filled
          {marginedLines > 0 ? (
            <>
              {" · "}
              <span className="tnum">{marginedLines}</span> with margin
            </>
          ) : null}
          {itemizedLines > 0 ? (
            <>
              {" · "}
              <span className="tnum">{itemizedLines}</span> with sub-items
            </>
          ) : null}
        </span>
        <span className="ml-auto text-caption text-ink-500">
          Totals update in the summary panel
        </span>
      </div>
    </div>
  );
}

/**
 * A control with a caption that is always visible, at every breakpoint. The
 * rows below are two-line grids rather than table-like columns, so there is no
 * header row for a breakpoint-conditional label to defer to -- and nothing to
 * drift out of alignment with one.
 */
function Captioned({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0">
      <span className="mb-1 block text-micro font-medium uppercase tracking-[0.1em] text-ink-500">
        {label}
      </span>
      {children}
    </div>
  );
}

/**
 * The bullet list editor shared by description bullets (plain lines) and
 * sub-item detail lines. The same controls, one meaning per context.
 */
function BulletsEditor({
  lines,
  addLabel,
  placeholder,
  ariaLabel,
  onAdd,
  onRemove,
  onChange,
}: {
  lines: string[];
  addLabel: string;
  placeholder: string;
  ariaLabel: (position: number) => string;
  onAdd: () => void;
  onRemove: (index: number) => void;
  onChange: (index: number, value: string) => void;
}) {
  return (
    <div
      className={
        lines.length > 0 ? "mt-1.5 space-y-1.5 border-l border-rule pl-3" : "mt-1"
      }
    >
      {lines.map((line, lineIndex) => (
        <div key={lineIndex} className="flex items-center gap-2">
          <span aria-hidden="true" className="select-none text-ink-300">
            —
          </span>
          <input
            type="text"
            aria-label={ariaLabel(lineIndex + 1)}
            placeholder={placeholder}
            value={line}
            onChange={(event) => onChange(lineIndex, event.target.value)}
            className="min-w-0 flex-1 rounded-md px-1.5 py-1 text-caption text-ink-700 outline-hidden transition-colors duration-150 placeholder:text-ink-400 hover:bg-paper-subtle focus:bg-white focus:ring-2 focus:ring-accent-500/20"
          />
          <button
            type="button"
            aria-label={`Remove ${ariaLabel(lineIndex + 1)}`}
            onClick={() => onRemove(lineIndex)}
            className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-danger-50 hover:text-danger-500 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-danger-400/40"
          >
            <svg width="9" height="9" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M4 4l8 8M12 4l-8 8" />
            </svg>
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={onAdd}
        className="inline-flex min-h-8 items-center gap-1 rounded-md px-2 py-1.5 text-caption text-ink-400 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
      >
        <svg width="9" height="9" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M8 3.333v9.334M3.333 8h9.334" />
        </svg>
        {addLabel}
      </button>
    </div>
  );
}

const cellClass =
  "w-full rounded-md border border-rule bg-white px-2.5 py-2 text-body text-ink-900 outline-hidden transition duration-150 ease-out-quart placeholder:text-ink-400 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20 disabled:cursor-not-allowed disabled:bg-paper-subtle disabled:text-ink-500";

/** Sub-item controls: one fixed height so a row reads as a single line. */
const subCellClass =
  "h-9 w-full rounded-md border border-rule bg-white px-2.5 text-caption text-ink-900 outline-hidden transition duration-150 ease-out-quart placeholder:text-ink-400 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20 disabled:cursor-not-allowed disabled:bg-paper-subtle disabled:text-ink-500";

const miniButtonClass =
  "inline-flex min-h-8 cursor-pointer items-center gap-1 rounded-md px-2 py-1.5 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40";

const uomChevronStyle = {
  backgroundImage:
    "url(\"data:image/svg+xml;charset=UTF-8,%3Csvg width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23767469' stroke-width='2' stroke-linecap='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")",
} as const;

/**
 * A UoM select that cannot lie: a controlled select renders an unknown value
 * as its first option, so when the stored unit is not a known one it is added
 * as an option of its own — visibly not one of the offered choices, one click
 * from being put right.
 */
function UoMSelect({
  value,
  options,
  onChange,
  ariaLabel,
  disabled,
  dense,
  dataAttrs,
}: {
  value: string;
  options: readonly { value: string; label: string }[];
  onChange: (value: string) => void;
  ariaLabel: string;
  disabled?: boolean;
  dense?: boolean;
  dataAttrs?: Record<string, string | number>;
}) {
  const known = options.some((option) => option.value === value);
  return (
    <select
      {...dataAttrs}
      aria-label={ariaLabel}
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
      className={`${dense ? subCellClass : cellClass} cursor-pointer appearance-none bg-[length:10px] bg-[right_0.6rem_center] bg-no-repeat pr-7`}
      style={uomChevronStyle}
    >
      {known ? null : <option value={value}>{value} (not recognised)</option>}
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
}

function ItemRow({
  item,
  index,
  canRemove,
  uomOptions,
  showPanel,
  onTogglePanel,
  onRemoveItem,
  onChange,
  onAddBullet,
  onRemoveBullet,
  onBulletChange,
  onEnableSubItems,
  onDisableSubItems,
  onAddSubRow,
  onRemoveSubRow,
  onPatchSubRow,
  onMoveSubRow,
  onPasteSubRows,
  onAddSubBullet,
  onRemoveSubBullet,
  onSubBulletChange,
  onPasteItems,
}: {
  item: EditorItem;
  index: number;
  canRemove: boolean;
  uomOptions: readonly { value: string; label: string }[];
  showPanel: boolean;
  onTogglePanel: () => void;
  onRemoveItem: (key: number) => void;
  onChange: (key: number, field: keyof EditorItem, value: string | string[] | boolean) => void;
  onAddBullet: (key: number) => void;
  onRemoveBullet: (key: number, bulletIndex: number) => void;
  onBulletChange: (key: number, bulletIndex: number, value: string) => void;
  onEnableSubItems: (key: number) => void;
  onDisableSubItems: (key: number) => void;
  onAddSubRow: (key: number) => void;
  onRemoveSubRow: (key: number, subId: number) => void;
  onPatchSubRow: (key: number, subId: number, patch: Partial<EditorSubItem>) => void;
  onMoveSubRow: (key: number, subId: number, direction: -1 | 1) => void;
  onPasteSubRows: (key: number, event: React.ClipboardEvent<HTMLTextAreaElement>) => void;
  onAddSubBullet: (key: number, subId: number) => void;
  onRemoveSubBullet: (key: number, subId: number, bulletIndex: number) => void;
  onSubBulletChange: (key: number, subId: number, bulletIndex: number, value: string) => void;
  onPasteItems: (key: number, event: React.ClipboardEvent<HTMLInputElement>) => void;
}) {
  const hasSubItems = item.subItems.length > 0;
  const rollup = hasSubItems ? subItemsRollup(item.subItems) : null;
  const qty = parseIntOrZero(item.qty);
  const unitCost = parseIntOrZero(item.unitPrice);
  const marginPercent = parseIntOrZero(item.marginPercent);
  // Margin first, then the row's own discount, matching backend/builder.go.
  const listUnit = applyMargin(unitCost, marginPercent, item.marginEnabled);
  const discountPercent = parseIntOrZero(item.discountPercent);
  const sellUnit = applyDiscount(listUnit, discountPercent, item.discountEnabled);
  const sellTotal = sellUnit * qty;
  // Signed: a discount below cost is a loss, and hiding that is worse than
  // showing an ugly number.
  const marginAmount = sellUnit - unitCost;

  // The six control cells live in one fragment: the description takes the
  // whole first line and the cells take the full-width grid below it.
  // Description bullets sit beside the controls when the panel is open:
  // bullets take a narrow left column and the six cells keep their single
  // inline row on the right, with narrower columns. Otherwise the cells
  // take the full-width grid.
  const showSplit = !hasSubItems && showPanel;
  const controlCells = hasSubItems && rollup ? (
    <>
      <Captioned label="UoM">
        <div className="flex h-10 items-center text-body text-ink-500">Lot</div>
      </Captioned>
      <Captioned label="Qty">
        <div className="flex h-10 items-center text-body tnum text-ink-500">
          1
        </div>
      </Captioned>
      <Captioned label="Unit cost">
        <div className="flex h-10 items-center text-body text-ink-300">
          —
        </div>
      </Captioned>
      <Captioned label="Margin">
        <div
          className="flex h-10 items-center"
          title="Overall margin across the sub-items: total profit over total cost. Updates as the sub-items change."
        >
          {rollup.cost > 0 ? (
            <span className="tnum whitespace-nowrap rounded-md border border-rule bg-white px-2 py-1 text-caption font-medium text-ink-700">
              {blendedMarginLabel(rollup.margin)}
            </span>
          ) : (
            <span className="text-caption text-ink-400">—</span>
          )}
        </div>
      </Captioned>
      <Captioned label="Discount">
        <div className="flex h-10 items-center text-body text-ink-300">
          —
        </div>
      </Captioned>
      <Captioned label="Sells for">
        <div
          className="tnum text-body font-semibold text-ink-950"
          title="The sum of the sub-items' sell prices, live as they change."
        >
          Rp{formatIdr(rollup.sell)}
        </div>
      </Captioned>
    </>
  ) : (
    <>
      <Captioned label="UoM">
        <UoMSelect
          value={item.uom}
          options={uomOptions}
          ariaLabel={`Unit of measure for line ${index + 1}`}
          onChange={(value) => onChange(item.key, "uom", value)}
          dataAttrs={{ "data-grid-cell": "", "data-grid-row": index, "data-grid-col": 1 }}
        />
      </Captioned>

      <Captioned label="Qty">
        <input
          type="text"
          data-grid-cell=""
          data-grid-row={index}
          data-grid-col={2}
          inputMode="numeric"
          aria-label={`Quantity for line ${index + 1}`}
          placeholder="1"
          value={item.qty}
          onChange={(event) =>
            onChange(item.key, "qty", event.target.value.replace(/\D/g, ""))
          }
          className={`${cellClass} text-right tnum`}
        />
      </Captioned>

      <Captioned label="Unit cost">
        <input
          type="text"
          data-grid-cell=""
          data-grid-row={index}
          data-grid-col={3}
          inputMode="numeric"
          aria-label={`Unit price for line ${index + 1}`}
          placeholder="0"
          value={formatMoneyInput(item.unitPrice)}
          onChange={(event) =>
            onChange(item.key, "unitPrice", event.target.value.replace(/\D/g, ""))
          }
          className={`${cellClass} text-right tnum`}
        />
      </Captioned>

      <Captioned label="Margin">
        <div className="flex items-center gap-2">
          <button
            type="button"
            role="switch"
            aria-checked={item.marginEnabled}
            aria-label={`Enable profit margin for line ${index + 1}`}
            onClick={() =>
              onChange(item.key, "marginEnabled", !item.marginEnabled)
            }
            className={`relative inline-flex h-[22px] w-[40px] shrink-0 cursor-pointer items-center rounded-full border transition-colors duration-200 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50 ${
              item.marginEnabled
                ? "border-accent-700 bg-accent-600"
                : "border-rule-strong bg-paper-sunk"
            }`}
          >
            <span
              aria-hidden="true"
              className={`h-[18px] w-[18px] rounded-full bg-white shadow-[0_1px_2px_rgba(60,52,34,0.25)] transition-transform duration-200 ease-out-quart ${
                item.marginEnabled ? "translate-x-[18px]" : "translate-x-[2px]"
              }`}
            />
          </button>
          <div
            className={`flex min-w-0 flex-1 items-center rounded-md border px-2.5 py-2 transition-colors duration-150 ${
              item.marginEnabled
                ? "border-rule bg-white focus-within:border-accent-400 focus-within:ring-2 focus-within:ring-accent-500/20"
                : "border-rule bg-paper-subtle"
            }`}
          >
            <input
              type="text"
              data-grid-cell=""
              data-grid-row={index}
              data-grid-col={4}
              inputMode="numeric"
              aria-label={`Profit margin percentage for line ${index + 1}`}
              value={item.marginPercent}
              disabled={!item.marginEnabled}
              onChange={(event) =>
                onChange(
                  item.key,
                  "marginPercent",
                  Math.min(
                    parseIntOrZero(event.target.value.replace(/\D/g, "")),
                    MAX_MARGIN_PERCENT
                  ).toString()
                )
              }
              className="min-w-0 flex-1 bg-transparent text-right text-body tnum text-ink-900 outline-hidden disabled:text-ink-350"
            />
            <span className="ml-0.5 shrink-0 text-body text-ink-400">%</span>
          </div>
        </div>
    </Captioned>

    <Captioned label="Discount">
      <div className="flex items-center gap-2">
        <button
          type="button"
          role="switch"
          aria-checked={item.discountEnabled}
          aria-label={`Enable a discount for line ${index + 1}`}
          onClick={() =>
            onChange(item.key, "discountEnabled", !item.discountEnabled)
          }
          className={`relative inline-flex h-[22px] w-[40px] shrink-0 cursor-pointer items-center rounded-full border transition-colors duration-200 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50 ${
            item.discountEnabled
              ? "border-accent-700 bg-accent-600"
              : "border-rule-strong bg-paper-sunk"
          }`}
        >
          <span
            aria-hidden="true"
            className={`h-[18px] w-[18px] rounded-full bg-white shadow-[0_1px_2px_rgba(60,52,34,0.25)] transition-transform duration-200 ease-out-quart ${
              item.discountEnabled ? "translate-x-[18px]" : "translate-x-[2px]"
            }`}
          />
        </button>
        <div
          className={`flex min-w-0 flex-1 items-center rounded-md border px-2.5 py-2 transition-colors duration-150 ${
            item.discountEnabled
              ? "border-rule bg-white focus-within:border-accent-400 focus-within:ring-2 focus-within:ring-accent-500/20"
              : "border-rule bg-paper-subtle"
          }`}
        >
          <input
            type="text"
            data-grid-cell=""
            data-grid-row={index}
            data-grid-col={5}
            inputMode="numeric"
            aria-label={`Discount percentage for line ${index + 1}`}
            value={item.discountPercent}
            disabled={!item.discountEnabled}
            onChange={(event) =>
              onChange(
                item.key,
                "discountPercent",
                Math.min(
                  parseIntOrZero(event.target.value.replace(/\D/g, "")),
                  MAX_DISCOUNT_PERCENT
                ).toString()
              )
            }
            className="min-w-0 flex-1 bg-transparent text-right text-body tnum text-ink-900 outline-hidden disabled:text-ink-350"
          />
          <span className="ml-0.5 shrink-0 text-body text-ink-400">%</span>
        </div>
      </div>

    </Captioned>

    <Captioned label="Sells for">
      <div className="tnum text-body font-semibold text-ink-950">
        Rp{formatIdr(sellTotal)}
      </div>
      <div className="tnum text-caption text-ink-400">
        Rp{formatIdr(sellUnit)} / unit
      </div>
      {item.discountEnabled ? (
        <div className="tnum text-caption text-accent-700">
          was Rp{formatIdr(listUnit)} · −Rp{formatIdr(listUnit * qty - sellTotal)}
        </div>
      ) : null}
    </Captioned>
    </>
  );

  return (
    <div className="animate-item-in px-4 py-3.5 transition-colors duration-150 hover:bg-paper-subtle/50">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
        <div className="relative">
          <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-caption tnum text-ink-400">
            {String(index + 1).padStart(2, "0")}
          </span>
          <input
            type="text"
            data-grid-cell=""
            data-grid-row={index}
            data-grid-col={0}
            aria-label={`Description for line ${index + 1}`}
            placeholder="Service or product description — paste from Excel for multiple lines"
            value={item.description}
            onChange={(event) =>
              onChange(item.key, "description", event.target.value)
            }
            onPaste={(event) => onPasteItems(item.key, event)}
            className={`${cellClass} pl-9`}
          />
        </div>

        <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-caption text-ink-400">
          <button
            type="button"
            onClick={onTogglePanel}
            className={`${miniButtonClass} gap-1`}
          >
            <svg
              width="9"
              height="9"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.75"
              className={`transition-transform duration-150 ease-out-quart ${showPanel ? "rotate-90" : ""}`}
            >
              <path d="M6 3.5l4 4.5-4 4.5" />
            </svg>
            {hasSubItems
              ? `${item.subItems.length} sub-item${item.subItems.length === 1 ? "" : "s"}`
              : item.bullets.length > 0
                ? `${item.bullets.length} description line${item.bullets.length === 1 ? "" : "s"}`
                : "Description lines"}
          </button>
          {hasSubItems ? null : (
            <button
              type="button"
              onClick={() => onEnableSubItems(item.key)}
              title="Each sub-item is priced on its own; the line becomes their sum and always quantity 1."
              className="inline-flex min-h-8 cursor-pointer items-center gap-1.5 rounded-md border border-accent-300 bg-accent-50 px-2.5 py-1.5 text-caption font-medium text-accent-800 transition-colors duration-150 hover:border-accent-500 hover:bg-accent-100 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
            >
              <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                <rect x="2.5" y="3" width="11" height="10" rx="1.5" />
                <path d="M8 3v10" />
              </svg>
              Split into sub-items
            </button>
          )}
          {hasSubItems ? null : item.discountEnabled ? (
            <span className="tnum text-accent-700">
              −{discountPercent}% · {marginAmount >= 0 ? "+" : "−"}
              Rp{formatIdr(Math.abs(marginAmount))}/unit
            </span>
          ) : item.marginEnabled ? (
            <span className="tnum text-success-700">
              +{marginPercent}% · +Rp{formatIdr(marginAmount)}/unit
            </span>
          ) : (
            <span className="text-ink-400">No margin</span>
          )}
        </div>

        {!hasSubItems && showPanel ? (
          <BulletsEditor
            lines={item.bullets}
            addLabel="Add description line"
            placeholder="Description line"
            ariaLabel={(position) =>
              `Description line ${position} for line ${index + 1}`
            }
            onAdd={() => onAddBullet(item.key)}
            onRemove={(bulletIndex) => onRemoveBullet(item.key, bulletIndex)}
            onChange={(bulletIndex, value) =>
              onBulletChange(item.key, bulletIndex, value)
            }
          />
        ) : null}
      </div>

        <button
          type="button"
          aria-label={`Remove line ${index + 1}`}
          onClick={() => onRemoveItem(item.key)}
          disabled={!canRemove}
          className="flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-danger-50 hover:text-danger-500 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-danger-400/40 disabled:opacity-0 disabled:hover:bg-transparent"
        >
          <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M2 4h12M5.333 4V2.667a1.333 1.333 0 011.334-1.334h2.666a1.333 1.333 0 011.334 1.334V4m2 0v9.333a1.333 1.333 0 01-1.334 1.334H4.667a1.333 1.333 0 01-1.334-1.334V4h9.334z" />
          </svg>
        </button>
      </div>

      {showSplit ? (
        <div className="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-[250px_minmax(0,1fr)]">
          <div className="min-w-0 lg:pt-[18px]">
            <BulletsEditor
              lines={item.bullets}
              addLabel="Add description line"
              placeholder="Description line"
              ariaLabel={(position) =>
                `Description line ${position} for line ${index + 1}`
              }
              onAdd={() => onAddBullet(item.key)}
              onRemove={(bulletIndex) => onRemoveBullet(item.key, bulletIndex)}
              onChange={(bulletIndex, value) =>
                onBulletChange(item.key, bulletIndex, value)
              }
            />
          </div>
          <div className="grid grid-cols-2 items-start gap-2 lg:grid-cols-[64px_52px_116px_140px_140px_minmax(0,1fr)]">
            {controlCells}
          </div>
        </div>
      ) : (
        <div className="mt-3 grid grid-cols-2 items-start gap-3 lg:grid-cols-[72px_68px_140px_184px_184px_minmax(0,1fr)]">
          {controlCells}
        </div>
      )}

      {hasSubItems && showPanel ? (
        <div className="mt-3 rounded-lg bg-paper-subtle/70 p-3 ring-1 ring-rule/60">
          <div className="-mx-3 -mt-3 mb-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-t-lg border-b border-rule/70 bg-white/70 px-3 py-3">
            <span className="text-micro font-semibold uppercase tracking-[0.1em] text-ink-600">
              Sub-items — priced individually; the line totals their sum
            </span>
            <div
              className="flex items-center gap-2"
              title={
                item.showSubPrices
                  ? "Customer copy: the main item becomes a title and each sub-item prints its own price."
                  : "Customer copy: the full price stays on the main item; the sub-items print as unbilled scope lines."
              }
            >
              <button
                type="button"
                role="switch"
                aria-checked={item.showSubPrices}
                aria-label={`Show sub-item prices to the customer on line ${index + 1}`}
                onClick={() =>
                  onChange(item.key, "showSubPrices", !item.showSubPrices)
                }
                className={`relative inline-flex h-[22px] w-[40px] shrink-0 cursor-pointer items-center rounded-full border transition-colors duration-200 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50 ${
                  item.showSubPrices
                    ? "border-accent-700 bg-accent-600"
                    : "border-rule-strong bg-paper-sunk"
                }`}
              >
                <span
                  aria-hidden="true"
                  className={`h-[18px] w-[18px] rounded-full bg-white shadow-[0_1px_2px_rgba(60,52,34,0.25)] transition-transform duration-200 ease-out-quart ${
                    item.showSubPrices ? "translate-x-[18px]" : "translate-x-[2px]"
                  }`}
                />
              </button>
              <span className="text-caption text-ink-700">
                Show sub-item prices to the customer
              </span>
            </div>
          </div>

          <div className="space-y-2">
            {item.subItems.map((sub, subIndex) => (
              <SubRow
                key={sub.id}
                sub={sub}
                subIndex={subIndex}
                index={index}
                canMoveUp={subIndex > 0}
                canMoveDown={subIndex < item.subItems.length - 1}
                uomOptions={uomOptions}
                onPatch={(patch) => onPatchSubRow(item.key, sub.id, patch)}
                onRemove={() => onRemoveSubRow(item.key, sub.id)}
                onMove={(direction) => onMoveSubRow(item.key, sub.id, direction)}
                onAddBullet={() => onAddSubBullet(item.key, sub.id)}
                onRemoveBullet={(bulletIndex) =>
                  onRemoveSubBullet(item.key, sub.id, bulletIndex)
                }
                onBulletChange={(bulletIndex, value) =>
                  onSubBulletChange(item.key, sub.id, bulletIndex, value)
                }
              />
            ))}
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1">
            <button
              type="button"
              onClick={() => onAddSubRow(item.key)}
              className={`${miniButtonClass} text-caption text-ink-500`}
            >
              <svg width="9" height="9" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M8 3.333v9.334M3.333 8h9.334" />
              </svg>
              Add sub-item
            </button>
            <button
              type="button"
              onClick={() => onDisableSubItems(item.key)}
              title="The sub-item descriptions return as description lines; quantities, costs and margins are discarded."
              className={`${miniButtonClass} text-caption text-ink-400 hover:text-danger-500`}
            >
              Convert back to a plain line
            </button>
          </div>

          <textarea
            value=""
            onChange={() => undefined}
            rows={1}
            aria-label={`Paste sub-item rows for line ${index + 1}`}
            placeholder="Or paste rows from Excel — description, qty, UoM, unit cost, margin %"
            onPaste={(event) => onPasteSubRows(item.key, event)}
            className="mt-2 w-full resize-none rounded-md border border-dashed border-rule bg-white/70 px-2.5 py-2 text-caption text-ink-500 outline-hidden transition-colors duration-150 placeholder:text-ink-400 hover:border-ink-300 focus:border-accent-400"
          />
        </div>
      ) : null}
    </div>
  );
}

function SubRow({
  sub,
  subIndex,
  index,
  canMoveUp,
  canMoveDown,
  uomOptions,
  onPatch,
  onRemove,
  onMove,
  onAddBullet,
  onRemoveBullet,
  onBulletChange,
}: {
  sub: EditorSubItem;
  subIndex: number;
  index: number;
  canMoveUp: boolean;
  canMoveDown: boolean;
  uomOptions: readonly { value: string; label: string }[];
  onPatch: (patch: Partial<EditorSubItem>) => void;
  onRemove: () => void;
  onMove: (direction: -1 | 1) => void;
  onAddBullet: () => void;
  onRemoveBullet: (bulletIndex: number) => void;
  onBulletChange: (bulletIndex: number, value: string) => void;
}) {
  const sellUnit = subItemSellUnit(sub);
  const sellTotal = sellUnit * parseIntOrZero(sub.qty);
  const marginPercent = parseIntOrZero(sub.marginPercent);
  const listUnit = applyMargin(
    parseIntOrZero(sub.unitCost),
    marginPercent,
    sub.marginEnabled
  );
  const label = `sub-item ${subIndex + 1} of line ${index + 1}`;

  return (
    <div className="rounded-md border border-rule/70 bg-white px-2.5 py-2">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <input
            type="text"
            aria-label={`Description for ${label}`}
            placeholder="Sub-item description"
            value={sub.description}
            onChange={(event) => onPatch({ description: event.target.value })}
            className={subCellClass}
          />
        </div>
        <div className="flex shrink-0 items-start gap-0.5">
          <button
            type="button"
            aria-label={`Move ${label} up`}
            disabled={!canMoveUp}
            onClick={() => onMove(-1)}
            className="flex h-9 w-7 cursor-pointer items-center justify-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent"
          >
            <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.75">
              <path d="M4 10l4-4 4 4" />
            </svg>
          </button>
          <button
            type="button"
            aria-label={`Move ${label} down`}
            disabled={!canMoveDown}
            onClick={() => onMove(1)}
            className="flex h-9 w-7 cursor-pointer items-center justify-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40 disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent"
          >
            <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.75">
              <path d="M4 6l4 4 4-4" />
            </svg>
          </button>
          <button
            type="button"
            aria-label={`Remove ${label}`}
            onClick={onRemove}
            className="flex h-9 w-7 cursor-pointer items-center justify-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-danger-50 hover:text-danger-500 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-danger-400/40"
          >
            <svg width="9" height="9" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M4 4l8 8M12 4l-8 8" />
            </svg>
          </button>
        </div>
      </div>

      <div className="mt-2 grid grid-cols-1 gap-3 lg:grid-cols-[250px_minmax(0,1fr)]">
        <div className="min-w-0 lg:pt-[18px]">
            <BulletsEditor
              lines={sub.bullets}
              addLabel="Add sub-sub-item"
              placeholder="Sub-sub-item — what this includes"
              ariaLabel={(position) => `Sub-sub-item ${position} for ${label}`}
              onAdd={onAddBullet}
              onRemove={onRemoveBullet}
              onChange={onBulletChange}
            />
        </div>
        <div className="grid grid-cols-2 items-start gap-2 lg:grid-cols-[44px_68px_104px_124px_124px_minmax(0,1fr)]">
        <Captioned label="Qty">
          <input
            type="text"
            inputMode="numeric"
            aria-label={`Quantity for ${label}`}
            placeholder="1"
            value={sub.qty}
            onChange={(event) =>
              onPatch({ qty: event.target.value.replace(/\D/g, "") })
            }
            className={`${subCellClass} text-right tnum`}
          />
        </Captioned>

        <Captioned label="UoM">
          <UoMSelect
            dense
            value={sub.uom}
            options={uomOptions}
            ariaLabel={`Unit of measure for ${label}`}
            onChange={(value) => onPatch({ uom: value })}
          />
        </Captioned>

        <Captioned label="Unit cost">
          <input
            type="text"
            inputMode="numeric"
            aria-label={`Unit cost for ${label}`}
            placeholder="0"
            value={formatMoneyInput(sub.unitCost)}
            onChange={(event) =>
              onPatch({ unitCost: event.target.value.replace(/\D/g, "") })
            }
            className={`${subCellClass} text-right tnum`}
          />
        </Captioned>

        <Captioned label="Margin">
          <div className="flex h-9 items-center gap-2">
            <button
              type="button"
              role="switch"
              aria-checked={sub.marginEnabled}
              aria-label={`Enable profit margin for ${label}`}
              onClick={() => onPatch({ marginEnabled: !sub.marginEnabled })}
              className={`relative inline-flex h-[22px] w-[40px] shrink-0 cursor-pointer items-center rounded-full border transition-colors duration-200 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50 ${
                sub.marginEnabled
                  ? "border-accent-700 bg-accent-600"
                  : "border-rule-strong bg-paper-sunk"
              }`}
            >
              <span
                aria-hidden="true"
                className={`h-[18px] w-[18px] rounded-full bg-white shadow-[0_1px_2px_rgba(60,52,34,0.25)] transition-transform duration-200 ease-out-quart ${
                  sub.marginEnabled ? "translate-x-[18px]" : "translate-x-[2px]"
                }`}
              />
            </button>
            <div
              className={`flex h-9 min-w-0 flex-1 items-center rounded-md border px-2.5 transition-colors duration-150 ${
                sub.marginEnabled
                  ? "border-rule bg-white focus-within:border-accent-400 focus-within:ring-2 focus-within:ring-accent-500/20"
                  : "border-rule bg-paper-subtle"
              }`}
            >
              <input
                type="text"
                inputMode="numeric"
                aria-label={`Profit margin percentage for ${label}`}
                value={sub.marginPercent}
                disabled={!sub.marginEnabled}
                onChange={(event) =>
                  onPatch({
                    marginPercent: Math.min(
                      parseIntOrZero(event.target.value.replace(/\D/g, "")),
                      MAX_MARGIN_PERCENT
                    ).toString(),
                  })
                }
                className="min-w-0 flex-1 bg-transparent text-right text-caption tnum text-ink-900 outline-hidden disabled:text-ink-350"
              />
              <span className="ml-0.5 shrink-0 text-caption text-ink-400">%</span>
            </div>
          </div>
        </Captioned>

        <Captioned label="Discount">
          <div className="flex h-9 items-center gap-2">
            <button
              type="button"
              role="switch"
              aria-checked={sub.discountEnabled}
              aria-label={`Enable a discount for ${label}`}
              onClick={() => onPatch({ discountEnabled: !sub.discountEnabled })}
              className={`relative inline-flex h-[22px] w-[40px] shrink-0 cursor-pointer items-center rounded-full border transition-colors duration-200 ease-out-quart focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/50 ${
                sub.discountEnabled
                  ? "border-accent-700 bg-accent-600"
                  : "border-rule-strong bg-paper-sunk"
              }`}
            >
              <span
                aria-hidden="true"
                className={`h-[18px] w-[18px] rounded-full bg-white shadow-[0_1px_2px_rgba(60,52,34,0.25)] transition-transform duration-200 ease-out-quart ${
                  sub.discountEnabled ? "translate-x-[18px]" : "translate-x-[2px]"
                }`}
              />
            </button>
            <div
              className={`flex h-9 min-w-0 flex-1 items-center rounded-md border px-2.5 transition-colors duration-150 ${
                sub.discountEnabled
                  ? "border-rule bg-white focus-within:border-accent-400 focus-within:ring-2 focus-within:ring-accent-500/20"
                  : "border-rule bg-paper-subtle"
              }`}
            >
              <input
                type="text"
                inputMode="numeric"
                aria-label={`Discount percentage for ${label}`}
                value={sub.discountPercent}
                disabled={!sub.discountEnabled}
                onChange={(event) =>
                  onPatch({
                    discountPercent: Math.min(
                      parseIntOrZero(event.target.value.replace(/\D/g, "")),
                      MAX_DISCOUNT_PERCENT
                    ).toString(),
                  })
                }
                className="min-w-0 flex-1 bg-transparent text-right text-caption tnum text-ink-900 outline-hidden disabled:text-ink-350"
              />
              <span className="ml-0.5 shrink-0 text-caption text-ink-400">%</span>
            </div>
          </div>
        </Captioned>

        <Captioned label="Sells for">
          <div className="tnum pt-0.5 text-caption font-semibold text-ink-950">
            Rp{formatIdr(sellTotal)}
          </div>
          <div className="tnum text-caption text-ink-400">
            Rp{formatIdr(sellUnit)} / unit
          </div>
        </Captioned>
        </div>
      </div>
    </div>
  );
}
