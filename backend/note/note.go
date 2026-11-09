// Package note parses the restricted Markdown subset a quotation note may
// carry (#540) and renders it to safe HTML.
//
// The subset is deliberately small: bold (**text**), italic (*text* or
// _text_), bullet items (- text), numbered items (1. text), paragraphs
// separated by blank lines, and line breaks inside a paragraph. Everything
// else -- headings, links, images, tables, underline, strikethrough, code,
// raw HTML -- is text, escaped where it is rendered. One parser decides what
// the source means so the HTML card and the native PDF renderer cannot
// disagree about it.
package note

import (
	"fmt"
	"html"
	"strings"
)

// Kind is the block type a source line opens.
type Kind int

const (
	// Paragraph is one or more consecutive non-list lines. Its single item
	// carries one Line per source line, so the line breaks print.
	Paragraph Kind = iota
	// Bullets is a run of "- item" lines.
	Bullets
	// Numbers is a run of "1. item" lines. Start is the first source number;
	// the items print sequentially from it, exactly as an <ol> renders.
	Numbers
)

// Document is a parsed note.
type Document struct {
	Blocks []Block
}

// Block is one paragraph or list.
type Block struct {
	Kind  Kind
	Start int
	Items []Item
}

// Item is one paragraph (the only item of a Paragraph block) or one list item.
// Lines are the source line breaks inside it; an empty line never appears here
// because it ends the block instead.
type Item struct {
	Lines []Line
}

// Line is one printed line: a sequence of styled text spans.
type Line []Span

// Span is one run of text with the inline styles that apply to it. Bold and
// italic never nest in the supported subset; a span carries at most one.
type Span struct {
	Text   string
	Bold   bool
	Italic bool
}

// Parse turns a note source into its block structure. Outer blank lines and
// whitespace-only lines are ignored; CRLF is normalized to LF.
func Parse(source string) Document {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")

	var doc Document
	var paragraph []Line
	var list *Block

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		doc.Blocks = append(doc.Blocks, Block{
			Kind:  Paragraph,
			Start: 1,
			Items: []Item{{Lines: paragraph}},
		})
		paragraph = nil
	}
	flushList := func() {
		if list != nil && len(list.Items) > 0 {
			doc.Blocks = append(doc.Blocks, *list)
		}
		list = nil
	}

	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			flushParagraph()
			flushList()
			continue
		}
		if text, ok := bulletText(line); ok {
			flushParagraph()
			if list == nil || list.Kind != Bullets {
				flushList()
				list = &Block{Kind: Bullets, Start: 1}
			}
			if spans := parseInline(text); len(spans) > 0 {
				list.Items = append(list.Items, Item{Lines: []Line{spans}})
			}
			continue
		}
		if number, text, ok := numberedText(line); ok {
			flushParagraph()
			if list == nil || list.Kind != Numbers {
				flushList()
				list = &Block{Kind: Numbers, Start: number}
			}
			if spans := parseInline(text); len(spans) > 0 {
				list.Items = append(list.Items, Item{Lines: []Line{spans}})
			}
			continue
		}
		flushList()
		paragraph = append(paragraph, parseInline(strings.TrimSpace(line)))
	}
	flushParagraph()
	flushList()
	return doc
}

// bulletText recognizes "- text" at the start of a line, including an empty
// item written as "-" after trailing whitespace is removed.
func bulletText(line string) (string, bool) {
	if line == "-" {
		return "", true
	}
	if !strings.HasPrefix(line, "- ") {
		return "", false
	}
	return strings.TrimSpace(line[2:]), true
}

// numberedText recognizes "1. text" at the start of a line. The number is
// kept as the list's start; the remaining items are sequential.
func numberedText(line string) (int, string, bool) {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(line) || line[i] != '.' {
		return 0, "", false
	}
	if i+1 >= len(line) || (line[i+1] != ' ' && line[i+1] != '\t') {
		return 0, "", false
	}
	number := 0
	for _, c := range line[:i] {
		number = number*10 + int(c-'0')
	}
	if number < 1 {
		return 0, "", false
	}
	return number, strings.TrimSpace(line[i+1:]), true
}

