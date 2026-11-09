package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The #518 archive route is a plain authenticated read. These tests pin the
// transport boundary only: auth, argument shape, and a real handler answer for
// an unknown record (Echo's route-missing 404 carries no JSON body, so a body
// assertion distinguishes "no such route" from "no such record").

func TestPurchaseOrderReconciliationSessionsRequiresSession(t *testing.T) {
	server, _ := testServer(t, "vera_api_518_route")
	request := httptest.NewRequest(http.MethodGet,
		"/v1/purchase-orders/1/reconciliation-sessions", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated history read = %d; want 401", recorder.Code)
	}
}

func TestPurchaseOrderReconciliationSessionsRejectsInvalidID(t *testing.T) {
	server, _ := testServer(t, "vera_api_518_invalid")
	token := doLogin(t, server, "api-admin@example.com", "test-password")
	for _, path := range []string{
		"/v1/purchase-orders/abc/reconciliation-sessions",
		"/v1/purchase-orders/0/reconciliation-sessions",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d; want 400", path, recorder.Code)
		}
	}
}

func TestPurchaseOrderReconciliationSessionsUnknownRecordIs404(t *testing.T) {
	server, _ := testServer(t, "vera_api_518_unknown")
	token := doLogin(t, server, "api-admin@example.com", "test-password")
	request := httptest.NewRequest(http.MethodGet,
		"/v1/purchase-orders/999999/reconciliation-sessions", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown record history = %d; want 404", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "purchase order not found") {
		t.Fatalf("404 body = %q; want the handler's JSON body, not a missing route", recorder.Body.String())
	}
}
