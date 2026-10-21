import type { ReactNode } from "react";
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
//
// Wave 2 renders the whole retained snapshot, grouped, rather than a handful of
// fields: a correction that changed only the billing address, the ship-to code,
// a contact detail or a date is still understandable months later.

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

/** One labelled group of retained values inside the "Previous values" block. */
function HistoryGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="min-w-0">
      <h3 className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
        {title}
      </h3>
      <div className="mt-1">{children}</div>
    </section>
  );
}

/** The two-column value grid every header group shares. */
function HistoryFields({ children }: { children: ReactNode }) {
  return <dl className="grid gap-2 text-small sm:grid-cols-2">{children}</dl>;
}

/**
 * One retained header value. An empty stored value reads as "—" instead of a
 * blank, so "the field was empty then" is distinguishable from "the panel does
 * not show this field".
 */
function HistoryField({
  label,
  value,
  className = "",
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-caption text-ink-500">{label}</dt>
      <dd className={`break-words text-ink-900 ${className}`.trim()}>
        {value.trim() === "" ? "—" : value}
      </dd>
    </div>
  );
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
                <span className="font-medium text-ink-600">Customer PO PDF: </span>
                {attachmentReferenceLabel(entry.Attachment)}
              </p>

              <details className="mt-2" open={expandPreviousValues}>
                <summary className="cursor-pointer text-small font-medium text-accent-700">
                  Previous values
                </summary>
                <div className="mt-3 grid gap-4">
                  <HistoryGroup title="Identity">
                    <HistoryFields>
                      <HistoryField label="Quote number" value={entry.QuoteNumber} className="tnum" />
                      <HistoryField label="Customer" value={entry.CustomerName} />
                      <HistoryField
                        label="Source revision"
                        value={String(entry.SourceRevision)}
                        className="tnum"
                      />
                      <HistoryField label="State" value={entry.State} />
                      <HistoryField label="Currency" value={entry.Currency} className="tnum" />
                      <HistoryField label="Tax zone" value={entry.TaxZone} />
                      <HistoryField label="Tax category" value={entry.TaxCategory} />
                      <HistoryField label="Customer PO number" value={entry.PONumber} className="tnum" />
                    </HistoryFields>
                  </HistoryGroup>

                  <HistoryGroup title="Addresses and contact">
                    <HistoryFields>
                      <HistoryField label="Billing address" value={entry.BillingAddress} />
                      <HistoryField label="Delivery address" value={entry.DeliveryAddress} />
                      <HistoryField label="Ship-to code" value={entry.ShipToCode} />
                      <HistoryField label="Contact email" value={entry.ContactEmail} />
                      <HistoryField label="Contact phone" value={entry.ContactPhone} className="tnum" />
                    </HistoryFields>
                  </HistoryGroup>

                  <HistoryGroup title="Dates and terms">
                    <HistoryFields>
                      <HistoryField label="PO date" value={entry.PODate ?? ""} className="tnum" />
                      <HistoryField label="Received date" value={entry.ReceivedDate} className="tnum" />
                      <HistoryField
                        label="Requested delivery date"
                        value={entry.RequestedDeliveryDate ?? ""}
                        className="tnum"
                      />
                      <HistoryField label="Payment terms" value={entry.PaymentTerms} />
                      <HistoryField label="Incoterms" value={entry.Incoterms} />
                    </HistoryFields>
                  </HistoryGroup>

                  <HistoryGroup title="Lines">
                    <div className="max-w-full overflow-x-auto scroll-thin">
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
                  </HistoryGroup>

                  <HistoryGroup title="Totals">
                    <dl className="grid gap-1 text-small text-ink-700">
                      <div className="flex flex-wrap items-baseline justify-between gap-2">
                        <dt>Whole-PO discount</dt>
                        <dd className="tnum whitespace-nowrap text-ink-900">
                          {entry.DiscountPercent}%
                        </dd>
                      </div>
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
                        <dt>{`PPN (${entry.PpnBps} bps)`}</dt>
                        <dd className="tnum whitespace-nowrap text-ink-900">
                          {formatMoney(currency, entry.PpnMinor)}
                        </dd>
                      </div>
                      {entry.AdjustmentMinor !== 0 ? (
                        <div className="flex flex-wrap items-baseline justify-between gap-2">
                          <dt>
                            Adjustment
                            {entry.AdjustmentReason === ""
                              ? ""
                              : ` · ${entry.AdjustmentReason}`}
                          </dt>
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
                  </HistoryGroup>
                </div>
              </details>
            </div>
          </li>
        ))}
      </ol>
    </Card>
  );
}
