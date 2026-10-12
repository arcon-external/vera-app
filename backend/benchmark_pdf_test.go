package backend

import (
	"context"
	"strconv"
	"testing"

	"vera-app/backend/render"
)

// The renderer is the most expensive thing in the codebase by two orders of
// magnitude -- the pricing arithmetic beside it is half a microsecond with zero
// allocations -- and until now it was not measured at all.
//
// BenchmarkBuildPDF used to cover it, but it was gated on VERA_RUN_CHROME_TESTS
// and exercised chromedp, the engine that ships in no binary. So the numbers that
// mattered were the ones nobody could run.
//
// This measures render.Active, which is whichever engine the build selected: the
// native one by default, the browser one under -tags chromedp. One benchmark, and
// it always measures what the binary would actually do.
//
// Sizes matter here because the cost is linear in document length and the
// constant is large -- a single export is tens of milliseconds, so a change that
// doubles it is worth seeing rather than feeling.

// benchmarkQuotation builds a prepared quotation of the given length, using the
// same field wiring Prepare does. The summary is real arithmetic rather than
// literals, so the benchmark measures rendering and not a fixture.
func benchmarkQuotation(b *testing.B, lines int) *Quotation {
	b.Helper()

	quotation := &Quotation{}
	if err := Startup(quotation, context.Background()); err != nil {
		b.Fatalf("startup: %v", err)
	}

	items := buildItems(realisticItems(lines), "IDR")
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
	return quotation
}

func BenchmarkRenderActive(b *testing.B) {
	// Skips when the build selected an engine this machine cannot run, rather
	// than failing with a message about a missing browser.
	render.RequireUsable(b)

	// Two sizes, so linear scaling is observable rather than asserted. A
	// realistic quotation is around twenty lines; a hundred is the load the
	// pagination tests already exercise.
	for _, lines := range []int{20, 100} {
		b.Run(engineName()+"/lines="+strconv.Itoa(lines), func(b *testing.B) {
			quotation := benchmarkQuotation(b, lines)

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
				// Bytes per second rather than per operation: the useful
				// question is how fast the document is produced, and it is the
				// one that stays comparable when the size changes.
				b.SetBytes(int64(len(pdf)))
			}
		})
	}
}

func engineName() string { return render.Active.Name() }
