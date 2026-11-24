package backend

import (
	"errors"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// Issue #652: a new revision is row-only. The builder keeps reading a stored
// legacy header discount for history, but Prepare refuses to create one.

func rowOnlyQuotationInput(items []QuotationItemInput, discountPercent uint8) QuotationInput {
	return QuotationInput{
		Items:           items,
		Subject:         "Row-only test",
		RecipientName:   "Test",
		Validity:        30,
		PaymentTerm:     30,
		LeadTimeDays:    30,
		CustomerID:      2,
		Incoterm:        "EXW",
		DiscountPercent: discountPercent,
		TaxZone:         TaxZoneFTZ,
	}
}

func TestPrepareRefusesANewWholeOrderDiscount(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())

	err := q.Prepare(rowOnlyQuotationInput([]QuotationItemInput{{
		Description: "Widget", Qty: 1, UoM: "Pc", UnitPrice: 100000,
	}}, 1))
	if !errors.Is(err, store.ErrQuotationHeaderDiscountRetired) {
		t.Fatalf("Prepare with a whole-order discount = %v; want ErrQuotationHeaderDiscountRetired", err)
	}

	if err := q.Prepare(rowOnlyQuotationInput([]QuotationItemInput{{
		Description: "Widget", Qty: 1, UoM: "Pc", UnitPrice: 100000,
	}}, 0)); err != nil {
		t.Fatalf("Prepare with a row-only input: %v", err)
	}
	if q.Data.DiscountPercent != 0 {
		t.Errorf("prepared whole-order discount = %d; want 0", q.Data.DiscountPercent)
	}
}

// TestRowOnlyTotalsAndMarginsEqualTheSumOfRows pins the arithmetic the new
// model promises: the document totals are exactly the summed row totals, and
// profit is exactly the summed per-row margins. Nothing is rounded twice or
// counted on a package rollup the rows did not earn.
func TestRowOnlyTotalsAndMarginsEqualTheSumOfRows(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())

	if err := q.Prepare(rowOnlyQuotationInput([]QuotationItemInput{
		{
			Description: "Margined and discounted", Qty: 2, UoM: "Pc", UnitPrice: 1_000_000,
			MarginEnabled: true, MarginPercent: 25,
			DiscountEnabled: true, DiscountPercent: 10,
		},
		{
			Description: "Discounted below cost", Qty: 3, UoM: "Pc", UnitPrice: 250_000,
			DiscountEnabled: true, DiscountPercent: 5,
		},
	}, 0)); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	var sellTotal, costTotal, rowDiscountTotal uint64
	var marginTotal int64
	for _, item := range q.Data.Items {
		sellTotal += item.Total
		costTotal += item.OriginalTotal
		rowDiscountTotal += item.DiscountAmount
		marginTotal += item.MarginAmount * int64(item.Qty)
	}

	if q.Data.SubTotal != sellTotal || q.Data.SellGrandTotal != sellTotal {
		t.Errorf("sell subtotal = %d / grand %d; want the summed rows %d",
			q.Data.SubTotal, q.Data.SellGrandTotal, sellTotal)
	}
	if q.Data.OriginalSubTotal != costTotal || q.Data.OriginalGrandTotal != costTotal {
		t.Errorf("cost subtotal = %d / grand %d; want the summed rows %d",
			q.Data.OriginalSubTotal, q.Data.OriginalGrandTotal, costTotal)
	}
	if q.Data.RowDiscountTotal != rowDiscountTotal {
		t.Errorf("row discount total = %d; want the summed rows %d",
			q.Data.RowDiscountTotal, rowDiscountTotal)
	}
	if q.Data.DiscountAmount != 0 || q.Data.OriginalDiscountAmount != 0 {
		t.Errorf("whole-order discounts = %d / %d; want the retired zero",
			q.Data.DiscountAmount, q.Data.OriginalDiscountAmount)
	}
	if q.Data.ProfitAmount != marginTotal {
		t.Errorf("profit = %d; want the summed row margins %d", q.Data.ProfitAmount, marginTotal)
	}
	if q.Data.GrandTotal != sellTotal {
		t.Errorf("customer grand = %d; want the summed rows %d (FTZ, no PPN)",
			q.Data.GrandTotal, sellTotal)
	}
	if q.Data.NettProfit != marginTotal {
		t.Errorf("nett profit = %d; want the summed row margins %d (FTZ, no tax)",
			q.Data.NettProfit, marginTotal)
	}

	// The internal report is the other reader of the same figures: it must
	// carry the identical sums, not a second calculation.
	bd, err := q.GetInternalBreakdown()
	if err != nil {
		t.Fatalf("GetInternalBreakdown: %v", err)
	}
	if bd.MarginalizedSubtotal != sellTotal || bd.SellSubtotal != sellTotal ||
		bd.OriginalSubtotal != costTotal || bd.CostSubtotal != costTotal {
		t.Errorf("report totals = sell %d/%d, cost %d/%d; want the summed rows %d/%d",
			bd.MarginalizedSubtotal, bd.SellSubtotal, bd.OriginalSubtotal, bd.CostSubtotal,
			sellTotal, costTotal)
	}
	if bd.ProfitAmount != marginTotal || bd.NettProfit != marginTotal {
		t.Errorf("report profit = %d / nett %d; want the summed row margins %d",
			bd.ProfitAmount, bd.NettProfit, marginTotal)
	}
	if bd.MarginalizedDiscount != 0 || bd.OriginalDiscount != 0 || bd.DiscountSell != 0 {
		t.Errorf("report whole-order discounts = %d/%d/%d; want the retired zero",
			bd.MarginalizedDiscount, bd.OriginalDiscount, bd.DiscountSell)
	}
	if bd.RowDiscountTotal != rowDiscountTotal {
		t.Errorf("report row discount total = %d; want the summed rows %d",
			bd.RowDiscountTotal, rowDiscountTotal)
	}
}

// TestBuildPricingStillReadsAStoredLegacyHeaderDiscount is the historical half
// of #652: an already-issued revision keeps its discount and printed totals,
// so the builder must still compute them when a legacy draft is re-read.
func TestBuildPricingStillReadsAStoredLegacyHeaderDiscount(t *testing.T) {
	items := mustBuildItems(t, []QuotationItemInput{
		{Description: "Plain", Qty: 1, UoM: "Lot", UnitPrice: 1000},
		{Description: "Row discounted", Qty: 1, UoM: "Lot", UnitPrice: 2000,
			DiscountEnabled: true, DiscountPercent: 10},
	}, "IDR")

	pricing := buildPricing(items, 10, 0, 0)

	// List 1000 + 1800 = 2800; the stored 10% whole-order discount is 280 and
	// the historical grand is 2520. The row discount (200) is a separate figure
	// and does not replace the legacy one.
	if pricing.MarginalizedSubtotal != 2800 || pricing.MarginalizedDiscount != 280 ||
		pricing.MarginalizedGrand != 2520 {
		t.Errorf("legacy totals = %d/%d/%d; want 2800/280/2520",
			pricing.MarginalizedSubtotal, pricing.MarginalizedDiscount, pricing.MarginalizedGrand)
	}
	if pricing.RowDiscountTotal != 200 {
		t.Errorf("row discount total = %d; want 200", pricing.RowDiscountTotal)
	}
	if pricing.OriginalGrand != 3000 || pricing.ProfitAmount != -480 {
		t.Errorf("cost/profit = %d/%d; want 3000/-480 (the discount cuts only the price)",
			pricing.OriginalGrand, pricing.ProfitAmount)
	}
}
