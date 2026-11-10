import { useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  ItemEditor,
  newSubItem,
  subItemSellUnit,
  type EditorItem,
  type EditorSubItem,
} from "./components/ItemEditor";
import { Badge, Button, Card, SectionHeading, fieldClass } from "./components/ui";
import {
  applyDiscount,
  applyMargin,
  formatMoney,
  parseIntOrZero,
  type CurrencyInfo,
} from "./format";
import "./index.css";

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const UOMS = [
  { value: "Pc", label: "Pc" },
  { value: "Set", label: "Set" },
  { value: "Lot", label: "Lot" },
  { value: "m", label: "m" },
  { value: "m2", label: "m²" },
  { value: "kg", label: "kg" },
];

const initialItems: EditorItem[] = [
  {
    key: 1,
    description: "Schneider Acti9 iC60N MCB, 2P, 16A, 6kA, C-curve",
    bullets: [],
    qty: "12",
    uom: "Pc",
    unitPrice: "454300",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: true,
    discountPercent: "5",
    subItems: [],
    showSubPrices: false,
  },
  {
    key: 2,
    description: "Phoenix Contact QUINT power supply, 24 VDC / 10 A",
    bullets: ["With redundancy module and buffer", "DIN-rail mounted"],
    qty: "8",
    uom: "Pc",
    unitPrice: "2817500",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    subItems: [],
    showSubPrices: false,
  },
  {
    key: 3,
    description: "LV distribution panel — assembly, wiring and testing",
    bullets: [],
    qty: "1",
    uom: "Lot",
    unitPrice: "",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    showSubPrices: true,
    subItems: [
      {
        ...newSubItem(1, "Main distribution panel, 24-way, IP54", "Pc"),
        qty: "1",
        unitCost: "18500000",
        discountEnabled: true,
        discountPercent: "5",
        bullets: ["Powder-coated steel enclosure", "Busbar and neutral link included"],
      },
      {
        ...newSubItem(2, "Wiring, termination and labelling", "Lot"),
        qty: "1",
        unitCost: "6750000",
        bullets: ["Two technicians", "Three days on site", "Test report supplied"],
      },
    ],
  },
  {
    key: 4,
    description: "Installation, termination and commissioning works",
    bullets: [],
    qty: "1",
    uom: "Lot",
    unitPrice: "22570000",
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: false,
    discountPercent: "0",
    subItems: [],
    showSubPrices: false,
  },
];

function listUnit(item: EditorItem): number {
  return applyMargin(
    parseIntOrZero(item.unitPrice),
    parseIntOrZero(item.marginPercent),
    item.marginEnabled
  );
}

function sellUnit(item: EditorItem): number {
  return applyDiscount(
    listUnit(item),
    parseIntOrZero(item.discountPercent),
    item.discountEnabled
  );
}

function subListUnit(sub: EditorSubItem): number {
  return applyMargin(
    parseIntOrZero(sub.unitCost),
    parseIntOrZero(sub.marginPercent),
    sub.marginEnabled
  );
}

/** A line's list and sell totals: the sum of its priced sub-items when it is
 *  split, otherwise its own quantity × unit price. */
function lineTotals(item: EditorItem): { list: number; sell: number } {
  if (item.subItems.length > 0) {
    let list = 0;
    let sell = 0;
    for (const sub of item.subItems) {
      const qty = parseIntOrZero(sub.qty);
      list += subListUnit(sub) * qty;
      sell += subItemSellUnit(sub) * qty;
    }
    return { list, sell };
  }
  const qty = parseIntOrZero(item.qty);
  return { list: listUnit(item) * qty, sell: sellUnit(item) * qty };
}

function Field({
  label,
  span = false,
  children,
}: {
  label: string;
  span?: boolean;
  children: React.ReactNode;
}) {
  return (
    <label className={`grid gap-1.5 ${span ? "sm:col-span-2" : ""}`}>
      <span className="text-caption font-medium text-ink-600">{label}</span>
      {children}
    </label>
  );
}

function SummaryRow({ label, value, muted = false }: { label: string; value: string; muted?: boolean }) {
  return (
    <div className={`flex items-baseline justify-between gap-4 text-small ${muted ? "text-ink-500" : "text-ink-800"}`}>
      <span>{label}</span>
      <span className="tnum whitespace-nowrap">{value}</span>
    </div>
  );
}

