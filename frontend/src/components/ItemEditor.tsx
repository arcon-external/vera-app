import { useMemo, useState } from "react";
import { Button } from "./ui";
import { applyMargin, formatIdr, parseIntOrZero } from "../format";

export interface EditorItem {
  key: number;
  description: string;
  subItems: string[];
  qty: string;
  uom: string;
  unitPrice: string;
  marginEnabled: boolean;
  marginPercent: string;
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
  onAddSubItem: (key: number) => void;
  onRemoveSubItem: (key: number, subIndex: number) => void;
  onSubItemChange: (key: number, subIndex: number, value: string) => void;
  onPasteItems: (key: number, event: React.ClipboardEvent<HTMLInputElement>) => void;
}

export function ItemEditor({
  items,
  uomOptions,
  onAddItem,
  onRemoveItem,
  onChange,
  onAddSubItem,
  onRemoveSubItem,
  onSubItemChange,
  onPasteItems,
}: ItemEditorProps) {
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});

  const filledLines = useMemo(
    () =>
      items.filter(
        (item) =>
          item.description.trim() !== "" ||
          item.subItems.some((sub) => sub.trim() !== "")
      ).length,
    [items]
  );

  const marginedLines = useMemo(
    () => items.filter((item) => item.marginEnabled).length,
    [items]
  );

  return (
    <div className="overflow-hidden rounded-xl border border-rule bg-white shadow-card">
      <div className="hidden grid-cols-[minmax(0,1fr)_72px_68px_126px_184px_36px] gap-3 border-b border-rule bg-paper-subtle px-4 py-2.5 text-micro font-medium uppercase tracking-[0.1em] text-ink-500 lg:grid">
        <span>Description</span>
        <span>UoM</span>
        <span className="text-right">Qty</span>
        <span className="text-right">Cost price (IDR)</span>
        <span>Margin</span>
        <span />
      </div>

      <div className="divide-y divide-rule">
        {items.map((item, index) => (
          <ItemRow
            key={item.key}
            item={item}
            index={index}
            canRemove={items.length > 1}
            uomOptions={uomOptions}
            showSubItems={expanded[item.key] ?? item.subItems.length > 0}
            onToggleSubItems={() =>
              setExpanded((prev) => ({ ...prev, [item.key]: !(prev[item.key] ?? item.subItems.length > 0) }))
            }
            onRemoveItem={onRemoveItem}
            onChange={onChange}
            onAddSubItem={onAddSubItem}
            onRemoveSubItem={onRemoveSubItem}
            onSubItemChange={onSubItemChange}
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
        </span>
        <span className="ml-auto text-caption text-ink-500">
          Totals update in the summary panel
        </span>
      </div>
    </div>
  );
}

/**
 * Wraps a stacked-layout field with a visible label below the `lg` breakpoint.
 * At `lg` the parent becomes `display: contents`, so this wrapper's element
 * joins the ledger grid as a column and the label is hidden in favor of the
 * grid header.
 */
function MobileField({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <span className="text-micro font-medium uppercase tracking-[0.1em] text-ink-500 lg:hidden">
        {label}
      </span>
      {children}
    </div>
  );
}

function ItemRow({
  item,
  index,
  canRemove,
  uomOptions,
  showSubItems,
  onToggleSubItems,
  onRemoveItem,
  onChange,
  onAddSubItem,
  onRemoveSubItem,
  onSubItemChange,
  onPasteItems,
}: {
  item: EditorItem;
  index: number;
  canRemove: boolean;
  uomOptions: readonly { value: string; label: string }[];
  showSubItems: boolean;
  onToggleSubItems: () => void;
  onRemoveItem: (key: number) => void;
  onChange: (key: number, field: keyof EditorItem, value: string | string[] | boolean) => void;
  onAddSubItem: (key: number) => void;
  onRemoveSubItem: (key: number, subIndex: number) => void;
  onSubItemChange: (key: number, subIndex: number, value: string) => void;
  onPasteItems: (key: number, event: React.ClipboardEvent<HTMLInputElement>) => void;
}) {
  const qty = parseIntOrZero(item.qty);
  const unitCost = parseIntOrZero(item.unitPrice);
  const marginPercent = parseIntOrZero(item.marginPercent);
  const sellUnit = applyMargin(unitCost, marginPercent, item.marginEnabled);
  const sellTotal = sellUnit * qty;
  const marginAmount = sellUnit - unitCost;
  const hasSubItems = item.subItems.length > 0;

  const cellClass =
    "w-full rounded-md border border-rule bg-white px-2.5 py-2 text-body text-ink-900 outline-none transition duration-150 ease-out-quart placeholder:text-ink-400 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20";

  return (
    <div className="grid animate-item-in grid-cols-1 gap-3 px-4 py-3.5 transition-colors duration-150 hover:bg-paper-subtle/50 lg:grid-cols-[minmax(0,1fr)_72px_68px_126px_184px_36px] lg:items-start">
      <div className="min-w-0">
        <div className="relative">
          <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-caption tnum text-ink-400">
            {String(index + 1).padStart(2, "0")}
          </span>
          <input
            type="text"
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

        <div className="mt-1.5 flex items-center gap-3 text-caption text-ink-400">
          <button
            type="button"
            onClick={onToggleSubItems}
            className="inline-flex min-h-8 items-center gap-1 rounded-md px-2 py-1.5 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40"
          >
            <svg
              width="9"
              height="9"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.75"
              className={`transition-transform duration-150 ease-out-quart ${showSubItems ? "rotate-90" : ""}`}
            >
              <path d="M6 3.5l4 4.5-4 4.5" />
            </svg>
            {hasSubItems ? `${item.subItems.length} sub-item${item.subItems.length === 1 ? "" : "s"}` : "Add sub-items"}
          </button>
          {item.marginEnabled ? (
            <span className="tnum text-success-700">
              +{marginPercent}% · +Rp{formatIdr(marginAmount)}/unit
            </span>
          ) : (
            <span className="text-ink-400">No margin</span>
          )}
        </div>

        {showSubItems ? (
          <div className="mt-2 space-y-1.5 border-l border-rule pl-3">
            {item.subItems.map((sub, subIndex) => (
              <div key={subIndex} className="flex items-center gap-2">
                <span aria-hidden="true" className="select-none text-ink-300">
                  —
                </span>
                <input
                  type="text"
                  aria-label={`Sub-item ${subIndex + 1} for line ${index + 1}`}
                  placeholder={`Sub-item ${subIndex + 1}`}
                  value={sub}
                  onChange={(event) =>
                    onSubItemChange(item.key, subIndex, event.target.value)
                  }
                  className="min-w-0 flex-1 rounded-md px-1.5 py-1 text-caption text-ink-700 outline-none transition-colors duration-150 placeholder:text-ink-400 hover:bg-paper-subtle focus:bg-white focus:ring-2 focus:ring-accent-500/20"
                />
                <button
                  type="button"
                  aria-label={`Remove sub-item ${subIndex + 1} from line ${index + 1}`}
                  onClick={() => onRemoveSubItem(item.key, subIndex)}
                  className="flex h-8 w-8 items-center justify-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-danger-50 hover:text-danger-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-danger-400/40"
                >
                  <svg width="9" height="9" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M4 4l8 8M12 4l-8 8" />
                  </svg>
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={() => onAddSubItem(item.key)}
              className="inline-flex min-h-8 items-center gap-1 rounded-md px-2 py-1.5 text-caption text-ink-400 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40"
            >
              <svg width="9" height="9" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M8 3.333v9.334M3.333 8h9.334" />
              </svg>
              Add sub-item
            </button>
          </div>
        ) : null}
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:contents">
        <MobileField label="UoM">
          <select
            aria-label={`Unit of measure for line ${index + 1}`}
            value={item.uom}
            onChange={(event) => onChange(item.key, "uom", event.target.value)}
            className={`${cellClass} cursor-pointer appearance-none bg-[length:10px] bg-[right_0.6rem_center] bg-no-repeat pr-7`}
            style={{
              backgroundImage:
                "url(\"data:image/svg+xml;charset=UTF-8,%3Csvg width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23767469' stroke-width='2' stroke-linecap='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")",
            }}
          >
            {uomOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </MobileField>

        <MobileField label="Qty">
          <input
            type="text"
            inputMode="numeric"
            aria-label={`Quantity for line ${index + 1}`}
            placeholder="1"
            value={item.qty}
            onChange={(event) =>
              onChange(item.key, "qty", event.target.value.replace(/\D/g, ""))
            }
            className={`${cellClass} text-right tnum`}
          />
        </MobileField>

        <MobileField label="Cost price (IDR)">
          <input
            type="text"
            inputMode="numeric"
            aria-label={`Cost price for line ${index + 1}`}
            placeholder="0"
            value={item.unitPrice}
            onChange={(event) =>
              onChange(item.key, "unitPrice", event.target.value.replace(/\D/g, ""))
            }
            className={`${cellClass} text-right tnum`}
          />
        </MobileField>

        <MobileField label="Margin">
          <div className="flex items-center gap-2.5 lg:flex-col lg:items-stretch lg:gap-1.5">
            <div className="flex items-center gap-2">
              <button
                type="button"
                role="switch"
                aria-checked={item.marginEnabled}
                aria-label={`Enable profit margin for line ${index + 1}`}
                onClick={() =>
                  onChange(item.key, "marginEnabled", !item.marginEnabled)
                }
                className={`relative inline-flex h-[22px] w-[40px] shrink-0 items-center rounded-full border transition-colors duration-200 ease-out-quart focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/50 ${
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
                        100
                      ).toString()
                    )
                  }
                  className="w-9 bg-transparent text-right text-body tnum text-ink-900 outline-none disabled:text-ink-350"
                />
                <span className="ml-0.5 text-body text-ink-400">%</span>
              </div>
            </div>
            <div className="min-w-0 lg:px-0.5 lg:text-right">
              <div className="tnum text-caption text-ink-400">
                {formatIdr(sellUnit)} <span className="text-ink-400">/ unit</span>
              </div>
              <div className="tnum text-small font-semibold text-ink-950">
                {formatIdr(sellTotal)}
              </div>
            </div>
          </div>
        </MobileField>
      </div>

      <button
        type="button"
        aria-label={`Remove line ${index + 1}`}
        onClick={() => onRemoveItem(item.key)}
        disabled={!canRemove}
        className="mx-auto flex h-9 w-9 items-center justify-center self-center rounded-md text-ink-300 transition-colors duration-150 hover:bg-danger-50 hover:text-danger-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-danger-400/40 disabled:opacity-0 disabled:hover:bg-transparent"
      >
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M2 4h12M5.333 4V2.667a1.333 1.333 0 011.334-1.334h2.666a1.333 1.333 0 011.334 1.334V4m2 0v9.333a1.333 1.333 0 01-1.334 1.334H4.667a1.333 1.333 0 01-1.334-1.334V4h9.334z" />
        </svg>
      </button>
    </div>
  );
}


