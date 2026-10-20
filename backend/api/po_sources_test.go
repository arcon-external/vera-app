package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// encodeDecode round-trips a decoded JSON body into a typed struct, so each
// assertion names its fields instead of digging through map[string]any.
func encodeDecode(t *testing.T, from any, to any) {
	t.Helper()
	encoded, err := json.Marshal(from)
	if err != nil {
		t.Fatalf("encode decoded body: %v", err)
	}
	if err := json.Unmarshal(encoded, to); err != nil {
		t.Fatalf("decode typed body: %v", err)
	}
}

// Issue #467: the PO source selector reads issued Quotations and their
// revisions. These tests drive the authenticated HTTP surface against a real
// disposable schema: pagination, search across every promised field, the
// exclusion of unsaved drafts, the eligibility passthrough, revision listing
// and the confirm-time revalidation.
//
// They deliberately observe public behaviour only: a row's JSON, a status
// code, and a persisted outcome. The SQL behind them may change entirely.

// poSourceInput builds a minimal valid quotation input. The item text is a
// parameter because search is the behaviour under test.
func poSourceInput(subject, item string) map[string]any {
	return map[string]any{
		"Items": []map[string]any{
			{"Description": item, "Qty": 1, "UoM": "Lot", "UnitPrice": 100000},
		},
		"Subject":       subject,
		"RecipientName": "Test Recipient",
		"TaxZone":       "FTZ",
		"Validity":      30,
		"PaymentTerm":   30,
		"LeadTimeDays":  10,
		"CustomerID":    2,
		"Incoterm":      "EXW",
	}
}

// poSourcePackageInput is one line whose priced sub-item carries the text the
// search must find. Sub-items are the composition of a package line, and the
// selector promises to search them too.
func poSourcePackageInput(subject, subItem string) map[string]any {
	return map[string]any{
		"Items": []map[string]any{
			{
				"Description": "Package line",
				"Qty":         1,
				"UoM":         "Lot",
				"SubItems": []map[string]any{
					{
						"Description": subItem,
						"UoM":         "Unit",
						"UnitCost":    100000,
						"Qty":         1,
					},
				},
			},
		},
		"Subject":       subject,
		"RecipientName": "Test Recipient",
		"TaxZone":       "FTZ",
		"Validity":      30,
		"PaymentTerm":   30,
		"LeadTimeDays":  10,
		"CustomerID":    2,
		"Incoterm":      "EXW",
	}
}

func exportForSelector(t *testing.T, server *Server, token string, input map[string]any) (int64, string) {
	t.Helper()
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, input)
	if code != http.StatusCreated {
		t.Fatalf("export = %d; want 201 (%v)", code, body)
	}
	id, _ := body["id"].(float64)
	number, _ := body["quoteNumber"].(string)
	if id < 1 || number == "" {
		t.Fatalf("export returned id %v number %q", body["id"], number)
	}
	return int64(id), number
}

func exportNextRevision(t *testing.T, server *Server, token, quoteNumber string, revision int, input map[string]any) {
	t.Helper()
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input":       input,
		"quoteNumber": quoteNumber,
		"revision":    revision,
	})
	if code != http.StatusCreated {
		t.Fatalf("export revision %d = %d; want 201 (%v)", revision, code, body)
	}
}

type poSourceRow struct {
	ID                  int64  `json:"id"`
	QuoteNumber         string `json:"quoteNumber"`
	Subject             string `json:"subject"`
	CustomerName        string `json:"customerName"`
	LatestRevision      int16  `json:"latestRevision"`
	IssuedRevisionCount int    `json:"issuedRevisionCount"`
	HasActivePO         bool   `json:"hasActivePO"`
}

type poSourceList struct {
	Quotations []poSourceRow `json:"quotations"`
	Page       int           `json:"page"`
	PageSize   int           `json:"pageSize"`
	Total      int           `json:"total"`
	TotalPages int           `json:"totalPages"`
}

func decodePOList(t *testing.T, body map[string]any) poSourceList {
	t.Helper()
	var list poSourceList
	encodeDecode(t, body, &list)
	return list
}

