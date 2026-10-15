import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { backend } from "@backend/models";
import {
  LoginUpdateNotice,
  fetchLoginUpdateNotice,
  loginUpdateNotice,
  updateLinkTarget,
} from "./LoginPage";

// Issue #436: the sign-in page's update notice. The suite runs in node with no
// DOM (the house pattern — SettingsPage.test.tsx), so the notice is pinned as
// markup through renderToString and the mount check is pinned as the pure
// functions the effect delegates to, driven with the binding mocked:
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
};

// The binding, mocked: LoginPage imports it for the mount effect, and a test
// that imports the page must supply every export it imports.
const mocked = vi.hoisted(() => ({ CheckForUpdates: vi.fn() }));

vi.mock("@backend/backend/UpdateAPI", () => ({
  CheckForUpdates: mocked.CheckForUpdates,
}));

function renderNotice(props: {
  current: string;
  latest: string;
  downloadURL: string;
  notesURL: string;
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

  it("falls back to the release notes when the server names no installer", () => {
    // An unknown platform gets no DownloadURL: the button reads as what it
    // does and aims at the notes URL.
    const html = renderNotice({
      current: "0.12.1",
      latest: "0.13.1",
      downloadURL: "",
      notesURL: NEWER.NotesURL,
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