// parseInline turns one line into styled spans. Bold is checked before italic
// so "**text**" is never read as two empty italic runs. An unmatched marker is
// literal text, and an underscore only emphasises at a word boundary, so a
// plain-text value such as snake_case_name survives.
func parseInline(text string) []Span {
	var spans []Span
	add := func(value string, bold, italic bool) {
		if value == "" {
			return
		}
		if n := len(spans); n > 0 && spans[n-1].Bold == bold && spans[n-1].Italic == italic {
			spans[n-1].Text += value
			return
		}
		spans = append(spans, Span{Text: value, Bold: bold, Italic: italic})
	}

	start := 0
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "**") {
			if j := strings.Index(text[i+2:], "**"); j > 0 {
				content := text[i+2 : i+2+j]
				if emphasizable(content) {
					add(text[start:i], false, false)
					add(content, true, false)
					i += 2 + j + 2
					start = i
					continue
				}
			}
		}
		if text[i] == '*' || text[i] == '_' {
			marker := text[i]
			if j := strings.IndexByte(text[i+1:], marker); j > 0 {
				content := text[i+1 : i+1+j]
				closed := i + 1 + j + 1
				boundaryOK := marker != '_' || (wordBoundaryBefore(text, i) && wordBoundaryAfter(text, closed))
				notBoldMarker := !strings.HasPrefix(text[i+1:], string(marker))
				if boundaryOK && notBoldMarker && emphasizable(content) {
					add(text[start:i], false, false)
					add(content, false, true)
					i = closed
					start = i
					continue
				}
			}
		}
		i++
	}
	add(text[start:], false, false)
	return spans
}

// emphasizable reports whether content may be wrapped by a marker pair. A
// marker pair whose content starts or ends with whitespace is literal text,
// which keeps "2 * 3 * 4" and "a ** b ** c" printing as typed.
func emphasizable(content string) bool {
	return content != "" && content == strings.TrimSpace(content)
}

// wordBoundaryBefore reports whether i starts a word: the previous byte is not
// a letter or digit.
func wordBoundaryBefore(text string, i int) bool {
	return i == 0 || !isWordByte(text[i-1])
}

// wordBoundaryAfter reports whether i ends a word: the next byte is not a
// letter or digit.
func wordBoundaryAfter(text string, i int) bool {
	return i >= len(text) || !isWordByte(text[i])
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// HTML renders the parsed note to the safe markup the quotation card prints.
// Every text span is escaped first; the only tags in the output are the ones
// this function writes. An empty document renders as an empty string, so a
// whitespace-only note adds no card.
func (d Document) HTML() string {
	var b strings.Builder
	for _, block := range d.Blocks {
		switch block.Kind {
		case Paragraph:
			if len(block.Items) == 0 {
				continue
			}
			b.WriteString("<p>")
			writeItem(&b, block.Items[0])
			b.WriteString("</p>")
		case Bullets:
			b.WriteString("<ul>")
			for _, item := range block.Items {
				b.WriteString("<li>")
				writeItem(&b, item)
				b.WriteString("</li>")
			}
			b.WriteString("</ul>")
		case Numbers:
			if block.Start == 1 {
				b.WriteString("<ol>")
			} else {
				fmt.Fprintf(&b, `<ol start="%d">`, block.Start)
			}
			for _, item := range block.Items {
				b.WriteString("<li>")
				writeItem(&b, item)
				b.WriteString("</li>")
			}
			b.WriteString("</ol>")
		}
	}
	return b.String()
}

// writeItem writes one item's lines, separated by <br>.
func writeItem(b *strings.Builder, item Item) {
	for i, line := range item.Lines {
		if i > 0 {
			b.WriteString("<br>")
		}
		for _, span := range line {
			text := html.EscapeString(span.Text)
			switch {
			case span.Bold:
				b.WriteString("<strong>")
				b.WriteString(text)
				b.WriteString("</strong>")
			case span.Italic:
				b.WriteString("<em>")
				b.WriteString(text)
				b.WriteString("</em>")
			default:
				b.WriteString(text)
			}
		}
	}
}

// HTML parses a note source and renders it, which is the call the application
// uses when it fills the template data.
func HTML(source string) string {
	return Parse(source).HTML()
}
