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
	"vera-app/backend/currency"
	"vera-app/backend/render"
	"vera-app/backend/render/quotation"
	"vera-app/backend/store"
	"vera-app/backend/theme"
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

// QuotationSubItemInput is one priced part of a modular line item. Sub-items
// carry their own cost, quantity and margin; the parent line's price is the
// sum of theirs, so a parent with sub-items never prices itself.
//
// SubSubItems are its scope lines -- description-only detail stating what the
// part includes. They have no standalone values and no life of their own: the
// linkage is structural, because the only way to send one is inside a
// sub-item.
type QuotationSubItemInput struct {
	Description     string
	UoM             string
	UnitCost        uint64
	Qty             uint32
	MarginPercent   uint16
	DiscountPercent uint16
	DiscountEnabled bool
	MarginEnabled   bool
	SubSubItems     []QuotationSubSubItemInput
}

// QuotationSubSubItemInput is one detail line beneath a priced sub-item. It
// has no standalone values -- no quantity, no cost, no margin -- it is linked
// to the sub-item it sits under and numbers as X.Y.Z.
type QuotationSubSubItemInput struct {
	Description string
}

// QuotationItemInput is a single line item from the frontend.
// No is assigned by the backend. UnitPrice is in raw IDR.
//
// SubItems turns the line into a modular package: each sub-item is priced on
// its own and the parent becomes the rollup (Qty 1, UoM Lot, price = Σ subs).
// ShowSubPrices chooses the customer view -- true prints each sub as a priced
// row beneath a title-only parent (state C), false keeps the parent as the
// full-value row and prints the subs as unpriced scope rows (state B). Either
// way the pricing is identical; only what the customer sees differs.
type QuotationItemInput struct {
	// Pointer-bearing fields first: strings and the sub-item slice lead, so
	// the collector's scan window is short and the struct stays at its 72-byte
	// size floor (see struct_alignment_test.go).
	Description     string
	UoM             string
	SubItems        []QuotationSubItemInput
	UnitPrice       uint64
	Qty             uint32
	MarginPercent   uint16
	DiscountPercent uint16
	DiscountEnabled bool
	MarginEnabled   bool
	ShowSubPrices   bool
}

// QuotationSubItem is one priced part of a prepared package line, with the
// display strings the customer template renders. The margin fields describe
// how the part was priced; nothing renders them into customer output.
//
// SubSubItems are the part's scope lines, numbered X.Y.Z; they carry no
// pricing and contribute nothing to any total.
type QuotationSubItem struct {
	Description         string
	SubNo               string
	UoM                 string
	SubSubItems         []QuotationSubSubItem
	DisplayUnitPrice    string
	DisplayTotal        string
	DisplayDiscount     string
	DisplayListPrice    string
	DisplayMarginAmount string
	OriginalUnitPrice   uint64
	OriginalTotal       uint64
	UnitPrice           uint64
	Total               uint64
	ListUnitPrice       uint64
	DiscountAmount      uint64
	MarginAmount        int64
	DiscountPercent     uint16
	MarginPercent       uint16
	DiscountEnabled     bool
	MarginEnabled       bool
	Qty                 uint32
}

// QuotationSubSubItem is one detail line beneath a priced sub-item. It has no
// values of its own and no printed label: it is a plain bullet under its sub,
// identified by position alone (storage keeps the ordering).
type QuotationSubSubItem struct {
	Description string
}

// QuotationItem is a prepared line item with computed display fields
// ready for template rendering.
//
// SubItems is populated only for modular lines; the template renders them as
// their own rows beneath the parent, priced (state C) or as plain scope rows
// (state B) depending on ShowSubPrices.
//
// Field order is chosen for memory layout: every string and the slice lead,
// word-sized numerics follow, and the flags and counts close. See
// struct_alignment_test.go.
type QuotationItem struct {
	Description              string
	DisplayDesc              template.HTML
	UoM                      string
	DisplayOriginalUnitPrice string
	DisplayOriginalTotal     string
	DisplayUnitPrice         string
	DisplayTotal             string
	DisplayMarginAmount      string
	SubItems                 []QuotationSubItem
	OriginalUnitPrice        uint64
	OriginalTotal            uint64
	MarginAmount             int64
	UnitPrice                uint64
	Total                    uint64
	ListUnitPrice            uint64
	ListTotal                uint64
	DiscountAmount           uint64
	DisplayDiscount          string
	DisplayListUnitPrice     string
	MarginPercent            uint16
	DiscountPercent          uint16
	MarginEnabled            bool
	DiscountEnabled          bool
	ShowSubPrices            bool
	No                       uint32
	Qty                      uint32
}

