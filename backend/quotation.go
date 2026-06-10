package backend

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

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
	HtmlTemplate          string
	LogoBase64            string
	SignatureBase64       string
	FontRegularBase64     string
	FontBoldBase64        string
	Date                  string
	ValidUntil            string
	SenderName            string
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
	PaymentTerms          uint8
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
		return fmt.Errorf("read template: %w", err)
	}

	InsertDate(quotation)
	InsertSenderRecipient(quotation)
	InsertQuotationInfo(quotation)
	InsertItemPrices(quotation)

	return nil
}

func (q *Quotation) build() ([]byte, error) {
	for i := range q.Data.Items {
		q.Data.Items[i].No = uint32(i + 1)
	}

	tmpl, err := template.New("quotation").Parse(q.Data.HtmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse quotation template: %w", err)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, q.Data); err != nil {
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
	err = chromedp.Run(
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

func validateSavePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	if !strings.EqualFold(filepath.Ext(absPath), ".pdf") {
		return "", fmt.Errorf("file must use .pdf extension")
	}

	parentDir := filepath.Dir(absPath)
	parentInfo, err := os.Stat(parentDir)
	if err != nil {
		return "", fmt.Errorf("invalid destination directory: %w", err)
	}
	if !parentInfo.IsDir() {
		return "", fmt.Errorf("destination parent is not a directory")
	}

	fileInfo, err := os.Lstat(absPath)
	if err == nil {
		if fileInfo.IsDir() {
			return "", fmt.Errorf("destination is a directory")
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("destination cannot be a symlink")
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check destination: %w", err)
	}

	return absPath, nil
}

var rupiahPrinter = message.NewPrinter(language.Indonesian)

func formatRupiah(amount uint64) string {
	return rupiahPrinter.Sprintf("%d", amount)
}

func formatDescription(raw string) template.HTML {
	clean := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(clean, "\n")

	mainParts := make([]string, 0, len(lines))
	bullets := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if item != "" {
				bullets = append(bullets, item)
			}
			continue
		}

		mainParts = append(mainParts, trimmed)
	}

	var b strings.Builder

	if len(mainParts) > 0 {
		b.WriteString(`<div class="desc-main">`)
		b.WriteString(html.EscapeString(strings.Join(mainParts, " ")))
		b.WriteString(`</div>`)
	}

	if len(bullets) > 0 {
		b.WriteString(`<ul class="desc-subitems">`)
		for _, item := range bullets {
			b.WriteString(`<li>`)
			b.WriteString(html.EscapeString(item))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}

	if b.Len() == 0 {
		return template.HTML(html.EscapeString(raw))
	}

	return template.HTML(b.String())
}
