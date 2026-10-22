package api

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"vera-app/backend/store"
)

// Issue #497: the authenticated Staff draft-link surface. These tests drive the
// real server and a disposable Postgres schema: a Staff member links a
// never-linked and a formerly-linked draft through the review, a saved record is
// refused by name rather than by the admin gate, an occupied target is 409, a
// stale version is 409, a currency mismatch is 422 with an edit-and-retry remedy,
// and a tax-basis difference needs an explicit keep/source choice.

type draftLinkReviewBody struct {
	Order struct {
		Currency    string `json:"currency"`
		SourceState string `json:"sourceState"`
	} `json:"order"`
	Source struct {
		QuotationID    int64  `json:"quotationId"`
		RevisionNumber int16  `json:"revisionNumber"`
		QuoteNumber    string `json:"quoteNumber"`
		Currency       string `json:"currency"`
		CustomerName   string `json:"customerName"`
	} `json:"source"`
	DocumentState string `json:"documentState"`
	DraftTax      struct {
		TaxZone         string `json:"taxZone"`
		TaxType         string `json:"taxType"`
		ServiceCategory string `json:"serviceCategory"`
		PpnBps          int16  `json:"ppnBps"`
	} `json:"draftTax"`
	SourceTax struct {
		TaxZone         string `json:"taxZone"`
		TaxType         string `json:"taxType"`
		ServiceCategory string `json:"serviceCategory"`
		PpnBps          int16  `json:"ppnBps"`
	} `json:"sourceTax"`
	TaxChoiceRequired bool `json:"taxChoiceRequired"`
}

func draftLinkReviewRequest(quotationID int64, revision int16) map[string]any {
	return map[string]any{
		"targetQuotationId":    quotationID,
		"targetRevisionNumber": revision,
	}
}

// draftLinkRequestBody builds the link payload from the stored row: its version,
// the target pair, and the review's explicit choices.
func draftLinkRequestBody(
	row purchaseOrderRow,
	quotationID int64,
	revision int16,
	decisions []map[string]any,
	missing []string,
	extra []int,
	taxChoice string,
) map[string]any {
	if decisions == nil {
		decisions = []map[string]any{}
	}
	if missing == nil {
		missing = []string{}
	}
	if extra == nil {
		extra = []int{}
	}
	return map[string]any{
		"expectedVersion":      row.Version,
		"targetQuotationId":    quotationID,
		"targetRevisionNumber": revision,
		"decisions":            decisions,
		"acknowledgedMissing":  missing,
		"acknowledgedExtra":    extra,
		"taxChoice":            taxChoice,
	}
}

// prepareSourceLessDraft creates a blank draft and saves the values a link test
// needs. The zero items leave every source line missing, which is the point.
func prepareSourceLessDraft(t *testing.T, server *Server, token string, extra map[string]any) purchaseOrderRow {
	t.Helper()
	code, body := createBlankPODraft(t, server, token, nil)
	if code != http.StatusCreated {
		t.Fatalf("blank create = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	update := map[string]any{
		"expectedVersion": draft.Version,
		"receivedDate":    "2026-10-20",
		"paymentTerms":    "30 days",
		"incoterms":       "EXW",
		"discountPercent": 10,
		"poNumber":        "PO-497-HTTP",
		"billingAddress":  "Jl. Entered before the link",
		"deliveryAddress": "Jl. Delivery SENTINEL",
		"contactEmail":    "sentinel@example.com",
		"items":           []map[string]any{},
	}
	for key, value := range extra {
		update[key] = value
	}
	code, body = doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, update)
	if code != http.StatusOK {
		t.Fatalf("save blank draft = %d (%v)", code, body)
	}
	return decodePurchaseOrder(t, body)
}

