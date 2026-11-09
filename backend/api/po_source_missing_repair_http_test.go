package api

import (
	"net/http"
	"strconv"
	"testing"
)

// Issue #481: finding and repairing a saved Purchase Order whose source
// Quotation was permanently deleted.
//
// The deletion, list filter, review and repair machinery all existed across
// #475 and #480. These tests prove they compose over the authenticated HTTP
// surface for the source-missing state: the archive finds the survivor, the
// review accepts it, staff are refused with the typed code, and the admin
// repair keeps the record's own values and assigned confirmation number while
// the eligibility rules still refuse an occupied target, a stale version and a
// cross-customer move after assignment.

// sourceMissingPOOverAPI creates one saved PO with an assigned confirmation
// number, then permanently deletes its Quotation record so the survivor is
// source missing. It returns the former quotation number and the surviving row.
func sourceMissingPOOverAPI(t *testing.T, server *Server, token string) (string, purchaseOrderRow) {
	t.Helper()
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)

	code, body := doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10)+"/confirmation-number", token, nil)
	if code != http.StatusOK {
		t.Fatalf("assign confirmation = %d (%v)", code, body)
	}
	assigned := decodePurchaseOrder(t, body)
	if assigned.ConfirmationNumber == nil {
		t.Fatal("confirmation number was not assigned")
	}

	preview := previewDeletionOverAPI(t, server, token, "quotation", quotationID)
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "record",
		"reason": "issued by mistake", "typedNumber": quoteNumber,
		"expectedFingerprint": preview.Fingerprint,
	})
	if code != http.StatusOK {
		t.Fatalf("delete quotation = %d (%v)", code, body)
	}

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("read the survivor = %d (%v)", code, body)
	}
	surviving := decodePurchaseOrder(t, body)
	if !surviving.SourceMissing {
		t.Fatal("the surviving purchase order is not source missing")
	}
	if surviving.ConfirmationNumber == nil ||
		*surviving.ConfirmationNumber != *assigned.ConfirmationNumber {
		t.Fatalf("survivor confirmation = %v; want %v",
			surviving.ConfirmationNumber, assigned.ConfirmationNumber)
	}
	return quoteNumber, surviving
}

