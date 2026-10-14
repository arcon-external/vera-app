package backend

// Template preview (issue #425).
//
// The Company settings Template tab edits the wording a customer reads and
// the four colours the document paints with, and staff need to see the real
// PDF beside the inputs -- not a client-side re-implementation that can drift
// from what an export prints. The two bindings here render a fixed mock
// quotation through the active engine (native gpdf in a default build, the
// browser under -tags chromedp) and hand back the document.
//
// The load-bearing property is statelessness. Prepare writes the shared
// instance's lastInput/Data and ExportTo reads lastInput back, so rendering
// the mock through the live binding would make the next export store and
// print the mock as a real signed revision (the #339 measurement). Both
// bindings below render on a scratch Quotation that carries copies of the
// template assets and never touches the receiver -- q.lastInput, q.Data and
// q.prepared read exactly as before afterwards. TestPreviewTemplateDocumentIsStateless
// pins it, including the export half: the bytes RenderPDF produces after a
// preview are the real document's, not the mock's.
//
// The mock is fixed on purpose: several plain lines, a discounted row so the
// Disc. column prints, a state-B package and a state-C package with detail
// bullets, and enough content to paginate to two pages -- the shapes most
// likely to break a template. The quote number is fake (customer 99 exists
// nowhere) and the QR points at a code that cannot verify, so a sample can
// never be mistaken for a document. No watermark: the sample is judged as the
// document looks.

import (
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/labels"
)

// sampleQuoteNumber is the number every template sample prints. Customer 99
// is not a client any company has, so no export can ever carry it.
const sampleQuoteNumber = "1599260001"

// sampleVerifyCode is the verification handle every template sample encodes.
// It is not hex, so it matches no real short_id: scanned, the portal answers
// not-found rather than verifying.
const sampleVerifyCode = "SAMPLE0000000000"

// sampleVerifyHost is the fallback host for the sample QR when no API client
// is configured (unit tests). A configured desktop points the code at its own
// portal instead, so staff scan the same portal a customer would.
const sampleVerifyHost = "https://api.arconpentapersada.co.id"

