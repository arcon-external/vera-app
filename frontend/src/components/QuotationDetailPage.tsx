import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  GetPOSourceOccupancy,
  GetQuotationRevision,
} from "@backend/backend/Quotation";
import { GetCurrencyCatalogue } from "@backend/backend/AdminAPI";
import type { ArchivedQuotation } from "../archive";
import { resolveCurrencyEntry } from "../archive";
import {
  decorateMoney,
  formatMoney,
  type CurrencyInfo,
} from "../format";

// The IDR entry, transcribed field-for-field from
// backend/currency/currency.go. This is the prop default only -- the live
// entry always arrives from the caller -- not a catalogue mirror.
const IDR_ENTRY: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

// signedMoney renders a profit figure that can be negative. A discount below
// cost is a loss, and printing it as a positive number would be a lie the
// reader has to notice for themselves. The sign stays outside the decorated
// figure (today's "−Rp" order), so IDR renders byte-identically.
function signedMoney(currency: CurrencyInfo, value: number): string {
  const text = formatMoney(currency, Math.abs(value));
  return `${value < 0 ? "−" : "+"}${text}`;
}

function taxDetailLabel(quotation: ArchivedQuotation): string {
  if (quotation.ServiceCategory === "Construction") {
    return "Construction 1,75% PPh22";
  }
  if (quotation.ServiceCategory === "NonConstruction") {
    return "Non-construction 2% PPh22";
  }
  return quotation.TaxCategory || quotation.TaxZone;
}
import { useToast } from "../toast";
import { copyQuotationToClipboard } from "../quotationClipboard";
import { Select } from "./Select";
import { Badge, Button, Card, SectionHeading } from "./ui";

function padRevision(revision: number): string {
  return String(revision).padStart(2, "0");
}

/**
 * Copies the viewed revision's entry values as versioned JSON text. Exported
 * for the SSR pin: the button renders with its label and disabled state, and
 * the write itself is unit-tested through copyQuotationToClipboard.
 */
export function QuotationCopyButton({
  quotation,
  disabled,
  onCopy,
}: {
  quotation: ArchivedQuotation;
  disabled: boolean;
  onCopy: (quotation: ArchivedQuotation) => void;
}) {
  return (
    <Button variant="secondary" disabled={disabled} onClick={() => onCopy(quotation)}>
      Copy to clipboard
    </Button>
  );
}

/**
 * The one Purchase Order occupying the displayed Quotation (#499). A draft
 * opens its editor; a finalized record opens the saved view. It carries no
 * revision: the one-PO rule spans every revision of the Quotation.
 */
export interface QuotationDetailPurchaseOrder {
  id: number;
  state: "draft" | "saved";
}

// A refused create hands the server's message through (the bindings marshal a
// typed error as {message}); this reads it without letting an object render as
// "[object Object]". Same shape as App's local helper, kept here because that
// one is not exported and a component does not import the app shell.
function purchaseOrderErrorMessage(error: unknown, fallback: string): string {
  if (typeof error === "string" && error.trim()) return error;
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message.trim()) return message;
  }
  return fallback;
}

interface QuotationDetailPageProps {
  quotationID: number;
  revision: number;
  onBack: () => void;
  onEdit: (quotation: ArchivedQuotation) => void;
  /**
   * Free Quotation: create its one PO from the displayed revision, through the
   * same source-bound create path the selector uses. It resolves once the PO
   * workspace has adopted the record, and rejects when the server refuses --
   * a concurrent occupation included.
   */
  onCreatePurchaseOrder: (quotationID: number, revision: number) => Promise<void>;
  /** Occupied Quotation: open the existing record (draft editor or saved view). */
  onOpenPurchaseOrder: (purchaseOrderID: number) => void;
  /** The live profile entry: the initial render's currency only. Once the
   * revision loads, every figure renders in its REVISION currency
   * (quotation.Currency) resolved below; the live entry is then just the
   * fallback for an unresolvable code. */
  currency?: CurrencyInfo;
}

