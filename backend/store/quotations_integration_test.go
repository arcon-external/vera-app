package store

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"vera-app/backend/apicontract"
)

func revisionList(revisions []int16) string {
	parts := make([]string, 0, len(revisions))
	for _, revision := range revisions {
		parts = append(parts, strconv.Itoa(int(revision)))
	}
	return strings.Join(parts, ",")
}

// resetTestSequences clears counter state so sequence tests are hermetic:
// reruns on a warm database observe the same allocations as a fresh one.
func resetTestSequences(t *testing.T, database *Store) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(
		"DELETE FROM %s.sequence_counters", database.configSchema())); err != nil {
		t.Fatalf("reset sequence counters: %v", err)
	}
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
	resetTestSequences(t, database)
	if pool, err := database.poolOrError(); err != nil {
		t.Fatalf("pool: %v", err)
	} else if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.quotations WHERE quote_number = '15029999'`, database.configSchema())); err != nil {
		t.Fatalf("clean explicit quotation: %v", err)
	}

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
	wantPrefix := apicontract.FormatQuoteNumber(2, CurrentYY(), 1)
	if first.QuoteNumber != wantPrefix {
		t.Errorf("quote number = %q; want %q", first.QuoteNumber, wantPrefix)
	}
	if first.UUID == "" {
		t.Error("saved revision has empty UUID")
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
	resetTestSequences(t, database)

	const customer, yy = 2, 26
	if got, warn, err := database.PeekQuoteSequence(ctx, customer, yy); err != nil || got != 1 || warn {
		t.Fatalf("fresh peek = %d, %v, %v; want 1, false, nil", got, warn, err)
	}
	for want := int64(1); want <= 2; want++ {
		got, warn, err := database.NextQuoteSequence(ctx, customer, yy)
		if err != nil || got != want || warn {
			t.Fatalf("allocate = %d, %v, %v; want %d, false, nil", got, warn, err, want)
		}
	}
	if got, warn, err := database.PeekQuoteSequence(ctx, customer, yy); err != nil || got != 3 || warn {
		t.Fatalf("peek after two allocations = %d, %v, %v; want 3, false, nil", got, warn, err)
	}
	if got, warn, err := database.NextQuoteSequence(ctx, customer, yy); err != nil || got != 3 || warn {
		t.Fatalf("allocate after peek = %d, %v, %v; want 3, false, nil", got, warn, err)
	}
}

func TestQuoteSequencesArePerCustomerPerYear(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}

	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_cycles"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	resetTestSequences(t, database)

	// Infineon at 0777 does not move Datalec: A,B,A allocates 1,1,2.
	if got, _, err := database.NextQuoteSequence(ctx, 1, 26); err != nil || got != 1 {
		t.Fatalf("Datalec first = %d, %v; want 1, nil", got, err)
	}
	if got, _, err := database.NextQuoteSequence(ctx, 2, 26); err != nil || got != 1 {
		t.Fatalf("Infineon first = %d, %v; want 1, nil", got, err)
	}
	if got, _, err := database.NextQuoteSequence(ctx, 1, 26); err != nil || got != 2 {
		t.Fatalf("Datalec second = %d, %v; want 2, nil", got, err)
	}
	// A new year restarts at 1.
	if got, _, err := database.NextQuoteSequence(ctx, 1, 27); err != nil || got != 1 {
		t.Fatalf("Datalec new year = %d, %v; want 1, nil", got, err)
	}
	// Out-of-scope inputs are rejected, not allocated.
	if _, _, err := database.NextQuoteSequence(ctx, 0, 26); err == nil {
		t.Error("customer code 0 allocated; want error")
	}
	if _, _, err := database.NextQuoteSequence(ctx, 100, 26); err == nil {
		t.Error("customer code 100 allocated; want error")
	}
	if _, _, err := database.PeekQuoteSequence(ctx, 1, 100); err == nil {
		t.Error("year 100 peeked; want error")
	}
}

func TestQuoteSequenceOverflowWidensInsteadOfWrapping(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}

	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_overflow"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	resetTestSequences(t, database)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	key := SequenceKey(4, 26)
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.sequence_counters (name, next_value) VALUES ($1, 9999)
		 ON CONFLICT (name) DO UPDATE SET next_value = 9999`, database.configSchema()), key); err != nil {
		t.Fatalf("seed counter: %v", err)
	}
	if got, warn, err := database.NextQuoteSequence(ctx, 4, 26); err != nil || got != 9999 || !warn {
		t.Fatalf("cap allocate = %d, %v, %v; want 9999, true, nil", got, warn, err)
	}
	got, warn, err := database.NextQuoteSequence(ctx, 4, 26)
	if err != nil || got != 10000 || !warn {
		t.Fatalf("overflow allocate = %d, %v, %v; want 10000, true, nil", got, warn, err)
	}
	if number := apicontract.FormatQuoteNumber(4, 26, got); number != "15042610000" {
		t.Errorf("overflow number = %q; want 15042610000", number)
	}
}

func TestQuoteNumberCompositeUniqueness(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}

	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_uniq"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.quotations WHERE quote_number = '1504260001'`, schema)); err != nil {
		t.Fatalf("clean quotations: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.customers WHERE customer_code IN (11, 12)`, schema)); err != nil {
		t.Fatalf("clean customers: %v", err)
	}
	// Identical numbers across different customers are accepted: the guard
	// is composite on (customer_id, quote_number), not global.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.customers (customer_code, legal_name) VALUES (11, 'Uniq A'), (12, 'Uniq B')
		 ON CONFLICT DO NOTHING`, schema)); err != nil {
		t.Fatalf("seed customers: %v", err)
	}
	var idA, idB int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT id FROM %s.customers WHERE customer_code = 11`, schema)).Scan(&idA); err != nil {
		t.Fatalf("fetch customer A: %v", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT id FROM %s.customers WHERE customer_code = 12`, schema)).Scan(&idB); err != nil {
		t.Fatalf("fetch customer B: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.quotations (quote_number, customer_id) VALUES ('1504260001', $1)`, schema), idA); err != nil {
		t.Fatalf("insert first: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.quotations (quote_number, customer_id) VALUES ('1504260001', $1)`, schema), idB); err != nil {
		t.Fatalf("same number for another customer rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.quotations (quote_number, customer_id) VALUES ('1504260001', $1)`, schema), idA); err == nil {
		t.Error("duplicate (customer, number) accepted; want unique violation")
	}
}
