package store

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestRevisionCurrencyPersistence pins that the issue currency travels with
// the immutable revision: a USD draft reads back as USD through the record,
// the list row and the verification evidence, while a draft that never set
// one stores as the default. Without the read-back, SignableFromEvidence
// could not reproduce the signed bytes and exported signatures would stop
// verifying.
func TestRevisionCurrencyPersistence(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}

	ctx := context.Background()
	database := openTestSchema(t, "vera_test_revision_currency")

	newDraft := func(currencyCode string) RevisionDraft {
		return RevisionDraft{
			CustomerCode:     2,
			CustomerName:     "Test Customer",
			Currency:         currencyCode,
			CustomerAddress:  "Test Address",
			RecipientName:    "Test Recipient",
			RecipientAddress: "Test Address",
			Subject:          "Currency quotation",
			Incoterm:         "EXW",
			ValidityDays:     30,
			PaymentTermDays:  30,
			LeadTimeDays:     10,
			TaxZone:          "FTZ",
			Items: []RevisionItem{{
				ItemNo: 1, Description: "Line", Qty: 1, UoM: "Lot",
				UnitPrice: 1000000, MarginalizedUnitPrice: 1000000,
				MarginalizedTotal: 1000000, OriginalTotal: 1000000,
			}},
			HTMLSnapshot: "<html>quotation</html>",
		}
	}

	usd, err := database.SaveRevision(ctx, newDraft("USD"))
	if err != nil {
		t.Fatalf("save USD revision: %v", err)
	}
	record, err := database.GetQuotation(ctx, usd.ID, nil)
	if err != nil {
		t.Fatalf("get USD revision: %v", err)
	}
	if record.Currency != "USD" {
		t.Errorf("record currency = %q; want USD", record.Currency)
	}
	evidence, err := database.GetRevisionEvidenceByShortID(ctx, ShortIDForRevision(usd.UUID, 0))
	if err != nil {
		t.Fatalf("get USD evidence: %v", err)
	}
	if evidence.Currency != "USD" {
		t.Errorf("evidence currency = %q; want USD", evidence.Currency)
	}
	list, err := database.ListQuotations(ctx)
	if err != nil {
		t.Fatalf("list quotations: %v", err)
	}
	if len(list) != 1 || list[0].Currency != "USD" {
		t.Fatalf("list = %#v; want one USD row", list)
	}

	// A draft that never set a currency stores as the default: every revision
	// written before the column existed reads back as what it was signed as.
	legacy, err := database.SaveRevision(ctx, newDraft(""))
	if err != nil {
		t.Fatalf("save currency-less revision: %v", err)
	}
	legacyEvidence, err := database.GetRevisionEvidenceByShortID(ctx, ShortIDForRevision(legacy.UUID, 0))
	if err != nil {
		t.Fatalf("get legacy evidence: %v", err)
	}
	if legacyEvidence.Currency != "IDR" {
		t.Errorf("legacy evidence currency = %q; want the IDR default", legacyEvidence.Currency)
	}
}
