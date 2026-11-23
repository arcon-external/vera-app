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

// Issue #652: new revisions are row-only, and a purchase order may not use a
// revision whose stored whole-order discount is nonzero. These tests drive the
// real API and a disposable schema because 409 is the contract staff see: the
// refusal must arrive at create, draft-link, repair and finalize, and a later
// row-only revision must open the same quotation again.

// setQuotationRevisionHeaderDiscountForTest writes the pre-#652 shape directly.
// New revisions refuse a whole-order discount, so only a fixture can create the
// stored history the guard protects.
func setQuotationRevisionHeaderDiscountForTest(
	t *testing.T, schema string, quotationID int64, revision, percent int16,
) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect for legacy header discount: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(`
		UPDATE %s.quotation_revisions
		SET discount_percent = $3
		WHERE quotation_id = $1 AND revision_number = $2`,
		quoteIdentForTest(schema)), quotationID, revision, percent); err != nil {
		t.Fatalf("set revision %d/%d whole-order discount: %v", quotationID, revision, err)
	}
}

func TestPurchaseOrderRefusesALegacyHeaderDiscountRevision(t *testing.T) {
	const schema = "vera_api_po_legacy_header"
	server, token := testQuotationServer(t, schema)

	legacyID, legacyNumber := exportForSelector(t, server, token,
		poSourceInput("Legacy header", "Legacy line"))
	setQuotationRevisionHeaderDiscountForTest(t, schema, legacyID, 0, 10)

	// Reading is never blocked: the stored revision keeps its printed value.
	code, body := doJSON(t, server, http.MethodGet,
		"/v1/quotations/"+strconv.FormatInt(legacyID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("read the legacy revision = %d (%v); want 200", code, body)
	}
	if body["discountPercent"] != 10.0 {
		t.Errorf("stored discount = %v; want the historical 10", body["discountPercent"])
	}

	// Create against the legacy revision is the typed 409.
	code, body = createPODraft(t, server, token, legacyID, 0, nil)
	if code != http.StatusConflict || body["code"] != "po_legacy_header_discount" {
		t.Fatalf("create on a legacy header revision = %d (%v); want 409/po_legacy_header_discount",
			code, body)
	}

	// A later row-only revision of the same quotation reopens the path: it is
	// the revision being ordered, never the quotation's history, that decides.
	exportNextRevision(t, server, token, legacyNumber, 1,
		poSourceInput("Legacy header", "Legacy line"))
	code, body = createPODraft(t, server, token, legacyID, 1, nil)
	if code != http.StatusCreated {
		t.Fatalf("create on the row-only revision 01 = %d (%v); want 201", code, body)
	}
	rowOnlyDraft := decodePurchaseOrder(t, body)

	// Finalize refuses a source that became legacy after the draft existed.
	setQuotationRevisionHeaderDiscountForTest(t, schema, legacyID, 1, 10)
	code, body = doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(rowOnlyDraft.ID, 10)+"/finalize", token,
		finalizeBody(rowOnlyDraft, "PO-652-LEGACY"))
	if code != http.StatusConflict || body["code"] != "po_legacy_header_discount" {
		t.Fatalf("finalize on a legacy header revision = %d (%v); want 409/po_legacy_header_discount",
			code, body)
	}

	// Draft link: a blank draft may point at no source, so a legacy target is
	// refused at both the preview and the commit rather than only at creation.
	code, body = createBlankPODraft(t, server, token, nil)
	if code != http.StatusCreated {
		t.Fatalf("blank create = %d (%v); want 201", code, body)
	}
	blank := decodePurchaseOrder(t, body)
	linkPath := "/v1/purchase-orders/" + strconv.FormatInt(blank.ID, 10) + "/source-link"
	linkTarget := map[string]any{
		"targetQuotationId":    legacyID,
		"targetRevisionNumber": 0,
	}
	code, body = doJSON(t, server, http.MethodPost, linkPath+"/review", token, linkTarget)
	if code != http.StatusConflict || body["code"] != "po_legacy_header_discount" {
		t.Fatalf("link preview on a legacy header revision = %d (%v); want 409/po_legacy_header_discount",
			code, body)
	}
	linkTarget["expectedVersion"] = blank.Version
	code, body = doJSON(t, server, http.MethodPost, linkPath, token, linkTarget)
	if code != http.StatusConflict || body["code"] != "po_legacy_header_discount" {
		t.Fatalf("link commit on a legacy header revision = %d (%v); want 409/po_legacy_header_discount",
			code, body)
	}

	// Repair: a saved record is the admin move case. Its review and its commit
	// both refuse the legacy target before any difference choice is asked for.
	rowOnlyID, _ := exportForSelector(t, server, token,
		poSourceInput("Repair row only", "Repair line"))
	code, body = createPODraft(t, server, token, rowOnlyID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create for repair base = %d (%v); want 201", code, body)
	}
	repairDraft := decodePurchaseOrder(t, body)
	code, body = doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(repairDraft.ID, 10)+"/finalize", token,
		finalizeBody(repairDraft, "PO-652-REPAIR"))
	if code != http.StatusOK {
		t.Fatalf("finalize the repair base = %d (%v); want 200", code, body)
	}
	saved := decodePurchaseOrder(t, body)

	repairPath := "/v1/purchase-orders/" + strconv.FormatInt(saved.ID, 10) + "/source-repair"
	repairTarget := map[string]any{
		"targetQuotationId":    legacyID,
		"targetRevisionNumber": 0,
	}
	code, body = doJSON(t, server, http.MethodPost, repairPath+"/review", token, repairTarget)
	if code != http.StatusConflict || body["code"] != "po_legacy_header_discount" {
		t.Fatalf("repair preview on a legacy header revision = %d (%v); want 409/po_legacy_header_discount",
			code, body)
	}
	repairBody := finalizeBody(saved, "PO-652-REPAIR")
	repairBody["targetQuotationId"] = legacyID
	repairBody["targetRevisionNumber"] = 0
	repairBody["reason"] = "Move to the reissued source"
	code, body = doJSON(t, server, http.MethodPost, repairPath, token, repairBody)
	if code != http.StatusConflict || body["code"] != "po_legacy_header_discount" {
		t.Fatalf("repair commit on a legacy header revision = %d (%v); want 409/po_legacy_header_discount",
			code, body)
	}
}