func listPOSources(t *testing.T, server *Server, token, query string) poSourceList {
	t.Helper()
	code, body := doJSON(t, server, http.MethodGet, "/v1/po-sources"+query, token, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /v1/po-sources%s = %d; want 200 (%v)", query, code, body)
	}
	return decodePOList(t, body)
}

func TestPOSourceListPaginatesIssuedQuotationsFifteenPerPage(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_page")

	for i := 1; i <= 16; i++ {
		exportForSelector(t, server, token, poSourceInput(fmt.Sprintf("Bulk quotation %02d", i), "Bulk item"))
	}

	first := listPOSources(t, server, token, "?page=1")
	if first.PageSize != 15 {
		t.Errorf("pageSize = %d; want 15", first.PageSize)
	}
	if first.Total != 16 {
		t.Errorf("total = %d; want 16", first.Total)
	}
	if first.TotalPages != 2 {
		t.Errorf("totalPages = %d; want 2", first.TotalPages)
	}
	if len(first.Quotations) != 15 {
		t.Fatalf("page 1 rows = %d; want 15", len(first.Quotations))
	}
	second := listPOSources(t, server, token, "?page=2")
	if len(second.Quotations) != 1 {
		t.Fatalf("page 2 rows = %d; want 1", len(second.Quotations))
	}
	// Newest first: the page-2 row is the oldest of the sixteen, and the
	// page-1 rows descend by id.
	if second.Quotations[0].Subject != "Bulk quotation 01" {
		t.Errorf("page 2 subject = %q; want the oldest quotation", second.Quotations[0].Subject)
	}
	if first.Quotations[0].ID <= first.Quotations[1].ID {
		t.Errorf("rows are not newest-first: ids %d then %d", first.Quotations[0].ID, first.Quotations[1].ID)
	}

	row := first.Quotations[0]
	if row.QuoteNumber == "" || !strings.Contains(row.CustomerName, "Infineon") {
		t.Errorf("row identity = %+v; want a quote number and the customer name", row)
	}
	if row.LatestRevision != 0 || row.IssuedRevisionCount != 1 {
		t.Errorf("row revision facts = latest %d, count %d; want 0 and 1", row.LatestRevision, row.IssuedRevisionCount)
	}
	if row.HasActivePO {
		t.Errorf("row claims an active PO before PO persistence exists")
	}
}

func TestPOSourceListCountsEveryIssuedRevisionAndShowsTheLatest(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_revisions_count")

	id, number := exportForSelector(t, server, token, poSourceInput("First issue", "First item"))
	exportNextRevision(t, server, token, number, 1, poSourceInput("Second issue", "Second item"))

	list := listPOSources(t, server, token, "")
	if len(list.Quotations) != 1 {
		t.Fatalf("rows = %d; want 1 quotation with two revisions", len(list.Quotations))
	}
	row := list.Quotations[0]
	if row.ID != id {
		t.Errorf("row id = %d; want %d", row.ID, id)
	}
	if row.LatestRevision != 1 {
		t.Errorf("latest revision = %d; want 1", row.LatestRevision)
	}
	if row.IssuedRevisionCount != 2 {
		t.Errorf("issued revision count = %d; want 2", row.IssuedRevisionCount)
	}
	if row.Subject != "Second issue" {
		t.Errorf("subject = %q; want the latest revision's subject", row.Subject)
	}
}

