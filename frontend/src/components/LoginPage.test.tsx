import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { backend } from "@backend/models";
import { Login } from "@backend/backend/AuthAPI";
import {
  LoginPage,
  LoginUpdateNotice,
  fetchLoginUpdateNotice,
  loginUpdateNotice,
  signInDeps,
  submitSignIn,
  updateLinkTarget,
} from "./LoginPage";
import type { SignedInUser } from "./LoginPage";

// Issues #436 and #434: the sign-in page's update notice, and the
// keep-me-signed-in checkbox. The suite runs in node with no DOM (the house
// pattern — SettingsPage.test.tsx), so each surface is pinned as markup
// through renderToString and the behaviour behind it is pinned as the pure
// functions the component delegates to, driven with the bindings mocked:
// CheckForUpdates answers for a newer build, for an up-to-date build, and with
// a rejection.
//
// What this test does NOT prove, stated so it is not assumed: effects never
// run under renderToString, so the effect itself — one call per mount, the
// answer dropped when the user signs in first — is hand-driven; so is the
// click that hands the URL to the browser (updateLinkTarget pins which URL
// that click uses, and BrowserOpenURL is the same call UpdateBanner makes).

// The shape GET /v1/app/latest answers for a newer build. The Go side has
// already resolved this platform's installer and decided availability
// (backend/update.go): the frontend consumes that decision, it never repeats it.
const NEWER: backend.UpdateInfo = {
  Current: "0.12.1",
  Latest: "0.13.1",
  NotesURL: "https://api.arconpentapersada.co.id/v1/app/changelog",
  DownloadURL:
    "https://vera-downloads-508411.storage.googleapis.com/Vera-v0.13.1-windows-amd64-installer.exe",
  UpdateAvailable: true,
  Required: false,
};

const UPDATED = {
  current: "0.12.1",
  latest: "0.13.1",
  downloadURL: NEWER.DownloadURL,
  notesURL: NEWER.NotesURL,
  required: false,
};

// The same served check with the build below the server's minimum supported
// version (backend/update.go: Required is the Go side's verdict).
const REQUIRED: backend.UpdateInfo = { ...NEWER, Required: true };

// The bindings, mocked: LoginPage imports CheckForUpdates for the mount
// effect and Login/RedeemInvite for the sign-in submit, and a test that
// imports the page must supply every export it imports.
const mocked = vi.hoisted(() => ({
  CheckForUpdates: vi.fn(),
  Login: vi.fn(),
  RedeemInvite: vi.fn(),
}));

vi.mock("@backend/backend/UpdateAPI", () => ({
  CheckForUpdates: mocked.CheckForUpdates,
}));

vi.mock("@backend/backend/AuthAPI", () => ({
  Login: mocked.Login,
  RedeemInvite: mocked.RedeemInvite,
}));

function renderNotice(props: {
  current: string;
  latest: string;
  downloadURL: string;
  notesURL: string;
  required: boolean;
}): string {
  return renderToString(<LoginUpdateNotice {...props} />);
}

describe("loginUpdateNotice resolves the served check", () => {
  it("turns a newer build into the notice's props", () => {
    expect(loginUpdateNotice(NEWER)).toEqual(UPDATED);
  });

  it("is null when the running build is up to date", () => {
    expect(
      loginUpdateNotice({ ...NEWER, Latest: "0.12.1", UpdateAvailable: false })
    ).toBeNull();
  });

  it("never re-decides availability: the Go side's UpdateAvailable is the only input", () => {
    // backend/update.go already compared the versions and resolved the
    // installer URL. A frontend that compared them again would disagree the
    // moment the server's rule changed (prereleases, minimum-supported).
    expect(
      loginUpdateNotice({ ...NEWER, Current: "0.13.1", UpdateAvailable: true })
    ).toEqual({ ...UPDATED, current: "0.13.1" });
    expect(
      loginUpdateNotice({ ...NEWER, Current: "0.13.1", UpdateAvailable: false })
    ).toBeNull();
  });
});

describe("fetchLoginUpdateNotice is the silent mount check", () => {
  it("resolves the notice when the check answers", async () => {
    await expect(fetchLoginUpdateNotice(async () => NEWER)).resolves.toEqual(UPDATED);
  });

  it("resolves null when the check resolves without an update", async () => {
    await expect(
      fetchLoginUpdateNotice(async () => ({ ...NEWER, UpdateAvailable: false }))
    ).resolves.toBeNull();
  });

  it("resolves null — silent — when the check rejects", async () => {
    // The rollout lock-out this issue exists for: the update service is
    // unreachable, and the sign-in screen must show no error for it.
    const check = vi.fn(async () => {
      throw new Error("reach the update service: dial tcp 10.0.0.1:443: connect: connection refused");
    });
    await expect(fetchLoginUpdateNotice(check)).resolves.toBeNull();
    expect(check).toHaveBeenCalledTimes(1);
  });
});

