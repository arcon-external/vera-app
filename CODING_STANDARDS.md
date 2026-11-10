# Coding standards

The judgement calls a reviewer owns when a test is green but its claim is still
open. Mechanical checks — formatting, vet, the type-check, the suite — live in
`AGENTS.md` §13 and in the tests themselves. These three rules cover the places
where a passing test can still prove the wrong thing. This is a review
reference, not an implementation guide.

## 1. A layout test asserts the user-visible relationship

Assert the relationship a reader sees, between named things, not a bound that
merely permits it.

- The quotation note's claim is that its bullet and number markers share one
  column: a bullet disc and a numbered marker start at the same x, so the two
  list types do not read as differently indented.
  `TestNoteListMarkersShareOneColumn`
  (`backend/render/gpdf/note_test.go`) measures the disc and the `1.` run and
  asserts their x values agree within 0.01pt.
- "Indented past the edge" is a different, weaker claim.
  `TestNoteRendersNumberedRows` only asserts a marker sits left of its text
  (`r.x >= textX` is the failure), which a marker 12pt right of the bullet
  column still passes. That test is honest about what it proves; a test that
  claims alignment must measure both markers.
- This is the layout form of `AGENTS.md` §12.5: move one marker and watch the
  assertion go red before restoring it. A layout test that cannot fail is
  decoration.

## 2. An editor test uses the shared contenteditable DOM fixtures

The editor's input is the DOM the browser's toolbar leaves behind. Assert the
note source serialized from that DOM, not the call the editor made.

- A stubbed `document.execCommand` proves the button asked the browser to run a
  command; it proves nothing about the DOM that follows. Keep the stub for the
  toolbar wiring in `frontend/src/components/NoteEditor.dom.test.tsx`, but never
  let it stand in for a serialization claim.
- The shapes are shared, not retyped per test: a top-level list, a list inside a
  contenteditable block, text before and after a list, and the empty list. They
  live in `frontend/src/noteFixtures.ts` (`NOTE_DOM_FIXTURES`, #559), and each
  fixture carries the literal source it must serialize to.
  `frontend/src/note.test.ts` runs the serializer over every shape;
  `frontend/src/components/NoteEditor.dom.test.tsx` feeds the same shape
  through the mounted editor.
- A one-off `innerHTML` string written for a single test is a DOM the toolbar
  never produces, so it can pass while the real toolbar shape flattens (#550).

## 3. A contract fixture uses the live shape

A fixture easier to write than the wire is a fixture that cannot catch a wire
regression.

- Versions are `v`-prefixed end to end: the release contract serves `vX.Y.Z`
  and the Go build stamps `vX.Y.Z` (`backend/version.go` parses either, but the
  live values carry the prefix). Fixtures therefore carry `Current: "v0.12.1"`,
  `Latest: "v0.13.1"`; a bare `0.12.1` fixture cannot fail the doubled-`v`
  regression that shipped in #546 and was fixed in #547.
- Field names and casing are the DTO's: `backend.UpdateInfo` is `Current`,
  `Latest`, `NotesURL`, `DownloadURL`, `UpdateAvailable`, `Required`, and
  `frontend/wailsjs/go/models.ts` mirrors it. A fixture with renamed or
  lowercased fields tests a contract nobody serves.
- One fixture, every consumer: `frontend/src/updateFixture.ts` (`UPDATE_NEWER`,
  `UPDATE_REQUIRED`, #558) is read by
  `frontend/src/components/LoginPage.test.tsx` and
  `frontend/src/ui-snapshot.harness.test.tsx`, and the in-app banner consumes
  the same DTO. A shape change breaks every consumer at once.
