package store

import (
	"context"
	"testing"
)

// recipientCompanyDraft is a minimal valid revision draft whose customer name
// is the company the document prints. The snapshot under test is exactly that
// name: it must travel with the revision rather than being re-read from the
// live customer row.
func recipientCompanyDraft(customerName, quoteNumber string, revision int16) RevisionDraft {
	return RevisionDraft{
		CustomerCode:     2,
		CustomerName:     customerName,
		CustomerAddress:  "Test Address",
		RecipientName:    "Test Recipient",
		RecipientAddress: "Test Address",
		Subject:          "Recipient company snapshot",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     10,
		TaxZone:          "FTZ",
		QuoteNumber:      quoteNumber,
		RevisionNumber:   revision,
		Items: []RevisionItem{{
			ItemNo: 1, Description: "Line", Qty: 1, UoM: "Lot",
			UnitPrice: 1000000, MarginalizedUnitPrice: 1000000,
			MarginalizedTotal: 1000000, OriginalTotal: 1000000,
		}},
		Totals:       RevisionTotals{OriginalSubtotal: 1000000, MarginalizedSubtotal: 1000000},
		HTMLSnapshot: "<html>quotation</html>",
	}
}

// renameTestCustomer renames the seeded client code 2 through the same store
// path an admin client edit uses. The code stays immutable.
func renameTestCustomer(t *testing.T, database *Store, name string) {
	t.Helper()
	ctx := context.Background()
	clients, err := database.ListClients(ctx)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	for _, client := range clients {
		if client.Code != 2 {
			continue
		}
		if _, err := database.UpdateClient(ctx, client.ID, name, "Test Address"); err != nil {
			t.Fatalf("rename client: %v", err)
		}
		return
	}
	t.Fatal("seeded client code 2 not found")
}

// TestCustomerRenameKeepsTheIssuedRecipientCompany is the integrity fix: the
// name printed and signed on a revision is the name stored on that revision,
// so renaming the customer afterwards cannot change what the old document
// says or what its verification code recomputes.
func TestCustomerRenameKeepsTheIssuedRecipientCompany(t *testing.T) {
	database := openTestSchema(t, "vera_test_recipient_company")
	ctx := context.Background()

	saved, err := database.SaveRevision(ctx, recipientCompanyDraft("PT Issued Name", "", 0))
	if err != nil {
		t.Fatalf("save revision: %v", err)
	}

	assertName := func(label, want string) {
		t.Helper()
		record, err := database.GetQuotation(ctx, saved.ID, nil)
		if err != nil {
			t.Fatalf("%s: get quotation: %v", label, err)
		}
		if record.CustomerName != want {
			t.Errorf("%s: read path company = %q; want %q", label, record.CustomerName, want)
		}
		evidence, err := database.GetRevisionEvidenceByShortID(ctx, saved.ShortID)
		if err != nil {
			t.Fatalf("%s: get revision evidence: %v", label, err)
		}
		if evidence.RecipientCompany != want {
			t.Errorf("%s: evidence company = %q; want %q", label, evidence.RecipientCompany, want)
		}
		list, err := database.ListQuotations(ctx)
		if err != nil {
			t.Fatalf("%s: list quotations: %v", label, err)
		}
		if len(list) != 1 || list[0].CustomerName != want {
			t.Errorf("%s: archive row = %#v; want one row naming %q", label, list, want)
		}
	}

	assertName("before rename", "PT Issued Name")
	renameTestCustomer(t, database, "PT Renamed")
	assertName("after rename", "PT Issued Name")
}

