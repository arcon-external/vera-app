package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Item and sub numbers are the stable identity a bought quantity, an Unsold
// remainder and a ghost row point at (#648). SaveRevision is the last seam a
// revision passes before it is stored, so it refuses a line or part that
// claims a number the previous revision used for something else. Removals
// leave gaps; new numbers go above every number the previous revision
// carried; edits to a still-present number are ordinary work.

func identityItem(no int, description string) RevisionItem {
	return RevisionItem{
		ItemNo:                no,
		Description:           description,
		Qty:                   1,
		UoM:                   "Lot",
		UnitPrice:             1000,
		OriginalTotal:         1000,
		MarginalizedUnitPrice: 1000,
		MarginalizedTotal:     1000,
		ListUnitPrice:         1000,
	}
}

func identityPackage(no int, description string, subs ...RevisionSubItem) RevisionItem {
	item := identityItem(no, description)
	item.SubItems = subs
	return item
}

func identityPart(subNo int, description string) RevisionSubItem {
	return RevisionSubItem{
		SubNo:          subNo,
		Description:    description,
		Qty:            1,
		UoM:            "Pc",
		UnitCost:       1000,
		UnitPrice:      1000,
		Total:          1000,
		ListUnitPrice:  1000,
		MarginAmount:   0,
		MarginPercent:  0,
		DiscountAmount: 0,
	}
}

func identityDraft(quoteNumber string, revision int16, items []RevisionItem) RevisionDraft {
	return RevisionDraft{
		QuoteNumber:      quoteNumber,
		RevisionNumber:   revision,
		CustomerCode:     2,
		CustomerName:     "PT. Identity",
		CustomerAddress:  "Jl. Identity 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          "Stable identity",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		TaxZone:          "NonFTZ",
		TaxCategory:      "SupplyInstall",
		Currency:         "IDR",
		HTMLSnapshot:     "<html>snapshot</html>",
		Items:            items,
	}
}

func identityNumbers(t *testing.T, database *Store, quotationID int64) []int {
	t.Helper()
	record, err := database.GetQuotation(context.Background(), quotationID, nil)
	if err != nil {
		t.Fatalf("GetQuotation: %v", err)
	}
	numbers := make([]int, 0, len(record.Items))
	for _, item := range record.Items {
		numbers = append(numbers, item.ItemNo)
	}
	return numbers
}

func TestSaveRevisionRefusesAReusedItemNumber(t *testing.T) {
	database := openTestSchema(t, "vera_identity_reused_item")
	ctx := context.Background()

	first, err := database.SaveRevision(ctx, identityDraft("", 0, []RevisionItem{
		identityItem(1, "Line one"),
		identityItem(3, "Line three"),
	}))
	if err != nil {
		t.Fatalf("first revision: %v", err)
	}

	// Line 2 was never used by revision 0; a new line claiming it would make
	// any purchase history keyed on 1.2 point at this product instead.
	_, err = database.SaveRevision(ctx, identityDraft(first.QuoteNumber, 1, []RevisionItem{
		identityItem(1, "Line one"),
		identityItem(2, "A different product"),
	}))
	if !errors.Is(err, ErrQuotationItemIdentityReused) {
		t.Fatalf("reused item number error = %v; want ErrQuotationItemIdentityReused", err)
	}
	if !strings.Contains(err.Error(), "item 2") {
		t.Errorf("error = %q; want it to name the reused number", err)
	}

	// The refusal must write nothing: the quotation stays on revision 0 with
	// its original lines.
	if got := identityNumbers(t, database, first.ID); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("stored numbers after the refusal = %v; want [1 3] at revision 0", got)
	}
	record, err := database.GetQuotation(ctx, first.ID, nil)
	if err != nil {
		t.Fatalf("GetQuotation after refusal: %v", err)
	}
	if record.RevisionNumber != 0 {
		t.Errorf("revision after the refusal = %d; want 0 — a refused revision was stored", record.RevisionNumber)
	}
}

func TestSaveRevisionAcceptsANewItemAboveTheMaximum(t *testing.T) {
	database := openTestSchema(t, "vera_identity_new_item_above")
	ctx := context.Background()

	first, err := database.SaveRevision(ctx, identityDraft("", 0, []RevisionItem{
		identityItem(1, "Line one"),
		identityItem(3, "Line three"),
	}))
	if err != nil {
		t.Fatalf("first revision: %v", err)
	}

	if _, err := database.SaveRevision(ctx, identityDraft(first.QuoteNumber, 1, []RevisionItem{
		identityItem(1, "Line one"),
		identityItem(3, "Line three"),
		identityItem(4, "A genuinely new product"),
	})); err != nil {
		t.Fatalf("a new line numbered above the previous maximum was refused: %v", err)
	}
	if got := identityNumbers(t, database, first.ID); len(got) != 3 ||
		got[0] != 1 || got[1] != 3 || got[2] != 4 {
		t.Errorf("stored numbers = %v; want [1 3 4] (the gap and the new number both survive)", got)
	}
}

