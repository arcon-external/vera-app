package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"os"
	"strconv"
	"strings"
	texttemplate "text/template"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/apicontract"
	"vera-app/backend/auth"
	"vera-app/backend/render"
	"vera-app/backend/render/quotation"
	"vera-app/backend/store"
)

// QuotationInput is the data contract between the frontend and the backend.
// The frontend sends this struct via Wails bindings when calling Prepare.
type QuotationInput struct {
	Incoterm        string
	Subject         string
	RecipientName   string
	TaxZone         string
	TaxCategory     string
	TaxType         string
	ServiceCategory string
	Items           []QuotationItemInput
	DiscountPercent uint8
	PaymentTerm     uint8
	LeadTime        uint8
	CustomerID      uint8
	Validity        uint8
}

// QuotationItemInput is a single line item from the frontend.
// No is assigned by the backend. UnitPrice is in raw IDR.
type QuotationItemInput struct {
	Description   string
	UoM           string
	UnitPrice     uint64
	Qty           uint32
	MarginPercent uint16
	MarginEnabled bool
}

// QuotationItem is a prepared line item with computed display fields
// ready for template rendering.
type QuotationItem struct {
	Description              string
	DisplayDesc              template.HTML
	UoM                      string
	DisplayOriginalUnitPrice string
	DisplayOriginalTotal     string
	DisplayUnitPrice         string
	DisplayTotal             string
	DisplayMarginAmount      string
	OriginalUnitPrice        uint64
	OriginalTotal            uint64
	MarginAmount             uint64
	UnitPrice                uint64
	Total                    uint64
	MarginEnabled            bool
	MarginPercent            uint16
	No                       uint32
	Qty                      uint32
}

// QuotationData is the full template data bag.
// It combines static template assets (set once at startup by readTemplate)
// with business data (set per-export by Prepare).
type QuotationData struct {
	HtmlTemplate              *template.Template
	DisplayDiscountAmount     string
	TaxZone                   string
	LogoBase64                string
	SignatureBase64           string
	FontRegularBase64         string
	FontBoldBase64            string
	Date                      string
	ValidUntil                string
	CompanyName               string
	Incoterm                  string
	ThemeQDeep                string
	ThemeQAccent              string
	ThemeQSoft                string
	ThemeQInk                 string
	RecipientName             string
	RecipientCompany          string
	RecipientAddress          string
	QuoteNumber               string
	QuoteUUID                 string
	VerifyURL                 string
	VerifyQRBase64            string
	VerifyShortID             string
	ServiceCategory           string
	Subject                   string
	DisplaySubTotal           string
	TaxType                   string
	CustomCSS                 template.CSS
	DisplayGrandTotal         string
	CompanyAddress            string
	TaxCategory               string
	DisplayNettProfit         string
	DisplayTaxAmount          string
	TaxLabel                  string
	DisplayPpnAmount          string
	Items                     []QuotationItem
	GrandTotal                uint64
	SellGrandTotal            uint64
	TaxAmount                 uint64
	DiscountAmount            uint64
	OriginalGrandTotal        uint64
	NettProfit                int64
	OriginalDiscountAmount    uint64
	OriginalSubTotal          uint64
	ProfitAmount              int64
	MarginalizedGrandAfterTax uint64
	PpnAmount                 uint64
	SubTotal                  uint64
	TaxRateBps                uint16
	RevisionID                uint16
	DiscountPercent           uint8
	LeadTime                  uint8
	PaymentTerm               uint8
	CustomerID                uint8
	Validity                  uint8
}

// Quotation is the Wails-bound application struct.
// It owns the Wails context and the full data bag for rendering.
type Quotation struct {
	ctx                context.Context
	store              *store.Store
	auth               *auth.Service
	companyIdentity    *CompanyIdentity
	cssTemplate        *texttemplate.Template
	archiveQuoteNumber string
	customerDirectory  []ManagedClient
	lastInput          QuotationInput
	Data               QuotationData
	archiveRevision    uint16
	prepared           bool
}

// CompanyIdentity is the quotation-relevant slice of the company profile:
// name and address are the only profile values the template prints.
type CompanyIdentity struct {
	Name    string
	Address string
	// Theme carries the quotation brand colors. Empty values are valid and
	// mean "use the stylesheet default" (the template guards each one).
	ThemeQDeep   string
	ThemeQAccent string
	ThemeQSoft   string
	ThemeQInk    string
}

