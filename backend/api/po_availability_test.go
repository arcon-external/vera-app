package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Issue #651: availability is the workspace's bought-state read. It lists the
// remaining leaves with customer-facing sell figures only (Story 42), and the
// create endpoint returns a typed 409 when the quotation or the requested leaf
// is exhausted.

type availabilityItemRow struct {
	QuotationItemNo          int    `json:"quotationItemNo"`
	QuotationSubNo           *int   `json:"quotationSubNo"`
	Label                    string `json:"label"`
	Description              string `json:"description"`
	UoM                      string `json:"uom"`
	QuotedQuantityScaled     int64  `json:"quotedQuantityScaled"`
	BoughtQuantityScaled     int64  `json:"boughtQuantityScaled"`
	DraftQuantityScaled      int64  `json:"draftQuantityScaled"`
	RemainingQuantityScaled  int64  `json:"remainingQuantityScaled"`
	OverBoughtQuantityScaled int64  `json:"overBoughtQuantityScaled"`
	QuotedUnitPriceMinor     int64  `json:"quotedUnitPriceMinor"`
}

type availabilityRow struct {
	QuotationID          int64                 `json:"quotationId"`
	RevisionNumber       int16                 `json:"revisionNumber"`
	LatestRevision       int16                 `json:"latestRevision"`
	AllBought            bool                  `json:"allBought"`
	LegacyHeaderDiscount bool                  `json:"legacyHeaderDiscount"`
	SavedPOCount         int                   `json:"savedPoCount"`
	DraftPOCount         int                   `json:"draftPoCount"`
	Items                []availabilityItemRow `json:"items"`
}

func decodeAvailability(t *testing.T, body map[string]any) availabilityRow {
	t.Helper()
	var row availabilityRow
	encodeDecode(t, body, &row)
	return row
}

func getAvailability(t *testing.T, server *Server, token string, quotationID int64) availabilityRow {
	t.Helper()
	code, body := doJSON(t, server, http.MethodGet,
		"/v1/po-sources/"+strconv.FormatInt(quotationID, 10)+"/availability", token, nil)
	if code != http.StatusOK {
		t.Fatalf("GET availability = %d (%v); want 200", code, body)
	}
	return decodeAvailability(t, body)
}

func TestPOSourceAvailabilityShapeAndFullBoughtRefusal(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_availability")
	quotationID, _ := exportForSelector(t, server, token, poSourceInput("Availability", "Sellable line"))

	code, body := doJSON(t, server, http.MethodGet,
		"/v1/po-sources/"+strconv.FormatInt(quotationID, 10)+"/availability", token, nil)
	if code != http.StatusOK {
		t.Fatalf("availability = %d (%v); want 200", code, body)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal availability: %v", err)
	}
	// Story 42: availability is a customer-facing shape. No supplier cost or
	// margin may appear anywhere in it.
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"cost", "margin", "profit", "pph"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("availability leaked %q: %s", forbidden, raw)
		}
	}
	availability := decodeAvailability(t, body)
	if availability.QuotationID != quotationID || availability.AllBought {
		t.Fatalf("fresh availability = %+v; want the quotation with room", availability)
	}
	if len(availability.Items) != 1 {
		t.Fatalf("availability items = %d; want the one leaf", len(availability.Items))
	}
	item := availability.Items[0]
	if item.QuotedQuantityScaled != 10_000 || item.RemainingQuantityScaled != 10_000 ||
		item.BoughtQuantityScaled != 0 || item.DraftQuantityScaled != 0 {
		t.Errorf("fresh item = %+v; want ten thousandths still remaining", item)
	}
	if item.QuotedUnitPriceMinor != 100_000 {
		t.Errorf("quoted price = %d; want the source sell 100000", item.QuotedUnitPriceMinor)
	}

	// Finalize one PO for the whole leaf; availability then says All bought and
	// the next create is a typed 409.
	code, body = createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create = %d (%v); want 201", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token,
		finalizeBody(draft, "PO-API-651-FULL"))
	if code != http.StatusOK {
		t.Fatalf("finalize = %d (%v); want 200", code, body)
	}
	availability = getAvailability(t, server, token, quotationID)
	if !availability.AllBought || availability.SavedPOCount != 1 {
		t.Errorf("after finalize = allBought %v / saved %d; want the quotation exhausted",
			availability.AllBought, availability.SavedPOCount)
	}
	if availability.Items[0].RemainingQuantityScaled != 0 {
		t.Errorf("remaining = %d; want zero", availability.Items[0].RemainingQuantityScaled)
	}

	code, body = createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusConflict || body["code"] != "quotation_fully_bought" {
		t.Fatalf("create on an exhausted quotation = %d (%v); want 409/quotation_fully_bought",
			code, body)
	}
}

