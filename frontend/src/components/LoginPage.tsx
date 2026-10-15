import { useEffect, useState } from "react";
import { Login, RedeemInvite } from "@backend/backend/AuthAPI";
import { CheckForUpdates } from "@backend/backend/UpdateAPI";
import { backend } from "@backend/models";
import { BrowserOpenURL } from "../../wailsjs/runtime";
import { Button, Card, StatusBanner } from "./ui";
import { VeraLogo } from "./VeraLogo";

export interface SignedInUser {
  ID: number;
  Email: string;
  DisplayName: string;
  Role: string;
  AvatarURL?: string;
}

/** What the sign-in submit needs: the Login binding and the three state
 *  transitions the form's lifecycle owns. */
export interface SignInDeps {
  login: (email: string, password: string, remember: boolean) => Promise<SignedInUser>;
  onSignedIn: (user: SignedInUser) => void;
  setBusy: (busy: boolean) => void;
  setError: (message: string) => void;
}

/** The component's deps, with the real Login binding wired in — the one
 *  place the page calls it. Exported so the wiring is pinned rather than
 *  assumed: LoginPage.test.tsx mocks the binding and asserts this object
 *  carries it, which is what proves the third argument crosses to the Go
 *  method in the order it takes it. */
export function signInDeps(deps: {
  onSignedIn: (user: SignedInUser) => void;
  setBusy: (busy: boolean) => void;
  setError: (message: string) => void;
}): SignInDeps {
  return {
    login: Login,
    ...deps,
  };
}

/** The sign-in form's submit, extracted from the component so a test can
 *  drive it without a DOM (the house pattern — SettingsPage's saves and
 *  companySaves.ts; there are no DOM tests in this repo). The remember flag
 *  is the checkbox state, and it rides along as Login's third argument:
 *  false is the 8-hour, memory-only session; true asks the server for the
 *  30-day one, which the Go side then writes to the OS keychain. */
export async function submitSignIn(
  state: { busy: boolean; email: string; password: string; remember: boolean },
  deps: SignInDeps
): Promise<void> {
  if (state.busy) return;
  deps.setBusy(true);
  deps.setError("");
  try {
    deps.onSignedIn(await deps.login(state.email.trim(), state.password, state.remember));
  } catch (err: unknown) {
    deps.setError(typeof err === "string" ? err : "We couldn't sign you in. Try again.");
  } finally {
    deps.setBusy(false);
  }
}

const inputClass =
  "w-full rounded-md border border-rule bg-white px-3 py-2.5 text-body text-ink-900 outline-hidden transition placeholder:text-ink-300 hover:border-ink-300 focus:border-accent-400 focus:ring-2 focus:ring-accent-500/20";

function ShieldIcon() {
  return (
    <svg
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M12 3l7 3v5c0 4.5-3 8.5-7 10-4-1.5-7-5.5-7-10V6l7-3z" />
      <path d="M9.5 12l2 2 3.5-4" />
    </svg>
  );
}

function DocumentIcon() {
  return (
    <svg
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M6 3h8l4 4v14H6V3z" />
      <path d="M14 3v4h4" />
      <path d="M9 12h6M9 15.5h6" />
    </svg>
  );
}

function ArchiveIcon() {
  return (
    <svg
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="3" y="4" width="18" height="5" rx="1" />
      <path d="M5 9v10h14V9" />
      <path d="M10 13h4" />
    </svg>
  );
}

function EyeIcon({ off }: { off: boolean }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {off ? (
        <>
          <path d="M3 3l18 18" />
          <path d="M10.6 5.2A9.8 9.8 0 0 1 12 5c5 0 8.5 7 8.5 7a17 17 0 0 1-2.7 3.7M6.6 6.6A16.4 16.4 0 0 0 3.5 12S7 19 12 19c1.5 0 2.9-.5 4.1-1.2" />
        </>
      ) : (
        <>
          <path d="M3.5 12S7 5 12 5s8.5 7 8.5 7-3.5 7-8.5 7S3.5 12 3.5 12z" />
          <circle cx="12" cy="12" r="2.5" />
        </>
      )}
    </svg>
  );
}

