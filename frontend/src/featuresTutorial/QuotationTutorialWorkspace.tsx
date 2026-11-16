import { useCallback, useRef, useState } from "react";
import { DetailsPanel } from "../components/DetailsPanel";
import type { DetailsState } from "../components/DetailsPanel";
import { ItemEditor, newSubItem } from "../components/ItemEditor";
import type { EditorItem, EditorSubItem } from "../components/ItemEditor";
import { SectionHeading, StatusBanner } from "../components/ui";
import { SummaryRail } from "../components/SummaryRail";
import type { CurrencyInfo } from "../format";
import {
  TUTORIAL_QUOTATION_CUSTOMER_ID,
  TUTORIAL_QUOTATION_CUSTOMER_OPTIONS,
  blankPracticeItem,
  tutorialQuotationDetails,
  tutorialQuotationItems,
} from "./practiceQuotation";

export interface QuotationTutorialWorkspaceProps {
  currency: CurrencyInfo;
  canonicalUnit: string;
  uomOptions: readonly { value: string; label: string }[];
  uomPendingLabel?: string;
  incotermOptions: readonly { value: string; label: string }[];
  taxZoneOptions: readonly { value: string; label: string }[];
  taxTypeOptions: readonly { value: string; label: string }[];
}

/**
 * Issue #605: the Quotation tour's practice editor. It composes the real
 * DetailsPanel, ItemEditor and SummaryRail -- so staff practise on the same
 * controls and the same pricing path -- over local state that never leaves
 * this component.
 *
 * The sandbox mirrors App's line-item handlers rather than reaching into the
 * real editor: the real draft, the archive and the client directory are
 * untouched by construction, and unmounting is the whole teardown. The one
 * deliberate difference is that clipboard grid parsing is not imported here;
 * a paste into the practice fields falls through to the browser's own paste.
 */
