package backend

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/wailsapp/wails/v2/pkg/runtime"

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
	Items                     []QuotationItem
	CustomCSS                 template.CSS
	HtmlTemplate              *template.Template
	LogoBase64                string
	SignatureBase64           string
	FontRegularBase64         string
	FontBoldBase64            string
	Date                      string
	ValidUntil                string
	RecipientName             string
	RecipientCompany          string
	RecipientAddress          string
	QuoteNumber               string
	RevisionID                uint16
	Subject                   string
	DisplaySubTotal           string
	DisplayDiscountAmount     string
	DisplayGrandTotal         string
	SubTotal                  uint64
	DiscountAmount            uint64
	GrandTotal                uint64
	OriginalSubTotal          uint64
	OriginalDiscountAmount    uint64
	OriginalGrandTotal        uint64
	MarginalizedGrandAfterTax uint64
	ProfitAmount              int64
	TaxRateBps                uint16
	TaxAmount                 uint64
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
	archiveQuoteNumber string
	archiveRevision    uint16
}

func (q *Quotation) SetStore(database *store.Store) {
	q.store = database
}

// SetRevisionContext tells Export whether this is a new quotation or the
// next immutable revision of an archived quotation.
func (q *Quotation) SetRevisionContext(quoteNumber string, revision uint16) error {
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

var customerMap = map[uint8]string{
	2: "PT. Infineon Technologies Batam",
	3: "PT. H-Tech Oilfield Equipment, NOV XL Systems",
	4: "PT. Datalec Global Solution",
	5: "PT. Panasonic Industrial Devices Batam",
	6: "PT. Surya Teknologi Batam",
}

var addressMap = map[uint8]string{
	2: "Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia",
	3: "Kecamatan Nongsa, Kota Batam, Kepulauan Riau, Indonesia",
	4: "Ruko Pollux Habibie No.E16, Kota Batam, Kepulauan Riau, Indonesia",
	5: "Puri Industrial Park, Baloi Permai, Batam Kota, Kepulauan Riau, Indonesia",
	6: "Batam Industrial Park, Jalan Beringin Lot 312-313, Muka Kuning Batam, Kepulauan Riau, Indonesia",
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

func checkCustomerID(id uint8) bool {
	_, exists := customerMap[id]
	return exists
}

func formatQuoteNumber(cusID uint8, seqID uint16) string {
	return fmt.Sprintf("15%02d%04d", cusID, seqID)
}

// Prepare validates user input and populates the QuotationData business fields.
// It does not generate the quote number — that is deferred to Export to avoid
// consuming a sequence number if the user never saves. Returns an error
// describing the first validation failure, or nil on success.
func (q *Quotation) Prepare(input QuotationInput) error {
	customerExists := checkCustomerID(input.CustomerID)
	if !customerExists {
		return fmt.Errorf("invalid customer ID: %d", input.CustomerID)
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
	taxBps, taxLabel, err := taxRateFor(input.TaxZone, input.TaxCategory)
	if err != nil {
		return err
	}

	validIncoterms := map[string]bool{"EXW": true, "FCA": true, "FAS": true, "FOB": true, "CFR": true, "CIF": true, "CPT": true, "CIP": true, "DPU": true, "DAP": true, "DDP": true}
	if !validIncoterms[strings.ToUpper(input.Incoterm)] {
		return fmt.Errorf("invalid incoterm: %s", input.Incoterm)
	}

	q.Data.CustomerID = input.CustomerID

	q.Data.RecipientName = input.RecipientName
	q.Data.RecipientCompany = customerMap[input.CustomerID]
	q.Data.RecipientAddress = addressMap[input.CustomerID]

	q.Data.Subject = input.Subject
	q.Data.Validity = input.Validity
	q.Data.PaymentTerm = input.PaymentTerm
	q.Data.LeadTime = input.LeadTime
	q.Data.DiscountPercent = input.DiscountPercent
	q.Data.TaxZone = input.TaxZone
	q.Data.TaxCategory = input.TaxCategory
	q.Data.Incoterm = input.Incoterm

	items := buildItems(input.Items)
	q.Data.Items = items

	date, validUntil := buildDates(input.Validity)
	q.Data.Date = date
	q.Data.ValidUntil = validUntil

	pricing := buildPricing(items, input.DiscountPercent, taxBps)
	q.Data.SubTotal = pricing.MarginalizedSubtotal
	q.Data.DiscountAmount = pricing.MarginalizedDiscount
	q.Data.GrandTotal = pricing.MarginalizedGrand
	q.Data.OriginalSubTotal = pricing.OriginalSubtotal
	q.Data.OriginalDiscountAmount = pricing.OriginalDiscount
	q.Data.OriginalGrandTotal = pricing.OriginalGrand
	q.Data.MarginalizedGrandAfterTax = pricing.MarginalizedGrandAfterTax
	q.Data.ProfitAmount = pricing.ProfitAmount

	q.Data.TaxRateBps = taxBps
	q.Data.TaxAmount = pricing.TaxAmount
	q.Data.NettProfit = pricing.NettProfit
	q.Data.TaxLabel = taxLabel

	displaySubTotal, displayDiscountAmount, displayGrandTotal := buildViews(
		pricing.MarginalizedSubtotal,
		pricing.MarginalizedDiscount,
		pricing.MarginalizedGrand,
	)
	q.Data.DisplaySubTotal = displaySubTotal
	q.Data.DisplayDiscountAmount = displayDiscountAmount
	q.Data.DisplayGrandTotal = displayGrandTotal
	q.Data.DisplayTaxAmount = formatRupiah(pricing.TaxAmount)
	q.Data.DisplayNettProfit = formatSignedRupiah(q.Data.NettProfit)

	q.lastInput = input
	q.prepared = true

	return nil
}

// Export opens a native save file dialog, generates the quote number,
// builds the PDF, and writes it to the chosen path. The quote number
// is only consumed after the user confirms the save dialog, preventing
// wasted sequence increments on cancellation.
func (q *Quotation) Export() error {
	if q.store == nil {
		return fmt.Errorf("Postgres store is not configured")
	}

	filePath, err := runtime.SaveFileDialog(q.ctx, runtime.SaveDialogOptions{
		Title:           "Save Quotation",
		DefaultFilename: "quotation.pdf",
		Filters:         []runtime.FileFilter{{DisplayName: "PDF Files (*.pdf)", Pattern: "*.pdf"}},
	})
	if err != nil {
		return fmt.Errorf("open save dialog: %w", err)
	}

	if filePath == "" {
		return fmt.Errorf("user cancelled")
	}

	if q.archiveQuoteNumber != "" {
		q.Data.QuoteNumber = q.archiveQuoteNumber
		q.Data.RevisionID = q.archiveRevision
	} else {
		sequenceContext := q.ctx
		if sequenceContext == nil {
			sequenceContext = context.Background()
		}
		sequenceID, err := q.store.NextQuoteSequence(sequenceContext)
		if err != nil {
			return fmt.Errorf("generate quote number from Postgres: %w", err)
		}
		q.Data.QuoteNumber = formatQuoteNumber(q.Data.CustomerID, sequenceID)
		q.Data.RevisionID = 0
	}

	validatedPath, err := validateSavePath(filePath)
	if err != nil {
		return fmt.Errorf("validate quotation path: %w", err)
	}
	pdfBuf, err := q.build()
	if err != nil {
		return fmt.Errorf("build quotation: %w", err)
	}
	if err := os.WriteFile(validatedPath, pdfBuf, 0o644); err != nil {
		return fmt.Errorf("write quotation: %w", err)
	}
	htmlSnapshot, err := q.GetQuotationPreviewHTML()
	if err != nil {
		return fmt.Errorf("render quotation archive snapshot: %w", err)
	}
	if _, err := q.store.SaveRevision(q.context(), q.revisionDraft(htmlSnapshot, pdfBuf, validatedPath)); err != nil {
		return fmt.Errorf("archive quotation revision: %w", err)
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
		TaxRateBps:       int16(q.Data.TaxRateBps),
		Items:            items,
		Totals: store.RevisionTotals{
			OriginalSubtotal:          int64(q.Data.OriginalSubTotal),
			OriginalDiscount:          int64(q.Data.OriginalDiscountAmount),
			OriginalGrand:             int64(q.Data.OriginalGrandTotal),
			MarginalizedSubtotal:      int64(q.Data.SubTotal),
			MarginalizedDiscount:      int64(q.Data.DiscountAmount),
			MarginalizedGrand:         int64(q.Data.GrandTotal),
			TaxAmount:                 int64(q.Data.TaxAmount),
			MarginalizedGrandAfterTax: int64(q.Data.MarginalizedGrandAfterTax),
			ProfitAmount:              q.Data.ProfitAmount,
			NettProfit:                q.Data.NettProfit,
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
