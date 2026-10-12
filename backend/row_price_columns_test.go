package backend

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

// Guards for issue #380: the item table prints the list unit price under
// Unit Price IDR and the net unit price under After Disc. IDR, on every
// quotation and in every row state. The After Disc. cell is always two lines:
// the net price, then the row's discount percentage -- or an em dash when the
// row carries none, the convention the old Disc. cell used.
//
// The example running through these tests is the issue's own: cost 100.000,
// qty 2, margin 20%, row discount 10% -- list 120.000, net 108.000, total
// 216.000. Before the change the table printed the net under the Unit Price
// heading beside a bare "10%", so a reader computed 194.400 against a Total
// of 216.000.

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

// assertPriceRow checks that one row prints list, net, percentage and total:
// the Unit Price cell, then the After Disc. cell's two lines (the net price
// and, beneath it, the discount percentage or an em dash), then the Total
// cell. The three cells are adjacent in the template, so they are matched as
// one block -- matching them separately would confuse rows that share a
// figure (an undiscounted row prints its list figure three times over).
func assertPriceRow(t *testing.T, page, list, net, pct, total string) {
	t.Helper()
	block := regexp.MustCompile(
		`<td>` + regexp.QuoteMeta(list) + `</td>\s*` +
			`<td class="after">` + `\s*` +
			`<div class="after-price">` + regexp.QuoteMeta(net) + `</div>` + `\s*` +
			`<div class="after-pct">` + regexp.QuoteMeta(pct) + `</div>` + `\s*` +
			`</td>\s*` +
			`<td>` + regexp.QuoteMeta(total) + `</td>`)
	if !block.MatchString(page) {
		t.Errorf("no row prints %s / %s / %s / %s as list / net / pct / total", list, net, pct, total)
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
	assertPriceRow(t, page, "120.000", "108.000", "10%", "216.000")
	// The undiscounted line prints the same figure in both price columns, with
	// an em dash for the percentage it does not carry.
	assertPriceRow(t, page, "50.000", "50.000", "\u2014", "150.000")
	// State C: the priced subs carry both prices; the parent is a merged
	// title with none of its own.
	assertPriceRow(t, page, "220.000", "209.000", "5%", "209.000")
	assertPriceRow(t, page, "75.000", "75.000", "\u2014", "150.000")
	// State B: the package parent carries the rolled-up list and net.
	assertPriceRow(t, page, "300.000", "300.000", "\u2014", "300.000")

	// The old bare-rate Disc. column is gone: percentages print only as the
	// After Disc. cell's second line, inside after-pct divs.
	if !strings.Contains(page, "<th>After Disc. IDR</th>") {
		t.Error("After Disc. IDR header missing from the document")
	}
	if strings.Contains(page, ">Disc.<") {
		t.Error("old Disc. header still printed on the customer document")
	}
	for _, pct := range []string{">10%<", ">5%<"} {
		if n := strings.Count(page, pct); n != 1 {
			t.Errorf("percentage %q printed %d times; want once, as one row's second line", pct, n)
		}
	}
	if n := strings.Count(page, "<div class=\"after-pct\">—</div>"); n != 3 {
		t.Errorf("em-dash second line printed %d times; want 3 (the undiscounted rows)", n)
	}

	// The native renderer reads renderInput, not the template, so the mapping
	// is pinned there too: UnitPrice carries the list, Disc packs the net and
	// the percentage line after a newline (see afterDiscCell). The seam type
	// still names the packed field Disc; the packing is the contract.
	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderInput: %v", err)
	}
	if got := in.Items[0]; got.UnitPrice != "120.000" || got.Disc != "108.000\n10%" || got.Total != "216.000" {
		t.Errorf("line renderer input = %q/%q/%q; want 120.000/108.000\\n10%%/216.000",
			got.UnitPrice, got.Disc, got.Total)
	}
	if got := in.Items[1]; got.UnitPrice != "50.000" || got.Disc != "50.000\n\u2014" || got.Total != "150.000" {
		t.Errorf("plain renderer input = %q/%q/%q; want 50.000/50.000\\n\u2014/150.000",
			got.UnitPrice, got.Disc, got.Total)
	}
	if got := in.Items[2].SubItems[0]; got.UnitPrice != "220.000" || got.Disc != "209.000\n5%" || got.Total != "209.000" {
		t.Errorf("sub renderer input = %q/%q/%q; want 220.000/209.000\\n5%%/209.000",
			got.UnitPrice, got.Disc, got.Total)
	}
	if got := in.Items[3]; got.UnitPrice != "300.000" || got.Disc != "300.000\n\u2014" || got.Total != "300.000" {
		t.Errorf("package renderer input = %q/%q/%q; want 300.000/300.000\\n\u2014/300.000",
			got.UnitPrice, got.Disc, got.Total)
	}
}

// TestAfterDiscSecondLineAgreesAcrossRenderers pins the one formatting the two
// renderers must share: the template's after-pct div and the seam's packed
// second line print the same string for every priced row. The template formats
// inline ({{if .DiscountEnabled}}...), the seam packs discountPctLine; this
// compares them row by row, so either side drifting fails here rather than in
// a customer's hands.
func TestAfterDiscSecondLineAgreesAcrossRenderers(t *testing.T) {
	q := prepareRowPriceFixture(t)
	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderInput: %v", err)
	}
	// Every after-pct div in document order.
	pctRe := regexp.MustCompile(`<div class="after-pct">(.*?)</div>`)
	divs := pctRe.FindAllStringSubmatch(page, -1)
	// Every priced row's packed second line in document order: the state-C
	// parent is a merged title with no after cell, and state-B scope rows are
	// merged too, so only priced rows contribute.
	secondLine := func(packed string) string {
		t.Helper()
		parts := strings.SplitN(packed, "\n", 2)
		if len(parts) != 2 {
			t.Fatalf("packed after cell %q has no second line", packed)
		}
		return parts[1]
	}
	var packed []string
	for _, it := range in.Items {
		if len(it.SubItems) > 0 && it.ShowSubPrices {
			// State C: the priced parts carry the cells.
			for _, sub := range it.SubItems {
				packed = append(packed, secondLine(sub.Disc))
			}
			continue
		}
		packed = append(packed, secondLine(it.Disc))
	}
	if len(divs) != len(packed) {
		t.Fatalf("template prints %d second lines, seam packs %d", len(divs), len(packed))
	}
	for i := range divs {
		if divs[i][1] != packed[i] {
			t.Errorf("row %d: template second line %q, seam second line %q", i+1, divs[i][1], packed[i])
		}
	}
}

// cutLast splits s at its last newline, reporting the head, the tail and
// whether a newline was present.
func cutLast(s string) (head, tail string, ok bool) {
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
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
	assertPriceRow(t, page, "1.000.000", "1.000.000", "—", "1.000.000")

	if _, err := q.build(); err != nil {
		t.Fatalf("the engine could not render a discount-free quotation: %v", err)
	}
}

// TestRowPriceColumnsLeaveCanonicalUnchanged is the compatibility guarantee
// for this rendering change: it prints different cells, but the signed
// payload carries the struct values, not the printed strings, so the
// canonical bytes for a fixed input must be byte-identical before and after.
// The golden below was captured before the change; if it ever differs, printed
// verification codes stop verifying -- stop and investigate, do not update it.
func TestRowPriceColumnsLeaveCanonicalUnchanged(t *testing.T) {
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
		t.Errorf("canonical bytes changed by a rendering-only change:\n got %q\nwant %q", got, golden)
	}
}
