package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/render"
	chromedprenderer "vera-app/backend/render/chromedp"
	"vera-app/backend/store"
)

// TestExportMockQuotation is the full-export mock: it mirrors the server's
// export endpoint in-process -- number allocation, verification identity,
// signing, persistence and both renderings -- so the resulting documents carry
// a real, scannable QR code that the public portal resolves.
//
// A QR that verifies requires the revision to exist in the database the portal
// reads, so this harness writes to the configured database by design. That is
// why it is triple-gated (an explicit opt-in, a DSN, a signing key), why the
// subject is forced to carry the {TEST} prefix the archive can filter on, and
// why it refuses to save anything whose subject lost that prefix.
//
// Requires:
//
//	VERA_MOCK_EXPORT=1         explicit opt-in; never runs by accident
//	VERA_POSTGRES_DSN          the database the portal reads
//	VERA_SIGNING_KEY           the same key the server signs with
//	VERA_VISUAL_ARTIFACT_DIR   where the PDFs are written
//
// Optional: VERA_MOCK_BASE_URL (defaults to the public API host, so the
// printed verification link is the one staff would scan).
func TestExportMockQuotation(t *testing.T) {
	if os.Getenv("VERA_MOCK_EXPORT") != "1" {
		t.Skip("set VERA_MOCK_EXPORT=1 (with VERA_POSTGRES_DSN, VERA_SIGNING_KEY, VERA_VISUAL_ARTIFACT_DIR) to export a real {TEST} mock")
	}
	dsn := os.Getenv("VERA_POSTGRES_DSN")
	signingKey := os.Getenv("VERA_SIGNING_KEY")
	dir := os.Getenv("VERA_VISUAL_ARTIFACT_DIR")
	if dsn == "" || signingKey == "" || dir == "" {
		t.Fatal("VERA_POSTGRES_DSN, VERA_SIGNING_KEY and VERA_VISUAL_ARTIFACT_DIR are all required")
	}
	base := os.Getenv("VERA_MOCK_BASE_URL")
	if base == "" {
		base = "https://api.arconpentapersada.co.id"
	}
	ctx := context.Background()

	// The store runs the versioned migrations on open, exactly as the app
	// does, so migrations 016/017 apply here if the database has not seen
	// them yet.
	database, err := store.Open(ctx, store.Config{DSN: dsn, Schema: os.Getenv("VERA_POSTGRES_SCHEMA")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	key, err := LoadSigningKey(signingKey)
	if err != nil {
		t.Fatalf("load signing key: %v", err)
	}
	signer := NewSigner(key)

	// The real client directory, so the recipient block matches production
	// text exactly; fall back to nothing rather than guessing if it is empty.
	stored, err := database.ListClients(ctx)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	if len(stored) == 0 {
		t.Fatal("database has no clients; a mock must name a real recipient")
	}
	directory := make([]ManagedClient, 0, len(stored))
	for _, c := range stored {
		directory = append(directory, ManagedClient{
			Name: c.Name, PrimaryAddress: c.PrimaryAddress,
			ID: c.ID, QuotationCount: c.QuotationCount, Code: c.Code,
		})
	}

	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	q.customerDirectory = directory
	// Attaching the store makes the production code resolve the company
	// profile (name, address, theme, logo) and the customer straight from the
	// database, which is the same path the server's export takes.
	q.SetStore(database)
	if err := Startup(q, ctx); err != nil {
		t.Fatalf("startup: %v", err)
	}

	// Customers resolve by code on both paths -- the store's
	// GetCustomerByCode and the cached directory both key on Code.
	customer := directory[0]
	for _, c := range directory {
		if c.Code == 2 { // the frontend's default selection
			customer = c
			break
		}
	}
	input := mockMixedInput()
	input.Subject = "{TEST} MEP and controls package — lighting, HVAC automation, exhaust and preventive maintenance"
	input.CustomerID = uint8(customer.Code)

	if err := q.Prepare(input); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	// The endpoint's sequence: allocate the number this customer's next
	// quotation would receive, then bind the identity and the verification
	// address before signing, so the printed QR and the stored snapshot agree.
	yy := store.CurrentYY()
	sequence, warn, err := database.NextQuoteSequence(ctx, int(input.CustomerID), yy)
	if err != nil {
		t.Fatalf("allocate quote number: %v", err)
	}
	quoteNumber := FormatQuoteNumber(input.CustomerID, uint8(yy), sequence)
	q.SetQuoteNumber(quoteNumber, 0)
	uuid, err := store.NewQuotationUUID()
	if err != nil {
		t.Fatalf("allocate quotation identity: %v", err)
	}
	shortID := store.ShortIDForRevision(uuid, 0)
	if err := q.SetVerifyURL(base + "/v/" + shortID); err != nil {
		t.Fatalf("build verification code: %v", err)
	}

	snapshot, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("render snapshot: %v", err)
	}
	draft, err := q.DraftRevision(snapshot, nil, "")
	if err != nil {
		t.Fatalf("build revision draft: %v", err)
	}
	draft.QuotationUUID = uuid
	signature, digest := signer.SignRevision(SignableFromData(q.Data))
	draft.Signature = signature
	draft.SigKeyID = signer.KeyID()
	draft.CanonicalSHA256 = digest
	draft.IssuerName = q.Data.CompanyName

	// The guard that keeps a mock from ever looking like a real quotation.
	if got := q.Data.Subject; len(got) < 6 || got[:6] != "{TEST}" {
		t.Fatalf("refusing to save: subject %q lost the {TEST} prefix", got)
	}

	record, err := database.SaveRevision(ctx, draft)
	if err != nil {
		t.Fatalf("save revision: %v", err)
	}

	// Both renderings of the identical signed document.
	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderer input: %v", err)
	}
	nativePDF, err := render.Active.Render(ctx, in)
	if err != nil {
		t.Fatalf("native render: %v", err)
	}
	browserPDF, err := chromedprenderer.New().Render(ctx, in)
	if err != nil {
		t.Fatalf("browser render: %v", err)
	}

	// Attach the production engine's PDF, the step the desktop performs after
	// saving; the portal's document view reads it.
	nativePath := filepath.Join(dir, "mock-"+record.QuoteNumber+"-native.pdf")
	browserPath := filepath.Join(dir, "mock-"+record.QuoteNumber+"-browser.pdf")
	if err := os.WriteFile(nativePath, nativePDF, 0o644); err != nil {
		t.Fatalf("write native: %v", err)
	}
	if err := os.WriteFile(browserPath, browserPDF, 0o644); err != nil {
		t.Fatalf("write browser: %v", err)
	}
	if err := database.AttachDocument(ctx, record.ID, record.RevisionNumber, nativePDF, nativePath); err != nil {
		t.Fatalf("attach document: %v", err)
	}

	if warn {
		t.Logf("sequence warning: this customer's numbering is approaching the 4-digit cap")
	}
	t.Logf("exported {TEST} quotation %s revision %d, verify at %s/v/%s",
		record.QuoteNumber, record.RevisionNumber, base, shortID)
	t.Logf("artifacts: %s and %s", nativePath, browserPath)
}