func TestSaveRevisionRefusesAReusedSubItemNumber(t *testing.T) {
	database := openTestSchema(t, "vera_identity_reused_sub")
	ctx := context.Background()

	first, err := database.SaveRevision(ctx, identityDraft("", 0, []RevisionItem{
		identityPackage(1, "Package line",
			identityPart(1, "Part one"),
			identityPart(3, "Part three"),
		),
	}))
	if err != nil {
		t.Fatalf("first revision: %v", err)
	}

	_, err = database.SaveRevision(ctx, identityDraft(first.QuoteNumber, 1, []RevisionItem{
		identityPackage(1, "Package line",
			identityPart(1, "Part one"),
			identityPart(2, "A different part"),
		),
	}))
	if !errors.Is(err, ErrQuotationItemIdentityReused) {
		t.Fatalf("reused sub-item number error = %v; want ErrQuotationItemIdentityReused", err)
	}
	if !strings.Contains(err.Error(), "sub-item 2") {
		t.Errorf("error = %q; want it to name the reused sub-item number", err)
	}
}

func TestSaveRevisionAcceptsANewSubItemAboveTheMaximum(t *testing.T) {
	database := openTestSchema(t, "vera_identity_new_sub_above")
	ctx := context.Background()

	first, err := database.SaveRevision(ctx, identityDraft("", 0, []RevisionItem{
		identityPackage(1, "Package line",
			identityPart(1, "Part one"),
			identityPart(3, "Part three"),
		),
	}))
	if err != nil {
		t.Fatalf("first revision: %v", err)
	}

	if _, err := database.SaveRevision(ctx, identityDraft(first.QuoteNumber, 1, []RevisionItem{
		identityPackage(1, "Package line",
			identityPart(1, "Part one"),
			identityPart(3, "Part three"),
			identityPart(4, "A genuinely new part"),
		),
	})); err != nil {
		t.Fatalf("a new part numbered above the previous maximum was refused: %v", err)
	}

	record, err := database.GetQuotation(ctx, first.ID, nil)
	if err != nil {
		t.Fatalf("GetQuotation: %v", err)
	}
	if len(record.Items) != 1 || len(record.Items[0].SubItems) != 3 {
		t.Fatalf("stored package = %d items / %d parts; want 1 / 3",
			len(record.Items), len(record.Items[0].SubItems))
	}
	got := record.Items[0].SubItems
	if got[0].SubNo != 1 || got[1].SubNo != 3 || got[2].SubNo != 4 {
		t.Errorf("stored part numbers = %d, %d, %d; want 1, 3, 4",
			got[0].SubNo, got[1].SubNo, got[2].SubNo)
	}
}

func TestSaveRevisionAllowsEditsToExistingNumbersAndRemovals(t *testing.T) {
	database := openTestSchema(t, "vera_identity_existing_edit")
	ctx := context.Background()

	first, err := database.SaveRevision(ctx, identityDraft("", 0, []RevisionItem{
		identityItem(1, "Line one"),
		identityItem(2, "Line two"),
		identityItem(3, "Line three"),
	}))
	if err != nil {
		t.Fatalf("first revision: %v", err)
	}

	// Removing line 2 and editing the lines that remain is ordinary work:
	// every submitted number still exists in the previous revision.
	edited := identityItem(1, "Line one, reworded")
	edited.UnitPrice = 1500
	edited.OriginalTotal = 1500
	edited.MarginalizedUnitPrice = 1500
	edited.MarginalizedTotal = 1500
	edited.ListUnitPrice = 1500
	if _, err := database.SaveRevision(ctx, identityDraft(first.QuoteNumber, 1, []RevisionItem{
		edited,
		identityItem(3, "Line three"),
	})); err != nil {
		t.Fatalf("an edit to existing numbers with one removal was refused: %v", err)
	}

	record, err := database.GetQuotation(ctx, first.ID, nil)
	if err != nil {
		t.Fatalf("GetQuotation: %v", err)
	}
	if len(record.Items) != 2 || record.Items[0].ItemNo != 1 || record.Items[1].ItemNo != 3 {
		t.Fatalf("stored lines = %d with numbers %v; want 2 lines numbered 1 and 3",
			len(record.Items), identityNumbers(t, database, first.ID))
	}
	if record.Items[0].Description != "Line one, reworded" || record.Items[0].UnitPrice != 1500 {
		t.Errorf("the edit did not survive storage: %q at %d",
			record.Items[0].Description, record.Items[0].UnitPrice)
	}
	if record.Items[1].Description != "Line three" {
		t.Errorf("line 3 moved or changed: %q", record.Items[1].Description)
	}
}