func TestPOSourceAvailabilityExhaustedItemRefusal(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_availability_item")
	input := poSourceInput("Two leaves", "First leaf")
	input["Items"] = []map[string]any{
		{"Description": "First leaf", "Qty": 1, "UoM": "Pc", "UnitPrice": 100_000},
		{"Description": "Second leaf", "Qty": 1, "UoM": "Pc", "UnitPrice": 200_000},
	}
	quotationID, _ := exportForSelector(t, server, token, input)

	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create the two-line draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	if len(draft.Items) != 2 {
		t.Fatalf("draft items = %d; want both leaves", len(draft.Items))
	}

	// Finalize a PO that takes only the first leaf and acknowledges the second
	// as not ordered. The second leaf stays orderable.
	finalize := finalizeBody(draft, "PO-API-651-PART")
	finalize["items"] = []map[string]any{{
		"quotationItemNo": draft.Items[0].QuotationItemNo,
		"quotationSubNo":  draft.Items[0].QuotationSubNo,
		"description":     draft.Items[0].Description,
		"quantityScaled":  draft.Items[0].QuantityScaled,
		"uom":             draft.Items[0].UoM,
		"unitPriceMinor":  draft.Items[0].UnitPriceMinor,
		"discountEnabled": draft.Items[0].DiscountEnabled,
		"discountPercent": draft.Items[0].DiscountPercent,
	}}
	finalize["acknowledgedMissing"] = []string{"2"}
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token, finalize)
	if code != http.StatusOK {
		t.Fatalf("finalize the first leaf only = %d (%v); want 200", code, body)
	}

	availability := getAvailability(t, server, token, quotationID)
	if availability.AllBought {
		t.Fatalf("a one-of-two order marked the quotation All bought: %+v", availability)
	}
	var first, second *availabilityItemRow
	for index := range availability.Items {
		switch availability.Items[index].QuotationItemNo {
		case 1:
			first = &availability.Items[index]
		case 2:
			second = &availability.Items[index]
		}
	}
	if first == nil || second == nil {
		t.Fatalf("availability items = %+v; want both leaves", availability.Items)
	}
	if first.RemainingQuantityScaled != 0 || second.RemainingQuantityScaled != 10_000 {
		t.Errorf("remaining = first %d / second %d; want 0/10000",
			first.RemainingQuantityScaled, second.RemainingQuantityScaled)
	}

	// Explicitly selecting the exhausted first leaf is the item refusal; an
	// empty selection defaults to the remaining second leaf and is allowed.
	code, body = createPODraft(t, server, token, quotationID, 0, map[string]any{
		"items": []map[string]any{{"quotationItemNo": 1}},
	})
	if code != http.StatusConflict || body["code"] != "po_item_already_bought" {
		t.Fatalf("create on an exhausted item = %d (%v); want 409/po_item_already_bought",
			code, body)
	}
	code, body = createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create from the remaining leaf = %d (%v); want 201", code, body)
	}
	secondDraft := decodePurchaseOrder(t, body)
	if len(secondDraft.Items) != 1 || secondDraft.Items[0].QuotationItemNo == nil ||
		*secondDraft.Items[0].QuotationItemNo != 2 {
		t.Errorf("second draft = %+v; want only the remaining second leaf", secondDraft.Items)
	}
}

func TestPOSourceListCarriesBoughtStateBadges(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_badges")
	quotationID, _ := exportForSelector(t, server, token, poSourceInput("Badges", "Badge line"))

	code, body := doJSON(t, server, http.MethodGet, "/v1/po-sources", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d (%v)", code, body)
	}
	list := decodePOList(t, body)
	if len(list.Quotations) != 1 {
		t.Fatalf("rows = %d; want one", len(list.Quotations))
	}
	row := list.Quotations[0]
	if row.ID != quotationID {
		t.Fatalf("row id = %d; want %d", row.ID, quotationID)
	}
	if row.AllBought || row.SavedPOCount != 0 || row.DraftPOCount != 0 ||
		row.RemainingLeafCount != 1 || row.LeafCount != 1 {
		t.Errorf("fresh row badges = %+v; want open with one remaining leaf", row)
	}

	// A draft is advisory: the row says Partial, shows the draft count, and is
	// still selectable.
	code, body = createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet, "/v1/po-sources", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list after draft = %d (%v)", code, body)
	}
	row = decodePOList(t, body).Quotations[0]
	if row.AllBought || row.DraftPOCount != 1 || row.SavedPOCount != 0 {
		t.Errorf("draft row badges = %+v; want partial with one advisory draft", row)
	}
}

// TestPurchaseOrderDraftExposesTheQuotedBaselineWithoutSupplierCost pins the
// #653 workspace shape: each source-backed PO line carries the frozen
// row-discounted quotation price and the quantity still orderable, and neither
// field is a supplier cost (Story 42).
func TestPurchaseOrderDraftExposesTheQuotedBaselineWithoutSupplierCost(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_baseline")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())

	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create = %d (%v); want 201", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	if len(draft.Items) != 2 {
		t.Fatalf("lines = %d; want 2", len(draft.Items))
	}
	first, second := draft.Items[0], draft.Items[1]
	// The broad fixture has 2 x 120000 at a 10% row discount, and 1 x 50000 at
	// the same discount. The baseline is what the customer agreed to.
	if first.QuotedUnitPriceMinor == nil || *first.QuotedUnitPriceMinor != 108_000 ||
		first.RemainingQuantityScaled == nil || *first.RemainingQuantityScaled != 20_000 {
		t.Errorf("line 1 baseline/remaining = %v/%v; want 108000/20000",
			first.QuotedUnitPriceMinor, first.RemainingQuantityScaled)
	}
	if second.QuotedUnitPriceMinor == nil || *second.QuotedUnitPriceMinor != 45_000 ||
		second.RemainingQuantityScaled == nil || *second.RemainingQuantityScaled != 10_000 {
		t.Errorf("line 2 baseline/remaining = %v/%v; want 45000/10000",
			second.QuotedUnitPriceMinor, second.RemainingQuantityScaled)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal PO draft: %v", err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"costunit", "costminor", "cost_unit", "marginminor", "pph"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("PO workspace leaked %q: %s", forbidden, raw)
		}
	}
}
