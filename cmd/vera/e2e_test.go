package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend"
	"vera-app/backend/api"
	"vera-app/backend/auth"
	"vera-app/backend/render"
	"vera-app/backend/store"
)

// This is the test the whole CLI exists for: it drives every command a person
// would actually run, in order, against a real veraserver and a real database,
// and checks the result the way the customer would.
//
// The database is a throwaway schema in the local test cluster, never anything
// else: the setup drops the schema outright, so pointing it at a database that
// matters would destroy it. VERA_POSTGRES_TEST_DSN is the only DSN this file
// reads, and it is documented as test-only for exactly this reason.
//
// The PDF is rendered here, in this process, by the application's own renderer.
// That is the part a pure HTTP client could not do, and it is why the CLI
// imports the application instead of reimplementing it.

const e2eSchema = "vera_cli_e2e"

const (
	e2eAdminEmail    = "cli-admin@example.com"
	e2eAdminName     = "CLI Admin"
	e2eAdminPassword = "cli-e2e-password"
)

// e2eServer boots a throwaway veraserver and returns its URL.
func e2eServer(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run the CLI end-to-end test")
	}
	ctx := context.Background()

	// A warm database would otherwise carry the previous run's users, and
	// bootstrap refuses to run when any exist.
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for the schema reset: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS "`+e2eSchema+`" CASCADE`); err != nil {
		pool.Close()
		t.Fatalf("reset the test schema: %v", err)
	}
	pool.Close()

	database, err := store.Open(ctx, store.Config{DSN: dsn, Schema: e2eSchema})
	if err != nil {
		t.Fatalf("open the test database: %v", err)
	}
	t.Cleanup(database.Close)

	service := auth.NewService(database)
	if _, err := service.Bootstrap(ctx, e2eAdminEmail, e2eAdminName, e2eAdminPassword); err != nil {
		t.Fatalf("bootstrap the admin: %v", err)
	}

	templates := &backend.Quotation{}
	if err := backend.Startup(templates, ctx); err != nil {
		t.Fatalf("load the embedded templates: %v", err)
	}

	server := api.NewServer(database, service, templates)
	// A fixed, non-secret seed so the run is deterministic. Production keys come
	// from VERA_SIGNING_KEY and never appear in source.
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(17 + i)
	}
	signer := backend.NewSigner(ed25519.NewKeyFromSeed(seed))
	if err := database.RegisterSigningKey(ctx, signer.KeyID(), signer.PublicKeyHex()); err != nil {
		t.Fatalf("register the signing key: %v", err)
	}
	server.SetSigner(signer)

	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	return httpServer.URL
}

// withStdin replaces standard input for the duration of fn, so a command that
// reads a password from a pipe can be driven from a test.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	original := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = original
		reader.Close()
	}()
	go func() {
		_, _ = writer.WriteString(input)
		writer.Close()
	}()
	fn()
}

// mustRun runs a command and fails the test unless it succeeded, returning
// stdout. Every step of this test is expected to work, so a non-zero exit is
// always a finding rather than something to branch on.
func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	code, stdout, stderr := runCLI(t, args...)
	if code != exitOK {
		t.Fatalf("vera %s\nexit %d\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), code, stdout, stderr)
	}
	return stdout
}

// decodeJSON reads a command's --json output.
func decodeJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout)
	}
	return payload
}

func TestEndToEndThroughTheCLI(t *testing.T) {
	// The export step renders through whichever engine the build selected.
	render.RequireUsable(t)
	url := e2eServer(t)
	dir := isolate(t)
	// isolate() points VERA_API_URL at a closed port so no unit test can reach
	// production by accident. This test wants the real thing, and wants the
	// session to remember which server it signed into, so the variable is
	// cleared and only the sign-in and verification commands name a URL.
	t.Setenv("VERA_API_URL", "")
	work := t.TempDir()

	// --- sign in -----------------------------------------------------------
	// The password goes in over a pipe, which is the documented way for a
	// script and the only form that is neither a flag nor a prompt.
	withStdin(t, e2eAdminPassword+"\n", func() {
		stdout := mustRun(t, "login", "--email", e2eAdminEmail, "--password-stdin", "--api", url, "--json")
		if payload := decodeJSON(t, stdout); payload["email"] != e2eAdminEmail {
			t.Errorf("login reported %v; want %s", payload["email"], e2eAdminEmail)
		}
	})

	sessionPath := filepath.Join(dir, "vera", "session.json")
	info, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatalf("login did not write a session: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session file mode = %o; want 600", perm)
	}

	// --- who am I ----------------------------------------------------------
	me := decodeJSON(t, mustRun(t, "whoami", "--json"))
	if me["email"] != e2eAdminEmail || me["role"] != "admin" {
		t.Errorf("whoami = %v; want the admin we signed in as", me)
	}

	// --- the client directory ---------------------------------------------
	// Clients are database rows, never code: adding one here is what makes the
	// code usable in the next command.
	clients := decodeJSON(t, mustRun(t, "clients", "list", "--json"))
	if _, ok := clients["clients"]; !ok {
		t.Errorf("clients list = %v; want a clients array", clients)
	}
	added := decodeJSON(t, mustRun(t, "clients", "add",
		"--code", "7", "--name", "PT End To End", "--address", "Jl. Test 1, Batam", "--json"))
	if added["code"] != float64(7) {
		t.Fatalf("clients add = %v; want code 7", added)
	}

	// --- price a quotation without filing it ------------------------------
	input := filepath.Join(work, "quotation.json")
	body := `{
	  "customerCode": 7,
	  "subject": "E2E acceptance quotation",
	  "recipientName": "Dr. End To End",
	  "incoterm": "DAP",
	  "taxZone": "FTZ",
	  "taxType": "Goods",
	  "items": [
	    {
	      "description": "Control panel replacement",
	      "uom": "Lot",
	      "qty": 2,
	      "unitCost": 15000000,
	      "marginEnabled": true,
	      "marginPercent": 20
	    },
	    {
	      "description": "Commissioning",
	      "uom": "LS",
	      "qty": 1,
	      "unitCost": 4000000,
	      "marginEnabled": true,
	      "marginPercent": 25,
	      "discountEnabled": true,
	      "discountPercent": 10
	    },
	    {
	      "description": "Trauma system upgrade",
	      "showSubPrices": true,
	      "subItems": [
	        {
	          "description": "Panel beds",
	          "uom": "Pc",
	          "qty": 2,
	          "unitCost": 8500000,
	          "marginEnabled": true,
	          "marginPercent": 20,
	          "details": ["Powder-coated, with mounting hardware"]
	        },
	        {
	          "description": "Monitor arms and cabling",
	          "uom": "LS",
	          "qty": 1,
	          "unitCost": 1200000
	        }
	      ]
	    }
	  ]
	}`
	if err := os.WriteFile(input, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	validated := decodeJSON(t, mustRun(t, "quotations", "validate", "--input", input, "--json"))
	internal, ok := validated["internal"].(map[string]any)
	if !ok {
		t.Fatalf("validate returned no internal breakdown: %v", validated)
	}
	// The margin is the line's own: 15.000.000 x 1.2 x 2 = 36.000.000. The
	// discounted line is 4.000.000 x 1.25 = 5.000.000 less 10% = 4.500.000.
	// The split line rolls up its parts: (8.500.000 x 1.2 x 2) + 1.200.000 =
	// 21.600.000. Customer subtotal: 36.000.000 + 4.500.000 + 21.600.000.
	if got := internal["DisplayMarginalizedSubtotal"]; got != "62.100.000" {
		t.Errorf("customer subtotal = %v; want 62.100.000", got)
	}
	// A discount cuts the price only, never what we pay, so the cost side is
	// untouched: 30.000.000 + 4.000.000 + 18.200.000.
	if got := internal["DisplayOriginalSubtotal"]; got != "52.200.000" {
		t.Errorf("cost subtotal = %v; want 52.200.000", got)
	}

	// The internal breakdown is the operator's, and it has to actually carry
	// the figures -- otherwise the check below proves nothing.
	if _, ok := internal["DisplayTax"]; !ok {
		t.Errorf("the internal breakdown has no tax line: %v", internal)
	}

	// --- the next number, without consuming it ----------------------------
	peek := decodeJSON(t, mustRun(t, "quotations", "next-number", "--client", "7", "--json"))
	peeked, _ := peek["quoteNumber"].(string)
	if peeked == "" {
		t.Fatalf("next-number returned %v", peek)
	}

	// --- export: number, signed revision, PDF, attach ---------------------
	pdfPath := filepath.Join(work, "quotation.pdf")
	exported := decodeJSON(t, mustRun(t, "quotations", "export",
		"--input", input, "--out", pdfPath, "--json"))

	quoteNumber, _ := exported["quoteNumber"].(string)
	if quoteNumber == "" {
		t.Fatalf("export returned no quote number: %v", exported)
	}
	if quoteNumber != peeked {
		t.Errorf("export issued %q but next-number promised %q; peeking must not consume the number",
			quoteNumber, peeked)
	}
	if !strings.HasPrefix(quoteNumber, "1507") {
		t.Errorf("quote number = %q; want the 15CCYYNNNN shape for client 7", quoteNumber)
	}

	pdf, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("export did not write a PDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("the exported file is not a PDF: %q", pdf[:min(8, len(pdf))])
	}
	if len(pdf) < 4096 {
		t.Errorf("the PDF is only %d bytes, too small to be a rendered quotation", len(pdf))
	}

	shortID, _ := exported["shortId"].(string)
	if shortID == "" {
		t.Fatalf("export returned no verification code: %v", exported)
	}

	// --- verify it the way a customer would -------------------------------
	// No session, through the public portal: this is the same check the QR
	// code leads to.
	verdict := decodeJSON(t, mustRun(t, "verify", shortID, "--api", url, "--json"))
	if verdict["verdict"] != "valid" || verdict["valid"] != true {
		t.Fatalf("the document did not verify: %v", verdict)
	}

	// Claiming the wrong figure must be caught. This is the assertion that
	// makes the previous one mean something: a checker that says "valid" to
	// everything would pass that and fail this.
	code, _, stderr := runCLI(t, "verify", shortID, "--amount", "1", "--api", url, "--json")
	if code != exitValidation {
		t.Errorf("verify with a wrong amount: exit %d, stderr %q; want %d", code, stderr, exitValidation)
	}

	// A code that does not exist is not valid either.
	code, _, _ = runCLI(t, "verify", "00000000", "--api", url, "--json")
	if code != exitNotFound {
		t.Errorf("verify of an unknown code: exit %d; want %d", code, exitNotFound)
	}

	// --- the archive -------------------------------------------------------
	list := decodeJSON(t, mustRun(t, "quotations", "list", "--json"))
	rows, _ := list["quotations"].([]any)
	if len(rows) != 1 {
		t.Fatalf("archive has %d quotations; want 1 (%v)", len(rows), list)
	}
	first, _ := rows[0].(map[string]any)
	id := int64(first["id"].(float64))
	if first["quoteNumber"] != quoteNumber {
		t.Errorf("archive row = %v; want the exported number %q", first["quoteNumber"], quoteNumber)
	}

	detail := decodeJSON(t, mustRun(t, "quotations", "show", fmt.Sprintf("%d", id), "--json"))
	if detail["quoteNumber"] != quoteNumber {
		t.Errorf("show returned %v; want %q", detail["quoteNumber"], quoteNumber)
	}

	// The stored snapshot is the customer's document exactly as issued, and
	// unlike the PDF its text can be read. That makes it the right thing to
	// hold the boundary against: this is the rule
	// TestRenderedQuotationHidesInternalValues pins for the template, checked
	// here through the CLI's own export path.
	//
	// The positive control comes first: a snapshot whose text could not be
	// searched would make every check below pass without proving anything.
	customerHTML := mustRun(t, "quotations", "show", fmt.Sprintf("%d", id), "--html")
	for _, expected := range []string{"E2E acceptance quotation", "PT End To End", quoteNumber} {
		if !strings.Contains(customerHTML, expected) {
			t.Fatalf("the stored customer document does not contain %q, so searching it proves nothing", expected)
		}
	}
	// Labels first. "margin" on its own is not on this list: it is a CSS
	// property, and the snapshot carries its own stylesheet.
	for _, forbidden := range []string{"Nett", "nett", "PPH22", "Profit", "profit"} {
		if strings.Contains(customerHTML, forbidden) {
			t.Errorf("the customer document contains %q, which must never reach a customer", forbidden)
		}
	}
	// Then the figures themselves, taken from this run rather than hard-coded.
	//
	// Only the genuinely internal ones: DisplayMarginalizedAfterTax is left out
	// because on a quotation with no PPN it equals the customer's own grand
	// total, so finding it there would be correct rather than a leak. Profit and
	// nett exist only on the operator's side.
	for _, label := range []string{"DisplayProfit", "DisplayNett", "DisplayTax"} {
		figure, _ := internal[label].(string)
		if figure == "" || figure == "0" {
			continue
		}
		if strings.Contains(customerHTML, figure) {
			t.Errorf("the customer document prints %s (%s), which is internal arithmetic", figure, label)
		}
	}

	revisions := decodeJSON(t, mustRun(t, "quotations", "revisions", fmt.Sprintf("%d", id), "--json"))
	if got, _ := revisions["revisions"].([]any); len(got) != 1 {
		t.Errorf("revisions = %v; want exactly the first one", revisions)
	}

	// --- drafts ------------------------------------------------------------
	saved := decodeJSON(t, mustRun(t, "drafts", "save", "--input", input, "--label", "E2E draft", "--json"))
	draftID := int64(saved["id"].(float64))
	drafts := decodeJSON(t, mustRun(t, "drafts", "list", "--json"))
	if got, _ := drafts["drafts"].([]any); len(got) != 1 {
		t.Errorf("drafts = %v; want the one we saved", drafts)
	}
	mustRun(t, "drafts", "rm", fmt.Sprintf("%d", draftID), "--yes", "--json")

	// --- sign out ----------------------------------------------------------
	mustRun(t, "logout", "--api", url, "--json")
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Errorf("logout left the session file behind (stat err = %v)", err)
	}
	// And the CLI now says so rather than making a request.
	code, _, stderr = runCLI(t, "clients", "list", "--api", url)
	if code != exitAuth {
		t.Errorf("after logout, clients list: exit %d, stderr %q; want %d", code, stderr, exitAuth)
	}
}