func TestPOSourceSearchCoversNumberSubjectCustomerAndItemText(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_search")
	_, electricalNumber := exportForSelector(t, server, token, poSourceInput("Electrical installation", "Cable tray"))
	exportForSelector(t, server, token, map[string]any{
		"Items": []map[string]any{
			{"Description": "Centrifugal pump", "Qty": 1, "UoM": "Lot", "UnitPrice": 100000},
		},
		"Subject": "Mechanical works", "RecipientName": "Test Recipient", "TaxZone": "FTZ",
		"Validity": 30, "PaymentTerm": 30, "LeadTimeDays": 10, "CustomerID": 3, "Incoterm": "EXW",
	})
	exportForSelector(t, server, token, poSourcePackageInput("Cooling package", "Chiller compressor"))
	exportForSelector(t, server, token, poSourceInput("Discount 5% for early payment", "Percent item"))
	exportForSelector(t, server, token, poSourceInput("Underscore_check subject", "Underscore item"))

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"item description, case-insensitive", "?search=CABLE%20TRAY", 1},
		{"sub-item description", "?search=chiller", 1},
		{"subject", "?search=ELECTRICAL", 1},
		{"quotation number", "?search=" + electricalNumber, 1},
		{"customer name", "?search=oilfield", 1},
		{"literal percent, not a wildcard", "?search=%25", 1},
		{"literal underscore, not a wildcard", "?search=_", 1},
		{"no match", "?search=zzzz-nothing", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list := listPOSources(t, server, token, tc.query)
			if len(list.Quotations) != tc.want {
				t.Fatalf("search %q rows = %d; want %d (%v)", tc.query, len(list.Quotations), tc.want, list.Quotations)
			}
			if list.Total != tc.want {
				t.Errorf("search %q total = %d; want %d", tc.query, list.Total, tc.want)
			}
		})
	}
}

func TestPOSourceSearchCoversEveryIssuedRevision(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_search_revisions")

	id, number := exportForSelector(t, server, token, poSourceInput("Gearbox supply", "Obsolete gearbox"))
	exportNextRevision(t, server, token, number, 1, poSourceInput("Gearbox supply", "Replacement pump housing"))

	// The item text exists only in revision 0. Revision 0 is selectable in
	// step two, so searching every issued revision is what makes it findable;
	// latest-only search would answer zero rows here.
	got := listPOSources(t, server, token, "?search=obsolete")
	if len(got.Quotations) != 1 {
		t.Fatalf("search in an earlier revision rows = %d; want 1 (%v)", len(got.Quotations), got.Quotations)
	}
	if got.Total != 1 {
		t.Errorf("search in an earlier revision total = %d; want 1", got.Total)
	}
	row := got.Quotations[0]
	if row.ID != id {
		t.Errorf("row id = %d; want %d", row.ID, id)
	}
	// One row per Quotation: the match came from revision 0, but the row
	// still describes the latest issued revision.
	if row.LatestRevision != 1 || row.Subject != "Gearbox supply" {
		t.Errorf("row = %+v; want the latest revision (1) with its subject", row)
	}

	// Searching the latest revision still works, and still yields one row.
	latest := listPOSources(t, server, token, "?search=replacement%20pump")
	if len(latest.Quotations) != 1 || latest.Quotations[0].LatestRevision != 1 {
		t.Fatalf("latest-revision search = %+v; want one row at revision 1", latest.Quotations)
	}

	// The revision list still offers 0: searchable and selectable describe the
	// same set of revisions.
	code, body := doJSON(t, server, http.MethodGet, "/v1/po-sources/"+strconv.FormatInt(id, 10)+"/revisions", token, nil)
	if code != http.StatusOK {
		t.Fatalf("revisions = %d; want 200 (%v)", code, body)
	}
	var detail struct {
		Revisions []struct {
			Revision int16 `json:"revision"`
		} `json:"revisions"`
	}
	encodeDecode(t, body, &detail)
	foundZero := false
	for _, revision := range detail.Revisions {
		if revision.Revision == 0 {
			foundZero = true
		}
	}
	if !foundZero {
		t.Errorf("revisions = %+v; want revision 0 offered for selection", detail.Revisions)
	}
}

func TestPOSourceListExcludesUnsavedDrafts(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_drafts")

	exportForSelector(t, server, token, poSourceInput("Issued quotation", "Issued item"))
	code, body := doJSON(t, server, http.MethodPost, "/v1/drafts", token, map[string]any{
		"label":     "Half-finished",
		"subject":   "Draft-only subject zzz",
		"itemCount": 0,
		"payload":   `{"items":[]}`,
	})
	if code != http.StatusOK {
		t.Fatalf("save draft = %d; want 200 (%v)", code, body)
	}

	list := listPOSources(t, server, token, "")
	if len(list.Quotations) != 1 || list.Total != 1 {
		t.Errorf("list includes a draft: rows %d total %d; want only the issued quotation", len(list.Quotations), list.Total)
	}
	searched := listPOSources(t, server, token, "?search=zzz")
	if len(searched.Quotations) != 0 {
		t.Errorf("search matched an unsaved draft: %v", searched.Quotations)
	}
}

