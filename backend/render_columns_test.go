package backend

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"vera-app/backend/render"

	"vera-app/backend/auth"
	gpdfrender "vera-app/backend/render/gpdf"
)

// itemHeadCellRe pulls the header cells out of the item table, and only that
// table: the document has other <th>s nowhere, but scoping it to the head table
// keeps the test honest if it ever grows one.
var (
	itemHeadTableRe = regexp.MustCompile(`(?s)<table class="items-head-table">(.*?)</table>`)
	headCellRe      = regexp.MustCompile(`<th>(.*?)</th>`)
	colWidthRe      = regexp.MustCompile(`<col style="width: ([\d.]+)%;">`)
)

func templateHeadLabels(t *testing.T, html string) []string {
	t.Helper()
	table := itemHeadTableRe.FindStringSubmatch(html)
	if table == nil {
		t.Fatal("the preview HTML has no items-head-table")
	}
	var out []string
	for _, m := range headCellRe.FindAllStringSubmatch(table[1], -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// templateColumnPercents reads the head table's colgroup widths, in order.
func templateColumnPercents(t *testing.T, html string) []float64 {
	t.Helper()
	table := itemHeadTableRe.FindStringSubmatch(html)
	if table == nil {
		t.Fatal("the preview HTML has no items-head-table")
	}
	var out []float64
	for _, m := range colWidthRe.FindAllStringSubmatch(table[1], -1) {
		pct, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatalf("bad colgroup width %q: %v", m[1], err)
		}
		out = append(out, pct)
	}
	return out
}

// wantItemColumns is the item table's single column set: the list Unit Price
// and the After Disc. net price print on every quotation, whether or not any
// row carries a discount.
var wantItemColumns = []string{"No.", "Description", "Qty.", "UoM", "Unit Price IDR", "After Disc. IDR", "Total IDR"}

// TestRenderersAgreeOnItemTableColumns is the check whose absence let a release
// ship a PDF that disagreed with its own preview.
//
// The template once added a Disc. column only when some row carried a per-row
// discount, and the native engine used to have a fixed six-column model with no
// idea the column existed. Nothing compared the two: the equivalence tests
// compare pagination, and they skip without a Chrome runtime anyway.
//
// This needs no browser. It renders the real template and asks the engine what
// it would print, which is exactly the pair that drifted.
func TestRenderersAgreeOnItemTableColumns(t *testing.T) {
	// Renders through whichever engine the build selected.
	render.RequireUsable(t)
	templates := &Quotation{}
	if err := Startup(templates, context.Background()); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	cases := []struct {
		name  string
		items []QuotationItemInput
	}{
		{
			name: "no row carries a discount",
			items: []QuotationItemInput{
				{Description: "plain line", UoM: "Lot", Qty: 1, UnitPrice: 1000000},
			},
		},
		{
			name: "a line carries a discount",
			items: []QuotationItemInput{
				{Description: "discounted line", UoM: "Lot", Qty: 1, UnitPrice: 1000000,
					DiscountEnabled: true, DiscountPercent: 10},
			},
		},
		{
			name: "only a sub-item carries a discount",
			items: []QuotationItemInput{
				{Description: "split line", UoM: "Lot", Qty: 1, ShowSubPrices: true,
					SubItems: []QuotationSubItemInput{
						{Description: "part", UoM: "Pc", Qty: 1, UnitCost: 500000,
							DiscountEnabled: true, DiscountPercent: 15},
					}},
			},
		},
		{
			name: "a package with unpriced scope rows",
			items: []QuotationItemInput{
				{Description: "scope line", UoM: "Lot", Qty: 1, ShowSubPrices: false,
					SubItems: []QuotationSubItemInput{
						{Description: "scope", UoM: "Pc", Qty: 1, UnitCost: 500000},
					}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := newTestQuotation()
			q.SetAuthService(auth.StaffTestService())
			q.Data.HtmlTemplate = templates.Data.HtmlTemplate
			q.Data.CustomCSS = templates.Data.CustomCSS
			q.Data.LogoBase64 = templates.Data.LogoBase64

			if err := q.Prepare(QuotationInput{
				Subject: "probe", RecipientName: "x", CustomerID: 2, Incoterm: "DAP",
				Validity: 30, PaymentTerm: 30, LeadTime: 30,
				TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
				Items: tc.items,
			}); err != nil {
				t.Fatalf("Prepare: %v", err)
			}

			html, err := q.GetQuotationPreviewHTML()
			if err != nil {
				t.Fatalf("preview: %v", err)
			}
			fromTemplate := templateHeadLabels(t, html)
			fromEngine := gpdfrender.HeadLabels()

			if len(fromTemplate) != len(wantItemColumns) {
				t.Fatalf("template printed %d columns %v; want %d %v",
					len(fromTemplate), fromTemplate, len(wantItemColumns), wantItemColumns)
			}
			for i, want := range wantItemColumns {
				if fromTemplate[i] != want {
					t.Errorf("template column %d: got %q, want %q", i+1, fromTemplate[i], want)
				}
				if fromEngine[i] != want {
					t.Errorf("engine column %d: got %q, want %q", i+1, fromEngine[i], want)
				}
			}

			// The geometries have to agree too, not just the labels: the
			// template lays the table out from its colgroup percentages and
			// the engine from its pixel grid, and a change to one side alone
			// moves every border on that side. The table is 646px wide on
			// both (see TestColumnEdgesOnWholePixels); the browser hands the
			// odd pixel to a different track than truncation does, so the
			// comparison allows a pixel and a half rather than demanding the
			// browser's own distribution, which only a browser can read.
			const tablePx = 646.0
			pcts := templateColumnPercents(t, html)
			edges := gpdfrender.ColumnEdgesPx()
			if len(pcts) != len(wantItemColumns) {
				t.Fatalf("template colgroup has %d widths %v; want %d",
					len(pcts), pcts, len(wantItemColumns))
			}
			cum := 0.0
			for i, pct := range pcts {
				cum += pct
				var wantEdge float64
				if i == len(pcts)-1 {
					wantEdge = tablePx
				} else {
					wantEdge = tablePx * cum / 100
				}
				if d := float64(edges[i+1]) - wantEdge; d < -1.5 || d > 1.5 {
					t.Errorf("column %d right edge: engine %dpx, template %.2fpx (off by %+.2f, tolerance 1.5)",
						i+1, edges[i+1], wantEdge, d)
				}
			}

			// And the engine has to survive producing the document, not just
			// describe it. Rendering the column set is where a missing column's
			// track index gets used: asserting the columns alone would not
			// notice an export that indexes off the end of the layout and
			// takes itself down. This is the difference between the two
			// renderers agreeing on paper and agreeing in the file.
			if _, err := q.build(); err != nil {
				t.Fatalf("the engine could not render this quotation: %v", err)
			}
		})
	}
}