export interface LoginUpdateNoticeProps {
  current: string;
  latest: string;
  downloadURL: string;
  notesURL: string;
  // Required is the Go side's verdict (backend/update.go: the running build is
  // below the server's minimum supported version), not a version comparison
  // repeated here. It is the state the login page must never understate: this
  // is the one surface a staff member locked out by a rollout ever sees.
  required: boolean;
}

// UpdateBanner's rule, verbatim: the installer the server names for this
// platform, the release notes otherwise. An unknown platform gets no URL, so
// the button points at the notes instead of a wrong binary.
export function updateLinkTarget(downloadURL: string, notesURL: string): string {
  return downloadURL || notesURL;
}

// What the login page shows for a resolved check, or null when nothing may
// appear: up to date means silent. Pure so the three states are pinned
// without a DOM (the mount effect never runs under renderToString).
export function loginUpdateNotice(
  info: backend.UpdateInfo
): LoginUpdateNoticeProps | null {
  if (!info.UpdateAvailable) return null;
  return {
    current: info.Current,
    latest: info.Latest,
    downloadURL: info.DownloadURL,
    notesURL: info.NotesURL,
    required: info.Required,
  };
}

// The mount check: fire-and-forget, and silent on every failure. A staff
// member locked out by a rollout must be able to sign in and see the fix, so
// the login screen must never show an error because the update check could
// not run, and nothing here may block or delay the form — a rejection
// resolves to null, the same as "no update".
export async function fetchLoginUpdateNotice(
  checkForUpdates: () => Promise<backend.UpdateInfo>
): Promise<LoginUpdateNoticeProps | null> {
  try {
    return loginUpdateNotice(await checkForUpdates());
  } catch {
    return null;
  }
}

// The sign-in page's update surface, above the form card. Never blocking and
// never an error: the version you are on, the version that fixes it, one
// action to fetch it. A build below the minimum supported version is the one
// state that must not read as a suggestion — UpdateBanner's wording and its
// danger tone are reused verbatim, because the login page is the only surface
// for the user who cannot sign in at all. Pure markup — no effects — so the
// snapshot harness can render it and the failure paths stay invisible.
export function LoginUpdateNotice({
  current,
  latest,
  downloadURL,
  notesURL,
  required,
}: LoginUpdateNoticeProps) {
  const target = updateLinkTarget(downloadURL, notesURL);
  const openDownload = () => {
    try {
      if (target) BrowserOpenURL(target);
    } catch {
      // Opening the browser must never break sign-in.
    }
  };
  return (
    <div className="mb-6" role="status">
      <StatusBanner tone={required ? "danger" : "info"}>
        <span className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span>
            {required ? (
              <>
                This Vera (v{current}) is below the minimum supported
                version. Please update to v{latest} to keep working.
              </>
            ) : (
              <>Vera v{latest} is available — you're on v{current}.</>
            )}
          </span>
          <Button
            variant={required ? "danger" : "primary"}
            size="sm"
            onClick={openDownload}
          >
            {downloadURL ? "Download update" : "See what's new"}
          </Button>
        </span>
      </StatusBanner>
    </div>
  );
}

