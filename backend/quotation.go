package backend

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
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

//go:embed assets/*
var backAssets embed.FS

//go:embed template/html/*
var htmlTemplate embed.FS

//go:embed template/css/*
var cssTemplate embed.FS

type QuotationItem struct {
	No               uint32
	Description      string
	DisplayDesc      template.HTML
	UoM              string
	DisplayUnitPrice string
	DisplayTotal     string
	UnitPrice        uint64
	Total            uint64
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
	QuoteNumber           string
	SenderName            string
	RecipientName         string
	RecipientCompany      string
	RecipientAddress      string
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
	Data QuotationData
	ctx  context.Context
}

func Startup(quotation *Quotation, ctx context.Context) error {
	quotation.ctx = ctx

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

	protoItemList := []QuotationItem{
		{
			Description: "Supply & Install lighting sensor system (PIR Schneider) at AHU block3 ,2A 2, Mezzanine Front & Rear with following scopes:\n- 6 sensors at Front AHU and 6 sensors at Rear AHU\n- 6-units of batten fixtures set with guard (fixtures 2x 1200mm LED tubes/lamps) for additional lightings\n- Rewiring with Auto-Manual selector and lighting switch.",
			Qty:         1,
			UoM:         "Lot",
			UnitPrice:   115725000,
			Total:       115725000,
		},
		{
			Description: "Supply & Install lighting sensor system (PIR Schneider) 14 sensor at CDA blok 4.3",
			Qty:         1,
			UoM:         "Lot",
			UnitPrice:   59970000,
			Total:       59970000,
		},
		{
			Description: "Supply & Install lighting sensor system (PIR Schneider) 5 sensor office blok9",
			Qty:         1,
			UoM:         "Lot",
			UnitPrice:   22170000,
			Total:       22170000,
		},
		{
			Description: "Supply and install aircond automation system at mesjid of Batam1, consisted of:\n- Metal enclosure panel Timers\n- Zelio Schneider.",
			Qty:         1,
			UoM:         "Lot",
			UnitPrice:   34050000,
			Total:       34050000,
		},
		{
			Description: "Supply & Install lighting sensor system (PIR Schneider) at AHU block3 ,2A 2, Mezzanine Front & Rear with following scopes:\n- 6 sensors at Front AHU and 6 sensors at Rear AHU\n- 6-units of batten fixtures set with guard (fixtures 2x 1200mm LED tubes/lamps) for additional lightings\n- Rewiring with Auto-Manual selector and lighting switch.",
			Qty:         1,
			UoM:         "Lot",
			UnitPrice:   115725000,
			Total:       115725000,
		},
		{
			Description: "Supply & Install lighting sensor system (PIR Schneider) 14 sensor at CDA blok 4.3",
			Qty:         1,
			UoM:         "Lot",
			UnitPrice:   59970000,
			Total:       59970000,
		},
		{
			Description: "Supply & Install lighting sensor system (PIR Schneider) 5 sensor office blok9",
			Qty:         1,
			UoM:         "Lot",
			UnitPrice:   22170000,
			Total:       22170000,
		},
		{
			Description: "Supply and install aircond automation system at mesjid of Batam1, consisted of:\n- Metal enclosure panel Timers\n- Zelio Schneider.",
			Qty:         1,
			UoM:         "Lot",
			UnitPrice:   34050000,
			Total:       34050000,
		},
	}

	for i := range protoItemList {
		protoItemList[i].DisplayDesc = formatDescription(protoItemList[i].Description)
		protoItemList[i].DisplayUnitPrice = formatRupiah(protoItemList[i].UnitPrice)
		protoItemList[i].DisplayTotal = formatRupiah(protoItemList[i].Total)
	}

	quotation.Data.CustomCSS = template.CSS(string(cssContent))
	quotation.Data.HtmlTemplate = string(htmlContent)
	quotation.Data.LogoBase64 = base64.StdEncoding.EncodeToString(logoBytes)
	quotation.Data.SignatureBase64 = base64.StdEncoding.EncodeToString(sigBytes)
	quotation.Data.FontRegularBase64 = base64.StdEncoding.EncodeToString(fontRegularBytes)
	quotation.Data.FontBoldBase64 = base64.StdEncoding.EncodeToString(fontBoldBytes)

	// configured variables, filled temporarily for prototyping
	quotation.Data.QuoteNumber = "APP-2026-01-001-R0"
	quotation.Data.SenderName = "Quotation Team"
	quotation.Data.RecipientName = "Recipient Name"
	quotation.Data.RecipientCompany = "Recipient Company"
	quotation.Data.RecipientAddress = "Recipient Address"
	quotation.Data.Subject = "Supply & Install lighting sensor system (PIR Schneider) at AHU Bat1"
	quotation.Data.Validity = 30
	quotation.Data.PaymentTerms = 90
	quotation.Data.LeadTime = 60
	quotation.Data.DiscountPercent = 5

	// dates
	now := time.Now()

	quotation.Data.Date = now.Format("January 02, 2006")
	quotation.Data.ValidUntil = now.AddDate(0, 0, int(quotation.Data.Validity)).Format("January 02, 2006")

	// must filled/calculated variables
	quotation.Data.Items = protoItemList

	for _, item := range protoItemList {
		quotation.Data.SubTotal += item.Total
	}

	quotation.Data.DiscountAmount = (quotation.Data.SubTotal * uint64(quotation.Data.DiscountPercent)) / 100
	quotation.Data.GrandTotal = quotation.Data.SubTotal - quotation.Data.DiscountAmount

	quotation.Data.DisplaySubTotal = formatRupiah(quotation.Data.SubTotal)
	quotation.Data.DisplayDiscountAmount = formatRupiah(quotation.Data.DiscountAmount)
	quotation.Data.DisplayGrandTotal = formatRupiah(quotation.Data.GrandTotal)

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

	err = os.WriteFile(validatedPath, pdfBuf, 0644)
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

func formatRupiah(amount uint64) string {
	p := message.NewPrinter(language.Indonesian)
	return p.Sprintf("%d", amount)
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
