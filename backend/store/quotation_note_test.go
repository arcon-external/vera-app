package store

import (
	"context"
	"strings"
	"testing"
)

// noteDraft is a minimal valid revision draft carrying one quotation note.
// The note is the only thing these tests vary.
func noteDraft(note string) RevisionDraft {
	return RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "PT Note Customer",
		CustomerAddress:  "Test Address",
		RecipientName:    "Test Recipient",
		RecipientAddress: "Test Address",
		Subject:          "Quotation note storage",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     10,
		TaxZone:          "FTZ",
		Note:             note,
		Items: []RevisionItem{{
			ItemNo: 1, Description: "Line", Qty: 1, UoM: "Lot",
			UnitPrice: 1000000, MarginalizedUnitPrice: 1000000,
			MarginalizedTotal: 1000000, OriginalTotal: 1000000,
		}},
		Totals:       RevisionTotals{OriginalSubtotal: 1000000, MarginalizedSubtotal: 1000000},
		HTMLSnapshot: "<html>quotation</html>",
	}
}

// TestRevisionNoteRoundTripsThroughReadAndEvidence proves the note is stored on
// the immutable revision and returned by both reads that recompute a
// verification code: the archive/detail read and the evidence read. Missing
// either one makes a noteful revision report tampering.
func TestRevisionNoteRoundTripsThroughReadAndEvidence(t *testing.T) {
	database := openTestSchema(t, "vera_test_quotation_note")
	ctx := context.Background()
	const note = "Delivery is ex-works.\nInstallation by the customer."

	saved, err := database.SaveRevision(ctx, noteDraft(note))
	if err != nil {
		t.Fatalf("save revision: %v", err)
	}

	record, err := database.GetQuotation(ctx, saved.ID, nil)
	if err != nil {
		t.Fatalf("get quotation: %v", err)
	}
	if record.Note != note {
		t.Errorf("stored revision note = %q; want %q", record.Note, note)
	}

	evidence, err := database.GetRevisionEvidenceByShortID(ctx, saved.ShortID)
	if err != nil {
		t.Fatalf("get revision evidence: %v", err)
	}
	if evidence.Note != note {
		t.Errorf("evidence note = %q; want %q", evidence.Note, note)
	}
}

// TestAbsentRevisionNoteReadsAsEmpty is the migration story: a revision issued
// before the column existed reads back an empty note from both paths, which is
// what keeps its canonical bytes and printed code unchanged.
func TestAbsentRevisionNoteReadsAsEmpty(t *testing.T) {
	database := openTestSchema(t, "vera_test_quotation_note_absent")
	ctx := context.Background()

	saved, err := database.SaveRevision(ctx, noteDraft(""))
	if err != nil {
		t.Fatalf("save revision: %v", err)
	}
	record, err := database.GetQuotation(ctx, saved.ID, nil)
	if err != nil {
		t.Fatalf("get quotation: %v", err)
	}
	if record.Note != "" {
		t.Errorf("stored revision note = %q; want empty", record.Note)
	}
	evidence, err := database.GetRevisionEvidenceByShortID(ctx, saved.ShortID)
	if err != nil {
		t.Fatalf("get revision evidence: %v", err)
	}
	if evidence.Note != "" {
		t.Errorf("evidence note = %q; want empty", evidence.Note)
	}
}

// TestRevisionNoteOverTheLimitIsRefused keeps the store's own boundary honest:
// the API and Prepare reject a long note with a validation message, and the
// store refuses it too rather than relying on the caller.
func TestRevisionNoteOverTheLimitIsRefused(t *testing.T) {
	database := openTestSchema(t, "vera_test_quotation_note_limit")
	ctx := context.Background()

	if _, err := database.SaveRevision(ctx, noteDraft(strings.Repeat("a", 1001))); err == nil {
		t.Fatal("a 1001-character note was stored; want a validation error")
	} else if !strings.Contains(err.Error(), "1000") {
		t.Errorf("over-limit error = %q; want it to name the 1000-character limit", err)
	}

	if _, err := database.SaveRevision(ctx, noteDraft(strings.Repeat("a", 1000))); err != nil {
		t.Errorf("a 1000-character note was refused: %v", err)
	}
}
