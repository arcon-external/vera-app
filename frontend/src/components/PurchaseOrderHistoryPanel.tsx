import { formatMoney, type CurrencyInfo } from "../format";
import {
  attachmentReferenceLabel,
  correctionKindLabel,
  formatHistoryTime,
  type PurchaseOrderHistoryEntry,
} from "../poCorrection";
import { formatQuantityScaled } from "../purchaseOrder";
import { Badge, Card, SectionHeading, StatusBanner } from "./ui";

// Issue #473: the readable correction history. Every entry is an immutable
// snapshot of the values a correction replaced, with the reason staff typed,
// the actor and time, and the customer PO PDF version the record referenced at
// that moment. Nothing here reads the live record, so a later source-link
// repair (#475) cannot rewrite what the history says.

export interface PurchaseOrderHistoryPanelProps {
  entries: PurchaseOrderHistoryEntry[];
  currency: CurrencyInfo;
  loading?: boolean;
  error?: string | null;
  /**
   * Opens every "Previous values" disclosure on first render. The snapshot
   * harness uses it so the evidence shows the retained values; the app leaves
   * it closed so a long history stays scannable.
   */
  expandPreviousValues?: boolean;
}

function historyTime(iso: string): string {
  const formatted = formatHistoryTime(iso);
  return formatted === "" ? "an unknown time" : formatted;
}

