package apicontract

import "testing"

// TestProtectedFieldListsComeFromOneOrderedSource pins the protected-field
// contract the API refuses by name. Both variants must read one ordered list:
// the full list is the contract's order, and the draft variant removes exactly
// the five source-basis names so a new protected field cannot be added to one
// variant and silently forgotten in the other (#493 review).
func TestProtectedFieldListsComeFromOneOrderedSource(t *testing.T) {
	wantAll := []string{
		"confirmationNumber",
		"confirmationWarn",
		"uuid",
		"customerId",
		"quotationId",
		"sourceRevision",
		"currency",
		"taxZone",
		"taxType",
		"serviceCategory",
		"taxCategory",
		"orgId",
		"state",
		"savedAt",
		"savedBy",
		"pdfAbsentConfirmed",
		"version",
		"id",
		"poNumberKey",
	}
	wantDraftExcluded := map[string]struct{}{
		"currency":        {},
		"taxZone":         {},
		"taxType":         {},
		"serviceCategory": {},
		"taxCategory":     {},
	}

	if len(purchaseOrderProtectedFields) != len(wantAll) {
		t.Fatalf("protected field count = %d; want %d", len(purchaseOrderProtectedFields), len(wantAll))
	}
	for index, field := range purchaseOrderProtectedFields {
		if field.name != wantAll[index] {
			t.Errorf("protected field %d = %q; want %q", index, field.name, wantAll[index])
		}
	}
	if len(purchaseOrderSourceBasisFields) != len(wantDraftExcluded) {
		t.Fatalf("source-basis exclusion count = %d; want %d",
			len(purchaseOrderSourceBasisFields), len(wantDraftExcluded))
	}
	for name := range wantDraftExcluded {
		if _, ok := purchaseOrderSourceBasisFields[name]; !ok {
			t.Errorf("%q is not excluded from the draft variant; want only %v",
				name, wantDraftExcluded)
		}
	}

	text := func(value string) *string { return &value }
	flag := func(value bool) *bool { return &value }
	count := func(value int64) *int64 { return &value }
	revision := func(value int16) *int16 { return &value }
	cases := []struct {
		name string
		set  func(*PurchaseOrderProtectedFieldsRequest)
	}{
		{"confirmationNumber", func(r *PurchaseOrderProtectedFieldsRequest) { r.ConfirmationNumber = text("1502260001") }},
		{"confirmationWarn", func(r *PurchaseOrderProtectedFieldsRequest) { r.ConfirmationWarn = flag(true) }},
		{"uuid", func(r *PurchaseOrderProtectedFieldsRequest) { r.UUID = text("uuid") }},
		{"customerId", func(r *PurchaseOrderProtectedFieldsRequest) { r.CustomerID = count(2) }},
		{"quotationId", func(r *PurchaseOrderProtectedFieldsRequest) { r.QuotationID = count(12) }},
		{"sourceRevision", func(r *PurchaseOrderProtectedFieldsRequest) { r.SourceRevision = revision(1) }},
		{"currency", func(r *PurchaseOrderProtectedFieldsRequest) { r.Currency = text("USD") }},
		{"taxZone", func(r *PurchaseOrderProtectedFieldsRequest) { r.TaxZone = text("FTZ") }},
		{"taxType", func(r *PurchaseOrderProtectedFieldsRequest) { r.TaxType = text("Goods") }},
		{"serviceCategory", func(r *PurchaseOrderProtectedFieldsRequest) { r.ServiceCategory = text("NonConstruction") }},
		{"taxCategory", func(r *PurchaseOrderProtectedFieldsRequest) { r.TaxCategory = text("SupplyInstall") }},
		{"orgId", func(r *PurchaseOrderProtectedFieldsRequest) { r.OrgID = text("org") }},
		{"state", func(r *PurchaseOrderProtectedFieldsRequest) { r.State = text("draft") }},
		{"savedAt", func(r *PurchaseOrderProtectedFieldsRequest) { r.SavedAt = text("2026-10-23T00:00:00Z") }},
		{"savedBy", func(r *PurchaseOrderProtectedFieldsRequest) { r.SavedBy = count(4) }},
		{"pdfAbsentConfirmed", func(r *PurchaseOrderProtectedFieldsRequest) { r.PdfAbsentConfirmed = flag(true) }},
		{"version", func(r *PurchaseOrderProtectedFieldsRequest) { r.Version = count(3) }},
		{"id", func(r *PurchaseOrderProtectedFieldsRequest) { r.ID = count(41) }},
		{"poNumberKey", func(r *PurchaseOrderProtectedFieldsRequest) { r.PONumberKey = text("1502260001") }},
	}
	if len(cases) != len(wantAll) {
		t.Fatalf("case count = %d; want %d", len(cases), len(wantAll))
	}
	for index, tc := range cases {
		if tc.name != wantAll[index] {
			t.Errorf("case %d = %q; want %q", index, tc.name, wantAll[index])
		}
		var request PurchaseOrderProtectedFieldsRequest
		tc.set(&request)
		if got := request.Named(); got != tc.name {
			t.Errorf("Named() with only %q set = %q", tc.name, got)
		}
		wantDraft := tc.name
		if _, excluded := wantDraftExcluded[tc.name]; excluded {
			wantDraft = ""
		}
		if got := request.NamedExceptSourceBasis(); got != wantDraft {
			t.Errorf("NamedExceptSourceBasis() with only %q set = %q; want %q",
				tc.name, got, wantDraft)
		}
	}
}

func TestFormatQuoteNumber(t *testing.T) {
	cases := []struct {
		customerCode int
		yy           int
		seq          int64
		want         string
	}{
		{2, 26, 1, "1502260001"},
		{1, 26, 112, "1501260112"},
		{2, 26, 777, "1502260777"},
		{6, 26, 42, "1506260042"},
		{2, 27, 1, "1502270001"},
		{2, 26, 9999, "1502269999"},
		// Overflow widens to 5 digits instead of wrapping.
		{2, 26, 10000, "15022610000"},
		{2, 26, 10001, "15022610001"},
	}
	for _, tc := range cases {
		if got := FormatQuoteNumber(tc.customerCode, tc.yy, tc.seq); got != tc.want {
			t.Errorf("FormatQuoteNumber(%d, %d, %d) = %q; want %q",
				tc.customerCode, tc.yy, tc.seq, got, tc.want)
		}
	}
}
