package backend

import (
	"encoding/json"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #496 wire compatibility. The API ships on every merge while desktops
// update on their own cadence, so two directions must hold:
//
//  1. an old-shaped payload (no sourceState / printedTotalsKnown) still decodes
//     into the new structs, and the mapper derives live/former;
//  2. a new never-linked payload still decodes into an OLD struct without an
//     error, landing in the Source-missing bucket with numeric zeros.
//
// Go's encoding/json treats JSON null into a non-pointer scalar as a silent
// no-op. These tests use that fact rather than a pointer-widening contract.

// oldPurchaseOrderRow is the shape a pre-#496 desktop was compiled against.
type oldPurchaseOrderRow struct {
	UUID            string `json:"uuid"`
	State           string `json:"state"`
	QuoteNumber     string `json:"quoteNumber"`
	CustomerName    string `json:"customerName"`
	PONumber        string `json:"poNumber"`
	ReceivedDate    string `json:"receivedDate"`
	Currency        string `json:"currency"`
	ID              int64  `json:"id"`
	QuotationID     int64  `json:"quotationId"`
	SourceRevision  int16  `json:"sourceRevision,omitempty"`
	CustomerID      int64  `json:"customerId"`
	SubtotalMinor   int64  `json:"subtotalMinor"`
	GrandTotalMinor int64  `json:"grandTotalMinor"`
	SourceMissing   bool   `json:"sourceMissing"`
}

func TestOldShapePayloadDecodesAndDerivesTheSourceState(t *testing.T) {
	// A live record whose source is revision 00: sourceRevision is absent
	// because it is omitempty, and revision 0 is still a real link.
	liveJSON := `{"id":1,"uuid":"u1","state":"draft","quotationId":7,
		"customerId":2,"quoteNumber":"1502260001","sourceMissing":false}`
	var live apicontract.PurchaseOrderRow
	if err := json.Unmarshal([]byte(liveJSON), &live); err != nil {
		t.Fatalf("old live payload into the new row: %v", err)
	}
	liveDTO := mapPurchaseOrderDTO(live)
	if liveDTO.SourceState != "live" || liveDTO.SourceMissing || !liveDTO.PrintedTotalsKnown {
		t.Errorf("derived live state = %q missing %v printedKnown %v; want live/false/true",
			liveDTO.SourceState, liveDTO.SourceMissing, liveDTO.PrintedTotalsKnown)
	}
	if liveDTO.SourceRevision != 0 {
		t.Errorf("revision = %d; revision 00 must survive as a real linked revision", liveDTO.SourceRevision)
	}

	// A formerly-linked record: the old wire said sourceMissing true.
	formerJSON := `{"id":2,"uuid":"u2","state":"draft","quotationId":9,
		"customerId":2,"quoteNumber":"1502260002","sourceMissing":true}`
	var former apicontract.PurchaseOrderRow
	if err := json.Unmarshal([]byte(formerJSON), &former); err != nil {
		t.Fatalf("old former payload into the new row: %v", err)
	}
	formerDTO := mapPurchaseOrderDTO(former)
	if formerDTO.SourceState != "former" || !formerDTO.SourceMissing || !formerDTO.PrintedTotalsKnown {
		t.Errorf("derived former state = %q missing %v printedKnown %v; want former/true/true",
			formerDTO.SourceState, formerDTO.SourceMissing, formerDTO.PrintedTotalsKnown)
	}
}

func TestNewNeverLinkedPayloadStillDecodesIntoAnOldRow(t *testing.T) {
	// The new wire keeps every numeric field numeric. An old desktop ignores
	// the added enum and lands the row in its existing Source-missing bucket.
	newJSON := `{"id":3,"uuid":"u3","state":"draft","quotationId":0,
		"customerId":0,"sourceRevision":0,"quoteNumber":"","customerName":"",
		"sourceMissing":true,"sourceState":"never","printedTotalsKnown":false,
		"subtotalMinor":0,"grandTotalMinor":0}`
	var old oldPurchaseOrderRow
	if err := json.Unmarshal([]byte(newJSON), &old); err != nil {
		t.Fatalf("new never-linked payload into an old row: %v", err)
	}
	if !old.SourceMissing || old.QuotationID != 0 || old.CustomerID != 0 || old.SourceRevision != 0 {
		t.Errorf("old reader saw = %+v; want numeric zeros and the Source-missing flag", old)
	}
}
