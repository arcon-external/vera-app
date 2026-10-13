package backend

import (
	"errors"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

func gatedPrepareInput() QuotationInput {
	return QuotationInput{
		Items: []QuotationItemInput{
			{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 1000},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      30,
		PaymentTerm:   30,
		LeadTimeDays:  30,
		CustomerID:    2,
		Incoterm:      "EXW",
		TaxZone:       TaxZoneFTZ,
	}
}

func TestPrepareRequiresSignIn(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	if err := ClearSessionToken(); err != nil {
		t.Fatalf("clear session: %v", err)
	}
	q := &Quotation{}
	if err := q.Prepare(gatedPrepareInput()); err == nil {
		t.Fatal("Prepare without auth service succeeded; want denial")
	}

	signedOut := &Quotation{}
	signedOut.SetAuthService(auth.NewStaticService(store.AuthUser{}, errors.New("not signed in")))
	if err := signedOut.Prepare(gatedPrepareInput()); err == nil {
		t.Fatal("Prepare without a session succeeded; want denial")
	}
}

func TestStaffCanPrepareButCannotDelete(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := q.Prepare(gatedPrepareInput()); err != nil {
		t.Fatalf("staff Prepare failed: %v", err)
	}
	if err := q.DeleteQuotation(1); err == nil {
		t.Fatal("staff DeleteQuotation succeeded; want admin-only denial")
	}
}

func TestAdminCanDelete(t *testing.T) {
	q := &Quotation{}
	q.SetAuthService(auth.AdminTestService())
	// No store attached, so it must fail on the store check, not the gate.
	if err := q.DeleteQuotation(1); err == nil || err.Error() == "admin access required" {
		t.Fatalf("admin DeleteQuotation error = %v; want store error, not denial", err)
	}
}
