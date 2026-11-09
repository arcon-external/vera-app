package api

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/store"
)

// B4: the review source carries the exact revision's normalized semantic
// triple for the link-time tax evidence. Pure mapper checks pin the legacy
// normalization; the HTTP check pins the wire field names end to end.
func TestPOReviewSourceCarriesNormalizedTaxTriple(t *testing.T) {
	legacy := store.PurchaseOrderSource{
		TaxZone: "NonFTZ", TaxCategory: "SupplyOnly",
		TaxType: "", ServiceCategory: "",
	}
	row := mapPurchaseOrderReview(store.PurchaseOrderReview{Source: legacy})
	if row.Source.TaxZone != "NonFTZ" || row.Source.TaxType != "Goods" || row.Source.ServiceCategory != "" {
		t.Errorf("legacy triple = %q/%q/%q; want NonFTZ/Goods/\"\" via the TaxCategory resolver",
			row.Source.TaxZone, row.Source.TaxType, row.Source.ServiceCategory)
	}
	current := store.PurchaseOrderSource{
		TaxZone: "NonFTZ", TaxCategory: "SupplyOnly",
		TaxType: "Goods", ServiceCategory: "",
	}
	row = mapPurchaseOrderReview(store.PurchaseOrderReview{Source: current})
	if row.Source.TaxZone != "NonFTZ" || row.Source.TaxType != "Goods" || row.Source.ServiceCategory != "" {
		t.Errorf("current triple = %q/%q/%q; want the persisted triple unchanged",
			row.Source.TaxZone, row.Source.TaxType, row.Source.ServiceCategory)
	}
}

func TestPOReviewSourceTripleOverHTTP(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_source_triple")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	var review struct {
		Source struct {
			TaxZone         string `json:"taxZone"`
			TaxType         string `json:"taxType"`
			ServiceCategory string `json:"serviceCategory"`
		} `json:"source"`
	}
	code, body = doJSON(t, server, http.MethodGet, path+"/review", token, nil)
	if code != http.StatusOK {
		t.Fatalf("review = %d (%v)", code, body)
	}
	encodeDecode(t, body, &review)
	if review.Source.TaxZone != "FTZ" || review.Source.TaxType != "Goods" || review.Source.ServiceCategory != "" {
		t.Errorf("source triple = %+v; want the FTZ/Goods basis", review.Source)
	}

	// A legacy revision row with empty type/service still projects the
	// normalized triple through the TaxCategory resolver.
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	tag, err := pool.Exec(context.Background(),
		`UPDATE "vera_api_po_source_triple".quotation_revisions SET tax_type = '', service_category = '' WHERE quotation_id = $1`,
		quotationID)
	if err != nil {
		t.Fatalf("simulate legacy revision: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("legacy simulation touched %d revisions; want exactly the exported one", tag.RowsAffected())
	}
	code, body = doJSON(t, server, http.MethodGet, path+"/review", token, nil)
	if code != http.StatusOK {
		t.Fatalf("legacy review = %d (%v)", code, body)
	}
	encodeDecode(t, body, &review)
	if review.Source.TaxZone != "FTZ" || review.Source.TaxType != "Goods" || review.Source.ServiceCategory != "" {
		t.Errorf("legacy source triple = %+v; want normalized FTZ/Goods", review.Source)
	}
}