func TestPOSourceListPassesEligibilityThroughAndAcceptsTheHideFilter(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_eligibility")

	exportForSelector(t, server, token, poSourceInput("Eligible quotation", "Eligible item"))

	list := listPOSources(t, server, token, "?hideUsed=true")
	if len(list.Quotations) != 1 {
		t.Fatalf("hideUsed rows = %d; want 1 while no PO exists", len(list.Quotations))
	}
	// The contract carries the flag even though #468 supplies the real query:
	// a row that omits eligibility would force the frontend to invent it.
	if list.Quotations[0].HasActivePO {
		t.Errorf("hasActivePO = true; want false until PO persistence exists")
	}
}

func TestPOSourceRevisionsAreIssuedOnlyNewestFirst(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_revision_list")

	id, number := exportForSelector(t, server, token, poSourceInput("Two revisions", "Item"))
	exportNextRevision(t, server, token, number, 1, poSourceInput("Two revisions", "Item"))
	soleID, _ := exportForSelector(t, server, token, poSourceInput("One revision", "Item"))

	code, body := doJSON(t, server, http.MethodGet, "/v1/po-sources/"+strconv.FormatInt(id, 10)+"/revisions", token, nil)
	if code != http.StatusOK {
		t.Fatalf("revisions = %d; want 200 (%v)", code, body)
	}
	var two struct {
		QuotationID  int64  `json:"quotationId"`
		QuoteNumber  string `json:"quoteNumber"`
		CustomerName string `json:"customerName"`
		Revisions    []struct {
			Revision int16  `json:"revision"`
			IssuedAt string `json:"issuedAt"`
		} `json:"revisions"`
	}
	encodeDecode(t, body, &two)
	if two.QuotationID != id || two.QuoteNumber != number || !strings.Contains(two.CustomerName, "Infineon") {
		t.Errorf("revision response identity = %+v; want quotation %d / %s", two, id, number)
	}
	if len(two.Revisions) != 2 {
		t.Fatalf("revisions = %d; want 2", len(two.Revisions))
	}
	if two.Revisions[0].Revision != 1 || two.Revisions[1].Revision != 0 {
		t.Errorf("revision order = %d, %d; want newest first (1, 0)", two.Revisions[0].Revision, two.Revisions[1].Revision)
	}
	for _, revision := range two.Revisions {
		if strings.TrimSpace(revision.IssuedAt) == "" {
			t.Errorf("revision %d has no issuedAt", revision.Revision)
		}
	}

	code, body = doJSON(t, server, http.MethodGet, "/v1/po-sources/"+strconv.FormatInt(soleID, 10)+"/revisions", token, nil)
	if code != http.StatusOK {
		t.Fatalf("sole revisions = %d; want 200 (%v)", code, body)
	}
	var sole struct {
		Revisions []any `json:"revisions"`
	}
	encodeDecode(t, body, &sole)
	if len(sole.Revisions) != 1 {
		t.Errorf("sole revision count = %d; want 1 so the selector can auto-select it", len(sole.Revisions))
	}

	code, _ = doJSON(t, server, http.MethodGet, "/v1/po-sources/99999999/revisions", token, nil)
	if code != http.StatusNotFound {
		t.Errorf("missing quotation revisions = %d; want 404", code)
	}
}

