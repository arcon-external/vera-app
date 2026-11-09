package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCustomerRenameKeepsIssuedVerificationAndIssuesNewName is the end-to-end
// proof for the recipient_company snapshot:
//
//   - the revision issued before the rename still verifies, and still names
//     the company it printed;
//   - the read path (archive detail) returns that same historical name;
//   - the revision issued after the rename prints and signs the new name;
//   - a second rename changes neither issued revision.
func TestCustomerRenameKeepsIssuedVerificationAndIssuesNewName(t *testing.T) {
	const schema = "vera_api_recipient_company"
	resetTestSchema(t, schema)
	server, token := testQuotationServer(t, schema)
	database := openTestStore(t, schema)
	attachedTestSigner(t, server, schema)
	ctx := context.Background()

	// Migration 011 seeds client code 2; testQuotationServer's seed keeps that
	// row rather than replacing it, so this is the name the first export prints.
	const issuedCompany = "PT. Infineon Technologies Batam"
	firstShort, quoteNumber, quotationID := exportForVerification(t, server, token, schema)

	assertDetail := func(label, wantName string, revision *int16) {
		t.Helper()
		path := "/v1/quotations/" + strconv.FormatInt(quotationID, 10)
		if revision != nil {
			path += "?revision=" + strconv.Itoa(int(*revision))
		}
		code, detail := doJSON(t, server, http.MethodGet, path, token, nil)
		if code != http.StatusOK {
			t.Fatalf("%s: detail = %d; want 200 (%v)", label, code, detail)
		}
		if got, _ := detail["customerName"].(string); got != wantName {
			t.Errorf("%s: detail company = %q; want %q", label, got, wantName)
		}
		snapshot, _ := detail["htmlSnapshot"].(string)
		if !strings.Contains(snapshot, wantName) {
			t.Errorf("%s: stored document does not print %q", label, wantName)
		}
	}

	assertGenuine := func(label, shortID, wantName, wantVerdict string) {
		t.Helper()
		code, page := fetchVerifyPage(t, server, shortID)
		if code != http.StatusOK {
			t.Fatalf("%s: verify page = %d; want 200", label, code)
		}
		if !strings.Contains(page, wantVerdict) {
			t.Errorf("%s: issued code is not %q", label, wantVerdict)
		}
		if strings.Contains(page, "Does not match") {
			t.Errorf("%s: genuine revision rendered as a mismatch", label)
		}
		if !strings.Contains(page, wantName) {
			t.Errorf("%s: verification page does not name %q", label, wantName)
		}
	}

	assertDetail("revision 0 at issue", issuedCompany, nil)
	assertGenuine("revision 0 at issue", firstShort, issuedCompany, "Verified")

	rename := func(name string) {
		t.Helper()
		clients, err := database.ListClients(ctx)
		if err != nil {
			t.Fatalf("list clients: %v", err)
		}
		for _, client := range clients {
			if client.Code != 2 {
				continue
			}
			if _, err := database.UpdateClient(ctx, client.ID, name, "Test Address"); err != nil {
				t.Fatalf("rename client: %v", err)
			}
			return
		}
		t.Fatal("seeded client code 2 not found")
	}
	rename("PT Renamed")

	// The issued revision keeps both its printed name and its verdict.
	assertDetail("revision 0 after rename", issuedCompany, nil)
	revisionZero := int16(0)
	assertDetail("revision 0 explicit after rename", issuedCompany, &revisionZero)
	assertGenuine("revision 0 after rename", firstShort, issuedCompany, "Verified")

	// A new revision is a new document: it prints and signs the renamed company.
	code, revised := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input": quotationInput(), "quoteNumber": quoteNumber, "revision": 1,
	})
	if code != http.StatusCreated {
		t.Fatalf("revision 1 export = %d; want 201 (%v)", code, revised)
	}
	secondShort, _ := revised["shortId"].(string)
	if secondShort == "" {
		t.Fatal("revision 1 export carries no shortId")
	}
	assertDetail("revision 1 after rename", "PT Renamed", nil)
	revisionOne := int16(1)
	assertDetail("revision 1 explicit after rename", "PT Renamed", &revisionOne)
	assertGenuine("revision 1 after rename", secondShort, "PT Renamed", "Verified")

	secondEvidence, err := database.GetRevisionEvidenceByShortID(ctx, secondShort)
	if err != nil {
		t.Fatalf("revision 1 evidence: %v", err)
	}
	if secondEvidence.RecipientCompany != "PT Renamed" {
		t.Errorf("revision 1 evidence company = %q; want the name it signed", secondEvidence.RecipientCompany)
	}

	// A later rename must not rewrite either issued revision.
	rename("PT Renamed Again")
	firstEvidence, err := database.GetRevisionEvidenceByShortID(ctx, firstShort)
	if err != nil {
		t.Fatalf("revision 0 evidence: %v", err)
	}
	if firstEvidence.RecipientCompany != issuedCompany {
		t.Errorf("revision 0 evidence company = %q; want its issued name", firstEvidence.RecipientCompany)
	}
	secondEvidence, err = database.GetRevisionEvidenceByShortID(ctx, secondShort)
	if err != nil {
		t.Fatalf("revision 1 evidence: %v", err)
	}
	if secondEvidence.RecipientCompany != "PT Renamed" {
		t.Errorf("revision 1 evidence company = %q; want its issued name", secondEvidence.RecipientCompany)
	}
	// Revision 0 is genuine history now that revision 1 exists: the portal
	// marks it superseded, never tampered.
	assertGenuine("revision 0 after second rename", firstShort, issuedCompany, "Superseded")
	assertGenuine("revision 1 after second rename", secondShort, "PT Renamed", "Verified")
}

// TestLegacyRevisionWithoutRecipientCompanySnapshotStillVerifies is the
// migration story at the verification seam: a revision written before the
// snapshot column existed reads back empty, falls back to the live customer
// name it was signed with, and its printed code still verifies.
func TestLegacyRevisionWithoutRecipientCompanySnapshotStillVerifies(t *testing.T) {
	const schema = "vera_api_recipient_company_legacy"
	resetTestSchema(t, schema)
	server, token := testQuotationServer(t, schema)
	database := openTestStore(t, schema)
	attachedTestSigner(t, server, schema)
	ctx := context.Background()

	const issuedCompany = "PT. Infineon Technologies Batam"
	shortID, _, quotationID := exportForVerification(t, server, token, schema)

	pool, err := pgxpool.New(ctx, database.DSN())
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `
		UPDATE `+schema+`.quotation_revisions
		SET recipient_company = ''
		WHERE quotation_id = $1`, quotationID); err != nil {
		t.Fatalf("clear the snapshot to the pre-migration shape: %v", err)
	}

	code, page := fetchVerifyPage(t, server, shortID)
	if code != http.StatusOK {
		t.Fatalf("verify page = %d; want 200", code)
	}
	if !strings.Contains(page, "Verified") {
		t.Error("a legacy revision without a snapshot no longer verifies")
	}
	if !strings.Contains(page, issuedCompany) {
		t.Errorf("legacy verification page does not name the live fallback %q", issuedCompany)
	}

	code, detail := doJSON(t, server, http.MethodGet,
		"/v1/quotations/"+strconv.FormatInt(quotationID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("detail = %d; want 200", code)
	}
	if got, _ := detail["customerName"].(string); got != issuedCompany {
		t.Errorf("legacy read path company = %q; want the live fallback %q", got, issuedCompany)
	}
}
