import type { JSX } from "react";
import { Button, StatusBanner } from "./ui";

/**
 * Issue #537: the local recovery snapshot.
 *
 * A crash or power loss can leave unsaved work on disk. The banner offers it
 * back after the next sign-in. It is never modal: the editor stays usable, and
 * Restore and Discard are the only two answers. Dismissing it by navigating
 * away is not an option the user is forced to make; the editor's own state is
 * untouched until they choose.
 */
export function RecoveryBanner({
  label,
  busy,
  error,
  onRestore,
  onDiscard,
}: {
  /** The time the snapshot was saved, e.g. "2:14 PM". */
  label: string;
  /** True while the restore or discard is in flight. */
  busy: boolean;
  /** A failure that did not silently discard the snapshot. */
  error: string | null;
  onRestore: () => void;
  onDiscard: () => void;
}): JSX.Element {
  return (
    <div
      className="mx-auto w-full max-w-[1560px] px-5 pt-5 lg:px-8"
      role="status"
    >
      <StatusBanner tone="info">
        <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span>
            Vera found unsaved work from {label}. Restore it or discard it.
            {error ? ` ${error}` : ""}
          </span>
          <span className="flex items-center gap-2">
            <Button
              variant="primary"
              size="sm"
              onClick={onRestore}
              disabled={busy}
            >
              {busy ? "Restoring…" : "Restore"}
            </Button>
            <Button variant="ghost" size="sm" onClick={onDiscard} disabled={busy}>
              Discard
            </Button>
          </span>
        </span>
      </StatusBanner>
    </div>
  );
}
