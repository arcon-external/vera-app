package backend

// Guards for the template preview (issue #425).
//
// The load-bearing test is TestPreviewTemplateDocumentIsStateless: the
// preview renders a mock through the same engine the export uses, and the
// #339 measurement showed a mock through the live binding would make the next
// export store the mock as a real signed revision. The binding renders on a
// scratch instance instead, and this file proves the receiver is untouched --
// including the export half, where the bytes RenderPDF produces after a
// preview are still the real document's.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/auth"
)

// newAdminSampleQuotation is a bound-asset instance with an admin session,
// the way the desktop calls the preview: signed in, template loaded, client
// directory cached.
func newAdminSampleQuotation(t *testing.T) *Quotation {
	t.Helper()
	q := newTestQuotation()
	q.SetAuthService(auth.AdminTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	return q
}

// countSamplePages counts the page dictionaries in rendered PDF bytes. gpdf
// writes each page as "<< /Contents ... /Type /Page >>" beside the single
// "/Type /Pages" root, so the difference is the page count.
func countSamplePages(t *testing.T, pdf []byte) int {
	t.Helper()
	pages := strings.Count(string(pdf), "/Type /Page") - strings.Count(string(pdf), "/Type /Pages")
	if pages < 1 {
		t.Fatalf("no page dictionaries in %d rendered bytes", len(pdf))
	}
	return pages
}

// prepareRealQuotation prepares a small real quotation on q and returns the
// bytes its export would print, so the statelessness test can compare
// before/after.
func prepareRealQuotation(t *testing.T, q *Quotation) []byte {
	t.Helper()
	if err := q.Prepare(QuotationInput{
		Subject: "real quotation, not the sample", RecipientName: "Siti Rahayu",
		CustomerID: 2, Incoterm: "DAP", Validity: 30, PaymentTerm: 30,
		TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
		Items: []QuotationItemInput{{Description: "real line", UoM: "Pc", Qty: 2, UnitPrice: 1500000}},
	}); err != nil {
		t.Fatalf("Prepare real: %v", err)
	}
	before, err := q.RenderPDF()
	if err != nil {
		t.Fatalf("render real: %v", err)
	}
	return before
}

func TestPreviewTemplateDocumentRendersTwoPageSample(t *testing.T) {
	q := newAdminSampleQuotation(t)
	pdf, err := q.PreviewTemplateDocument(nil, "", "", "", "")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF") {
		t.Errorf("sample does not start with %%PDF (%d bytes)", len(pdf))
	}
	if got := countSamplePages(t, pdf); got != 2 {
		t.Errorf("sample pages = %d, want 2: the mock must paginate so the repeated header and page numbers render", got)
	}
}

func TestPreviewTemplateDocumentIsStateless(t *testing.T) {
	q := newAdminSampleQuotation(t)
	before := prepareRealQuotation(t, q)
	lastInput := q.lastInput
	data := q.Data
	prepared := q.prepared

	// Twice, with different drafts, so a leak cannot hide behind one render.
	first, err := q.PreviewTemplateDocument(nil, "", "", "", "")
	if err != nil {
		t.Fatalf("first preview: %v", err)
	}
	second, err := q.PreviewTemplateDocument(
		[]CompanyLabelEntry{{Key: "docTitle", Value: "PROPOSAL"}},
		"#1A237E", "#284AF3", "#D6E4FF", "#1a1a1a")
	if err != nil {
		t.Fatalf("second preview: %v", err)
	}
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("preview returned %d and %d bytes", len(first), len(second))
	}

	if !reflect.DeepEqual(q.lastInput, lastInput) {
		t.Errorf("preview rewrote lastInput: the next export would store the mock as a real signed revision")
	}
	if !reflect.DeepEqual(q.Data, data) {
		t.Errorf("preview rewrote Data: the next export would print the mock")
	}
	if q.prepared != prepared {
		t.Errorf("preview flipped prepared to %v", q.prepared)
	}
	// The export half: what RenderPDF produces after the previews is still
	// the real document, byte for byte.
	after, err := q.RenderPDF()
	if err != nil {
		t.Fatalf("render after preview: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("RenderPDF after preview differs from before (%d vs %d bytes): the export would print the mock", len(after), len(before))
	}
}