export function PurchaseOrderHistoryPanel({
  entries,
  currency,
  loading = false,
  error = null,
  expandPreviousValues = false,
}: PurchaseOrderHistoryPanelProps) {
  return (
    <Card className="p-4" data-po-history>
      <SectionHeading
        title="Correction history"
        hint={
          entries.length === 0
            ? "Nothing recorded yet"
            : `${entries.length} ${entries.length === 1 ? "snapshot" : "snapshots"}`
        }
      />
      {loading ? (
        <p className="mt-2 text-small text-ink-500" role="status">
          Loading the correction history…
        </p>
      ) : null}
      {error !== null ? (
        <div className="mt-2">
          <StatusBanner tone="danger">{error}</StatusBanner>
        </div>
      ) : null}
      {!loading && error === null && entries.length === 0 ? (
        <p className="mt-2 rounded-lg border border-dashed border-rule px-3 py-4 text-small text-ink-500">
          No corrections have been recorded for this purchase order.
        </p>
      ) : null}

      <ol className="mt-3 grid gap-3">
        {entries.map((entry) => (
          <li key={entry.UUID} className="min-w-0">
            <div className="min-w-0 rounded-lg border border-rule bg-paper-subtle/40 p-3">
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone={entry.Kind === "draft_overwrite" ? "neutral" : "accent"}>
                  {correctionKindLabel(entry.Kind)}
                </Badge>
                <span className="tnum whitespace-nowrap text-small font-medium text-ink-900">
                  Version {entry.PreviousVersion} → {entry.ResultingVersion}
                </span>
                <span className="text-caption text-ink-500">
                  {entry.ActorName.trim() === "" ? "Unknown staff member" : entry.ActorName}{" "}
                  · {historyTime(entry.CreatedAt)}
                </span>
              </div>
              <p className="mt-2 text-small text-ink-800">
                <span className="font-medium text-ink-900">Reason: </span>
                {entry.Reason}
              </p>
              <p className="mt-1 text-caption text-ink-500">
                {attachmentReferenceLabel(entry.Attachment)}
              </p>

              <details className="mt-2" open={expandPreviousValues}>
                <summary className="cursor-pointer text-small font-medium text-accent-700">
                  Previous values
                </summary>
                <dl className="mt-3 grid gap-2 text-small sm:grid-cols-2">
                  <div>
                    <dt className="text-caption text-ink-500">Customer PO number</dt>
                    <dd className="tnum text-ink-900">{entry.PONumber || "—"}</dd>
                  </div>
                  <div>
                    <dt className="text-caption text-ink-500">Payment terms</dt>
                    <dd className="text-ink-900">{entry.PaymentTerms || "—"}</dd>
                  </div>
                  <div>
                    <dt className="text-caption text-ink-500">Incoterms</dt>
                    <dd className="text-ink-900">{entry.Incoterms || "—"}</dd>
                  </div>
                  <div>
                    <dt className="text-caption text-ink-500">Delivery address</dt>
                    <dd className="text-ink-900">{entry.DeliveryAddress || "—"}</dd>
                  </div>
                  {entry.AdjustmentMinor !== 0 ? (
                    <div className="sm:col-span-2">
                      <dt className="text-caption text-ink-500">Adjustment</dt>
                      <dd className="tnum whitespace-nowrap text-ink-900">
                        {formatMoney(currency, entry.AdjustmentMinor)}
                        {entry.AdjustmentReason === ""
                          ? ""
                          : ` · ${entry.AdjustmentReason}`}
                      </dd>
                    </div>
                  ) : null}
                </dl>

                <div className="mt-3 max-w-full overflow-x-auto scroll-thin">
                  <table className="w-full min-w-[560px] text-left text-body">
                    <caption className="sr-only">
                      Lines replaced by this correction
                    </caption>
                    <thead>
                      <tr className="border-b border-rule text-micro uppercase tracking-[0.1em] text-ink-500">
                        <th className="py-2 pr-3 font-medium">Line</th>
                        <th className="py-2 pr-3 font-medium">Description</th>
                        <th className="py-2 pr-3 text-right font-medium">Quantity</th>
                        <th className="py-2 pr-3 font-medium">UoM</th>
                        <th className="py-2 pr-3 text-right font-medium">Unit price</th>
                        <th className="py-2 pr-3 text-right font-medium">Discount</th>
                        <th className="py-2 text-right font-medium">Line total</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-rule">
                      {entry.Items.map((item) => (
                        <tr key={item.LineNumber}>
                          <td className="tnum py-2 pr-3 align-middle text-ink-700">
                            {item.LineNumber}
                          </td>
                          <td className="py-2 pr-3 align-middle text-ink-900">
                            {item.Description || "—"}
                          </td>
                          <td className="tnum py-2 pr-3 text-right align-middle text-ink-700">
                            {formatQuantityScaled(item.QuantityScaled)}
                          </td>
                          <td className="py-2 pr-3 align-middle text-ink-700">
                            {item.UoM || "—"}
                          </td>
                          <td className="tnum whitespace-nowrap py-2 pr-3 text-right align-middle text-ink-700">
                            {formatMoney(currency, item.UnitPriceMinor)}
                          </td>
                          <td className="tnum whitespace-nowrap py-2 pr-3 text-right align-middle text-ink-700">
                            {item.DiscountMinor === 0
                              ? "—"
                              : formatMoney(currency, -item.DiscountMinor)}
                          </td>
                          <td className="tnum whitespace-nowrap py-2 text-right align-middle text-ink-900">
                            {formatMoney(currency, item.NetMinor)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                <dl className="mt-3 grid gap-1 text-small text-ink-700">
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <dt>Subtotal</dt>
                    <dd className="tnum whitespace-nowrap text-ink-900">
                      {formatMoney(currency, entry.SubtotalMinor)}
                    </dd>
                  </div>
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <dt>Discount</dt>
                    <dd className="tnum whitespace-nowrap text-ink-900">
                      {formatMoney(currency, -entry.DiscountMinor)}
                    </dd>
                  </div>
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <dt>PPN</dt>
                    <dd className="tnum whitespace-nowrap text-ink-900">
                      {formatMoney(currency, entry.PpnMinor)}
                    </dd>
                  </div>
                  {entry.AdjustmentMinor !== 0 ? (
                    <div className="flex flex-wrap items-baseline justify-between gap-2">
                      <dt>Adjustment</dt>
                      <dd className="tnum whitespace-nowrap text-ink-900">
                        {formatMoney(currency, entry.AdjustmentMinor)}
                      </dd>
                    </div>
                  ) : null}
                  <div className="flex flex-wrap items-baseline justify-between gap-2 border-t border-rule pt-1 font-medium text-ink-900">
                    <dt>Grand total</dt>
                    <dd className="tnum whitespace-nowrap">
                      {formatMoney(currency, entry.GrandTotalMinor)}
                    </dd>
                  </div>
                </dl>
              </details>
            </div>
          </li>
        ))}
      </ol>
    </Card>
  );
}