// QuotationData is the full template data bag.
// It combines static template assets (set once at startup by readTemplate)
// with business data (set per-export by Prepare).
type QuotationData struct {
	HtmlTemplate *template.Template
	// The item table always prints both price columns -- the list Unit Price
	// and the After Disc. net price -- on every quotation, so there is no
	// flag to pick a column set by and no span to precompute for the merged
	// package rows (Description through Total is always six columns).
	DisplayDiscountAmount string
	TaxZone               string
	LogoBase64            string
	SignatureBase64       string
	FontRegularBase64     string
	FontBoldBase64        string
	Date                  string
	ValidUntil            string
	CompanyName           string
	Incoterm              string
	// Currency is the normalized company code (unknown reads as IDR). It
	// decides the exponent and symbol of every Display string Prepare builds,
	// and the template reads CurrencyPrefix/CurrencySuffix around the totals
	// plus Currency itself in the item-table headers.
	Currency       string
	CurrencyPrefix string
	CurrencySuffix string
	ThemeQDeep     string
	ThemeQAccent   string
	ThemeQSoft     string
	ThemeQInk      string
	// Palette is the theme resolved to every colour the document paints
	// with: the four bases above plus each derived role. It is refreshed
	// wherever the bases are set (readTemplate at startup, refreshCustomCSS
	// after Prepare) so the stylesheet always executes against the current
	// brand. See backend/theme for the derivation.
	Palette                   theme.Palette
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
	// activeDraftID is the draft the editor currently holds, 0 when the
	// quotation is not a resumed draft. It is sent with the export so the
	// server can consume the draft as part of the same request: a draft that
	// has become a quotation has nothing left to be, and doing it there means
	// a crash cannot leave a stale copy behind.
	activeDraftID int64
}

