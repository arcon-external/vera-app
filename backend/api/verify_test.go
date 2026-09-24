package api

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend"
	"vera-app/backend/store"
)

// testSigningSeed is a fixed, non-secret seed so tests are deterministic.
// Production keys come from VERA_SIGNING_KEY and never appear in source.
var testSigningSeed = func() []byte {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(31 - i)
	}
	return seed
}()

// resetVerifySchema drops a test schema so reruns on a warm database behave
// exactly like a fresh one. Without it the second run of a test hits
// "bootstrap is disabled: users already exist".
func resetVerifySchema(t *testing.T, schema string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect for schema reset: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		"DROP SCHEMA IF EXISTS "+quoteIdentForTest(schema)+" CASCADE"); err != nil {
		t.Fatalf("reset schema %s: %v", schema, err)
	}
}

// quoteIdentForTest quotes a schema name for the reset statement. Test
// schema names are literals from source, never user input.
func quoteIdentForTest(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// openTestStore opens the schema's store for direct assertions and seeding.
func openTestStore(t *testing.T, schema string) *store.Store {
	t.Helper()
	database, err := store.Open(context.Background(), store.Config{
		DSN:    strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")),
		Schema: schema,
	})
	if err != nil {
		t.Fatalf("open test store %s: %v", schema, err)
	}
	t.Cleanup(database.Close)
	return database
}

// attachedTestSigner installs a deterministic signer on a test server and
// registers its public key, exactly as boot does — otherwise verification
// cannot resolve the key and honestly reports "unavailable".
func attachedTestSigner(t *testing.T, server *Server, schema string) *backend.Signer {
	t.Helper()
	database, err := store.Open(context.Background(), store.Config{
		DSN:    strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")),
		Schema: schema,
	})
	if err != nil {
		t.Fatalf("open test database for signing: %v", err)
	}
	t.Cleanup(database.Close)
	signer := backend.NewSigner(ed25519.NewKeyFromSeed(testSigningSeed))
	if err := database.RegisterSigningKey(context.Background(), signer.KeyID(), signer.PublicKeyHex()); err != nil {
		t.Fatalf("register test signing key: %v", err)
	}
	server.SetSigner(signer)
	return signer
}

// exportForVerification exports one quotation and returns its short id and
// quotation id, with the signer attached so the revision carries a signature.
func exportForVerification(t *testing.T, server *Server, token, schema string) (shortID, quoteNumber string, quotationID int64) {
	t.Helper()
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, quotationInput())
	if code != http.StatusCreated {
		t.Fatalf("export = %d; want 201 (%v)", code, body)
	}
	shortID, _ = body["shortId"].(string)
	if shortID == "" {
		t.Fatal("export response carries no shortId; the desktop cannot print a QR")
	}
	quoteNumber, _ = body["quoteNumber"].(string)
	id, _ := body["id"].(float64)
	return shortID, quoteNumber, int64(id)
}

// fetchVerifyPage renders one verification page and returns status plus body.
func fetchVerifyPage(t *testing.T, server *Server, shortID string) (int, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/v/"+shortID, nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func TestVerifyPageShowsVerifiedDocument(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_ok")
	server, token := testQuotationServer(t, "vera_api_verify_ok")
	attachedTestSigner(t, server, "vera_api_verify_ok")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_ok")

	code, body := fetchVerifyPage(t, server, shortID)
	if code != http.StatusOK {
		t.Fatalf("verify = %d; want 200", code)
	}
	for _, want := range []string{"Verified", "1502", "Grand total", "Issued to"} {
		if !strings.Contains(body, want) {
			t.Errorf("verified page missing %q", want)
		}
	}
	if strings.Contains(body, "Does not match") {
		t.Error("valid document rendered as a mismatch")
	}
}

