import { backend } from "@backend/models";
import type { EditorItem } from "./components/ItemEditor";

/**
 * Issue #648: quotation lines and sub-items carry a stable number that is the
 * identity a purchase-order allocation names when it buys. The number is not
 * a position: removing line 1 of 3 must leave lines 2 and 3 still named 2 and
 * 3, and a line added afterwards must take a number nothing has used before,
 * never the removed line's. Renumbering by position is what let a bought
 * quantity follow the wrong product.
 *
 * A number of 0 means "not numbered yet" -- the shape a component default has
 * before the editor hands it an identity, and what a draft written before this
 * change carries. The editor numbers those by position on load, the only
 * identity they ever had.
 */

/** The number of a line or sub-item that has never been given an identity. */
export const UNNUMBERED = 0;

/** The description text a plain line stores: main text plus "- " bullets. */
export function buildDescription(description: string, bullets: string[]): string {
  const parts = [description];
  for (const line of bullets) {
    parts.push("- " + line);
  }
  return parts.join("\n");
}

/** The highest line number in use; 0 when none carries one. */
export function highestItemNo(items: readonly { no: number }[]): number {
  return items.reduce((highest, item) => Math.max(highest, item.no), 0);
}

/** The number a line added now may take: one past every loaded line. */
export function nextItemNo(items: readonly { no: number }[]): number {
  return highestItemNo(items) + 1;
}

/**
 * Hands out the next line number and advances the counter. The counter is a
 * high-water mark: removing a line never lowers it, so the removed number can
 * never be handed to a different product later in the same editing session.
 */
export function takeItemNo(counter: { next: number }): number {
  const no = Math.max(1, Math.trunc(counter.next));
  counter.next = no + 1;
  return no;
}

/** The highest sub-item number on one line; 0 when it has none. */
export function highestSubNo(subItems: readonly { no: number }[]): number {
  return subItems.reduce((highest, sub) => Math.max(highest, sub.no), 0);
}

/**
 * The next sub-item number for one line: the line's stored high-water mark,
 * or one past its highest numbered sub-item. Sub numbers are per line -- the
 * store keys `sub_no` on the revision item -- so two lines may both own a 1.
 */
export function nextSubNo(item: EditorItem): number {
  return Math.max(1, item.nextSubNo ?? 0, highestSubNo(item.subItems) + 1);
}

/**
 * Hands out `count` sub-item numbers for one line from its high-water mark,
 * and returns the advanced mark to store back on the line. The mark survives
 * a removal, so a bought sub-item's number is never reused by a new one.
 */
export function takeSubNos(
  item: EditorItem,
  count: number
): { nos: number[]; nextSubNo: number } {
  let no = nextSubNo(item);
  const nos: number[] = [];
  for (let i = 0; i < Math.max(0, Math.trunc(count)); i += 1) {
    nos.push(no);
    no += 1;
  }
  return { nos, nextSubNo: no };
}

/**
 * Gives every unnumbered line and sub-item a number at or above what is
 * already loaded. A draft written before identity existed has no numbers at
 * all; position is the only identity it ever had, so that is what it gets.
 * Numbered rows keep their numbers untouched, gaps included.
 */
export function numberMissingItems(items: readonly EditorItem[]): {
  items: EditorItem[];
  nextItemNo: number;
} {
  let itemNo = nextItemNo(items);
  const numbered = items.map((item) => {
    let nextSub = Math.max(1, item.nextSubNo ?? 0, highestSubNo(item.subItems) + 1);
    const subItems = item.subItems.map((sub) => {
      if (sub.no > UNNUMBERED) return sub;
      const numberedSub = { ...sub, no: nextSub };
      nextSub += 1;
      return numberedSub;
    });
    const no = item.no > UNNUMBERED ? item.no : itemNo;
    if (item.no <= UNNUMBERED) itemNo += 1;
    return { ...item, no, subItems, nextSubNo: nextSub };
  });
  return { items: numbered, nextItemNo: itemNo };
}

/** The key a bought quantity is stored under: one line, one sub-item depth. */
export function boughtLeafKey(itemNo: number, subNo = 0): string {
  return `${itemNo}:${subNo}`;
}

