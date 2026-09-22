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
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/apicontract"
	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// QuotationInput is the data contract between the frontend and the backend.
// The frontend sends this struct via Wails bindings when calling Prepare.
type QuotationInput struct {
	Items           []QuotationItemInput
	Subject         string
	RecipientName   string
	DiscountPercent uint8
	TaxZone         string
	TaxCategory     string
	TaxType         string
	ServiceCategory string
	Validity        uint8
	PaymentTerm     uint8
	LeadTime        uint8
	CustomerID      uint8
	Incoterm        string
}

// QuotationItemInput is a single line item from the frontend.
// No is assigned by the backend. UnitPrice is in raw IDR.
type QuotationItemInput struct {
	Description   string
	Qty           uint32
	UoM           string
	UnitPrice     uint64
	MarginEnabled bool
	MarginPercent uint8
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
	MarginPercent            uint8
	No                       uint32
	Qty                      uint32
}

// QuotationData is the full template data bag.
// It combines static template assets (set once at startup by readTemplate)
// with business data (set per-export by Prepare).
type QuotationData struct {
	Items             []QuotationItem
	CustomCSS         template.CSS
	HtmlTemplate      *template.Template
	LogoBase64        string
	SignatureBase64   string
	FontRegularBase64 string
	FontBoldBase64    string
	Date              string
	ValidUntil        string
	CompanyName       string
	CompanyAddress    string
	RecipientName     string
	RecipientCompany  string
	RecipientAddress  string
	QuoteNumber       string
	QuoteUUID         string
	// VerifyURL is the public verification address printed as a QR code on
	// the document. Empty before export (the handle does not exist yet), in
	// which case the template omits the code rather than printing a dead one.
	VerifyURL      string
	VerifyQRBase64 string
	// VerifyShortID is the handle inside VerifyURL, printed in the caption so
	// a reader (or a support call) can reference the document without
	// scanning it.
	VerifyShortID             string
	RevisionID                uint16
	Subject                   string
	DisplaySubTotal           string
	DisplayDiscountAmount     string
	DisplayGrandTotal         string
	SubTotal                  uint64
	DiscountAmount            uint64
	GrandTotal                uint64
	SellGrandTotal            uint64
	OriginalSubTotal          uint64
	OriginalDiscountAmount    uint64
	OriginalGrandTotal        uint64
	MarginalizedGrandAfterTax uint64
	ProfitAmount              int64
	TaxRateBps                uint16
	TaxAmount                 uint64
	PpnAmount                 uint64
	DisplayPpnAmount          string
	NettProfit                int64
	TaxLabel                  string
	DisplayTaxAmount          string
	DisplayNettProfit         string
	Validity                  uint8
	PaymentTerm               uint8
	LeadTime                  uint8
	DiscountPercent           uint8
	TaxZone                   string
	TaxCategory               string
	TaxType                   string
	ServiceCategory           string
	CustomerID                uint8
	Incoterm                  string
}

// Quotation is the Wails-bound application struct.
// It owns the Wails context and the full data bag for rendering.
type Quotation struct {
	ctx                context.Context
	Data               QuotationData
	lastInput          QuotationInput
	prepared           bool
	store              *store.Store
	auth               *auth.Service
	archiveQuoteNumber string
	archiveRevision    uint16
	// customerDirectory caches the server-owned client list for desktop
	// Prepare resolution. The server path always resolves via the store;
	// both read the same truth.
	customerDirectory []ManagedClient
	// companyIdentity caches the stored company profile for desktop
	// rendering. Same pattern as the client directory.
	companyIdentity *CompanyIdentity
}

// CompanyIdentity is the quotation-relevant slice of the company profile:
// name and address are the only profile values the template prints.
type CompanyIdentity struct {
	Name    string
	Address string
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
	q.companyIdentity = &CompanyIdentity{Name: profile.LegalName, Address: profile.Address}
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
		return CompanyIdentity{Name: profile.LegalName, Address: profile.Address}, nil
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
	}
	return quotation
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

	for _, item := range input.Items {
		if strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("item description is required")
		}

		if item.Qty == 0 {
			return fmt.Errorf("item quantity must be greater than zero")
		}

		if strings.TrimSpace(item.UoM) == "" {
			return fmt.Errorf("item unit of measure is required")
		}

		if item.MarginPercent > 100 {
			return fmt.Errorf("item margin percent cannot be greater than 100")
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

	return nil
}

// saveFileDialog opens the native save dialog. It is a variable so tests can
// substitute a stub; production uses the Wails runtime dialog.
var saveFileDialog = runtime.SaveFileDialog

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
// preventing wasted sequence increments on cancellation.
func (q *Quotation) Export() error {
	if err := q.requireUser(q.ctx); err != nil {
		return err
	}
	client, err := api()
	if err != nil {
		return err
	}

	filePath, err := saveFileDialog(q.ctx, runtime.SaveDialogOptions{
		Title:           "Save Quotation",
		DefaultFilename: q.exportFilename(),
		Filters:         []runtime.FileFilter{{DisplayName: "PDF Files (*.pdf)", Pattern: "*.pdf"}},
	})
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

func (q *Quotation) build() ([]byte, error) {
	var body bytes.Buffer
	if err := q.Data.HtmlTemplate.Execute(&body, q.Data); err != nil {
		return nil, fmt.Errorf("execute quotation template: %w", err)
	}

	if body.Len() == 0 {
		return nil, fmt.Errorf("template executed but produced empty output")
	}

	baseCtx := q.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	ctx, cancel := chromedp.NewContext(baseCtx)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	var buf []byte
	err := chromedp.Run(
		ctx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}

			return page.SetDocumentContent(tree.Frame.ID, body.String()).Do(ctx)
		}),
		chromedp.WaitVisible(`table`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			printData, _, err := page.PrintToPDF().WithPrintBackground(true).WithDisplayHeaderFooter(false).WithPaperWidth(8.27).WithPaperHeight(11.69).Do(ctx)
			buf = printData
			return err
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("render quotation pdf: %w", err)
	}

	if len(buf) == 0 {
		return nil, fmt.Errorf("render quotation pdf: empty output")
	}

	return buf, nil
}
