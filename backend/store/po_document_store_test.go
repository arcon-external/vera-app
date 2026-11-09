package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Issue #472: one optional customer PO PDF, versioned, organization-scoped and
// deliberately removable. These tests drive the real store against a
// disposable schema: exact bytes, prior-version retention, one active document
// under a real concurrent race, the database's own active-pointer index, and
// removal metadata that survives as history.

func attachTestDocument(t *testing.T, database *Store, ctx context.Context, purchaseOrderID int64, user AuthUser, data []byte) PurchaseOrderDocument {
	t.Helper()
	document, err := database.AttachPurchaseOrderDocument(ctx, PurchaseOrderDocumentUpload{
		PurchaseOrderID: purchaseOrderID,
		Data:            data,
		Filename:        "customer-po.pdf",
		UploadedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("attach document: %v", err)
	}
	return document
}

func documentSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func documentTableCount(t *testing.T, database *Store, purchaseOrderID int64) int {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.purchase_order_documents WHERE purchase_order_id = $1",
		database.configSchema()), purchaseOrderID).Scan(&count); err != nil {
		t.Fatalf("count documents: %v", err)
	}
	return count
}

func TestAttachPurchaseOrderDocumentStoresExactBytesAndMetadata(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_attach")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Attach")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	data := minimalPDF(nil, "")
	document, err := database.AttachPurchaseOrderDocument(ctx, PurchaseOrderDocumentUpload{
		PurchaseOrderID: draft.ID,
		Data:            data,
		Filename:        `C:\incoming\Customer PO #17.pdf`,
		UploadedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if document.Version != 1 || !document.Active {
		t.Errorf("version/active = %d/%v; want 1/true", document.Version, document.Active)
	}
	if document.SizeBytes != int64(len(data)) {
		t.Errorf("size = %d; want %d", document.SizeBytes, len(data))
	}
	if document.SHA256 != documentSHA(data) {
		t.Errorf("sha = %q; want %q", document.SHA256, documentSHA(data))
	}
	if document.ContentType != "application/pdf" {
		t.Errorf("content type = %q; want application/pdf", document.ContentType)
	}
	if document.OriginalFilename != "Customer PO #17.pdf" {
		t.Errorf("filename = %q; want the base name only", document.OriginalFilename)
	}
	if document.UploadedBy != user.ID || document.UploadedAt.IsZero() {
		t.Errorf("uploader/time = %d/%v; want %d and a time", document.UploadedBy, document.UploadedAt, user.ID)
	}

	loaded, err := database.GetActivePurchaseOrderDocument(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load active: %v", err)
	}
	if !bytes.Equal(loaded.Data, data) {
		t.Error("stored bytes differ from the uploaded bytes")
	}
	if loaded.SHA256 != document.SHA256 {
		t.Error("stored sha differs from the attach response")
	}
}

func TestAttachPurchaseOrderDocumentRefusesInvalidEncryptedAndOversized(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_validate")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Validate")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"garbage", []byte("definitely not a pdf"), ErrPurchaseOrderDocumentInvalid},
		{"encrypted", minimalPDF(nil, "/Encrypt 9 0 R"), ErrPurchaseOrderDocumentEncrypted},
		{"oversized", bytes.Repeat([]byte("x"), int(MaxPurchaseOrderDocumentBytes)+1), ErrPurchaseOrderDocumentTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := database.AttachPurchaseOrderDocument(ctx, PurchaseOrderDocumentUpload{
				PurchaseOrderID: draft.ID,
				Data:            tc.data,
				Filename:        "x.pdf",
				UploadedBy:      user.ID,
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v; want %v", err, tc.want)
			}
			if count := documentTableCount(t, database, draft.ID); count != 0 {
				t.Fatalf("refused upload stored %d rows", count)
			}
		})
	}
}

func TestReplacingAPurchaseOrderDocumentRetainsThePriorVersion(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_replace")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Replace")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	firstData := minimalPDF(nil, "")
	secondData := minimalPDF([]string{"3 0 obj\n<< /Type /Metadata >>\nendobj\n"}, "")
	first := attachTestDocument(t, database, ctx, draft.ID, user, firstData)
	second := attachTestDocument(t, database, ctx, draft.ID, user, secondData)

	if second.Version != 2 || !second.Active {
		t.Fatalf("second version = %d active=%v; want 2/true", second.Version, second.Active)
	}
	state, err := database.GetPurchaseOrderDocumentState(ctx, draft.ID)
	if err != nil {
		t.Fatalf("document state: %v", err)
	}
	if state.Active == nil || state.Active.Version != 2 {
		t.Fatalf("active = %+v; want version 2", state.Active)
	}
	if len(state.Versions) != 2 {
		t.Fatalf("history = %d; want both versions", len(state.Versions))
	}
	if state.Versions[0].Version != 2 || state.Versions[1].Version != 1 {
		t.Errorf("history order = %d,%d; want newest first", state.Versions[0].Version, state.Versions[1].Version)
	}
	if state.Versions[1].Active || state.Versions[1].ReplacedAt == nil {
		t.Errorf("prior version was not marked replaced: %+v", state.Versions[1])
	}

	old, err := database.GetPurchaseOrderDocumentVersion(ctx, draft.ID, int32(first.Version))
	if err != nil {
		t.Fatalf("load version 1: %v", err)
	}
	if !bytes.Equal(old.Data, firstData) {
		t.Error("version 1 bytes changed when version 2 was attached")
	}
	active, err := database.GetActivePurchaseOrderDocument(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load active: %v", err)
	}
	if !bytes.Equal(active.Data, secondData) {
		t.Error("active bytes are not the replacement")
	}
}

