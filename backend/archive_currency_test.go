package backend

import (
	"encoding/json"
	"net/http"
	"testing"

	"vera-app/backend/apicontract"
)

// TestListQuotationsCarriesCurrency pins the summary side of the issue #336
// Wave B contract at the desktop mapping: the code the server emits per row
// reaches the summary the archive page resolves per-row entries from. Without
// it every row renders in the live profile currency, corrupting history.
func TestListQuotationsCarriesCurrency(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"quotations": []apicontract.QuotationSummaryRow{
				{ID: 7, QuoteNumber: "15020007", RevisionNumber: 1, CustomerName: "Acme",
					Subject: "USD job", CreatedAt: "2026-09-12T00:00:00Z",
					Currency: "USD", GrandTotal: 150000, DisplayGrand: "1,500.00"},
				{ID: 8, QuoteNumber: "15020008", RevisionNumber: 0, CustomerName: "Acme",
					Subject: "IDR job", CreatedAt: "2026-09-12T00:00:00Z",
					Currency: "IDR", GrandTotal: 261000, DisplayGrand: "261.000"},
			},
		})
	})

	q := newTestQuotation()
	rows, err := q.ListQuotations()
	if err != nil {
		t.Fatalf("ListQuotations failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d; want 2", len(rows))
	}
	if rows[0].Currency != "USD" {
		t.Errorf("row 0 currency = %q; want USD", rows[0].Currency)
	}
	if rows[1].Currency != "IDR" {
		t.Errorf("row 1 currency = %q; want IDR", rows[1].Currency)
	}
}