describe("LoginUpdateNotice markup", () => {
  it("prints both versions and a Download update button beside the copy", () => {
    // The <!-- --> separators are React's own, between the text and each
    // interpolation; the sentence they render reads
    // "Vera v0.13.1 is available — you're on v0.12.1."
    const html = renderNotice(loginUpdateNotice(NEWER)!);
    expect(html).toContain(
      "Vera v<!-- -->0.13.1<!-- --> is available — you&#x27;re on v<!-- -->0.12.1<!-- -->."
    );
    expect(html).toContain(">Download update</button>");
    expect(html).toContain('role="status"');
  });

  it("states the lock-out as danger, in UpdateBanner's words, when the build is below the minimum supported version", () => {
    // The Judge's finding on #436: the login page is the only surface for a
    // staff member who cannot sign in, so the unsupported-build state must
    // not read as the mild "is available" suggestion. Tone, wording and the
    // Download button all come with it — the button is the way out, so it
    // stays in both states.
    //
    // Rendered through loginUpdateNotice, not hand-written props: the plumbing
    // (Required → required) is what makes this red if it is dropped again.
    const html = renderNotice(loginUpdateNotice(REQUIRED)!);
    expect(html).toContain('role="alert"');
    expect(html).toContain("bg-danger-50 text-danger-700");
    expect(html).toContain(
      "This Vera (v<!-- -->0.12.1<!-- -->) is below the minimum supported version. Please update to v<!-- -->0.13.1<!-- --> to keep working."
    );
    expect(html).toContain(">Download update</button>");
  });

  it("falls back to the release notes when the server names no installer", () => {
    // An unknown platform gets no DownloadURL: the button reads as what it
    // does and aims at the notes URL.
    const html = renderNotice({
      current: "0.12.1",
      latest: "0.13.1",
      downloadURL: "",
      notesURL: NEWER.NotesURL,
      required: false,
    });
    expect(html).toContain(">See what&#x27;s new</button>");
    expect(updateLinkTarget("", NEWER.NotesURL)).toBe(NEWER.NotesURL);
    expect(updateLinkTarget(NEWER.DownloadURL, NEWER.NotesURL)).toBe(NEWER.DownloadURL);
  });

  it("keeps the copy and the button on one wrapped row, never a wide fixed line", () => {
    // The 360px login column: the copy must wrap, so the row is flex-wrap.
    const html = renderNotice(loginUpdateNotice(NEWER)!);
    expect(html).toContain("flex flex-wrap items-center gap-x-3 gap-y-2");
  });
});

// Issue #434: the keep-me-signed-in checkbox. The suite runs in node with no
// DOM (the house pattern — there are no DOM tests in this repo), so the
// checkbox's default and its copy are pinned as markup through
// renderToString, and the choice's journey to the server is pinned through
// the two functions the component's submit delegates to: signInDeps carries
// the real Login binding, and submitSignIn passes the flag as that binding's
// third argument.
//
// What this test does NOT prove, stated so it is not assumed: effects never
// run and renderToString fires no events, so the click that submits the
// form — the component's own submit wiring the checkbox state into that
// third argument — stays hand-driven; so does checking the box.

// The answer the binding gives for a good sign-in, shaped as the Go DTO the
// desktop hands on.
const SIGNED_IN: SignedInUser = {
  ID: 7,
  Email: "staff@example.com",
  DisplayName: "Staff",
  Role: "staff",
};

function signInSpies() {
  return {
    onSignedIn: vi.fn(),
    setBusy: vi.fn(),
    setError: vi.fn(),
  };
}

