package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Issue #504: the extraction surface is deleted, and this is the permanent
// guard against it silently returning.
//
// The assertion is the external behaviour — an HTTP status — rather than the
// absence of a file or a symbol, so it survives any future refactor that keeps
// the routes gone. No session is sent on purpose: a route that still exists
// answers 401 through requireSession before its handler runs, so a re-added
// route fails this test even though the deleted path answers 404. That also
// keeps the guard runnable without Postgres, which is what makes it a permanent
// part of the default suite rather than an integration test that skips.
func TestExtractionRoutesAreGone(t *testing.T) {
	server := NewServer(nil, nil, nil)
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