// CompanyIdentity is the quotation-relevant slice of the company profile:
// name and address are the only profile values the template prints.
type CompanyIdentity struct {
	Name    string
	Address string
	// Currency is the company's ISO 4217 code, normalized to a catalogue
	// entry by Prepare (unknown or empty reads as IDR). It decides the
	// exponent and symbol every Display string on the document uses.
	Currency string
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
		Currency:     profile.Currency,
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
			Currency:     profile.Currency,
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

// SetDraftContext records the draft the editor is working from, so a
// successful export consumes it on the server. Zero clears it.
func (q *Quotation) SetDraftContext(draftID int64) error {
	if err := q.requireUser(q.ctx); err != nil {
		return err
	}
	q.activeDraftID = draftID
	return nil
}

// ClearDraftContext forgets the active draft without touching the server,
// for when the user navigates away or starts a new quotation.
func (q *Quotation) ClearDraftContext() {
	q.activeDraftID = 0
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
// data so theme values set by Prepare reach the CSS. The palette resolves
// first, so the executed stylesheet always carries the current brand's full
// role set, not just its four bases. A nil parsed template (tests that never
// ran Startup) leaves the CSS untouched.
func (q *Quotation) refreshCustomCSS() error {
	if q.cssTemplate == nil {
		return nil
	}
	q.resolvePalette()
	var cssBuf strings.Builder
	if err := q.cssTemplate.Execute(&cssBuf, q.Data); err != nil {
		return fmt.Errorf("execute css template: %w", err)
	}
	q.Data.CustomCSS = template.CSS(cssBuf.String())
	return nil
}

// resolvePalette derives the document's full colour set from the four stored
// brand values. Empty or malformed values fall back per field (see theme).
func (q *Quotation) resolvePalette() {
	q.Data.Palette = theme.Theme{
		Deep:   q.Data.ThemeQDeep,
		Accent: q.Data.ThemeQAccent,
		Soft:   q.Data.ThemeQSoft,
		Ink:    q.Data.ThemeQInk,
	}.Resolve()
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

// MaxDiscountPercent is the ceiling on a per-row discount and on the
// whole-quotation discount. 100% is a free line, which is a real thing to
// quote, and nothing above it is a price rather than a discount.
const MaxDiscountPercent = 100

// validateSubItems checks one item's priced sub-items and returns them
// normalized, ready for the builder.
//
// A line with sub-items is a package: the parent stops being a priceable row
// and becomes the rollup of its parts. That is why the parent's quantity and
// unit are pinned to 1 and Lot (the parent's price IS the sum, so a second lot
// would double-count), why the parent's own margin is refused (the parts carry
// the margin now), and why description bullets are refused (the sub rows
// replace them; keeping both would print the scope twice). ShowSubPrices with
// no sub-items is a contradiction and is refused rather than ignored, because a
// flag the code silently drops today is a bug it ships tomorrow.
func validateSubItems(item QuotationItemInput, itemNo int) ([]QuotationSubItemInput, error) {
	if len(item.SubItems) == 0 {
		if item.ShowSubPrices {
			return nil, fmt.Errorf("item %d: sub-item breakdown requested but no sub-items given", itemNo)
		}
		return nil, nil
	}

	// 1 exactly, so the parent contributes its price once; Lot, because a
	// package is quoted as a lot. Case-insensitive so "lot" from a paste
	// still climbs the same path if the editor ever sends it that way.
	if item.Qty != 1 {
		return nil, fmt.Errorf("item %d: a line with sub-items must have quantity 1 (the package is the rollup of its parts)", itemNo)
	}
	if !strings.EqualFold(strings.TrimSpace(item.UoM), "lot") {
		return nil, fmt.Errorf("item %d: a line with sub-items must use the unit Lot", itemNo)
	}
	if item.MarginEnabled {
		return nil, fmt.Errorf("item %d: a line with sub-items takes its margin per sub-item; clear the line margin", itemNo)
	}
	for _, line := range strings.Split(item.Description, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			return nil, fmt.Errorf("item %d: bullet lines in the description duplicate the sub-items beneath; move the scope into sub-items", itemNo)
		}
	}

	normalized := make([]QuotationSubItemInput, 0, len(item.SubItems))
	for j, sub := range item.SubItems {
		description := strings.TrimSpace(sub.Description)
		if description == "" {
			return nil, fmt.Errorf("item %d sub-item %d: description is required", itemNo, j+1)
		}
		if strings.ContainsAny(description, "\n\r") {
			return nil, fmt.Errorf("item %d sub-item %d: description must be a single line", itemNo, j+1)
		}
		if sub.Qty == 0 {
			return nil, fmt.Errorf("item %d sub-item %d: quantity must be greater than zero", itemNo, j+1)
		}
		uom := strings.TrimSpace(sub.UoM)
		if uom == "" {
			return nil, fmt.Errorf("item %d sub-item %d: unit of measure is required", itemNo, j+1)
		}
		if !plausibleUoM(uom) {
			return nil, fmt.Errorf("item %d sub-item %d: unit of measure %q is not a unit", itemNo, j+1, sub.UoM)
		}
		if sub.MarginPercent > MaxMarginPercent {
			return nil, fmt.Errorf("item %d sub-item %d: margin percent cannot be greater than %d", itemNo, j+1, MaxMarginPercent)
		}
		if sub.DiscountPercent > MaxDiscountPercent {
			return nil, fmt.Errorf("item %d sub-item %d: discount percent cannot be greater than %d", itemNo, j+1, MaxDiscountPercent)
		}
		sub.Description = description
		sub.UoM = uom

		// Sub-sub-items spell out what one priced part includes. They carry
		// no values of their own -- no quantity, no cost, no margin -- and
		// exist only through their parent sub-item, which is also why the
		// structure itself (a field on the sub) enforces the linkage: there
		// is no way to send one that is not attached to a sub.
		normalizedSubSubs := make([]QuotationSubSubItemInput, 0, len(sub.SubSubItems))
		for k, low := range sub.SubSubItems {
			lowDescription := strings.TrimSpace(low.Description)
			if lowDescription == "" {
				return nil, fmt.Errorf("item %d sub-item %d detail %d: description is required", itemNo, j+1, k+1)
			}
			if strings.ContainsAny(lowDescription, "\n\r") {
				return nil, fmt.Errorf("item %d sub-item %d detail %d: description must be a single line", itemNo, j+1, k+1)
			}
			normalizedSubSubs = append(normalizedSubSubs, QuotationSubSubItemInput{
				Description: lowDescription,
			})
		}
		sub.SubSubItems = normalizedSubSubs
		normalized = append(normalized, sub)
	}
	return normalized, nil
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
		if item.DiscountPercent > MaxDiscountPercent {
			return fmt.Errorf("item discount percent cannot be greater than %d", MaxDiscountPercent)
		}

		normalized, err := validateSubItems(item, i+1)
		if err != nil {
			return err
		}
		input.Items[i].SubItems = normalized
		if len(normalized) > 0 {
			// A package is quoted as a Lot by definition; canonicalize instead
			// of letting "lot", "LOT" or "Lot " reach the customer document.
			input.Items[i].UoM = "Lot"
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
	// Unknown or empty codes render as the default rather than breaking a
	// document: this is the second fallback, after the store's write-time
	// validation.
	q.Data.Currency = currency.Normalize(identity.Currency)
	q.Data.CurrencyPrefix, q.Data.CurrencySuffix = currency.Affixes(q.Data.Currency)

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

	items := buildItems(input.Items, q.Data.Currency)
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
		q.Data.Currency,
	)
	q.Data.DisplaySubTotal = displaySubTotal
	q.Data.DisplayDiscountAmount = displayDiscountAmount
	q.Data.DisplayGrandTotal = displayGrandTotal
	q.Data.DisplayPpnAmount = formatMoney(q.Data.Currency, pricing.PpnAmount)
	q.Data.DisplayTaxAmount = formatMoney(q.Data.Currency, pricing.TaxAmount)
	q.Data.DisplayNettProfit = formatSignedMoney(q.Data.Currency, q.Data.NettProfit)

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
// Export opens the desktop's save dialog and writes the quotation where the
// user chose. The dialog is the only part of exporting that needs a window.
func (q *Quotation) Export(defaultDir string) error {
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
	return q.ExportTo(filePath)
}

// ExportTo runs one export against a path the caller already chose: ask the
// server for a number and store the signed revision, render the document here,
// write it, and attach it to the revision it belongs to.
//
// Split out of Export so the CLI drives the identical sequence with --out in
// place of a dialog. One implementation is what stops the document a script
// produces from drifting from the one the desktop produces.
func (q *Quotation) ExportTo(filePath string) error {
	if err := q.requireUser(q.ctx); err != nil {
		return err
	}
	client, err := api()
	if err != nil {
		return err
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
	// A resumed draft is consumed by the export itself. The server deletes it
	// only after the revision is stored, so a failed export always leaves the
	// draft intact.
	if q.activeDraftID > 0 {
		payload["draftId"] = q.activeDraftID
	}
	var exported apicontract.ExportResponse
	if err := client.Post("/v1/quotations/export", payload, &exported); err != nil {
		return err
	}
	// The draft is gone server-side now; forget it locally so a later export
	// in the same session cannot try to consume it a second time.
	q.activeDraftID = 0
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

// RenderPDF renders the prepared quotation as a PDF without storing anything.
//
// Export does this in the middle of its work; the CLI needs it on its own for
// `vera quotations render`, and for tests that want the document without a
// revision attached to it. Rendering is pure Go, so this needs no browser and no
// display.
func (q *Quotation) RenderPDF() ([]byte, error) {
	return q.build()
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
		subItems := make([]store.RevisionSubItem, 0, len(item.SubItems))
		for i, sub := range item.SubItems {
			lows := make([]store.RevisionSubSubItem, 0, len(sub.SubSubItems))
			for k, low := range sub.SubSubItems {
				lows = append(lows, store.RevisionSubSubItem{
					SubSubNo:    k + 1,
					Description: low.Description,
				})
			}
			subItems = append(subItems, store.RevisionSubItem{
				SubNo:           i + 1,
				Description:     sub.Description,
				Qty:             int64(sub.Qty),
				UoM:             sub.UoM,
				UnitCost:        int64(sub.OriginalUnitPrice),
				UnitPrice:       int64(sub.UnitPrice),
				MarginAmount:    sub.MarginAmount,
				Total:           int64(sub.Total),
				ListUnitPrice:   int64(sub.ListUnitPrice),
				MarginPercent:   int16(sub.MarginPercent),
				DiscountPercent: int16(sub.DiscountPercent),
				MarginEnabled:   sub.MarginEnabled,
				DiscountEnabled: sub.DiscountEnabled,
				SubSubItems:     lows,
			})
		}
		items = append(items, store.RevisionItem{
			ItemNo:                int(item.No),
			Description:           item.Description,
			Qty:                   int64(item.Qty),
			UoM:                   item.UoM,
			UnitPrice:             int64(item.OriginalUnitPrice),
			MarginEnabled:         item.MarginEnabled,
			MarginPercent:         int16(item.MarginPercent),
			MarginAmount:          item.MarginAmount,
			OriginalTotal:         int64(item.OriginalTotal),
			MarginalizedUnitPrice: int64(item.UnitPrice),
			MarginalizedTotal:     int64(item.Total),
			ShowSubPrices:         item.ShowSubPrices,
			ListUnitPrice:         int64(item.ListUnitPrice),
			DiscountPercent:       int16(item.DiscountPercent),
			DiscountEnabled:       item.DiscountEnabled,
			SubItems:              subItems,
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
		Currency:         q.Data.Currency,
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
		parts := make([]quotation.SubItem, 0, len(it.SubItems))
		for _, sub := range it.SubItems {
			bullets := make([]string, 0, len(sub.SubSubItems))
			for _, low := range sub.SubSubItems {
				bullets = append(bullets, low.Description)
			}
			// The item table prints two prices: the list unit price
			// (marginalised, before the row's own discount) under Unit Price
			// IDR, and the net unit price under After Disc. IDR. The After Disc.
			// cell carries a second line -- the row's discount percentage, or
			// "0%" when it carries none -- packed after a newline, because the
			// seam type below is frozen (shared with renderers this change may
			// not touch) and cannot gain a field for it. The template renders
			// its two lines from DisplayUnitPrice plus DiscountEnabled/Percent
			// directly; this packs the same two strings so the native renderer
			// splits out an identical pair. See discountPctLine for the one
			// formatting both sides must agree on.
			parts = append(parts, quotation.SubItem{
				SubNo:       sub.SubNo,
				Description: sub.Description,
				Qty:         sub.Qty,
				UoM:         sub.UoM,
				UnitPrice:   sub.DisplayListPrice,
				Total:       sub.DisplayTotal,
				Disc:        afterDiscCell(sub.DisplayUnitPrice, sub.DiscountEnabled, sub.DiscountPercent),
				Bullets:     bullets,
			})
		}
		items = append(items, quotation.Item{
			No:            int(it.No),
			DisplayDesc:   string(it.DisplayDesc),
			Qty:           it.Qty,
			UoM:           it.UoM,
			UnitPrice:     it.DisplayListUnitPrice,
			Total:         it.DisplayTotal,
			Disc:          afterDiscCell(it.DisplayUnitPrice, it.DiscountEnabled, it.DiscountPercent),
			SubItems:      parts,
			ShowSubPrices: it.ShowSubPrices,
		})
	}

	// The template prints the PPN line only when a PPN was actually charged
	// ({{if .PpnAmount}}), so a zero-tax zone must present an empty display
	// amount to the renderers too -- passing "0" made the native engine draw
	// a "PPN 11% Rp0" row the browser never prints.
	displayPpn := ""
	if q.Data.PpnAmount > 0 {
		displayPpn = q.Data.DisplayPpnAmount
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

		// The currency travels on the seam so the native renderer prints the
		// same headers and totals the template renders from the same code.
		Currency: q.Data.Currency,

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
		DiscountAmount:    q.Data.DiscountAmount,
		DisplayPpn:        displayPpn,
		DisplayGrandTotal: q.Data.DisplayGrandTotal,
		DiscountPercent:   q.Data.DiscountPercent,

		// The after-discount column is unconditional now: every quotation
		// prints both price columns, so the seam flag is always on. It stays
		// on the seam type because the renderers still read it; removing it
		// would touch both renderer packages.
		ShowLineDiscounts: true,

		VerificationCode: q.Data.VerifyShortID,
	}, nil
}

// discountPctLine is the After Disc. cell's second line for one row: the
// row's discount percentage, or "0%" when it carries none. It reproduces the
// template's {{if .DiscountEnabled}}{{.DiscountPercent}}%{{else}}0%{{end}}
// exactly, so the HTML preview, the stored snapshot and the PDF print the
// same string for the same row.
//
// It lives here, on the application's side of the renderer seam, rather than
// in either renderer: the seam type (render/quotation) is frozen to this
// change, so the percentage cannot travel in its own field, and the template
// cannot call Go helpers without a FuncMap in template.go, which is also
// frozen. The template therefore formats the same one-liner inline, and the
// agreement between the two is pinned by test, not by shared code.
func discountPctLine(enabled bool, percent uint16) string {
	if !enabled {
		return "0%"
	}
	return strconv.Itoa(int(percent)) + "%"
}

// afterDiscCell packs the After Disc. cell's two lines -- the net unit price
// and the discount percentage line -- into the seam's Disc string, separated
// by a newline the native renderer splits on. See discountPctLine for why the
// packing lives here.
func afterDiscCell(net string, enabled bool, percent uint16) string {
	return net + "\n" + discountPctLine(enabled, percent)
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