// DefaultCompanyIdentity is the last-resort header when the stored profile
// is unreachable (offline desktop with an empty cache). It is a fallback,
// never data: reachable paths always print stored values.
func DefaultCompanyIdentity() CompanyIdentity {
	return CompanyIdentity{
		Name:    "PT Arcon Penta Persada",
		Address: "Perkantoran Graha Kadin Blk C no. 6 Jl. Engku Putri, Kel. Belian, Kec. Batam, Kota Batam 29433, Indonesia",
	}
}

// RefreshCompanyIdentity reloads the cached company profile from the
// server. The Company form calls it after every save so the next preview
// prints fresh values without a restart.
func (q *Quotation) RefreshCompanyIdentity() error {
	if err := q.requireUser(q.ctx); err != nil {
		return err
	}
	client, err := api()
	if err != nil {
		return err
	}
	var profile CompanyProfile
	if err := client.Get("/v1/company", &profile); err != nil {
		return err
	}
	q.companyIdentity = &CompanyIdentity{
		Name:         profile.LegalName,
		Address:      profile.Address,
		ThemeQDeep:   profile.ThemeQDeep,
		ThemeQAccent: profile.ThemeQAccent,
		ThemeQSoft:   profile.ThemeQSoft,
		ThemeQInk:    profile.ThemeQInk,
	}
	// The desktop preview renders from this instance's template data, so it
	// needs the uploaded artwork too — otherwise previews would show the
	// baked logo while server PDFs show the upload.
	if profile.HasLogo {
		var artwork struct {
			LogoBase64 string `json:"logoBase64"`
			LogoMime   string `json:"logoMime"`
		}
		if err := client.Get("/v1/admin/company/logo", &artwork); err != nil {
			return err
		}
		if artwork.LogoBase64 == "" {
			return fmt.Errorf("company has no uploaded logo")
		}
		q.Data.LogoBase64 = artwork.LogoBase64
	}
	return nil
}

// resolveCompanyIdentity maps the stored profile to header values. The
// server path resolves via the store (authoritative, fresh every render);
// the desktop path uses the cache, fetching once when empty and falling
// back to built-in defaults only when unreachable.
func (q *Quotation) resolveCompanyIdentity() (CompanyIdentity, error) {
	if q.store != nil {
		profile, err := q.store.GetCompanyProfile(q.context())
		if err != nil {
			return CompanyIdentity{}, err
		}
		return CompanyIdentity{
			Name:         profile.LegalName,
			Address:      profile.Address,
			ThemeQDeep:   profile.ThemeQDeep,
			ThemeQAccent: profile.ThemeQAccent,
			ThemeQSoft:   profile.ThemeQSoft,
			ThemeQInk:    profile.ThemeQInk,
		}, nil
	}
	if q.companyIdentity == nil {
		if err := q.RefreshCompanyIdentity(); err != nil {
			return DefaultCompanyIdentity(), nil
		}
	}
	identity := *q.companyIdentity
	if strings.TrimSpace(identity.Name) == "" {
		identity.Name = DefaultCompanyIdentity().Name
	}
	return identity, nil
}

func (q *Quotation) SetStore(database *store.Store) {
	q.store = database
}

// SetAuthService attaches the authentication service used to gate methods.
func (q *Quotation) SetAuthService(service *auth.Service) {
	q.auth = service
}

// requireUser denies callers without a signed-in user. Auth enforcement is
// nil-safe in tests: methods using it must set the service in production.
func (q *Quotation) requireUser(ctx context.Context) error {
	if q.auth != nil {
		_, err := q.auth.RequireUser(ctx)
		return err
	}
	if SessionToken() == "" {
		return fmt.Errorf("not signed in")
	}
	return nil
}

// requireAdmin denies callers without an admin role.
func (q *Quotation) requireAdmin(ctx context.Context) error {
	if q.auth != nil {
		_, err := q.auth.RequireAdmin(ctx)
		return err
	}
	// Without a local auth service the role is enforced server-side, which
	// surfaces 403 for staff. A token must still be present to call at all.
	if SessionToken() == "" {
		return fmt.Errorf("not signed in")
	}
	return nil
}

// SetRevisionContext tells Export whether this is a new quotation or the
// next immutable revision of an archived quotation.
func (q *Quotation) SetRevisionContext(quoteNumber string, revision uint16) error {
	if err := q.requireUser(q.ctx); err != nil {
		return err
	}
	if strings.TrimSpace(quoteNumber) == "" {
		return fmt.Errorf("quote number is required for a revision")
	}
	q.archiveQuoteNumber = strings.TrimSpace(quoteNumber)
	q.archiveRevision = revision
	q.Data.QuoteNumber = q.archiveQuoteNumber
	q.Data.RevisionID = revision
	return nil
}