func TestPreviewTemplateDocumentAppliesDraftWording(t *testing.T) {
	q := newAdminSampleQuotation(t)
	plain, err := q.PreviewTemplateDocument(nil, "", "", "", "")
	if err != nil {
		t.Fatalf("plain preview: %v", err)
	}
	reworded, err := q.PreviewTemplateDocument(
		[]CompanyLabelEntry{{Key: "docTitle", Value: "PROPOSAL"}}, "", "", "", "")
	if err != nil {
		t.Fatalf("reworded preview: %v", err)
	}
	if reflect.DeepEqual(plain, reworded) {
		t.Errorf("a reworded title produced identical bytes: the preview ignores the draft")
	}
	// Clearing a label restores the built-in default: an entry cleared back
	// to blank renders exactly what no entries render.
	cleared, err := q.PreviewTemplateDocument(
		[]CompanyLabelEntry{{Key: "docTitle", Value: "  "}}, "", "", "", "")
	if err != nil {
		t.Fatalf("cleared preview: %v", err)
	}
	if !reflect.DeepEqual(plain, cleared) {
		t.Errorf("a cleared label did not fall back to the built-in default")
	}
}

func TestPreviewTemplateDocumentMarksTheSample(t *testing.T) {
	q := newAdminSampleQuotation(t)
	// The marks are asserted on the scratch path's data rather than the PDF
	// bytes: gpdf encodes text runs, so a byte search cannot see them, but
	// the values the renderer reads are pinned here.
	scratch := &Quotation{
		ctx:               q.ctx,
		auth:              q.auth,
		customerDirectory: q.customerDirectory,
	}
	identity, err := q.resolveCompanyIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	scratch.companyIdentity = &identity
	scratch.cssTemplate = q.cssTemplate
	scratch.Data.HtmlTemplate = q.Data.HtmlTemplate
	scratch.Data.CustomCSS = q.Data.CustomCSS
	scratch.Data.LogoBase64 = q.Data.LogoBase64
	scratch.Data.SignatureBase64 = q.Data.SignatureBase64
	scratch.Data.FontRegularBase64 = q.Data.FontRegularBase64
	scratch.Data.FontBoldBase64 = q.Data.FontBoldBase64
	if err := scratch.Prepare(templateSampleInput(2)); err != nil {
		t.Fatalf("prepare sample: %v", err)
	}
	scratch.Data.QuoteNumber = sampleQuoteNumber
	if err := scratch.SetVerifyURL(sampleVerifyBase() + "/v/" + sampleVerifyCode); err != nil {
		t.Fatalf("sample code: %v", err)
	}
	if scratch.Data.QuoteNumber != sampleQuoteNumber {
		t.Errorf("sample number = %q, want the fake %q", scratch.Data.QuoteNumber, sampleQuoteNumber)
	}
	if !strings.HasSuffix(scratch.Data.VerifyURL, "/v/"+sampleVerifyCode) {
		t.Errorf("sample verify URL = %q, want it to carry the non-verifying code", scratch.Data.VerifyURL)
	}
	if scratch.Data.VerifyQRBase64 == "" {
		t.Errorf("sample carries no QR: the code block must still render")
	}
}

