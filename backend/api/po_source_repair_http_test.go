package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Issue #475: the authenticated admin-only source-repair surface.
//
// These tests drive the real server and disposable Postgres. They assert the
// permission boundary at the route (staff sees 403, not an empty list), the
// selector's repair mode, the review against the candidate source, the
// required reason and version, the confirmation-number customer rule, the
// occupied-target refusal, and that an ordinary correction cannot carry the
// source link.

// repairBody builds the repair payload from a stored record: its current
// values, the target pair and the reason. Tests change only what they exercise.
func repairBody(row purchaseOrderRow, targetQuotationID int64, targetRevision int16, reason string) map[string]any {
	body := correctionBody(row, reason)
	body["targetQuotationId"] = targetQuotationID
	body["targetRevisionNumber"] = targetRevision
	body["decisions"] = []map[string]any{}
	body["acknowledgedMissing"] = []string{}
	body["acknowledgedExtra"] = []int{}
	return body
}

func reviewBody(targetQuotationID int64, targetRevision int16) map[string]any {
	return map[string]any{
		"targetQuotationId":    targetQuotationID,
		"targetRevisionNumber": targetRevision,
	}
}

type sourceRepairRow struct {
	PreviousQuotationID    int64  `json:"previousQuotationId"`
	PreviousSourceRevision int16  `json:"previousSourceRevision"`
	PreviousQuoteNumber    string `json:"previousQuoteNumber"`
	PreviousCustomerName   string `json:"previousCustomerName"`
	NewQuotationID         int64  `json:"newQuotationId"`
	NewSourceRevision      int16  `json:"newSourceRevision"`
	NewQuoteNumber         string `json:"newQuoteNumber"`
	NewCustomerName        string `json:"newCustomerName"`
	Decisions              []struct {
		Key    string `json:"key"`
		Choice string `json:"choice"`
	} `json:"decisions"`
}

// repairHistoryRow is one history entry with its repair satellite.
type repairHistoryRow struct {
	Kind           string           `json:"kind"`
	Reason         string           `json:"reason"`
	QuoteNumber    string           `json:"quoteNumber"`
	SourceRevision int16            `json:"sourceRevision"`
	Repair         *sourceRepairRow `json:"repair"`
}

func decodeRepairHistory(t *testing.T, body map[string]any) []repairHistoryRow {
	t.Helper()
	var response struct {
		History []repairHistoryRow `json:"history"`
	}
	encodeDecode(t, body, &response)
	return response.History
}

