import { useCallback, useEffect, useState } from "react";
import { DeleteQuotation, ListQuotations } from "@backend/backend/Quotation";
import type { ArchivedQuotationSummary } from "../archive";
import { useToast } from "../toast";

interface ArchivePageProps {
  onNewQuotation: () => void;
  onOpenQuotation: (id: number, revision: number) => void;
}

export function ArchivePage({ onNewQuotation, onOpenQuotation }: ArchivePageProps) {
  const [rows, setRows] = useState<ArchivedQuotationSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = (await ListQuotations()) as ArchivedQuotationSummary[];
      setRows(result);
    } catch {
      const message = "We couldn't load the quotation archive. Check the database connection and try again.";
      setError(message);
      pushToast("error", message);
    } finally {
      setLoading(false);
    }
  }, [pushToast]);

  useEffect(() => {
    void load();
  }, [load]);

  const handleDelete = async (id: number) => {
    if (!window.confirm("Delete this quotation and all of its revisions? This cannot be undone.")) {
      return;
    }
    try {
      await DeleteQuotation(id);
      setRows((current) => current.filter((row) => row.ID !== id));
      pushToast("success", "Quotation deleted.");
    } catch {
      pushToast("error", "We couldn't delete this quotation. Try again.");
    }
  };

  return (
    <section className="space-y-8" aria-labelledby="archive-heading">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-slate-400">
            Archive
          </p>
          <h2 id="archive-heading" className="mt-2 text-[26px] font-semibold tracking-[-0.03em] text-slate-900">
            Quotation history
          </h2>
          <p className="mt-2 max-w-2xl text-[14px] leading-relaxed text-slate-500">
            Open a quotation to inspect its internal calculations, review its customer copy, or create the next revision.
          </p>
        </div>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => void load()}
            className="min-h-11 rounded-xl border border-slate-200 bg-white px-4 text-[13px] font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900 active:scale-[0.98] focus:outline-none focus:ring-2 focus:ring-emerald-500/20"
          >
            Refresh
          </button>
          <button
            type="button"
            onClick={onNewQuotation}
            className="min-h-11 rounded-xl bg-emerald-600 px-4 text-[13px] font-semibold text-white transition hover:bg-emerald-500 active:scale-[0.98] focus:outline-none focus:ring-2 focus:ring-emerald-500/30"
          >
            New quotation
          </button>
        </div>
      </header>

      <div className="rounded-2xl border border-slate-200/80 bg-white shadow-[0_1px_3px_rgba(0,0,0,0.03)]">
        {loading && (
          <div className="space-y-3 p-6" aria-label="Loading quotations">
            {[0, 1, 2].map((item) => (
              <div key={item} className="h-14 animate-pulse rounded-xl bg-slate-100" />
            ))}
          </div>
        )}

        {!loading && error && (
          <div className="p-8 text-center">
            <p className="text-[14px] font-medium text-red-700">{error}</p>
            <button
              type="button"
              onClick={() => void load()}
              className="mt-4 min-h-11 rounded-xl border border-red-200 px-4 text-[13px] font-medium text-red-700 transition hover:bg-red-50 active:scale-[0.98]"
            >
              Try again
            </button>
          </div>
        )}

        {!loading && !error && rows.length === 0 && (
          <div className="p-12 text-center">
            <p className="text-[15px] font-medium text-slate-800">No quotations saved yet.</p>
            <p className="mx-auto mt-2 max-w-md text-[13px] leading-relaxed text-slate-500">
              Prepare and export your first quotation to start a revision history.
            </p>
            <button
              type="button"
              onClick={onNewQuotation}
              className="mt-5 min-h-11 rounded-xl bg-emerald-600 px-4 text-[13px] font-semibold text-white transition hover:bg-emerald-500 active:scale-[0.98]"
            >
              Create quotation
            </button>
          </div>
        )}

        {!loading && !error && rows.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] text-left text-[13px]">
              <thead>
                <tr className="border-b border-slate-100 text-[11px] font-medium tracking-[0.08em] uppercase text-slate-400">
                  <th className="px-6 py-4 font-medium">Quotation</th>
                  <th className="px-4 py-4 font-medium">Customer</th>
                  <th className="px-4 py-4 font-medium">Subject</th>
                  <th className="px-4 py-4 text-center font-medium">Revision</th>
                  <th className="px-4 py-4 text-right font-medium">Grand total</th>
                  <th className="px-6 py-4" />
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {rows.map((row) => (
                  <tr key={row.ID} className="group transition hover:bg-slate-50/70">
                    <td className="px-6 py-4">
                      <button
                        type="button"
                        onClick={() => onOpenQuotation(row.ID, row.RevisionNumber)}
                        className="text-left font-semibold text-slate-900 underline-offset-4 hover:text-emerald-700 hover:underline focus:outline-none focus:ring-2 focus:ring-emerald-500/20"
                      >
                        {row.QuoteNumber}
                      </button>
                      <p className="mt-1 text-[11px] text-slate-400">{row.CreatedAt.slice(0, 10)}</p>
                    </td>
                    <td className="px-4 py-4 text-slate-700">{row.CustomerName}</td>
                    <td className="max-w-[260px] px-4 py-4 text-slate-600">{row.Subject}</td>
                    <td className="px-4 py-4 text-center tabular-nums text-slate-500">
                      {String(row.RevisionNumber).padStart(2, "0")}
                    </td>
                    <td className="px-4 py-4 text-right font-medium tabular-nums text-slate-800">
                      Rp{row.DisplayGrand}
                    </td>
                    <td className="px-6 py-4 text-right">
                      <button
                        type="button"
                        aria-label={`Delete quotation ${row.QuoteNumber}`}
                        onClick={() => void handleDelete(row.ID)}
                        className="min-h-11 rounded-lg px-3 text-[12px] font-medium text-slate-400 transition hover:bg-red-50 hover:text-red-600 focus:outline-none focus:ring-2 focus:ring-red-500/20"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  );
}
