import { useCallback, useEffect, useRef, useState } from "react";
import {
  DecideDeletion,
  ListDeletionRequests,
} from "@backend/backend/AdminAPI";
import { MyDeletionRequests } from "@backend/backend/AuthAPI";
import { useToast } from "../toast";
import { Badge, Button } from "./ui";
import type { SignedInUser } from "./LoginPage";

interface PendingRow {
  ID: number;
  UserID: number;
  Email: string;
  DisplayName: string;
  Status: string;
  RequestedAt: string;
}

interface OwnRow {
  ID: number;
  Status: string;
}

function errorMessage(err: unknown, fallback: string): string {
  return typeof err === "string" && err.trim() ? err : fallback;
}

function seenKey(userID: number): string {
  return `vera-seen-deletion-${userID}`;
}

function loadSeen(userID: number): Set<number> {
  try {
    const raw = window.localStorage.getItem(seenKey(userID));
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    if (Array.isArray(parsed)) {
      return new Set(parsed.filter((id): id is number => typeof id === "number"));
    }
  } catch {
    // Corrupt storage must never break notifications.
  }
  return new Set();
}

export function NotificationBell({ user }: { user: SignedInUser }) {
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState<PendingRow[]>([]);
  const [mine, setMine] = useState<OwnRow[]>([]);
  const seenRef = useRef<Set<number>>(loadSeen(user.ID));
  const { pushToast } = useToast();
  const isAdmin = user.Role === "admin";

  const refresh = useCallback(async () => {
    try {
      const own = (await MyDeletionRequests()) as OwnRow[];
      setMine(own);
      for (const request of own) {
        if (
          (request.Status === "approved" || request.Status === "rejected") &&
          !seenRef.current.has(request.ID)
        ) {
          seenRef.current.add(request.ID);
          pushToast(
            "info",
            request.Status === "approved"
              ? "Your deletion request was approved. You'll be signed out within a minute."
              : "Your deletion request was rejected. Your account stands as-is."
          );
        }
      }
      if (isAdmin) {
        const rows = (await ListDeletionRequests()) as PendingRow[];
        setPending(rows);
        const fresh = rows.filter((row) => !seenRef.current.has(row.ID));
        if (fresh.length > 0) {
          for (const row of fresh) seenRef.current.add(row.ID);
          pushToast(
            "info",
            fresh.length === 1
              ? `New deletion request from ${fresh[0].DisplayName}.`
              : `${fresh.length} new deletion requests need review.`
          );
        }
      }
    } catch {
      // Poll failures stay silent; the session poll owns connectivity errors.
    } finally {
      try {
        window.localStorage.setItem(
          seenKey(user.ID),
          JSON.stringify([...seenRef.current])
        );
      } catch {
        // Storage failures must never break notifications.
      }
    }
  }, [isAdmin, pushToast, user.ID]);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 60000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const decide = async (id: number, approve: boolean) => {
    try {
      await DecideDeletion(id, approve);
      pushToast(
        "success",
        approve
          ? "Approved. They'll be signed out within a minute."
          : "Request rejected."
      );
      await refresh();
    } catch (err: unknown) {
      pushToast("error", errorMessage(err, "We couldn't decide this request."));
    }
  };

  // Badge counts items needing attention: pending reviews for admins,
  // the staffer's own pending request for staff. Decided outcomes arrive
  // via toast exactly once (seen-gated above), so the badge never depends
  // on mutable ref state and cannot go stale.
  const badgeCount = isAdmin
    ? pending.length
    : mine.filter((request) => request.Status === "pending").length;

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        onBlur={(event) => {
          if (!event.currentTarget.parentElement?.contains(event.relatedTarget as Node)) {
            setOpen(false);
          }
        }}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={
          badgeCount > 0
            ? `Notifications, ${badgeCount} unread`
            : "Notifications"
        }
        className="relative inline-flex min-h-9 w-9 items-center justify-center rounded-lg text-ink-500 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-500/40"
      >
        <svg
          width="17"
          height="17"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
          <path d="M13.7 21a2 2 0 0 1-3.4 0" />
        </svg>
        {badgeCount > 0 ? (
          <span
            aria-hidden="true"
            className="absolute right-1 top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-ink-900 px-1 text-micro font-semibold text-white"
          >
            {badgeCount > 9 ? "9+" : badgeCount}
          </span>
        ) : null}
      </button>
      {open ? (
        <div
          role="menu"
          aria-label="Notifications"
          className="absolute right-0 top-full z-40 mt-2 w-[min(360px,calc(100vw-2rem))] rounded-xl border border-rule bg-white p-2 shadow-pop"
        >
          {isAdmin ? (
            <div className="px-3 pb-1 pt-2">
              <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
                Needs review
              </p>
              {pending.length === 0 ? (
                <p className="py-2 text-small text-ink-500">No pending requests.</p>
              ) : (
                <ul className="divide-y divide-rule">
                  {pending.map((row) => (
                    <li key={row.ID} className="flex items-center gap-2 py-2.5">
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-small font-medium text-ink-900">
                          {row.DisplayName}
                        </p>
                        <p className="truncate text-caption text-ink-500">
                          {row.Email} · {row.RequestedAt.slice(0, 10)}
                        </p>
                      </div>
                      <Button
                        variant="danger"
                        size="xs"
                        onClick={() => void decide(row.ID, true)}
                      >
                        Approve
                      </Button>
                      <Button
                        variant="ghost"
                        size="xs"
                        onClick={() => void decide(row.ID, false)}
                      >
                        Reject
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          ) : null}
          <div className="px-3 pb-2 pt-1">
            <p className="text-caption font-medium uppercase tracking-[0.14em] text-ink-500">
              Your requests
            </p>
            {mine.length === 0 ? (
              <p className="py-2 text-small text-ink-500">
                {isAdmin
                  ? "You have no personal requests."
                  : "No deletion requests. Request one from Settings."}
              </p>
            ) : (
              <ul className="divide-y divide-rule">
                {mine.map((request) => (
                  <li
                    key={request.ID}
                    className="flex items-center justify-between gap-2 py-2"
                  >
                    <span className="text-small text-ink-700">
                      Request #{request.ID}
                    </span>
                    <Badge
                      tone={request.Status === "pending" ? "internal" : "neutral"}
                    >
                      {request.Status === "pending"
                        ? "Pending review"
                        : request.Status === "approved"
                          ? "Approved"
                          : "Rejected"}
                    </Badge>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}