func TestStaffLinksANeverLinkedDraftOverHTTP(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_497_never")
	sourceID, sourceNumber := exportForSelector(t, server, adminToken, quotationInput())
	seedStaff(t, server.database, "link-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "link-staff@example.com", "staff-password")
	draft := prepareSourceLessDraft(t, server, staffToken, nil)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	code, body := doJSON(t, server, http.MethodPost, path+"/source-link/review", staffToken,
		draftLinkReviewRequest(sourceID, 0))
	if code != http.StatusOK {
		t.Fatalf("staff draft-link review = %d (%v)", code, body)
	}
	var review draftLinkReviewBody
	encodeDecode(t, body, &review)
	if review.Source.QuotationID != sourceID || review.Source.QuoteNumber != sourceNumber {
		t.Errorf("review source = %d %q; want %d %q",
			review.Source.QuotationID, review.Source.QuoteNumber, sourceID, sourceNumber)
	}
	if review.Order.SourceState != "never" || review.Source.Currency != "IDR" {
		t.Errorf("review state/currency = %q/%q; want never/IDR",
			review.Order.SourceState, review.Source.Currency)
	}
	if review.TaxChoiceRequired {
		t.Error("matching FTZ bases must not require a tax choice")
	}

	code, body = doJSON(t, server, http.MethodPost, path+"/source-link", staffToken,
		draftLinkRequestBody(draft, sourceID, 0, nil, []string{"1", "2"}, nil, ""))
	if code != http.StatusOK {
		t.Fatalf("staff draft link = %d (%v)", code, body)
	}
	linked := decodePurchaseOrder(t, body)
	if linked.SourceState != "live" || linked.QuotationID != sourceID ||
		linked.QuoteNumber != sourceNumber || linked.CustomerName == "" {
		t.Errorf("linked = %+v; want the source identity", linked)
	}
	if linked.PONumber != "PO-497-HTTP" || linked.PaymentTerms != "30 days" ||
		linked.Incoterms != "EXW" || linked.DeliveryAddress != "Jl. Delivery SENTINEL" ||
		linked.ContactEmail != "sentinel@example.com" || linked.DiscountPercent != 10 {
		t.Errorf("PO-owned values moved: %+v", linked)
	}
	if linked.BillingAddress == "" || linked.BillingAddress == "Jl. Entered before the link" {
		t.Errorf("billing address = %q; a new customer's address comes from the revision",
			linked.BillingAddress)
	}
	if !linked.PrintedTotalsKnown {
		t.Error("a linked draft must know its printed source totals")
	}

	// A draft link writes no correction or repair history.
	code, body = doJSON(t, server, http.MethodGet, path+"/history", staffToken, nil)
	if code != http.StatusOK {
		t.Fatalf("history = %d (%v)", code, body)
	}
	var history struct {
		History []purchaseOrderCorrectionRow `json:"history"`
	}
	encodeDecode(t, body, &history)
	if len(history.History) != 0 {
		t.Errorf("link wrote %d history entries; want none", len(history.History))
	}

	// The finalized-record repair stays admin-only for Staff.
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair/review", staffToken,
		draftLinkReviewRequest(sourceID, 0))
	if code != http.StatusForbidden {
		t.Errorf("staff source repair after link = %d (%v); want 403", code, body)
	}
}