export interface QuotationDetailHeaderProps {
  quotation: ArchivedQuotation;
  activeRevision: number;
  revisionOptions: { value: number; label: string }[];
  onRevisionChange: (revision: number) => void;
  onBack: () => void;
  onEdit: () => void;
  /** The one PO occupying this Quotation, or null when it is free (#499). */
  purchaseOrder: QuotationDetailPurchaseOrder | null;
  /** True until the occupancy read answers; creation waits for it. */
  purchaseOrderPending: boolean;
  /** Free: create from the revision on screen. */
  onCreatePurchaseOrder: () => void;
  /** Occupied: open the existing record by id. */
  onOpenPurchaseOrder: (purchaseOrderID: number) => void;
  /**
   * True while the displayed revision is being replaced. Both the copy and the
   * create action are disabled: each must act on the revision on screen, and
   * the previous revision's figures are still mounted during the fetch.
   */
  revisionSwitching?: boolean;
  /** Copy is optional so the header's PO-only pins keep rendering without it. */
  onCopy?: (quotation: ArchivedQuotation) => void;
}

// The issued-Quotation header, exported so its two entry actions can be
// rendered without a DOM: the page itself fetches in an effect and prints a
// loading skeleton to SSR.
export function QuotationDetailHeader({
  quotation,
  activeRevision,
  revisionOptions,
  onRevisionChange,
  onBack,
  onEdit,
  purchaseOrder,
  purchaseOrderPending,
  onCreatePurchaseOrder,
  onOpenPurchaseOrder,
  revisionSwitching = false,
  onCopy,
}: QuotationDetailHeaderProps) {
  return (
    <header>
      <button
        type="button"
        onClick={onBack}
        className="inline-flex min-h-8 items-center gap-1.5 rounded-md px-2 py-1.5 text-small text-ink-500 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
      >
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M10 3.5L5.5 8 10 12.5" />
        </svg>
        Archive
      </button>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <h1
            id="quotation-detail-heading"
            className="tnum text-display font-semibold text-ink-950"
          >
            {quotation.QuoteNumber}
          </h1>
          {revisionOptions.length > 1 ? (
            <div className="w-[220px]">
              <Select
                value={activeRevision}
                options={revisionOptions}
                ariaLabel="Quotation revision"
                onChange={(next) => onRevisionChange(Number(next))}
              />
            </div>
          ) : (
            <Badge tone="accent">
              <span className="tnum">
                Revision {padRevision(quotation.RevisionNumber)}
              </span>
            </Badge>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {/* #499: exactly one action. A free Quotation creates from the
              displayed revision; an occupied one opens its existing record and
              never offers a second creation attempt. */}
          {purchaseOrder === null ? (
            <Button
              variant="secondary"
              aria-label="Create PO from this Quotation"
              disabled={purchaseOrderPending || revisionSwitching}
              onClick={onCreatePurchaseOrder}
            >
              Create PO from this Quotation
            </Button>
          ) : (
            <Button
              variant="secondary"
              aria-label={purchaseOrder.state === "saved" ? "View PO" : "Open PO draft"}
              onClick={() => onOpenPurchaseOrder(purchaseOrder.id)}
            >
              {purchaseOrder.state === "saved" ? "View PO" : "Open PO draft"}
            </Button>
          )}
          {onCopy ? (
            <QuotationCopyButton
              quotation={quotation}
              disabled={revisionSwitching}
              onCopy={onCopy}
            />
          ) : null}
          <Button variant="primary" onClick={onEdit}>
            Edit revision {padRevision(quotation.RevisionNumber)}
          </Button>
        </div>
      </div>
      <p className="mt-2 max-w-3xl text-body text-ink-500">
        {quotation.CustomerName} — {quotation.Subject}
      </p>
    </header>
  );
}

export function QuotationDetailPage({
  quotationID,
  revision,
  onBack,
  onEdit,
  onCreatePurchaseOrder,
  onOpenPurchaseOrder,
  currency = IDR_ENTRY,
}: QuotationDetailPageProps) {
  const [quotation, setQuotation] = useState<ArchivedQuotation | null>(null);
  const [loading, setLoading] = useState(true);
  const [switching, setSwitching] = useState(false);
  const [activeRevision, setActiveRevision] = useState(revision);
  // R4: the revision the mounted record actually belongs to. A failed fetch
  // must not leave the selector naming a revision this page never displayed,
  // so the catch puts the selector back on this one.
  const loadedRevision = useRef<number | null>(null);
  // #499: the occupancy answer and the create attempt. The answer chooses the
  // label; the attempt is in flight only long enough to hand the record to the
  // PO workspace.
  const [purchaseOrder, setPurchaseOrder] =
    useState<QuotationDetailPurchaseOrder | null>(null);
  const [purchaseOrderPending, setPurchaseOrderPending] = useState(true);
  const [creatingPurchaseOrder, setCreatingPurchaseOrder] = useState(false);
  const { pushToast } = useToast();
  // The catalogue, fetched like App.tsx fetches it: the revision code
  // resolves to its entry once loaded. Null until then -- figures fall back
  // to the live entry rather than rendering nothing.
  const [catalogue, setCatalogue] = useState<CurrencyInfo[] | null>(null);
  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const entries = (await GetCurrencyCatalogue()) as CurrencyInfo[];
        if (active) setCatalogue(entries);
      } catch {
        if (active) setCatalogue(null);
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  // The narrow occupancy read (#499). Keyed to the Quotation, not the
  // displayed revision: the PO follows every revision, so switching the
  // selector must not make an occupied Quotation look free.
  const readPurchaseOrder = useCallback(
    async (id: number): Promise<QuotationDetailPurchaseOrder | null> => {
      const occupancy = await GetPOSourceOccupancy(id);
      const active = occupancy.ActivePO;
      if (!active) return null;
      return {
        id: active.ID,
        // Only "saved" is finalized; anything else is the editable draft the
        // server labels. The state is read, never assumed.
        state: active.State === "saved" ? "saved" : "draft",
      };
    },
    []
  );

  useEffect(() => {
    let active = true;
    setPurchaseOrder(null);
    setPurchaseOrderPending(true);
    void readPurchaseOrder(quotationID)
      .then((found) => {
        if (active) setPurchaseOrder(found);
      })
      .catch(() => {
        // A failed read cannot decide the action. Creation stays offered
        // because the server's create transaction is the real occupancy guard;
        // a concurrent occupation answers 409 and is handled below.
        if (active) setPurchaseOrder(null);
      })
      .finally(() => {
        if (active) setPurchaseOrderPending(false);
      });
    return () => {
      active = false;
    };
  }, [quotationID, readPurchaseOrder]);

  // Reset to the revision requested by the archive when the target changes.
  // Clearing the loaded record makes a different quotation show the skeleton
  // rather than the previous quotation's data.
  useEffect(() => {
    setActiveRevision(revision);
    setQuotation(null);
    loadedRevision.current = null;
  }, [revision, quotationID]);

  useEffect(() => {
    let active = true;
    if (quotation === null) {
      setLoading(true);
    } else {
      setSwitching(true);
    }
    void GetQuotationRevision(quotationID, activeRevision)
      .then((result) => {
        if (active) {
          loadedRevision.current = result.RevisionNumber;
          setQuotation(result as ArchivedQuotation);
        }
      })
      .catch(() => {
        pushToast(
          "error",
          "We couldn't load this quotation. Return to the archive and try again."
        );
        // R4: the failed revision never reached the page. Put the selector
        // back on the record that is still mounted so the action, the content
        // and the selector cannot disagree; a first-load failure has no
        // record to return to and keeps the not-found card.
        const fallback = loadedRevision.current;
        if (active && fallback !== null && fallback !== activeRevision) {
          setActiveRevision(fallback);
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false);
          setSwitching(false);
        }
      });
    return () => {
      active = false;
    };
    // `quotation` is intentionally excluded: it only distinguishes the first
    // load from a revision switch and would otherwise re-trigger the fetch.
    // The loaded record's revision is read through loadedRevision instead.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [quotationID, activeRevision, pushToast]);

  const revisionOptions = useMemo(() => {
    const revisions = quotation?.AvailableRevisions ?? [activeRevision];
    return revisions.map((value, index) => ({
      value,
      label:
        index === 0
          ? `Revision ${padRevision(value)} — latest`
          : `Revision ${padRevision(value)}`,
    }));
  }, [quotation?.AvailableRevisions, activeRevision]);

  // The REVISION currency once loaded; the live entry before that and for
  // any code the catalogue cannot resolve.
  const entry = resolveCurrencyEntry(catalogue, quotation?.Currency, currency);

  // R4: creation is only offered for the record on screen. The selector and
  // the mounted record can differ while a switch is loading (and, without the
  // rollback above, after it fails), so the action is gated on both naming the
  // same revision -- never on activeRevision alone.
  const revisionAligned =
    quotation !== null && quotation.RevisionNumber === activeRevision;

  // #499: create from the revision on screen. App owns the source-bound create
  // call and the PO workspace; this page only supplies the exact displayed
  // pair (the header is disabled while a switch is in flight, so the pair it
  // reads is the revision the user sees). A refusal is resolved by re-reading
  // occupancy: a concurrent create answers 409, and that must switch the action
  // to Open PO draft / View PO rather than retry.
  const handleCreatePurchaseOrder = useCallback(async () => {
    if (quotation === null || quotation.RevisionNumber !== activeRevision) return;
    if (creatingPurchaseOrder) return;
    setCreatingPurchaseOrder(true);
    try {
      await onCreatePurchaseOrder(quotation.ID, quotation.RevisionNumber);
    } catch (error) {
      let found: QuotationDetailPurchaseOrder | null = null;
      try {
        found = await readPurchaseOrder(quotation.ID);
      } catch {
        found = null;
      }
      if (found !== null) {
        setPurchaseOrder(found);
        pushToast(
          "error",
          found.state === "saved"
            ? "This quotation already has a finalized purchase order. Use View PO to open it."
            : "This quotation already has a purchase order draft. Use Open PO draft to continue it."
        );
      } else {
        pushToast(
          "error",
          purchaseOrderErrorMessage(
            error,
            "The purchase order could not be created."
          )
        );
      }
    } finally {
      setCreatingPurchaseOrder(false);
    }
  }, [
    quotation,
    activeRevision,
    creatingPurchaseOrder,
    onCreatePurchaseOrder,
    readPurchaseOrder,
    pushToast,
  ]);

  if (loading) {
    return (
      <div className="space-y-4" aria-label="Loading quotation">
        <div className="h-8 w-64 animate-pulse rounded-md bg-paper-muted" />
        <div className="h-72 animate-pulse rounded-xl bg-paper-muted" />
      </div>
    );
  }

  if (!quotation) {
    return (
      <Card className="flex flex-col items-center gap-3 px-6 py-16 text-center">
        <p className="text-body text-danger-700">Quotation not found.</p>
        <Button onClick={onBack}>Return to archive</Button>
      </Card>
    );
  }

  return (
    <section
      aria-labelledby="quotation-detail-heading"
      aria-busy={switching}
      className={`space-y-6 transition-opacity duration-200 ease-out-quart ${
        switching ? "opacity-60" : "opacity-100"
      }`}
    >
      <QuotationDetailHeader
        quotation={quotation}
        activeRevision={activeRevision}
        revisionOptions={revisionOptions}
        onRevisionChange={(next) => setActiveRevision(Number(next))}
        onBack={onBack}
        onEdit={() => onEdit(quotation)}
        purchaseOrder={purchaseOrder}
        purchaseOrderPending={purchaseOrderPending}
        revisionSwitching={switching || !revisionAligned || creatingPurchaseOrder}
        onCreatePurchaseOrder={() => void handleCreatePurchaseOrder()}
        onOpenPurchaseOrder={onOpenPurchaseOrder}
        onCopy={(current) =>
          void copyQuotationToClipboard(current, navigator.clipboard, pushToast)
        }
      />

      <Card className="border-internal-200 bg-internal-50/40 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <SectionHeading title="Internal record" />
            <h2 className="text-title font-semibold text-ink-950">
              Line economics and totals
            </h2>
            <p className="mt-1 max-w-2xl text-small text-ink-500">
              Retained for internal review. None of these values appear in the
              customer quotation.
            </p>
          </div>
          <Badge tone="internal">Internal only</Badge>
        </div>

        <div className="mt-5 overflow-x-auto scroll-thin rounded-lg border border-internal-200 bg-white">
          <table className="w-full min-w-[860px] table-fixed text-left text-body">
            <colgroup>
              <col className="w-12" />
              <col />
              <col className="w-24" />
              <col className="w-56" />
              <col className="w-48" />
            </colgroup>
            <thead>
              <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                <th className="px-4 py-2.5 font-medium">No</th>
                <th className="px-4 py-2.5 font-medium">Description</th>
                <th className="px-4 py-2.5 text-center font-medium">Qty</th>
                <th className="px-4 py-2.5 text-right font-medium">
                  Unit after margin
                </th>
                <th className="px-4 py-2.5 text-right font-medium">
                  Total after margin
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-rule">
              {quotation.Items.map((item) => (
                <tr key={item.ItemNo} className="align-top">
                  <td className="tnum px-4 py-2.5 text-ink-400">
                    {item.ItemNo}
                  </td>
                  <td className="whitespace-pre-wrap break-words px-4 py-2.5 text-body leading-relaxed text-ink-700">
                    {item.Description}
                  </td>
                  <td className="px-4 py-2.5 text-center text-body tnum text-ink-700">
                    {item.Qty} {item.UoM}
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <div className="tnum text-body font-medium text-ink-900">
                      {decorateMoney(entry, item.DisplayMarginalizedUnit)}
                    </div>
                    {item.DiscountEnabled ? (
                      <div className="tnum mt-0.5 text-caption text-accent-700">
                        −{item.DiscountPercent}% off · was{" "}
                        {formatMoney(entry, item.ListUnitPrice)}
                      </div>
                    ) : null}
                    <div className="tnum mt-0.5 text-caption text-internal-700">
                      {item.MarginEnabled
                        ? `+${item.MarginPercent}% · ${signedMoney(entry, item.MarginAmount)}`
                        : "No margin"}
                    </div>
                  </td>
                  <td className="tnum px-4 py-2.5 text-right text-body text-ink-900">
                    {decorateMoney(entry, item.DisplayMarginalizedTotal)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="mt-5 grid items-start gap-4 lg:grid-cols-2">
          <div className="rounded-lg border border-rule bg-white p-4">
            <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
              Original baseline
            </p>
            <dl className="mt-2.5 divide-y divide-rule">
              {/* No cost-side discount: the cost subtotal is the grand total,
                  so showing both would print the same figure twice. */}
              <DetailRow
                label="Cost total (undiscounted)"
                value={quotation.DisplayOriginalGrand}
                currency={entry}
              />
            </dl>
          </div>
          <div className="rounded-lg border border-rule bg-white p-4">
            <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
              Marginalized result
            </p>
            <dl className="mt-2.5 divide-y divide-rule">
              <DetailRow
                label="Customer subtotal"
                value={quotation.DisplayMarginalizedSubtotal}
                currency={entry}
              />
              {quotation.MarginalizedDiscount > 0 ? (
                <DetailRow
                  label={`Discount ${quotation.DiscountPercent}% (on the price)`}
                  value={`−${quotation.DisplayMarginalizedDiscount}`}
                  currency={entry}
                />
              ) : null}
              <DetailRow
                label="Grand total before tax"
                value={quotation.DisplayMarginalizedGrand}
                currency={entry}
              />
              {quotation.PpnAmount > 0 ? (
                <DetailRow
                  label="PPN 11% (on the quotation)"
                  value={`+${quotation.DisplayPpn}`}
                  currency={entry}
                />
              ) : null}
              {quotation.TaxAmount > 0 ? (
                <DetailRow
                  label={`Services tax, internal — ${taxDetailLabel(quotation)}`}
                  value={quotation.DisplayTax}
                  currency={entry}
                />
              ) : null}
              <DetailRow
                label="Grand total after tax"
                value={quotation.DisplayMarginalizedAfterTax}
                currency={entry}
              />
              <DetailRow label="Nett profit" value={quotation.DisplayNett} currency={entry} strong />
            </dl>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden">
        <header className="border-b border-rule px-5 py-4">
          <h2 className="text-title font-semibold text-ink-950">
            Customer copy
          </h2>
          <p className="mt-1 text-small text-ink-500">
            The rendered quotation as the customer receives it. Margin, tax,
            and nett profit are excluded.
          </p>
        </header>
        <div className="bg-paper-muted p-3 lg:p-5">
          <iframe
            title="Rendered quotation"
            srcDoc={quotation.HTMLSnapshot}
            sandbox=""
            className="h-[min(820px,72vh)] min-h-[420px] w-full border-0 bg-white shadow-card"
          />
        </div>
      </Card>
    </section>
  );
}

function DetailRow({
  label,
  value,
  currency,
  strong = false,
}: {
  label: string;
  value: string;
  currency: CurrencyInfo;
  strong?: boolean;
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-2">
      <dt className="text-small text-ink-500">{label}</dt>
      <dd
        className={`tnum text-small ${
          strong ? "font-semibold text-ink-950" : "text-ink-900"
        }`}
      >
        {decorateMoney(currency, value)}
      </dd>
    </div>
  );
}
