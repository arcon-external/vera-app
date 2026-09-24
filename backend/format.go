package backend

import (
	"fmt"
	"html"
	"html/template"
	"strings"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var rupiahPrinter = message.NewPrinter(language.Indonesian)

func formatRupiah(amount uint64) string {
	return rupiahPrinter.Sprintf("%d", amount)
}

func formatSignedRupiah(amount int64) string {
	return rupiahPrinter.Sprintf("%d", amount)
}

func formatDescription(raw string) template.HTML {
	clean := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(clean, "\n")

	mainParts := make([]string, 0, len(lines))
	bullets := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if item != "" {
				bullets = append(bullets, item)
			}
			continue
		}

		mainParts = append(mainParts, trimmed)
	}

	var b strings.Builder

	if len(mainParts) > 0 {
		b.WriteString(`<div class="desc-main">`)
		b.WriteString(html.EscapeString(strings.Join(mainParts, " ")))
		b.WriteString(`</div>`)
	}

	if len(bullets) > 0 {
		b.WriteString(`<ul class="desc-subitems">`)
		for _, item := range bullets {
			b.WriteString(`<li>`)
			b.WriteString(html.EscapeString(item))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}

	if b.Len() == 0 {
		return template.HTML(html.EscapeString(raw))
	}

	return template.HTML(b.String())
}

// maxExportSubjectRunes caps the subject segment of export filenames.
const maxExportSubjectRunes = 60

// BuildExportFilename renders the default save-dialog name for a quotation
// PDF: Arcon_Quotation_XXX_YYY, or Arcon_Quotation_XXX_Rev.Z_YYY once it is
// a revision (revision 0 carries no suffix). The subject keeps spaces but
// loses filesystem-illegal characters; an empty subject falls back to
// "Quotation" so the name is never blank.
func BuildExportFilename(quoteNumber string, revision uint16, subject string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		if unicode.IsControl(r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(subject))
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	runes := []rune(cleaned)
	if len(runes) > maxExportSubjectRunes {
		cut := runes[:maxExportSubjectRunes]
		// Break at a word boundary so long subjects never end mid-word
		// ("...Systems a.pdf"); spaceless strings keep the hard cut.
		if lastSpace := strings.LastIndex(string(cut), " "); lastSpace > 0 {
			cut = cut[:lastSpace]
		}
		cleaned = strings.TrimSpace(string(cut))
	}
	if cleaned == "" {
		cleaned = "Quotation"
	}
	if revision > 0 {
		return fmt.Sprintf("Arcon_Quotation_%s_Rev.%d_%s.pdf", quoteNumber, revision, cleaned)
	}
	return fmt.Sprintf("Arcon_Quotation_%s_%s.pdf", quoteNumber, cleaned)
}
