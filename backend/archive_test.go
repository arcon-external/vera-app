package backend

import (
	"testing"

	"vera-app/backend/auth"
)

func TestRevisionDraftPreservesInternalAndCustomerValues(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	q.Data = QuotationData{
		CustomerID:                2,
		RecipientCompany:          "PT. Test Customer",
		RecipientAddress:          "Test address",
		RevisionID:                1,
		RecipientName:             "Test recipient",
		Subject:                   "Test quotation",
		Incoterm:                  "EXW",
		Validity:                  30,
		PaymentTerm:               90,
		LeadTime:                  10,
		DiscountPercent:           5,
		TaxZone:                   TaxZoneNonFTZ,
		TaxCategory:               TaxCategorySupplyInstall,
		TaxRateBps:                200,
		OriginalSubTotal:          100000,
		OriginalDiscountAmount:    5000,
		OriginalGrandTotal:        95000,
		SubTotal:                  120000,
		DiscountAmount:            6000,
		GrandTotal:                114000,
		TaxAmount:                 2280,
		MarginalizedGrandAfterTax: 111720,
		ProfitAmount:              19000,
		NettProfit:                16720,
		Items: []QuotationItem{{
			No:                1,
			Description:       "Test item",
			Qty:               1,
			UoM:               "Unit",
			OriginalUnitPrice: 100000,
			OriginalTotal:     100000,
			MarginEnabled:     true,
			MarginPercent:     20,
			MarginAmount:      20000,
			UnitPrice:         120000,
			Total:             120000,
		}},
	}
	q.Data.QuoteNumber = "15020001"

	draft := q.revisionDraft("<html>snapshot</html>", []byte{1}, "/tmp/quotation.pdf")
	if draft.QuoteNumber != "15020001" || draft.RevisionNumber != 1 {
		t.Errorf("draft identity = %q/revision %d; want 15020001/1", draft.QuoteNumber, draft.RevisionNumber)
	}
	if draft.Items[0].UnitPrice != 100000 || draft.Items[0].MarginalizedUnitPrice != 120000 {
		t.Errorf("draft original/marginalized unit = %d/%d; want 100000/120000", draft.Items[0].UnitPrice, draft.Items[0].MarginalizedUnitPrice)
	}
	if draft.Totals.NettProfit != 16720 {
		t.Errorf("draft nett profit = %d; want 16720", draft.Totals.NettProfit)
	}
	if draft.HTMLSnapshot == "" || len(draft.PDFData) != 1 {
		t.Error("draft did not preserve rendered document snapshots")
	}

}