func (q *Quotation) ClearRevisionContext() {
	q.archiveQuoteNumber = ""
	q.archiveRevision = 0
	q.Data.QuoteNumber = ""
	q.Data.RevisionID = 0
}

// Startup initialises the Quotation struct at app launch.
// It stores the Wails context (required for runtime.SaveFileDialog)
// and loads all static template assets (CSS, HTML, fonts, logo, signature).
func Startup(quotation *Quotation, ctx context.Context) error {
	quotation.ctx = ctx

	err := readTemplate(quotation)
	if err != nil {
		return err
	}

	return nil
}

// ListCustomers fetches the server-owned client directory for the editor
// dropdown and caches it for Prepare resolution.
func (q *Quotation) ListCustomers() ([]ManagedClient, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return nil, err
	}
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Clients []ManagedClient `json:"clients"`
	}
	if err := client.Get("/v1/clients", &response); err != nil {
		return nil, err
	}
	if response.Clients == nil {
		response.Clients = []ManagedClient{}
	}
	q.customerDirectory = response.Clients
	return response.Clients, nil
}

// resolveCustomer maps a client code to display name and default address.
// The server path resolves via the store (authoritative); the desktop path
// resolves from the cached directory, refreshing once on a miss (a client
// added on another device after sign-in).
func (q *Quotation) resolveCustomer(id uint8) (company, address string, err error) {
	if q.store != nil {
		ref, err := q.store.GetCustomerByCode(q.context(), int(id))
		if err != nil {
			return "", "", fmt.Errorf("invalid customer ID: %d", id)
		}
		return ref.Name, ref.Address, nil
	}
	lookup := func() (string, string, bool) {
		for _, entry := range q.customerDirectory {
			if int(entry.Code) == int(id) {
				return entry.Name, entry.PrimaryAddress, true
			}
		}
		return "", "", false
	}
	if company, address, ok := lookup(); ok {
		return company, address, nil
	}
	if _, err := q.ListCustomers(); err != nil {
		return "", "", fmt.Errorf("invalid customer ID: %d (refresh the client list and try again)", id)
	}
	if company, address, ok := lookup(); ok {
		return company, address, nil
	}
	return "", "", fmt.Errorf("invalid customer ID: %d", id)
}

// FormatQuoteNumber renders a plain-digit quotation number for a customer,
// issue year, and sequence. It is the shared implementation used by desktop
// export and the API server; the canonical format lives in apicontract so
// store can build numbers without an import cycle.
func FormatQuoteNumber(cusID uint8, yy uint8, seq int64) string {
	return apicontract.FormatQuoteNumber(int(cusID), int(yy), seq)
}

// NewQuotationForRequest builds a request-scoped quotation wired with the
// store, the caller's identity, and preloaded template assets. The API server
// uses one instance per request because Quotation carries mutable state.
func NewQuotationForRequest(database *store.Store, user store.AuthUser, templates *Quotation) *Quotation {
	quotation := &Quotation{}
	quotation.SetStore(database)
	quotation.SetAuthService(auth.NewStaticService(user, nil))
	if templates != nil {
		quotation.Data.HtmlTemplate = templates.Data.HtmlTemplate
		quotation.Data.CustomCSS = templates.Data.CustomCSS
		quotation.Data.LogoBase64 = templates.Data.LogoBase64
		quotation.Data.SignatureBase64 = templates.Data.SignatureBase64
		quotation.Data.FontRegularBase64 = templates.Data.FontRegularBase64
		quotation.Data.FontBoldBase64 = templates.Data.FontBoldBase64
		quotation.cssTemplate = templates.cssTemplate
	}
	return quotation
}

// refreshCustomCSS re-executes the parsed stylesheet against the current
// data so theme values set by Prepare reach the CSS. A nil parsed template
// (tests that never ran Startup) leaves the CSS untouched.
func (q *Quotation) refreshCustomCSS() error {
	if q.cssTemplate == nil {
		return nil
	}
	var cssBuf strings.Builder
	if err := q.cssTemplate.Execute(&cssBuf, q.Data); err != nil {
		return fmt.Errorf("execute css template: %w", err)
	}
	q.Data.CustomCSS = template.CSS(cssBuf.String())
	return nil
}

