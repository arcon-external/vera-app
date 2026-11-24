package api

import (
	"net/http"
	"testing"
)

// TestExportRefusesAReusedItemNumberOverHTTP is the client-facing half of
// #648's identity guard: the store refuses a revision that hands a new product
// a number the previous revision used for something else, and the export
// endpoint must answer with a typed 422 rather than the generic 409 that maps
// every other store failure. The desktop shows the code, so it has to survive
// the HTTP seam.
func TestExportRefusesAReusedItemNumberOverHTTP(t *testing.T) {
	server, _, token := closureTestServer(t, "vera_api_item_identity")

	// Revision 0 keeps a gap at 2, as a quotation whose line 2 was removed
	// before its first export would.
	first := poSourceInput("Stable identity", "Line one")
	first["Items"] = []map[string]any{
		{"No": 1, "Description": "Line one", "Qty": 1, "UoM": "Lot", "UnitPrice": 100000},
		{"No": 3, "Description": "Line three", "Qty": 1, "UoM": "Lot", "UnitPrice": 300000},
	}
	_, quoteNumber := exportForSelector(t, server, token, first)

	second := poSourceInput("Stable identity", "Line one")
	second["Items"] = []map[string]any{
		{"No": 1, "Description": "Line one", "Qty": 1, "UoM": "Lot", "UnitPrice": 100000},
		{"No": 2, "Description": "A different product", "Qty": 1, "UoM": "Lot", "UnitPrice": 200000},
	}
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input":       second,
		"quoteNumber": quoteNumber,
		"revision":    1,
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("reused item number export = %d; want 422 (%v)", code, body)
	}
	if body["code"] != "quotation_item_identity_reused" {
		t.Errorf("reused item number code = %v; want quotation_item_identity_reused", body["code"])
	}
}
