import { useCallback, useEffect, useRef, useState } from "react";
import { DeleteQuotation, ListQuotations } from "@backend/backend/Quotation";
import type { ArchivedQuotationSummary } from "../archive";
import { useToast } from "../toast";
import { Badge, Button, Card } from "./ui";

interface ArchivePageProps {
  onNewQuotation: () => void;
  onOpenQuotation: (id: number, revision: number) => void;
}

export function ArchivePage({ onNewQuotation, onOpenQuotation }: ArchivePageProps) {
  const [rows, setRows] = useState<ArchivedQuotationSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [confirmingId, setConfirmingId] = useState<number | null>(null);
  const confirmTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
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

  useEffect(
    () => () => {
      if (confirmTimer.current) clearTimeout(confirmTimer.current);
    },
    []
  );

  const requestDelete = (id: number) => {
    setConfirmingId(id);
    if (confirmTimer.current) clearTimeout(confirmTimer.current);
    confirmTimer.current = setTimeout(() => setConfirmingId(null), 6000);
  };

  const handleDelete = async (id: number) => {
    if (confirmTimer.current) clearTimeout(confirmTimer.current);
    try {
      await DeleteQuotation(id);
      setRows((current) => current.filter((row) => row.ID !== id));
      setConfirmingId(null);
      pushToast("success", "Quotation deleted.");
    } catch {
      pushToast("error", "We couldn't delete this quotation. Try again.");
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
          <div className="overflow-x-auto scroll-thin">
            <table className="w-full min-w-[860px] text-left">
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
                {rows.map((row) => (
                  <tr
                    key={row.ID}
                    onClick={() => onOpenQuotation(row.ID, row.RevisionNumber)}
                    className="group cursor-pointer transition-colors duration-150 hover:bg-paper-subtle/60"
                  >
                    <td className="px-5 py-3 align-top">
                      <button
                        type="button"
                        onClick={(event) => {
                          event.stopPropagation();
                          onOpenQuotation(row.ID, row.RevisionNumber);
                        }}
                        className="rounded text-body font-medium text-ink-900 underline-offset-4 transition-colors duration-150 hover:text-accent-700 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40"
                      >
                        <span className="tnum">{row.QuoteNumber}</span>
                      </button>
                      <p className="tnum mt-0.5 text-caption text-ink-400">
                        {row.CreatedAt.slice(0, 10)}
                      </p>
                    </td>
                    <td className="px-4 py-3 align-top text-body text-ink-700">
                      {row.CustomerName}
                    </td>
                    <td className="max-w-[320px] px-4 py-3 align-top text-body text-ink-600">
                      <span className="line-clamp-2">{row.Subject}</span>
                    </td>
                    <td className="px-4 py-3 align-top text-center">
                      <Badge tone="neutral">
                        <span className="tnum">
                          {String(row.RevisionNumber).padStart(2, "0")}
                        </span>
                      </Badge>
                    </td>
                    <td className="px-4 py-3 text-right align-top text-body font-medium tnum text-ink-950">
                      Rp{row.DisplayGrand}
                    </td>
                    <td className="px-5 py-3 text-right align-top">
                      {confirmingId === row.ID ? (
                        <span className="inline-flex items-center gap-1.5">
                          <Button
                            variant="danger"
                            size="sm"
                            onClick={(event) => {
                              event.stopPropagation();
                              void handleDelete(row.ID);
                            }}
                          >
                            Confirm
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={(event) => {
                              event.stopPropagation();
                              setConfirmingId(null);
                            }}
                          >
                            Cancel
                          </Button>
                        </span>
                      ) : (
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`Delete quotation ${row.QuoteNumber}`}
                          onClick={(event) => {
                            event.stopPropagation();
                            requestDelete(row.ID);
                          }}
                          className="hover:!bg-danger-50 hover:!text-danger-600"
                        >
                          Delete
                        </Button>
                      )}
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
          Deleting a quotation also removes every revision it contains. This
          cannot be undone.
        </p>
      ) : null}
    </section>
  );
}