func TestPurchaseOrderSourceRepairIsAdminOnlyOverTheAPI(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_475_admin")
	seedQuotationClient(t, server.database, 4, "Other Customer")
	originalID, originalNumber := exportForSelector(t, server, adminToken, quotationInput())
	targetID, targetNumber := exportForSelector(t, server, adminToken, quotationInput())
	occupiedID, _ := exportForSelector(t, server, adminToken, quotationInput())
	saved := savedPOOverAPI(t, server, adminToken, originalID)
	// A second active record occupies an unrelated Quotation; repair mode must
	// leave it disabled, because only the record being repaired is exempt.
	createPODraft(t, server, adminToken, occupiedID, 0, nil)

	seedStaff(t, server.database, "repair-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "repair-staff@example.com", "staff-password")

	path := "/v1/purchase-orders/" + strconv.FormatInt(saved.ID, 10)

	// A staff member cannot review or perform a repair, and cannot ask the
	// selector for repair mode.
	code, body := doJSON(t, server, http.MethodPost, path+"/source-repair/review", staffToken,
		reviewBody(targetID, 0))
	if code != http.StatusForbidden {
		t.Fatalf("staff review = %d (%v); want 403", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", staffToken,
		repairBody(saved, targetID, 0, "move the link"))
	if code != http.StatusForbidden {
		t.Fatalf("staff repair = %d (%v); want 403", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/po-sources?excludePOId="+strconv.FormatInt(saved.ID, 10), staffToken, nil)
	if code != http.StatusForbidden {
		t.Fatalf("staff repair-mode selector = %d (%v); want 403", code, body)
	}

	// An admin sees the repaired record's own quotation selectable, while an
	// unrelated occupied quotation stays disabled.
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/po-sources?excludePOId="+strconv.FormatInt(saved.ID, 10), adminToken, nil)
	if code != http.StatusOK {
		t.Fatalf("admin repair-mode selector = %d (%v)", code, body)
	}
	rows := decodePOList(t, body).Quotations
	byID := map[int64]poSourceRow{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if row, ok := byID[originalID]; !ok || row.ActivePO != nil {
		t.Errorf("repaired quotation = %v; want it selectable", row.ActivePO)
	}
	if row, ok := byID[occupiedID]; !ok || row.ActivePO == nil {
		t.Errorf("unrelated occupied quotation = %v; want it still disabled", row.ActivePO)
	}

	// The review compares against the candidate revision without writing.
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair/review", adminToken,
		reviewBody(targetID, 0))
	if code != http.StatusOK {
		t.Fatalf("admin review = %d (%v)", code, body)
	}
	var review struct {
		Order struct {
			QuotationID int64 `json:"quotationId"`
		} `json:"order"`
		Source struct {
			QuotationID int64  `json:"quotationId"`
			QuoteNumber string `json:"quoteNumber"`
			Revision    int16  `json:"revisionNumber"`
		} `json:"source"`
	}
	encodeDecode(t, body, &review)
	if review.Source.QuotationID != targetID || review.Source.QuoteNumber != targetNumber {
		t.Errorf("review source = %d %q; want %d %q",
			review.Source.QuotationID, review.Source.QuoteNumber, targetID, targetNumber)
	}
	if review.Order.QuotationID != originalID {
		t.Errorf("review changed the record to %d; want it left on %d", review.Order.QuotationID, originalID)
	}

	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", adminToken,
		repairBody(saved, targetID, 0, "the order followed the other quotation"))
	if code != http.StatusOK {
		t.Fatalf("admin repair = %d (%v)", code, body)
	}
	repaired := decodePurchaseOrder(t, body)
	if repaired.QuotationID != targetID || repaired.QuoteNumber != targetNumber {
		t.Errorf("repaired source = %d %q; want %d %q",
			repaired.QuotationID, repaired.QuoteNumber, targetID, targetNumber)
	}
	if repaired.ID != saved.ID {
		t.Errorf("repair returned record %d; want the same %d", repaired.ID, saved.ID)
	}

	code, body = doJSON(t, server, http.MethodGet, path+"/history", adminToken, nil)
	if code != http.StatusOK {
		t.Fatalf("history = %d (%v)", code, body)
	}
	history := decodeRepairHistory(t, body)
	if len(history) != 1 || history[0].Kind != "source_repair" || history[0].Repair == nil {
		t.Fatalf("history = %+v; want one source_repair entry with its link", history)
	}
	if history[0].Repair.PreviousQuoteNumber != originalNumber ||
		history[0].Repair.NewQuoteNumber != targetNumber {
		t.Errorf("history link = %q -> %q; want %q -> %q",
			history[0].Repair.PreviousQuoteNumber, history[0].Repair.NewQuoteNumber,
			originalNumber, targetNumber)
	}
}

func TestPurchaseOrderSourceRepairProtectsTheConfirmationNumberOverTheAPI(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_475_confirm")
	seedQuotationClient(t, server.database, 4, "Other Customer")
	originalID, _ := exportForSelector(t, server, adminToken, quotationInput())
	sameCustomerID, _ := exportForSelector(t, server, adminToken, quotationInput())
	otherCustomerInput := quotationInput()
	otherCustomerInput["CustomerID"] = 4
	otherCustomerID, _ := exportForSelector(t, server, adminToken, otherCustomerInput)
	// A second free revision of the same other customer, because the
	// before-assignment case below occupies the first one.
	otherCustomerAgainID, _ := exportForSelector(t, server, adminToken, otherCustomerInput)
	saved := savedPOOverAPI(t, server, adminToken, originalID)

	code, body := doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10)+"/confirmation-number",
		adminToken, nil)
	if code != http.StatusOK {
		t.Fatalf("assign confirmation = %d (%v)", code, body)
	}
	assigned := decodePurchaseOrder(t, body)
	if assigned.ConfirmationNumber == nil {
		t.Fatal("confirmation number was not assigned")
	}

	// Same customer: the number survives the repair.
	code, body = doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(assigned.ID, 10)+"/source-repair", adminToken,
		repairBody(assigned, sameCustomerID, 0, "same customer, right revision"))
	if code != http.StatusOK {
		t.Fatalf("same-customer repair = %d (%v)", code, body)
	}
	kept := decodePurchaseOrder(t, body)
	if kept.ConfirmationNumber == nil || *kept.ConfirmationNumber != *assigned.ConfirmationNumber {
		t.Errorf("confirmation number = %v; want the assigned %v",
			kept.ConfirmationNumber, assigned.ConfirmationNumber)
	}

	// Before assignment, another customer is allowed. A second unassigned
	// record on the original link proves the boundary from the allowed side.
	// It carries its own customer number, because two saved records of one
	// customer may not share a PO number.
	unassigned, _ := exportForSelector(t, server, adminToken, quotationInput())
	unassignedDraft := decodePurchaseOrder(t, mustCreatePODraft(t, server, adminToken, unassigned))
	before := finalizePOOverAPI(t, server, adminToken, unassignedDraft, "PO-475-OTHER", false)
	code, body = doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(before.ID, 10)+"/source-repair", adminToken,
		repairBody(before, otherCustomerID, 0, "the order belongs to the other customer"))
	if code != http.StatusOK {
		t.Fatalf("cross-customer repair before assignment = %d (%v); want 200", code, body)
	}
	moved := decodePurchaseOrder(t, body)
	if moved.CustomerID == before.CustomerID {
		t.Fatalf("cross-customer repair kept customer %d; want it moved", before.CustomerID)
	}
	if moved.CustomerName != "PT. Datalec Global Solution" {
		t.Errorf("cross-customer customer name = %q; want the directory's PT. Datalec Global Solution", moved.CustomerName)
	}
	if moved.QuotationID != otherCustomerID {
		t.Errorf("cross-customer repair source = %d; want %d", moved.QuotationID, otherCustomerID)
	}

	// Another customer after assignment: refused, with the source unchanged.
	code, body = doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(kept.ID, 10)+"/source-repair", adminToken,
		repairBody(kept, otherCustomerAgainID, 0, "move to another customer"))
	if code != http.StatusConflict {
		t.Fatalf("cross-customer repair after assignment = %d (%v); want 409", code, body)
	}
	if body["code"] != "po_confirmation_customer_locked" {
		t.Errorf("cross-customer code = %v; want po_confirmation_customer_locked", body["code"])
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(kept.ID, 10), adminToken, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen after refused repair = %d (%v)", code, body)
	}
	unchanged := decodePurchaseOrder(t, body)
	if unchanged.QuotationID != sameCustomerID {
		t.Errorf("refused repair moved the source to %d; want it left on %d",
			unchanged.QuotationID, sameCustomerID)
	}
}

