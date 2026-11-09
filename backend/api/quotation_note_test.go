package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPrepareEndpointCarriesTheQuotationNote pins the prepare wire: the note
// is accepted, trimmed exactly as Prepare trims it, and returned on the
// customer preview so the review screen shows the text that will print.
func TestPrepareEndpointCarriesTheQuotationNote(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_note_prepare")

	input := quotationInput()
	input["Note"] = "  Delivery is ex-works.\nInstallation by the customer.  "
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/prepare", token, input)
	if code != http.StatusOK {
		t.Fatalf("prepare = %d; want 200 (%v)", code, body)
	}
	customer, _ := body["customer"].(map[string]any)
	const want = "Delivery is ex-works.\nInstallation by the customer."
	if got, _ := customer["Note"].(string); got != want {
		t.Errorf("customer preview note = %q; want %q", got, want)
	}
}

// TestPrepareEndpointRejectsANoteOverTheLimit pins the validation error the
// desktop and CLI surface: 1001 characters is a 422 naming the limit, not a
// database failure later in the export.
func TestPrepareEndpointRejectsANoteOverTheLimit(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_note_limit")

	input := quotationInput()
	input["Note"] = strings.Repeat("a", 1001)
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/prepare", token, input)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("over-limit prepare = %d; want 422 (%v)", code, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, "1000") {
		t.Errorf("over-limit error = %q; want it to name the 1000-character limit", message)
	}
}

// TestExportStoresSignsAndReturnsTheQuotationNote is the end-to-end note
// proof at the API seam:
//
//   - export accepts the note, stores it on the immutable revision, and the
//     detail read returns it;
//   - the evidence read returns the same note, so the printed code verifies;
//   - editing the stored note after issue breaks verification.
func TestExportStoresSignsAndReturnsTheQuotationNote(t *testing.T) {
	const schema = "vera_api_quote_note_export"
	resetTestSchema(t, schema)
	server, token := testQuotationServer(t, schema)
	database := openTestStore(t, schema)
	attachedTestSigner(t, server, schema)
	ctx := context.Background()

	const note = "Delivery is ex-works.\nInstallation by the customer."
	input := quotationInput()
	input["Note"] = note
	code, exported := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, input)
	if code != http.StatusCreated {
		t.Fatalf("export = %d; want 201 (%v)", code, exported)
	}
	shortID, _ := exported["shortId"].(string)
	if shortID == "" {
		t.Fatal("export response carries no shortId")
	}
	id, _ := exported["id"].(float64)

	code, detail := doJSON(t, server, http.MethodGet,
		"/v1/quotations/"+strconv.Itoa(int(id)), token, nil)
	if code != http.StatusOK {
		t.Fatalf("detail = %d; want 200", code)
	}
	if got, _ := detail["note"].(string); got != note {
		t.Errorf("detail note = %q; want %q", got, note)
	}

	evidence, err := database.GetRevisionEvidenceByShortID(ctx, shortID)
	if err != nil {
		t.Fatalf("revision evidence: %v", err)
	}
	if evidence.Note != note {
		t.Errorf("evidence note = %q; want %q", evidence.Note, note)
	}

	code, page := fetchVerifyPage(t, server, shortID)
	if code != http.StatusOK {
		t.Fatalf("verify page = %d; want 200", code)
	}
	if !strings.Contains(page, "Verified") || strings.Contains(page, "Does not match") {
		t.Fatal("a genuine note-bearing revision did not verify")
	}

	// Editing the note in storage is tampering with signed content.
	pool, err := pgxpool.New(ctx, database.DSN())
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `
		UPDATE `+schema+`.quotation_revisions
		SET note = 'Installation is free.'
		WHERE short_id = $1`, shortID); err != nil {
		t.Fatalf("tamper the stored note: %v", err)
	}
	code, page = fetchVerifyPage(t, server, shortID)
	if code != http.StatusOK {
		t.Fatalf("tampered verify = %d; want 200 with a mismatch verdict", code)
	}
	if !strings.Contains(page, "Does not match our records") {
		t.Error("a tampered note still verified: the note is not covered by the signature")
	}
}