describe("the keep-me-signed-in checkbox", () => {
  it("sits between the password field and the submit button, and defaults to unchecked", () => {
    const html = renderToString(<LoginPage onSignedIn={() => undefined} />);
    expect(html).toContain('type="checkbox"');
    // React omits the attribute when checked is false, so its absence is the
    // default: a shared machine gets the 8-hour session unless someone asks.
    expect(html).not.toContain('checked=""');
    const password = html.indexOf('type="password"');
    const checkbox = html.indexOf('type="checkbox"');
    const submit = html.indexOf(">Sign in</button>");
    expect(password).toBeGreaterThan(-1);
    expect(checkbox).toBeGreaterThan(password);
    expect(checkbox).toBeLessThan(submit);
  });

  it("prints the frozen copy, the helper in the caption ink", () => {
    const html = renderToString(<LoginPage onSignedIn={() => undefined} />);
    expect(html).toContain("Keep me signed in on this device");
    expect(html).toContain(
      "Stays signed in for 30 days. On a shared device, leave this unchecked."
    );
    // The helper's classes, pinned exactly as the issue froze them: a class
    // that compiles to nothing still looks right in source, so the built CSS
    // is what proves text-caption and text-ink-500 exist.
    expect(html).toContain('class="text-caption text-ink-500"');
  });

  it("names the choice in the sign-in form only — the password field it follows exists only there", () => {
    // The checkbox lives inside the sign-in branch, after the password
    // label; setup mode renders no password field and no checkbox. The
    // ordering assertion above is what holds this true.
    const html = renderToString(<LoginPage onSignedIn={() => undefined} />);
    expect(html).toContain("Activate with an invite code");
    expect(html.split('type="checkbox"').length - 1).toBe(1);
  });
});

describe("submitSignIn passes the checkbox state as Login's third argument", () => {
  beforeEach(() => {
    mocked.Login.mockReset();
    mocked.Login.mockResolvedValue(SIGNED_IN);
  });

  it("sends false when the box is unchecked", async () => {
    const spies = signInSpies();
    await submitSignIn(
      { busy: false, email: "  staff@example.com  ", password: "correct horse", remember: false },
      { login: mocked.Login, ...spies }
    );
    expect(mocked.Login).toHaveBeenCalledWith("staff@example.com", "correct horse", false);
    expect(spies.onSignedIn).toHaveBeenCalledWith(SIGNED_IN);
  });

  it("sends true when the box is checked", async () => {
    const spies = signInSpies();
    await submitSignIn(
      { busy: false, email: "staff@example.com", password: "correct horse", remember: true },
      { login: mocked.Login, ...spies }
    );
    expect(mocked.Login).toHaveBeenCalledWith("staff@example.com", "correct horse", true);
    expect(spies.onSignedIn).toHaveBeenCalledWith(SIGNED_IN);
  });

  it("drives the busy and error state around the call", async () => {
    const spies = signInSpies();
    await submitSignIn(
      { busy: false, email: "staff@example.com", password: "correct horse", remember: false },
      { login: mocked.Login, ...spies }
    );
    expect(spies.setBusy.mock.calls).toEqual([[true], [false]]);
    expect(spies.setError).toHaveBeenCalledTimes(1);
    expect(spies.setError).toHaveBeenCalledWith("");
  });

  it("does nothing while a sign-in is already running", async () => {
    const spies = signInSpies();
    await submitSignIn(
      { busy: true, email: "staff@example.com", password: "correct horse", remember: true },
      { login: mocked.Login, ...spies }
    );
    expect(mocked.Login).not.toHaveBeenCalled();
    expect(spies.setBusy).not.toHaveBeenCalled();
    expect(spies.onSignedIn).not.toHaveBeenCalled();
  });

  it("puts a server refusal string on the banner and a generic sentence for anything else", async () => {
    mocked.Login.mockRejectedValue("invalid credentials");
    const refused = signInSpies();
    await submitSignIn(
      { busy: false, email: "staff@example.com", password: "wrong", remember: true },
      { login: mocked.Login, ...refused }
    );
    expect(refused.setError).toHaveBeenCalledWith("invalid credentials");
    expect(refused.onSignedIn).not.toHaveBeenCalled();
    // The button always comes back: a failed sign-in must not wedge the form.
    expect(refused.setBusy).toHaveBeenLastCalledWith(false);

    mocked.Login.mockRejectedValue(new Error("dial tcp 10.0.0.1:443: connection refused"));
    const broken = signInSpies();
    await submitSignIn(
      { busy: false, email: "staff@example.com", password: "x", remember: false },
      { login: mocked.Login, ...broken }
    );
    expect(broken.setError).toHaveBeenCalledWith("We couldn't sign you in. Try again.");
    expect(broken.onSignedIn).not.toHaveBeenCalled();
  });
});

describe("signInDeps wires the real Login binding", () => {
  it("hands the binding over unchanged, so the third argument is the Go method's third argument", () => {
    const deps = signInDeps({
      onSignedIn: () => undefined,
      setBusy: () => undefined,
      setError: () => undefined,
    });
    expect(deps.login).toBe(Login);
    // The module is mocked in this suite, so "the real binding" is the mock:
    // if the component stopped calling Login here, this fails.
    expect(deps.login).toBe(mocked.Login);
  });
});
