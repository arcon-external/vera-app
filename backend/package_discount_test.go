package backend

import (
	"context"
	"strings"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// Guards for issue #383: a package whose sub-items carry discounts printed
// 0% beside a visibly reduced price, and an undiscounted row printed an
// em-dash instead of a percentage.

// TestPackageDiscountPercent pins the derivation itself: the summed discount
// over the summed list price, integer arithmetic, multiply before divide,
// truncating, with a zero list sum guarded to 0.
func TestPackageDiscountPercent(t *testing.T) {
	cases := []struct {
		name        string
		listSum     uint64
		discountSum uint64
		want        uint16
	}{
		{"exact rate", 2_000_000, 100_000, 5},
		// The reporter's document: 52.100.000 list against 46.380.000 net is
		// 5.720.000 off, which is 10.98% -- and truncating yields 10%, not
		// the 11% the issue sketched. The sketch rounded; the house rule
		// truncates, so 10% is what the cell prints.
		{"reporter figures truncate to ten", 52_100_000, 5_720_000, 10},
		// The shared fixture's state-C package: 11.000 over 370.000 is
		// 2.97%, which truncates to 2%.
		{"fraction truncates", 370_000, 11_000, 2},
		{"no discount is zero", 1_000_000, 0, 0},
		{"zero list sum cannot divide by zero", 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := packageDiscountPercent(tc.listSum, tc.discountSum); got != tc.want {
				t.Errorf("packageDiscountPercent(%d, %d) = %d; want %d",
					tc.listSum, tc.discountSum, got, tc.want)
			}
		})
	}
}

// TestPackageRollupDerivesDiscountFields pins what the rollup stores: the
// derived percentage and flag alongside the already-derived amount, with the
// money untouched -- and a stale parent input cleared, because the parent's
// own discount fields are ignored once it has sub-items.
func TestPackageRollupDerivesDiscountFields(t *testing.T) {
	items := buildItems([]QuotationItemInput{{
		Description: "discounted package",
		UoM:         "Lot",
		Qty:         1,
		// A stale parent discount: the editor zeroes these when sub-items
		// are enabled, but the API and CLI take raw input, so the rollup
		// must not let them linger.
		DiscountEnabled: true,
		DiscountPercent: 25,
		SubItems: []QuotationSubItemInput{
			{Description: "part a", UoM: "Pc", Qty: 1, UnitCost: 1_000_000,
				DiscountEnabled: true, DiscountPercent: 10},
			{Description: "part b", UoM: "Pc", Qty: 2, UnitCost: 500_000},
		},
	}, {
		Description: "plain package",
		UoM:         "Lot",
		Qty:         1,
		SubItems: []QuotationSubItemInput{
			{Description: "scope", UoM: "Pc", Qty: 1, UnitCost: 300_000},
		},
	}}, "IDR")

	discounted := items[0]
	// List 2.000.000, discount 100.000, net 1.900.000: the rate is 5%.
	if discounted.ListTotal != 2_000_000 || discounted.Total != 1_900_000 {
		t.Errorf("package money = %d/%d; want 2000000/1900000",
			discounted.ListTotal, discounted.Total)
	}
	if discounted.DiscountAmount != 100_000 {
		t.Errorf("package discount amount = %d; want 100000", discounted.DiscountAmount)
	}
	if !discounted.DiscountEnabled || discounted.DiscountPercent != 5 {
		t.Errorf("package discount = enabled %v at %d%%; want enabled at 5%%",
			discounted.DiscountEnabled, discounted.DiscountPercent)
	}
	// The stale parent input (enabled at 25%) is gone: the rate comes from
	// the parts, never from the parent's own fields.
	if discounted.DiscountPercent == 25 {
		t.Error("package kept the parent input's 25%; the rate must be derived from the parts")
	}

	plain := items[1]
	if plain.DiscountEnabled || plain.DiscountPercent != 0 || plain.DiscountAmount != 0 {
		t.Errorf("undiscounted package discount = enabled %v at %d%% amount %d; want disabled at 0%% amount 0",
			plain.DiscountEnabled, plain.DiscountPercent, plain.DiscountAmount)
	}

	// MarginPercent is deliberately not derived: it is internal-only, never
	// printed, and on a plain line it means "the rate that was applied" --
	// a package has no single applied rate. See the rollup comment.
	for i, item := range items {
		if item.MarginPercent != 0 || item.MarginEnabled {
			t.Errorf("item %d package margin = enabled %v at %d%%; want disabled at 0%%",
				i+1, item.MarginEnabled, item.MarginPercent)
		}
	}
}

