import { useCallback, useEffect, useState } from "react";
import { ChangePassword, LogoutEverywhere, RequestDeletion } from "@backend/backend/AuthAPI";
import {
  DecideDeletion,
  ListDeletionRequests,
} from "@backend/backend/AdminAPI";
import { useToast } from "../toast";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

interface DeletionRequestRow {
  ID: number;
  UserID: number;
  Email: string;
  DisplayName: string;
  Status: string;
  RequestedAt: string;
}

function errorMessage(err: unknown, fallback: string): string {
  return typeof err === "string" && err.trim() ? err : fallback;
}

const fieldClass =
  "w-full rounded-md border border-rule bg-white px-3 py-2 text-body text-ink-900 outline-none transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20";

export function SettingsPage({ isAdmin }: { isAdmin: boolean }) {
  return (
    <section aria-labelledby="settings-heading" className="space-y-6">
      <div>
        <h1 id="settings-heading" className="text-display font-semibold text-ink-950">
          Settings
        </h1>
        <p className="mt-1.5 max-w-3xl text-body text-ink-500">
          Your own account. Team management lives on the Team page for admins.
        </p>
      </div>

      <PasswordCard />
      <SessionsCard />
      <DeletionCard />
      {isAdmin ? <DeletionQueue /> : null}
    </section>
  );
}

function PasswordCard() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { pushToast } = useToast();

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    if (next !== confirm) {
      setError("The new passwords do not match.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await ChangePassword(current, next);
      setCurrent("");
      setNext("");
      setConfirm("");
      pushToast("success", "Password changed. Other devices were signed out.");
    } catch (err: unknown) {
      const message = errorMessage(err, "We couldn't change your password.");
      setError(message);
      pushToast("error", message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="p-5">
      <SectionHeading title="Password" />
      <form onSubmit={submit} className="grid max-w-xl gap-4">
        {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
        <label className="flex flex-col gap-1.5">
          <span className="text-caption font-medium text-ink-600">Current password</span>
          <input
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(event) => setCurrent(event.target.value)}
            placeholder="Required unless this is your first setup"
            className={fieldClass}
          />
        </label>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">New password</span>
            <input
              type="password"
              autoComplete="new-password"
              required
              minLength={10}
              value={next}
              onChange={(event) => setNext(event.target.value)}
              placeholder="At least 10 characters"
              className={fieldClass}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-caption font-medium text-ink-600">Confirm new password</span>
            <input
              type="password"
              autoComplete="new-password"
              required
              value={confirm}
              onChange={(event) => setConfirm(event.target.value)}
              placeholder="Repeat it"
              className={fieldClass}
            />
          </label>
        </div>
        <div>
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Changing…" : "Change password"}
          </Button>
        </div>
      </form>
    </Card>
  );
}

function SessionsCard() {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const { pushToast } = useToast();

  const confirm = async () => {
    setBusy(true);
    try {
      await LogoutEverywhere();
      pushToast("success", "Signed out everywhere. Sign in again to continue.");
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't sign out everywhere."));
    } finally {
      setBusy(false);
      setConfirming(false);
    }
  };

  return (
    <Card className="p-5">
      <SectionHeading title="Sessions" />
      <p className="max-w-2xl text-body text-ink-600">
        Ends every session for this account on all devices, including this one.
        You will be asked to sign in again.
      </p>
      <div className="mt-4">
        {confirming ? (
          <span className="flex flex-wrap items-center gap-2">
            <span className="text-caption text-ink-600">Sign out everywhere, including here?</span>
            <Button variant="danger" size="sm" disabled={busy} onClick={() => void confirm()}>
              Sign out everywhere
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setConfirming(false)}>
              Keep me signed in
            </Button>
          </span>
        ) : (
          <Button variant="secondary" onClick={() => setConfirming(true)}>
            Sign out everywhere
          </Button>
        )}
      </div>
    </Card>
  );
}

function DeletionCard() {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [requested, setRequested] = useState(false);
  const { pushToast } = useToast();

  const confirm = async () => {
    setBusy(true);
    try {
      await RequestDeletion(true);
      setRequested(true);
      pushToast("success", "Deletion requested. An admin must approve it.");
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't record your request."));
    } finally {
      setBusy(false);
      setConfirming(false);
    }
  };

  return (
    <Card className="border-danger-200 p-5">
      <SectionHeading title="Delete account" />
      {requested ? (
        <StatusBanner tone="info">
          Your request is pending. An admin must approve it; your quotations and
          history stay intact either way.
        </StatusBanner>
      ) : (
        <>
          <p className="max-w-2xl text-body text-ink-600">
            Request deletion of your account. An admin reviews and approves it —
            nothing is deleted automatically, and your past quotations are kept
            for the archive.
          </p>
          <div className="mt-4">
            {confirming ? (
              <span className="flex flex-wrap items-center gap-2">
                <span className="text-caption text-ink-600">
                  Ask an admin to delete your account?
                </span>
                <Button variant="danger" size="sm" disabled={busy} onClick={() => void confirm()}>
                  Request deletion
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setConfirming(false)}>
                  Keep account
                </Button>
              </span>
            ) : (
              <Button variant="danger" onClick={() => setConfirming(true)}>
                Request account deletion
              </Button>
            )}
          </div>
        </>
      )}
    </Card>
  );
}

function DeletionQueue() {
  const [rows, setRows] = useState<DeletionRequestRow[]>([]);
  const [loading, setLoading] = useState(true);
  const { pushToast } = useToast();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = (await ListDeletionRequests()) as DeletionRequestRow[];
      setRows(result);
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't load deletion requests."));
    } finally {
      setLoading(false);
    }
  }, [pushToast]);

  useEffect(() => {
    void load();
  }, [load]);

  const decide = async (id: number, approve: boolean) => {
    try {
      await DecideDeletion(id, approve);
      pushToast("success", approve ? "Account deleted." : "Request rejected.");
      await load();
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't decide this request."));
    }
  };

  return (
    <Card className="overflow-hidden">
      <div className="border-b border-rule px-5 py-4">
        <SectionHeading title="Deletion requests" />
        <p className="text-body text-ink-500">
          Approving deactivates the account immediately and signs them out
          everywhere. Records are kept; nothing is purged.
        </p>
      </div>
      {loading ? (
        <div className="h-24 animate-pulse rounded-xl bg-paper-muted m-5" aria-label="Loading requests" />
      ) : rows.length === 0 ? (
        <p className="px-5 py-8 text-center text-body text-ink-500">
          No pending requests.
        </p>
      ) : (
        <ul className="divide-y divide-rule">
          {rows.map((row) => (
            <li key={row.ID} className="flex flex-wrap items-center gap-3 px-5 py-3.5">
              <div className="min-w-0 flex-1">
                <div className="font-medium text-ink-900">{row.DisplayName}</div>
                <div className="mt-0.5 text-caption text-ink-500">
                  {row.Email} · requested {row.RequestedAt.slice(0, 10)}
                </div>
              </div>
              <Badge tone="internal">Pending</Badge>
              <div className="flex items-center gap-1.5">
                <Button variant="danger" size="sm" onClick={() => void decide(row.ID, true)}>
                  Approve
                </Button>
                <Button variant="ghost" size="sm" onClick={() => void decide(row.ID, false)}>
                  Reject
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