export function QuotationTutorialWorkspace({
  currency,
  canonicalUnit,
  uomOptions,
  uomPendingLabel,
  incotermOptions,
  taxZoneOptions,
  taxTypeOptions,
}: QuotationTutorialWorkspaceProps) {
  const nextKey = useRef(3);
  const nextSubId = useRef(3);
  const [customerId, setCustomerId] = useState(TUTORIAL_QUOTATION_CUSTOMER_ID);
  const [details, setDetails] = useState<DetailsState>(tutorialQuotationDetails);
  const [items, setItems] = useState<EditorItem[]>(() =>
    tutorialQuotationItems(canonicalUnit)
  );

  const changeDetails = useCallback((patch: Partial<DetailsState>) => {
    setDetails((current) => ({ ...current, ...patch }));
  }, []);

  const addItem = useCallback(() => {
    setItems((prev) => [
      ...prev,
      blankPracticeItem(nextKey.current++, canonicalUnit),
    ]);
  }, [canonicalUnit]);

  const removeItem = useCallback((key: number) => {
    setItems((prev) =>
      prev.length <= 1 ? prev : prev.filter((item) => item.key !== key)
    );
  }, []);

  const changeItem = useCallback(
    (key: number, field: keyof EditorItem, value: string | string[] | boolean) => {
      setItems((prev) =>
        prev.map((item) => (item.key === key ? { ...item, [field]: value } : item))
      );
    },
    []
  );

  const addBullet = useCallback((key: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key ? { ...item, bullets: [...item.bullets, ""] } : item
      )
    );
  }, []);

  const removeBullet = useCallback((key: number, bulletIndex: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? { ...item, bullets: item.bullets.filter((_, i) => i !== bulletIndex) }
          : item
      )
    );
  }, []);

  const changeBullet = useCallback(
    (key: number, bulletIndex: number, value: string) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                bullets: item.bullets.map((line, i) =>
                  i === bulletIndex ? value : line
                ),
              }
            : item
        )
      );
    },
    []
  );

  const enableSubItems = useCallback(
    (key: number) => {
      setItems((prev) =>
        prev.map((item) => {
          if (item.key !== key) return item;
          const converted = item.bullets
            .filter((line) => line.trim() !== "")
            .map((line) => newSubItem(nextSubId.current++, line.trim(), canonicalUnit));
          return {
            ...item,
            bullets: [],
            qty: "1",
            uom: canonicalUnit,
            showSubPrices: false,
            subItems:
              converted.length > 0
                ? converted
                : [newSubItem(nextSubId.current++, "", canonicalUnit)],
          };
        })
      );
    },
    [canonicalUnit]
  );

  const disableSubItems = useCallback((key: number) => {
    setItems((prev) =>
      prev.map((item) => {
        if (item.key !== key) return item;
        const lines = item.subItems
          .map((sub) => sub.description.trim())
          .filter((line) => line !== "");
        return { ...item, bullets: lines, subItems: [], showSubPrices: false };
      })
    );
  }, []);

  const addSubRow = useCallback(
    (key: number) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: [
                  ...item.subItems,
                  newSubItem(nextSubId.current++, "", canonicalUnit),
                ],
              }
            : item
        )
      );
    },
    [canonicalUnit]
  );

  const removeSubRow = useCallback((key: number, subId: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? { ...item, subItems: item.subItems.filter((sub) => sub.id !== subId) }
          : item
      )
    );
  }, []);

  const patchSubRow = useCallback(
    (key: number, subId: number, patch: Partial<EditorSubItem>) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: item.subItems.map((sub) =>
                  sub.id === subId ? { ...sub, ...patch } : sub
                ),
              }
            : item
        )
      );
    },
    []
  );

  const moveSubRow = useCallback((key: number, subId: number, direction: -1 | 1) => {
    setItems((prev) =>
      prev.map((item) => {
        if (item.key !== key) return item;
        const at = item.subItems.findIndex((sub) => sub.id === subId);
        const target = at + direction;
        if (at === -1 || target < 0 || target >= item.subItems.length) return item;
        const next = [...item.subItems];
        [next[at], next[target]] = [next[target], next[at]];
        return { ...item, subItems: next };
      })
    );
  }, []);

  const addSubBullet = useCallback((key: number, subId: number) => {
    setItems((prev) =>
      prev.map((item) =>
        item.key === key
          ? {
              ...item,
              subItems: item.subItems.map((sub) =>
                sub.id === subId ? { ...sub, bullets: [...sub.bullets, ""] } : sub
              ),
            }
          : item
      )
    );
  }, []);

  const removeSubBullet = useCallback(
    (key: number, subId: number, bulletIndex: number) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: item.subItems.map((sub) =>
                  sub.id === subId
                    ? {
                        ...sub,
                        bullets: sub.bullets.filter((_, i) => i !== bulletIndex),
                      }
                    : sub
                ),
              }
            : item
        )
      );
    },
    []
  );

  const changeSubBullet = useCallback(
    (key: number, subId: number, bulletIndex: number, value: string) => {
      setItems((prev) =>
        prev.map((item) =>
          item.key === key
            ? {
                ...item,
                subItems: item.subItems.map((sub) =>
                  sub.id === subId
                    ? {
                        ...sub,
                        bullets: sub.bullets.map((line, i) =>
                          i === bulletIndex ? value : line
                        ),
                      }
                    : sub
                ),
              }
            : item
        )
      );
    },
    []
  );

  return (
    <div
      className="flex flex-col gap-6"
      data-tutorial-practice="quotation"
    >
      <StatusBanner tone="info">
        <span>
          <strong>Practice quotation.</strong> This is a sandbox with made-up
          lines — type, price and split anything here. Nothing is saved to your
          workspace.
        </span>
      </StatusBanner>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="flex min-w-0 flex-col gap-6">
          <DetailsPanel
            value={details}
            customerId={customerId}
            customerOptions={TUTORIAL_QUOTATION_CUSTOMER_OPTIONS}
            incotermOptions={incotermOptions}
            taxZoneOptions={taxZoneOptions}
            taxTypeOptions={taxTypeOptions}
            onChange={changeDetails}
            onCustomerChange={setCustomerId}
            onPaste={() => undefined}
          />

          <div>
            <div data-tutorial="quotation-lines-header">
              <SectionHeading
                title="Line items"
                action={
                  <span className="text-caption text-ink-500">
                    Toggle margin per line
                  </span>
                }
              />
            </div>
            <ItemEditor
              items={items}
              currency={currency}
              uomOptions={uomOptions}
              uomPendingLabel={uomPendingLabel}
              onAddItem={addItem}
              onRemoveItem={removeItem}
              onChange={changeItem}
              onAddBullet={addBullet}
              onRemoveBullet={removeBullet}
              onBulletChange={changeBullet}
              onEnableSubItems={enableSubItems}
              onDisableSubItems={disableSubItems}
              onAddSubRow={addSubRow}
              onRemoveSubRow={removeSubRow}
              onPatchSubRow={patchSubRow}
              onMoveSubRow={moveSubRow}
              onPasteSubRows={() => undefined}
              onAddSubBullet={addSubBullet}
              onRemoveSubBullet={removeSubBullet}
              onSubBulletChange={changeSubBullet}
              onPasteItems={() => undefined}
            />
          </div>
        </div>

        <div className="xl:sticky xl:top-[80px] xl:self-start">
          <SummaryRail
            items={items}
            currency={currency}
            discountPercent={details.discountPercent}
            taxZone={details.taxZone}
            taxType={details.taxType}
            serviceCategory={details.serviceCategory}
          />
        </div>
      </div>
    </div>
  );
}
