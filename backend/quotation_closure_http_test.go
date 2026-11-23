package backend

import (
	"encoding/json"
	"net/http"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #656 phase B: the desktop wrappers. The API suite proves the server;
// these prove the desktop sends the promised request and maps the answer back
// into the archive shape the detail page already renders.

func TestCloseQuotationPostsTheReasonAndMapsTheDetail(t *testing.T) {
	closedAt := "2026-11-20T03:04:05Z"
	validUntil := "2026-12-01"
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/quotations/5/close" {
			t.Errorf("%s %s; want POST /v1/quotations/5/close",
				request.Method, request.URL.Path)
		}
		var body apicontract.QuotationCloseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode close body: %v", err)
		}
		if body.Reason != "customer cancelled" {
			t.Errorf("reason = %q; want the exact sentence", body.Reason)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.QuotationDetail{
			ID: 5, QuoteNumber: "15020005", Revision: 2,
			CustomerName: "Acme", Subject: "Detail",
			Closed: true, ClosedAt: &closedAt, ClosedReason: "customer cancelled",
			ClosedByName: "Closure Admin", ValidUntil: &validUntil,
			AvailableRevisions: []int16{2, 1},
			Items:              []apicontract.QuotationItemRow{},
		})
	})

	q := newTestQuotation()
	updated, err := q.CloseQuotation(5, "customer cancelled")
	if err != nil {
		t.Fatalf("CloseQuotation failed: %v", err)
	}
	if !updated.Closed || updated.ClosedAt == nil || *updated.ClosedAt != closedAt ||
		updated.ClosedReason != "customer cancelled" ||
		updated.ClosedByName != "Closure Admin" ||
		updated.ValidUntil == nil || *updated.ValidUntil != validUntil {
		t.Errorf("mapped closure = %+v; want the stored close", updated)
	}
	if updated.RevisionNumber != 2 || len(updated.AvailableRevisions) != 2 {
		t.Errorf("mapped revision = %d/%v; want 2 with its revisions",
			updated.RevisionNumber, updated.AvailableRevisions)
	}
}

func TestReopenQuotationPostsAndMapsTheOpenDetail(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/quotations/5/reopen" {
			t.Errorf("%s %s; want POST /v1/quotations/5/reopen",
				request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.QuotationDetail{
			ID: 5, QuoteNumber: "15020005", Revision: 2,
			CustomerName: "Acme", Subject: "Detail",
			Closed: false, AvailableRevisions: []int16{2, 1},
			Items: []apicontract.QuotationItemRow{},
		})
	})

	q := newTestQuotation()
	updated, err := q.ReopenQuotation(5)
	if err != nil {
		t.Fatalf("ReopenQuotation failed: %v", err)
	}
	if updated.Closed || updated.ClosedAt != nil || updated.ClosedReason != "" ||
		updated.ClosedByName != "" {
		t.Errorf("reopen mapping = %+v; want an open quotation", updated)
	}
}

func TestQuotationClosureBindingsRequireSignIn(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	_ = ClearSessionToken()
	q := newTestQuotation()
	if _, err := q.CloseQuotation(5, "reason"); err == nil {
		t.Fatal("CloseQuotation without a session succeeded; want denial")
	}
	if _, err := q.ReopenQuotation(5); err == nil {
		t.Fatal("ReopenQuotation without a session succeeded; want denial")
	}
}
