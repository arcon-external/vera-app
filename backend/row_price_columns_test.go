package backend

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/render/quotation"
)

// Guards for issues #380 and #398: the item table prints the list unit price
// under Unit Price IDR and the net unit price under After Disc. IDR, on every
// quotation and in every row state. The After Disc. cell is one line since
// #398 -- the net price alone; the discount percentage no longer prints.
//
// The example running through these tests is #380's own: cost 100.000, qty 2,
// margin 20%, row discount 10% -- list 120.000, net 108.000, total 216.000.
// Before #380 the table printed the net under the Unit Price heading beside a
// bare "10%", so a reader computed 194.400 against a Total of 216.000.

// prepareRowPriceFixture builds the mixed document every guard here renders:
// a discounted plain line, an undiscounted plain line, a package with priced
// subs (one discounted), and a package with unpriced scope rows.
func prepareRowPriceFixture(t *testing.T) *Quotation {
	t.Helper()
	t.Setenv("VERA_NO_KEYRING", "1")
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(QuotationInput{
		Subject: "row prices", RecipientName: "x", CustomerID: 2, Incoterm: "DAP",
		Validity: 30, PaymentTerm: 30, LeadTime: 30,
		TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
		Items: []QuotationItemInput{
			{Description: "Widget", UoM: "Lot", Qty: 2, UnitPrice: 100000,
				MarginEnabled: true, MarginPercent: 20,
				DiscountEnabled: true, DiscountPercent: 10},
			{Description: "plain line", UoM: "Pc", Qty: 3, UnitPrice: 50000},
			{Description: "package priced", UoM: "Lot", Qty: 1, ShowSubPrices: true,
				SubItems: []QuotationSubItemInput{
					{Description: "part a", UoM: "Pc", Qty: 1, UnitCost: 200000,
						MarginEnabled: true, MarginPercent: 10,
						DiscountEnabled: true, DiscountPercent: 5},
					{Description: "part b", UoM: "Pc", Qty: 2, UnitCost: 75000},
				}},
			{Description: "package scope", UoM: "Lot", Qty: 1, ShowSubPrices: false,
				SubItems: []QuotationSubItemInput{
					{Description: "scope a", UoM: "Pc", Qty: 1, UnitCost: 300000},
				}},
		},
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return q
}

// assertPriceRow checks that one row prints list, net and total: the Unit
// Price cell, then the After Disc. cell (the net price, one line), then the
// Total cell. The three cells are adjacent in the template, so they are
// matched as one block -- matching them separately would confuse rows that
// share a figure (an undiscounted row prints its list figure three times
// over). Each price cell carries its font size from the shared rule
// (PriceSizePx, issue #398), so the block pattern accepts any size.
func assertPriceRow(t *testing.T, page, list, net, total string) {
	t.Helper()
	block := regexp.MustCompile(
		`<td style="font-size: [0-9.]+px">` + regexp.QuoteMeta(list) + `</td>\s*` +
			`<td class="after" style="font-size: [0-9.]+px">` + regexp.QuoteMeta(net) + `</td>\s*` +
			`<td style="font-size: [0-9.]+px">` + regexp.QuoteMeta(total) + `</td>`)
	if !block.MatchString(page) {
		t.Errorf("no row prints %s / %s / %s as list / net / total", list, net, total)
	}
}

// TestItemTablePrintsListAndNetPrices pins what the two price columns print:
// the marginalised list figure under Unit Price IDR, the net under
// After Disc. IDR, and net times quantity under Total IDR -- in all three row
// states, and on the renderer's input as well as in the template's cells.
func TestItemTablePrintsListAndNetPrices(t *testing.T) {
	q := prepareRowPriceFixture(t)

	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	// The discounted plain line: the issue's example, end to end.
	assertPriceRow(t, page, "120.000", "108.000", "216.000")
	// The undiscounted line prints the same figure in both price columns.
	assertPriceRow(t, page, "50.000", "50.000", "150.000")
	// State C: the priced subs carry both prices; the parent is a merged
	// title with none of its own.
	assertPriceRow(t, page, "220.000", "209.000", "209.000")
	assertPriceRow(t, page, "75.000", "75.000", "150.000")
	// State B: the package parent carries the rolled-up list and net.
	assertPriceRow(t, page, "300.000", "300.000", "300.000")

	// The discount percentage no longer prints anywhere in the item table
	// (issue #398): no after-pct div, no bare percentage cell.
	if !strings.Contains(page, "<th>After Disc. IDR</th>") {
		t.Error("After Disc. IDR header missing from the document")
	}
	if strings.Contains(page, ">Disc.<") {
		t.Error("old Disc. header still printed on the customer document")
	}
	if strings.Contains(page, "after-pct") {
		t.Error("the removed discount-percentage line still renders (after-pct present)")
	}
	for _, pct := range []string{">10%<", ">5%<", ">0%<"} {
		if strings.Contains(page, pct) {
			t.Errorf("discount percentage %q still printed in the item table", pct)
		}
	}

	// The native renderer reads renderInput, not the template, so the mapping
	// is pinned there too: UnitPrice carries the list, Disc carries the net.
	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderInput: %v", err)
	}
	if got := in.Items[0]; got.UnitPrice != "120.000" || got.Disc != "108.000" || got.Total != "216.000" {
		t.Errorf("line renderer input = %q/%q/%q; want 120.000/108.000/216.000",
			got.UnitPrice, got.Disc, got.Total)
	}
	if got := in.Items[1]; got.UnitPrice != "50.000" || got.Disc != "50.000" || got.Total != "150.000" {
		t.Errorf("plain renderer input = %q/%q/%q; want 50.000/50.000/150.000",
			got.UnitPrice, got.Disc, got.Total)
	}
	if got := in.Items[2].SubItems[0]; got.UnitPrice != "220.000" || got.Disc != "209.000" || got.Total != "209.000" {
		t.Errorf("sub renderer input = %q/%q/%q; want 220.000/209.000/209.000",
			got.UnitPrice, got.Disc, got.Total)
	}
	// The state-C parent is a merged title with no after cell of its own, but
	// its fields still carry the rollup.
	if got := in.Items[2]; got.UnitPrice != "370.000" || got.Disc != "359.000" || got.Total != "359.000" {
		t.Errorf("package renderer input = %q/%q/%q; want 370.000/359.000/359.000",
			got.UnitPrice, got.Disc, got.Total)
	}
	if got := in.Items[3]; got.UnitPrice != "300.000" || got.Disc != "300.000" || got.Total != "300.000" {
		t.Errorf("package renderer input = %q/%q/%q; want 300.000/300.000/300.000",
			got.UnitPrice, got.Disc, got.Total)
	}
}

// TestPriceCellsCarryTheirSharedSize pins the template half of the shared
// price-size rule (issue #398): every price cell carries an inline font-size
// equal to quotation.PriceSizePx of its own text, in all three row states.
// The rule itself is pinned in backend/render/gpdf; this pins that the
// template actually consults it rather than leaving the size to the body.
func TestPriceCellsCarryTheirSharedSize(t *testing.T) {
	q := prepareRowPriceFixture(t)
	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderInput: %v", err)
	}
	// Every price figure the seam carries must appear with its computed size.
	// The state-C parent (370.000/359.000) is deliberately absent: it is a
	// merged title row with no price cells of its own.
	sizes := []string{"11", "10", "9", "8"}
	for _, text := range []string{"120.000", "108.000", "216.000", "50.000",
		"220.000", "209.000", "75.000", "300.000"} {
		size := strconv.FormatFloat(quotation.PriceSizePx(text), 'f', -1, 64)
		if !containsAny(sizes, size) {
			t.Fatalf("test table does not cover size %s", size)
		}
		want := `style="font-size: ` + size + `px">` + text + `</td>`
		if !strings.Contains(page, want) {
			t.Errorf("no price cell prints %q at its shared size %s", text, size)
		}
	}
	_ = in
}

