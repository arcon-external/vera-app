package backend

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"vera-app/backend/auth"
	gpdfrender "vera-app/backend/render/gpdf"
)

// itemHeadCellRe pulls the header cells out of the item table, and only that
// table: the document has other <th>s nowhere, but scoping it to the head table
// keeps the test honest if it ever grows one.
var (
	itemHeadTableRe = regexp.MustCompile(`(?s)<table class="items-head-table">(.*?)</table>`)
	headCellRe      = regexp.MustCompile(`<th>(.*?)</th>`)
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

// TestRenderersAgreeOnItemTableColumns is the check whose absence let a release
// ship a PDF that disagreed with its own preview.
//
// The template adds a Disc. column only when some row carries a per-row
// discount, and the native engine used to have a fixed six-column model with no
// idea the column existed. Nothing compared the two: the equivalence tests
// compare pagination, and they skip without a Chrome runtime anyway.
//
// This needs no browser. It renders the real template and asks the engine what
// it would print, which is exactly the pair that drifted.
func TestRenderersAgreeOnItemTableColumns(t *testing.T) {
	templates := &Quotation{}
	if err := Startup(templates, context.Background()); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	cases := []struct {
		name    string
		items   []QuotationItemInput
		wantCol int
	}{
		{
			name: "no row carries a discount",
			items: []QuotationItemInput{
				{Description: "plain line", UoM: "Lot", Qty: 1, UnitPrice: 1000000},
			},
			wantCol: 6,
		},
		{
			name: "a line carries a discount",
			items: []QuotationItemInput{
				{Description: "discounted line", UoM: "Lot", Qty: 1, UnitPrice: 1000000,
					DiscountEnabled: true, DiscountPercent: 10},
			},
			wantCol: 7,
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
			wantCol: 7,
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
			fromEngine := gpdfrender.HeadLabels(q.Data.ShowLineDiscounts)

			if len(fromTemplate) != tc.wantCol {
				t.Fatalf("template printed %d columns %v; want %d",
					len(fromTemplate), fromTemplate, tc.wantCol)
			}
			if len(fromEngine) != len(fromTemplate) {
				t.Fatalf("the engine would print %d columns %v, the template %d %v.\n"+
					"The preview, the stored snapshot and the PDF must show the same table.",
					len(fromEngine), fromEngine, len(fromTemplate), fromTemplate)
			}
			for i := range fromTemplate {
				if fromTemplate[i] != fromEngine[i] {
					t.Errorf("column %d: template says %q, engine says %q",
						i+1, fromTemplate[i], fromEngine[i])
				}
			}

			// And the engine has to survive producing the document, not just
			// describe it. Rendering the column set is where a missing column's
			// track index gets used: the discount cell's text is "—" on a row
			// that carries none, which is not empty, so a renderer that trusts
			// "there is always a track" indexes off the end of the layout and
			// takes the export down. Asserting the columns alone would not
			// notice; this is the difference between the two renderers agreeing
			// on paper and agreeing in the file.
			if _, err := q.build(); err != nil {
				t.Fatalf("the engine could not render this quotation: %v", err)
			}
		})
	}
}
