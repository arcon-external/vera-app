import { Button, StatusBanner } from "./ui";

/**
 * Ticket #536: the "Stay signed in" window. The banner appears once the
 * session's idle deadline is within ten minutes, and its one action asks the
 * server to slide that deadline. It is never modal: a quotation tool must not
 * stand between the user and their deadline.
 */
export function SessionRenewBanner({
  minutesLeft,
  busy,
  error,
  onRenew,
}: {
  /** Whole minutes until the idle deadline, rounded up. */
  minutesLeft: number;
  /** True while the renewal request is in flight. */
  busy: boolean;
  /** The last operational failure, shown without ending the session. */
  error: string | null;
  onRenew: () => void;
}) {
  const when =
    minutesLeft <= 1 ? "in less than a minute" : `in about ${minutesLeft} minutes`;
  return (
    <div
      className="mx-auto w-full max-w-[1560px] px-5 pt-5 lg:px-8"
      role="status"
    >
      <StatusBanner tone="info">
        <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span>
            Your session ends {when}. Stay signed in to keep working without
            signing in again.
            {error ? ` ${error}` : ""}
          </span>
          <span className="flex items-center gap-2">
            <Button
              variant="primary"
              size="sm"
              onClick={onRenew}
              disabled={busy}
            >
              {busy ? "Extending…" : "Stay signed in"}
            </Button>
          </span>
        </span>
      </StatusBanner>
    </div>
  );
}
