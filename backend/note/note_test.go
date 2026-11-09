package note

// Guards for the restricted Markdown subset a quotation note may carry (#540).
//
// The note is one signed string. The parser is the single place that decides
// what the source means, so the HTML card and the native renderer cannot
// disagree about the subset. What it pins is external behaviour: the rendered
// HTML, the escaped text, and the block/line structure both renderers consume.

import (
	"strings"
	"testing"
)

// TestPlainTextKeepsItsLineBreaks is the compatibility rule: a note written
// before the rich-text editor existed must still print every line, with no
// markup interpretation.
func TestPlainTextKeepsItsLineBreaks(t *testing.T) {
	doc := Parse("Delivery is ex-works.\nInstallation by the customer.")
	if len(doc.Blocks) != 1 {
		t.Fatalf("plain note parsed into %d blocks; want 1", len(doc.Blocks))
	}
	if got := doc.Blocks[0].Kind; got != Paragraph {
		t.Fatalf("plain note block kind = %v; want Paragraph", got)
	}
	if got := len(doc.Blocks[0].Items[0].Lines); got != 2 {
		t.Fatalf("plain note has %d lines; want 2", got)
	}
	want := "<p>Delivery is ex-works.<br>Installation by the customer.</p>"
	if got := doc.HTML(); got != want {
		t.Errorf("plain note HTML drifted:\n got %q\nwant %q", got, want)
	}
}

// TestBlankLineSeparatesParagraphs pins the paragraph rule: one blank line ends
// a paragraph and the next non-blank line starts a new one.
func TestBlankLineSeparatesParagraphs(t *testing.T) {
	doc := Parse("First paragraph.\n\nSecond paragraph.")
	if len(doc.Blocks) != 2 {
		t.Fatalf("note parsed into %d blocks; want 2", len(doc.Blocks))
	}
	want := "<p>First paragraph.</p><p>Second paragraph.</p>"
	if got := doc.HTML(); got != want {
		t.Errorf("paragraph HTML drifted:\n got %q\nwant %q", got, want)
	}
}

// TestBoldAndItalicRenderAsStrongAndEm proves the two inline markers become
// semantic tags and their text is escaped exactly like any other text.
func TestBoldAndItalicRenderAsStrongAndEm(t *testing.T) {
	doc := Parse("Please **confirm** access and *arrive early*.")
	want := "<p>Please <strong>confirm</strong> access and <em>arrive early</em>.</p>"
	if got := doc.HTML(); got != want {
		t.Errorf("inline HTML drifted:\n got %q\nwant %q", got, want)
	}

	underscore := Parse("Please _confirm_ access.")
	if got, want := underscore.HTML(), "<p>Please <em>confirm</em> access.</p>"; got != want {
		t.Errorf("underscore italic HTML drifted:\n got %q\nwant %q", got, want)
	}
}

// TestSpacedMarkersPrintLiterally keeps multiplication and spaced asterisks
// from being read as emphasis: a marker pair whose content starts or ends with
// whitespace is text, not formatting.
func TestSpacedMarkersPrintLiterally(t *testing.T) {
	for _, source := range []string{"2 * 3 * 4", "a ** b ** c", "a _ b _ c"} {
		if got, want := Parse(source).HTML(), "<p>"+source+"</p>"; got != want {
			t.Errorf("source %q rendered %q; want it literal %q", source, got, want)
		}
	}
}

// TestUnmatchedMarkersPrintLiterally keeps a stray marker from swallowing the
// rest of the note: it is text, not a formatting instruction.
func TestUnmatchedMarkersPrintLiterally(t *testing.T) {
	for _, source := range []string{"2 * 3 = 6", "unclosed **bold", "snake_case_name"} {
		doc := Parse(source)
		if got, want := doc.HTML(), "<p>"+source+"</p>"; got != want {
			t.Errorf("source %q rendered %q; want it literal %q", source, got, want)
		}
	}
}