// DraftRevision builds the immutable archive draft for prepared data. The
// caller supplies the rendered HTML snapshot and optional PDF bytes/path;
// PDF attachment may arrive later via the document endpoint.
func (q *Quotation) DraftRevision(htmlSnapshot string, pdfData []byte, pdfPath string) (store.RevisionDraft, error) {
	if !q.prepared {
		return store.RevisionDraft{}, fmt.Errorf("no prepared quotation: call Prepare first")
	}
	return q.revisionDraft(htmlSnapshot, pdfData, pdfPath), nil
}

// CustomerID returns the prepared quotation's customer ID.
func (q *Quotation) CustomerID() uint8 {
	return q.Data.CustomerID
}

// SetVerifyURL records the public verification address and renders its QR
// image for the document. It is set once the revision's verification handle
// exists — after the server has allocated the revision.
func (q *Quotation) SetVerifyURL(url string) error {
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		q.Data.VerifyURL = ""
		q.Data.VerifyQRBase64 = ""
		q.Data.VerifyShortID = ""
		return nil
	}
	encoded, err := BuildVerificationQR(trimmed)
	if err != nil {
		return err
	}
	q.Data.VerifyURL = trimmed
	q.Data.VerifyQRBase64 = encoded
	q.Data.VerifyShortID = trimmed[strings.LastIndex(trimmed, "/")+1:]
	return nil
}

// VerifyURL returns the verification address currently bound to the
// document, if any.
func (q *Quotation) VerifyURL() string { return q.Data.VerifyURL }

// SetQuoteNumber assigns an allocated quote number and revision, used by the
// API server which allocates sequences itself instead of showing a dialog.
func (q *Quotation) SetQuoteNumber(quoteNumber string, revision uint16) {
	q.Data.QuoteNumber = quoteNumber
	q.Data.RevisionID = revision
}

// uoMMaxLen is the longest a unit of measure may be.
//
// This is a bound, not an allowlist, and the distinction matters. The obvious
// rule -- the UoM must be one of the five values in the editor's UOM_OPTIONS --
// was tried and is wrong: the test fixtures carry "Month" for a monthly
// maintenance line and "Unit" for several others, neither of which the editor
// offers, so a staff member preparing a service quotation cannot set them
// through the UI at all. Enforcing the five here would reject legitimate work on
// the server while fixing nothing the editor does not already prevent.
//
// So the backend checks the one thing it can check without inventing policy: that
// the value is shaped like a unit label rather than like a sentence. That is the
// actual harm. A pasted description whose cell contained a tab arrives as two
// columns and its tail was stored as the UoM, then printed in the customer
// PDF's UoM column, where it overflowed into the unit price beside it. A unit
// label is a short token; "Repair leaking pipe 2 inch RO reject at toilet block
// 1" is not.
const uoMMaxLen = 12

// plausibleUoM reports whether s could be a unit of measure: a short token of
// letters and digits, allowing the punctuation units legitimately use ("m2",
// "linear-m", "pack/6").
//
// A UoM that is not plausible is a value that no unit label would take, and the
// only way one arrives is a field mix-up — so it is refused rather than printed
// on a document a customer sees.
func plausibleUoM(s string) bool {
	if s == "" || len(s) > uoMMaxLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '/', r == '.':
		default:
			return false
		}
	}
	return true
}

// MaxMarginPercent is the highest profit margin a line may carry.
//
// 1000% means selling at eleven times cost, which is ordinary for a low-cost
// fitting on a service visit — the previous ceiling of 100% capped those at
// double cost, which is not enough to build a quote around.
//
// It is exported because the editor clamps to the same number, and a limit that
// exists in two places without a shared name is a limit that eventually
// disagrees with itself. Discount percentage is a different setting and keeps
// its own, much lower, ceiling of 100.
const MaxMarginPercent = 1000