func TestPurchaseOrderSourceRepairRequiresReasonVersionAndAFreeTarget(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_475_rules")
	originalID, _ := exportForSelector(t, server, adminToken, quotationInput())
	targetID, _ := exportForSelector(t, server, adminToken, quotationInput())
	occupiedID, _ := exportForSelector(t, server, adminToken, quotationInput())
	saved := savedPOOverAPI(t, server, adminToken, originalID)
	path := "/v1/purchase-orders/" + strconv.FormatInt(saved.ID, 10)

	// A reason is required.
	code, body := doJSON(t, server, http.MethodPost, path+"/source-repair", adminToken,
		repairBody(saved, targetID, 0, "   "))
	if code != http.StatusUnprocessableEntity || body["code"] != "po_repair_reason_required" {
		t.Fatalf("missing reason = %d (%v); want 422 po_repair_reason_required", code, body)
	}

	// The loaded version is required, and a stale one is the named conflict.
	stale := repairBody(saved, targetID, 0, "stale editor")
	stale["expectedVersion"] = saved.Version - 1
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", adminToken, stale)
	if code != http.StatusConflict || body["code"] != "po_version_conflict" {
		t.Fatalf("stale repair = %d (%v); want 409 po_version_conflict", code, body)
	}

	// The editable payload still cannot carry the old source link.
	withProtected := repairBody(saved, targetID, 0, "try the protected field")
	withProtected["quotationId"] = originalID
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", adminToken, withProtected)
	if code != http.StatusUnprocessableEntity || body["code"] != "po_protected_field" {
		t.Fatalf("protected field = %d (%v); want 422 po_protected_field", code, body)
	}

	// An ordinary correction cannot move the link either: the field is refused
	// before the store is reached.
	code, body = doJSON(t, server, http.MethodPost, path+"/correction", adminToken,
		func() map[string]any {
			correction := correctionBody(saved, "ordinary correction")
			correction["quotationId"] = targetID
			return correction
		}())
	if code != http.StatusUnprocessableEntity || body["code"] != "po_protected_field" {
		t.Fatalf("correction with a link = %d (%v); want 422 po_protected_field", code, body)
	}

	// An occupied target is refused by name.
	createPODraft(t, server, adminToken, occupiedID, 0, nil)
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", adminToken,
		repairBody(saved, occupiedID, 0, "move to an occupied quotation"))
	if code != http.StatusConflict || body["code"] != "po_target_occupied" {
		t.Fatalf("occupied target = %d (%v); want 409 po_target_occupied", code, body)
	}

	// The target Quotation must still exist.
	code, body = doJSON(t, server, http.MethodPost, path+"/source-repair", adminToken,
		repairBody(saved, 9_999_999, 0, "move to a missing quotation"))
	if code != http.StatusNotFound {
		t.Fatalf("missing target = %d (%v); want 404", code, body)
	}
}

