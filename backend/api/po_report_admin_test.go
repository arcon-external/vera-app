package api

import (
	"net/http"
	"testing"
)

// Issue #683: the Reports workspace is an admin surface, and the API is the
// authority for that. Both report reads deny a staff session with the same
// stable code the route gate names, and an admin still reads the report. The
// gate runs before the handler, so the refusal never depends on the query.
func TestPurchaseOrderReportRoutesRequireAnAdmin(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_report_admin_gate")
	seedStaff(t, server.database, "report-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "report-staff@example.com", "staff-password")
	reportValues := reportQueryParams("2026-01-01", "2027-01-01", "month", "", "IDR")
	fulfillmentValues := fulfillmentParams("IDR", "", "", 1, 25)

	routes := []struct {
		name string
		url  string
	}{
		{name: "report", url: reportURL(reportValues)},
		{name: "fulfillment", url: fulfillmentURL(fulfillmentValues)},
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			code, body := doJSON(t, server, http.MethodGet, route.url, staffToken, nil)
			if code != http.StatusForbidden {
				t.Fatalf("staff %s = %d (%v); want 403", route.name, code, body)
			}
			if got, _ := body["code"].(string); got != "reports_forbidden" {
				t.Errorf("staff %s code = %q; want reports_forbidden", route.name, got)
			}

			code, body = doJSON(t, server, http.MethodGet, route.url, adminToken, nil)
			if code != http.StatusOK {
				t.Fatalf("admin %s = %d (%v); want 200", route.name, code, body)
			}
		})
	}
}
