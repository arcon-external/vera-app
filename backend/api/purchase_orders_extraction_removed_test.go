package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Issue #504: the extraction surface is deleted, and this is the permanent
// guard against it silently returning.
//
// Two assertions cover the two ways a route can return: the route table must
// contain no extraction path at all, and an HTTP request to each known path must
// answer 404. The route-table check catches a live registration even when its
// handler would answer 404 for the tested id; the HTTP check catches a handler
// that is mounted outside the table. No session is sent on purpose: a route that
// still exists normally answers 401 through requireSession before its handler
// runs, so the HTTP assertion is red for the expected re-add. Both checks run
// without Postgres, which is what makes this a permanent part of the default
// suite rather than an integration test that skips.
func TestExtractionRoutesAreGone(t *testing.T) {
	server := NewServer(nil, nil, nil)
	for _, route := range server.echo.Routes() {
		if strings.Contains(route.Path, "extraction") {
			t.Fatalf("%s %s is still registered; the extraction surface is deleted (#504)",
				route.Method, route.Path)
		}
	}
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/purchase-orders/extraction/availability"},
		{http.MethodPost, "/v1/purchase-orders/1/extractions"},
		{http.MethodGet, "/v1/purchase-orders/1/extractions/latest"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, nil)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("%s %s = %d; want 404 — the extraction surface is deleted (#504)",
					route.method, route.path, recorder.Code)
			}
		})
	}
}