/**
 * Maps a quotation's availability answer onto its leaf keys. Only leaves with
 * a bought quantity are kept: a missing key means "nothing bought here", and
 * a failed read means "unknown" -- neither may paint a fabricated zero.
 */
export function boughtQuantitiesByLeaf(
  items: readonly {
    quotationItemNo: number;
    quotationSubNo?: number | null;
    boughtQuantityScaled: number;
  }[]
): Map<string, number> {
  const bought = new Map<string, number>();
  for (const item of items) {
    if (item.boughtQuantityScaled <= 0) continue;
    bought.set(
      boughtLeafKey(item.quotationItemNo, item.quotationSubNo ?? 0),
      item.boughtQuantityScaled
    );
  }
  return bought;
}

/**
 * The `No` C1 gives `QuotationItemInput`/`QuotationSubItemInput`. The Wails
 * bindings regenerate after both branches merge; until then the generated
 * classes do not declare it, so the number is attached structurally.
 */
export type NumberedQuotationSubItemInput = backend.QuotationSubItemInput & {
  No: number;
};
export type NumberedQuotationItemInput = Omit<
  backend.QuotationItemInput,
  "SubItems"
> & {
  No: number;
  SubItems: NumberedQuotationSubItemInput[];
};

function hasContent(item: EditorItem): boolean {
  return (
    item.description.trim() !== "" ||
    item.bullets.some((line) => line.trim() !== "") ||
    item.subItems.some((sub) => sub.description.trim() !== "")
  );
}

/**
 * Builds the `Items` Prepare receives, carrying each line's stable number.
 *
 * The number is attached after the generated class constructs its nested
 * sub-items: the generated constructor copies only the fields it knows, and
 * it does not know `No` until the bindings are regenerated. The whole
 * `QuotationInput` therefore takes this array after construction too -- its
 * own `Items` conversion would drop the number the same way.
 */
export function buildQuotationItemInputs(
  items: readonly EditorItem[],
  canonicalUnit: string
): NumberedQuotationItemInput[] {
  return items.filter(hasContent).map((item) => {
    const input =
      item.subItems.length === 0
        ? new backend.QuotationItemInput({
            Description: buildDescription(item.description, item.bullets),
            Qty: Number.parseInt(item.qty, 10) || 0,
            UoM: item.uom,
            UnitPrice: Number.parseInt(item.unitPrice, 10) || 0,
            MarginEnabled: item.marginEnabled,
            MarginPercent: Number.parseInt(item.marginPercent, 10) || 0,
            DiscountEnabled: item.discountEnabled,
            DiscountPercent: Number.parseInt(item.discountPercent, 10) || 0,
            SubItems: [],
            ShowSubPrices: false,
          })
        : // A line with sub-items never prices itself: Qty 1 and the served
          // canonical unit are placeholders the backend replaces with the
          // sub-item rollup, and the margin controls do not apply.
          new backend.QuotationItemInput({
            Description: item.description,
            Qty: 1,
            UoM: canonicalUnit,
            UnitPrice: 0,
            MarginEnabled: false,
            MarginPercent: 0,
            DiscountEnabled: false,
            DiscountPercent: 0,
            ShowSubPrices: item.showSubPrices,
            SubItems: item.subItems.map(
              (sub) =>
                new backend.QuotationSubItemInput({
                  Description: sub.description,
                  Qty: Number.parseInt(sub.qty, 10) || 0,
                  UoM: sub.uom,
                  UnitCost: Number.parseInt(sub.unitCost, 10) || 0,
                  MarginEnabled: sub.marginEnabled,
                  MarginPercent: Number.parseInt(sub.marginPercent, 10) || 0,
                  DiscountEnabled: sub.discountEnabled,
                  DiscountPercent: Number.parseInt(sub.discountPercent, 10) || 0,
                  SubSubItems: sub.bullets
                    .filter((line) => line.trim() !== "")
                    .map(
                      (line) =>
                        new backend.QuotationSubSubItemInput({
                          Description: line,
                        })
                    ),
                })
            ),
          });
    (input as unknown as { No: number }).No = item.no;
    input.SubItems.forEach((sub, index) => {
      (sub as unknown as { No: number }).No = item.subItems[index].no;
    });
    return input as unknown as NumberedQuotationItemInput;
  });
}