// TestBulletListRendersAsUL covers the bullet block and bold inside an item.
func TestBulletListRendersAsUL(t *testing.T) {
	doc := Parse("Before:\n- **First** item\n- Second item")
	want := "<p>Before:</p><ul><li><strong>First</strong> item</li><li>Second item</li></ul>"
	if got := doc.HTML(); got != want {
		t.Errorf("bullet HTML drifted:\n got %q\nwant %q", got, want)
	}
}

// TestNumberedListRenumbersFromItsFirstItem pins the list semantics: the first
// source number sets the start, and the rest are sequential, exactly as an
// <ol> renders. The editor may emit every marker as "1.".
func TestNumberedListRenumbersFromItsFirstItem(t *testing.T) {
	doc := Parse("1. Notify security\n1. Log the visit")
	want := "<ol><li>Notify security</li><li>Log the visit</li></ol>"
	if got := doc.HTML(); got != want {
		t.Errorf("numbered HTML drifted:\n got %q\nwant %q", got, want)
	}
	if got := doc.Blocks[0].Start; got != 1 {
		t.Errorf("numbered list start = %d; want 1", got)
	}

	started := Parse("3. Third\n4. Fourth")
	if got, want := started.HTML(), `<ol start="3"><li>Third</li><li>Fourth</li></ol>`; got != want {
		t.Errorf("started list HTML drifted:\n got %q\nwant %q", got, want)
	}
	if got := started.Blocks[0].Start; got != 3 {
		t.Errorf("numbered list start = %d; want 3", got)
	}
}

// TestRawHTMLIsEscaped is the security rule: pasted markup is text. No tag in
// the source may become a tag in the document.
func TestRawHTMLIsEscaped(t *testing.T) {
	source := `Delivery <script>alert("x")</script> & <b>bold</b>.`
	doc := Parse(source)
	html := doc.HTML()
	for _, raw := range []string{"<script>", "</script>", "<b>bold</b>"} {
		if strings.Contains(html, raw) {
			t.Errorf("raw markup %q survived into the note HTML", raw)
		}
	}
	want := `<p>Delivery &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt; &amp; &lt;b&gt;bold&lt;/b&gt;.</p>`
	if html != want {
		t.Errorf("escaped HTML drifted:\n got %q\nwant %q", html, want)
	}
}

// TestCRLFAndOuterBlankLinesAreNormalized proves a Windows source and a source
// with surrounding blank lines produce the same document.
func TestCRLFAndOuterBlankLinesAreNormalized(t *testing.T) {
	want := Parse("Line one\nLine two").HTML()
	for _, source := range []string{"Line one\r\nLine two", "\nLine one\nLine two\n", "  \nLine one\nLine two\n\n"} {
		if got := Parse(source).HTML(); got != want {
			t.Errorf("source %q rendered %q; want %q", source, got, want)
		}
	}
}

// TestEmptySourceProducesNothing is the no-card rule at the parser seam.
func TestEmptySourceProducesNothing(t *testing.T) {
	for _, source := range []string{"", " ", "\n\t\n"} {
		doc := Parse(source)
		if len(doc.Blocks) != 0 {
			t.Errorf("source %q parsed into %d blocks; want none", source, len(doc.Blocks))
		}
		if html := doc.HTML(); html != "" {
			t.Errorf("source %q rendered %q; want nothing", source, html)
		}
	}
}

// TestEmptyListItemsAreDropped keeps an empty bullet from drawing a marker
// with no line box, which the browser and the native renderer would size
// differently.
func TestEmptyListItemsAreDropped(t *testing.T) {
	doc := Parse("- First\n- \n- Third")
	if got, want := doc.HTML(), "<ul><li>First</li><li>Third</li></ul>"; got != want {
		t.Errorf("empty bullet HTML drifted:\n got %q\nwant %q", got, want)
	}
}