// Prepare validates user input and populates the QuotationData business fields.
// It does not generate the quote number — that is deferred to Export to avoid
// consuming a sequence number if the user never saves. Returns an error
// describing the first validation failure, or nil on success.
func (q *Quotation) Prepare(input QuotationInput) error {
	if err := q.requireUser(q.ctx); err != nil {
		return err
	}
	companyName, companyAddress, err := q.resolveCustomer(input.CustomerID)
	if err != nil {
		return err
	}

	if len(input.Items) < 1 {
		return fmt.Errorf("at least one item is required")
	}

	for i, item := range input.Items {
		if strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("item description is required")
		}

		if item.Qty == 0 {
			return fmt.Errorf("item quantity must be greater than zero")
		}

		// The UoM is the one field the editor cannot render an arbitrary value
		// into -- a <select> bound to the stored value shows something else when
		// the value is not one of the options -- so a value that slipped through
		// used to reach the customer PDF silently, printed in the UoM column and
		// overflowing into the unit price beside it. See plausibleUoM for why this
		// is a bound rather than a list of the five units the editor offers.
		uom := strings.TrimSpace(item.UoM)
		if uom == "" {
			return fmt.Errorf("item %d: unit of measure is required", i+1)
		}
		if !plausibleUoM(uom) {
			return fmt.Errorf("item %d: unit of measure %q is not a unit", i+1, item.UoM)
		}

		if item.MarginPercent > MaxMarginPercent {
			return fmt.Errorf("item margin percent cannot be greater than %d", MaxMarginPercent)
		}
	}

	if input.Subject == "" {
		return fmt.Errorf("subject is required")
	}

	if strings.TrimSpace(input.RecipientName) == "" {
		return fmt.Errorf("recipient name are required")
	}

	if input.Validity < 1 {
		return fmt.Errorf("validity must be greater than zero")
	}

	if input.PaymentTerm < 1 {
		return fmt.Errorf("payment term must be greater than zero")
	}

	if input.LeadTime < 1 {
		return fmt.Errorf("lead time must be greater than zero")
	}

	if input.DiscountPercent > 100 {
		return fmt.Errorf("discount percent cannot be greater than 100")
	}

	if input.TaxZone == "" {
		input.TaxZone = TaxZoneFTZ
	}
	taxType, serviceCategory := input.TaxType, input.ServiceCategory
	if taxType == "" {
		// Old clients send only TaxCategory; map it forward so mixed-version
		// rollouts keep working. Old FTZ sends nothing at all: it is goods.
		// An old category alongside FTZ stays rejected, as before.
		if input.TaxZone == TaxZoneFTZ {
			if input.TaxCategory != "" {
				return fmt.Errorf("FTZ must not have a tax category")
			}
			taxType = TaxTypeGoods
		} else {
			var ok bool
			taxType, serviceCategory, ok = legacyTaxCategoryToNew(input.TaxCategory)
			if !ok {
				return fmt.Errorf("invalid tax category: %q", input.TaxCategory)
			}
		}
	}
	ppnBps, servicesBps, taxLabel, err := resolveTax(input.TaxZone, taxType, serviceCategory)
	if err != nil {
		return err
	}

	validIncoterms := map[string]bool{"EXW": true, "FCA": true, "FAS": true, "FOB": true, "CFR": true, "CIF": true, "CPT": true, "CIP": true, "DPU": true, "DAP": true, "DDP": true}
	if !validIncoterms[strings.ToUpper(input.Incoterm)] {
		return fmt.Errorf("invalid incoterm: %s", input.Incoterm)
	}

	q.Data.CustomerID = input.CustomerID

	q.Data.RecipientName = input.RecipientName
	q.Data.RecipientCompany = companyName
	q.Data.RecipientAddress = companyAddress

	identity, err := q.resolveCompanyIdentity()
	if err != nil {
		return err
	}
	q.Data.CompanyName = identity.Name
	q.Data.CompanyAddress = identity.Address
	q.Data.ThemeQDeep = identity.ThemeQDeep
	q.Data.ThemeQAccent = identity.ThemeQAccent
	q.Data.ThemeQSoft = identity.ThemeQSoft
	q.Data.ThemeQInk = identity.ThemeQInk

	q.Data.Subject = input.Subject
	q.Data.Validity = input.Validity
	q.Data.PaymentTerm = input.PaymentTerm
	q.Data.LeadTime = input.LeadTime
	q.Data.DiscountPercent = input.DiscountPercent
	q.Data.TaxZone = input.TaxZone
	q.Data.TaxCategory = input.TaxCategory
	q.Data.TaxType = taxType
	q.Data.ServiceCategory = serviceCategory
	q.Data.Incoterm = input.Incoterm

	items := buildItems(input.Items)
	q.Data.Items = items

	date, validUntil := buildDates(input.Validity)
	q.Data.Date = date
	q.Data.ValidUntil = validUntil

	pricing := buildPricing(items, input.DiscountPercent, ppnBps, servicesBps)
	q.Data.SubTotal = pricing.MarginalizedSubtotal
	q.Data.DiscountAmount = pricing.MarginalizedDiscount
	q.Data.GrandTotal = pricing.CustomerGrandTotal
	q.Data.SellGrandTotal = pricing.MarginalizedGrand
	q.Data.OriginalSubTotal = pricing.OriginalSubtotal
	q.Data.OriginalDiscountAmount = pricing.OriginalDiscount
	q.Data.OriginalGrandTotal = pricing.OriginalGrand
	q.Data.MarginalizedGrandAfterTax = pricing.MarginalizedGrandAfterTax
	q.Data.ProfitAmount = pricing.ProfitAmount
	q.Data.PpnAmount = pricing.PpnAmount

	q.Data.TaxRateBps = ppnBps + servicesBps
	q.Data.TaxAmount = pricing.TaxAmount
	q.Data.NettProfit = pricing.NettProfit
	q.Data.TaxLabel = taxLabel

	displaySubTotal, displayDiscountAmount, displayGrandTotal := buildViews(
		pricing.MarginalizedSubtotal,
		pricing.MarginalizedDiscount,
		pricing.CustomerGrandTotal,
	)
	q.Data.DisplaySubTotal = displaySubTotal
	q.Data.DisplayDiscountAmount = displayDiscountAmount
	q.Data.DisplayGrandTotal = displayGrandTotal
	q.Data.DisplayPpnAmount = formatRupiah(pricing.PpnAmount)
	q.Data.DisplayTaxAmount = formatRupiah(pricing.TaxAmount)
	q.Data.DisplayNettProfit = formatSignedRupiah(q.Data.NettProfit)

	q.lastInput = input
	q.prepared = true

	// Theme values are set above; re-execute the stylesheet so they reach
	// the CSS. A render without Startup (unit tests) keeps whatever CSS it
	// already holds.
	if err := q.refreshCustomCSS(); err != nil {
		return err
	}

	return nil
}

