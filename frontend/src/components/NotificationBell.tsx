import { useCallback, useEffect, useRef, useState } from "react";
import { ListDeletionRequests } from "@backend/backend/AdminAPI";
import { ListEmailChangeRequests } from "@backend/backend/AdminAPI";
import {
  MyDeletionRequests,
  MyEmailChangeRequests,
} from "@backend/backend/AuthAPI";
import { backend } from "@backend/models";
import { useToast } from "../toast";
import { Badge, Button } from "./ui";
import type { SignedInUser } from "./LoginPage";

interface PendingDeletion {
  kind: "deletion";
  ID: number;
  DisplayName: string;
  Email: string;
  RequestedAt: string;
}

interface PendingEmailChange {
  kind: "email";
  ID: number;
  DisplayName: string;
  Email: string;
  ProposedEmail: string;
  RequestedAt: string;
}

type PendingItem = PendingDeletion | PendingEmailChange;

interface OwnRequest {
  kind: "deletion" | "email";
  ID: number;
  Status: string;
  ProposedEmail?: string;
}

function seenKey(kind: string, userID: number): string {
  return `vera-seen-${kind}-${userID}`;
}

function loadSeen(kind: string, userID: number): Set<number> {
  try {
    const raw = window.localStorage.getItem(seenKey(kind, userID));
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    if (Array.isArray(parsed)) {
      return new Set(parsed.filter((id): id is number => typeof id === "number"));
    }
  } catch {
    // Corrupt storage must never break notifications.
  }
  return new Set();
}

function saveSeen(kind: string, userID: number, seen: Set<number>) {
  try {
    window.localStorage.setItem(seenKey(kind, userID), JSON.stringify([...seen]));
  } catch {
    // Storage failures must never break notifications.
  }
}

export function NotificationBell({
  user,
  onOpenTeam,
}: {
  user: SignedInUser;
  onOpenTeam?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState<PendingItem[]>([]);
  const [mine, setMine] = useState<OwnRequest[]>([]);
  const seenDeletionRef = useRef<Set<number>>(loadSeen("deletion", user.ID));
  const seenEmailRef = useRef<Set<number>>(loadSeen("email", user.ID));
  const { pushToast } = useToast();
  const isAdmin = user.Role === "admin";

  const refresh = useCallback(async () => {
    try {
      const ownDeletion = (await MyDeletionRequests()) as backend.OwnDeletionRequest[];
      const ownEmail = (await MyEmailChangeRequests()) as backend.EmailChangeRequest[];
      const own: OwnRequest[] = [
        ...ownDeletion.map((request) => ({
          kind: "deletion" as const,
          ID: request.ID,
          Status: request.Status,
        })),
        ...ownEmail.map((request) => ({
          kind: "email" as const,
          ID: request.ID,
          Status: request.Status,
          ProposedEmail: request.ProposedEmail,
        })),
      ];
      setMine(own);
      for (const request of own) {
        const seen = request.kind === "deletion" ? seenDeletionRef.current : seenEmailRef.current;
        if (
          (request.Status === "approved" || request.Status === "rejected") &&
          !seen.has(request.ID)
        ) {
          seen.add(request.ID);
          const noun = request.kind === "deletion" ? "deletion request" : "email change";
          pushToast(
            "info",
            request.Status === "approved"
              ? `Your ${noun} was approved.` +
                  (request.kind === "deletion"
                    ? " You'll be signed out within a minute."
                    : " Use the new address next time you sign in.")
              : `Your ${noun} was rejected. Everything stands as-is.`
          );
        }
      }
      saveSeen("deletion", user.ID, seenDeletionRef.current);
      saveSeen("email", user.ID, seenEmailRef.current);
      if (isAdmin) {
        const deletions = (await ListDeletionRequests()) as backend.DeletionRequest[];
        const emails = (await ListEmailChangeRequests()) as backend.EmailChangeRow[];
        const rows: PendingItem[] = [
          ...deletions.map((row) => ({
            kind: "deletion" as const,
            ID: row.ID,
            DisplayName: row.DisplayName,
            Email: row.Email,
            RequestedAt: row.RequestedAt,
          })),
          ...emails.map((row) => ({
            kind: "email" as const,
            ID: row.ID,
            DisplayName: row.DisplayName,
            Email: row.Email,
            ProposedEmail: row.ProposedEmail,
            RequestedAt: row.RequestedAt,
          })),
        ];
        setPending(rows);
        const freshDeletion = deletions.filter((row) => !seenDeletionRef.current.has(row.ID));
        const freshEmail = emails.filter((row) => !seenEmailRef.current.has(row.ID));
        for (const row of freshDeletion) seenDeletionRef.current.add(row.ID);
        for (const row of freshEmail) seenEmailRef.current.add(row.ID);
        const freshCount = freshDeletion.length + freshEmail.length;
        if (freshCount > 0) {
          pushToast(
            "info",
            freshCount === 1
              ? "A new request needs review on the Team page."
              : `${freshCount} new requests need review on the Team page.`
          );
        }
        saveSeen("deletion", user.ID, seenDeletionRef.current);
        saveSeen("email", user.ID, seenEmailRef.current);
      }
    } catch {
      // Poll failures stay silent; the session poll owns connectivity errors.
    } finally {
      saveSeen("deletion", user.ID, seenDeletionRef.current);
      saveSeen("email", user.ID, seenEmailRef.current);
    }
  }, [isAdmin, pushToast, user.ID]);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 60000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const openTeam = () => {
    setOpen(false);
    onOpenTeam?.();
  };

  // Badge counts items needing attention: pending reviews for admins,
  // the staffer's own pending requests for staff. Decided outcomes arrive
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
        className="relative inline-flex min-h-9 w-9 items-center justify-center rounded-lg text-ink-500 transition-colors duration-150 hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
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
                <>
                  <ul className="divide-y divide-rule">
                    {pending.map((row) => (
                      <li key={`${row.kind}-${row.ID}`} className="flex items-center gap-2 py-2.5">
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-small font-medium text-ink-900">
                            {row.DisplayName}{" "}
                            <span className="font-normal text-ink-400">
                              · {row.kind === "deletion" ? "deletion" : "email change"}
                            </span>
                          </p>
                          <p className="truncate text-caption text-ink-500">
                            {row.kind === "deletion"
                              ? `${row.Email} · ${row.RequestedAt.slice(0, 10)}`
                              : `${row.Email} → ${row.ProposedEmail}`}
                          </p>
                        </div>
                        <Button variant="ghost" size="xs" onClick={openTeam}>
                          Review
                        </Button>
                      </li>
                    ))}
                  </ul>
                  <div className="pb-1 pt-1">
                    <Button variant="secondary" size="xs" onClick={openTeam}>
                      Open Team page
                    </Button>
                  </div>
                </>
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
                  : "No requests. Start one from Settings."}
              </p>
            ) : (
              <ul className="divide-y divide-rule">
                {mine.map((request) => (
                  <li
                    key={`${request.kind}-${request.ID}`}
                    className="flex items-center justify-between gap-2 py-2"
                  >
                    <span className="text-small text-ink-700">
                      {request.kind === "deletion"
                        ? `Deletion request #${request.ID}`
                        : `Email change to ${request.ProposedEmail ?? ""}`}
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
