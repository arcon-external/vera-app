package backend

import (
	"strings"
	"testing"

	"vera-app/backend/auth"
)

func noteInput(note string) QuotationInput {
	return QuotationInput{
		Items: []QuotationItemInput{{
			Description: "Control panel", Qty: 1, UoM: "Lot", UnitPrice: 1000000,
		}},
		Subject:       "Quotation note",
		RecipientName: "Budi Santoso",
		Validity:      30,
		PaymentTerm:   30,
		CustomerID:    2,
		Incoterm:      "EXW",
		Note:          note,
	}
}

// TestPrepareTrimsAndCarriesTheNote proves the note is normalized once, at the
// seam every caller goes through, and reaches all three consumers: the signed
// data, the customer preview, and the immutable revision draft.
func TestPrepareTrimsAndCarriesTheNote(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	const want = "Delivery is ex-works.\nInstallation by the customer."
	if err := q.Prepare(noteInput("  " + want + "  ")); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if q.Data.Note != want {
		t.Errorf("data note = %q; want %q", q.Data.Note, want)
	}

	preview, err := q.GetCustomerPreview()
	if err != nil {
		t.Fatalf("customer preview: %v", err)
	}
	if preview.Note != want {
		t.Errorf("customer preview note = %q; want %q", preview.Note, want)
	}

	draft, err := q.DraftRevision("<html>quotation</html>", nil, "")
	if err != nil {
		t.Fatalf("revision draft: %v", err)
	}
	if draft.Note != want {
		t.Errorf("revision draft note = %q; want %q", draft.Note, want)
	}
}

// TestPrepareTreatsAWhitespaceOnlyNoteAsAbsent keeps "empty means no block"
// true for a note a person typed as spaces: trimming happens before the
// canonical check, so the printed document and the signed payload agree.
func TestPrepareTreatsAWhitespaceOnlyNoteAsAbsent(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := q.Prepare(noteInput(" \n\t ")); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if q.Data.Note != "" {
		t.Errorf("data note = %q; want empty after trimming", q.Data.Note)
	}
}

// TestPrepareRejectsANoteOverTheLimit proves the 1000-character rule is
// counted in characters, not bytes: a 1000-character multibyte note is
// accepted and a 1001-character one is refused with the limit named.
func TestPrepareRejectsANoteOverTheLimit(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())

	if err := q.Prepare(noteInput(strings.Repeat("é", 1000))); err != nil {
		t.Fatalf("a 1000-character multibyte note was refused: %v", err)
	}

	err := q.Prepare(noteInput(strings.Repeat("é", 1001)))
	if err == nil {
		t.Fatal("a 1001-character note was accepted")
	}
	if !strings.Contains(err.Error(), "1000") {
		t.Errorf("over-limit error = %q; want it to name the 1000-character limit", err)
	}
}