func TestSourceMissingPurchaseOrderIsFoundAndRepairedOverTheAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_481_repair")
	formerNumber, surviving := sourceMissingPOOverAPI(t, server, token)

	// The archive finds the survivor by the former number and by the
	// independent source-missing predicate.
	code, body := doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders?search="+formerNumber, token, nil)
	if code != http.StatusOK {
		t.Fatalf("search by former number = %d (%v)", code, body)
	}
	found := listPurchaseOrders(t, server, token, "?search="+formerNumber).PurchaseOrders
	if len(found) != 1 || found[0].ID != surviving.ID || !found[0].SourceMissing {
		t.Fatalf("search by former number = %+v; want the survivor", found)
	}
	only := listPurchaseOrders(t, server, token, "?sourceMissing=only").PurchaseOrders
	if len(only) != 1 || only[0].ID != surviving.ID {
		t.Fatalf("sourceMissing=only = %+v; want the survivor", only)
	}

	// Staff may read the record but cannot enter the repair, and the refusal is
	// the typed code the UI binds to rather than a bare 403.
	seedStaff(t, server.database, "source-missing-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "source-missing-staff@example.com", "staff-password")
	path := "/v1/purchase-orders/" + strconv.FormatInt(surviving.ID, 10)

	// Targets: one occupied by another active PO, one for another customer,
	// and one free same-customer revision.
	occupiedTarget, _ := exportForSelector(t, server, token, quotationInput())
	createPODraft(t, server, token, occupiedTarget, 0, nil)
	otherInput := quotationInput()
	otherInput["CustomerID"] = 3
	otherInput["Subject"] = "Other customer source"
	otherTarget, _ := exportForSelector(t, server, token, otherInput)
	freeTarget, _ := exportForSelector(t, server, token, quotationInput())

	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair/review", staffToken,
		reviewBody(freeTarget, 0))
	if code != http.StatusForbidden || body["code"] != "po_repair_forbidden" {
		t.Fatalf("staff review = %d (%v); want 403/po_repair_forbidden", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", staffToken,
		repairBody(surviving, freeTarget, 0, "staff must not repair"))
	if code != http.StatusForbidden || body["code"] != "po_repair_forbidden" {
		t.Fatalf("staff repair = %d (%v); want 403/po_repair_forbidden", code, body)
	}

	// Admin review sees the missing state and the candidate source.
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair/review", token,
		reviewBody(freeTarget, 0))
	if code != http.StatusOK {
		t.Fatalf("admin review = %d (%v)", code, body)
	}
	var review struct {
		Order struct {
			SourceMissing bool `json:"sourceMissing"`
		} `json:"order"`
		Source struct {
			QuotationID int64 `json:"quotationId"`
		} `json:"source"`
	}
	encodeDecode(t, body, &review)
	if !review.Order.SourceMissing {
		t.Error("the review lost the missing-source state")
	}
	if review.Source.QuotationID != freeTarget {
		t.Errorf("review source = %d; want %d", review.Source.QuotationID, freeTarget)
	}

	// A stale version is a named conflict, not an overwrite.
	stale := repairBody(surviving, freeTarget, 0, "stale editor")
	stale["expectedVersion"] = surviving.Version - 1
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", token, stale)
	if code != http.StatusConflict || body["code"] != "po_version_conflict" {
		t.Fatalf("stale repair = %d (%v); want 409/po_version_conflict", code, body)
	}

	// An occupied target is refused and the record stays missing.
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", token,
		repairBody(surviving, occupiedTarget, 0, "target already occupied"))
	if code != http.StatusConflict || body["code"] != "po_target_occupied" {
		t.Fatalf("occupied target = %d (%v); want 409/po_target_occupied", code, body)
	}

	// A different customer is refused after the confirmation number exists.
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", token,
		repairBody(surviving, otherTarget, 0, "move to another customer"))
	if code != http.StatusConflict || body["code"] != "po_confirmation_customer_locked" {
		t.Fatalf("cross-customer repair = %d (%v); want 409/po_confirmation_customer_locked", code, body)
	}

	// The same-customer link succeeds and keeps every protected value.
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", token,
		repairBody(surviving, freeTarget, 0, "the source quotation was deleted"))
	if code != http.StatusOK {
		t.Fatalf("admin repair = %d (%v)", code, body)
	}
	repaired := decodePurchaseOrder(t, body)
	if repaired.SourceMissing {
		t.Error("the repaired record still claims a missing source")
	}
	if repaired.QuotationID != freeTarget || repaired.QuoteNumber == "" {
		t.Errorf("repaired source = %d/%q; want %d", repaired.QuotationID, repaired.QuoteNumber, freeTarget)
	}
	if repaired.QuoteNumber == formerNumber {
		t.Error("the repaired record still carries the former quote number as current")
	}
	if repaired.ConfirmationNumber == nil ||
		*repaired.ConfirmationNumber != *surviving.ConfirmationNumber {
		t.Errorf("repair changed the confirmation number %v -> %v",
			surviving.ConfirmationNumber, repaired.ConfirmationNumber)
	}
	if repaired.PONumber != surviving.PONumber ||
		repaired.GrandTotalMinor != surviving.GrandTotalMinor ||
		len(repaired.Items) != len(surviving.Items) {
		t.Errorf("repair did not keep the record's own values: %+v", repaired)
	}

	// The history names the deleted former source and the new one.
	code, body = doJSON(t, server, http.MethodGet, path+"/history", token, nil)
	if code != http.StatusOK {
		t.Fatalf("history = %d (%v)", code, body)
	}
	history := decodeRepairHistory(t, body)
	if len(history) != 1 || history[0].Kind != "source_repair" || history[0].Repair == nil {
		t.Fatalf("history = %+v; want one source repair", history)
	}
	if history[0].Repair.PreviousQuoteNumber != formerNumber {
		t.Errorf("history previous source = %q; want the deleted %q",
			history[0].Repair.PreviousQuoteNumber, formerNumber)
	}
	if history[0].Repair.NewQuotationID != freeTarget {
		t.Errorf("history new source = %d; want %d",
			history[0].Repair.NewQuotationID, freeTarget)
	}

	// The filter follows the repaired state: no source-missing rows remain.
	if rows := listPurchaseOrders(t, server, token, "?sourceMissing=only").PurchaseOrders; len(rows) != 0 {
		t.Errorf("sourceMissing=only after repair = %+v; want none", rows)
	}
}
