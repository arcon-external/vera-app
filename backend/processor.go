package backend

import (
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"time"
)

//go:embed assets/*
var backAssets embed.FS

//go:embed template/html/*
var htmlTemplate embed.FS

//go:embed template/css/*
var cssTemplate embed.FS

func ReadTemplate(q *Quotation) error {
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

	q.Data.CustomCSS = template.CSS(cssContent)
	q.Data.HtmlTemplate = string(htmlContent)
	q.Data.LogoBase64 = base64.StdEncoding.EncodeToString(logoBytes)
	q.Data.SignatureBase64 = base64.StdEncoding.EncodeToString(sigBytes)
	q.Data.FontRegularBase64 = base64.StdEncoding.EncodeToString(fontRegularBytes)
	q.Data.FontBoldBase64 = base64.StdEncoding.EncodeToString(fontBoldBytes)

	return nil
}

func InsertDate(q *Quotation) {
	now := time.Now()

	q.Data.Validity = 30
	q.Data.PaymentTerms = 90
	q.Data.LeadTime = 60
	q.Data.Date = now.Format("January 02, 2006")
	q.Data.ValidUntil = now.AddDate(0, 0, int(q.Data.Validity)).Format("January 02, 2006")
}

func InsertSenderRecipient(q *Quotation) {
	q.Data.SenderName = "Quotation Team"
	q.Data.RecipientName = "Recipient Name"
	q.Data.RecipientCompany = "Recipient Company"
	q.Data.RecipientAddress = "Recipient Address"
}

func InsertItemPrices(q *Quotation) {
	q.Data.Items = itemList

	q.Data.DiscountPercent = 5
	for _, item := range itemList {
		q.Data.SubTotal += item.Total
	}

	q.Data.DiscountAmount = (q.Data.SubTotal * uint64(q.Data.DiscountPercent)) / 100
	q.Data.GrandTotal = q.Data.SubTotal - q.Data.DiscountAmount

	q.Data.DisplaySubTotal = formatRupiah(q.Data.SubTotal)
	q.Data.DisplayDiscountAmount = formatRupiah(q.Data.DiscountAmount)
	q.Data.DisplayGrandTotal = formatRupiah(q.Data.GrandTotal)
}

func InsertQuotationInfo(q *Quotation) {
	q.Data.QuoteNumber = "APP-2026-01-001-R0"
	q.Data.Subject = "Supply & Install lighting sensor system (PIR Schneider) at AHU Bat1"
}