// preparePackageDiscountFixture builds the reporter's state: an itemised line
// whose sub-items carry discounts with sub-prices toggled not visible, beside
// a package without discounts and a plain undiscounted line.
func preparePackageDiscountFixture(t *testing.T) *Quotation {
	t.Helper()
	t.Setenv("VERA_NO_KEYRING", "1")
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(QuotationInput{
		Subject: "package discount", RecipientName: "x", CustomerID: 2, Incoterm: "DAP",
		Validity: 30, PaymentTerm: 30, LeadTimeDays: 30,
		TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
		Items: []QuotationItemInput{
			{Description: "discounted package", UoM: "Lot", Qty: 1, ShowSubPrices: false,
				SubItems: []QuotationSubItemInput{
					{Description: "part a", UoM: "Pc", Qty: 1, UnitCost: 1_000_000,
						DiscountEnabled: true, DiscountPercent: 10},
					{Description: "part b", UoM: "Pc", Qty: 2, UnitCost: 500_000},
				}},
			{Description: "plain package", UoM: "Lot", Qty: 1, ShowSubPrices: false,
				SubItems: []QuotationSubItemInput{
					{Description: "scope", UoM: "Pc", Qty: 1, UnitCost: 300_000},
				}},
			{Description: "plain line", UoM: "Pc", Qty: 3, UnitPrice: 50_000},
		},
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return q
}

// TestPackageDiscountPrintsDerivedRate pins the cell in both renderers: the
// discounted package prints its derived 5%, and every undiscounted row prints
// 0% -- with the em-dash nowhere in any After Disc. cell.
func TestPackageDiscountPrintsDerivedRate(t *testing.T) {
	q := preparePackageDiscountFixture(t)

	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	// List 2.000.000, net 1.900.000, derived 5%, total 1.900.000.
	assertPriceRow(t, page, "2.000.000", "1.900.000", "1.900.000")
	assertPriceRow(t, page, "300.000", "300.000", "300.000")
	assertPriceRow(t, page, "50.000", "50.000", "150.000")
	if strings.Contains(page, "<div class=\"after-pct\">—</div>") {
		t.Error("em-dash second line still printed on the customer document; every row must state a percentage")
	}

	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderInput: %v", err)
	}
	if got := in.Items[0]; got.UnitPrice != "2.000.000" || got.Disc != "1.900.000" || got.Total != "1.900.000" {
		t.Errorf("package renderer input = %q/%q/%q; want 2.000.000/1.900.000/1.900.000",
			got.UnitPrice, got.Disc, got.Total)
	}
	if got := in.Items[1].Disc; got != "300.000" {
		t.Errorf("undiscounted package renderer input = %q; want 300.000", got)
	}
	if got := in.Items[2].Disc; got != "50.000" {
		t.Errorf("plain renderer input = %q; want 50.000", got)
	}
}

// TestZeroListPackageCannotDivideByZero pins the guard: a package whose parts
// carry no list price at all still prepares, with the rate at zero.
func TestZeroListPackageCannotDivideByZero(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(QuotationInput{
		Subject: "zero list", RecipientName: "x", CustomerID: 2, Incoterm: "DAP",
		Validity: 30, PaymentTerm: 30, LeadTimeDays: 30,
		TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
		Items: []QuotationItemInput{
			{Description: "free package", UoM: "Lot", Qty: 1, ShowSubPrices: false,
				SubItems: []QuotationSubItemInput{
					{Description: "pro-bono part", UoM: "Pc", Qty: 1, UnitCost: 0},
				}},
		},
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	item := q.Data.Items[0]
	if item.ListTotal != 0 || item.Total != 0 {
		t.Errorf("zero-list package money = %d/%d; want 0/0", item.ListTotal, item.Total)
	}
	if item.DiscountEnabled || item.DiscountPercent != 0 {
		t.Errorf("zero-list package discount = enabled %v at %d%%; want disabled at 0%%",
			item.DiscountEnabled, item.DiscountPercent)
	}
}

// oldPackageEvidence is a revision exactly as stored by every export before
// #383: a package with discounted subs whose own discount_percent column is 0,
// because the rollup never set it.
func oldPackageEvidence() store.RevisionEvidence {
	return store.RevisionEvidence{
		QuoteNumber:   "1502260001",
		RecipientName: "x", RecipientCompany: "y", RecipientAddress: "z",
		Subject: "s", Incoterm: "DAP",
		IssuerName: "PT Arcon Penta Persada",
		Items: []store.EvidenceItem{{
			Description: "package priced", Qty: 1, UoM: "Lot",
			UnitPrice: 359000, Total: 359000,
			DiscountPercent: 0,
			Subs: []store.EvidenceSub{
				{Description: "part a", Qty: 1, UoM: "Pc", UnitPrice: 209000, Total: 209000, DiscountPercent: 5},
				{Description: "part b", Qty: 2, UoM: "Pc", UnitPrice: 75000, Total: 150000},
			},
		}},
	}
}

// TestOldPackageRevisionStillVerifies is the proof the signature section of
// #383 demands rather than asserts: verification re-derives from the stored
// columns, never by re-rendering, so an old revision's stored 0 still matches
// its own signature after the rollup starts deriving the rate. Nothing in the
// signing path changed; this pins that an old printed code verifies on the
// new code.
func TestOldPackageRevisionStillVerifies(t *testing.T) {
	key := testPrivateKey()
	signer := NewSigner(key)

	old := oldPackageEvidence()
	mapped := SignableFromEvidence(old)
	if strings.Contains(string(CanonicalRevision(mapped)), "item_1_discount_percent") {
		t.Fatalf("an old stored 0 emitted a per-row discount field:\n%s", CanonicalRevision(mapped))
	}
	signature, _ := signer.SignRevision(mapped)

	// The stored columns are unchanged by this change, so the same mapping
	// verifies -- this is the path the portal takes for a printed code.
	if !VerifyRevision(signer.publicKey, SignableFromEvidence(old), signature) {
		t.Error("a revision stored before #383 no longer verifies; old printed codes must keep verifying")
	}

	// And the new code signs a different value for the same document -- the
	// measured consequence, not a regression: the field was previously
	// omitted, now it carries the derived rate.
	q := prepareRowPriceFixture(t)
	current := SignableFromData(q.Data)
	if !strings.Contains(string(CanonicalRevision(current)), "item_3_discount_percent: 2") {
		t.Errorf("the derived package rate is not signed:\n%s", CanonicalRevision(current))
	}
}
