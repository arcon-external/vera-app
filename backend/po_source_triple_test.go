package backend

import (
	"testing"

	"vera-app/backend/apicontract"
)

// B4: the desktop review DTO carries the source tax triple straight through
// from the HTTP row. Normalization happens server-side; the desktop maps.
func TestMapPurchaseOrderSourceTripleDTO(t *testing.T) {
	row := apicontract.PurchaseOrderReviewRow{
		Source: apicontract.PurchaseOrderSourceRow{
			QuotationID:     12,
			RevisionNumber:  0,
			QuoteNumber:     "1502260042",
			TaxZone:         "NonFTZ",
			TaxType:         "Goods",
			ServiceCategory: "",
		},
	}
	dto := mapPurchaseOrderReviewDTO(row)
	if dto.Source.TaxZone != "NonFTZ" || dto.Source.TaxType != "Goods" || dto.Source.ServiceCategory != "" {
		t.Errorf("source triple = %+v; want NonFTZ/Goods/\"\" passed through",
			dto.Source)
	}
}
