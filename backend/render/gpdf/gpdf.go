// Package gpdf lays out and renders a quotation without a browser.
//
// The layout engine underneath is the original pdfengine code, unchanged: it
// models this one fixed template explicitly rather than interpreting CSS, because
// a generic CSS interpreter would be a second browser to maintain and would not
// be any more faithful than measuring the template's own geometry was.
//
// This file is the seam between that engine and the application's quotation. The
// engine's own Input is a rendering-shaped model -- description already split
// from its bullets, a quantity already formatted, the revision already padded --
// so converting into it is a decision about the page, and it belongs here rather
// than in build() where it would apply to both renderers.
package gpdf

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"vera-app/backend/render/quotation"
)

// New returns the native renderer.
//
// The two Inter faces are read on the first export rather than here, so a broken
// font fails that export with the name in the error instead of preventing the
// application from starting. The engine is not always the selected one and must
// not be able to break startup while dormant.
func New() quotation.Named { return &lazy{} }

type lazy struct {
	err  error
	eng  *Engine
	once sync.Once
}

// Name implements quotation.Named.
func (*lazy) Name() string { return "gpdf" }

// Render loads the engine on first use, then renders a per-render copy.
//
// The shared engine is never mutated: WithTheme copies it with the request's
// palette, so concurrent exports do not race on the theme. The copy shares
// the parsed fonts and shapers, which are read-only after construction.
func (l *lazy) Render(_ context.Context, in quotation.Input) ([]byte, error) {
	l.once.Do(func() {
		l.eng, l.err = NewEngine(in.Fonts, "assets/inter-medium.ttf", "assets/inter-bold.ttf")
	})
	if l.err != nil {
		return nil, fmt.Errorf("load render engine: %w", l.err)
	}

	eng := l.eng.WithTheme(Theme{
		Deep:   in.Theme.Deep,
		Accent: in.Theme.Accent,
		Soft:   in.Theme.Soft,
		Ink:    in.Theme.Ink,
	})

	return eng.Render(Input{
		CompanyName:    in.CompanyName,
		CompanyAddress: splitLines(in.CompanyAddress),
		LogoPNG:        in.LogoPNG,

		Subject:          in.Subject,
		RecipientName:    in.RecipientName,
		RecipientCompany: in.RecipientCompany,
		RecipientAddress: in.RecipientAddress,

		QuoteNumber: in.QuoteNumber,
		Date:        in.Date,
		ValidUntil:  in.ValidUntil,
		Revision:    fmt.Sprintf("%02d", in.RevisionID),

		// The application resolved the effective labels and substituted the
		// terms sentences; this renderer prints them. It composes no wording of
		// its own -- a second copy of a sentence is a sentence that can
		// disagree (the #377 lesson, applied to the whole document's labels by
		// #339). The engine completes an empty field at its own door too, so a
		// caller that builds an engine Input directly still prints the built-in
		// wording.
		Labels: in.Labels,
		Terms:  in.Terms,

		Items: engineItems(in.Items),

		ShowLineDiscounts: in.ShowLineDiscounts,

		DisplaySubTotal:   in.DisplaySubTotal,
		DisplayDiscount:   in.DisplayDiscount,
		DiscountAmount:    in.DiscountAmount,
		PpnAmount:         in.PpnAmount,
		DisplayPpn:        in.DisplayPpn,
		DisplayGrandTotal: in.DisplayGrandTotal,
		DiscountPercent:   strconv.Itoa(int(in.DiscountPercent)),
		Currency:          in.Currency,

		Closing:          in.Labels.Closing,
		VerificationCode: in.VerificationCode,
		QRPNG:            in.QRPNG,
	})
}

// engineItems flattens quotation lines into the engine's row model, splitting
// each description back into its main text and its bullets. Package lines
// carry their parts through as structured data; their descriptions hold no
// bullets (validation refuses both at once).
func engineItems(items []quotation.Item) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		main, subs := splitDescription(it.DisplayDesc)
		row := Item{
			No:        it.No,
			DescMain:  main,
			SubItems:  subs,
			Qty:       formatQty(it.Qty),
			UoM:       it.UoM,
			UnitPrice: it.UnitPrice,
			Total:     it.Total,
			Disc:      it.Disc,
		}
		if len(it.SubItems) > 0 {
			pkg := &Package{ShowPrices: it.ShowSubPrices, Parts: make([]Part, 0, len(it.SubItems))}
			for _, sub := range it.SubItems {
				pkg.Parts = append(pkg.Parts, Part{
					SubNo:     sub.SubNo,
					Desc:      sub.Description,
					Qty:       formatQty(sub.Qty),
					UoM:       sub.UoM,
					UnitPrice: sub.UnitPrice,
					Total:     sub.Total,
					Disc:      sub.Disc,
					Bullets:   sub.Bullets,
				})
			}
			row.Package = pkg
		}
		out = append(out, row)
	}
	return out
}

// splitDescription reverses formatDescription: the bullets it emitted as list
// items come back as bullets, and the leading text as the description.
//
// formatDescription is HTML, so the reversal is on tags rather than on text. It
// is not a general HTML parser and is not meant to be: the only producer of this
// markup is the function above, and the engine has to agree with it exactly.
func splitDescription(html string) (string, []string) {
	var main strings.Builder
	var bullets []string

	rest := html
	// Everything before the first list item is the description.
	if i := strings.Index(rest, "<ul"); i >= 0 {
		main.WriteString(stripTags(rest[:i]))
		rest = rest[i:]
	} else {
		return stripTags(rest), nil
	}
	for {
		i := strings.Index(rest, "<li>")
		if i < 0 {
			break
		}
		rest = rest[i+len("<li>"):]
		j := strings.Index(rest, "</li>")
		if j < 0 {
			break
		}
		bullets = append(bullets, stripTags(rest[:j]))
		rest = rest[j+len("</li>"):]
	}
	return strings.TrimSpace(main.String()), bullets
}

// stripTags removes tags and decodes the handful of entities formatDescription
// can emit.
func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(s[i])
			}
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&#34;", `"`)
	out = strings.ReplaceAll(out, "&#39;", "'")
	out = strings.ReplaceAll(out, "&quot;", `"`)
	return out
}

// splitLines breaks a multi-line profile field into its lines.
func splitLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// formatQty renders a quantity the way the template does: plain digits, no
// thousands separator.
func formatQty(q uint32) string { return strconv.FormatUint(uint64(q), 10) }
