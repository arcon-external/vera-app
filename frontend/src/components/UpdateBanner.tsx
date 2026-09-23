import { useCallback, useEffect, useState } from "react";
import { CheckForUpdates } from "@backend/backend/UpdateAPI";
import { backend } from "@backend/models";
import { BrowserOpenURL } from "../../wailsjs/runtime";
import { useToast } from "../toast";
import { Button, StatusBanner } from "./ui";

type UpdateState = backend.UpdateInfo & { checked: boolean; offline: boolean };

const DISMISS_PREFIX = "vera-update-dismissed-";
const STATE_KEY = "vera-update-state";

function readCache(): UpdateState | null {
  try {
    const raw = window.localStorage.getItem(STATE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as UpdateState;
    if (typeof parsed.Latest !== "string") return null;
    return { ...parsed, checked: true, offline: true };
  } catch {
    return null;
  }
}

function isDismissed(latest: string): boolean {
  if (!latest) return true;
  try {
    return window.localStorage.getItem(DISMISS_PREFIX + latest) === "1";
  } catch {
    return false;
  }
}

// useUpdateState checks once per launch (after sign-in) and caches the
// result, so offline launches still show a stale-but-honest state instead
// of an error. Failures are silent here by design: the check must never
// block startup, break offline use, or alias into a sign-out.
export function useUpdateState(signedIn: boolean): {
  state: UpdateState | null;
  recheck: () => Promise<void>;
  dismiss: () => void;
} {
  const [state, setState] = useState<UpdateState | null>(null);
  const { pushToast } = useToast();

  const recheck = useCallback(async () => {
    try {
      const info = (await CheckForUpdates()) as backend.UpdateInfo;
      const next: UpdateState = { ...info, checked: true, offline: false };
      setState(next);
      try {
        window.localStorage.setItem(STATE_KEY, JSON.stringify(next));
      } catch {
        // Cache is best-effort.
      }
    } catch (err: unknown) {
      const message = typeof err === "string" && err.trim() ? err : "";
      // Offline or unreachable server: fall back to cache, stay silent.
      // Anything else surfaces once so a broken check is visible.
      if (/reach|network|fetch|failed|econn|timeout/i.test(message)) {
        setState((current) => current ?? readCache());
      } else {
        pushToast("error", "We couldn't check for updates.");
      }
    }
  }, [pushToast]);

  useEffect(() => {
    if (signedIn) void recheck();
    else setState(null);
  }, [signedIn, recheck]);

  const dismiss = useCallback(() => {
    setState((current) => {
      if (current?.Latest) {
        try {
          window.localStorage.setItem(DISMISS_PREFIX + current.Latest, "1");
        } catch {
          // Dismissal is best-effort.
        }
      }
      return null;
    });
  }, []);

  return { state, recheck, dismiss };
}

// UpdateBanner renders above the shell on every page while an update is
// pending. Never modal, never blocking: a quotation tool must not stand
// between the user and their deadline.
export function UpdateBanner({
  state,
  onDismiss,
}: {
  state: UpdateState;
  onDismiss: () => void;
}) {
  if (!state.UpdateAvailable || isDismissed(state.Latest)) return null;
  const openDownload = () => {
    try {
      const target = state.DownloadURL || state.NotesURL;
      if (target) BrowserOpenURL(target);
    } catch {
      // Opening the browser must never break the app.
    }
  };
  return (
    <div className="mx-auto w-full max-w-[1560px] px-5 pt-5 lg:px-8" role="status">
      <StatusBanner tone={state.Required ? "danger" : "info"}>
        <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span>
            {state.Required ? (
              <>
                This Vera ({state.Current || "dev"}) is below the minimum
                supported version. Please update to {state.Latest} to keep
                working.
              </>
            ) : (
              <>
                Vera {state.Latest} is available (you have{" "}
                {state.Current || "dev"}).
              </>
            )}{" "}
            {state.offline ? "Last checked while offline. " : ""}
            {state.DownloadURL
              ? "Download the installer for your system — it handles the rest."
              : "See the release notes to download it."}
          </span>
          <span className="flex items-center gap-2">
            <Button
              variant={state.Required ? "danger" : "primary"}
              size="sm"
              onClick={openDownload}
            >
              {state.DownloadURL ? "Download update" : "See what's new"}
            </Button>
            {!state.Required ? (
              <Button variant="ghost" size="sm" onClick={onDismiss}>
                Dismiss
              </Button>
            ) : null}
          </span>
        </span>
      </StatusBanner>
    </div>
  );
}
