package localseed

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"vera-app/backend/extract/pdfgen"
	"vera-app/backend/store"
)

// Synthetic documents only. Every page says what it is, so a screenshot of a
// seeded document cannot be mistaken for a customer's paperwork. The generator
// is backend/extract/pdfgen, the same text-support package the extraction tests
// use; nothing here is a production renderer and nothing claims extraction
// accuracy.

func quotationPDF(
	quoteNumber string,
	revision int16,
	customer, subject string,
	items []store.RevisionItem,
	totals store.RevisionTotals,
) []byte {
	lines := []pdfgen.Line{
		{Text: "FICTIONAL MOCK QUOTATION - NOT A REAL CUSTOMER DOCUMENT", Y: 60, Size: 13},
		{Text: "Created by the local Vera seed for manual testing only.", Y: 82, Size: 9},
		{Text: "Quotation " + quoteNumber + "  Revision " + fmt.Sprintf("%02d", revision), Y: 112},
		{Text: "Customer: " + customer, Y: 130},
		{Text: "Subject: " + subject, Y: 148},
		{Text: "Line  Description                                   Qty  UoM   Unit price      Total", Y: 184, Size: 9},
	}
	y := 202.0
	for _, item := range items {
		lines = append(lines, pdfgen.Line{
			Text: truncate(fmt.Sprintf("%-5d %-44s %4d %-5s %14s %14s",
				item.ItemNo, item.Description, item.Qty, item.UoM,
				rupiah(item.MarginalizedUnitPrice), rupiah(item.MarginalizedTotal)), 110),
			Y: y, Size: 8,
		})
		y += 14
		for _, sub := range item.SubItems {
			lines = append(lines, pdfgen.Line{
				Text: truncate(fmt.Sprintf("      %d.%d %-42s %4d %-5s %14s %14s",
					item.ItemNo, sub.SubNo, sub.Description, sub.Qty, sub.UoM,
					rupiah(sub.UnitPrice), rupiah(sub.Total)), 110),
				Y: y, Size: 8,
			})
			y += 14
		}
	}
	y += 12
	lines = append(lines,
		pdfgen.Line{Text: "Subtotal        " + rupiah(totals.MarginalizedSubtotal), Y: y, Size: 9},
		pdfgen.Line{Text: "Discount        " + rupiah(totals.MarginalizedDiscount), Y: y + 16, Size: 9},
		pdfgen.Line{Text: "PPN             " + rupiah(totals.PpnAmount), Y: y + 32, Size: 9},
		pdfgen.Line{Text: "Grand total     " + rupiah(totals.CustomerGrandTotal), Y: y + 48, Size: 10},
		pdfgen.Line{Text: "SYNTHETIC PDF - no extraction accuracy is claimed.", Y: y + 74, Size: 8},
	)
	return pdfgen.Build([][]pdfgen.Line{lines})
}

func quotationSnapshot(quoteNumber string, revision int16, customer, subject string) string {
	var builder strings.Builder
	builder.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>Fictional mock quotation</title></head><body>")
	builder.WriteString("<h1>FICTIONAL MOCK QUOTATION</h1>")
	builder.WriteString("<p>Created by the local Vera seed for manual testing only.</p>")
	builder.WriteString("<p>Quotation " + htmlEscape(quoteNumber) + " revision " + fmt.Sprintf("%02d", revision) + "</p>")
	builder.WriteString("<p>Customer: " + htmlEscape(customer) + "</p>")
	builder.WriteString("<p>Subject: " + htmlEscape(subject) + "</p>")
	builder.WriteString("<p>SYNTHETIC DOCUMENT - not a real customer quotation.</p>")
	builder.WriteString("</body></html>")
	return builder.String()
}

