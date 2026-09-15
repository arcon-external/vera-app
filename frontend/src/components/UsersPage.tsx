import { Fragment, useCallback, useEffect, useMemo, useState } from "react";
import {
  CreateUser,
  DeleteUser,
  IssuePasswordReset,
  ListUsers,
  SetUserActive,
  SetUserRole,
} from "@backend/backend/AdminAPI";
import { backend } from "@backend/models";
import { useToast } from "../toast";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";
import { Select } from "./Select";

type ManagedUser = backend.ManagedUser;

function errorMessage(err: unknown, fallback: string): string {
  return typeof err === "string" && err.trim() ? err : fallback;
}

export function UsersPage() {
  const [users, setUsers] = useState<ManagedUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [confirmId, setConfirmId] = useState<number | null>(null);
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [deleteText, setDeleteText] = useState("");
  const [codeCopied, setCodeCopied] = useState(false);
  const [invite, setInvite] = useState<{ email: string; token: string } | null>(null);
  const { pushToast } = useToast();

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = (await ListUsers()) as ManagedUser[];
      setUsers(result);
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't load users. Try again.");
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
    if (!needle) return users;
    return users.filter(
      (user) =>
        user.Email.toLowerCase().includes(needle) ||
        user.DisplayName.toLowerCase().includes(needle)
    );
  }, [users, query]);

  const refreshRow = useCallback(async () => {
    try {
      setUsers((await ListUsers()) as ManagedUser[]);
    } catch {
      // List errors already surfaced by load(); stay silent on background refresh.
    }
  }, []);

  const handleRole = async (user: ManagedUser, role: string) => {
    if (user.Role === role) return;
    try {
      await SetUserRole(user.ID, role);
      pushToast("success", `${user.Email} is now ${role}.`);
      await refreshRow();
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't change the role."));
    }
  };

  const handleActive = async (user: ManagedUser, active: boolean) => {
    try {
      await SetUserActive(user.ID, active);
      pushToast(
        "success",
        active ? `${user.Email} reactivated.` : `${user.Email} deactivated and signed out everywhere.`
      );
      setConfirmId(null);
      await refreshRow();
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't update this account."));
    }
  };

  const handleReset = async (user: ManagedUser) => {
    try {
      const token = (await IssuePasswordReset(user.ID)) as string;
      setInvite({ email: user.Email, token });
      setCodeCopied(false);
      pushToast("success", `Invite code issued for ${user.Email}.`);
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't issue an invite code."));
    }
  };

  const copyInviteCode = async (token: string) => {
    try {
      await navigator.clipboard.writeText(token);
      setCodeCopied(true);
    } catch {
      setCodeCopied(false);
    }
  };

  const handleDelete = async (user: ManagedUser) => {
    try {
      const email = (await DeleteUser(user.ID)) as string;
      pushToast("success", `Permanently deleted ${email}.`);
      setDeleteId(null);
      setDeleteText("");
      await refreshRow();
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't delete this account."));
    }
  };

  return (
    <section aria-labelledby="users-heading" className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 id="users-heading" className="text-display font-semibold text-ink-950">
            Team
          </h1>
          <p className="mt-1.5 max-w-3xl text-body text-ink-500">
            Who can sign in, what they may do, and when they last worked. Only
            admins see this page.
          </p>
        </div>
        <Button variant="primary" onClick={() => setShowCreate((open) => !open)}>
          New user
        </Button>
      </div>

      {showCreate ? (
        <CreateUserForm
          onDone={(email, token) => {
            setShowCreate(false);
            setInvite({ email, token });
            setCodeCopied(false);
            void refreshRow();
          }}
          onCancel={() => setShowCreate(false)}
        />
      ) : null}

      {invite ? (
        <StatusBanner tone="info">
          <span className="font-medium">Invite code for {invite.email}:</span>{" "}
          <button
            type="button"
            title="Click to copy"
            aria-label={`Copy invite code for ${invite.email}`}
            onClick={() => void copyInviteCode(invite.token)}
            className="tnum inline-flex cursor-pointer items-center gap-1.5 rounded border border-transparent bg-white/70 px-1.5 py-0.5 transition hover:border-rule-strong hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40"
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
              className="shrink-0 opacity-60"
            >
              <rect x="9" y="9" width="12" height="12" rx="2" />
              <path d="M5 15V5a2 2 0 0 1 2-2h10" />
            </svg>
            {invite.token}
          </button>{" "}
          <span className="text-caption">
            {codeCopied ? "Copied." : "Click the code to copy. "}
          </span>
          Share it once — a new code replaces this one, and it expires in an hour.
          <button
            type="button"
            onClick={() => setInvite(null)}
            className="ml-2 underline underline-offset-2 hover:text-ink-900"
          >
            Dismiss
          </button>
        </StatusBanner>
      ) : null}

      <Card className="overflow-hidden">
        <div className="border-b border-rule px-5 py-3">
          <input
            type="search"
            aria-label="Search users"
            placeholder="Search name or email…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="w-full max-w-sm rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-none transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20"
          />
        </div>

        {loading ? (
          <div className="space-y-4 p-6" aria-label="Loading users">
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
            <p className="text-lead font-medium text-ink-900">No matching accounts</p>
            <p className="max-w-md text-body text-ink-500">
              {users.length === 0
                ? "Create the first team member to get started."
                : "Try a different search."}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto scroll-thin">
            <table className="w-full min-w-[860px] table-fixed text-left text-body">
              <colgroup>
                <col />
                <col className="w-36" />
                <col className="w-28" />
                <col className="w-32" />
                <col className="w-56" />
              </colgroup>
              <thead>
                <tr className="border-b border-rule bg-paper-subtle text-micro uppercase tracking-[0.1em] text-ink-500">
                  <th className="px-5 py-2.5 font-medium">Account</th>
                  <th className="px-4 py-2.5 font-medium">Role</th>
                  <th className="px-4 py-2.5 font-medium">Status</th>
                  <th className="px-4 py-2.5 font-medium">Last login</th>
                  <th className="px-5 py-2.5 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-rule">
                {visible.map((user) => (
                  <Fragment key={user.ID}>
                  <tr className="align-middle transition-colors duration-150 hover:bg-paper-subtle/50">
                    <td className="px-5 py-3">
                      <div className="font-medium text-ink-900">{user.DisplayName}</div>
                      <div className="mt-0.5 text-caption text-ink-500">{user.Email}</div>
                      {user.MustChangePassword ? (
                        <div className="mt-1.5">
                          <Badge tone="internal">Must set password</Badge>
                        </div>
                      ) : null}
                    </td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center gap-1.5">
                        <span
                          aria-hidden="true"
                          className={`h-1.5 w-1.5 rounded-full ${
                            user.Role === "admin" ? "bg-accent-600" : "bg-ink-300"
                          }`}
                        />
                        <select
                          aria-label={`Role for ${user.Email}`}
                          value={user.Role}
                          onChange={(event) => void handleRole(user, event.target.value)}
                          className="cursor-pointer rounded-md border border-transparent bg-transparent py-1 pr-1 text-body text-ink-800 outline-none transition hover:border-rule focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20"
                        >
                          <option value="staff">Staff</option>
                          <option value="admin">Admin</option>
                        </select>
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <Badge tone={user.IsActive ? "success" : "neutral"}>
                        {user.IsActive ? "Active" : "Off"}
                      </Badge>
                    </td>
                    <td className="tnum px-4 py-3 text-body text-ink-600">
                      {user.LastLoginAt ? user.LastLoginAt.slice(0, 10) : "—"}
                    </td>
                    <td className="px-5 py-3">
                      <div className="flex items-center justify-end gap-1.5">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => void handleReset(user)}
                        >
                          Invite code
                        </Button>
                        {!user.IsActive && deleteId !== user.ID ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`Permanently delete ${user.Email}`}
                            onClick={() => {
                              setDeleteId(user.ID);
                              setDeleteText("");
                            }}
                            className="hover:!bg-danger-50 hover:!text-danger-600"
                          >
                            Delete
                          </Button>
                        ) : null}
                        {confirmId === user.ID ? (
                          <>
                            <Button
                              variant="danger"
                              size="sm"
                              onClick={() => void handleActive(user, !user.IsActive)}
                            >
                              Confirm
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setConfirmId(null)}
                            >
                              Keep
                            </Button>
                          </>
                        ) : (
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={user.IsActive ? `Deactivate ${user.Email}` : `Reactivate ${user.Email}`}
                            onClick={() => setConfirmId(user.ID)}
                            className="hover:!bg-danger-50 hover:!text-danger-600"
                          >
                            {user.IsActive ? "Deactivate" : "Reactivate"}
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                  {deleteId === user.ID ? (
                    <tr className="bg-danger-50/50">
                      <td colSpan={5} className="px-5 py-3">
                        <p className="text-small text-ink-800">
                          Permanently delete{" "}
                          <span className="font-medium">{user.Email}</span>? Their
                          sessions and codes go with the account; audit entries
                          stay but lose the name. Type the email to confirm.
                        </p>
                        <div className="mt-2 flex flex-col gap-2 sm:flex-row">
                          <input
                            type="text"
                            autoComplete="off"
                            value={deleteText}
                            onChange={(event) => setDeleteText(event.target.value)}
                            placeholder={user.Email}
                            aria-label={`Type ${user.Email} to confirm deletion`}
                            className="w-full max-w-[280px] rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-none transition placeholder:text-ink-300 hover:border-ink-300 focus:border-danger-300 focus:ring-2 focus:ring-danger-500/20"
                          />
                          <div className="flex gap-1.5">
                            <Button
                              variant="danger"
                              size="sm"
                              disabled={deleteText.trim().toLowerCase() !== user.Email.toLowerCase()}
                              onClick={() => void handleDelete(user)}
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
        Deactivating signs the person out on their device immediately. Role and
        status changes take effect on their next action. Deletion is permanent
        and only offered for deactivated accounts.
      </p>
    </section>
  );
}

function CreateUserForm({
  onDone,
  onCancel,
}: {
  onDone: (email: string, token: string) => void;
  onCancel: () => void;
}) {
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [role, setRole] = useState("staff");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const user = (await CreateUser(email.trim(), displayName.trim(), role)) as {
        ID: number;
        Email: string;
      };
      const token = (await IssuePasswordReset(user.ID)) as string;
      pushToast("success", `Account created for ${user.Email}.`);
      onDone(user.Email, token);
    } catch (err: unknown) {
      setError(errorMessage(err, "We couldn't create this account."));
    } finally {
      setBusy(false);
    }
  };

  const fieldClass =
    "w-full rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-none transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20";

  return (
    <Card className="p-5">
      <SectionHeading title="New team member" />
      <form onSubmit={submit} className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {error ? (
          <div className="sm:col-span-2 lg:col-span-4">
            <StatusBanner tone="danger">{error}</StatusBanner>
          </div>
        ) : null}
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">Email</span>
          <input
            type="email"
            required
            autoFocus
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder="name@company.com"
            className={fieldClass}
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">Display name</span>
          <input
            type="text"
            required
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            placeholder="Full name"
            className={fieldClass}
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">Role</span>
          <Select
            value={role}
            ariaLabel="Role"
            options={[
              { value: "staff", label: "Staff — quotations only" },
              { value: "admin", label: "Admin — full access" },
            ]}
            onChange={(next) => setRole(String(next))}
          />
        </label>
        <div className="flex items-end gap-2">
          <Button type="submit" variant="primary" disabled={busy} className="flex-1">
            {busy ? "Creating…" : "Create account"}
          </Button>
          <Button variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
        </div>
      </form>
      <p className="mt-3 text-caption leading-relaxed text-ink-400">
        The account starts locked without a password. Share the invite code once —
        it expires in an hour and can only be used a single time.
      </p>
    </Card>
  );
}
