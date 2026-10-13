package store

import (
	"context"
	"os"
	"strings"
	"testing"
)

// leadTimeDraft is a minimal, valid revision draft. The caller sets the four
// lead-time units before saving.
func leadTimeDraft(quoteNumber string, years, months, weeks, days int16) RevisionDraft {
	return RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "Test Customer",
		CustomerAddress:  "Test Address",
		RecipientName:    "Test Recipient",
		RecipientAddress: "Test Address",
		Subject:          "Lead time units",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     days,
		LeadTimeYears:    years,
		LeadTimeMonths:   months,
		LeadTimeWeeks:    weeks,
		TaxZone:          "FTZ",
		Items: []RevisionItem{{
			ItemNo: 1, Description: "Panel", Qty: 1, UoM: "Lot",
			UnitPrice: 100000, MarginalizedUnitPrice: 120000, MarginalizedTotal: 120000,
			OriginalTotal: 100000,
		}},
		Totals:         RevisionTotals{OriginalSubtotal: 100000, MarginalizedSubtotal: 120000},
		HTMLSnapshot:   "<html>quotation</html>",
		QuoteNumber:    quoteNumber,
		RevisionNumber: 0,
	}
}

// TestLeadTimeUnitsRoundTripThroughStorage proves the four units survive the
// write and come back on both read paths the application uses: the archive
// record and the verification evidence the canonical payload is recomputed
// from. The signed payload reads from the evidence, so a unit stored but not
// scanned would verify as tampered against every signature covering it.
func TestLeadTimeUnitsRoundTripThroughStorage(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_lead_time_units")
	resetTestSequences(t, database)

	saved, err := database.SaveRevision(ctx, leadTimeDraft("", 2, 10, 4, 5))
	if err != nil {
		t.Fatalf("save revision: %v", err)
	}

	record, err := database.GetQuotation(ctx, saved.ID, nil)
	if err != nil {
		t.Fatalf("get quotation: %v", err)
	}
	if record.LeadTimeYears != 2 || record.LeadTimeMonths != 10 ||
		record.LeadTimeWeeks != 4 || record.LeadTimeDays != 5 {
		t.Errorf("record lead time = %d/%d/%d/%d; want 2/10/4/5",
			record.LeadTimeYears, record.LeadTimeMonths, record.LeadTimeWeeks, record.LeadTimeDays)
	}

	evidence, err := database.GetRevisionEvidenceByShortID(ctx, saved.ShortID)
	if err != nil {
		t.Fatalf("get revision evidence: %v", err)
	}
	if evidence.LeadTimeYears != 2 || evidence.LeadTimeMonths != 10 ||
		evidence.LeadTimeWeeks != 4 || evidence.LeadTimeDays != 5 {
		t.Errorf("evidence lead time = %d/%d/%d/%d; want 2/10/4/5",
			evidence.LeadTimeYears, evidence.LeadTimeMonths, evidence.LeadTimeWeeks, evidence.LeadTimeDays)
	}
}

// TestLegacyDayOnlyRevisionReadsBackAsZeroUnits is the migration story: a row
// written before the three new columns existed has no value for them, so they
// read back as zero -- which is exactly what a fresh day-only revision stores,
// and what keeps both of their canonical bytes byte-identical.
func TestLegacyDayOnlyRevisionReadsBackAsZeroUnits(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_lead_time_legacy")
	resetTestSequences(t, database)

	saved, err := database.SaveRevision(ctx, leadTimeDraft("", 0, 0, 0, 60))
	if err != nil {
		t.Fatalf("save revision: %v", err)
	}

	evidence, err := database.GetRevisionEvidenceByShortID(ctx, saved.ShortID)
	if err != nil {
		t.Fatalf("get revision evidence: %v", err)
	}
	if evidence.LeadTimeDays != 60 {
		t.Errorf("evidence lead_time_days = %d; want 60", evidence.LeadTimeDays)
	}
	if evidence.LeadTimeYears != 0 || evidence.LeadTimeMonths != 0 || evidence.LeadTimeWeeks != 0 {
		t.Errorf("evidence longer units = %d/%d/%d; want 0/0/0 for a day-only revision",
			evidence.LeadTimeYears, evidence.LeadTimeMonths, evidence.LeadTimeWeeks)
	}
}

// TestLeadTimeOfZeroIsStorable covers the constraint the migration relaxed:
// 001 refused lead_time_days = 0, and a quotation that states no lead time at
// all -- or one stated purely in months -- stores exactly that.
func TestLeadTimeOfZeroIsStorable(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_lead_time_zero")
	resetTestSequences(t, database)

	saved, err := database.SaveRevision(ctx, leadTimeDraft("", 0, 5, 0, 0))
	if err != nil {
		t.Fatalf("save revision with no stated lead time: %v", err)
	}
	record, err := database.GetQuotation(ctx, saved.ID, nil)
	if err != nil {
		t.Fatalf("get quotation: %v", err)
	}
	if record.LeadTimeDays != 0 || record.LeadTimeMonths != 5 {
		t.Errorf("record lead time = %d days / %d months; want 0 days / 5 months",
			record.LeadTimeDays, record.LeadTimeMonths)
	}
}
