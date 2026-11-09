import { useCallback, useEffect, useRef, useState } from "react";
import { ListQuotations } from "@backend/backend/Quotation";
import { GetCurrencyCatalogue } from "@backend/backend/AdminAPI";
import type { ArchivedQuotationSummary } from "../archive";
import { resolveCurrencyEntry } from "../archive";
import type { CurrencyInfo } from "../format";
import { decorateMoney } from "../format";
import { loadBindings, matchShortcut } from "../shortcuts";
import { useToast } from "../toast";
import { Badge, Button, Card } from "./ui";
import { DeletionDialog } from "./DeletionDialog";

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

interface ArchivePageProps {
  onNewQuotation: () => void;
  onOpenQuotation: (id: number, revision: number) => void;
  /** The live profile entry: fallback while the catalogue loads and for
   * rows whose code it cannot resolve. Each row renders in its own
   * REVISION currency (row.Currency), never this one, once resolved. */
  currency?: CurrencyInfo;
  /** Whether the signed-in user may permanently delete a quotation. */
  isAdmin?: boolean;
}

export function ArchivePage({
  onNewQuotation,
  onOpenQuotation,
  currency = IDR_ENTRY,
  isAdmin = false,
}: ArchivePageProps) {
  const [rows, setRows] = useState<ArchivedQuotationSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  // #480: the delete action opens the shared permanent-deletion dialog, which
  // asks for the scope, the typed number and (for a chain) the ten-letter
  // challenge. The old two-click inline delete is gone: it could remove a
  // quotation without a reason, a typed confirmation or a surviving-PO plan.
  const [deletionTarget, setDeletionTarget] = useState<{ id: number; label: string } | null>(null);
  // The catalogue, fetched like App.tsx fetches it: row codes resolve to
  // entries here, so each row renders in its revision currency. Null until
  // loaded (or when the binding fails) -- rows then fall back to the live
  // entry rather than rendering nothing.
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
  const listRef = useRef<HTMLDivElement>(null);
  const [activeIndex, setActiveIndex] = useState(-1);
  const { pushToast } = useToast();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = (await ListQuotations()) as ArchivedQuotationSummary[];
      setRows(result);
    } catch {
      const message =
        "We couldn't load the quotation archive. Check the database connection and try again.";
      setError(message);
      pushToast("error", message);
    } finally {
      setLoading(false);
    }
  }, [pushToast]);

  useEffect(() => {
    void load();
  }, [load]);

  // List navigation: arrows move a highlight, Enter opens. Focused buttons
  // keep native behavior (Enter clicks), so the handler yields whenever
  // the event starts inside a control.
  const focusRowButton = (index: number) => {
    const button = listRef.current?.querySelector(
      `[data-archive-row="${index}"] button`
    );
    if (button instanceof HTMLElement) button.focus();
  };
  const listKeyDown = (event: React.KeyboardEvent) => {
    if (rows.length === 0) return;
    const inControl = !!(event.target as HTMLElement).closest?.(
      "button,input,select,a"
    );
    const action = matchShortcut(event.nativeEvent, loadBindings(), "archive");
    if (!action) return;
    if (action === "archiveUp" || action === "archiveDown") {
      event.preventDefault();
      const next =
        action === "archiveUp"
          ? Math.max(0, (activeIndex < 0 ? rows.length : activeIndex) - 1)
          : Math.min(rows.length - 1, activeIndex + 1);
      setActiveIndex(next);
      focusRowButton(next);
      return;
    }
    // Enter on a focused control keeps its native click; elsewhere it
    // opens the highlighted row.
    if (action === "archiveOpen" && !inControl && activeIndex >= 0 && activeIndex < rows.length) {
      const row = rows[activeIndex];
      onOpenQuotation(row.ID, row.RevisionNumber);
    }
  };

  return (
    <section aria-labelledby="archive-heading" className="flex flex-col gap-5">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1
            id="archive-heading"
            className="text-title font-semibold text-ink-950"
          >
            Archive
          </h1>
          <p className="mt-1 max-w-2xl text-body text-ink-500">
            One row per quotation showing its latest revision, newest first.
            Open one to inspect its internal economics, review the customer
            copy, step back through earlier revisions, or create the next one.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Button onClick={() => void load()} disabled={loading}>
            Refresh
          </Button>
          <Button variant="primary" onClick={onNewQuotation}>
            New quotation
          </Button>
        </div>
      </header>

      <Card className="overflow-hidden">
        {loading ? (
          <div className="space-y-3 p-6" aria-label="Loading quotations">
            {[0, 1, 2, 3].map((item) => (
              <div
                key={item}
                className="h-12 animate-pulse rounded-lg bg-paper-muted"
              />
            ))}
          </div>
        ) : error ? (
          <div className="flex flex-col items-center gap-3 px-6 py-14 text-center">
            <p className="max-w-md text-body text-danger-700">{error}</p>
            <Button onClick={() => void load()}>Try again</Button>
          </div>
        ) : rows.length === 0 ? (
          <div className="flex flex-col items-center gap-3 px-6 py-16 text-center">
            <span className="flex h-11 w-11 items-center justify-center rounded-lg border border-rule bg-paper-subtle text-ink-400">
              <svg width="18" height="18" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4">
                <path d="M2 3.333h12v9.334H2zM2 6.667h12M5.333 9.333h2" />
              </svg>
            </span>
            <p className="text-lead font-medium text-ink-900">
              No quotations saved yet
            </p>
            <p className="max-w-md text-body text-ink-500">
              Prepare and export a quotation to start its revision history.
            </p>
            <Button variant="primary" onClick={onNewQuotation} className="mt-1">
              Create quotation
            </Button>
          </div>
        ) : (
          <div ref={listRef} onKeyDown={listKeyDown} className="overflow-x-auto scroll-thin">
            <table className="w-full min-w-[860px] text-left text-body">
              <thead>
                <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                  <th className="px-5 py-3 font-medium">Quotation</th>
                  <th className="px-4 py-3 font-medium">Customer</th>
                  <th className="px-4 py-3 font-medium">Subject</th>
                  <th className="px-4 py-3 text-center font-medium">
                    Latest rev
                  </th>
                  <th className="px-4 py-3 text-right font-medium">
                    Grand total
                  </th>
                  <th className="px-5 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-rule">
                {rows.map((row, index) => (
                  <tr
                    key={row.ID}
                    data-archive-row={index}
                    onClick={() => onOpenQuotation(row.ID, row.RevisionNumber)}
                    className={`group cursor-pointer transition-colors duration-150 hover:bg-paper-subtle/60 ${
                      activeIndex === index ? "bg-paper-subtle" : ""
                    }`}
                  >
                    <td className="px-5 py-3 align-middle">
                      <button
                        type="button"
                        onClick={(event) => {
                          event.stopPropagation();
                          onOpenQuotation(row.ID, row.RevisionNumber);
                        }}
                        className="rounded text-body font-medium text-ink-900 underline-offset-4 transition-colors duration-150 hover:text-accent-700 hover:underline focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                      >
                        <span className="tnum">{row.QuoteNumber}</span>
                      </button>
                      <p className="tnum mt-0.5 text-caption text-ink-400">
                        {row.CreatedAt.slice(0, 10)}
                      </p>
                    </td>
                    <td className="px-4 py-3 align-middle text-body text-ink-700">
                      {row.CustomerName}
                    </td>
                    <td className="max-w-[320px] px-4 py-3 align-middle text-body text-ink-600">
                      <span className="[display:-webkit-box] overflow-hidden [-webkit-box-orient:vertical] [-webkit-line-clamp:2]">
                        {row.Subject}
                      </span>
                    </td>
                    <td className="px-4 py-3 align-middle text-center">
                      <Badge tone="neutral">
                        <span className="tnum">
                          {String(row.RevisionNumber).padStart(2, "0")}
                        </span>
                      </Badge>
                    </td>
                    <td className="px-4 py-3 text-right align-middle text-body font-medium tnum text-ink-950">
                      {decorateMoney(resolveCurrencyEntry(catalogue, row.Currency, currency), row.DisplayGrand)}
                    </td>
                    <td className="px-5 py-3 text-right align-middle">
                      {isAdmin ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`Delete quotation ${row.QuoteNumber}`}
                          onClick={(event) => {
                            event.stopPropagation();
                            setDeletionTarget({
                              id: row.ID,
                              label: `quotation ${row.QuoteNumber}`,
                            });
                          }}
                          className="hover:!bg-danger-50 hover:!text-danger-600"
                        >
                          Delete
                        </Button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {rows.length > 0 ? (
        <p className="text-caption text-ink-500">
          Deleting a record is permanent. A single quotation deletion removes
          every revision it contains and leaves a connected purchase order as
          Source missing; whole-chain deletion removes both.
        </p>
      ) : null}

      {deletionTarget !== null ? (
        <DeletionDialog
          targetType="quotation"
          targetId={deletionTarget.id}
          targetLabel={deletionTarget.label}
          onClose={() => setDeletionTarget(null)}
          onDeleted={() => {
            setRows((current) =>
              current.filter((row) => row.ID !== deletionTarget.id)
            );
            setDeletionTarget(null);
            pushToast("success", "Quotation deleted.");
          }}
        />
      ) : null}
    </section>
  );
}
