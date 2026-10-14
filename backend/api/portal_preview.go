package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/store"
	"vera-app/backend/theme"
)

// The admin portal preview: the verification page as it renders right now,
// with the colours typed but not yet saved, so a rebrand is judged before it is
// committed. It exists because the portal has five states a colour change can
// make confusable, and the one thing that page must never be is ambiguous about
// whether a document is genuine.
//
// Four rules hold this together, and each is pinned by a test:
//
//  1. It renders the real template. The preview is not a React port and not a
//     markup copy: it executes verify_template.html through the same helpers
//     the live portal does, so it cannot drift from the page a customer sees.
//  2. It touches no real record. The evidence is fabricated in-process and the
//     database is read once, for the issuer name. Nothing is written, and no
//     revision is reachable from here.
//  3. It is frameable, and the live portal is not: the same CSP serves both
//     except frame-ancestors, because the preview has to load inside the
//     settings iframe while a customer-facing page must never be framed.
//  4. It has its own rate-limit budget. Sharing the public verifier's bucket
//     would let a rebranding admin starve the customers scanning real
//     quotations from the same office address.

// portalPreviewVerdicts are the five states the toggle renders, in the order
// the segmented control shows them. The classes come from the template.
var portalPreviewVerdicts = []string{
	verdictValid,
	verdictSuperseded,
	verdictMismatch,
	verdictNotFound,
	verdictUnavailable,
}

// portalPreviewParams is the entire accepted surface: the verdict and the four
// portal colours. Anything else in the query string is refused rather than
// ignored, so the endpoint cannot grow a hidden input — a company edits four
// colours, and this preview is what proves nothing else is editable.
var portalPreviewParams = map[string]bool{
	"verdict": true,
	"deep":    true,
	"accent":  true,
	"soft":    true,
	"ink":     true,
}

// portalPreviewCSP is the live portal's CSP with one deliberate difference:
// frame-ancestors allows 'self' so the admin settings page can embed it. The
// live page keeps 'none' — a customer-facing verification page must never be
// framed by anyone.
const portalPreviewCSP = "default-src 'none'; style-src 'unsafe-inline'; " +
	"form-action 'self'; base-uri 'none'; frame-ancestors 'self'"

// previewEvidence is the fabricated document the preview verifies. Its field
// set mirrors store.RevisionEvidence on purpose: the view is built through the
// same helper the live path uses, so a field the real page needs is a field
// this shape must carry. The figures are constant across every verdict — the
// toggle changes the verdict, never the document, which is what makes the five
// states comparable.
func previewEvidence() store.RevisionEvidence {
	return store.RevisionEvidence{
		QuoteNumber:      "15CC260001",
		RevisionNumber:   0,
		CreatedAt:        time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		RecipientName:    "Budi Santoso",
		RecipientCompany: "PT Contoh Nusantara",
		// Empty so the live profile's legal name fills it: the preview shows the
		// issuer from the General tab, which is the proof that the two sections
		// of settings are talking about one company.
		IssuerName: "",
		Currency:   "IDR",
		GrandTotal: 150_000_000_00,
		PpnAmount:  16_500_000_00,
		ItemCount:  3,
	}
}

// verifyDisplay fills the fields a reader sees on any verification page from
// the record. Both the live portal and the admin preview build their view
// through here, so the preview cannot show a field the real page cannot — and a
// field later added to the portal is added to this shape's consumer in the
// same change.
func verifyDisplay(view *verifyView, evidence store.RevisionEvidence) {
	view.QuoteNo = evidence.QuoteNumber
	view.Revision = "Rev " + pad2(evidence.RevisionNumber)
	view.IssueDate = evidence.CreatedAt.Format("2 January 2006")
	view.Recipient = evidence.RecipientCompany
	view.Issuer = evidence.IssuerName
	view.Total = moneyWithSymbol(evidence.Currency, evidence.GrandTotal)
	if evidence.PpnAmount > 0 {
		view.PPN = moneyWithSymbol(evidence.Currency, evidence.PpnAmount)
	}
}

// verifyBrand resolves the portal palette from the four company colours.
// Typed-but-unsaved colours reach the template through the same ResolvePortal
// derivation the stored ones do, so what the preview shows is what the page
// will show once the profile save lands. The status colours are not part of
// this palette: they are literals in the template, which is why a brand colour
// can never restyle a verdict (see backend/theme).
func verifyBrand(view *verifyView, deep, accent, soft, ink string) {
	view.Portal = theme.Theme{Deep: deep, Accent: accent, Soft: soft, Ink: ink}.ResolvePortal()
}