// containsAny reports whether the slice holds the value.
func containsAny(all []string, v string) bool {
	for _, s := range all {
		if s == v {
			return true
		}
	}
	return false
}

// TestItemTableAlwaysHasAfterDiscColumn pins the column's unconditional
// presence: a quotation with no row discount anywhere still prints both price
// columns, with the list figure repeated in the net column.
func TestItemTableAlwaysHasAfterDiscColumn(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(QuotationInput{
		Subject: "no discounts", RecipientName: "x", CustomerID: 2, Incoterm: "DAP",
		Validity: 30, PaymentTerm: 30, LeadTime: 30,
		TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
		Items: []QuotationItemInput{
			{Description: "plain line", UoM: "Lot", Qty: 1, UnitPrice: 1000000},
		},
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(page, "<th>After Disc. IDR</th>") {
		t.Error("a quotation with no discounts prints no After Disc. IDR column")
	}
	assertPriceRow(t, page, "1.000.000", "1.000.000", "1.000.000")

	if _, err := q.build(); err != nil {
		t.Fatalf("the engine could not render a discount-free quotation: %v", err)
	}
}

// TestRowPriceColumnsChangeCanonicalOnlyByTheDerivedPackageRate is the
// compatibility record for the #383 data change. The cell work itself prints
// different strings but signs the struct values, not the printed strings --
// yet deriving the package's discount percentage is a data change, not a
// rendering-only one: a package with sub-discounts now signs its effective
// rate (item_3_discount_percent below) where the field was previously omitted.
//
// That is the entire delta for this fixed input: one added line. If the bytes
// ever differ anywhere else, printed verification codes stop verifying --
// stop and investigate, do not update the golden. CanonicalPrefix is untouched
// by this change, and old revisions (whose stored package percent is 0, so the
// field stays omitted for them) keep verifying: TestOldPackageRevisionStillVerifies
// proves it rather than asserting it.
func TestRowPriceColumnsChangeCanonicalOnlyByTheDerivedPackageRate(t *testing.T) {
	q := prepareRowPriceFixture(t)

	const golden = "VERA1:QUOTE\n" +
		"number: \n" +
		"revision: 00\n" +
		"recipient_name: x\n" +
		"recipient_company: PT. Infineon Technologies Batam\n" +
		"recipient_address: Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia\n" +
		"subject: row prices\n" +
		"incoterm: DAP\n" +
		"validity_days: 30\n" +
		"payment_term_days: 30\n" +
		"lead_time_days: 30\n" +
		"discount_percent: 0\n" +
		"issuer: PT Arcon Penta Persada\n" +
		"currency: IDR\n" +
		"item_count: 4\n" +
		"item_1: Widget|2|Lot|108000|216000\n" +
		"item_1_discount_percent: 10\n" +
		"item_2: plain line|3|Pc|50000|150000\n" +
		"item_3: package priced|1|Lot|359000|359000\n" +
		"item_3_discount_percent: 2\n" +
		"item_3_subs: 2\n" +
		"item_3_sub_1: part a|1|Pc|209000|209000\n" +
		"item_3_sub_1_discount_percent: 5\n" +
		"item_3_sub_2: part b|2|Pc|75000|150000\n" +
		"item_4: package scope|1|Lot|300000|300000\n" +
		"item_4_subs: 1\n" +
		"item_4_sub_1: scope a|1|Pc|300000|300000\n" +
		"subtotal: 1025000\n" +
		"discount_amount: 0\n" +
		"ppn_amount: 0\n" +
		"grand_total: 1025000"

	if got := string(CanonicalRevision(SignableFromData(q.Data))); got != golden {
		t.Errorf("canonical bytes changed beyond the derived package rate:\n got %q\nwant %q", got, golden)
	}
}
