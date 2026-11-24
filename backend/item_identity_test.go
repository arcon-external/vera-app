package backend

import (
	"strings"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// Item and sub numbers are the stable identity a bought quantity, an Unsold
// remainder and a ghost row point at (#648). The builder used to number by
// array position, so removing a line ahead of a bought leaf shifted every
// following number and silently re-pointed the purchase history at a
// different product. These tests pin the rule: a submitted number survives,
// a new line numbers above every preserved one, and removals leave gaps.

func TestBuildItemsPreservesRemovedLineNumbers(t *testing.T) {
	// Line 1 of 3 was removed before export. The surviving lines must keep
	// their numbers rather than closing the gap.
	items, err := buildItems([]QuotationItemInput{
		{No: 2, Description: "Line two", Qty: 1, UoM: "Lot", UnitPrice: 2000},
		{No: 3, Description: "Line three", Qty: 1, UoM: "Lot", UnitPrice: 3000},
	}, "IDR")
	if err != nil {
		t.Fatalf("buildItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("buildItems returned %d items; want 2", len(items))
	}
	if items[0].No != 2 || items[1].No != 3 {
		t.Errorf("numbers = %d, %d; want 2, 3 — removing line 1 must not renumber the lines after it",
			items[0].No, items[1].No)
	}
}

func TestBuildItemsAssignsANewLineAboveTheMaximum(t *testing.T) {
	// A line added to a quotation whose surviving lines are 2 and 3 takes the
	// next number above every submitted one, in the position the editor put
	// it. It must not take a number that was ever used.
	items, err := buildItems([]QuotationItemInput{
		{Description: "New line", Qty: 1, UoM: "Lot", UnitPrice: 1000},
		{No: 2, Description: "Line two", Qty: 1, UoM: "Lot", UnitPrice: 2000},
		{No: 3, Description: "Line three", Qty: 1, UoM: "Lot", UnitPrice: 3000},
	}, "IDR")
	if err != nil {
		t.Fatalf("buildItems: %v", err)
	}
	got := []uint32{items[0].No, items[1].No, items[2].No}
	want := []uint32{4, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("numbers = %v; want %v (a new line numbers above the highest submitted number)", got, want)
		}
	}
}

func TestBuildItemsPreservesSubItemNumbersAndGaps(t *testing.T) {
	// A package's parts follow the same rule inside their parent: part 2 was
	// removed, so 1 and 3 stay 1 and 3, and a new part takes 4. The printed
	// labels derive from the preserved numbers.
	items, err := buildItems([]QuotationItemInput{{
		No:          2,
		Description: "Package line",
		Qty:         1,
		UoM:         "Lot",
		SubItems: []QuotationSubItemInput{
			{No: 1, Description: "Part one", UoM: "Pc", Qty: 1, UnitCost: 1000},
			{No: 3, Description: "Part three", UoM: "Pc", Qty: 1, UnitCost: 3000},
			{Description: "New part", UoM: "Pc", Qty: 1, UnitCost: 4000},
		},
	}}, "IDR")
	if err != nil {
		t.Fatalf("buildItems: %v", err)
	}
	subs := items[0].SubItems
	if len(subs) != 3 {
		t.Fatalf("sub-items = %d; want 3", len(subs))
	}
	wantNos := []uint32{1, 3, 4}
	wantLabels := []string{"2.1", "2.3", "2.4"}
	for i := range subs {
		if subs[i].No != wantNos[i] || subs[i].SubNo != wantLabels[i] {
			t.Errorf("sub %d = No %d label %q; want No %d label %q",
				i, subs[i].No, subs[i].SubNo, wantNos[i], wantLabels[i])
		}
	}
}

func TestBuildItemsRefusesADuplicateItemNumber(t *testing.T) {
	_, err := buildItems([]QuotationItemInput{
		{No: 2, Description: "First claim on 2", Qty: 1, UoM: "Lot", UnitPrice: 1000},
		{No: 2, Description: "Second claim on 2", Qty: 1, UoM: "Lot", UnitPrice: 2000},
	}, "IDR")
	if err == nil {
		t.Fatal("two lines claiming number 2 were accepted; one identity cannot name two products")
	}
	if !strings.Contains(err.Error(), "number 2 is used more than once") {
		t.Errorf("error = %q; want it to name the repeated number", err)
	}
}

func TestBuildItemsRefusesADuplicateSubItemNumber(t *testing.T) {
	_, err := buildItems([]QuotationItemInput{{
		No:          1,
		Description: "Package line",
		Qty:         1,
		UoM:         "Lot",
		SubItems: []QuotationSubItemInput{
			{No: 1, Description: "First part", UoM: "Pc", Qty: 1, UnitCost: 1000},
			{No: 1, Description: "Second part", UoM: "Pc", Qty: 1, UnitCost: 2000},
		},
	}}, "IDR")
	if err == nil {
		t.Fatal("two parts of one package claiming number 1 were accepted")
	}
	if !strings.Contains(err.Error(), "number 1 is used more than once") {
		t.Errorf("error = %q; want it to name the repeated number", err)
	}
}

// TestPreparePreservesSubmittedNumbers is the preview/export seam: the builder
// rule must reach a prepared quotation, not only a direct builder call.
func TestPreparePreservesSubmittedNumbers(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	err := q.Prepare(QuotationInput{
		Items: []QuotationItemInput{
			{No: 2, Description: "Line two", Qty: 1, UoM: "Lot", UnitPrice: 2000},
			{
				No:          3,
				Description: "Package line",
				Qty:         1,
				UoM:         "Lot",
				SubItems: []QuotationSubItemInput{
					{No: 1, Description: "Part one", UoM: "Pc", Qty: 1, UnitCost: 1000},
					{No: 4, Description: "Part four", UoM: "Pc", Qty: 1, UnitCost: 4000},
					{Description: "New part", UoM: "Pc", Qty: 1, UnitCost: 5000},
				},
			},
		},
		Subject:       "Stable identity",
		RecipientName: "Budi Santoso",
		Validity:      30,
		PaymentTerm:   30,
		Incoterm:      "EXW",
		TaxZone:       TaxZoneFTZ,
		TaxType:       TaxTypeGoods,
		CustomerID:    2,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(q.Data.Items) != 2 {
		t.Fatalf("prepared items = %d; want 2", len(q.Data.Items))
	}
	if q.Data.Items[0].No != 2 || q.Data.Items[1].No != 3 {
		t.Errorf("prepared item numbers = %d, %d; want 2, 3",
			q.Data.Items[0].No, q.Data.Items[1].No)
	}
	subs := q.Data.Items[1].SubItems
	if len(subs) != 3 {
		t.Fatalf("prepared sub-items = %d; want 3", len(subs))
	}
	wantNos := []uint32{1, 4, 5}
	wantLabels := []string{"3.1", "3.4", "3.5"}
	for i := range subs {
		if subs[i].No != wantNos[i] || subs[i].SubNo != wantLabels[i] {
			t.Errorf("prepared sub %d = No %d label %q; want No %d label %q",
				i, subs[i].No, subs[i].SubNo, wantNos[i], wantLabels[i])
		}
	}

	// The stored identity is the same number, not the array position: the
	// revision draft is what SaveRevision compares with the previous revision.
	draft, err := q.DraftRevision("<html></html>", nil, "")
	if err != nil {
		t.Fatalf("DraftRevision: %v", err)
	}
	if len(draft.Items) != 2 || draft.Items[0].ItemNo != 2 || draft.Items[1].ItemNo != 3 {
		t.Fatalf("draft item numbers = %v; want 2 and 3", draftItems(draft.Items))
	}
	stored := draft.Items[1].SubItems
	if len(stored) != 3 {
		t.Fatalf("draft sub-items = %d; want 3", len(stored))
	}
	for i := range stored {
		if stored[i].SubNo != int(wantNos[i]) {
			t.Errorf("draft sub %d = %d; want %d", i, stored[i].SubNo, wantNos[i])
		}
	}
}

// draftItems names the item numbers in failure output without importing the
// store package into every assertion above.
func draftItems(items []store.RevisionItem) []int {
	numbers := make([]int, 0, len(items))
	for _, item := range items {
		numbers = append(numbers, item.ItemNo)
	}
	return numbers
}