// saveFileDialog opens the native save dialog. It is a variable so tests can
// substitute a stub; production uses the Wails runtime dialog.
var saveFileDialog = runtime.SaveFileDialog

// PickDirectory opens a native folder picker for the Settings export
// default. An empty string means the user cancelled; callers treat it as
// "keep the current setting".
func (q *Quotation) PickDirectory() (string, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return "", err
	}
	return runtime.OpenDirectoryDialog(q.ctx, runtime.OpenDialogOptions{
		Title: "Choose export folder",
	})
}

// exportFilename previews the Arcon_Quotation name for the save dialog.
// Archive re-exports already know their number; new quotations peek the next
// full number (server owns the format) without consuming it. Any failure
// falls back to quotation.pdf so export always proceeds; the server
// allocates authoritatively at export.
func (q *Quotation) exportFilename() string {
	if q.archiveQuoteNumber != "" {
		return BuildExportFilename(q.archiveQuoteNumber, q.archiveRevision, q.lastInput.Subject)
	}
	var number string
	if client, err := api(); err == nil {
		var response struct {
			Number string `json:"number"`
		}
		if err := client.Get("/v1/quotations/next-sequence?customerId="+strconv.Itoa(int(q.lastInput.CustomerID)), &response); err == nil {
			number = response.Number
		}
	}
	if number == "" {
		return "quotation.pdf"
	}
	return BuildExportFilename(number, 0, q.lastInput.Subject)
}