function App() {
  const [items, setItems] = useState<EditorItem[]>(initialItems);
  const [nextKey, setNextKey] = useState(5);
  const nextSubId = useRef(3);

  const addItem = () => {
    const key = nextKey;
    setNextKey((current) => current + 1);
    setItems((current) => [
      ...current,
      {
        key,
        description: "",
        bullets: [],
        qty: "1",
        uom: "Pc",
        unitPrice: "",
        marginEnabled: false,
        marginPercent: "0",
        discountEnabled: false,
        discountPercent: "0",
        subItems: [],
        showSubPrices: false,
      },
    ]);
  };

  const removeItem = (key: number) => {
    setItems((current) => (current.length > 1 ? current.filter((item) => item.key !== key) : current));
  };

  const changeItem = (key: number, field: keyof EditorItem, value: string | string[] | boolean) => {
    setItems((current) =>
      current.map((item) => (item.key === key ? ({ ...item, [field]: value } as EditorItem) : item))
    );
  };

  const patchItems = (key: number, patch: (item: EditorItem) => EditorItem) => {
    setItems((current) => current.map((item) => (item.key === key ? patch(item) : item)));
  };

  const enableSubItems = (key: number) => {
    patchItems(key, (item) => {
      const converted = item.bullets
        .filter((line) => line.trim() !== "")
        .map((line) => newSubItem(nextSubId.current++, line.trim(), "Lot"));
      return {
        ...item,
        bullets: [],
        qty: "1",
        uom: "Lot",
        showSubPrices: false,
        subItems:
          converted.length > 0 ? converted : [newSubItem(nextSubId.current++, "", "Lot")],
      };
    });
  };

  const disableSubItems = (key: number) => {
    patchItems(key, (item) => ({
      ...item,
      bullets: item.subItems
        .map((sub) => sub.description.trim())
        .filter((line) => line !== ""),
      subItems: [],
      showSubPrices: false,
    }));
  };

  const addSubRow = (key: number) => {
    patchItems(key, (item) => ({
      ...item,
      subItems: [...item.subItems, newSubItem(nextSubId.current++, "", "Lot")],
    }));
  };

  const removeSubRow = (key: number, subId: number) => {
    patchItems(key, (item) => ({
      ...item,
      subItems: item.subItems.filter((sub) => sub.id !== subId),
    }));
  };

  const patchSubRow = (key: number, subId: number, patch: Partial<EditorSubItem>) => {
    patchItems(key, (item) => ({
      ...item,
      subItems: item.subItems.map((sub) => (sub.id === subId ? { ...sub, ...patch } : sub)),
    }));
  };

  const moveSubRow = (key: number, subId: number, direction: -1 | 1) => {
    patchItems(key, (item) => {
      const at = item.subItems.findIndex((sub) => sub.id === subId);
      const target = at + direction;
      if (at === -1 || target < 0 || target >= item.subItems.length) return item;
      const next = [...item.subItems];
      [next[at], next[target]] = [next[target], next[at]];
      return { ...item, subItems: next };
    });
  };

  const addSubBullet = (key: number, subId: number) => {
    patchSubRow(key, subId, { bullets: [...(findSub(items, key, subId)?.bullets ?? []), ""] });
  };

  const removeSubBullet = (key: number, subId: number, bulletIndex: number) => {
    const sub = findSub(items, key, subId);
    if (!sub) return;
    patchSubRow(key, subId, {
      bullets: sub.bullets.filter((_, index) => index !== bulletIndex),
    });
  };

  const subBulletChange = (key: number, subId: number, bulletIndex: number, value: string) => {
    const sub = findSub(items, key, subId);
    if (!sub) return;
    patchSubRow(key, subId, {
      bullets: sub.bullets.map((line, index) => (index === bulletIndex ? value : line)),
    });
  };

  const addBullet = (key: number) => {
    patchItems(key, (item) => ({ ...item, bullets: [...item.bullets, ""] }));
  };

  const removeBullet = (key: number, bulletIndex: number) => {
    patchItems(key, (item) => ({
      ...item,
      bullets: item.bullets.filter((_, index) => index !== bulletIndex),
    }));
  };

  const bulletChange = (key: number, bulletIndex: number, value: string) => {
    patchItems(key, (item) => ({
      ...item,
      bullets: item.bullets.map((line, index) => (index === bulletIndex ? value : line)),
    }));
  };

  const noop = () => {};

  const totals = useMemo(() => {
    let lineTotalsSum = 0;
    let rowDiscounts = 0;
    let subtotal = 0;
    for (const item of items) {
      const line = lineTotals(item);
      lineTotalsSum += line.list;
      subtotal += line.sell;
      rowDiscounts += line.list - line.sell;
    }
    const ppn = Math.round(subtotal * 0.11);
    const grand = subtotal + ppn;
    return { lineTotals: lineTotalsSum, rowDiscounts, subtotal, ppn, grand };
  }, [items]);

  const reviewItems = [
    {
      kind: "Changed",
      status: "Unresolved",
      label: "Payment terms",
      group: "Header",
      source: "30 days after delivery",
      po: "50% advance, balance 30 days after delivery",
    },
    {
      kind: "Changed",
      status: "Resolved",
      label: "Line 01",
      group: "Lines",
      source: "Discount: No discount",
      po: "Discount: 5%",
      resolved: "Kept PO",
    },
    {
      kind: "Extra",
      status: "Unresolved",
      label: "Line 04",
      group: "Lines",
      source: "Not in quotation",
      po: "Installation, termination and commissioning works",
    },
    {
      kind: "Changed",
      status: "Unresolved",
      label: "Line 03 · sub-item 1",
      group: "Sub-items",
      source: "Discount: No discount",
      po: "Discount: 5%",
    },
    {
      kind: "Missing",
      status: "Unresolved",
      label: "Source line 03",
      group: "Lines",
      source: "Freight and insurance",
      po: "Not in purchase order",
    },
    {
      kind: "Changed",
      status: "Unresolved",
      label: "Printed grand total",
      group: "Totals",
      source: "Rp54.200.000",
      po: formatMoney(IDR, totals.grand),
    },
  ];

  return (
    <div className="min-h-screen bg-paper pb-32 text-ink-800">
      <header className="sticky top-0 z-30 border-b border-rule bg-paper/95 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-[1240px] items-center gap-3 px-4 sm:px-6">
          <div className="flex items-center gap-2">
            <span className="grid h-[22px] w-[22px] place-items-center rounded-md bg-accent-700 text-[11px] font-semibold text-white">V</span>
            <span className="text-small font-semibold text-ink-950">Vera</span>
            <span className="text-caption text-ink-300">/</span>
            <span className="text-small text-ink-600">Purchase Orders</span>
          </div>
          <nav className="ml-3 hidden items-center gap-1 sm:flex" aria-label="Purchase order workspace">
            <span className="rounded-md bg-paper-muted px-2.5 py-1.5 text-small text-ink-900">Editor</span>
            <span className="px-2.5 py-1.5 text-small text-ink-500">Drafts</span>
            <span className="px-2.5 py-1.5 text-small text-ink-500">Archive</span>
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <span className="hidden rounded-full border border-rule px-2 py-1 text-micro uppercase tracking-[0.12em] text-ink-400 sm:inline">Prototype</span>
            <span className="grid h-7 w-7 place-items-center rounded-full bg-accent-100 text-[10px] font-semibold text-accent-800">HW</span>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-[1240px] px-4 py-6 sm:px-6">
        <div className="mb-4 rounded-lg border border-accent-200 bg-accent-50 px-3 py-2 text-caption text-accent-800">
          Prototype — the line items below are the actual <code className="font-mono">ItemEditor</code> imported from the quotation editor, in purchase-order mode. Split lines, sub-items and sub-item description lines are the same component and the same behavior.
        </div>

        <section className="relative overflow-hidden rounded-[14px] bg-accent-800 px-5 py-5 text-white shadow-card sm:px-6 sm:py-6">
          <div className="pointer-events-none absolute -bottom-8 right-4 select-none text-[122px] font-bold leading-none tracking-[-0.08em] text-white/5">PO</div>
          <div className="relative z-10 flex items-center justify-between gap-3">
            <span className="text-micro uppercase tracking-[0.18em] text-accent-200">Purchase order</span>
            <span className="rounded-md border border-white/25 bg-white/10 px-2 py-0.5 text-micro uppercase tracking-wide">Draft</span>
          </div>
          <div className="relative z-10 mt-4 flex items-end justify-between gap-6">
            <div>
              <h1 className="tnum text-[30px] font-semibold leading-none tracking-[-0.035em] sm:text-[34px]">PO-2026-0142</h1>
              <p className="mt-2 text-small text-accent-100">PT. Infineon Technologies Batam · Batamindo Industrial Park</p>
            </div>
            <div className="hidden text-right text-micro uppercase tracking-[0.1em] text-accent-300 sm:block">
              <span>Source quotation</span>
              <strong className="mt-1 block text-small normal-case tracking-normal text-white">1502260002 · rev 00</strong>
            </div>
          </div>
          <div className="relative z-10 mt-5 flex flex-wrap gap-x-7 gap-y-3 border-t border-white/15 pt-3 text-small text-accent-100">
            <span><span className="block text-[9px] uppercase tracking-[0.14em] text-accent-300">Issued</span>10 Nov 2026</span>
            <span><span className="block text-[9px] uppercase tracking-[0.14em] text-accent-300">Delivery</span>24 Nov 2026</span>
            <span><span className="block text-[9px] uppercase tracking-[0.14em] text-accent-300">Terms</span>DDP · IDR</span>
            <span><span className="block text-[9px] uppercase tracking-[0.14em] text-accent-300">Tax</span>Non-FTZ / Goods</span>
          </div>
        </section>

        <div className="mt-5 grid gap-6 xl:grid-cols-[minmax(0,1fr)_340px]">
          <div className="flex min-w-0 flex-col gap-6">
            <Card className="p-5">
              <SectionHeading title="Purchase order details" hint="Snapshot from the linked quotation" />
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Customer" span>
                  <input readOnly value="PT. Infineon Technologies Batam" className={`${fieldClass} bg-paper-subtle text-ink-600`} />
                </Field>
                <Field label="Customer PO number">
                  <input defaultValue="PO-2026-0142" className={fieldClass} />
                </Field>
                <Field label="PO date">
                  <input type="date" defaultValue="2026-11-10" className={fieldClass} />
                </Field>
                <Field label="Received date">
                  <input type="date" defaultValue="2026-11-10" className={fieldClass} />
                </Field>
                <Field label="Requested delivery">
                  <input type="date" defaultValue="2026-11-24" className={fieldClass} />
                </Field>
                <Field label="Payment terms" span>
                  <input defaultValue="50% advance, balance 30 days after delivery" className={fieldClass} />
                </Field>
                <Field label="Incoterms">
                  <select defaultValue="DDP" className={fieldClass}>
                    <option>DDP</option><option>FOB</option><option>CIF</option><option>EXW</option>
                  </select>
                </Field>
                <Field label="Ship-to code">
                  <input defaultValue="BAT-INF-01" className={fieldClass} />
                </Field>
                <Field label="Contact email">
                  <input defaultValue="hendra.wijaya@infineon.com" className={fieldClass} />
                </Field>
                <Field label="Contact phone">
                  <input defaultValue="+62 811 7700 142" className={fieldClass} />
                </Field>
                <Field label="Billing address" span>
                  <textarea readOnly defaultValue="Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia" className={`${fieldClass} min-h-20 resize-y bg-paper-subtle text-ink-600`} />
                </Field>
                <Field label="Delivery address" span>
                  <textarea readOnly defaultValue="Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia" className={`${fieldClass} min-h-20 resize-y bg-paper-subtle text-ink-600`} />
                </Field>
              </div>
            </Card>

            <section>
              <SectionHeading
                title="Line items"
                hint={`${items.length} lines · ${items.reduce((count, item) => count + item.subItems.length, 0)} sub-items`}
                action={<Button size="sm" onClick={addItem}>+ Add line</Button>}
              />
              <ItemEditor
                items={items}
                currency={IDR}
                uomOptions={UOMS}
                mode="purchase-order"
                onAddItem={addItem}
                onRemoveItem={removeItem}
                onChange={changeItem}
                onAddBullet={addBullet}
                onRemoveBullet={removeBullet}
                onBulletChange={bulletChange}
                onEnableSubItems={enableSubItems}
                onDisableSubItems={disableSubItems}
                onAddSubRow={addSubRow}
                onRemoveSubRow={removeSubRow}
                onPatchSubRow={patchSubRow}
                onMoveSubRow={moveSubRow}
                onPasteSubRows={noop}
                onAddSubBullet={addSubBullet}
                onRemoveSubBullet={removeSubBullet}
                onSubBulletChange={subBulletChange}
                onPasteItems={noop}
              />
            </section>
          </div>

          <aside className="flex flex-col gap-4 xl:sticky xl:top-20 xl:self-start">
            <Card className="p-5">
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-caption uppercase tracking-[0.14em] text-ink-500">Summary</h2>
                <span className="tnum text-caption text-ink-400">{items.length} lines</span>
              </div>
              <div className="mt-4 space-y-2.5">
                <SummaryRow label="Line totals" value={formatMoney(IDR, totals.lineTotals)} />
                <SummaryRow label="Row discounts" value={`−${formatMoney(IDR, totals.rowDiscounts)}`} muted />
                <SummaryRow label="Subtotal" value={formatMoney(IDR, totals.subtotal)} />
                <SummaryRow label="PPN 11%" value={formatMoney(IDR, totals.ppn)} />
              </div>
              <div className="mt-4 border-t border-rule pt-3">
                <div className="flex items-baseline justify-between gap-4">
                  <span className="text-small font-medium text-ink-800">Grand total</span>
                  <span className="tnum text-lead font-semibold text-ink-950">{formatMoney(IDR, totals.grand)}</span>
                </div>
                <p className="mt-1 text-caption text-ink-500">Customer-facing, including PPN 11%</p>
              </div>
            </Card>

            <Card className="p-4">
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-caption uppercase tracking-[0.14em] text-ink-500">Review differences</h2>
                <Badge tone="internal">5 unresolved</Badge>
              </div>
              <p className="mt-1 text-caption text-ink-500">Compared with quotation <span className="tnum font-medium text-ink-700">1502260002 · rev 00</span></p>
              <div className="mt-3 divide-y divide-rule border-y border-rule">
                {reviewItems.map((item) => (
                  <div key={`${item.label}-${item.kind}`} className="py-3">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge tone={item.kind === "Missing" ? "danger" : item.kind === "Changed" ? "internal" : "neutral"}>{item.kind}</Badge>
                      <span className="text-micro uppercase tracking-wide text-ink-400">{item.status}</span>
                      <span className="text-small font-medium text-ink-900">{item.label}</span>
                    </div>
                    <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-caption">
                      <span className="rounded bg-paper-subtle px-1.5 py-0.5 text-ink-600">{item.source}</span>
                      <span className="text-ink-300">→</span>
                      <span className="rounded bg-accent-50 px-1.5 py-0.5 text-accent-800">{item.po}</span>
                    </div>
                    <div className="mt-2 flex justify-end">
                      {item.resolved ? <span className="rounded-full border border-success-200 bg-success-50 px-2 py-0.5 text-micro text-success-700">{item.resolved}</span> : <span className="text-caption text-ink-400">Use quotation · Keep PO</span>}
                    </div>
                  </div>
                ))}
              </div>
              <Button variant="secondary" size="sm" className="mt-3 w-full">Open full review</Button>
            </Card>
          </aside>
        </div>
      </main>

      <div className="fixed inset-x-0 bottom-0 z-40 border-t border-rule bg-paper/95 backdrop-blur">
        <div className="mx-auto flex max-w-[1240px] flex-wrap items-center justify-between gap-3 px-4 py-2.5 sm:px-6">
          <div className="flex flex-wrap items-center gap-2 text-caption text-ink-500">
            <Badge tone="internal">Unsaved</Badge>
            <span>Draft · not finalized · 5 unresolved</span>
          </div>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <Button variant="ghost">Cancel</Button>
            <Button variant="secondary">Save draft</Button>
            <Button variant="primary">Final Save</Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function findSub(items: EditorItem[], key: number, subId: number): EditorSubItem | undefined {
  return items.find((item) => item.key === key)?.subItems.find((sub) => sub.id === subId);
}

createRoot(document.getElementById("root")!).render(<App />);
