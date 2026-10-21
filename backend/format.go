package backend

import (
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"vera-app/backend/currency"
)

var rupiahPrinter = message.NewPrinter(language.Indonesian)

func formatRupiah(amount uint64) string {
	return rupiahPrinter.Sprintf("%d", amount)
}

func formatSignedRupiah(amount int64) string {
	return rupiahPrinter.Sprintf("%d", amount)
}

// formatMoney renders a non-negative minor-unit amount as a bare number in
// the company currency's home convention: the item table's cells stay bare
// while the totals carry the symbol. For IDR this is formatRupiah exactly.
func formatMoney(code string, amount uint64) string {
	return currency.FormatAmount(code, amount)
}

// formatSignedMoney renders a signed minor-unit amount the same way.
func formatSignedMoney(code string, amount int64) string {
	return currency.FormatSignedAmount(code, amount)
}

func formatDescription(raw string) template.HTML {
	main, bullets := splitDescriptionLines(raw)

	var b strings.Builder

	if main != "" {
		b.WriteString(`<div class="desc-main">`)
		b.WriteString(html.EscapeString(main))
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

// splitDescriptionLines parses a raw description once, for every consumer of
// its two halves. Lines whose trimmed form starts with "- " are description
// lines; every other non-empty line joins the main description with single
// spaces. It is the one place that decides what a bullet is, so the main-only
// HTML below and the numbered line rows beside it cannot disagree about which
// lines are which.
func splitDescriptionLines(raw string) (main string, lines []string) {
	clean := strings.ReplaceAll(raw, "\r\n", "\n")

	mainParts := make([]string, 0)
	bullets := make([]string, 0)

	for _, line := range strings.Split(clean, "\n") {
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

	return strings.Join(mainParts, " "), bullets
}

// formatDescriptionMain renders the main half of a raw description only: the
// bullet lines travel separately as numbered DescriptionLines, so emitting
// them here too would print every line twice. A description of bullets only
// has no main half, and its HTML is empty with it.
func formatDescriptionMain(raw string) template.HTML {
	main, _ := splitDescriptionLines(raw)
	if main == "" {
		return template.HTML("")
	}
	return template.HTML(`<div class="desc-main">` + html.EscapeString(main) + `</div>`)
}

// FormatLeadTime composes the lead time sentence the document prints:
// "2 Years, 10 Months, 4 Weeks, and 5 Days".
//
// The four units are stated independently and print as entered -- there is no
// normalisation, because "4 Weeks" is what staff typed and converting it to
// "28 Days" would print something nobody chose. Zeros are skipped, so a lead
// time of 60 days reads "60 Days" rather than "0 Years, 0 Months, 0 Weeks,
// and 60 Days", and a lead time with no unit at all reads "-".
//
// It lives here, beside the money formatters, because it is a display
// composition and not a modelling decision: exported once by the app layer
// (Prepare), by the archive detail (api/mapDetail), and by both renderers
// through the seam, so no two of them can disagree about the sentence.
func FormatLeadTime(years, months, weeks, days uint8) string {
	units := []struct {
		count    uint8
		singular string
		plural   string
	}{
		{years, "Year", "Years"},
		{months, "Month", "Months"},
		{weeks, "Week", "Weeks"},
		{days, "Day", "Days"},
	}
	segments := make([]string, 0, len(units))
	for _, unit := range units {
		if unit.count == 0 {
			continue
		}
		word := unit.plural
		if unit.count == 1 {
			word = unit.singular
		}
		segments = append(segments, strconv.Itoa(int(unit.count))+" "+word)
	}
	switch len(segments) {
	case 0:
		// Nothing stated. A dash reads as "not applicable"; "0 days" would
		// read as a commitment to deliver instantly.
		return "-"
	case 1:
		return segments[0]
	case 2:
		return segments[0] + " and " + segments[1]
	default:
		return strings.Join(segments[:len(segments)-1], ", ") + ", and " + segments[len(segments)-1]
	}
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