func purchaseOrderTextPDF(poNumber, sourceQuote, customer string, items []store.PurchaseOrderItem, grandTotal int64) []byte {
	lines := []pdfgen.Line{
		{Text: "FICTIONAL MOCK CUSTOMER PURCHASE ORDER - NOT A REAL DOCUMENT", Y: 60, Size: 12},
		{Text: "Created by the local Vera seed for manual testing only.", Y: 82, Size: 9},
		{Text: "Customer PO " + poNumber, Y: 112},
		{Text: "Source quotation " + sourceQuote, Y: 130},
		{Text: "Customer " + customer, Y: 148},
		{Text: "Line  Description                                   Qty   Unit price      Net", Y: 184, Size: 9},
	}
	y := 202.0
	for index, item := range items {
		lines = append(lines, pdfgen.Line{
			Text: truncate(fmt.Sprintf("%-5d %-44s %6s %14s %14s",
				index+1, item.Description, quantity(item.QuantityScaled),
				rupiah(item.UnitPriceMinor), rupiah(item.NetMinor)), 110),
			Y: y, Size: 8,
		})
		y += 14
	}
	lines = append(lines,
		pdfgen.Line{Text: "Grand total     " + rupiah(grandTotal), Y: y + 16, Size: 10},
		pdfgen.Line{Text: "SYNTHETIC PDF - no extraction accuracy is claimed.", Y: y + 42, Size: 8},
	)
	return pdfgen.Build([][]pdfgen.Line{lines})
}

// purchaseOrderMixedPDF is the mixed manual/scanned fixture: a text header page
// a person could type from, followed by an image-only page with no text layer,
// the shape a scanned table has.
func purchaseOrderMixedPDF(poNumber string, items []store.PurchaseOrderItem, grandTotal int64) []byte {
	header := []pdfgen.Line{
		{Text: "FICTIONAL MOCK CUSTOMER PURCHASE ORDER - SYNTHETIC MIXED MANUAL/SCANNED", Y: 60, Size: 12},
		{Text: "Created by the local Vera seed for manual testing only.", Y: 82, Size: 9},
		{Text: "Customer PO " + poNumber + " (header typed manually)", Y: 112},
		{Text: "The second page is an image-only synthetic scan.", Y: 130},
		{Text: "Grand total on the typed page: " + rupiah(grandTotal), Y: 148},
	}
	y := 184.0
	for index, item := range items {
		header = append(header, pdfgen.Line{
			Text: truncate(fmt.Sprintf("%-4d %-50s %8s %14s",
				index+1, item.Description, quantity(item.QuantityScaled), rupiah(item.NetMinor)), 110),
			Y: y, Size: 8,
		})
		y += 14
	}
	scan := syntheticScanPage()
	return pdfgen.BuildMixed([]pdfgen.Page{
		{Lines: header},
		{Image: &scan},
	})
}

// syntheticScanPage is a deterministic grey "scanned table": no text, no
// customer content, just enough structure that a reader sees a page image.
func syntheticScanPage() pdfgen.ImagePage {
	const width, height = 600, 800
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	background := color.RGBA{R: 246, G: 246, B: 242, A: 255}
	border := color.RGBA{R: 90, G: 90, B: 90, A: 255}
	rule := color.RGBA{R: 160, G: 160, B: 160, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.SetRGBA(x, y, background)
		}
	}
	for x := 40; x < width-40; x++ {
		canvas.SetRGBA(x, 40, border)
		canvas.SetRGBA(x, height-40, border)
	}
	for y := 40; y < height-40; y++ {
		canvas.SetRGBA(40, y, border)
		canvas.SetRGBA(width-40, y, border)
	}
	for y := 120; y < height-80; y += 42 {
		for x := 70; x < width-70; x++ {
			canvas.SetRGBA(x, y, rule)
		}
	}
	return pdfgen.NewImagePage(canvas, 0)
}

func rupiah(amount int64) string {
	negative := amount < 0
	if negative {
		amount = -amount
	}
	digits := strconv.FormatInt(amount, 10)
	var grouped strings.Builder
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}
	if negative {
		return "Rp -" + grouped.String()
	}
	return "Rp " + grouped.String()
}

func quantity(scaled int64) string {
	whole := scaled / store.QuantityScale
	fraction := scaled % store.QuantityScale
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	text := fmt.Sprintf("%d.%04d", whole, fraction)
	return strings.TrimRight(text, "0")
}

func truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max]
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return replacer.Replace(value)
}
