import {
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { GetPurchaseOrderFulfillment } from "@backend/backend/Quotation";
import { errorMessage } from "../errors";
import type { CurrencyInfo } from "../format";
import {
  FULFILLMENT_DEFAULT_PAGE_SIZE,
  FULFILLMENT_STATUS_OPTIONS,
  formatFulfillmentDate,
  formatFulfillmentLoss,
  formatFulfillmentMoney,
  formatFulfillmentQuantity,
  fulfillmentPageBounds,
  fulfillmentQueryKey,
  fulfillmentStatusLabel,
  fulfillmentStatusTone,
  groupFulfillmentItems,
  normalizePurchaseOrderFulfillmentPage,
  type PurchaseOrderFulfillmentItem,
  type PurchaseOrderFulfillmentPage,
  type PurchaseOrderFulfillmentQuery,
  type PurchaseOrderFulfillmentQuotation,
} from "../poFulfillment";
import { Badge, Button, Card, StatusBanner } from "./ui";

/**
 * Issue #657 phase B: the Fulfillment drill-down card.
 *
 * It is its own paginated read, deliberately not part of the #629 report
 * snapshot: the Reports page can show Actual, Pipeline and Variance without
 * paying for a drill-down nobody opened, and the card can page without
 * re-reading the report. The server owns the status derivation and every
 * figure; this card renders them and never computes a cost.
 *
 * There is no date filter, and the copy says so: bought state spans revisions
 * and time, so a date range would answer half the question.
 */

export type PurchaseOrderFulfillmentLoader = (
  query: PurchaseOrderFulfillmentQuery
) => Promise<PurchaseOrderFulfillmentPage>;

const defaultLoader: PurchaseOrderFulfillmentLoader = async (query) =>
  normalizePurchaseOrderFulfillmentPage(
    await GetPurchaseOrderFulfillment(query)
  );

export interface PurchaseOrderFulfillmentCardProps {
  /** The report's selected currency; the drill-down is one partition. */
  currency: string;
  /** Null until the catalogue answers; a missing code renders "-". */
  catalogue: readonly CurrencyInfo[] | null;
  /** False while the Reports page is mounted but hidden. */
  active?: boolean;
  /** Bumped by App after a PO save/correction/delete or a quotation close/reopen. */
  mutationEpoch?: number;
  /** Bumped by the Reports page's own Refresh control. */
  refreshToken?: number;
  /** Injected by tests; the live page uses the Wails binding. */
  load?: PurchaseOrderFulfillmentLoader;
}

interface ReadState {
  key: string;
  page: PurchaseOrderFulfillmentPage | null;
  error: string;
  loading: boolean;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return formatFulfillmentMoney(entry, minor);
}

// The table's one column model. Every quotation renders this exact colgroup, so
// an open quotation and a closed one have identical boundaries: a reader can
// scan down the screen without the columns shifting. Widths are percentages of
// the card's content width, and `table-fixed` makes the browser honour them
// instead of growing a column to fit one long value.
const FULFILLMENT_COLUMN_WIDTHS = [
  "5%", // item
  "16%", // description
  "6%", // quoted
  "6%", // bought
  "6%", // unsold
  "6%", // over-bought
  "12%", // realized
  "12%", // unsold value
  "13%", // variance
  "18%", // saved POs
] as const;

// The table starts at md, where ten fixed columns are tight, and gets its normal
// reading size at xl. The quantity headers are one short word each, so they stay
// a step smaller at every width to fit their 6% columns.
const TABLE_HEAD_CELL =
  "px-2 py-2 text-left text-[10px] font-medium uppercase tracking-normal text-ink-500 xl:px-3 xl:text-[11px]";
const TABLE_HEAD_ITEM =
  "px-1 py-2 text-left text-[10px] font-medium uppercase tracking-normal text-ink-500 xl:px-2 xl:text-[11px]";
const TABLE_HEAD_QUANTITY =
  "px-1 py-2 text-right text-[9px] font-medium uppercase tracking-normal text-ink-500 xl:px-2 xl:text-[10px]";
const TABLE_BODY_CELL = "px-2 py-2 xl:px-3";
const TABLE_ITEM_CELL = "px-1 py-2 xl:px-2";
const TABLE_QUANTITY_CELL =
  "tnum whitespace-nowrap px-1 py-2 text-right text-small text-ink-900 xl:px-2";

function FulfillmentColgroup() {
  return (
    <colgroup>
      {FULFILLMENT_COLUMN_WIDTHS.map((width, index) => (
        <col key={index} style={{ width }} />
      ))}
    </colgroup>
  );
}

function MoneyLine({
  label,
  name,
  value,
  negative = false,
  narrow = false,
}: {
  label: string;
  name: string;
  value: string;
  negative?: boolean;
  narrow?: boolean;
}) {
  return (
    <div className="flex flex-col gap-0 xl:flex-row xl:items-baseline xl:justify-between xl:gap-3">
      <dt className="text-[10px] text-ink-500 xl:text-caption">{label}</dt>
      <dd
        data-fulfillment-money={narrow ? undefined : name}
        data-fulfillment-narrow-money={narrow ? name : undefined}
        className={`tnum whitespace-nowrap text-small font-medium ${
          negative ? "text-danger-700" : "text-ink-950"
        } xl:text-right`}
      >
        {value}
      </dd>
    </div>
  );
}

function QuantityCell({
  name,
  value,
}: {
  name: string;
  value: string;
}) {
  return (
    <td data-fulfillment-quantity={name} className={TABLE_QUANTITY_CELL}>
      {value}
    </td>
  );
}

function varianceParts(
  item: PurchaseOrderFulfillmentItem,
  quotationStatus: string
): { quantityExtra: boolean; showClosedLoss: boolean; hasVariance: boolean } {
  const quantityExtra =
    item.overBoughtQuantityScaled > 0 ||
    item.quantityExtraIncomeAmountMinor !== 0 ||
    item.quantityExtraIncomeMarginMinor !== 0;
  const showClosedLoss =
    quotationStatus === "closed" ||
    item.closedLossAmountMinor !== 0 ||
    item.closedLossMarginMinor !== 0;
  return {
    quantityExtra,
    showClosedLoss,
    hasVariance:
      quantityExtra ||
      item.priceExtraIncomeMinor !== 0 ||
      item.priceLossMinor !== 0 ||
      showClosedLoss,
  };
}

function VarianceLines({
  item,
  quotationStatus,
  entry,
  narrow = false,
}: {
  item: PurchaseOrderFulfillmentItem;
  quotationStatus: string;
  entry: CurrencyInfo | null;
  narrow?: boolean;
}) {
  const { quantityExtra, showClosedLoss, hasVariance } = varianceParts(
    item,
    quotationStatus
  );
  if (!hasVariance) {
    return <span className="text-small text-ink-500">—</span>;
  }
  return (
    <dl className="space-y-1">
      {quantityExtra ? (
        <>
          <MoneyLine
            narrow={narrow}
            label="Qty extra"
            name="quantity-extra-amount"
            value={money(entry, item.quantityExtraIncomeAmountMinor)}
          />
          <MoneyLine
            narrow={narrow}
            label="Qty margin"
            name="quantity-extra-margin"
            value={money(entry, item.quantityExtraIncomeMarginMinor)}
            negative={item.quantityExtraIncomeMarginMinor < 0}
          />
        </>
      ) : null}
      {item.priceExtraIncomeMinor !== 0 ? (
        <MoneyLine
          narrow={narrow}
          label="Price extra"
          name="price-extra-income"
          value={money(entry, item.priceExtraIncomeMinor)}
        />
      ) : null}
      {item.priceLossMinor !== 0 ? (
        <MoneyLine
          narrow={narrow}
          label="Price loss"
          name="price-loss"
          value={formatFulfillmentLoss(entry, item.priceLossMinor)}
          negative
        />
      ) : null}
      {showClosedLoss ? (
        <>
          <MoneyLine
            narrow={narrow}
            label="Closed loss"
            name="closed-loss-amount"
            value={formatFulfillmentLoss(entry, item.closedLossAmountMinor)}
            negative={item.closedLossAmountMinor !== 0}
          />
          <MoneyLine
            narrow={narrow}
            label="Closed margin"
            name="closed-loss-margin"
            value={formatFulfillmentLoss(entry, item.closedLossMarginMinor)}
            negative={item.closedLossMarginMinor !== 0}
          />
        </>
      ) : null}
    </dl>
  );
}

function ContributionList({
  item,
  narrow = false,
}: {
  item: PurchaseOrderFulfillmentItem;
  narrow?: boolean;
}) {
  if (item.purchaseOrders.length === 0) {
    return <span className="text-small text-ink-500">None saved</span>;
  }
  return (
    <ul className="space-y-1">
      {item.purchaseOrders.map((contribution) => (
        <li
          key={`${contribution.id}-${contribution.poNumber}`}
          data-fulfillment-contribution={narrow ? undefined : contribution.poNumber}
          data-fulfillment-narrow-contribution={
            narrow ? contribution.poNumber : undefined
          }
          className="break-words text-small text-ink-700"
        >
          <span className="block font-medium text-ink-950">
            {contribution.poNumber}
          </span>
          <span className="mt-0.5 block text-caption text-ink-500">
            <span className="tnum">
              {formatFulfillmentQuantity(contribution.allocatedQuantityScaled)}
            </span>{" "}
            · {formatFulfillmentDate(contribution.receivedDate)}
          </span>
        </li>
      ))}
    </ul>
  );
}

function ItemRow({
  item,
  quotationStatus,
  entry,
}: {
  item: PurchaseOrderFulfillmentItem;
  quotationStatus: string;
  entry: CurrencyInfo | null;
}) {
  return (
    <tr
      data-fulfillment-item={item.label}
      data-fulfillment-group={item.groupTitle === "" ? undefined : item.groupTitle}
      className="border-t border-rule align-top"
    >
      <td className={`${TABLE_ITEM_CELL} text-small font-medium text-ink-950`}>
        {item.label}
      </td>
      <td className={`${TABLE_BODY_CELL} break-words text-small text-ink-700`}>
        {item.description}
        {item.removed ? (
          <span
            data-fulfillment-removed=""
            className="ml-2 whitespace-nowrap text-[10px] font-medium text-danger-700"
          >
            Removed
          </span>
        ) : null}
      </td>
      <QuantityCell
        name="quoted"
        value={formatFulfillmentQuantity(item.quotedQuantityScaled)}
      />
      <QuantityCell
        name="bought"
        value={formatFulfillmentQuantity(item.boughtQuantityScaled)}
      />
      <QuantityCell
        name="unsold"
        value={formatFulfillmentQuantity(item.unsoldQuantityScaled)}
      />
      <QuantityCell
        name="over-bought"
        value={formatFulfillmentQuantity(item.overBoughtQuantityScaled)}
      />
      <td className={TABLE_BODY_CELL}>
        <dl className="space-y-1">
          <MoneyLine
            label="Amount"
            name="realized-amount"
            value={money(entry, item.realizedAmountMinor)}
          />
          <MoneyLine
            label="Margin"
            name="realized-margin"
            value={money(entry, item.realizedMarginMinor)}
            negative={item.realizedMarginMinor < 0}
          />
        </dl>
      </td>
      <td className={TABLE_BODY_CELL}>
        <dl className="space-y-1">
          <MoneyLine
            label="Amount"
            name="unsold-amount"
            value={money(entry, item.unsoldAmountMinor)}
          />
          <MoneyLine
            label="Margin"
            name="unsold-margin"
            value={money(entry, item.unsoldMarginMinor)}
            negative={item.unsoldMarginMinor < 0}
          />
        </dl>
      </td>
      <td className={TABLE_BODY_CELL}>
        <VarianceLines
          item={item}
          quotationStatus={quotationStatus}
          entry={entry}
        />
      </td>
      <td className={TABLE_BODY_CELL}>
        <ContributionList item={item} />
      </td>
    </tr>
  );
}

function NarrowQuantity({
  name,
  label,
  value,
}: {
  name: string;
  label: string;
  value: string;
}) {
  return (
    <div>
      <dt className="text-[10px] font-medium uppercase leading-tight tracking-normal text-ink-500">
        {label}
      </dt>
      <dd
        data-fulfillment-narrow-quantity={name}
        className="tnum mt-0.5 text-small font-medium text-ink-950"
      >
        {value}
      </dd>
    </div>
  );
}

function NarrowItem({
  item,
  quotationStatus,
  entry,
}: {
  item: PurchaseOrderFulfillmentItem;
  quotationStatus: string;
  entry: CurrencyInfo | null;
}) {
  return (
    <div
      data-fulfillment-narrow-item={item.label}
      data-fulfillment-narrow-group={
        item.groupTitle === "" ? undefined : item.groupTitle
      }
      className="border-t border-rule px-3 py-3 first:border-t-0"
    >
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="text-small font-semibold text-ink-950">
          {item.label}
        </span>
        {item.removed ? (
          <span
            data-fulfillment-narrow-removed=""
            className="text-[10px] font-medium text-danger-700"
          >
            Removed
          </span>
        ) : null}
      </div>
      <p className="mt-1 text-small leading-relaxed text-ink-700">
        {item.description}
      </p>

      <dl className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
        <NarrowQuantity
          name="quoted"
          label="Quoted"
          value={formatFulfillmentQuantity(item.quotedQuantityScaled)}
        />
        <NarrowQuantity
          name="bought"
          label="Bought"
          value={formatFulfillmentQuantity(item.boughtQuantityScaled)}
        />
        <NarrowQuantity
          name="unsold"
          label="Unsold"
          value={formatFulfillmentQuantity(item.unsoldQuantityScaled)}
        />
        <NarrowQuantity
          name="over-bought"
          label="Over-bought"
          value={formatFulfillmentQuantity(item.overBoughtQuantityScaled)}
        />
      </dl>

      <div className="mt-3 grid grid-cols-2 gap-3">
        <div>
          <p className="text-[10px] font-semibold uppercase tracking-normal text-ink-500">
            Realized
          </p>
          <dl className="mt-1 space-y-1">
            <MoneyLine
              narrow
              label="Amount"
              name="realized-amount"
              value={money(entry, item.realizedAmountMinor)}
            />
            <MoneyLine
              narrow
              label="Margin"
              name="realized-margin"
              value={money(entry, item.realizedMarginMinor)}
              negative={item.realizedMarginMinor < 0}
            />
          </dl>
        </div>
        <div>
          <p className="text-[10px] font-semibold uppercase tracking-normal text-ink-500">
            Unsold value
          </p>
          <dl className="mt-1 space-y-1">
            <MoneyLine
              narrow
              label="Amount"
              name="unsold-amount"
              value={money(entry, item.unsoldAmountMinor)}
            />
            <MoneyLine
              narrow
              label="Margin"
              name="unsold-margin"
              value={money(entry, item.unsoldMarginMinor)}
              negative={item.unsoldMarginMinor < 0}
            />
          </dl>
        </div>
      </div>

      <div className="mt-3">
        <p className="text-[10px] font-semibold uppercase tracking-normal text-ink-500">
          Variance
        </p>
        <div className="mt-1">
          <VarianceLines
            narrow
            item={item}
            quotationStatus={quotationStatus}
            entry={entry}
          />
        </div>
      </div>

      <div className="mt-3">
        <p className="text-[10px] font-semibold uppercase tracking-normal text-ink-500">
          Saved POs
        </p>
        <div className="mt-1">
          <ContributionList narrow item={item} />
        </div>
      </div>
    </div>
  );
}

function NarrowQuotationItems({
  quotation,
  entry,
}: {
  quotation: PurchaseOrderFulfillmentQuotation;
  entry: CurrencyInfo | null;
}) {
  const groups = groupFulfillmentItems(quotation.items);
  if (quotation.items.length === 0) {
    return (
      <p className="px-3 py-3 text-small text-ink-500">
        No priced leaves on this quotation.
      </p>
    );
  }
  return (
    <>
      {groups.map((group) => (
        <div key={`${group.title}-${group.items[0]?.label ?? ""}`}>
          {group.title === "" ? null : (
            <div
              data-fulfillment-narrow-group-title={group.title}
              className="border-t border-rule bg-paper-subtle/30 px-3 py-1.5 text-caption font-semibold uppercase tracking-[0.08em] text-ink-600"
            >
              {group.title}
            </div>
          )}
          {group.items.map((item) => (
            <NarrowItem
              key={item.label}
              item={item}
              quotationStatus={quotation.status}
              entry={entry}
            />
          ))}
        </div>
      ))}
    </>
  );
}

function QuotationBlock({
  quotation,
  entry,
}: {
  quotation: PurchaseOrderFulfillmentQuotation;
  entry: CurrencyInfo | null;
}) {
  const groups = groupFulfillmentItems(quotation.items);
  return (
    <article
      data-fulfillment-quotation={quotation.quoteNumber}
      className="rounded-lg border border-rule"
    >
      <header className="flex flex-wrap items-baseline justify-between gap-3 border-b border-rule bg-paper-subtle/40 px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-body font-semibold text-ink-950">
              {quotation.quoteNumber}
            </h3>
            <span data-fulfillment-status={quotation.status}>
              <Badge tone={fulfillmentStatusTone(quotation.status)}>
                {fulfillmentStatusLabel(quotation.status)}
              </Badge>
            </span>
          </div>
          <p className="mt-0.5 truncate text-small text-ink-600">
            {quotation.customerName}
          </p>
        </div>
        <dl className="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-caption text-ink-500">
          <div className="flex items-baseline gap-1">
            <dt>Revision</dt>
            <dd className="tnum font-medium text-ink-700">
              {quotation.latestRevision}
            </dd>
          </div>
          <div className="flex items-baseline gap-1">
            <dt>Issued</dt>
            <dd className="tnum font-medium text-ink-700">{quotation.issuedAt}</dd>
          </div>
          <div className="flex items-baseline gap-1">
            <dt>Valid until</dt>
            <dd className="tnum font-medium text-ink-700">
              {quotation.validUntil}
            </dd>
          </div>
        </dl>
      </header>

      {/*
        Below md: a stacked block per item. The numbers never scroll
        horizontally; they sit in a quantity grid (two columns at phone width,
        four from sm), a two-column value grid and full-width variance and
        contribution lists.
      */}
      <div className="md:hidden" data-fulfillment-narrow="">
        <NarrowQuotationItems quotation={quotation} entry={entry} />
      </div>

      {/*
        md and up: one fixed table. Both quotation tables share
        FULFILLMENT_COLUMN_WIDTHS, so their bodies line up across the screen.
      */}
      <div className="hidden md:block">
        <table
          data-fulfillment-table=""
          className="w-full table-fixed border-collapse"
        >
          <caption className="sr-only">
            Quoted, bought, unsold and variance figures for quotation{" "}
            {quotation.quoteNumber}
          </caption>
          <FulfillmentColgroup />
          <thead>
            <tr>
              <th scope="col" className={TABLE_HEAD_ITEM}>Item</th>
              <th scope="col" className={TABLE_HEAD_CELL}>Description</th>
              <th scope="col" className={TABLE_HEAD_QUANTITY}>Quoted</th>
              <th scope="col" className={TABLE_HEAD_QUANTITY}>Bought</th>
              <th scope="col" className={TABLE_HEAD_QUANTITY}>Unsold</th>
              <th scope="col" className={TABLE_HEAD_QUANTITY}>Over</th>
              <th scope="col" className={TABLE_HEAD_CELL}>Realized</th>
              <th scope="col" className={TABLE_HEAD_CELL}>Unsold value</th>
              <th scope="col" className={TABLE_HEAD_CELL}>Variance</th>
              <th scope="col" className={TABLE_HEAD_CELL}>Saved POs</th>
            </tr>
          </thead>
          {quotation.items.length === 0 ? (
            <tbody>
              <tr>
                <td colSpan={10} className="px-2 py-3 text-small text-ink-500 xl:px-3">
                  No priced leaves on this quotation.
                </td>
              </tr>
            </tbody>
          ) : (
            groups.map((group) => (
              <tbody key={`${group.title}-${group.items[0]?.label ?? ""}`}>
                {group.title === "" ? null : (
                  <tr data-fulfillment-group-title={group.title}>
                    <th
                      scope="colgroup"
                      colSpan={10}
                      className="bg-paper-subtle/30 px-2 py-1.5 text-left text-caption font-semibold uppercase tracking-[0.08em] text-ink-600 xl:px-3"
                    >
                      {group.title}
                    </th>
                  </tr>
                )}
                {group.items.map((item) => (
                  <ItemRow
                    key={item.label}
                    item={item}
                    quotationStatus={quotation.status}
                    entry={entry}
                  />
                ))}
              </tbody>
            ))
          )}
        </table>
      </div>

      <section
        data-fulfillment-extras=""
        className="border-t border-rule px-4 py-3"
      >
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h4 className="text-caption font-semibold uppercase tracking-[0.08em] text-ink-500">
            Extra expenses
          </h4>
          <p className="text-caption text-ink-500">
            Listed separately; never counted in the quantities above.
          </p>
        </div>
        {quotation.extraExpenses.length === 0 ? (
          <p className="mt-1.5 text-small text-ink-500">
            No extra expense lines on saved purchase orders.
          </p>
        ) : (
          <ul className="mt-2 space-y-1.5">
            {quotation.extraExpenses.map((extra) => (
              <li
                key={`${extra.purchaseOrderId}-${extra.lineNumber}`}
                data-fulfillment-extra={`${extra.poNumber}:${extra.lineNumber}`}
                className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 text-small text-ink-700"
              >
                <span className="min-w-0">
                  <span className="font-medium text-ink-950">
                    {extra.description}
                  </span>
                  {extra.groupTitle === "" ? null : (
                    <span className="ml-2 text-caption text-ink-500">
                      {extra.groupTitle}
                    </span>
                  )}
                  <span className="ml-2 text-caption text-ink-500">
                    {extra.poNumber} line {extra.lineNumber}
                  </span>
                </span>
                <span className="tnum whitespace-nowrap">
                  {formatFulfillmentQuantity(extra.quantityScaled)} {extra.uom} ·{" "}
                  {money(entry, extra.amountMinor)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </article>
  );
}

export interface PurchaseOrderFulfillmentCardViewProps {
  /** The already-loaded page; null while loading or after a failure. */
  page: PurchaseOrderFulfillmentPage | null;
  /** The report's selected currency, used until the page echoes its own. */
  currency: string;
  /** Null until the catalogue answers; a missing code renders "-". */
  catalogue: readonly CurrencyInfo[] | null;
  status: string;
  searchDraft: string;
  pageNumber: number;
  loading: boolean;
  error: string;
  onStatusChange: (status: string) => void;
  onSearchDraftChange: (value: string) => void;
  onSearchSubmit: () => void;
  onClearFilters: () => void;
  onPreviousPage: () => void;
  onNextPage: () => void;
  onRetry: () => void;
}

/**
 * The presentational fulfillment card. The container owns the loader, the
 * freshness epoch and the request state; this view owns the screen. The split
 * exists so the snapshot harness can render a settled page with
 * renderToString, which runs no effects: without it, every snapshot would show
 * only the loading shell.
 */
export function PurchaseOrderFulfillmentCardView({
  page,
  currency,
  catalogue,
  status,
  searchDraft,
  pageNumber,
  loading,
  error,
  onStatusChange,
  onSearchDraftChange,
  onSearchSubmit,
  onClearFilters,
  onPreviousPage,
  onNextPage,
  onRetry,
}: PurchaseOrderFulfillmentCardViewProps) {
  const displayCurrency = page?.filter.currency ?? currency;
  const entry =
    catalogue?.find((item) => item.code === displayCurrency) ?? null;
  const bounds = fulfillmentPageBounds(
    page?.total ?? 0,
    page?.pageSize ?? FULFILLMENT_DEFAULT_PAGE_SIZE,
    pageNumber
  );

  return (
    <Card
      data-purchase-order-fulfillment=""
      className="min-w-0 p-5"
      aria-labelledby="fulfillment-heading"
    >
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2
            id="fulfillment-heading"
            className="text-title font-semibold text-ink-950"
          >
            Fulfillment
          </h2>
          <p className="mt-1 max-w-3xl text-small leading-relaxed text-ink-500">
            What each quotation asked for, what saved purchase orders actually
            bought, and what is still unsold or closed. No date filter: bought
            state spans revisions and time, so a date range would answer half
            the question.
          </p>
        </div>
        <p className="tnum whitespace-nowrap text-caption text-ink-500">
          {page === null
            ? " "
            : `${page.total} ${
                page.total === 1 ? "quotation" : "quotations"
              } · Page ${bounds.page} of ${bounds.totalPages}`}
        </p>
      </header>

      <div className="mt-4 flex flex-wrap items-end gap-3">
        <label className="flex flex-col gap-1 text-caption font-medium text-ink-600">
          Status
          <select
            aria-label="Fulfillment status"
            className="min-h-9 rounded-md border border-rule-strong bg-white px-2 text-small text-ink-900"
            value={status}
            onChange={(event) => onStatusChange(event.target.value)}
          >
            {FULFILLMENT_STATUS_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            onSearchSubmit();
          }}
        >
          <label className="flex flex-col gap-1 text-caption font-medium text-ink-600">
            Search
            <input
              aria-label="Search quotations"
              className="min-h-9 w-56 rounded-md border border-rule-strong bg-white px-2 text-small text-ink-900"
              placeholder="Quote number or customer"
              type="search"
              value={searchDraft}
              onChange={(event) => onSearchDraftChange(event.target.value)}
            />
          </label>
          <Button aria-label="Search fulfillment" type="submit">
            Search
          </Button>
          <Button
            aria-label="Clear fulfillment filters"
            type="button"
            onClick={onClearFilters}
          >
            Clear
          </Button>
        </form>
        <div className="ml-auto flex items-center gap-2">
          <Button
            aria-label="Previous fulfillment page"
            disabled={!bounds.hasPrevious || loading}
            onClick={onPreviousPage}
          >
            Previous
          </Button>
          <Button
            aria-label="Next fulfillment page"
            disabled={!bounds.hasNext || loading}
            onClick={onNextPage}
          >
            Next
          </Button>
        </div>
      </div>

      {loading ? (
        <p role="status" className="mt-4 text-small text-ink-500">
          Loading fulfillment…
        </p>
      ) : error !== "" ? (
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
          <StatusBanner tone="danger">{error}</StatusBanner>
          <Button aria-label="Retry fulfillment" onClick={onRetry}>
            Retry
          </Button>
        </div>
      ) : page === null || page.quotations.length === 0 ? (
        <div className="mt-4 rounded-lg border border-dashed border-rule-strong px-4 py-6">
          <p className="text-small font-medium text-ink-700">
            No quotations match these filters.
          </p>
          <p className="mt-1 text-small text-ink-500">
            No date filter applies: every saved purchase-order buy for the
            selected currency is considered, across revisions.
          </p>
        </div>
      ) : (
        <div className="mt-4 space-y-3">
          {page.quotations.map((quotation) => (
            <QuotationBlock
              key={quotation.quotationId}
              quotation={quotation}
              entry={entry}
            />
          ))}
        </div>
      )}

      {page === null ? null : (
        <p className="mt-3 text-caption text-ink-500">
          As of {page.asOf}. Aggregate titles are presentation only: they add no
          row and no quantity of their own.
        </p>
      )}
    </Card>
  );
}

/**
 * The live card: one paginated read and nothing else. Every mutation or filter
 * change advances the read key, which hides the old page on the first render
 * after the change; the view then shows either the new page, the error or the
 * empty state.
 */
export function PurchaseOrderFulfillmentCard({
  currency,
  catalogue,
  active = true,
  mutationEpoch = 0,
  refreshToken = 0,
  load = defaultLoader,
}: PurchaseOrderFulfillmentCardProps) {
  const [status, setStatus] = useState("all");
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [pageNumber, setPageNumber] = useState(1);
  const [retry, setRetry] = useState(0);
  const [read, setRead] = useState<ReadState>({
    key: "",
    page: null,
    error: "",
    loading: true,
  });

  const query = useMemo<PurchaseOrderFulfillmentQuery>(
    () => ({
      currency,
      status,
      search,
      page: pageNumber,
      pageSize: FULFILLMENT_DEFAULT_PAGE_SIZE,
    }),
    [currency, pageNumber, search, status]
  );
  const queryKey = fulfillmentQueryKey(query);
  // A page belongs to one query AND one freshness generation. Including the
  // mutation epoch means a save or close hides the old figures on the first
  // render after the bump, not on the frame after the next effect.
  const readKey = `${queryKey}|${mutationEpoch}|${refreshToken}|${retry}`;

  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    setRead({ key: readKey, page: null, error: "", loading: true });
    void (async () => {
      try {
        const page = await load(query);
        if (!cancelled) {
          setRead({ key: readKey, page, error: "", loading: false });
        }
      } catch (caught) {
        if (!cancelled) {
          setRead({
            key: readKey,
            page: null,
            error: errorMessage(
              caught,
              "The fulfillment drill-down could not be read."
            ),
            loading: false,
          });
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [active, load, query, readKey]);

  const submitSearch = useCallback(() => {
    setSearch(searchDraft.trim());
    setPageNumber(1);
  }, [searchDraft]);

  const changeStatus = useCallback((next: string) => {
    setStatus(next);
    setPageNumber(1);
  }, []);

  const clearFilters = useCallback(() => {
    setStatus("all");
    setSearch("");
    setSearchDraft("");
    setPageNumber(1);
  }, []);

  const current = read.key === readKey;
  const loading = !current || read.loading;
  const readError = current ? read.error : "";
  const page = current ? read.page : null;
  const bounds = fulfillmentPageBounds(
    page?.total ?? 0,
    page?.pageSize ?? FULFILLMENT_DEFAULT_PAGE_SIZE,
    pageNumber
  );

  return (
    <PurchaseOrderFulfillmentCardView
      page={page}
      currency={currency}
      catalogue={catalogue}
      status={status}
      searchDraft={searchDraft}
      pageNumber={pageNumber}
      loading={loading}
      error={readError}
      onStatusChange={changeStatus}
      onSearchDraftChange={setSearchDraft}
      onSearchSubmit={submitSearch}
      onClearFilters={clearFilters}
      onPreviousPage={() => setPageNumber(Math.max(1, bounds.page - 1))}
      onNextPage={() => setPageNumber(bounds.page + 1)}
      onRetry={() => setRetry((current) => current + 1)}
    />
  );
}
