import { Fragment, useCallback, useEffect, useMemo, useState } from "react";
import {
  CreateClient,
  DeleteClient,
  ListClients,
  UpdateClient,
} from "@backend/backend/AdminAPI";
import { backend } from "@backend/models";
import { useToast } from "../toast";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

type ManagedClient = backend.ManagedClient;

function errorMessage(err: unknown, fallback: string): string {
  return typeof err === "string" && err.trim() ? err : fallback;
}

export function ClientsPage() {
  const [clients, setClients] = useState<ManagedClient[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [editName, setEditName] = useState("");
  const [editAddress, setEditAddress] = useState("");
  const [editBusy, setEditBusy] = useState(false);
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [deleteText, setDeleteText] = useState("");
  const { pushToast } = useToast();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = (await ListClients()) as ManagedClient[];
      setClients(result);
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't load clients. Try again.");
      setError(message);
      pushToast("error", message);
    } finally {
      setLoading(false);
    }
  }, [pushToast]);

  useEffect(() => {
    void load();
  }, [load]);

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return clients;
    return clients.filter(
      (client) =>
        client.Name.toLowerCase().includes(needle) ||
        String(client.Code).includes(needle)
    );
  }, [clients, query]);

  const refreshRow = useCallback(async () => {
    try {
      setClients((await ListClients()) as ManagedClient[]);
    } catch {
      // List errors already surfaced by load(); stay silent on background refresh.
    }
  }, []);

  const nextCode = useMemo(() => {
    const max = clients.reduce((top, client) => Math.max(top, client.Code), 0);
    return max + 1;
  }, [clients]);

  const handleEditSave = async (client: ManagedClient) => {
    const name = editName.trim();
    const address = editAddress.trim();
    if (!name || !address || editBusy) return;
    setEditBusy(true);
    try {
      await UpdateClient(client.ID, name, address);
      pushToast("success", `${name} updated.`);
      setEditId(null);
      await refreshRow();
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't update this client."));
    } finally {
      setEditBusy(false);
    }
  };

  const handleDelete = async (client: ManagedClient) => {
    try {
      const name = (await DeleteClient(client.ID)) as string;
      pushToast("success", `Deleted ${name}.`);
      setDeleteId(null);
      setDeleteText("");
      await refreshRow();
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't delete this client."));
    }
  };

  return (
    <section aria-labelledby="clients-heading" className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 id="clients-heading" className="text-display font-semibold text-ink-950">
            Clients
          </h1>
          <p className="mt-1.5 max-w-3xl text-body text-ink-500">
            Who quotations are made for. New clients appear in the editor
            immediately — no rebuild. Only admins see this page.
          </p>
        </div>
        <Button variant="primary" onClick={() => setShowCreate((open) => !open)}>
          New client
        </Button>
      </div>

      {showCreate ? (
        <CreateClientForm
          nextCode={nextCode}
          onDone={() => {
            setShowCreate(false);
            void refreshRow();
          }}
          onCancel={() => setShowCreate(false)}
        />
      ) : null}

      <Card className="overflow-hidden">
        <div className="border-b border-rule px-5 py-3">
          <input
            type="search"
            aria-label="Search clients"
            placeholder="Search name or code…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="w-full max-w-sm rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-hidden transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20"
          />
        </div>

        {loading ? (
          <div className="space-y-4 p-6" aria-label="Loading clients">
            <div className="h-8 w-64 animate-pulse rounded-md bg-paper-muted" />
            <div className="h-72 animate-pulse rounded-xl bg-paper-muted" />
          </div>
        ) : error ? (
          <div className="flex flex-col items-center gap-3 px-6 py-16 text-center">
            <p className="text-body text-danger-700">{error}</p>
            <Button onClick={() => void load()}>Try again</Button>
          </div>
        ) : visible.length === 0 ? (
          <div className="flex flex-col items-center gap-3 px-6 py-16 text-center">
            <p className="text-lead font-medium text-ink-900">No matching clients</p>
            <p className="max-w-md text-body text-ink-500">
              {clients.length === 0
                ? "Create the first client to get started."
                : "Try a different search."}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto scroll-thin">
            <table className="w-full min-w-[860px] table-fixed text-left text-body">
              <colgroup>
                <col className="w-20" />
                <col />
                <col className="w-28" />
                <col className="w-44" />
              </colgroup>
              <thead>
                <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                  <th className="px-5 py-2.5 font-medium">Code</th>
                  <th className="px-4 py-2.5 font-medium">Client</th>
                  <th className="px-4 py-2.5 font-medium">Quotes</th>
                  <th className="px-5 py-2.5 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-rule">
                {visible.map((client) => (
                  <Fragment key={client.ID}>
                    <tr className="align-middle transition-colors duration-150 hover:bg-paper-subtle/50">
                      <td className="tnum px-5 py-3 font-medium text-ink-900">
                        {String(client.Code).padStart(2, "0")}
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-900">{client.Name}</div>
                        <div className="mt-0.5 text-caption text-ink-500">{client.PrimaryAddress}</div>
                      </td>
                      <td className="tnum px-4 py-3 text-body text-ink-600">
                        {client.QuotationCount}
                      </td>
                      <td className="px-5 py-3">
                        <div className="flex flex-wrap items-center justify-end gap-1.5">
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`Edit ${client.Name}`}
                            onClick={() => {
                              setEditId(client.ID);
                              setEditName(client.Name);
                              setEditAddress(client.PrimaryAddress);
                              setDeleteId(null);
                            }}
                            className="shrink-0 whitespace-nowrap"
                          >
                            Edit
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`Delete ${client.Name}`}
                            onClick={() => {
                              setDeleteId(client.ID);
                              setDeleteText("");
                              setEditId(null);
                            }}
                            className="shrink-0 whitespace-nowrap hover:!bg-danger-50 hover:!text-danger-600"
                          >
                            Delete
                          </Button>
                        </div>
                      </td>
                    </tr>
                    {editId === client.ID ? (
                      <tr className="bg-accent-50/50">
                        <td colSpan={4} className="px-5 py-3">
                          <p className="text-small text-ink-800">
                            Editing <span className="font-medium">{client.Name}</span>.
                            The code ({String(client.Code).padStart(2, "0")}) cannot be
                            changed — quotation cycles are keyed by it.
                          </p>
                          <div className="mt-2 grid max-w-2xl gap-2">
                            <input
                              type="text"
                              value={editName}
                              onChange={(event) => setEditName(event.target.value)}
                              placeholder="Legal name"
                              aria-label="Client legal name"
                              className="w-full rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-hidden transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20"
                            />
                            <input
                              type="text"
                              value={editAddress}
                              onChange={(event) => setEditAddress(event.target.value)}
                              placeholder="Primary address"
                              aria-label="Client primary address"
                              className="w-full rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-hidden transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20"
                            />
                            <div className="flex gap-1.5">
                              <Button
                                variant="primary"
                                size="sm"
                                disabled={editBusy || !editName.trim() || !editAddress.trim()}
                                onClick={() => void handleEditSave(client)}
                              >
                                {editBusy ? "Saving…" : "Save client"}
                              </Button>
                              <Button variant="ghost" size="sm" onClick={() => setEditId(null)}>
                                Cancel
                              </Button>
                            </div>
                          </div>
                        </td>
                      </tr>
                    ) : null}
                    {deleteId === client.ID ? (
                      <tr className="bg-danger-50/50">
                        <td colSpan={4} className="px-5 py-3">
                          {client.QuotationCount > 0 ? (
                            <p className="text-small text-ink-800">
                              <span className="font-medium">{client.Name}</span> has{" "}
                              <span className="tnum font-medium">{client.QuotationCount}</span>{" "}
                              quotation{client.QuotationCount === 1 ? "" : "s"} and cannot be
                              deleted — archive history must keep pointing at it.
                            </p>
                          ) : (
                            <>
                              <p className="text-small text-ink-800">
                                Permanently delete{" "}
                                <span className="font-medium">{client.Name}</span>?
                                Type the name to confirm.
                              </p>
                              <div className="mt-2 flex flex-col gap-2 sm:flex-row">
                                <input
                                  type="text"
                                  autoComplete="off"
                                  value={deleteText}
                                  onChange={(event) => setDeleteText(event.target.value)}
                                  placeholder={client.Name}
                                  aria-label={`Type ${client.Name} to confirm deletion`}
                                  className="w-full max-w-[280px] rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-hidden transition placeholder:text-ink-300 hover:border-ink-300 focus:border-danger-300 focus:ring-2 focus:ring-danger-500/20"
                                />
                                <div className="flex gap-1.5">
                                  <Button
                                    variant="danger"
                                    size="sm"
                                    disabled={deleteText.trim().toLowerCase() !== client.Name.toLowerCase()}
                                    onClick={() => void handleDelete(client)}
                                  >
                                    Delete forever
                                  </Button>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => {
                                      setDeleteId(null);
                                      setDeleteText("");
                                    }}
                                  >
                                    Keep
                                  </Button>
                                </div>
                              </div>
                            </>
                          )}
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <p className="text-caption leading-relaxed text-ink-400">
        Client codes feed quotation numbers, so a code is permanent once
        created. Renames and address changes apply to future quotations;
        past revisions keep their snapshots.
      </p>
    </section>
  );
}

function CreateClientForm({
  nextCode,
  onDone,
  onCancel,
}: {
  nextCode: number;
  onDone: () => void;
  onCancel: () => void;
}) {
  const [code, setCode] = useState(nextCode > 99 ? 0 : nextCode);
  const [name, setName] = useState("");
  const [address, setAddress] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const created = (await CreateClient(code, name.trim(), address.trim())) as ManagedClient;
      pushToast("success", `${created.Name} created with code ${String(created.Code).padStart(2, "0")}.`);
      onDone();
    } catch (err: unknown) {
      setError(errorMessage(err, "We couldn't create this client."));
    } finally {
      setBusy(false);
    }
  };

  const fieldClass =
    "w-full rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-hidden transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20";

  return (
    <Card className="p-5">
      <SectionHeading title="New client" />
      <form onSubmit={submit} className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {error ? (
          <div className="sm:col-span-2 lg:col-span-4">
            <StatusBanner tone="danger">{error}</StatusBanner>
          </div>
        ) : null}
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">Code (0 = next free)</span>
          <input
            type="text"
            inputMode="numeric"
            value={code === 0 ? "" : String(code)}
            onChange={(event) => {
              const parsed = Number.parseInt(event.target.value.replace(/\D/g, ""), 10);
              setCode(Number.isFinite(parsed) ? Math.min(parsed, 99) : 0);
            }}
            placeholder={nextCode > 99 ? "Codes full" : `Next: ${nextCode}`}
            aria-label="Client code, empty for next free"
            className={fieldClass}
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">Legal name</span>
          <input
            type="text"
            required
            autoFocus
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="PT Example Persada"
            className={fieldClass}
          />
        </label>
        <label className="flex flex-col gap-1.5 sm:col-span-2 lg:col-span-1">
          <span className="text-caption font-medium text-ink-600">Primary address</span>
          <input
            type="text"
            required
            value={address}
            onChange={(event) => setAddress(event.target.value)}
            placeholder="Street, city, province"
            className={fieldClass}
          />
        </label>
        <div className="flex items-end gap-2">
          <Button type="submit" variant="primary" disabled={busy} className="flex-1">
            {busy ? "Creating…" : "Create client"}
          </Button>
          <Button variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
        </div>
      </form>
      <p className="mt-3 text-caption leading-relaxed text-ink-400">
        The code feeds quotation numbers and can never change — leave it empty
        to take the next free one.
      </p>
    </Card>
  );
}