// templateSampleInput is the fixed mock the preview bindings render. It
// covers the shapes most likely to break a template: plain lines, one row
// carrying its own discount (so the Disc. column prints), a state-B package
// whose first part carries detail bullets, a state-C package with priced
// parts and a detail line, and enough rows to paginate to two pages.
//
// Everything but the customer is fixed; the customer is resolved by the
// caller from the cached directory, because the recipient block needs a real
// name and address to lay out like one.
func templateSampleInput(customerID uint8) QuotationInput {
	items := []QuotationItemInput{
		{Description: "Supply and install split-wall air conditioning units at administration block level 3", UoM: "Lot", UnitPrice: 18_500_000, Qty: 2, MarginEnabled: true, MarginPercent: 20},
		{Description: "Quarterly preventive maintenance for chiller plant including coil cleaning and controls check", UoM: "Month", UnitPrice: 4_750_000, Qty: 6, DiscountEnabled: true, DiscountPercent: 10},
		{Description: "Replacement of chilled-water pump seals and bearings", UoM: "set", UnitPrice: 6_200_000, Qty: 3},
		{Description: "Thermal insulation repair for exposed chilled-water piping at roof level", UoM: "m2", UnitPrice: 385_000, Qty: 40, MarginEnabled: true, MarginPercent: 35},
		{ // state B: subs priced internally, main carries the values; first
			// part carries detail bullets.
			Description: "Overhaul of air handling unit AHU-2 at production hall:",
			UoM:         "Lot",
			UnitPrice:   24_800_000,
			Qty:         1,
			SubItems: []QuotationSubItemInput{
				{Description: "Supply and fit replacement blower motor 15HP", UoM: "EA", UnitCost: 8_400_000, Qty: 1, MarginEnabled: true, MarginPercent: 20,
					SubSubItems: []QuotationSubSubItemInput{
						{Description: "Alignment and vibration testing after fit-off"},
						{Description: "Disposal of the removed motor"},
					}},
				{Description: "Replace filter bank and clean drain pan", UoM: "LS", UnitCost: 3_900_000, Qty: 1, MarginEnabled: true, MarginPercent: 30},
				{Description: "Belt drive set and pulley alignment", UoM: "LS", UnitCost: 2_150_000, Qty: 1},
			},
		},
		{Description: "Supply of copper refrigerant piping with fittings for VRF extension", UoM: "meter", UnitPrice: 265_000, Qty: 120, MarginEnabled: true, MarginPercent: 25},
		{Description: "Nitrogen pressure test and vacuum evacuation of extended refrigerant circuit", UoM: "LS", UnitPrice: 3_400_000, Qty: 1},
		{Description: "Commissioning and performance report for VRF extension", UoM: "LS", UnitPrice: 2_800_000, Qty: 1, DiscountEnabled: true, DiscountPercent: 5},
		{Description: "Annual service contract for toilet exhaust fans across three blocks", UoM: "Year", UnitPrice: 12_600_000, Qty: 1},
		{Description: "Emergency call-out labour for after-hours breakdown response", UoM: "Hour", UnitPrice: 450_000, Qty: 24, MarginEnabled: true, MarginPercent: 40},
		{Description: "Supply and replace exhaust fan belt guards and safety mesh", UoM: "Pc", UnitPrice: 175_000, Qty: 36},
		{Description: "Duct static pressure rebalancing after AHU-2 overhaul", UoM: "Day", UnitPrice: 1_900_000, Qty: 4},
		{ // state C: parent is a title, every sub priced; one part carries a
			// detail line.
			Description:   "Supply and install fresh-air make-up system at canteen:",
			UoM:           "Lot",
			UnitPrice:     31_200_000,
			Qty:           1,
			ShowSubPrices: true,
			SubItems: []QuotationSubItemInput{
				{Description: "Make-up air unit 5,000 CMH with pre-filter", UoM: "EA", UnitCost: 14_800_000, Qty: 1, MarginEnabled: true, MarginPercent: 18},
				{Description: "Spiral ductwork with hangers and diffusers", UoM: "LS", UnitCost: 9_300_000, Qty: 1, MarginEnabled: true, MarginPercent: 22,
					SubSubItems: []QuotationSubSubItemInput{
						{Description: "Fire damper at kitchen wall penetration"},
					}},
				{Description: "Electrical starter panel and control wiring", UoM: "LS", UnitCost: 4_100_000, Qty: 1},
				{Description: "Testing, adjusting and balancing with report", UoM: "LS", UnitCost: 2_200_000, Qty: 1},
			},
		},
		{Description: "Training session for building maintenance crew on new control panels", UoM: "Day", UnitPrice: 2_500_000, Qty: 2},
		{Description: "Spare filter set for make-up air unit first-year operation", UoM: "set", UnitPrice: 1_150_000, Qty: 4, MarginEnabled: true, MarginPercent: 30},
	}
	return QuotationInput{
		CustomerID:     customerID,
		Items:          items,
		Subject:        "Sample air-conditioning works for template preview",
		RecipientName:  "Sample Recipient",
		Incoterm:       "DAP",
		TaxZone:        TaxZoneNonFTZ,
		TaxType:        TaxTypeGoods,
		Validity:       30,
		PaymentTerm:    30,
		LeadTimeMonths: 1,
		LeadTimeWeeks:  2,
	}
}

