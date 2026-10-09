package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestOldRevisionItemsReadBackAsNoRowDiscount is the migration story for
// #274, tested at the layer that can actually break it.
//
// Migration 020 added discount_enabled, discount_percent and list_unit_price
// to quotation_revision_items. Every revision written before it carries no row
// discount and its signature was computed over canonical bytes that never
// mentioned one, so two things have to hold for the printed codes out there to
// keep verifying:
//
//  1. an old row reads back as "no discount", and the store still hands the
//     verifier exactly the priced figures it did before; and
//  2. the derived discount money is zero rather than negative -- list_unit_price
//     defaults to 0 on those rows, so deriving unconditionally would produce
//     0 - unit, a figure that never meant anything.
//
// If this fails, do NOT bump CanonicalPrefix to make it pass -- that stops every
// code already printed from verifying. Find what started emitting the field.
func TestOldRevisionItemsReadBackAsNoRowDiscount(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_legacy_discount")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}

	const quoteNumber = "15029998"
	schema := database.configSchema()
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.quotations WHERE quote_number = $1`, schema), quoteNumber); err != nil {
		t.Fatalf("clean legacy quotation: %v", err)
	}

	record, err := database.SaveRevision(ctx, RevisionDraft{
		HTMLSnapshot:     "<html><body>legacy quotation</body></html>",
		QuoteNumber:      quoteNumber,
		CustomerCode:     2,
		CustomerName:     "Test Customer",
		CustomerAddress:  "Test Address",
		RecipientName:    "Test Recipient",
		RecipientAddress: "Test Address",
		Subject:          "Legacy quotation",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     10,
		TaxZone:          "FTZ",
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "Control panel",
			Qty:                   2,
			UoM:                   "Lot",
			UnitPrice:             15000000,
			MarginEnabled:         true,
			MarginPercent:         20,
			MarginAmount:          3000000,
			OriginalTotal:         30000000,
			MarginalizedUnitPrice: 18000000,
			MarginalizedTotal:     36000000,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:          30000000,
			OriginalGrand:             30000000,
			MarginalizedSubtotal:      36000000,
			MarginalizedGrand:         36000000,
			CustomerGrandTotal:        36000000,
			MarginalizedGrandAfterTax: 36000000,
		},
	})
	if err != nil {
		t.Fatalf("save legacy-shaped revision: %v", err)
	}

	// Rewrite the item row to exactly the shape migration 020 left behind on
	// production data: the new columns present, at their defaults.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_revision_items
		SET discount_enabled = FALSE, discount_percent = 0, list_unit_price = 0
		WHERE revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE quotation_id = $1 AND revision_number = 0
		)`, schema, schema), record.ID); err != nil {
		t.Fatalf("reset discount columns to their migration defaults: %v", err)
	}

	got, err := database.GetQuotation(ctx, record.ID, nil)
	if err != nil {
		t.Fatalf("read quotation: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("quotation has %d items; want 1", len(got.Items))
	}
	item := got.Items[0]

	if item.DiscountEnabled {
		t.Error("a revision written before #274 reads discount_enabled = true")
	}
	if item.DiscountPercent != 0 {
		t.Errorf("discount_percent = %d; want 0", item.DiscountPercent)
	}
	if item.ListUnitPrice != 0 {
		t.Errorf("list_unit_price = %d; want the migration default of 0", item.ListUnitPrice)
	}
	// The bug this test exists for: list 0 minus unit 18000000 is a negative
	// "discount" on every quotation ever issued.
	if item.DiscountAmount != 0 {
		t.Errorf("derived discount amount = %d on a row with no discount; want 0 "+
			"(list_unit_price defaults to 0 on old rows)", item.DiscountAmount)
	}
	// And the priced figures the verifier signs over must be untouched.
	if item.MarginalizedUnitPrice != 18000000 || item.MarginalizedTotal != 36000000 {
		t.Errorf("item = %d/%d; want 18000000/36000000, unchanged by the migration",
			item.MarginalizedUnitPrice, item.MarginalizedTotal)
	}

	evidence, err := database.GetRevisionEvidenceByShortID(ctx, record.ShortID)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	if len(evidence.Items) != 1 {
		t.Fatalf("evidence has %d items; want 1", len(evidence.Items))
	}
	if evidence.Items[0].DiscountPercent != 0 {
		t.Errorf("evidence discount_percent = %d; want 0, or the canonical bytes change",
			evidence.Items[0].DiscountPercent)
	}
	if evidence.Items[0].UnitPrice != 18000000 || evidence.Items[0].Total != 36000000 {
		t.Errorf("evidence item = %d/%d; want 18000000/36000000",
			evidence.Items[0].UnitPrice, evidence.Items[0].Total)
	}
}
