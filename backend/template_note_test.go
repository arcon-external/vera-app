package backend

// Guards for the printed quotation note (#534).
//
// The note is stored, signed and carried by #533; this file covers the print
// layer only. What it pins is external behaviour: the customer document carries
// exactly one note card, in the items block between the last line row and the
// terms, as escaped plain text with its line breaks intact, and an empty note
// leaves the document exactly as it was.

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/labels"
)

// prepareWithNote prepares a one-line quotation carrying the given note and
// label set, the way a signed-in desktop or a server-side Prepare would.
func prepareWithNote(t *testing.T, note string, set labels.DocumentLabels) *Quotation {
	t.Helper()
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	identity := DefaultCompanyIdentity()
	identity.Labels = set
	q.companyIdentity = &identity
	if err := q.Prepare(noteInput(note)); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return q
}

// TestPreviewPrintsOneNoteCardUnderTheItems is the shape guard: one card, not
// one per line item, placed after the items table and before the terms.
func TestPreviewPrintsOneNoteCardUnderTheItems(t *testing.T) {
	const note = "Delivery is ex-works.\nInstallation by the customer."
	q := prepareWithNote(t, note, labels.DefaultLabels())

	html, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	if got := strings.Count(html, `class="note-card"`); got != 1 {
		t.Errorf("the document prints %d note cards; want exactly one", got)
	}
	if !strings.Contains(html, `<h3 class="note-label">Note</h3>`) {
		t.Error("the note card does not carry the built-in Note label")
	}
	if !strings.Contains(html, note) {
		t.Errorf("the note's line break was not preserved; the document does not carry %q", note)
	}

	footer := strings.Index(html, `<section class="footer">`)
	if footer < 0 {
		t.Fatal("the document carries no footer section")
	}
	noteAt := strings.Index(html, `class="note-card"`)
	lastTable := strings.LastIndex(html[:footer], "</table>")
	if lastTable < 0 {
		t.Fatal("the document carries no items table before the footer")
	}
	if noteAt < lastTable {
		t.Error("the note card prints before the items table; it belongs below the last line row")
	}
	if noteAt > footer {
		t.Error("the note card prints after the terms and totals; it belongs in the items block")
	}
}

// TestPreviewEscapesTheNoteAsPlainText proves pasted markup cannot alter the
// document: the note is data, escaped exactly like any other template value.
func TestPreviewEscapesTheNoteAsPlainText(t *testing.T) {
	const note = `Delivery <script>alert("x")</script> & <b>bold</b>.`
	q := prepareWithNote(t, note, labels.DefaultLabels())

	html, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	for _, raw := range []string{"<script>", "</script>", "<b>bold</b>"} {
		if strings.Contains(html, raw) {
			t.Errorf("the note printed raw markup %q; it must be escaped as plain text", raw)
		}
	}
	want := `Delivery &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt; &amp; &lt;b&gt;bold&lt;/b&gt;.`
	if !strings.Contains(html, want) {
		t.Errorf("the escaped note is not in the document; want %q", want)
	}
}

// TestPreviewOmitsTheNoteCardWhenEmpty is the "most quotations are unchanged"
// rule: no note, no card, and a whitespace-only note resolves to the same
// document as an absent one.
func TestPreviewOmitsTheNoteCardWhenEmpty(t *testing.T) {
	empty := prepareWithNote(t, "", labels.DefaultLabels())
	spaces := prepareWithNote(t, " \n\t ", labels.DefaultLabels())

	emptyHTML, err := empty.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("empty preview: %v", err)
	}
	spacesHTML, err := spaces.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("whitespace preview: %v", err)
	}

	for _, marker := range []string{`class="note-card"`, `class="note-label"`} {
		if strings.Contains(emptyHTML, marker) {
			t.Errorf("an empty note still printed %q", marker)
		}
	}
	if emptyHTML != spacesHTML {
		t.Error("a whitespace-only note changed the document; it must print nothing, like an empty note")
	}
}

// TestNoteLabelFollowsTheWording proves the card's heading is editable through
// the same label set as the rest of the customer-visible wording.
func TestNoteLabelFollowsTheWording(t *testing.T) {
	set := labels.DefaultLabels()
	set.Note = "Catatan"
	q := prepareWithNote(t, "Kirim setelah pukul 09.00.", set)

	html, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(html, `<h3 class="note-label">Catatan</h3>`) {
		t.Error("the reworded note label did not reach the document")
	}
	if strings.Contains(html, `<h3 class="note-label">Note</h3>`) {
		t.Error("the document still prints the built-in Note label beside the reworded one")
	}
}

// TestNoteCardStylesheetPreservesLineBreaksAndBreaks pins the two stylesheet
// declarations the printed note depends on: pre-wrap so a typed line break
// prints, and break-inside: avoid so the card is atomic across a page break.
// The oracle test only runs with a browser, so this guard fails in a plain
// `go test` when either declaration is removed.
func TestNoteCardStylesheetPreservesLineBreaksAndBreaks(t *testing.T) {
	raw, err := os.ReadFile("template/css/template-fixed.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	css := string(raw)

	body := regexp.MustCompile(`(?s)\.note-card\s+\.note-body\s*\{([^}]*)\}`).FindStringSubmatch(css)
	if body == nil {
		t.Fatal("no `.note-card .note-body { ... }` rule in the stylesheet")
	}
	if !strings.Contains(body[1], "white-space: pre-wrap") {
		t.Error("the note body does not preserve line breaks; add white-space: pre-wrap")
	}

	card := regexp.MustCompile(`(?s)\.note-card\s*\{([^}]*)\}`).FindStringSubmatch(css)
	if card == nil {
		t.Fatal("no `.note-card { ... }` rule in the stylesheet")
	}
	for _, decl := range []string{
		"break-inside: avoid",
		"page-break-inside: avoid",
		"border-left: 3px solid var(--accent-blue)",
	} {
		if !strings.Contains(card[1], decl) {
			t.Errorf("the note card rule is missing %q", decl)
		}
	}
}

// TestRenderInputCarriesTheNote pins the renderer seam: the note the template
// prints is the same note the native renderer is handed.
func TestRenderInputCarriesTheNote(t *testing.T) {
	const note = "Delivery is ex-works.\nInstallation by the customer."
	q := prepareWithNote(t, note, labels.DefaultLabels())

	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("render input: %v", err)
	}
	if in.Note != note {
		t.Errorf("seam note = %q; want %q", in.Note, note)
	}
}
