import { useEffect, useRef, useState } from "react";
import {
  ItemRow,
  type EditorItem,
  type EditorSubItem,
} from "./ItemEditor";
import type { CurrencyInfo } from "../format";
import { formatQuantityScaled } from "../purchaseOrder";
import type { SourceItem } from "../poSourceItems";

const UOMS = [
  "Lot",
  "Pc",
  "EA",
  "LS",
  "set",
  "meter",
  "m2",
  "m3",
  "Hour",
  "Day",
  "Week",
  "Month",
  "Year",
  "Quarter",
].map((value) => ({ value, label: value }));

export function editorItemFromSource(item: SourceItem): EditorItem {
  const subItems: EditorSubItem[] = item.subItems.map((sub) => ({
    id: sub.subNo,
    description: sub.description,
    qty: formatQuantityScaled(sub.quantityScaled),
    uom: sub.uom,
    unitCost: String(sub.unitPriceMinor),
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: sub.discountEnabled,
    discountPercent: String(sub.discountPercent),
    bullets: [...sub.descriptionLines],
  }));
  return {
    key: item.itemNo,
    description: item.description,
    bullets: [...item.descriptionLines],
    qty: formatQuantityScaled(item.quantityScaled),
    uom: item.uom,
    unitPrice: String(item.unitPriceMinor),
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: item.discountEnabled,
    discountPercent: String(item.discountPercent),
    subItems,
    showSubPrices: true,
  };
}

function useScrollReveal() {
  const ref = useRef<HTMLDivElement>(null);
  const [progress, setProgress] = useState(0);

  useEffect(() => {
    const reduced =
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduced) {
      setProgress(1);
      return;
    }
    let frame = 0;
    let highest = 0;
    const update = () => {
      const element = ref.current;
      if (element === null) return;
      const rect = element.getBoundingClientRect();
      const viewport = window.innerHeight;
      const start = viewport * 0.92;
      const end = viewport * 0.55;
      const value = Math.max(0, Math.min(1, (start - rect.top) / (start - end)));
      if (value > highest) {
        highest = value;
        setProgress(value);
      }
    };
    const onScroll = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(update);
    };
    update();
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
    };
  }, []);

  return { ref, progress };
}

export function QuotationGhost({
  item,
  currency,
  onAccept,
  onReject,
  existing = false,
  pointer = false,
}: {
  item: SourceItem;
  currency: CurrencyInfo;
  onAccept: () => void;
  onReject: () => void;
  existing?: boolean;
  pointer?: boolean;
}) {
  const { ref, progress } = useScrollReveal();
  const editorItem = editorItemFromSource(item);
  const split = item.subItems.length > 0;
  const noop = () => undefined;

  return (
    <div
      ref={ref}
      data-quotation-ghost
      style={{
        opacity: progress,
        transform: `translateY(${(1 - progress) * 16}px)`,
        willChange: "opacity, transform",
      }}
      className="quotation-ghost relative rounded-lg border border-dashed border-accent-300 bg-accent-50/30"
    >
      {pointer ? (
        <>
          <span
            aria-hidden="true"
            className="absolute left-10 -top-4 h-2.5 w-[3px] -translate-x-1/2 rounded-full bg-accent-300"
          />
          <span
            aria-hidden="true"
            className="absolute left-10 top-0 size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-accent-400 ring-2 ring-accent-50"
          />
        </>
      ) : null}

      <div className="flex items-start justify-between gap-3 px-3 pb-2 pt-3">
        <div className="min-w-0">
          <p className="text-small font-semibold text-accent-800">
            Quotation {split ? "item" : "line"} {item.label}
          </p>
          <p className="mt-0.5 text-caption text-ink-600">
            {existing
              ? "Your line already has values. Using this replaces them."
              : "Not on your order yet. Using this adds it."}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <button
            type="button"
            aria-label={`Use quotation ${item.label}`}
            onClick={onAccept}
            className="grid size-8 place-items-center rounded-md bg-accent-800 text-small font-semibold text-white hover:bg-accent-900"
          >
            ✓
          </button>
          <button
            type="button"
            aria-label={`Ignore quotation ${item.label}`}
            onClick={onReject}
            className="grid size-8 place-items-center rounded-md border border-rule-strong bg-white text-small text-ink-600 hover:bg-paper-subtle"
          >
            ✕
          </button>
        </div>
      </div>

      <div
        className="quotation-ghost-preview pointer-events-none select-none px-1 pb-2"
        inert
      >
        <ItemRow
          item={editorItem}
          index={0}
          canRemove={false}
          currency={currency}
          uomOptions={UOMS}
          mode="purchase-order"
          showPanel={split || item.descriptionLines.length > 0}
          allowMergeSubItems={false}
          onTogglePanel={noop}
          onRemoveItem={noop}
          onChange={noop}
          onAddBullet={noop}
          onRemoveBullet={noop}
          onBulletChange={noop}
          onEnableSubItems={noop}
          onDisableSubItems={noop}
          onAddSubRow={noop}
          onRemoveSubRow={noop}
          onPatchSubRow={noop}
          onMoveSubRow={noop}
          onPasteSubRows={noop}
          onAddSubBullet={noop}
          onRemoveSubBullet={noop}
          onSubBulletChange={noop}
          onPasteItems={noop}
        />
      </div>
    </div>
  );
}
