package backend

import (
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed assets/*
var backAssets embed.FS

//go:embed template/html/*
var htmlTemplate embed.FS

//go:embed template/css/*
var cssTemplate embed.FS

// BakedInLogo returns the built-in company artwork shipped with the app.
// Uploaded logos override it per profile; this is the fallback the Settings
// preview shows when no upload exists.
func BakedInLogo() ([]byte, error) {
	logoBytes, err := backAssets.ReadFile("assets/arcon-logo.png")
	if err != nil {
		return nil, fmt.Errorf("load logo asset: %w", err)
	}
	return logoBytes, nil
}

func readTemplate(q *Quotation) error {
	cssContent, err := cssTemplate.ReadFile("template/css/template-fixed.css")
	if err != nil {
		return fmt.Errorf("load css template: %w", err)
	}

	htmlContent, err := htmlTemplate.ReadFile("template/html/template-fixed.html")
	if err != nil {
		return fmt.Errorf("load html template: %w", err)
	}

	logoBytes, err := backAssets.ReadFile("assets/arcon-logo.png")
	if err != nil {
		return fmt.Errorf("load logo asset: %w", err)
	}

	sigBytes, err := backAssets.ReadFile("assets/signature.png")
	if err != nil {
		return fmt.Errorf("load signature asset: %w", err)
	}

	fontRegularBytes, err := backAssets.ReadFile("assets/inter-medium.ttf")
	if err != nil {
		return fmt.Errorf("load regular font asset: %w", err)
	}

	fontBoldBytes, err := backAssets.ReadFile("assets/inter-bold.ttf")
	if err != nil {
		return fmt.Errorf("load bold font asset: %w", err)
	}

	htmlTmpl, err := template.New("quotation").Parse(string(htmlContent))
	if err != nil {
		return fmt.Errorf("parse quotation template: %w", err)
	}

	q.Data.FontRegularBase64 = base64.StdEncoding.EncodeToString(fontRegularBytes)
	q.Data.FontBoldBase64 = base64.StdEncoding.EncodeToString(fontBoldBytes)

	// Build the stylesheet with text/template, not html/template. The stylesheet
	// lands in a raw-text <style> body, and html/template escapes every
	// interpolation it cannot place in a CSS context: the embedded base64 fonts
	// come out as "...&#43;..." instead of "...+...". Because <style> is a
	// raw-text element the browser never decodes those entities, so the data: URI
	// is corrupt, the @font-face fails to load, and every quotation silently
	// falls back to Helvetica -- which is what the preview showed while the
	// export drew Inter.
	//
	// text/template is safe here: every interpolation is either an embedded asset
	// (the Inter TTFs) or a theme colour already restricted to strict #RRGGBB by
	// store.isHexColor. Nothing user-supplied reaches this file.
	cssTmpl, err := texttemplate.New("css").Parse(string(cssContent))
	if err != nil {
		return fmt.Errorf("parse css template: %w", err)
	}

	var cssBuf strings.Builder
	if err := cssTmpl.Execute(&cssBuf, q.Data); err != nil {
		return fmt.Errorf("execute css template: %w", err)
	}

	q.cssTemplate = cssTmpl
	q.Data.CustomCSS = template.CSS(cssBuf.String())
	q.Data.HtmlTemplate = htmlTmpl
	q.Data.LogoBase64 = base64.StdEncoding.EncodeToString(logoBytes)
	q.Data.SignatureBase64 = base64.StdEncoding.EncodeToString(sigBytes)

	return nil
}
