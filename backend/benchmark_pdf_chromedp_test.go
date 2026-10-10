//go:build chromedp

package backend

import (
	"context"
	"os"
	"testing"
)

// This benchmark measures the browser engine, which is why it is behind the
// same tag: an untagged build has no chromedp to measure. See
// benchmark_pdf_test.go for the engine that ships.

func BenchmarkBuildPDF(b *testing.B) {
	if os.Getenv("VERA_RUN_CHROME_TESTS") == "" {
		b.Skip("set VERA_RUN_CHROME_TESTS=1 to exercise the chromedp PDF build")
	}

	quotation := &Quotation{}
	if err := Startup(quotation, context.Background()); err != nil {
		b.Fatalf("startup: %v", err)
	}

	items := buildItems(realisticItems(20))
	summary := buildPricing(items, 5, 1100, 0)

	quotation.Data.Subject = "Supply and installation of electrical and mechanical systems"
	quotation.Data.RecipientName = "Benchmark Recipient"
	quotation.Data.RecipientCompany = "Benchmark Recipient Co"
	quotation.Data.Items = items
	quotation.Data.DiscountPercent = 5
	quotation.Data.SubTotal = summary.MarginalizedSubtotal
	quotation.Data.DiscountAmount = summary.MarginalizedDiscount
	quotation.Data.GrandTotal = summary.CustomerGrandTotal
	quotation.Data.SellGrandTotal = summary.MarginalizedGrand
	quotation.Data.OriginalSubTotal = summary.OriginalSubtotal
	quotation.Data.OriginalDiscountAmount = summary.OriginalDiscount
	quotation.Data.OriginalGrandTotal = summary.OriginalGrand
	quotation.Data.MarginalizedGrandAfterTax = summary.MarginalizedGrandAfterTax
	quotation.Data.ProfitAmount = summary.ProfitAmount
	quotation.Data.NettProfit = summary.NettProfit
	quotation.Data.PpnAmount = summary.PpnAmount
	quotation.Data.DisplaySubTotal = formatRupiah(summary.MarginalizedSubtotal)
	quotation.Data.DisplayDiscountAmount = formatRupiah(summary.MarginalizedDiscount)
	quotation.Data.DisplayGrandTotal = formatRupiah(summary.CustomerGrandTotal)
	quotation.Data.DisplayPpnAmount = formatRupiah(summary.PpnAmount)
	quotation.Data.DisplayTaxAmount = formatRupiah(summary.TaxAmount)
	quotation.Data.DisplayNettProfit = formatSignedRupiah(summary.NettProfit)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pdf, err := quotation.build()
		if err != nil {
			b.Fatalf("build: %v", err)
		}
		if len(pdf) == 0 {
			b.Fatal("empty pdf")
		}
		b.SetBytes(int64(len(pdf)))
	}
}