// TestVerifyPageNeverLeaksInternalValues is the body audit: the page may
// show the six public fields and nothing from internal economics.
func TestVerifyPageNeverLeaksInternalValues(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_leak")
	server, token := testQuotationServer(t, "vera_api_verify_leak")
	attachedTestSigner(t, server, "vera_api_verify_leak")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_leak")

	_, body := fetchVerifyPage(t, server, shortID)
	// Audit only what is rendered, never the stylesheet: CSS legitimately
	// contains words like "margin".
	content := body
	if start := strings.Index(content, "<body"); start >= 0 {
		content = content[start:]
	}
	for _, forbidden := range []string{
		"MarginPercent", "margin_percent", "NettProfit", "nett", "profit",
		"TaxAmount", "tax_amount", "margin_amount", "original_total",
		"marginalized", "cost", "UnitCost",
	} {
		if strings.Contains(strings.ToLower(content), strings.ToLower(forbidden)) {
			t.Errorf("verification page leaks internal value %q", forbidden)
		}
	}
}

func TestVerifyPageRejectsUnknownAndMalformed(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_missing")
	server, token := testQuotationServer(t, "vera_api_verify_missing")
	attachedTestSigner(t, server, "vera_api_verify_missing")
	exportForVerification(t, server, token, "vera_api_verify_missing")

	// Unknown, malformed, wrong-length, and traversal-shaped inputs must all
	// return the same page: an attacker learns nothing from the difference.
	responses := map[string]string{}
	// Single-segment inputs only: anything containing a slash is a
	// different route entirely and never reaches the verification handler.
	for _, input := range []string{
		"0000000000000000",
		"zzzz",
		strings.Repeat("a", 400),
		"9f3a4c2e-1b7d-4a8e",
		"ZZZZZZZZZZZZZZZZ",
	} {
		code, body := fetchVerifyPage(t, server, input)
		if code != http.StatusNotFound {
			t.Errorf("input %q = %d; want 404", input, code)
		}
		if !strings.Contains(body, "Not found") {
			t.Errorf("input %q rendered %q; want the not-found page", input, firstLineOf(body))
		}
		responses[input] = body
	}
	// Every rejection must be byte-identical: no oracle for enumerators.
	var reference string
	for input, body := range responses {
		if reference == "" {
			reference = body
			continue
		}
		if body != reference {
			t.Errorf("rejection for %q differs from other rejections", input)
		}
	}
}

func TestVerifyPageDetectsStoredTampering(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_tamper")
	server, token := testQuotationServer(t, "vera_api_verify_tamper")
	database := openTestStore(t, "vera_api_verify_tamper")
	attachedTestSigner(t, server, "vera_api_verify_tamper")

	shortID, _, quotationID := exportForVerification(t, server, token, "vera_api_verify_tamper")

	// Alter the stored record after signing: verification must fail loudly.
	pool, err := pgxpool.New(context.Background(), database.DSN())
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), `
		UPDATE vera_api_verify_tamper.quotation_revision_totals
		SET customer_grand_total = 1
		WHERE revision_id = (
			SELECT id FROM vera_api_verify_tamper.quotation_revisions
			WHERE quotation_id = $1)`, quotationID); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	code, body := fetchVerifyPage(t, server, shortID)
	if code != http.StatusOK {
		t.Fatalf("tampered verify = %d; want 200 with a mismatch verdict", code)
	}
	if !strings.Contains(body, "Does not match our records") {
		t.Error("tampered record still rendered as valid")
	}
	if !strings.Contains(body, "altered after issue") {
		t.Error("tamper explanation missing from the page")
	}
}

func TestVerifyPageFlagsSupersededRevision(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_super")
	server, token := testQuotationServer(t, "vera_api_verify_super")
	attachedTestSigner(t, server, "vera_api_verify_super")

	firstShort, quoteNumber, _ := exportForVerification(t, server, token, "vera_api_verify_super")

	second := quotationInput()
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input": second, "quoteNumber": quoteNumber, "revision": 1,
	})
	if code != http.StatusCreated {
		t.Fatalf("second export = %d; want 201 (%v)", code, body)
	}

	_, page := fetchVerifyPage(t, server, firstShort)
	if !strings.Contains(page, "Superseded") {
		t.Error("older revision not flagged as superseded")
	}
	if strings.Contains(page, "Does not match") {
		t.Error("superseded revision rendered as fraud; it is genuine history")
	}
	if !strings.Contains(page, "Rev 01") {
		t.Error("superseded page does not name the current revision")
	}
}

