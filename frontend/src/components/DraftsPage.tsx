import { useCallback, useEffect, useRef, useState } from "react";
import { DeleteDraft, ListDrafts } from "@backend/backend/Quotation";
import type { DraftSummaryDTO } from "../drafts";
import { relativeSavedAt } from "../drafts";
import { useToast } from "../toast";
import { Button, Card } from "./ui";

interface DraftsPageProps {
  onOpenDraft: (id: number) => void;
  onNewQuotation: () => void;
}

export function DraftsPage({ onOpenDraft, onNewQuotation }: DraftsPageProps) {
  const [rows, setRows] = useState<DraftSummaryDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [confirmingId, setConfirmingId] = useState<number | null>(null);
  const [openingId, setOpeningId] = useState<number | null>(null);
  const confirmTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { pushToast } = useToast();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setRows(((await ListDrafts()) as DraftSummaryDTO[]) ?? []);
    } catch {
      const message =
        "We couldn't load your drafts. Check the connection and try again.";
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
      await DeleteDraft(id);
      setRows((current) => current.filter((row) => row.ID !== id));
      setConfirmingId(null);
      pushToast("success", "Draft deleted.");
    } catch {
      pushToast("error", "We couldn't delete this draft. Try again.");
    }
  };

  // A draft is disposable and this is the only copy, so opening is worth a
  // beat of feedback -- the payload can be a sizeable item list.
  const handleOpen = async (id: number) => {
    setOpeningId(id);
    onOpenDraft(id);
    setOpeningId(null);
  };

  return (
    <section aria-labelledby="drafts-heading" className="flex flex-col gap-5">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 id="drafts-heading" className="text-title font-semibold text-ink-950">
            Drafts
          </h1>
          <p className="mt-1 max-w-2xl text-body text-ink-500">
            Quotations you saved but never exported. Only you can see these.
            Opening one puts it back in the editor; exporting it turns it into a
            real quotation and removes it from here.
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
          <div className="space-y-3 p-6" aria-label="Loading drafts">
            {[0, 1, 2].map((row) => (
              <div key={row} className="h-12 animate-pulse rounded-lg bg-paper-muted" />
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
            <p className="text-lead font-medium text-ink-900">No saved drafts</p>
            <p className="max-w-md text-body text-ink-500">
              Half-finished work is lost when the editor closes. Save a draft in
              the editor to pick up where you left off next time.
            </p>
            <Button variant="primary" onClick={onNewQuotation} className="mt-1">
              Start a quotation
            </Button>
          </div>
        ) : (
          <div className="overflow-x-auto scroll-thin">
            <table className="w-full min-w-[720px] text-left text-body">
              <thead>
                <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                  <th className="px-5 py-3 font-medium">Draft</th>
                  <th className="px-4 py-3 font-medium">Customer</th>
                  <th className="px-4 py-3 text-center font-medium">Lines</th>
                  <th className="px-4 py-3 font-medium">Saved</th>
                  <th className="px-5 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-rule">
                {rows.map((row) => (
                  <tr
                    key={row.ID}
                    onClick={() => void handleOpen(row.ID)}
                    className="group cursor-pointer transition-colors duration-150 hover:bg-paper-subtle/60"
                  >
                    <td className="max-w-[380px] px-5 py-3 align-middle">
                      <button
                        type="button"
                        onClick={(event) => {
                          event.stopPropagation();
                          void handleOpen(row.ID);
                        }}
                        className="block w-full rounded text-left text-body font-medium text-ink-900 underline-offset-4 transition-colors duration-150 hover:text-accent-700 hover:underline focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                      >
                        <span className="[display:-webkit-box] overflow-hidden [-webkit-box-orient:vertical] [-webkit-line-clamp:1]">
                          {row.DisplayName}
                        </span>
                      </button>
                      {row.Subject && row.Label ? (
                        <p className="mt-0.5 max-w-[360px] text-caption text-ink-400">
                          <span className="[display:-webkit-box] overflow-hidden [-webkit-box-orient:vertical] [-webkit-line-clamp:1]">
                            {row.Subject}
                          </span>
                        </p>
                      ) : null}
                    </td>
                    <td className="px-4 py-3 align-middle text-body text-ink-700">
                      {row.CustomerName || <span className="text-ink-400">—</span>}
                    </td>
                    <td className="px-4 py-3 text-center align-middle text-body tnum text-ink-600">
                      {row.ItemCount}
                    </td>
                    <td className="px-4 py-3 align-middle text-caption text-ink-500">
                      {relativeSavedAt(row.UpdatedAt)}
                    </td>
                    <td className="px-5 py-3 text-right align-middle">
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
                      ) : openingId === row.ID ? (
                        <span className="text-caption text-ink-400">Opening…</span>
                      ) : (
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`Delete draft ${row.DisplayName}`}
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
          A draft is not a quotation: it has no quote number, it is not signed,
          and it cannot be verified online. Deleting one cannot be undone.
        </p>
      ) : null}
    </section>
  );
}