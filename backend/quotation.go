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
)

type QuotationInput struct {
	Items            []QuotationItemInput
	Subject          string
	RecipientName    string
	RecipientCompany string
	RecipientAddress string
	DiscountPercent  uint8
	Validity         uint8
	PaymentTerm      uint8
	LeadTime         uint8
}

type QuotationItemInput struct {
	Description string
	Qty         uint32
	UoM         string
	UnitPrice   uint64
}

type QuotationItem struct {
	Description      string
	DisplayDesc      template.HTML
	UoM              string
	DisplayUnitPrice string
	DisplayTotal     string
	UnitPrice        uint64
	Total            uint64
	No               uint32
	Qty              uint32
}

type QuotationData struct {
	Items                 []QuotationItem
	CustomCSS             template.CSS
	HtmlTemplate          *template.Template
	LogoBase64            string
	SignatureBase64       string
	FontRegularBase64     string
	FontBoldBase64        string
	Date                  string
	ValidUntil            string
	RecipientName         string
	RecipientCompany      string
	RecipientAddress      string
	QuoteNumber           string
	Subject               string
	DisplaySubTotal       string
	DisplayDiscountAmount string
	DisplayGrandTotal     string
	SubTotal              uint64
	DiscountAmount        uint64
	GrandTotal            uint64
	Validity              uint8
	PaymentTerm           uint8
	LeadTime              uint8
	DiscountPercent       uint8
}

type Quotation struct {
	ctx  context.Context
	Data QuotationData
}

func Startup(quotation *Quotation, ctx context.Context) error {
	quotation.ctx = ctx

	err := ReadTemplate(quotation)
	if err != nil {
		return err
	}

	return nil
}

func (q *Quotation) Prepare(input QuotationInput) error {
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
	}

	if input.Subject == "" {
		return fmt.Errorf("subject is required")
	}

	if strings.TrimSpace(input.RecipientName) == "" {
		return fmt.Errorf("recipient name are required")
	}

	if strings.TrimSpace(input.RecipientCompany) == "" {
		return fmt.Errorf("recipient company name are required")
	}

	if strings.TrimSpace(input.RecipientAddress) == "" {
		return fmt.Errorf("recipient address are required")
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

	q.Data.RecipientName = input.RecipientName
	q.Data.RecipientCompany = input.RecipientCompany
	q.Data.RecipientAddress = input.RecipientAddress

	// TODO: generate quote number with proper logic
	q.Data.QuoteNumber = fmt.Sprintf("Q-%d", time.Now().Unix())

	q.Data.Subject = input.Subject
	q.Data.Validity = input.Validity
	q.Data.PaymentTerm = input.PaymentTerm
	q.Data.LeadTime = input.LeadTime
	q.Data.DiscountPercent = input.DiscountPercent

	items := buildItems(input.Items)
	q.Data.Items = items

	date, validUntil := buildDates(input.Validity)
	q.Data.Date = date
	q.Data.ValidUntil = validUntil

	subTotal, discountAmount, grandTotal := buildPrices(items, input.DiscountPercent)
	q.Data.SubTotal = subTotal
	q.Data.DiscountAmount = discountAmount
	q.Data.GrandTotal = grandTotal

	displaySubTotal, displayDiscountAmount, displayGrandTotal := buildViews(subTotal, discountAmount, grandTotal)
	q.Data.DisplaySubTotal = displaySubTotal
	q.Data.DisplayDiscountAmount = displayDiscountAmount
	q.Data.DisplayGrandTotal = displayGrandTotal

	return nil
}

func (q *Quotation) Export() error {
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

	err = q.save(filePath)
	if err != nil {
		return fmt.Errorf("save quotation: %w", err)
	}

	return nil
}

func (q *Quotation) save(path string) error {
	validatedPath, err := validateSavePath(path)
	if err != nil {
		return err
	}

	pdfBuf, err := q.build()
	if err != nil {
		return err
	}

	err = os.WriteFile(validatedPath, pdfBuf, 0o644)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
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