export function LoginPage({ onSignedIn }: { onSignedIn: (user: SignedInUser) => void }) {
  const [mode, setMode] = useState<"signin" | "setup">("signin");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [capsOn, setCapsOn] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [code, setCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [activatedEmail, setActivatedEmail] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [remember, setRemember] = useState(false);
  const [updateNotice, setUpdateNotice] = useState<LoginUpdateNoticeProps | null>(null);

  // One check per mount, fire-and-forget: the form never awaits it and a
  // failure resolves to null, so a broken or unreachable update service
  // cannot delay, block, or put an error on the sign-in screen. The guard
  // drops the answer when the user signs in before it lands.
  useEffect(() => {
    let live = true;
    void fetchLoginUpdateNotice(CheckForUpdates).then((notice) => {
      if (live) setUpdateNotice(notice);
    });
    return () => {
      live = false;
    };
  }, []);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    await submitSignIn(
      { busy, email, password, remember },
      signInDeps({ onSignedIn, setBusy, setError })
    );
  };

  const switchMode = (next: "signin" | "setup") => {
    setMode(next);
    setError("");
    setActivatedEmail(null);
    setCopied(false);
  };

  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  const activate = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    const token = code.replace(/\s/g, "");
    if (!token) {
      setError("Enter the invite code your administrator shared.");
      return;
    }
    if (newPassword.length < 10) {
      setError("Choose a password of at least 10 characters.");
      return;
    }
    if (newPassword !== confirmPassword) {
      setError("The two passwords don't match.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const email = (await RedeemInvite(token, newPassword)) as string;
      setActivatedEmail(email);
      setCode("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (err: unknown) {
      const message = typeof err === "string" ? err : "";
      if (/invalid|expired/i.test(message)) {
        setError("That code is invalid or expired. Ask your administrator for a new one.");
      } else {
        setError(message || "We couldn't activate your account. Try again.");
      }
    } finally {
      setBusy(false);
    }
  };

  const trackCaps = (event: React.KeyboardEvent) => {
    setCapsOn(
      typeof event.getModifierState === "function" && event.getModifierState("CapsLock")
    );
  };

  return (
    <div className="flex min-h-full items-center justify-center px-5 py-12">
      <div className="w-full max-w-[420px]">
        <div className="mb-6 flex flex-col items-center text-center">
          <VeraLogo className="h-16 w-auto" />
          <p className="mt-3 text-display font-semibold tracking-[-0.03em] text-ink-950">
            Vera
          </p>
          <p className="mt-1 text-body text-ink-500">
            Quotation Generator · PT Arcon Penta Persada
          </p>
        </div>
        {mode === "signin" && updateNotice !== null ? (
          <LoginUpdateNotice {...updateNotice} />
        ) : null}
        <Card className="p-6">
          {mode === "signin" ? (
          <form onSubmit={submit} className="flex flex-col gap-4">
            {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
            <label className="flex flex-col gap-1.5">
              <span className="text-caption font-medium text-ink-600">Email</span>
              <input
                type="email"
                autoComplete="username"
                autoFocus
                required
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="you@company.com"
                className={inputClass}
              />
            </label>
            <label className="flex flex-col gap-1.5">
              <span className="text-caption font-medium text-ink-600">Password</span>
              <span className="relative block">
                <input
                  type={showPassword ? "text" : "password"}
                  autoComplete="current-password"
                  required
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  onKeyDown={trackCaps}
                  onKeyUp={trackCaps}
                  placeholder="••••••••"
                  className={`${inputClass} pr-11`}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((visible) => !visible)}
                  aria-label={showPassword ? "Hide password" : "Show password"}
                  aria-pressed={showPassword}
                  tabIndex={-1}
                  className="absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-r-md text-ink-400 transition hover:text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                >
                  <EyeIcon off={!showPassword} />
                </button>
              </span>
              {capsOn ? (
                <span className="text-caption text-ink-500">Caps Lock is on.</span>
              ) : null}
            </label>
            {/* The remember-me choice (issue #434). Unchecked by default: a
                shared machine stays on the 8-hour session unless someone
                asks for the 30-day one out loud. The Go side keeps that
                choice — memory only when unchecked, the OS keychain when
                checked — so this box is the whole decision. Sign-in mode
                only; activation and password reset sign nobody in. */}
            <label className="flex items-start gap-2.5">
              <input
                type="checkbox"
                checked={remember}
                onChange={(event) => setRemember(event.target.checked)}
                className="mt-0.5 h-4 w-4 shrink-0 rounded-sm border-rule accent-accent-600"
              />
              <span className="flex flex-col gap-0.5">
                <span className="text-caption font-medium text-ink-600">
                  Keep me signed in on this device
                </span>
                <span className="text-caption text-ink-500">
                  Stays signed in for 30 days. On a shared device, leave this
                  unchecked.
                </span>
              </span>
            </label>
            <Button type="submit" variant="dark" disabled={busy} className="mt-1 w-full">
              {busy ? "Signing in…" : "Sign in"}
            </Button>
            <p className="text-center text-caption text-ink-500">
              First time here?{" "}
              <button
                type="button"
                onClick={() => switchMode("setup")}
                className="font-medium text-ink-900 underline underline-offset-2 hover:text-ink-700"
              >
                Activate with an invite code
              </button>
            </p>
          </form>
          ) : activatedEmail !== null ? (
          <div className="flex flex-col gap-4">
            <StatusBanner tone="info">
              Your account is activated. Sign in with your new password.
            </StatusBanner>
            {activatedEmail ? (
              <div className="flex items-center justify-between gap-3 rounded-md border border-rule bg-paper-subtle px-3 py-2.5">
                <span className="truncate text-body text-ink-900">{activatedEmail}</span>
                <button
                  type="button"
                  onClick={() => void copyText(activatedEmail)}
                  className="shrink-0 rounded-md px-2 py-1 text-caption font-medium text-ink-600 transition hover:bg-paper-muted hover:text-ink-900 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                >
                  {copied ? "Copied" : "Copy"}
                </button>
              </div>
            ) : null}
            <Button variant="dark" onClick={() => switchMode("signin")} className="w-full">
              Finish — back to sign in
            </Button>
          </div>
          ) : (
          <form onSubmit={activate} className="flex flex-col gap-4">
            {error ? <StatusBanner tone="danger">{error}</StatusBanner> : null}
            <label className="flex flex-col gap-1.5">
              <span className="text-caption font-medium text-ink-600">Invite code</span>
              <input
                type="text"
                autoComplete="off"
                autoFocus
                required
                value={code}
                onChange={(event) => setCode(event.target.value)}
                placeholder="Paste the code from your administrator"
                className={inputClass}
              />
            </label>
            <label className="flex flex-col gap-1.5">
              <span className="text-caption font-medium text-ink-600">New password</span>
              <span className="relative block">
                <input
                  type={showPassword ? "text" : "password"}
                  autoComplete="new-password"
                  required
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                  onKeyDown={trackCaps}
                  onKeyUp={trackCaps}
                  placeholder="At least 10 characters"
                  className={`${inputClass} pr-11`}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((visible) => !visible)}
                  aria-label={showPassword ? "Hide password" : "Show password"}
                  aria-pressed={showPassword}
                  tabIndex={-1}
                  className="absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-r-md text-ink-400 transition hover:text-ink-700 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-accent-500/40"
                >
                  <EyeIcon off={!showPassword} />
                </button>
              </span>
              {capsOn ? (
                <span className="text-caption text-ink-500">Caps Lock is on.</span>
              ) : null}
            </label>
            <label className="flex flex-col gap-1.5">
              <span className="text-caption font-medium text-ink-600">Confirm password</span>
              <input
                type={showPassword ? "text" : "password"}
                autoComplete="new-password"
                required
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                placeholder="Repeat the new password"
                className={inputClass}
              />
            </label>
            <Button type="submit" variant="dark" disabled={busy} className="mt-1 w-full">
              {busy ? "Activating…" : "Activate account"}
            </Button>
            <p className="text-center text-caption text-ink-500">
              Already activated?{" "}
              <button
                type="button"
                onClick={() => switchMode("signin")}
                className="font-medium text-ink-900 underline underline-offset-2 hover:text-ink-700"
              >
                Back to sign in
              </button>
            </p>
          </form>
          )}
        </Card>
        <ul className="mt-6 flex flex-col gap-2.5 border-t border-rule pt-5">
          <li className="flex items-center justify-center gap-2 text-caption text-ink-500">
            <ShieldIcon />
            Admin-managed accounts, one person per device
          </li>
          <li className="flex items-center justify-center gap-2 text-caption text-ink-500">
            <DocumentIcon />
            Margins with FTZ and non-FTZ tax computed inside
          </li>
          <li className="flex items-center justify-center gap-2 text-caption text-ink-500">
            <ArchiveIcon />
            Every export kept as a numbered revision
          </li>
        </ul>
        <p className="mt-4 text-center text-caption leading-relaxed text-ink-400">
          Contact your administrator if you cannot sign in.
        </p>
      </div>
    </div>
  );
}