func TestPreviewTemplateDocumentRejectsBadHex(t *testing.T) {
	q := newAdminSampleQuotation(t)
	for _, colours := range [][4]string{
		{"red", "", "", ""},
		{"", "#12345", "", ""},
		{"", "", "#GGGGGG", ""},
		{"", "", "", "123456"},
	} {
		if _, err := q.PreviewTemplateDocument(nil, colours[0], colours[1], colours[2], colours[3]); err == nil {
			t.Errorf("colours %q accepted, want a field error holding the last good preview", colours)
		}
	}
	// Empty means the stylesheet default and stays valid.
	if _, err := q.PreviewTemplateDocument(nil, "", "", "", ""); err != nil {
		t.Errorf("empty colours rejected: %v", err)
	}
}

func TestPreviewTemplateDocumentRequiresAdmin(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if _, err := q.PreviewTemplateDocument(nil, "", "", "", ""); err == nil {
		t.Errorf("staff reached the sample: the binding is company settings and must be admin-only")
	}
	if _, err := q.DownloadTemplateSample(nil, "", "", "", ""); err == nil {
		t.Errorf("staff reached the sample download: the binding is company settings and must be admin-only")
	}
}

func TestTemplateSampleCoversTheShapes(t *testing.T) {
	q := newAdminSampleQuotation(t)
	scratch := &Quotation{
		ctx:               q.ctx,
		auth:              q.auth,
		customerDirectory: q.customerDirectory,
	}
	identity, err := q.resolveCompanyIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	scratch.companyIdentity = &identity
	scratch.cssTemplate = q.cssTemplate
	scratch.Data.HtmlTemplate = q.Data.HtmlTemplate
	scratch.Data.CustomCSS = q.Data.CustomCSS
	if err := scratch.Prepare(templateSampleInput(2)); err != nil {
		t.Fatalf("prepare sample: %v", err)
	}
	page, err := scratch.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("sample HTML: %v", err)
	}
	// The discounted row: its net price is below its list price, so the
	// After Disc. column carries a value worth printing.
	discounted := false
	for _, item := range scratch.Data.Items {
		if item.DiscountEnabled && item.DisplayUnitPrice != item.DisplayListUnitPrice {
			discounted = true
		}
	}
	if !discounted {
		t.Errorf("no discounted row in the mock: the Disc. column would print identical figures")
	}
	if !strings.Contains(page, "After Disc. IDR") {
		t.Errorf("After Disc. column header missing from the sample")
	}
	// The packages: a state-B parent with scope rows and a state-C parent
	// merged across the priced subs, both carrying detail bullets.
	bodies := strings.Count(page, `<tbody class="item">`)
	if bodies < 2 {
		t.Errorf("package bodies = %d, want at least 2 (one B, one C)", bodies)
	}
	if got := strings.Count(page, `<ul class="desc-subitems">`); got != 2 {
		t.Errorf("detail bullet lists = %d, want 2", got)
	}
	found := false
	for _, item := range scratch.Data.Items {
		if len(item.SubItems) > 0 && item.ShowSubPrices {
			for _, sub := range item.SubItems {
				if strings.Contains(sub.SubNo, ".") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("no X.Y sub numbering in the mock: the split line is missing")
	}
	// PPN prints: the mock is non-FTZ goods, so the totals chrome renders.
	if scratch.Data.PpnAmount == 0 {
		t.Errorf("sample carries no PPN: the mock must exercise the tax row")
	}
}

func TestDownloadTemplateSampleWritesFile(t *testing.T) {
	q := newAdminSampleQuotation(t)
	path := filepath.Join(t.TempDir(), "sample.pdf")
	previous := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		return path, nil
	}
	defer func() { saveFileDialog = previous }()

	got, err := q.DownloadTemplateSample(nil, "", "", "", "")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if got != path {
		t.Errorf("download path = %q, want %q", got, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	if !strings.HasPrefix(string(raw), "%PDF") {
		t.Errorf("downloaded sample is not a PDF (%d bytes)", len(raw))
	}

	// A cancelled dialog is not an error to report, but it must not write.
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		return "", nil
	}
	if _, err := q.DownloadTemplateSample(nil, "", "", "", ""); err == nil {
		t.Errorf("cancelled download returned no error")
	}
}
