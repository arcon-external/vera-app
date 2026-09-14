package store

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

func revisionList(revisions []int16) string {
	parts := make([]string, 0, len(revisions))
	for _, revision := range revisions {
		parts = append(parts, strconv.Itoa(int(revision)))
	}
	return strings.Join(parts, ",")
}

func TestQuotationRevisionPersistence(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}

	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	draft := RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "Test Customer",
		CustomerAddress:  "Test Address",
		RecipientName:    "Test Recipient",
		RecipientAddress: "Test Address",
		Subject:          "Initial quotation",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     10,
		DiscountPercent:  5,
		TaxZone:          "FTZ",
		TaxRateBps:       0,
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "Test item",
			Qty:                   1,
			UoM:                   "Unit",
			UnitPrice:             100000,
			MarginEnabled:         true,
			MarginPercent:         20,
			MarginAmount:          20000,
			OriginalTotal:         100000,
			MarginalizedUnitPrice: 120000,
			MarginalizedTotal:     120000,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:          100000,
			OriginalDiscount:          5000,
			OriginalGrand:             95000,
			MarginalizedSubtotal:      120000,
			MarginalizedDiscount:      6000,
			MarginalizedGrand:         114000,
			MarginalizedGrandAfterTax: 114000,
			ProfitAmount:              19000,
			NettProfit:                19000,
		},
		HTMLSnapshot: "<html>quotation</html>",
		PDFData:      []byte{1, 2, 3},
	}

	first, err := database.SaveRevision(ctx, draft)
	if err != nil {
		t.Fatalf("save revision 0: %v", err)
	}
	if first.RevisionNumber != 0 {
		t.Errorf("first revision = %d; want 0", first.RevisionNumber)
	}
	if !strings.HasPrefix(first.QuoteNumber, "1502") {
		t.Errorf("quote number = %q; want customer prefix 1502", first.QuoteNumber)
	}

	draft.QuoteNumber = first.QuoteNumber
	draft.RevisionNumber = 1
	draft.Subject = "Edited quotation"
	second, err := database.SaveRevision(ctx, draft)
	if err != nil {
		t.Fatalf("save revision 1: %v", err)
	}
	if second.RevisionNumber != 1 {
		t.Errorf("second revision = %d; want 1", second.RevisionNumber)
	}

	latest, err := database.GetQuotation(ctx, first.ID, nil)
	if err != nil {
		t.Fatalf("get latest revision: %v", err)
	}
	if latest.RevisionNumber != 1 || latest.Subject != "Edited quotation" {
		t.Errorf("latest = revision %d subject %q; want revision 1 edited quotation", latest.RevisionNumber, latest.Subject)
	}

	revisionZero := int16(0)
	historical, err := database.GetQuotation(ctx, first.ID, &revisionZero)
	if err != nil {
		t.Fatalf("get historical revision: %v", err)
	}
	if historical.RevisionNumber != 0 || historical.Subject != "Initial quotation" {
		t.Errorf("historical = revision %d subject %q; want revision 0 initial quotation", historical.RevisionNumber, historical.Subject)
	}

	list, err := database.ListQuotations(ctx)
	if err != nil {
		t.Fatalf("list quotations: %v", err)
	}
	// The archive shows one row per quotation at its newest revision.
	if len(list) != 1 || list[0].RevisionNumber != 1 {
		t.Fatalf("list = %#v; want a single row at revision 1", list)
	}
	if gaps := revisionList(latest.AvailableRevisions); gaps != "1,0" {
		t.Errorf("available revisions = %s; want 1,0", gaps)
	}

	if err := database.DeleteQuotation(ctx, first.ID); err != nil {
		t.Fatalf("delete quotation: %v", err)
	}
	if _, err := database.GetQuotation(ctx, first.ID, nil); err == nil {
		t.Fatal("expected deleted quotation lookup to fail")
	}

	draft.QuoteNumber = "15029999"
	draft.RevisionNumber = 0
	explicit, err := database.SaveRevision(ctx, draft)
	if err != nil {
		t.Fatalf("save explicitly numbered revision 0: %v", err)
	}
	if explicit.QuoteNumber != "15029999" || explicit.RevisionNumber != 0 {
		t.Errorf("explicit revision = %q/%d; want 15029999/0", explicit.QuoteNumber, explicit.RevisionNumber)
	}
	if err := database.DeleteQuotation(ctx, explicit.ID); err != nil {
		t.Fatalf("delete explicitly numbered quotation: %v", err)
	}
}

func TestPeekQuoteSequenceNeverConsumes(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}

	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_peek"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	if got, err := database.PeekQuoteSequence(ctx); err != nil || got != 1 {
		t.Fatalf("fresh peek = %d, %v; want 1, nil", got, err)
	}
	for want := uint16(1); want <= 2; want++ {
		got, err := database.NextQuoteSequence(ctx)
		if err != nil || got != want {
			t.Fatalf("allocate = %d, %v; want %d, nil", got, err, want)
		}
	}
	if got, err := database.PeekQuoteSequence(ctx); err != nil || got != 3 {
		t.Fatalf("peek after two allocations = %d, %v; want 3, nil", got, err)
	}
	if got, err := database.NextQuoteSequence(ctx); err != nil || got != 3 {
		t.Fatalf("allocate after peek = %d, %v; want 3, nil", got, err)
	}
}
