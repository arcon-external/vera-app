package backend

import (
	"context"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

// The unit-of-measure list (issue #337) changes three behaviours at once:
// what Prepare accepts, what a package parent must carry (the canonical unit
// instead of a hardcoded "Lot"), and what the customer document prints (the
// company label instead of the stored value). These tests pin each seam.

// packageInput builds one package line (a parent with priced sub-items) plus
// one plain line, so each test exercises both the canonical-unit gate and the
// list gate on the same document.
func unitsInput() QuotationInput {
	return QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{
			{
				Description: "Package:",
				UoM:         "Lot",
				UnitPrice:   1_000_000,
				Qty:         1,
				SubItems: []QuotationSubItemInput{
					{Description: "Part one", UoM: "m2", UnitCost: 500_000, Qty: 2},
					{Description: "Part two", UoM: "EA", UnitCost: 100_000, Qty: 1},
				},
			},
			{Description: "Plain line", UoM: "m2", UnitPrice: 400_000, Qty: 2},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      30,
		PaymentTerm:   30,
		LeadTimeDays:  30,
		Incoterm:      "FOB",
	}
}

// TestPrepareRefusesUnitNotOnCompanyList pins both list gates: a plain line's
// unit and a sub-item's unit that are neither company units nor legacy
// spellings are refused, and each message names the row and the value.
func TestPrepareRefusesUnitNotOnCompanyList(t *testing.T) {
	// A plain line carrying a plausible but unlisted unit.
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items:      []QuotationItemInput{{Description: "Item", Qty: 1, UoM: "Box", UnitPrice: 1000}},
		Subject:    "Test", RecipientName: "Test",
		Validity: 30, PaymentTerm: 30, LeadTimeDays: 30, Incoterm: "FOB",
	})
	if err == nil {
		t.Fatal("Prepare accepted the unit Box, which is on neither the company list nor the legacy set")
	}
	for _, want := range []string{"item 1", `unit of measure "Box" is not on the company's unit list`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}

	// A priced sub-item carrying the same unlisted unit.
	q = newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	err = q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{{
			Description: "Package:", UoM: "Lot", UnitPrice: 1_000_000, Qty: 1,
			SubItems: []QuotationSubItemInput{
				{Description: "Part", UoM: "Box", UnitCost: 500_000, Qty: 1},
			},
		}},
		Subject: "Test", RecipientName: "Test",
		Validity: 30, PaymentTerm: 30, LeadTimeDays: 30, Incoterm: "FOB",
	})
	if err == nil {
		t.Fatal("Prepare accepted a sub-item unit Box, which is on neither list")
	}
	for _, want := range []string{"item 1 sub-item 1", `unit of measure "Box" is not on the company's unit list`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// TestPrepareAcceptsUnitOnCompanyList is the positive half: the units a
// quotation legitimately carries — seeds, legacy spellings, and folded
// case-variants — all pass the same gate.
func TestPrepareAcceptsUnitOnCompanyList(t *testing.T) {
	for _, uom := range []string{"set", "m2", "Year", "Pc", "meter", "LOT"} {
		t.Run(uom, func(t *testing.T) {
			q := newTestQuotation()
			q.SetAuthService(auth.StaffTestService())
			err := q.Prepare(QuotationInput{
				CustomerID: 2,
				Items:      []QuotationItemInput{{Description: "Item", Qty: 1, UoM: uom, UnitPrice: 1000}},
				Subject:    "Test", RecipientName: "Test",
				Validity: 30, PaymentTerm: 30, LeadTimeDays: 30, Incoterm: "FOB",
			})
			if err != nil {
				t.Errorf("Prepare rejected the valid UoM %q: %v", uom, err)
			}
		})
	}
}

// quotationWithCanonical builds a test quotation whose canonical unit is not
// the default "Lot".
func quotationWithCanonical(canonical string) *Quotation {
	q := newTestQuotation()
	identity := DefaultCompanyIdentity()
	identity.CanonicalUoM = canonical
	q.companyIdentity = &identity
	return q
}

// TestCanonicalUoMReplacesLot pins that the package-parent gate reads the
// company's canonical setting rather than the hardcoded "Lot": with the
// canonical set to "Pc", a package parent in "Pc" — "pc" from a paste
// included — passes and is normalized to the stored spelling, and "Lot" is
// refused with a message naming the canonical.
func TestCanonicalUoMReplacesLot(t *testing.T) {
	input := unitsInput()
	input.Items[1] = QuotationItemInput{Description: "Plain line", UoM: "EA", UnitPrice: 400_000, Qty: 2}
	input.Items[0].UoM = "pc" // paste case-variant of the canonical

	q := quotationWithCanonical("Pc")
	q.SetAuthService(auth.StaffTestService())
	if err := q.Prepare(input); err != nil {
		t.Fatalf("Prepare with canonical Pc: %v", err)
	}
	if got := q.Data.Items[0].UoM; got != "Pc" {
		t.Errorf("package parent UoM = %q; want it canonicalized to the stored \"Pc\"", got)
	}

	q = quotationWithCanonical("Pc")
	q.SetAuthService(auth.StaffTestService())
	wrong := unitsInput()
	wrong.Items[0].UoM = "Lot"
	err := q.Prepare(wrong)
	if err == nil {
		t.Fatal("a package parent in Lot was accepted with the canonical set to Pc")
	}
	if !strings.Contains(err.Error(), "must use the unit Pc") {
		t.Errorf("error %q does not name the canonical unit", err.Error())
	}
}

// TestUnitLabelResolutionPrintsCompanyLabels pins the value → label seam: a
// unit that stores as "m2" prints as "m²" on every customer-facing surface —
// the preview HTML and the renderer's input (whose executed body both
// renderers draw from) — while q.Data keeps the raw value that signs and
// stores.
func TestUnitLabelResolutionPrintsCompanyLabels(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(unitsInput()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// The store path would resolve this; a unit test injects the same label
	// map after Prepare to exercise resolution without a database.
	q.units["m2"] = "m²"

	display := q.displayData()
	if got := display.Items[0].SubItems[0].UoM; got != "m²" {
		t.Errorf("display sub UoM = %q; want the company label \"m²\"", got)
	}
	if got := display.Items[1].UoM; got != "m²" {
		t.Errorf("display line UoM = %q; want the company label \"m²\"", got)
	}

	// The raw values must have survived the copy untouched.
	if got := q.Data.Items[0].SubItems[0].UoM; got != "m2" {
		t.Errorf("q.Data sub UoM mutated to %q; want the raw \"m2\"", got)
	}
	if got := q.Data.Items[1].UoM; got != "m2" {
		t.Errorf("q.Data line UoM mutated to %q; want the raw \"m2\"", got)
	}

	// The renderer's input resolves too, and the executed template body — the
	// shared seam both PDF engines print — carries the label.
	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderInput: %v", err)
	}
	if got := in.Items[0].SubItems[0].UoM; got != "m²" {
		t.Errorf("renderer sub UoM = %q; want \"m²\"", got)
	}
	if !strings.Contains(in.HTML, "m²") {
		t.Error("the executed template body does not contain the resolved label m²")
	}

	preview, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("GetQuotationPreviewHTML: %v", err)
	}
	if !strings.Contains(preview, "m²") {
		t.Error("the preview HTML does not contain the resolved label m²")
	}

	// A value that is not on the list — a legacy spelling or a unit removed
	// after the quotation was prepared — prints itself, never a blank or a
	// guess.
	if got := q.uomLabel("Unit"); got != "Unit" {
		t.Errorf("uomLabel(Unit) = %q; want the raw fallback \"Unit\"", got)
	}
}