// TestNewRevisionAfterRenameSnapshotsTheNewCompany proves the snapshot is
// per revision: the old revision keeps the name it printed, and the revision
// issued after the rename stores, prints and signs the new one.
func TestNewRevisionAfterRenameSnapshotsTheNewCompany(t *testing.T) {
	database := openTestSchema(t, "vera_test_recipient_company_revision")
	ctx := context.Background()

	first, err := database.SaveRevision(ctx, recipientCompanyDraft("PT Old Name", "", 0))
	if err != nil {
		t.Fatalf("save first revision: %v", err)
	}
	renameTestCustomer(t, database, "PT New Name")
	second, err := database.SaveRevision(ctx,
		recipientCompanyDraft("PT New Name", first.QuoteNumber, 1))
	if err != nil {
		t.Fatalf("save second revision: %v", err)
	}

	revisionZero, revisionOne := int16(0), int16(1)
	oldRecord, err := database.GetQuotation(ctx, first.ID, &revisionZero)
	if err != nil {
		t.Fatalf("get revision 0: %v", err)
	}
	if oldRecord.CustomerName != "PT Old Name" {
		t.Errorf("revision 0 company = %q; want the name it printed", oldRecord.CustomerName)
	}
	newRecord, err := database.GetQuotation(ctx, first.ID, &revisionOne)
	if err != nil {
		t.Fatalf("get revision 1: %v", err)
	}
	if newRecord.CustomerName != "PT New Name" {
		t.Errorf("revision 1 company = %q; want the renamed company", newRecord.CustomerName)
	}

	oldEvidence, err := database.GetRevisionEvidenceByShortID(ctx, first.ShortID)
	if err != nil {
		t.Fatalf("get revision 0 evidence: %v", err)
	}
	if oldEvidence.RecipientCompany != "PT Old Name" {
		t.Errorf("revision 0 evidence company = %q; want the name it signed", oldEvidence.RecipientCompany)
	}
	newEvidence, err := database.GetRevisionEvidenceByShortID(ctx, second.ShortID)
	if err != nil {
		t.Fatalf("get revision 1 evidence: %v", err)
	}
	if newEvidence.RecipientCompany != "PT New Name" {
		t.Errorf("revision 1 evidence company = %q; want the renamed company", newEvidence.RecipientCompany)
	}

	// The archive row is the latest revision, so it shows the new name.
	list, err := database.ListQuotations(ctx)
	if err != nil {
		t.Fatalf("list quotations: %v", err)
	}
	if len(list) != 1 || list[0].CustomerName != "PT New Name" {
		t.Fatalf("archive row = %#v; want one row naming the renamed company", list)
	}
}

// TestEmptyRecipientCompanySnapshotFallsBackToLiveCustomer is the migration
// story: revisions written before the snapshot existed read back empty, and
// keep their current behaviour by resolving the live customer name.
func TestEmptyRecipientCompanySnapshotFallsBackToLiveCustomer(t *testing.T) {
	database := openTestSchema(t, "vera_test_recipient_company_legacy")
	ctx := context.Background()

	saved, err := database.SaveRevision(ctx, recipientCompanyDraft("PT Legacy Name", "", 0))
	if err != nil {
		t.Fatalf("save revision: %v", err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE `+database.configSchema()+`.quotation_revisions
		SET recipient_company = ''
		WHERE quotation_id = $1`, saved.ID); err != nil {
		t.Fatalf("clear recipient company snapshot: %v", err)
	}

	renameTestCustomer(t, database, "PT Renamed Live Name")

	record, err := database.GetQuotation(ctx, saved.ID, nil)
	if err != nil {
		t.Fatalf("get quotation: %v", err)
	}
	if record.CustomerName != "PT Renamed Live Name" {
		t.Errorf("read path company = %q; want the live fallback", record.CustomerName)
	}
	evidence, err := database.GetRevisionEvidenceByShortID(ctx, saved.ShortID)
	if err != nil {
		t.Fatalf("get revision evidence: %v", err)
	}
	if evidence.RecipientCompany != "PT Renamed Live Name" {
		t.Errorf("evidence company = %q; want the live fallback", evidence.RecipientCompany)
	}
	list, err := database.ListQuotations(ctx)
	if err != nil {
		t.Fatalf("list quotations: %v", err)
	}
	if len(list) != 1 || list[0].CustomerName != "PT Renamed Live Name" {
		t.Errorf("archive row = %#v; want the live fallback", list)
	}
}