func TestPurchaseOrderSourceRepairOfAMissingRecordIsNotFound(t *testing.T) {
	server, adminToken := testQuotationServer(t, "vera_api_po_475_missing")
	originalID, _ := exportForSelector(t, server, adminToken, quotationInput())
	targetID, _ := exportForSelector(t, server, adminToken, quotationInput())
	saved := savedPOOverAPI(t, server, adminToken, originalID)
	purchaseOrderTestExec(t, "vera_api_po_475_missing",
		fmt.Sprintf(`DELETE FROM %s.purchase_orders WHERE id = $1`,
			quoteIdentForTest("vera_api_po_475_missing")), saved.ID)

	code, body := doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10)+"/source-repair", adminToken,
		repairBody(saved, targetID, 0, "the record is gone"))
	if code != http.StatusNotFound {
		t.Fatalf("repair of a deleted record = %d (%v); want 404", code, body)
	}
	if count := countPurchaseOrderRows(t, "vera_api_po_475_missing", saved.ID); count != 0 {
		t.Errorf("the API recreated the deleted record (%d rows)", count)
	}
}

// countPurchaseOrderRows answers whether a deleted record came back under the
// same identity. Nothing in this slice ever inserts from a write path that took
// an id, so a non-zero count would be a regression, not a fixture effect.
func countPurchaseOrderRows(t *testing.T, schema string, id int64) int {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to count rows: %v", err)
	}
	defer pool.Close()
	var count int
	if err := pool.QueryRow(context.Background(), fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.purchase_orders WHERE id = $1`,
		quoteIdentForTest(schema)), id).Scan(&count); err != nil {
		t.Fatalf("count purchase order rows: %v", err)
	}
	return count
}