func TestVerifySubmitComparesPaperClaims(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_claims")
	server, token := testQuotationServer(t, "vera_api_verify_claims")
	attachedTestSigner(t, server, "vera_api_verify_claims")
	shortID, quoteNumber, quotationID := exportForVerification(t, server, token, "vera_api_verify_claims")

	// Matching claims keep the page valid.
	form := url.Values{"id": {shortID}}
	request := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Verified") {
		t.Fatalf("matching lookup = %d; want a verified page", recorder.Code)
	}

	// An altered amount must be named, with both values shown.
	form = url.Values{"id": {shortID}, "amount": {"Rp200.000"}}
	request = httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if !strings.Contains(body, "Grand total") {
		t.Error("claim comparison did not name the differing field")
	}
	if !strings.Contains(body, "Rp200.000") {
		t.Error("claim comparison did not show the claimed value")
	}
	if !strings.Contains(body, "most common sign") {
		t.Error("claim comparison did not explain what the difference means")
	}

	// Fully matching claims must not render a differs section: there is
	// nothing to list, and the heading would contradict the verdict.
	code, detail := doJSON(t, server, http.MethodGet, "/v1/quotations/"+strconv.Itoa(int(quotationID)), token, nil)
	if code != http.StatusOK {
		t.Fatalf("detail = %d; want 200", code)
	}
	grandTotal, _ := detail["grandTotal"].(float64)
	form = url.Values{"id": {shortID}, "number": {quoteNumber}, "amount": {strconv.FormatInt(int64(grandTotal), 10)}}
	request = httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	body = recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "Verified") {
		t.Fatalf("all-match claims = %d; want a verified page", recorder.Code)
	}
	if strings.Contains(body, "What differs from our record") {
		t.Error("all-match claims rendered a differs section with nothing in it")
	}
}

func TestVerifyPageAcceptsPastedLink(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_link")
	server, token := testQuotationServer(t, "vera_api_verify_link")
	attachedTestSigner(t, server, "vera_api_verify_link")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_link")

	form := url.Values{"id": {"https://example.run.app/v/" + shortID}}
	request := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), "Verified") {
		t.Error("pasted verification link did not resolve")
	}
}

func TestVerifyPageSetsSecurityHeaders(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_headers")
	server, token := testQuotationServer(t, "vera_api_verify_headers")
	attachedTestSigner(t, server, "vera_api_verify_headers")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_headers")

	request := httptest.NewRequest(http.MethodGet, "/v/"+shortID, nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("%s = %q; want %q", header, got, want)
		}
	}
}

func TestVerifyPageRateLimits(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_rate")
	server, token := testQuotationServer(t, "vera_api_verify_rate")
	attachedTestSigner(t, server, "vera_api_verify_rate")
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_rate")

	// The limiter allows a burst then refuses; verification is a human-pace
	// action, so a tight window is correct here.
	limited := false
	for i := 0; i < 40; i++ {
		request := httptest.NewRequest(http.MethodGet, "/v/"+shortID, nil)
		request.RemoteAddr = "203.0.113.7:1234"
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("rate limiter never engaged after 40 rapid checks")
	}
}

func TestVerifyUnavailableWithoutSigner(t *testing.T) {
	resetVerifySchema(t, "vera_api_verify_nosigner")
	server, token := testQuotationServer(t, "vera_api_verify_nosigner")
	attachedTestSigner(t, server, "vera_api_verify_nosigner")

	// Export with no signer attached: the revision stores no signature, and
	// a server that cannot sign must say so rather than claim validity.
	server.SetSigner(nil)
	shortID, _, _ := exportForVerification(t, server, token, "vera_api_verify_nosigner")
	_, body := fetchVerifyPage(t, server, shortID)
	if !strings.Contains(body, "Verification unavailable") {
		t.Error("unsigned server did not report verification unavailable")
	}
	if strings.Contains(body, "Verified") {
		t.Error("unsigned server reported a document as verified")
	}
}

func firstLineOf(body string) string {
	if idx := strings.IndexByte(body, '\n'); idx >= 0 {
		return strings.TrimSpace(body[:idx])
	}
	return strings.TrimSpace(body)
}