// TestUnitLabelsNeverEnterCanonicalBytes pins the byte-compatibility promise:
// the canonical payload is computed from q.Data's raw values, so changing the
// label map after Prepare — which only affects what prints — leaves the signed
// bytes identical, and the signable carries the stored spelling.
func TestUnitLabelsNeverEnterCanonicalBytes(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(unitsInput()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	raw := string(CanonicalRevision(SignableFromData(q.Data)))
	q.units["m2"] = "m²"
	relabelled := string(CanonicalRevision(SignableFromData(q.Data)))
	if raw != relabelled {
		t.Errorf("a label-only change altered the canonical bytes:\n--- before ---\n%s\n--- after ---\n%s",
			raw, relabelled)
	}

	signable := SignableFromData(q.Data)
	if len(signable.Items) < 2 {
		t.Fatalf("signable has %d items; want 2", len(signable.Items))
	}
	if signable.Items[1].UoM != "m2" {
		t.Errorf("signed line UoM = %q; want the raw stored value \"m2\"", signable.Items[1].UoM)
	}
	if len(signable.Items[0].Subs) < 1 || signable.Items[0].Subs[0].UoM != "m2" {
		t.Errorf("signed sub UoM = %+v; want the raw stored value \"m2\"", signable.Items[0].Subs)
	}
}

// TestRevisionDraftKeepsRawUnits pins that the stored revision snapshots the
// value, not the label: the label is presentation, and history holds what was
// validated and signed.
func TestRevisionDraftKeepsRawUnits(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(unitsInput()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	q.units["m2"] = "m²"
	q.Data.QuoteNumber = "15020001"

	draft := q.revisionDraft("<html>snapshot</html>", nil, "")
	if len(draft.Items) < 2 {
		t.Fatalf("draft has %d items; want 2", len(draft.Items))
	}
	if draft.Items[1].UoM != "m2" {
		t.Errorf("draft line UoM = %q; want the raw stored value \"m2\"", draft.Items[1].UoM)
	}
	if len(draft.Items[0].SubItems) < 1 || draft.Items[0].SubItems[0].UoM != "m2" {
		t.Errorf("draft sub UoM = %+v; want the raw stored value \"m2\"", draft.Items[0].SubItems)
	}
}

// TestDisplayDataNeverMutatesRawData is the guard on the deep copy itself: the
// item slice is copied, and so is every sub-item slice, so resolving labels on
// one call can never corrupt the values the signable and the stored revision
// read.
func TestDisplayDataNeverMutatesRawData(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(unitsInput()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	q.units["m2"] = "m²"

	for i := 0; i < 2; i++ {
		_ = q.displayData()
		if got := q.Data.Items[0].SubItems[0].UoM; got != "m2" {
			t.Fatalf("call %d: q.Data sub UoM mutated to %q; want \"m2\"", i+1, got)
		}
		if got := q.Data.Items[1].UoM; got != "m2" {
			t.Fatalf("call %d: q.Data line UoM mutated to %q; want \"m2\"", i+1, got)
		}
	}
}
