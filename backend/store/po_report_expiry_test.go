package store

import (
	"testing"
	"time"

	"vera-app/backend/poreport"
)

// TestPurchaseOrderReportExpiryAgreesWithFulfillmentAtTheBoundary pins the two
// expiry computations to one rule. The fulfillment SQL derives the WIB expiry
// date in Postgres while quotationExpiryAt derives it in Go for the selector and
// the workspace; the report pipeline keeps Unsold through both. A quotation on
// the validity boundary must read open in fulfillment and not expired in
// quotationExpiryAt, and the next calendar day must move both at once. If they
// ever disagree, this test names it rather than letting a screen call a live
// offer lapsed.
func TestPurchaseOrderReportExpiryAgreesWithFulfillmentAtTheBoundary(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_expiry_agreement")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Expiry Boundary", "Boundary source")
	finalizeReportPO(t, database, ctx, user, quotation, "PO-EXPIRY-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})

	const validityDays int16 = 30
	// Issued on the WIB day 2026-10-20 (03:00 in Jakarta, still the previous
	// UTC day), the offer's last valid day is 2026-11-19 -- the report's own
	// as-of day. Both rules must keep it open. One WIB day earlier it lapses;
	// the same issue date one as-of day later lapses as well.
	cases := []struct {
		name           string
		issuedAt       time.Time
		now            time.Time
		wantValidUntil string
		wantExpired    bool
		wantStatus     string
	}{
		{
			name:           "last valid day is still valid",
			issuedAt:       time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC),
			now:            reportNow(),
			wantValidUntil: "2026-11-19",
			wantExpired:    false,
			wantStatus:     string(poreport.FulfillmentStatusOpen),
		},
		{
			name:           "the day after the last valid day lapses",
			issuedAt:       time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC),
			now:            reportNow().AddDate(0, 0, 1),
			wantValidUntil: "2026-11-19",
			wantExpired:    true,
			wantStatus:     string(poreport.FulfillmentStatusExpiredUnclosed),
		},
		{
			name:           "one WIB day earlier has already lapsed",
			issuedAt:       time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC),
			now:            reportNow(),
			wantValidUntil: "2026-11-18",
			wantExpired:    true,
			wantStatus:     string(poreport.FulfillmentStatusExpiredUnclosed),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			setQuotationIssueWindow(t, database, quotation.ID, testCase.issuedAt, validityDays)

			report, err := database.ReportPurchaseOrders(ctx,
				reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"),
				testCase.now)
			if err != nil {
				t.Fatalf("ReportPurchaseOrders: %v", err)
			}
			page, err := database.ReportPurchaseOrderFulfillment(ctx,
				fulfillmentQuery("IDR", "all", "", 1, 100), testCase.now)
			if err != nil {
				t.Fatalf("ReportPurchaseOrderFulfillment: %v", err)
			}
			row := fulfillmentQuotation(t, page, quotation.QuoteNumber)

			// The Go rule the selector and workspace use.
			validUntil, expired := quotationExpiryAt(testCase.issuedAt, validityDays, testCase.now)
			if got := validUntil.Format(time.DateOnly); got != testCase.wantValidUntil {
				t.Fatalf("quotationExpiryAt valid-until = %s; want %s",
					got, testCase.wantValidUntil)
			}
			if expired != testCase.wantExpired {
				t.Errorf("quotationExpiryAt expired = %v; want %v", expired, testCase.wantExpired)
			}

			// The fulfillment SQL must agree on both figures.
			if row.ValidUntil != testCase.wantValidUntil {
				t.Errorf("fulfillment valid-until = %s; want %s (quotationExpiryAt says %s)",
					row.ValidUntil, testCase.wantValidUntil, validUntil.Format(time.DateOnly))
			}
			if row.Status != testCase.wantStatus {
				t.Errorf("fulfillment status = %q; want %q", row.Status, testCase.wantStatus)
			}
			if gotExpired := row.Status == string(poreport.FulfillmentStatusExpiredUnclosed); gotExpired != expired {
				t.Errorf("expiry disagreement: fulfillment expired=%v, quotationExpiryAt expired=%v",
					gotExpired, expired)
			}

			// Expiry is never a loss: the three bought of five quoted units
			// leave the same two-unit Unsold pipeline whichever day it is.
			if report.Pipeline.UnsoldAmountMinor != 20_000 ||
				report.Pipeline.UnsoldMarginMinor != 8_000 {
				t.Errorf("pipeline unsold = %d/%d; want 20000/8000 on every expiry day",
					report.Pipeline.UnsoldAmountMinor, report.Pipeline.UnsoldMarginMinor)
			}
		})
	}
}
