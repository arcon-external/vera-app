package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vera-app/backend/poreport"
)

// Issue #648: #655 added the extra line kind. A client that predates it sends
// no line kind, which the store reads as the compatibility value quotation, so
// a legacy save over a record that carries an extra line would silently move
// the pass-through amount out of Extra Expense and into Uncosted. The same 409
// that protects an aggregate line must protect the extra line, and an explicit
// kind -- extra or quotation -- stays the newer client's decision.

// assertLegacyExtraBucket proves the pass-through amount is still reported as
// extra expense rather than unknown cost at the moment the caller names.
func assertLegacyExtraBucket(t *testing.T, database *Store, ctx context.Context, step string) {
	t.Helper()
	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.ExtraExpenseMinor != 1_000_000 || report.Totals.UncostedMinor != 0 {
		t.Fatalf("%s: extra/uncosted = %d/%d; want 1.000.000/0 (a pass-through amount, never unknown cost)",
			step, report.Totals.ExtraExpenseMinor, report.Totals.UncostedMinor)
	}
}

// TestPurchaseOrderCorrectionRefusesALegacyKindOverAnExtraLine is the red proof:
// a saved record with an extra line, corrected by a client that sends no line
// kind, is refused with the typed outdated-client error, names the line, and
// leaves the record and the report exactly as they were. The same payload with
// the explicit kind saves, so the guard refuses the absent kind rather than the
// extra line.
func TestPurchaseOrderCorrectionRefusesALegacyKindOverAnExtraLine(t *testing.T) {
	database := openTestSchema(t, "vera_po_648_legacy_extra")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Legacy Extra", "Legacy extra source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000,
		}})
	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-648-LEGACY", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})
	if kinds := readStoredLineKinds(t, database, saved.ID); kinds[2] != PurchaseOrderLineKindExtra {
		t.Fatalf("fixture line 2 kind = %q; want extra", kinds[2])
	}
	assertLegacyExtraBucket(t, database, ctx, "the saved fixture")

	legacy := correctionFromSavedOrder(saved, user, "a client that predates the line kind")
	for index := range legacy.Items {
		legacy.Items[index].LineKind = ""
	}
	_, err := database.CorrectPurchaseOrder(ctx, legacy)
	if !errors.Is(err, ErrPurchaseOrderClientOutdated) {
		t.Fatalf("legacy correction = %v; want ErrPurchaseOrderClientOutdated", err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error %q does not name the extra line", err.Error())
	}

	unchanged, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reopen after the refused correction: %v", err)
	}
	if unchanged.Version != saved.Version {
		t.Errorf("version after the refused correction = %d; want %d",
			unchanged.Version, saved.Version)
	}
	if kinds := readStoredLineKinds(t, database, saved.ID); kinds[2] != PurchaseOrderLineKindExtra {
		t.Errorf("stored kind of line 2 = %q; want extra", kinds[2])
	}
	assertLegacyExtraBucket(t, database, ctx, "after the refused legacy correction")

	// The same lines with the explicit kind save normally.
	explicit := correctionFromSavedOrder(unchanged, user, "the updated client keeps the extra")
	corrected, err := database.CorrectPurchaseOrder(ctx, explicit)
	if err != nil {
		t.Fatalf("explicit extra correction = %v; want it saved", err)
	}
	if kinds := readStoredLineKinds(t, database, corrected.ID); kinds[2] != PurchaseOrderLineKindExtra {
		t.Errorf("stored kind after the explicit correction = %q; want extra", kinds[2])
	}
	assertLegacyExtraBucket(t, database, ctx, "after the explicit extra correction")

	// An explicit quotation on a stored extra is the newer client deliberately
	// reclassifying the line, so it is allowed: only an absent kind is refused.
	reclassified := correctionFromSavedOrder(corrected, user, "the newer client reclassifies the extra")
	reclassified.Items[1].LineKind = PurchaseOrderLineKindQuotation
	moved, err := database.CorrectPurchaseOrder(ctx, reclassified)
	if err != nil {
		t.Fatalf("explicit reclassification = %v; want it allowed", err)
	}
	if kinds := readStoredLineKinds(t, database, moved.ID); kinds[2] != PurchaseOrderLineKindQuotation {
		t.Errorf("stored kind after the explicit reclassification = %q; want quotation", kinds[2])
	}
}

// TestPurchaseOrderDraftSaveRefusesALegacyKindOverAnExtraLine proves the shared
// line writer is the seam every save path crosses: the draft save is refused
// too, with the ids present and with them absent (the older editor may send
// neither), while the same lines with their kind keep saving.
func TestPurchaseOrderDraftSaveRefusesALegacyKindOverAnExtraLine(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_648_legacy_extra_draft")

	items := append(append([]PurchaseOrderItem{}, fixture.order.Items...), extraLine())
	withExtra, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, items, nil))
	if err != nil {
		t.Fatalf("save the draft with an extra line: %v", err)
	}
	if kinds := readStoredLineKinds(t, fixture.database, withExtra.ID); kinds[4] != PurchaseOrderLineKindExtra {
		t.Fatalf("stored kind of the extra line = %q; want extra", kinds[4])
	}

	legacy := append([]PurchaseOrderItem{}, withExtra.Items...)
	for index := range legacy {
		legacy[index].LineKind = ""
	}
	_, err = fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(withExtra, fixture.user, legacy, nil))
	if !errors.Is(err, ErrPurchaseOrderClientOutdated) {
		t.Fatalf("legacy draft save = %v; want ErrPurchaseOrderClientOutdated", err)
	}
	if !strings.Contains(err.Error(), "line 4") {
		t.Errorf("error %q does not name the extra line", err.Error())
	}
	unchanged, err := fixture.database.GetPurchaseOrder(fixture.ctx, withExtra.ID)
	if err != nil {
		t.Fatalf("reopen after the refused draft save: %v", err)
	}
	if unchanged.Version != withExtra.Version {
		t.Errorf("version after the refused draft save = %d; want %d",
			unchanged.Version, withExtra.Version)
	}
	if kinds := readStoredLineKinds(t, fixture.database, withExtra.ID); kinds[4] != PurchaseOrderLineKindExtra {
		t.Errorf("stored kind of the extra line after the refusal = %q; want extra", kinds[4])
	}

	// An older editor may also drop every id. The line identity is then the
	// entered values, so the same guard must still see the stored extra.
	anonymous := append([]PurchaseOrderItem{}, withExtra.Items...)
	for index := range anonymous {
		anonymous[index].LineKind = ""
		anonymous[index].ID = 0
		anonymous[index].ClientKey = ""
	}
	_, err = fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(withExtra, fixture.user, anonymous, nil))
	if !errors.Is(err, ErrPurchaseOrderClientOutdated) {
		t.Fatalf("identity-less legacy draft save = %v; want ErrPurchaseOrderClientOutdated", err)
	}

	// The same payload with the explicit kind saves and keeps the extra.
	explicit := append([]PurchaseOrderItem{}, withExtra.Items...)
	current, err := fixture.database.GetPurchaseOrder(fixture.ctx, withExtra.ID)
	if err != nil {
		t.Fatalf("reopen before the explicit save: %v", err)
	}
	saved, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(current, fixture.user, explicit, nil))
	if err != nil {
		t.Fatalf("explicit draft save = %v; want it saved", err)
	}
	if kinds := readStoredLineKinds(t, fixture.database, saved.ID); kinds[4] != PurchaseOrderLineKindExtra {
		t.Errorf("stored kind after the explicit save = %q; want extra", kinds[4])
	}
}
