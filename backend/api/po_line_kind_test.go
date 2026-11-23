package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Issue #655: line_kind crosses the HTTP contract. These tests drive the real
// server and disposable Postgres. The read carries every line's kind, an
// absent write value is the compatible quotation, an explicit extra
// round-trips through save, finalize and the correction snapshot, and an
// unknown kind or an extra that names a quotation reference is a named 422.

func TestPurchaseOrderLineKindRoundTripsOverTheAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_line_kind")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := decodePurchaseOrder(t, mustCreatePODraft(t, server, token, quotationID))
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	// Every line the server copied from the quotation is quotation-kind. This
	// is the read half: before #655 the field did not exist on the wire.
	if len(draft.Items) != 2 {
		t.Fatalf("items = %d; want the two source lines", len(draft.Items))
	}
	for index, item := range draft.Items {
		if item.LineKind != "quotation" {
			t.Errorf("source line %d kind = %q; want quotation", index+1, item.LineKind)
		}
	}

	// Save: the quotation lines omit lineKind exactly as an older client does,
	// and one explicit extra (no reference) is added. Absent must read back as
	// quotation, the extra as extra.
	body := draftUpdateBody(draft)
	items, _ := body["items"].([]map[string]any)
	items = append(items, map[string]any{
		"description":    "Freight surcharge",
		"quantityScaled": 20000,
		"uom":            "Lot",
		"unitPriceMinor": 500000,
		"lineKind":       "extra",
	})
	body["items"] = items
	code, response := doJSON(t, server, http.MethodPut, path, token, body)
	if code != http.StatusOK {
		t.Fatalf("save with an extra = %d (%v)", code, response)
	}
	updated := decodePurchaseOrder(t, response)
	if len(updated.Items) != 3 {
		t.Fatalf("saved items = %d; want 3", len(updated.Items))
	}
	if updated.Items[0].LineKind != "quotation" || updated.Items[1].LineKind != "quotation" {
		t.Errorf("absent-kind lines read %q / %q; want quotation",
			updated.Items[0].LineKind, updated.Items[1].LineKind)
	}
	extra := updated.Items[2]
	if extra.LineKind != "extra" || extra.QuotationItemNo != nil || extra.QuotationSubNo != nil {
		t.Errorf("extra line = %+v; want kind extra with no reference", extra)
	}
	if extra.Description != "Freight surcharge" || extra.QuantityScaled != 20000 ||
		extra.UoM != "Lot" || extra.UnitPriceMinor != 500000 {
		t.Errorf("extra values = %+v; want the editable entry", extra)
	}

	// Reopen: the stored kind is what a new session reads.
	code, response = doJSON(t, server, http.MethodGet, path, token, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen = %d (%v)", code, response)
	}
	reopened := decodePurchaseOrder(t, response)
	if len(reopened.Items) != 3 || reopened.Items[2].LineKind != "extra" {
		t.Errorf("reopened lines = %+v; want the extra preserved", reopened.Items)
	}

	// Finalize the record so its correction snapshot can be read. An extra
	// line is an acknowledged difference; the body states each line's kind.
	finalBody := finalizeBody(reopened, "PO-655-KIND")
	finalItems, _ := finalBody["items"].([]map[string]any)
	for index := range finalItems {
		if index < len(reopened.Items) {
			finalItems[index]["lineKind"] = reopened.Items[index].LineKind
		}
	}
	finalBody["items"] = finalItems
	finalBody["acknowledgedExtra"] = []int{3}
	code, response = doJSON(t, server, http.MethodPost, path+"/finalize", token, finalBody)
	if code != http.StatusOK {
		t.Fatalf("finalize with an extra = %d (%v)", code, response)
	}
	saved := decodePurchaseOrder(t, response)
	if saved.State != "saved" || len(saved.Items) != 3 || saved.Items[2].LineKind != "extra" {
		t.Errorf("saved record = %+v; want the extra still extra", saved)
	}

	// The correction snapshots the kinds it replaced, so the history can tell
	// an intentional extra from a legacy free line.
	correctBody := correctionBody(saved, "Restate the freight line")
	correctItems, _ := correctBody["items"].([]map[string]any)
	for index := range correctItems {
		if index < len(saved.Items) {
			correctItems[index]["lineKind"] = saved.Items[index].LineKind
		}
	}
	correctBody["items"] = correctItems
	code, response = doJSON(t, server, http.MethodPost, path+"/correction", token, correctBody)
	if code != http.StatusOK {
		t.Fatalf("correction = %d (%v)", code, response)
	}
	code, response = doJSON(t, server, http.MethodGet, path+"/history", token, nil)
	if code != http.StatusOK {
		t.Fatalf("history = %d (%v)", code, response)
	}
	var history struct {
		History []struct {
			Items []struct {
				LineNumber int    `json:"lineNumber"`
				LineKind   string `json:"lineKind"`
			} `json:"items"`
		} `json:"history"`
	}
	encodeDecode(t, response, &history)
	if len(history.History) != 1 || len(history.History[0].Items) != 3 {
		t.Fatalf("history = %+v; want one three-line snapshot", history.History)
	}
	wantKinds := []string{"quotation", "quotation", "extra"}
	for index, item := range history.History[0].Items {
		if item.LineKind != wantKinds[index] {
			t.Errorf("snapshot line %d kind = %q; want %q",
				item.LineNumber, item.LineKind, wantKinds[index])
		}
	}
}