// Export opens a native save file dialog, persists the revision through the
// API server (which allocates the quote number), builds the PDF locally,
// writes it to the chosen path, and uploads the bytes back to the server.
// The quote number is only consumed after the user confirms the save dialog,
// preventing wasted sequence increments on cancellation. defaultDir is the
// caller's preferred starting folder (the Settings export default); blank
// or missing folders fall back to the OS default silently.
func (q *Quotation) Export(defaultDir string) error {
	if err := q.requireUser(q.ctx); err != nil {
		return err
	}
	client, err := api()
	if err != nil {
		return err
	}

	options := runtime.SaveDialogOptions{
		Title:           "Save Quotation",
		DefaultFilename: q.exportFilename(),
		Filters:         []runtime.FileFilter{{DisplayName: "PDF Files (*.pdf)", Pattern: "*.pdf"}},
	}
	if dir := strings.TrimSpace(defaultDir); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			options.DefaultDirectory = dir
		}
	}

	filePath, err := saveFileDialog(q.ctx, options)
	if err != nil {
		return fmt.Errorf("open save dialog: %w", err)
	}

	if filePath == "" {
		return fmt.Errorf("user cancelled")
	}

	validatedPath, err := validateSavePath(filePath)
	if err != nil {
		return fmt.Errorf("validate quotation path: %w", err)
	}

	payload := map[string]any{"input": q.lastInput}
	if q.archiveQuoteNumber != "" {
		payload["quoteNumber"] = q.archiveQuoteNumber
		payload["revision"] = q.archiveRevision
	}
	var exported apicontract.ExportResponse
	if err := client.Post("/v1/quotations/export", payload, &exported); err != nil {
		return err
	}
	if exported.QuoteNumber == "" {
		return fmt.Errorf("export quotation: empty quote number from server")
	}
	q.Data.QuoteNumber = exported.QuoteNumber
	q.Data.QuoteUUID = exported.UUID
	q.Data.RevisionID = uint16(exported.Revision)

	// Bind the printed document to its verification address. The server
	// already rendered the same link into the stored snapshot, so the PDF
	// this client builds and the archived copy agree.
	if exported.ShortID != "" {
		if err := q.SetVerifyURL(client.BaseURL() + "/v/" + exported.ShortID); err != nil {
			return fmt.Errorf("build verification code: %w", err)
		}
	}

	pdfBuf, err := q.build()
	if err != nil {
		return fmt.Errorf("build quotation: %w", err)
	}
	if err := os.WriteFile(validatedPath, pdfBuf, 0o644); err != nil {
		return fmt.Errorf("write quotation: %w", err)
	}
	var document struct {
		HasPDF bool `json:"hasPdf"`
	}
	if err := client.Put(
		"/v1/quotations/"+strconv.FormatInt(exported.ID, 10)+
			"/revisions/"+strconv.Itoa(int(exported.Revision))+"/document",
		map[string]string{
			"pdfBase64": base64.StdEncoding.EncodeToString(pdfBuf),
			"pdfPath":   validatedPath,
		},
		&document,
	); err != nil {
		return fmt.Errorf("attach quotation PDF: %w", err)
	}

	return nil
}

func (q *Quotation) context() context.Context {
	if q.ctx == nil {
		return context.Background()
	}
	return q.ctx
}

func (q *Quotation) revisionDraft(htmlSnapshot string, pdfData []byte, pdfPath string) store.RevisionDraft {
	items := make([]store.RevisionItem, 0, len(q.Data.Items))
	for _, item := range q.Data.Items {
		items = append(items, store.RevisionItem{
			ItemNo:                int(item.No),
			Description:           item.Description,
			Qty:                   int64(item.Qty),
			UoM:                   item.UoM,
			UnitPrice:             int64(item.OriginalUnitPrice),
			MarginEnabled:         item.MarginEnabled,
			MarginPercent:         int16(item.MarginPercent),
			MarginAmount:          int64(item.MarginAmount),
			OriginalTotal:         int64(item.OriginalTotal),
			MarginalizedUnitPrice: int64(item.UnitPrice),
			MarginalizedTotal:     int64(item.Total),
		})
	}

	return store.RevisionDraft{
		QuoteNumber:      q.Data.QuoteNumber,
		CustomerCode:     int16(q.Data.CustomerID),
		CustomerName:     q.Data.RecipientCompany,
		CustomerAddress:  q.Data.RecipientAddress,
		RevisionNumber:   int16(q.Data.RevisionID),
		RecipientName:    q.Data.RecipientName,
		RecipientAddress: q.Data.RecipientAddress,
		Subject:          q.Data.Subject,
		Incoterm:         q.Data.Incoterm,
		ValidityDays:     int16(q.Data.Validity),
		PaymentTermDays:  int16(q.Data.PaymentTerm),
		LeadTimeDays:     int16(q.Data.LeadTime),
		DiscountPercent:  int16(q.Data.DiscountPercent),
		TaxZone:          q.Data.TaxZone,
		TaxCategory:      q.Data.TaxCategory,
		TaxType:          q.Data.TaxType,
		ServiceCategory:  q.Data.ServiceCategory,
		TaxRateBps:       int16(q.Data.TaxRateBps),
		Items:            items,
		Totals: store.RevisionTotals{
			OriginalSubtotal:          int64(q.Data.OriginalSubTotal),
			OriginalDiscount:          int64(q.Data.OriginalDiscountAmount),
			OriginalGrand:             int64(q.Data.OriginalGrandTotal),
			MarginalizedSubtotal:      int64(q.Data.SubTotal),
			MarginalizedDiscount:      int64(q.Data.DiscountAmount),
			MarginalizedGrand:         int64(q.Data.SellGrandTotal),
			TaxAmount:                 int64(q.Data.TaxAmount),
			MarginalizedGrandAfterTax: int64(q.Data.MarginalizedGrandAfterTax),
			ProfitAmount:              q.Data.ProfitAmount,
			NettProfit:                q.Data.NettProfit,
			PpnAmount:                 int64(q.Data.PpnAmount),
			CustomerGrandTotal:        int64(q.Data.GrandTotal),
		},
		HTMLSnapshot: htmlSnapshot,
		PDFData:      pdfData,
		PDFPath:      pdfPath,
	}
}