func TestPOSourceSelectionRevalidatesAtConfirmTime(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_validate")

	id, number := exportForSelector(t, server, token, poSourceInput("Revalidated", "Item"))
	exportNextRevision(t, server, token, number, 1, poSourceInput("Revalidated", "Item"))

	code, body := doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, map[string]any{
		"quotationId":    id,
		"revisionNumber": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("validate = %d; want 200 (%v)", code, body)
	}
	if valid, _ := body["valid"].(bool); !valid {
		t.Fatalf("valid pair rejected: %v", body)
	}
	if revision, _ := body["revisionNumber"].(float64); int16(revision) != 1 {
		t.Errorf("validated revision = %v; want 1", body["revisionNumber"])
	}

	// Revision 0 is a real revision, not a missing value: supplied as zero it
	// must revalidate like any other issued revision.
	code, body = doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, map[string]any{
		"quotationId":    id,
		"revisionNumber": 0,
	})
	if code != http.StatusOK {
		t.Fatalf("validate revision 0 = %d; want 200 (%v)", code, body)
	}
	if valid, _ := body["valid"].(bool); !valid {
		t.Errorf("issued revision 0 rejected: %v", body)
	}

	code, body = doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, map[string]any{
		"quotationId":    id,
		"revisionNumber": 9,
	})
	if code != http.StatusOK {
		t.Fatalf("missing revision = %d; want a 200 validation answer", code)
	}
	if valid, _ := body["valid"].(bool); valid {
		t.Errorf("missing revision reported valid: %v", body)
	}
	if reason, _ := body["reason"].(string); reason != "revision_not_found" {
		t.Errorf("missing revision reason = %q; want revision_not_found", reason)
	}

	code, body = doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, map[string]any{
		"quotationId":    99999999,
		"revisionNumber": 0,
	})
	if code != http.StatusOK {
		t.Fatalf("missing quotation = %d; want a 200 validation answer", code)
	}
	if reason, _ := body["reason"].(string); reason != "quotation_not_found" {
		t.Errorf("missing quotation reason = %q; want quotation_not_found", reason)
	}

	if err := server.database.DeleteQuotation(context.Background(), id); err != nil {
		t.Fatalf("delete quotation: %v", err)
	}
	_, body = doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, map[string]any{
		"quotationId":    id,
		"revisionNumber": 1,
	})
	if valid, _ := body["valid"].(bool); valid {
		t.Errorf("deleted quotation reported valid: %v", body)
	}

	code, _ = doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, map[string]any{
		"quotationId":    0,
		"revisionNumber": -1,
	})
	if code != http.StatusBadRequest {
		t.Errorf("malformed validation = %d; want 400", code)
	}
}

func TestPOSourceRevalidationRequiresAnExplicitRevision(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_sources_validate_explicit")

	id, _ := exportForSelector(t, server, token, poSourceInput("Explicit revision", "Item"))

	cases := []struct {
		name string
		body map[string]any
	}{
		{"absent revision", map[string]any{"quotationId": id}},
		{"null revision", map[string]any{"quotationId": id, "revisionNumber": nil}},
		{"negative revision", map[string]any{"quotationId": id, "revisionNumber": -1}},
		{"absent quotation", map[string]any{"revisionNumber": 0}},
		{"zero quotation", map[string]any{"quotationId": 0, "revisionNumber": 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("validate %v = %d; want 400 (%v)", tc.body, code, body)
			}
		})
	}

	// Missing rows are answers, not malformed requests: they keep the 200
	// contract the confirm screen renders.
	code, body := doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, map[string]any{
		"quotationId":    99999999,
		"revisionNumber": 0,
	})
	if code != http.StatusOK {
		t.Fatalf("missing quotation = %d; want 200 (%v)", code, body)
	}
	if reason, _ := body["reason"].(string); reason != "quotation_not_found" {
		t.Errorf("missing quotation reason = %q; want quotation_not_found", reason)
	}

	code, body = doJSON(t, server, http.MethodPost, "/v1/po-sources/validate", token, map[string]any{
		"quotationId":    id,
		"revisionNumber": 9,
	})
	if code != http.StatusOK {
		t.Fatalf("missing revision = %d; want 200 (%v)", code, body)
	}
	if reason, _ := body["reason"].(string); reason != "revision_not_found" {
		t.Errorf("missing revision reason = %q; want revision_not_found", reason)
	}
}

func TestPOSourceEndpointsRequireASession(t *testing.T) {
	server, _ := testQuotationServer(t, "vera_api_po_sources_auth")

	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/po-sources"},
		{http.MethodGet, "/v1/po-sources/1/revisions"},
		{http.MethodPost, "/v1/po-sources/validate"},
	} {
		code, _ := doJSON(t, server, request.method, request.path, "", map[string]any{
			"quotationId": 1, "revisionNumber": 0,
		})
		if code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session = %d; want 401", request.method, request.path, code)
		}
	}
}
