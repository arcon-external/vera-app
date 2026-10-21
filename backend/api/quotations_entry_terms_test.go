package api

import (
	"testing"

	"vera-app/backend/store"
)

// TestMapDetailCarriesEntryTerms pins the copy/paste contract (#485): the
// detail DTO carries the archived revision's own entry values — the customer
// code the editor resolves, the client-directory addresses, and the validity /
// payment terms the editor restores — so the desktop copy path reads them
// rather than the zero values the DTO used to leave behind.
func TestMapDetailCarriesEntryTerms(t *testing.T) {
	record := store.QuotationRecord{
		CustomerCode:     4,
		CustomerAddress:  "Jl. Klien 12",
		RecipientAddress: "Gedung Penerima 3",
		ValidityDays:     45,
		PaymentTermDays:  60,
	}
	detail := mapDetail(record, false)
	if detail.CustomerCode != 4 {
		t.Errorf("customerCode = %d; want 4", detail.CustomerCode)
	}
	if detail.CustomerAddress != "Jl. Klien 12" {
		t.Errorf("customerAddress = %q; want the stored client address", detail.CustomerAddress)
	}
	if detail.RecipientAddress != "Gedung Penerima 3" {
		t.Errorf("recipientAddress = %q; want the stored recipient address", detail.RecipientAddress)
	}
	if detail.ValidityDays != 45 {
		t.Errorf("validityDays = %d; want 45", detail.ValidityDays)
	}
	if detail.PaymentTermDays != 60 {
		t.Errorf("paymentTermDays = %d; want 60", detail.PaymentTermDays)
	}
}