// build renders the customer PDF through the application's selected renderer.
//
// The choice itself lives in backend/render, in one line, and is a compile-time
// edit rather than a setting. This function's whole job is to hand the renderer
// a renderer-neutral view of the quotation and get bytes back.
func (q *Quotation) build() ([]byte, error) {
	in, err := q.renderInput()
	if err != nil {
		return nil, err
	}
	return render.Active.Render(q.ctx, in)
}

// renderInput builds the renderer's view of this quotation.
//
// The HTML template is executed here rather than inside the browser renderer,
// because executing it is a job of the application -- the renderer is handed a
// document, not a template. It also means the browser renderer reads exactly
// one field of the result, which is what keeps the two implementations from
// drifting into sharing a convention that only one of them honours.
func (q *Quotation) renderInput() (quotation.Input, error) {
	var body bytes.Buffer
	if err := q.Data.HtmlTemplate.Execute(&body, q.Data); err != nil {
		return quotation.Input{}, fmt.Errorf("execute quotation template: %w", err)
	}

	if body.Len() == 0 {
		return quotation.Input{}, fmt.Errorf("template executed but produced empty output")
	}

	logo, err := decodeDataURL(q.Data.LogoBase64)
	if err != nil {
		return quotation.Input{}, fmt.Errorf("decode logo: %w", err)
	}
	qr, err := decodeDataURL(q.Data.VerifyQRBase64)
	if err != nil {
		return quotation.Input{}, fmt.Errorf("decode verification qr: %w", err)
	}

	items := make([]quotation.Item, 0, len(q.Data.Items))
	for _, it := range q.Data.Items {
		items = append(items, quotation.Item{
			No:          int(it.No),
			DisplayDesc: string(it.DisplayDesc),
			Qty:         it.Qty,
			UoM:         it.UoM,
			UnitPrice:   it.DisplayUnitPrice,
			Total:       it.DisplayTotal,
		})
	}

	return quotation.Input{
		HTML:  body.String(),
		Fonts: backAssets,

		Theme: quotation.Theme{
			Deep:   q.Data.ThemeQDeep,
			Accent: q.Data.ThemeQAccent,
			Soft:   q.Data.ThemeQSoft,
			Ink:    q.Data.ThemeQInk,
		},
		LogoPNG: logo,
		QRPNG:   qr,

		CompanyName:    q.Data.CompanyName,
		CompanyAddress: q.Data.CompanyAddress,

		Subject:          q.Data.Subject,
		RecipientName:    q.Data.RecipientName,
		RecipientCompany: q.Data.RecipientCompany,
		RecipientAddress: q.Data.RecipientAddress,

		QuoteNumber: q.Data.QuoteNumber,
		Date:        q.Data.Date,
		ValidUntil:  q.Data.ValidUntil,
		RevisionID:  q.Data.RevisionID,

		Validity:        q.Data.Validity,
		PaymentTerm:     q.Data.PaymentTerm,
		LeadTime:        q.Data.LeadTime,
		Incoterm:        q.Data.Incoterm,
		ServiceCategory: q.Data.ServiceCategory,

		Items: items,

		DisplaySubTotal:   q.Data.DisplaySubTotal,
		DisplayDiscount:   q.Data.DisplayDiscountAmount,
		DisplayPpn:        q.Data.DisplayPpnAmount,
		DisplayGrandTotal: q.Data.DisplayGrandTotal,
		DiscountPercent:   q.Data.DiscountPercent,

		VerificationCode: q.Data.VerifyShortID,
	}, nil
}

// decodeDataURL turns an embedded data URL or bare base64 payload into bytes.
func decodeDataURL(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	if i := strings.Index(s, ","); strings.HasPrefix(s, "data:") && i > 0 {
		s = s[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	return raw, nil
}