// isPreviewHex reports whether a colour value is acceptable on the preview
// seam: empty means the stylesheet default, anything else must be a strict
// #RRGGBB. The store enforces the same rule at write time; the preview
// enforces it at render time because the colours here are unsaved drafts that
// have passed no write path yet.
func isPreviewHex(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, r := range value[1:] {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// resolveSampleCustomer picks the mock's customer from the cached directory:
// code 2 when it is there, otherwise the first entry, refreshing once on an
// empty cache. The sample needs a real recipient block to lay out like one.
func (q *Quotation) resolveSampleCustomer() (uint8, error) {
	for _, entry := range q.customerDirectory {
		if int(entry.Code) == 2 {
			return uint8(entry.Code), nil
		}
	}
	if len(q.customerDirectory) == 0 {
		if _, err := q.ListCustomers(); err != nil {
			return 0, fmt.Errorf("load client directory for template sample: %w", err)
		}
	}
	for _, entry := range q.customerDirectory {
		if int(entry.Code) == 2 {
			return uint8(entry.Code), nil
		}
	}
	if len(q.customerDirectory) > 0 {
		if code := q.customerDirectory[0].Code; code > 0 && code < 256 {
			return uint8(code), nil
		}
	}
	return 0, fmt.Errorf("no clients available for template sample")
}

// sampleVerifyBase is the portal host the sample QR points at: the
// configured API's own host, so staff scan the portal they would verify
// against, falling back to the public host when no client is configured.
func sampleVerifyBase() string {
	if client, err := api(); err == nil {
		if base := strings.TrimRight(client.BaseURL(), "/"); base != "" {
			return base
		}
	}
	return sampleVerifyHost
}

// renderTemplateSample builds the sample document for the given draft wording
// and colours. Entries carry Key/Value pairs in the served shape; a blank
// value clears that label back to its built-in default, exactly as the store
// resolves it. Colours are the four quotation bases, empty meaning default.
//
// Everything renders on a scratch instance: the receiver's lastInput, Data
// and prepared flag are never written, so a preview can never leak into the
// next export.
func (q *Quotation) renderTemplateSample(entries []CompanyLabelEntry, deep, accent, soft, ink string) ([]byte, error) {
	for field, value := range map[string]string{
		"deep": deep, "accent": accent, "soft": soft, "ink": ink,
	} {
		if !isPreviewHex(strings.TrimSpace(value)) {
			return nil, fmt.Errorf("template sample: %s colour %q is not a #RRGGBB colour", field, value)
		}
	}
	overrides := make(map[string]string, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.Key) == "" {
			continue
		}
		overrides[entry.Key] = entry.Value
	}
	customerID, err := q.resolveSampleCustomer()
	if err != nil {
		return nil, err
	}
	// The letterhead beneath the draft: the live profile's name, address,
	// currency and units, so the sample lays out like a real document from
	// this company. Only the wording and the four colours come from the
	// draft; everything else is what an export would print today.
	identity, err := q.resolveCompanyIdentity()
	if err != nil {
		return nil, fmt.Errorf("resolve company identity for template sample: %w", err)
	}
	identity.Labels = labels.ResolveOverrides(overrides)
	identity.ThemeQDeep = strings.TrimSpace(deep)
	identity.ThemeQAccent = strings.TrimSpace(accent)
	identity.ThemeQSoft = strings.TrimSpace(soft)
	identity.ThemeQInk = strings.TrimSpace(ink)

	// The scratch instance carries the template assets, the caller's context
	// and credentials, and the cached directory -- and nothing else is shared.
	// Prepare writes only here; the receiver it was borrowed from is never
	// written back to.
	scratch := &Quotation{
		ctx:               q.ctx,
		auth:              q.auth,
		customerDirectory: q.customerDirectory,
		companyIdentity:   &identity,
		cssTemplate:       q.cssTemplate,
	}
	scratch.Data.HtmlTemplate = q.Data.HtmlTemplate
	scratch.Data.CustomCSS = q.Data.CustomCSS
	scratch.Data.LogoBase64 = q.Data.LogoBase64
	scratch.Data.SignatureBase64 = q.Data.SignatureBase64
	scratch.Data.FontRegularBase64 = q.Data.FontRegularBase64
	scratch.Data.FontBoldBase64 = q.Data.FontBoldBase64

	if err := scratch.Prepare(templateSampleInput(customerID)); err != nil {
		return nil, fmt.Errorf("prepare template sample: %w", err)
	}
	scratch.Data.QuoteNumber = sampleQuoteNumber
	scratch.Data.RevisionID = 0
	if err := scratch.SetVerifyURL(sampleVerifyBase() + "/v/" + sampleVerifyCode); err != nil {
		return nil, fmt.Errorf("build sample verification code: %w", err)
	}
	return scratch.build()
}

// PreviewTemplateDocument renders the template sample for the given draft
// wording and colours and returns the PDF bytes. Admin-only: the wording and
// colours are company settings, and the editor that calls this is the admin
// Company form.
func (q *Quotation) PreviewTemplateDocument(entries []CompanyLabelEntry, deep, accent, soft, ink string) ([]byte, error) {
	if err := q.requireAdmin(q.ctx); err != nil {
		return nil, err
	}
	return q.renderTemplateSample(entries, deep, accent, soft, ink)
}

// DownloadTemplateSample renders the same sample PreviewTemplateDocument
// returns and saves it where the user chooses. It returns the written path,
// like Export and DownloadBackup do.
func (q *Quotation) DownloadTemplateSample(entries []CompanyLabelEntry, deep, accent, soft, ink string) (string, error) {
	if err := q.requireAdmin(q.ctx); err != nil {
		return "", err
	}
	pdf, err := q.renderTemplateSample(entries, deep, accent, soft, ink)
	if err != nil {
		return "", err
	}
	filePath, err := saveFileDialog(q.ctx, runtime.SaveDialogOptions{
		Title:           "Save Template Sample",
		DefaultFilename: "template-sample.pdf",
		Filters:         []runtime.FileFilter{{DisplayName: "PDF Files (*.pdf)", Pattern: "*.pdf"}},
	})
	if err != nil {
		return "", fmt.Errorf("open save dialog: %w", err)
	}
	if filePath == "" {
		return "", fmt.Errorf("user cancelled")
	}
	validatedPath, err := validateSavePath(filePath)
	if err != nil {
		return "", fmt.Errorf("validate sample path: %w", err)
	}
	if err := os.WriteFile(validatedPath, pdf, 0o644); err != nil {
		return "", fmt.Errorf("write template sample: %w", err)
	}
	return validatedPath, nil
}
