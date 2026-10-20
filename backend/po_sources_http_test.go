package backend

import (
	"encoding/json"
	"net/http"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #467: the desktop reaches the selector through these bindings. The API
// integration suite proves the server behaviour; these tests prove the desktop
// wrapper sends the promised query and maps the answer without dropping a
// field the modal renders.

func TestListPOSourceQuotationsMapsPageAndQuery(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/po-sources" {
			t.Errorf("path = %s; want /v1/po-sources", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("page") != "2" {
			t.Errorf("page = %q; want 2", query.Get("page"))
		}
		if query.Get("search") != "infineon" {
			t.Errorf("search = %q; want infineon", query.Get("search"))
		}
		if query.Get("hideUsed") != "true" {
			t.Errorf("hideUsed = %q; want true", query.Get("hideUsed"))
		}
		_ = json.NewEncoder(writer).Encode(apicontract.POSourceListResponse{
			Page:       2,
			PageSize:   15,
			Total:      17,
			TotalPages: 2,
			Quotations: []apicontract.POSourceQuotationRow{{
				ID: 12, QuoteNumber: "1502260012", Subject: "Electrical",
				CustomerName:   "PT. Infineon Technologies Batam",
				LatestRevision: 2, IssuedRevisionCount: 3, HasActivePO: false,
			}},
		})
	})

	q := newTestQuotation()
	page, err := q.ListPOSourceQuotations(POSourceQuery{
		Page: 2, Search: "infineon", HideUsed: true,
	})
	if err != nil {
		t.Fatalf("ListPOSourceQuotations failed: %v", err)
	}
	if page.Page != 2 || page.PageSize != 15 || page.Total != 17 || page.TotalPages != 2 {
		t.Errorf("page metadata = %+v; want 2/15/17/2", page)
	}
	if len(page.Quotations) != 1 {
		t.Fatalf("rows = %d; want 1", len(page.Quotations))
	}
	row := page.Quotations[0]
	if row.ID != 12 || row.QuoteNumber != "1502260012" || row.LatestRevision != 2 ||
		row.IssuedRevisionCount != 3 || row.HasActivePO {
		t.Errorf("row = %+v; want the full selectable shape", row)
	}
}

func TestListPOSourceRevisionsMapsNewestFirst(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/po-sources/12/revisions" {
			t.Errorf("path = %s; want /v1/po-sources/12/revisions", request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.POSourceRevisionsResponse{
			QuotationID: 12, QuoteNumber: "1502260012", Subject: "Electrical",
			CustomerName: "PT. Infineon Technologies Batam",
			Revisions: []apicontract.POSourceRevisionRow{
				{Revision: 2, IssuedAt: "2026-10-20T10:00:00+07:00"},
				{Revision: 1, IssuedAt: "2026-10-18T10:00:00+07:00"},
			},
		})
	})

	q := newTestQuotation()
	detail, err := q.ListPOSourceRevisions(12)
	if err != nil {
		t.Fatalf("ListPOSourceRevisions failed: %v", err)
	}
	if detail.QuotationID != 12 || detail.QuoteNumber != "1502260012" {
		t.Errorf("identity = %+v; want quotation 12", detail)
	}
	if len(detail.Revisions) != 2 || detail.Revisions[0].Revision != 2 {
		t.Errorf("revisions = %+v; want newest first", detail.Revisions)
	}
}

func TestValidatePOSourcePostsTheExactPair(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/po-sources/validate" || request.Method != http.MethodPost {
			t.Errorf("%s %s; want POST /v1/po-sources/validate", request.Method, request.URL.Path)
		}
		var body apicontract.POSourceValidateRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.QuotationID != 12 || body.RevisionNumber != 2 {
			t.Errorf("body = %+v; want 12/2", body)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.POSourceValidationResponse{
			Valid: false, Reason: "revision_not_found",
			QuotationID: 12, RevisionNumber: 2,
		})
	})

	q := newTestQuotation()
	validation, err := q.ValidatePOSource(POSourceSelection{QuotationID: 12, RevisionNumber: 2})
	if err != nil {
		t.Fatalf("ValidatePOSource failed: %v", err)
	}
	if validation.Valid || validation.Reason != "revision_not_found" {
		t.Errorf("validation = %+v; want an invalid revision answer", validation)
	}
}

func TestPOSourceBindingsRequireSignIn(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	_ = ClearSessionToken()
	q := newTestQuotation()
	if _, err := q.ListPOSourceQuotations(POSourceQuery{Page: 1}); err == nil {
		t.Error("ListPOSourceQuotations without a session succeeded")
	}
	if _, err := q.ListPOSourceRevisions(1); err == nil {
		t.Error("ListPOSourceRevisions without a session succeeded")
	}
	if _, err := q.ValidatePOSource(POSourceSelection{QuotationID: 1}); err == nil {
		t.Error("ValidatePOSource without a session succeeded")
	}
}
