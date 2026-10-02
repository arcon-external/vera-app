package backend

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// Benchmarks for the pricing path quoted in the project README and on the
// engineering resume. Run with:
//
//	go test ./backend -run '^$' -bench 'Pricing' -benchmem
//
// Two numbers matter and they are not the same:
//
//   - BenchmarkBuildPricing measures the arithmetic only. Every amount is a
//     uint64 and every rate a basis point, so there is no float rounding and
//     no I/O. This is the number that justifies the "integer basis points"
//     claim: cost does not scale with document size in any meaningful way.
//
//   - BenchmarkBuildItemsAndPricing measures the realistic per-document path,
//     which also formats each line for display (rupiahPrinter.Sprintf).
//     Formatting dominates, so this is the honest figure for "time to price
//     one quotation" and it is the number worth quoting externally.
//
// Fixtures are deterministic — no Math.random, no time dependence — so the
// numbers are reproducible across runs and machines.

// realisticItems builds n line items with varied quantities, prices and
// margins, shaped like an electrical/mechanical contracting quotation.
func realisticItems(n int) []QuotationItemInput {
	units := []string{"Lot", "PC", "EA", "Set", "Meter"}
	basePrices := []uint64{
		48_500_000, 1_250_000, 12_750_000, 7_200_000, 3_450_000,
		96_000_000, 875_000, 22_300_000, 5_600_000, 1_050_000,
	}
	quantities := []uint32{1, 120, 4, 1, 12, 2, 60, 8, 3, 250}
	margins := []uint8{20, 25, 18, 15, 22, 12, 30, 16, 24, 10}

	items := make([]QuotationItemInput, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, QuotationItemInput{
			Description:   fmt.Sprintf("Line item %03d — supply and installation of plant", i+1),
			Qty:           quantities[i%len(quantities)],
			UoM:           units[i%len(units)],
			UnitPrice:     basePrices[i%len(basePrices)],
			MarginEnabled: i%7 != 0, // a realistic mix, not every line margined
			MarginPercent: margins[i%len(margins)],
		})
	}
	return items
}

// benchmarkSizes spans a typical quotation through a very large one.
var benchmarkSizes = []int{4, 20, 100, 500}

// BenchmarkBuildPricing measures the integer arithmetic alone: subtotals,
// discount, PPN, services tax and profit. Items are prepared once outside the
// loop so only buildPricing is timed.
func BenchmarkBuildPricing(b *testing.B) {
	for _, n := range benchmarkSizes {
		b.Run(fmt.Sprintf("lines=%d", n), func(b *testing.B) {
			items := buildItems(realisticItems(n))
			// Non-FTZ goods: PPN 11% on the customer document. Matches the
			// common production case in builder_test.go.
			const ppnBps, servicesBps = 1100, 0

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				summary := buildPricing(items, 5, ppnBps, servicesBps)
				// Keep the result observable so it cannot be optimised away.
				if summary.CustomerGrandTotal == 0 {
					b.Fatal("unexpected zero grand total")
				}
			}
		})
	}
}

// BenchmarkBuildItemsAndPricing measures the full in-process path a user
// triggers on Prepare: apply per-line markup, format every display string,
// then run the arithmetic. This is the number to quote for per-document
// latency because it includes the allocation-heavy formatting that
// BenchmarkBuildPricing deliberately excludes.
func BenchmarkBuildItemsAndPricing(b *testing.B) {
	for _, n := range benchmarkSizes {
		b.Run(fmt.Sprintf("lines=%d", n), func(b *testing.B) {
			input := realisticItems(n)
			const ppnBps, servicesBps = 1100, 0

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				items := buildItems(input)
				summary := buildPricing(items, 5, ppnBps, servicesBps)
				if summary.CustomerGrandTotal == 0 {
					b.Fatal("unexpected zero grand total")
				}
			}
		})
	}
}

// BenchmarkApplyMarkup isolates the single per-line operation that the money
// path exists to get right: cost + trunc(cost * margin / 100), in integers.
func BenchmarkApplyMarkup(b *testing.B) {
	costs := []uint64{1_250_000, 12_750_000, 48_500_000, 96_000_000}
	margins := []uint8{10, 15, 20, 25, 30}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := applyMarkup(costs[i%len(costs)], margins[i%len(margins)])
		if got == 0 {
			b.Fatal("unexpected zero markup")
		}
	}
}

// BenchmarkBuildPDF measures the full document render, which shells out to
// headless Chrome via chromedp's PrintToPDF. Every call opens a fresh browser
// context, so this is dominated by process and IPC cost — not by the pricing
// arithmetic measured above.
//
// It is opt-in, mirroring the existing chromedp test, because it needs a
// working Chrome runtime and is far too slow for `go test ./...`:
//
//	VERA_RUN_CHROME_TESTS=1 go test ./backend -run '^$' -bench 'BuildPDF' -benchmem
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
