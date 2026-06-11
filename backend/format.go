package backend

import (
	"html"
	"html/template"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var rupiahPrinter = message.NewPrinter(language.Indonesian)

func formatRupiah(amount uint64) string {
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
