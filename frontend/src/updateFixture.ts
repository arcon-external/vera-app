import type { backend } from "@backend/models";

/**
 * The live shape of GET /v1/app/latest, as `backend/update.go` serves it.
 *
 * Ticket #558: the release contract and the Go build stamp both carry the `v`
 * prefix (`v0.15.0`). Fixtures used to be written by hand as bare `0.x`
 * values, so the doubled-`v` regression (#546 shipped "Vera vv0.15.0") passed
 * every test: no fixture ever looked like the live contract. The login-page
 * test, the in-app banner test and the snapshot harness all read their
 * versions from here, so a fixture that drifts from the served shape is one
 * edit away from being caught.
 *
 * `UpdateAvailable` and `Required` are the Go side's verdicts, not comparisons
 * this module makes — the frontend consumes that decision (backend/update.go).
 */
export const UPDATE_NEWER: backend.UpdateInfo = {
  Current: "v0.12.1",
  Latest: "v0.13.1",
  NotesURL: "https://api.arconpentapersada.co.id/v1/app/changelog",
  DownloadURL:
    "https://vera-downloads-508411.storage.googleapis.com/Vera-v0.13.1-windows-amd64-installer.exe",
  UpdateAvailable: true,
  Required: false,
};

/** The same served check when the running build is below the server's
 *  minimum supported version — the state the login page must never
 *  understate. */
export const UPDATE_REQUIRED: backend.UpdateInfo = {
  ...UPDATE_NEWER,
  Required: true,
};