// previewViewFor builds the whole view for one state, through the same helper
// the live portal uses. The document is constant; only the verdict and the
// wording change.
func previewViewFor(verdict string, portal theme.PortalPalette, issuer, detail string) verifyView {
	view := verifyView{
		Verdict:  verdict,
		LookupID: "",
		// The preview never compares paper claims, so it renders no diff section
		// in any state.
		AnyClaim: false,
		Portal:   portal,
		Detail:   detail,
	}
	verifyDisplay(&view, previewEvidence())
	if view.Issuer == "" {
		view.Issuer = issuer
	}
	return view
}

// defaultPreviewIssuer is what the template already prints when a stored
// profile carries no legal name. Kept identical so the preview cannot show a
// name a customer page would not.
const defaultPreviewIssuer = "PT Arcon Penta Persada"

// previewColor keeps a strict #RRGGBB value and replaces anything else with the
// fallback, mirroring theme.fallback so a bad query value cannot reach the
// template as an empty or malformed colour.
func previewColor(value, fallback string) string {
	if len(value) == 7 && value[0] == '#' {
		valid := true
		for i := 1; i < 7; i++ {
			c := value[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				valid = false
				break
			}
		}
		if valid {
			return value
		}
	}
	return fallback
}

// isPortalPreviewVerdict reports whether verdict is one the template renders.
func isPortalPreviewVerdict(verdict string) bool {
	for _, candidate := range portalPreviewVerdicts {
		if candidate == verdict {
			return true
		}
	}
	return false
}

// portalPreview renders the verification page for the settings editor.
func (s *Server) portalPreview(context echo.Context) error {
	if !s.previewLimiter.allow(clientIP(context)) {
		return s.renderPortalPreview(context, http.StatusTooManyRequests, previewViewFor(
			verdictUnavailable, theme.DefaultTheme().ResolvePortal(), "",
			"Too many previews from this device. Please wait a moment and try again."))
	}

	// The surface is closed. An unknown parameter is refused rather than
	// ignored, so this endpoint can never quietly accept a fifth editable thing.
	query := context.QueryParams()
	for name := range query {
		if !portalPreviewParams[name] {
			return context.JSON(http.StatusBadRequest, map[string]string{
				"error": "unknown preview parameter: " + name,
			})
		}
	}

	verdict := strings.ToLower(strings.TrimSpace(context.QueryParam("verdict")))
	if !isPortalPreviewVerdict(verdict) {
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": "unknown verdict: " + verdict,
		})
	}

	// Strict hex or nothing: a colour that is not a colour falls back to the
	// brand default rather than reaching the template, exactly as the store
	// does on write.
	themeInput := theme.Theme{
		Deep:   previewColor(context.QueryParam("deep"), theme.DefaultDeep),
		Accent: previewColor(context.QueryParam("accent"), theme.DefaultAccent),
		Soft:   previewColor(context.QueryParam("soft"), theme.DefaultSoft),
		Ink:    previewColor(context.QueryParam("ink"), theme.DefaultInk),
	}

	// The only database read: the issuer name, which lives in the profile the
	// General tab edits. The preview therefore proves the two sections are
	// connected — a colour chosen here is judged beside the company's own name.
	// A failed read leaves the name empty and the template falls back to its
	// default issuer, exactly as a customer page does today.
	issuer := ""
	if profile, err := s.database.GetCompanyProfile(context.Request().Context()); err == nil {
		issuer = profile.LegalName
	}

	view := previewViewFor(verdict, themeInput.ResolvePortal(), issuer, "")
	if view.Issuer == "" {
		view.Issuer = defaultPreviewIssuer
	}
	return s.renderPortalPreview(context, http.StatusOK, view)
}

// renderPortalPreview sends one verification page for the settings iframe. The
// headers are the live page's hardening with the framing difference noted
// above, and the body is a full document for an iframe srcDoc.
func (s *Server) renderPortalPreview(context echo.Context, status int, view verifyView) error {
	if view.Headline == "" {
		headline, detail := verifyTitles(view.Verdict)
		view.Headline = headline
		if view.Detail == "" {
			view.Detail = detail
		}
	}
	if view.Portal.Deep == "" {
		view.Portal = theme.DefaultTheme().ResolvePortal()
	}
	context.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	context.Response().Header().Set("X-Content-Type-Options", "nosniff")
	context.Response().Header().Set("X-Frame-Options", "SAMEORIGIN")
	context.Response().Header().Set("Referrer-Policy", "no-referrer")
	context.Response().Header().Set("Content-Security-Policy", portalPreviewCSP)
	context.Response().Header().Set("Cache-Control", "no-store, max-age=0")
	return context.Render(status, "verify_template.html", view)
}
