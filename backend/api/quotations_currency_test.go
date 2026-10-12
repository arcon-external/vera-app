package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// TestMapSummaryRowCarriesCurrency pins the B1 contract: every archive row
// carries its own revision currency, so a mixed-currency archive is readable
// (USD 15,000.00 next to Rp-style 15.000.000, not two bare numbers).
func TestMapSummaryRowCarriesCurrency(t *testing.T) {
	at := time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC)
	rows := []store.QuotationSummary{
		{ID: 1, QuoteNumber: "15022601", RevisionNumber: 0, CustomerName: "Acme",
			Subject: "USD quote", CreatedAt: at, GrandTotal: 1500000, Currency: "USD"},
		{ID: 2, QuoteNumber: "15022602", RevisionNumber: 1, CustomerName: "Indo",
			Subject: "IDR quote", CreatedAt: at, GrandTotal: 15000000, Currency: "IDR"},
		{ID: 3, QuoteNumber: "15022603", RevisionNumber: 0, CustomerName: "Legacy",
			Subject: "no code stored", CreatedAt: at, GrandTotal: 9000, Currency: ""},
	}
	got := make([]apicontract.QuotationSummaryRow, 0, len(rows))
	for _, row := range rows {
		got = append(got, mapSummaryRow(row))
	}

	if got[0].Currency != "USD" {
		t.Errorf("USD row currency = %q; want USD", got[0].Currency)
	}
	if got[0].DisplayGrand != "15,000.00" {
		t.Errorf("USD row display = %q; want 15,000.00", got[0].DisplayGrand)
	}
	if got[1].Currency != "IDR" {
		t.Errorf("IDR row currency = %q; want IDR", got[1].Currency)
	}
	if got[1].DisplayGrand != "15.000.000" {
		t.Errorf("IDR row display = %q; want 15.000.000", got[1].DisplayGrand)
	}
	// An empty stored code normalises to the default, matching the display
	// strings (which already fall back to IDR formatting for unknown codes).
	if got[2].Currency != "IDR" {
		t.Errorf("empty-code row currency = %q; want the IDR default", got[2].Currency)
	}
	if got[2].DisplayGrand != "9.000" {
		t.Errorf("empty-code row display = %q; want 9.000", got[2].DisplayGrand)
	}
}

// TestMapDetailCarriesCurrency pins the same for the detail DTO: the revision
// currency travels with the money it describes.
func TestMapDetailCarriesCurrency(t *testing.T) {
	usd := store.QuotationRecord{
		QuotationSummary: store.QuotationSummary{
			ID: 7, QuoteNumber: "15022607", RevisionNumber: 0,
			CustomerName: "Acme", Subject: "USD detail",
		},
		// The outer field, not the embedded summary's: GetQuotation scans
		// r.currency into it, and it shadows the embedded one.
		Currency: "USD",
		Totals: store.RevisionTotals{
			MarginalizedGrand: 1500000, CustomerGrandTotal: 1500000,
		},
	}
	detail := mapDetail(usd, false)
	if detail.Currency != "USD" {
		t.Errorf("detail currency = %q; want USD", detail.Currency)
	}
	if detail.DisplayMarginalizedGrand != "15,000.00" {
		t.Errorf("detail display grand = %q; want 15,000.00", detail.DisplayMarginalizedGrand)
	}

	idr := usd
	idr.Currency = "IDR"
	idr.Totals.MarginalizedGrand = 15000000
	idr.Totals.CustomerGrandTotal = 15000000
	detail = mapDetail(idr, true)
	if detail.Currency != "IDR" {
		t.Errorf("detail currency = %q; want IDR", detail.Currency)
	}
	if detail.DisplayMarginalizedGrand != "15.000.000" {
		t.Errorf("detail display grand = %q; want 15.000.000", detail.DisplayMarginalizedGrand)
	}
	if !detail.HasPDF {
		t.Error("hasPdf = false; want true to flow through untouched")
	}
}

// TestCurrencyContractJSONKeys guards the wire shape Round 2 consumes: both
// DTOs serialise a "currency" key, always present (no omitempty), defaulting
// to IDR.
func TestCurrencyContractJSONKeys(t *testing.T) {
	summary, err := json.Marshal(apicontract.QuotationSummaryRow{Currency: "IDR"})
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if !strings.Contains(string(summary), `"currency":"IDR"`) {
		t.Errorf("summary JSON = %s; want a currency key", summary)
	}
	detail, err := json.Marshal(apicontract.QuotationDetail{Currency: "IDR"})
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	if !strings.Contains(string(detail), `"currency":"IDR"`) {
		t.Errorf("detail JSON = %s; want a currency key", detail)
	}
}
