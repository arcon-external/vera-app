import { formatMoney, type CurrencyInfo } from "../format";
import { MAX_AGGREGATE_TITLE_LENGTH, sanitizeQuantityInput } from "../poAggregate";
import type { PurchaseOrderAggregateGroupState } from "../purchaseOrder";
import { Button, fieldClass } from "./ui";

/**
 * #654: one aggregate line's header, as the editor renders it above its child
 * display groups.
 *
 * It is presentation only. The total and unit price shown here are derived
 * from the child rows on every render; the header stores neither. There is no
 * cost, margin, variance, supplier or PPN input because an aggregate line has
 * no economics of its own (Story 42) and no quantity that allocates anything:
 * the display quantity and unit are the customer-facing "1 Lot" label.
 */
export interface PurchaseOrderAggregateHeaderProps {
  group: PurchaseOrderAggregateGroupState;
  /** 0-based position in the editor's headers; used only for unique labels. */
  index: number;
  /** The number of child display groups (a split package counts once). */
  childCount: number;
  /** The derived sum of the child nets; null when a child cannot be totalled. */
  totalMinor: number | null;
  /** round_half_away(total÷display qty); null when out of the safe range. */
  displayUnitPriceMinor: number | null;
  currency: CurrencyInfo;
  uomOptions: readonly { value: string; label: string }[];
  onChange: (
    patch: Partial<
      Pick<
        PurchaseOrderAggregateGroupState,
        "title" | "orderedQuantity" | "orderedUoM" | "showChildrenToCustomer"
      >
    >
  ) => void;
  onUngroup: () => void;
}

function codePointLength(value: string): number {
  return Array.from(value).length;
}

export function PurchaseOrderAggregateHeader({
  group,
  index,
  childCount,
  totalMinor,
  displayUnitPriceMinor,
  currency,
  uomOptions,
  onChange,
  onUngroup,
}: PurchaseOrderAggregateHeaderProps) {
  const label = `aggregate line ${index + 1}`;
  const title = group.title.trim() === "" ? label : group.title.trim();
  const titleLength = codePointLength(group.title);
  const knownUnit = uomOptions.some((option) => option.value === group.orderedUoM);
  return (
    <div
      data-aggregate-header-card={group.key}
      className="bg-paper-subtle/60 px-3 py-3"
    >
      <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline justify-between gap-3">
            <label
              htmlFor={`po-aggregate-title-${group.key}`}
              className="text-small font-semibold text-ink-900"
            >
              Aggregate line
            </label>
            <span className="text-caption tabular-nums text-ink-400">
              {titleLength}/{MAX_AGGREGATE_TITLE_LENGTH}
            </span>
          </div>
          <input
            id={`po-aggregate-title-${group.key}`}
            data-aggregate-title-input={group.key}
            aria-label={`Aggregate line ${index + 1} title`}
            className={`${fieldClass} mt-1`}
            value={group.title}
            maxLength={MAX_AGGREGATE_TITLE_LENGTH}
            placeholder="Combined items"
            onChange={(event) => onChange({ title: event.target.value })}
          />
        </div>
        <div className="shrink-0 text-right">
          <span className="block text-[10px] font-semibold uppercase tracking-[0.12em] text-ink-500">
            Total
          </span>
          <span
            data-aggregate-total={group.key}
            className="tnum whitespace-nowrap text-body font-semibold text-ink-950"
          >
            {totalMinor === null ? "—" : formatMoney(currency, totalMinor)}
          </span>
        </div>
      </div>

      <div className="mt-2 grid gap-3 sm:grid-cols-[minmax(0,10rem)_minmax(0,8rem)_minmax(0,1fr)]">
        <label className="grid gap-1">
          <span className="text-caption font-medium text-ink-600">
            Display quantity
          </span>
          <input
            aria-label={`Aggregate line ${index + 1} display quantity`}
            inputMode="decimal"
            className={fieldClass}
            value={group.orderedQuantity}
            onChange={(event) =>
              onChange({ orderedQuantity: sanitizeQuantityInput(event.target.value) })
            }
          />
        </label>
        <label className="grid gap-1">
          <span className="text-caption font-medium text-ink-600">Unit</span>
          <select
            aria-label={`Aggregate line ${index + 1} display unit`}
            className={fieldClass}
            value={group.orderedUoM}
            onChange={(event) => onChange({ orderedUoM: event.target.value })}
          >
            {knownUnit ? null : (
              <option value={group.orderedUoM}>{group.orderedUoM}</option>
            )}
            {uomOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <div className="grid gap-1">
          <span className="text-caption font-medium text-ink-600">
            Unit price (derived)
          </span>
          <span
            data-aggregate-unit-price={group.key}
            className="tnum flex min-h-10 items-center whitespace-nowrap text-body text-ink-800"
          >
            {displayUnitPriceMinor === null
              ? "—"
              : formatMoney(currency, displayUnitPriceMinor)}
          </span>
        </div>
      </div>

      <p className="mt-2 text-caption text-ink-500">
        Display only — it does not change what was ordered.
      </p>

      <div className="mt-2 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-rule/70 pt-2">
        <label className="flex items-start gap-2 text-caption text-ink-700">
          <input
            type="checkbox"
            aria-label={`Show children to the customer for ${label}`}
            className="mt-0.5 size-4 shrink-0 cursor-pointer rounded border-rule-strong accent-accent-700"
            checked={group.showChildrenToCustomer}
            onChange={(event) =>
              onChange({ showChildrenToCustomer: event.target.checked })
            }
          />
          <span>
            Show children to the customer
            <span className="block text-ink-500">
              Off prints the title and total only.
            </span>
          </span>
        </label>
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-caption text-ink-500">
            <span className="tnum">{childCount}</span>{" "}
            {childCount === 1 ? "item" : "items"} in this aggregate
          </span>
          <Button
            size="xs"
            variant="secondary"
            aria-label={`Ungroup ${title}`}
            onClick={onUngroup}
          >
            Ungroup
          </Button>
        </div>
      </div>
    </div>
  );
}