// TestPurchaseOrderLineKindRefusalsAreNamed proves the store's typed refusal
// reaches a client as the promised 422 code, not a 500.
func TestPurchaseOrderLineKindRefusalsAreNamed(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_line_kind_refusals")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := decodePurchaseOrder(t, mustCreatePODraft(t, server, token, quotationID))
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	cases := []struct {
		name  string
		patch map[string]any
	}{
		{name: "unknown kind", patch: map[string]any{"lineKind": "pass_through"}},
		{
			name: "extra names a quotation reference",
			// overwrite the source reference after the kind: an extra line was
			// never on the quotation, so a reference is a contradiction.
			patch: map[string]any{"lineKind": "extra"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// Reload so a case never reuses a version an earlier case moved.
			code, snapshot := doJSON(t, server, http.MethodGet, path, token, nil)
			if code != http.StatusOK {
				t.Fatalf("reload = %d (%v)", code, snapshot)
			}
			current := decodePurchaseOrder(t, snapshot)
			body := draftUpdateBody(current)
			items, _ := body["items"].([]map[string]any)
			for key, value := range testCase.patch {
				items[0][key] = value
			}
			if testCase.name == "extra names a quotation reference" {
				items[0]["quotationItemNo"] = 1
				items[0]["quotationSubNo"] = nil
			}
			body["items"] = items
			code, response := doJSON(t, server, http.MethodPut, path, token, body)
			if code != http.StatusUnprocessableEntity {
				t.Fatalf("save = %d (%v); want 422", code, response)
			}
			if response["code"] != "po_line_kind_invalid" {
				t.Errorf("code = %v; want po_line_kind_invalid", response["code"])
			}
			if message, _ := response["error"].(string); message == "" {
				t.Errorf("a refusal must carry its message: %v", response)
			}
		})
	}
}

// TestPurchaseOrderAggregateAcceptsAnExtraChildOverTheAPI proves an explicit
// extra can be an aggregate child, the group total includes it, and neither
// the line nor the header carries internal economics (Story 42).
func TestPurchaseOrderAggregateAcceptsAnExtraChildOverTheAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_line_kind_aggregate")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := decodePurchaseOrder(t, mustCreatePODraft(t, server, token, quotationID))
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	body := draftUpdateBody(draft)
	items, _ := body["items"].([]map[string]any)
	items[0]["aggregateGroupIndex"] = 1
	// Groups must be contiguous: the extra joins the first quotation line and
	// the second source line stays ungrouped.
	items = append([]map[string]any{items[0], {
		"description":         "Freight surcharge",
		"quantityScaled":      20000,
		"uom":                 "Lot",
		"unitPriceMinor":      500000,
		"lineKind":            "extra",
		"aggregateGroupIndex": 1,
	}}, items[1])
	body["items"] = items
	body["aggregateGroups"] = []map[string]any{
		aggregateGroupPayload("Delivered package", 10000, "Lot", true),
	}
	code, response := doJSON(t, server, http.MethodPut, path, token, body)
	if code != http.StatusOK {
		t.Fatalf("save grouped extra = %d (%v)", code, response)
	}
	saved := decodePurchaseOrder(t, response)
	if len(saved.AggregateGroups) != 1 {
		t.Fatalf("groups = %d; want 1", len(saved.AggregateGroups))
	}
	// 2 x 108000 (the quotation line) + 2 x 500000 (the extra) = 1216000.
	group := saved.AggregateGroups[0]
	if group.TotalMinor != 1216000 || !group.ShowChildrenToCustomer {
		t.Errorf("group = %+v; want total 1216000 with children shown", group)
	}
	if len(saved.Items) != 3 || saved.Items[1].LineKind != "extra" {
		t.Fatalf("saved lines = %+v; want the extra as the second line", saved.Items)
	}
	if saved.Items[0].AggregateGroupID == nil || *saved.Items[0].AggregateGroupID != group.ID {
		t.Errorf("quotation child membership = %v; want group %d",
			saved.Items[0].AggregateGroupID, group.ID)
	}
	if saved.Items[1].AggregateGroupID == nil || *saved.Items[1].AggregateGroupID != group.ID {
		t.Errorf("extra child membership = %v; want group %d",
			saved.Items[1].AggregateGroupID, group.ID)
	}
	if saved.Items[2].AggregateGroupID != nil {
		t.Errorf("ungrouped line membership = %v; want none", saved.Items[2].AggregateGroupID)
	}
	// Grouping is presentation only: the extra's amount is still its own.
	if saved.SubtotalMinor != saved.Items[0].NetMinor+saved.Items[1].NetMinor+saved.Items[2].NetMinor {
		t.Errorf("subtotal %d does not equal the sum of its lines (%+v)",
			saved.SubtotalMinor, saved.Items)
	}

	// Story 42 on the wire: no cost, margin, variance, PPH or supplier key on
	// the extra line or the aggregate header.
	forbidden := []string{"cost", "margin", "variance", "pph", "supplier"}
	assertNoInternalEconomics := func(what string, encoded map[string]any) {
		t.Helper()
		for key := range encoded {
			lower := strings.ToLower(key)
			for _, word := range forbidden {
				if strings.Contains(lower, word) {
					t.Errorf("%s carries %q (%q)", what, key, word)
				}
			}
		}
	}
	rawItems, _ := response["items"].([]any)
	if len(rawItems) != 3 {
		t.Fatalf("raw items = %d; want 3", len(rawItems))
	}
	extraJSON, _ := rawItems[1].(map[string]any)
	assertNoInternalEconomics("the extra line", extraJSON)
	rawGroups, _ := response["aggregateGroups"].([]any)
	groupJSON, _ := rawGroups[0].(map[string]any)
	assertNoInternalEconomics("the aggregate header", groupJSON)
}