func TestStaffLinksAFormerlyLinkedDraftOverHTTP(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_497_former")
	sourceID, sourceNumber := exportForSelector(t, server, adminToken, quotationInput())
	replacementID, replacementNumber := exportForSelector(t, server, adminToken, quotationInput())
	seedStaff(t, server.database, "former-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "former-staff@example.com", "staff-password")

	code, body := createPODraft(t, server, staffToken, sourceID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create linked draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)

	users, err := server.database.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	var adminID int64
	for _, user := range users {
		if user.Role == "admin" {
			adminID = user.ID
			break
		}
	}
	if adminID < 1 {
		t.Fatal("no admin user to delete the source")
	}
	org, err := server.database.DefaultOrganization(context.Background())
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	orgContext := store.ContextWithOrg(context.Background(), org.ID)
	preview, err := server.database.PreviewDeletion(orgContext, store.DeletionPreviewInput{
		TargetType: store.DeletionTargetQuotation, TargetID: sourceID, ActorID: adminID,
	})
	if err != nil {
		t.Fatalf("preview deletion: %v", err)
	}
	if _, err := server.database.ConfirmDeletion(orgContext, store.DeletionConfirmInput{
		TargetType: store.DeletionTargetQuotation, TargetID: sourceID,
		Scope: store.DeletionScopeRecord, Reason: "issued by mistake",
		TypedNumber: sourceNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: adminID,
	}); err != nil {
		t.Fatalf("delete the source quotation: %v", err)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), staffToken, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen former draft = %d (%v)", code, body)
	}
	former := decodePurchaseOrder(t, body)
	if former.SourceState != "former" || !former.SourceMissing {
		t.Fatalf("source state = %q missing=%v; want former", former.SourceState, former.SourceMissing)
	}

	path := "/v1/purchase-orders/" + strconv.FormatInt(former.ID, 10)
	code, body = doJSON(t, server, http.MethodPost, path+"/source-link/review", staffToken,
		draftLinkReviewRequest(replacementID, 0))
	if code != http.StatusOK {
		t.Fatalf("former draft review = %d (%v)", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/source-link", staffToken,
		draftLinkRequestBody(former, replacementID, 0, nil, nil, nil, ""))
	if code != http.StatusOK {
		t.Fatalf("former draft link = %d (%v)", code, body)
	}
	linked := decodePurchaseOrder(t, body)
	if linked.SourceState != "live" || linked.QuotationID != replacementID ||
		linked.QuoteNumber != replacementNumber {
		t.Errorf("linked former draft = %+v; want replacement %d %q",
			linked, replacementID, replacementNumber)
	}
}

func TestDraftLinkRefusesASavedRecordAndAnOccupiedTargetOverHTTP(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_497_refusals")
	sourceID, _ := exportForSelector(t, server, adminToken, quotationInput())
	occupiedID, _ := exportForSelector(t, server, adminToken, quotationInput())
	seedStaff(t, server.database, "refuse-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "refuse-staff@example.com", "staff-password")

	// A saved record is a named 409, not a generic 500 or the admin 403.
	saved := savedPOOverAPI(t, server, adminToken, sourceID)
	savedPath := "/v1/purchase-orders/" + strconv.FormatInt(saved.ID, 10)
	code, body := doJSON(t, server, http.MethodPost, savedPath+"/source-link/review", staffToken,
		draftLinkReviewRequest(occupiedID, 0))
	if code != http.StatusConflict || body["code"] != "po_not_draft" {
		t.Errorf("saved review = %d %v; want 409 po_not_draft", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost, savedPath+"/source-link", staffToken,
		draftLinkRequestBody(saved, occupiedID, 0, nil, nil, nil, ""))
	if code != http.StatusConflict || body["code"] != "po_not_draft" {
		t.Errorf("saved link = %d %v; want 409 po_not_draft", code, body)
	}

	// One active record already occupies the target.
	code, body = createPODraft(t, server, staffToken, occupiedID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("occupy target = %d (%v)", code, body)
	}
	draft := prepareSourceLessDraft(t, server, staffToken, nil)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	code, body = doJSON(t, server, http.MethodPost, path+"/source-link/review", staffToken,
		draftLinkReviewRequest(occupiedID, 0))
	if code != http.StatusConflict || body["code"] != "po_target_occupied" {
		t.Errorf("occupied review = %d %v; want 409 po_target_occupied", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/source-link", staffToken,
		draftLinkRequestBody(draft, occupiedID, 0, nil, []string{"1", "2"}, nil, ""))
	if code != http.StatusConflict || body["code"] != "po_target_occupied" {
		t.Errorf("occupied link = %d %v; want 409 po_target_occupied", code, body)
	}
}

func TestDraftLinkStaleVersionAndCurrencyMismatchOverHTTP(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_497_stale_currency")
	ctx := context.Background()
	profile, err := server.database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("read company profile: %v", err)
	}

	// A USD revision, then the profile returns to IDR so the blank draft is IDR.
	profile.Currency = "USD"
	if _, err := server.database.UpdateCompanyGeneral(ctx, profile); err != nil {
		t.Fatalf("set USD company currency: %v", err)
	}
	usdSourceID, _ := exportForSelector(t, server, adminToken, quotationInput())
	profile.Currency = "IDR"
	if _, err := server.database.UpdateCompanyGeneral(ctx, profile); err != nil {
		t.Fatalf("restore IDR company currency: %v", err)
	}
	idrSourceID, _ := exportForSelector(t, server, adminToken, quotationInput())
	seedStaff(t, server.database, "stale-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "stale-staff@example.com", "staff-password")

	draft := prepareSourceLessDraft(t, server, staffToken, nil)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	// The preview reports both currencies so the UI can offer the remedy.
	code, body := doJSON(t, server, http.MethodPost, path+"/source-link/review", staffToken,
		draftLinkReviewRequest(usdSourceID, 0))
	if code != http.StatusOK {
		t.Fatalf("cross-currency review = %d (%v)", code, body)
	}
	var review draftLinkReviewBody
	encodeDecode(t, body, &review)
	if review.Order.Currency != "IDR" || review.Source.Currency != "USD" {
		t.Errorf("review currencies = %q/%q; want IDR/USD",
			review.Order.Currency, review.Source.Currency)
	}

	code, body = doJSON(t, server, http.MethodPost, path+"/source-link", staffToken,
		draftLinkRequestBody(draft, usdSourceID, 0, nil, []string{"1", "2"}, nil, ""))
	if code != http.StatusUnprocessableEntity || body["code"] != "po_currency_mismatch" {
		t.Fatalf("cross-currency link = %d %v; want 422 po_currency_mismatch", code, body)
	}

	// The remedy is editing the never-linked draft's currency, then retrying.
	code, body = doJSON(t, server, http.MethodPut, path, staffToken, map[string]any{
		"expectedVersion": draft.Version,
		"receivedDate":    "2026-10-20",
		"paymentTerms":    "30 days",
		"incoterms":       "EXW",
		"discountPercent": 10,
		"poNumber":        "PO-497-HTTP",
		"currency":        "USD",
		"items":           []map[string]any{},
	})
	if code != http.StatusOK {
		t.Fatalf("edit draft currency = %d (%v)", code, body)
	}
	edited := decodePurchaseOrder(t, body)
	if edited.Currency != "USD" {
		t.Fatalf("edited currency = %q; want USD", edited.Currency)
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/source-link", staffToken,
		draftLinkRequestBody(edited, usdSourceID, 0, nil, []string{"1", "2"}, nil, ""))
	if code != http.StatusOK {
		t.Fatalf("link after the currency edit = %d (%v)", code, body)
	}

	// A stale version is the same typed conflict a draft save produces.
	other := prepareSourceLessDraft(t, server, staffToken, nil)
	otherPath := "/v1/purchase-orders/" + strconv.FormatInt(other.ID, 10)
	staleBody := draftLinkRequestBody(other, idrSourceID, 0, nil, []string{"1", "2"}, nil, "")
	staleBody["expectedVersion"] = other.Version + 1
	code, body = doJSON(t, server, http.MethodPost, otherPath+"/source-link", staffToken, staleBody)
	if code != http.StatusConflict || body["code"] != "po_version_conflict" {
		t.Errorf("stale link = %d %v; want 409 po_version_conflict", code, body)
	}
}

func TestDraftLinkRefusesALiveDraftAndAProtectedCustomerOverHTTP(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_497_live_protected")
	liveSourceID, liveSourceNumber := exportForSelector(t, server, adminToken, quotationInput())
	targetID, _ := exportForSelector(t, server, adminToken, quotationInput())
	seedStaff(t, server.database, "live-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "live-staff@example.com", "staff-password")

	code, body := createPODraft(t, server, staffToken, liveSourceID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create live draft = %d (%v)", code, body)
	}
	live := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(live.ID, 10)

	// A draft that already follows a live source is not re-linked by this
	// slice, on either route.
	code, body = doJSON(t, server, http.MethodPost, path+"/source-link/review", staffToken,
		draftLinkReviewRequest(targetID, 0))
	if code != http.StatusConflict || body["code"] != "po_link_not_eligible" {
		t.Errorf("live draft review = %d %v; want 409 po_link_not_eligible", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/source-link", staffToken,
		draftLinkRequestBody(live, targetID, 0, nil, nil, nil, ""))
	if code != http.StatusConflict || body["code"] != "po_link_not_eligible" {
		t.Errorf("live draft link = %d %v; want 409 po_link_not_eligible", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet, path, staffToken, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen live draft = %d (%v)", code, body)
	}
	unchanged := decodePurchaseOrder(t, body)
	if unchanged.QuotationID != liveSourceID || unchanged.QuoteNumber != liveSourceNumber ||
		unchanged.SourceState != "live" {
		t.Errorf("refused re-link changed the live draft: %+v", unchanged)
	}

	// The link route refuses a client-supplied customer identity by name; the
	// handler's protected-field guard runs before the store.
	draft := prepareSourceLessDraft(t, server, staffToken, nil)
	draftPath := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	protected := draftLinkRequestBody(draft, targetID, 0, nil, []string{"1", "2"}, nil, "")
	protected["customerId"] = 2
	code, body = doJSON(t, server, http.MethodPost, draftPath+"/source-link", staffToken, protected)
	if code != http.StatusUnprocessableEntity || body["code"] != "po_protected_field" {
		t.Errorf("protected customerId link = %d %v; want 422 po_protected_field", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet, draftPath, staffToken, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen after the protected refusal = %d (%v)", code, body)
	}
	if decodePurchaseOrder(t, body).SourceState != "never" {
		t.Error("a protected-field refusal linked or moved the draft")
	}
}

func TestDraftLinkTaxChoiceOverHTTP(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_497_tax")
	input := quotationInput()
	input["TaxZone"] = "NonFTZ"
	input["TaxType"] = "Goods"
	sourceID, _ := exportForSelector(t, server, adminToken, input)
	sourceAgainID, _ := exportForSelector(t, server, adminToken, input)
	seedStaff(t, server.database, "tax-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "tax-staff@example.com", "staff-password")

	draft := prepareSourceLessDraft(t, server, staffToken, map[string]any{
		"items": []map[string]any{{
			"description":    "Manual line entered before linking",
			"quantityScaled": 10000,
			"uom":            "Pc",
			"unitPriceMinor": 100000,
		}},
	})
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	code, body := doJSON(t, server, http.MethodPost, path+"/source-link/review", staffToken,
		draftLinkReviewRequest(sourceID, 0))
	if code != http.StatusOK {
		t.Fatalf("tax review = %d (%v)", code, body)
	}
	var review draftLinkReviewBody
	encodeDecode(t, body, &review)
	if !review.TaxChoiceRequired || review.DraftTax.TaxZone != "FTZ" ||
		review.SourceTax.TaxZone != "NonFTZ" || review.SourceTax.PpnBps != 1100 {
		t.Errorf("tax review = required %v draft %q source %q ppn %d; want true FTZ NonFTZ 1100",
			review.TaxChoiceRequired, review.DraftTax.TaxZone, review.SourceTax.TaxZone,
			review.SourceTax.PpnBps)
	}

	code, body = doJSON(t, server, http.MethodPost, path+"/source-link", staffToken,
		draftLinkRequestBody(draft, sourceID, 0, nil, []string{"1", "2"}, []int{1}, ""))
	if code != http.StatusUnprocessableEntity || body["code"] != "po_tax_choice_required" {
		t.Fatalf("link without a tax choice = %d %v; want 422 po_tax_choice_required", code, body)
	}

	code, body = doJSON(t, server, http.MethodPost, path+"/source-link", staffToken,
		draftLinkRequestBody(draft, sourceID, 0, nil, []string{"1", "2"}, []int{1}, "keep"))
	if code != http.StatusOK {
		t.Fatalf("link keeping the basis = %d (%v)", code, body)
	}
	kept := decodePurchaseOrder(t, body)
	if kept.TaxZone != "FTZ" || kept.PpnBps != 0 || kept.PpnMinor != 0 {
		t.Errorf("kept basis = %q ppn %d/%d; want FTZ 0",
			kept.TaxZone, kept.PpnBps, kept.PpnMinor)
	}

	adopt := prepareSourceLessDraft(t, server, staffToken, map[string]any{
		"items": []map[string]any{{
			"description":    "Adopted-basis line",
			"quantityScaled": 10000,
			"uom":            "Pc",
			"unitPriceMinor": 100000,
		}},
	})
	adoptPath := "/v1/purchase-orders/" + strconv.FormatInt(adopt.ID, 10)
	code, body = doJSON(t, server, http.MethodPost, adoptPath+"/source-link", staffToken,
		draftLinkRequestBody(adopt, sourceAgainID, 0, nil, []string{"1", "2"}, []int{1}, "source"))
	if code != http.StatusOK {
		t.Fatalf("link adopting the basis = %d (%v)", code, body)
	}
	adopted := decodePurchaseOrder(t, body)
	if adopted.TaxZone != "NonFTZ" || adopted.PpnBps != 1100 || adopted.PpnMinor <= 0 {
		t.Errorf("adopted basis = %q ppn %d/%d; want NonFTZ 1100 and a positive amount",
			adopted.TaxZone, adopted.PpnBps, adopted.PpnMinor)
	}
}