func TestAttachPurchaseOrderDocumentKeepsOneActiveUnderARealRace(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_race")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Race")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	// Eight uploads rather than two: a single lucky interleaving must not be
	// able to make an unserialized version allocator look correct.
	const racers = 8
	var wait sync.WaitGroup
	results := make([]PurchaseOrderDocument, racers)
	errorsSeen := make([]error, racers)
	for index := 0; index < racers; index++ {
		data := minimalPDF([]string{fmt.Sprintf(
			"3 0 obj\n<< /Type /Metadata /Label (race-%d) >>\nendobj\n", index)}, "")
		wait.Add(1)
		go func(index int, data []byte) {
			defer wait.Done()
			results[index], errorsSeen[index] = database.AttachPurchaseOrderDocument(ctx, PurchaseOrderDocumentUpload{
				PurchaseOrderID: draft.ID,
				Data:            data,
				Filename:        fmt.Sprintf("race-%d.pdf", index),
				UploadedBy:      user.ID,
			})
		}(index, data)
	}
	wait.Wait()
	for index, err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent attach %d: %v", index, err)
		}
	}
	versionsSeen := make(map[int32]bool, racers)
	for _, result := range results {
		if versionsSeen[result.Version] {
			t.Fatalf("version %d was allocated twice", result.Version)
		}
		versionsSeen[result.Version] = true
	}
	state, err := database.GetPurchaseOrderDocumentState(ctx, draft.ID)
	if err != nil {
		t.Fatalf("document state: %v", err)
	}
	if state.Active == nil {
		t.Fatal("the race left no active document")
	}
	activeCount := 0
	for _, version := range state.Versions {
		if version.Active {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("active documents = %d; want exactly 1", activeCount)
	}
	if len(state.Versions) != racers {
		t.Fatalf("versions = %d; want all %d concurrent uploads retained", len(state.Versions), racers)
	}

	// The database is the final guard: even code that forgot to swap the
	// pointer cannot store a second active row. Break the Go path deliberately
	// and prove the index refuses it.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	_, err = pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_documents (
			org_id, purchase_order_id, version, pdf_data, sha256, size_bytes,
			content_type, original_filename, uploaded_by, active
		) VALUES (
			(SELECT org_id FROM %s.purchase_orders WHERE id = $1),
			$1, 99, $2, $3, $4, 'application/pdf', 'forged.pdf', $5, TRUE
		)`, database.configSchema(), database.configSchema()),
		draft.ID, []byte("%PDF-1.7"), documentSHA([]byte("%PDF-1.7")), 8, user.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" ||
		pgErr.ConstraintName != "purchase_order_documents_active_idx" {
		t.Fatalf("second active insert error = %v; want the active-pointer unique violation", err)
	}
}

func TestRemovingThePurchaseOrderDocumentRetainsHistoryAndNeedsConfirmation(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_remove")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Remove")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	document := attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))

	if _, err := database.RemovePurchaseOrderDocument(ctx, PurchaseOrderDocumentRemoval{
		PurchaseOrderID: draft.ID,
		RemovedBy:       user.ID,
	}); !errors.Is(err, ErrPurchaseOrderDocumentRemovalNotConfirmed) {
		t.Fatalf("unconfirmed removal = %v; want ErrPurchaseOrderDocumentRemovalNotConfirmed", err)
	}
	if _, err := database.GetActivePurchaseOrderDocument(ctx, draft.ID); err != nil {
		t.Fatalf("unconfirmed removal removed the document: %v", err)
	}

	removed, err := database.RemovePurchaseOrderDocument(ctx, PurchaseOrderDocumentRemoval{
		PurchaseOrderID: draft.ID,
		RemovedBy:       user.ID,
		Confirmed:       true,
	})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed.Active || removed.RemovedAt == nil || removed.RemovedBy == nil || *removed.RemovedBy != user.ID {
		t.Errorf("removal metadata = active %v at %v by %v", removed.Active, removed.RemovedAt, removed.RemovedBy)
	}
	if _, err := database.GetActivePurchaseOrderDocument(ctx, draft.ID); !errors.Is(err, ErrPurchaseOrderDocumentNotFound) {
		t.Fatalf("active after removal = %v; want ErrPurchaseOrderDocumentNotFound", err)
	}
	history, err := database.GetPurchaseOrderDocumentVersion(ctx, draft.ID, int32(document.Version))
	if err != nil {
		t.Fatalf("removed version is not retrievable: %v", err)
	}
	if history.Active || history.RemovedAt == nil || !bytes.Equal(history.Data, document.Data) {
		t.Errorf("retained history = %+v", history)
	}
	if _, err := database.RemovePurchaseOrderDocument(ctx, PurchaseOrderDocumentRemoval{
		PurchaseOrderID: draft.ID, RemovedBy: user.ID, Confirmed: true,
	}); !errors.Is(err, ErrPurchaseOrderDocumentNotFound) {
		t.Fatalf("second removal = %v; want ErrPurchaseOrderDocumentNotFound", err)
	}
}

func TestPurchaseOrderDocumentsAreScopedToTheirOrganization(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_org")
	firstQuotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer A", "Org A")
	secondQuotation := seedFinalizeQuotation(t, database, 3, "PT. Buyer B", "Org B")
	user := poTestUser(t, database)
	ctxA := poContext(t, database)
	draftA := createFinalizeDraft(t, database, ctxA, user, firstQuotation)
	attachTestDocument(t, database, ctxA, draftA.ID, user, minimalPDF(nil, ""))

	// A second organization is a test fixture, not a product feature; the
	// product seeds exactly one until tenancy grows. It exists here to prove
	// that an org-scoped read cannot cross the boundary.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var orgB string
	if err := pool.QueryRow(context.Background(), fmt.Sprintf(
		"INSERT INTO %s.organizations (name) VALUES ('Second Org') RETURNING id::text",
		database.configSchema())).Scan(&orgB); err != nil {
		t.Fatalf("create second org: %v", err)
	}
	ctxB := ContextWithOrg(context.Background(), orgB)
	draftB, err := database.CreatePurchaseOrderDraft(ctxB, PurchaseOrderCreate{
		QuotationID:    secondQuotation.ID,
		RevisionNumber: secondQuotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create org B draft: %v", err)
	}
	documentB := attachTestDocument(t, database, ctxB, draftB.ID, user, minimalPDF(nil, ""))

	if _, err := database.GetActivePurchaseOrderDocument(ctxA, draftB.ID); !errors.Is(err, ErrPurchaseOrderDocumentNotFound) {
		t.Fatalf("org A read org B document = %v; want not found", err)
	}
	if _, err := database.GetPurchaseOrderDocumentState(ctxA, draftB.ID); !errors.Is(err, ErrPurchaseOrderDocumentNotFound) {
		t.Fatalf("org A read org B state = %v; want not found", err)
	}
	if _, err := database.GetPurchaseOrderDocumentVersion(ctxA, draftB.ID, int32(documentB.Version)); !errors.Is(err, ErrPurchaseOrderDocumentNotFound) {
		t.Fatalf("org A read org B version = %v; want not found", err)
	}
	if _, err := database.RemovePurchaseOrderDocument(ctxA, PurchaseOrderDocumentRemoval{
		PurchaseOrderID: draftB.ID, RemovedBy: user.ID, Confirmed: true,
	}); !errors.Is(err, ErrPurchaseOrderDocumentNotFound) {
		t.Fatalf("org A removed org B document = %v; want not found", err)
	}
	// And the reverse direction: org B cannot reach org A's document, which
	// would otherwise be reachable by guessing a sequential id.
	if _, err := database.GetActivePurchaseOrderDocument(ctxB, draftA.ID); !errors.Is(err, ErrPurchaseOrderDocumentNotFound) {
		t.Fatalf("org B read org A document = %v; want not found", err)
	}
	stateB, err := database.GetPurchaseOrderDocumentState(ctxB, draftB.ID)
	if err != nil || stateB.Active == nil || stateB.Active.Version != documentB.Version {
		t.Fatalf("org B own state = %+v, %v", stateB, err)
	}
}

func TestAttachingAndReplacingLeavesPurchaseOrderFieldsUntouched(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_fields")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Fields")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	before, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load before: %v", err)
	}
	attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))
	attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF([]string{"3 0 obj\n<< /Type /Metadata >>\nendobj\n"}, ""))
	after, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("attaching changed purchase order fields:\nbefore %+v\nafter  %+v", before, after)
	}
}

func TestFinalizePurchaseOrderWritesAbsenceFieldsOnlyWhenThereIsNoPDF(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_absence")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Absence")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	absence := func(purchaseOrderID int64) (bool, bool) {
		t.Helper()
		pool, err := database.poolOrError()
		if err != nil {
			t.Fatalf("pool: %v", err)
		}
		var at, by bool
		if err := pool.QueryRow(ctx, fmt.Sprintf(
			`SELECT pdf_absent_confirmed_at IS NOT NULL, pdf_absent_confirmed_by IS NOT NULL
			 FROM %s.purchase_orders WHERE id = $1`, database.configSchema()),
			purchaseOrderID).Scan(&at, &by); err != nil {
			t.Fatalf("read absence fields: %v", err)
		}
		return at, by
	}

	t.Run("attached save needs no confirmation and writes no absence fields", func(t *testing.T) {
		draft := createFinalizeDraft(t, database, ctx, user, quotation)
		attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))
		input := finalizeInput(draft, user, "PO-ATTACHED")
		input.DocumentConfirmed = false
		saved, err := database.FinalizePurchaseOrder(ctx, input)
		if err != nil {
			t.Fatalf("finalize attached: %v", err)
		}
		if saved.PDFAbsentConfirmed {
			t.Error("attached save recorded a no-PDF confirmation")
		}
		if at, by := absence(draft.ID); at || by {
			t.Errorf("attached save wrote absence fields: at=%v by=%v", at, by)
		}
	})

	t.Run("an attached save clears a stray confirmation", func(t *testing.T) {
		secondQuotation := seedFinalizeQuotation(t, database, 4, "PT. Buyer 2", "Stray confirmation")
		draft := createFinalizeDraft(t, database, ctx, user, secondQuotation)
		attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))
		input := finalizeInput(draft, user, "PO-ATTACHED-STRAY")
		// A client that sends the checkbox anyway must not turn an attached
		// save into a recorded absence.
		input.DocumentConfirmed = true
		saved, err := database.FinalizePurchaseOrder(ctx, input)
		if err != nil {
			t.Fatalf("finalize attached: %v", err)
		}
		if saved.PDFAbsentConfirmed {
			t.Error("attached save recorded a no-PDF confirmation")
		}
		if at, by := absence(draft.ID); at || by {
			t.Errorf("attached save wrote absence fields: at=%v by=%v", at, by)
		}
	})

	t.Run("blank save writes both absence fields", func(t *testing.T) {
		secondQuotation := seedFinalizeQuotation(t, database, 5, "PT. Buyer 3", "Absence 2")
		draft := createFinalizeDraft(t, database, ctx, user, secondQuotation)
		input := finalizeInput(draft, user, "PO-NO-PDF")
		input.DocumentConfirmed = false
		if _, err := database.FinalizePurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderPDFConfirmationRequired) {
			t.Fatalf("unconfirmed blank save = %v; want confirmation required", err)
		}
		if at, by := absence(draft.ID); at || by {
			t.Errorf("refused save wrote absence fields: at=%v by=%v", at, by)
		}
		input.DocumentConfirmed = true
		saved, err := database.FinalizePurchaseOrder(ctx, input)
		if err != nil {
			t.Fatalf("finalize blank: %v", err)
		}
		if !saved.PDFAbsentConfirmed {
			t.Error("the confirmed blank save did not record the absence")
		}
		if at, by := absence(draft.ID); !at || !by {
			t.Errorf("blank save absence fields = at %v by %v; want both set", at, by)
		}
	})
}

func TestGetPurchaseOrderReviewReportsTheDocumentState(t *testing.T) {
	database := openTestSchema(t, "vera_po_document_review_state")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Review state")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	review, err := database.GetPurchaseOrderReview(ctx, draft.ID)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if review.DocumentState != PurchaseOrderDocumentNone {
		t.Errorf("document state = %q; want none", review.DocumentState)
	}
	attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))
	review, err = database.GetPurchaseOrderReview(ctx, draft.ID)
	if err != nil {
		t.Fatalf("review after attach: %v", err)
	}
	if review.DocumentState != PurchaseOrderDocumentAttached {
		t.Errorf("document state = %q; want attached", review.DocumentState)
	}
	if _, err := database.RemovePurchaseOrderDocument(ctx, PurchaseOrderDocumentRemoval{
		PurchaseOrderID: draft.ID, RemovedBy: user.ID, Confirmed: true,
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	review, err = database.GetPurchaseOrderReview(ctx, draft.ID)
	if err != nil {
		t.Fatalf("review after removal: %v", err)
	}
	if review.DocumentState != PurchaseOrderDocumentNone {
		t.Errorf("document state after removal = %q; want none", review.DocumentState)
	}
}
