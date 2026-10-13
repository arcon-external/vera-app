package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"vera-app/backend/store"
)

// setDraftTestProfileCurrency moves the live company currency the way the
// admin settings picker would, so a draft can be stamped under one code and
// exported under another.
func setDraftTestProfileCurrency(t *testing.T, database *store.Store, code string) {
	t.Helper()
	if _, err := database.UpdateCompanyProfile(context.Background(), store.CompanyProfile{
		LegalName: "PT Arcon Penta Persada",
		Currency:  code,
	}); err != nil {
		t.Fatalf("set company currency %s: %v", code, err)
	}
}

// TestExportRefusesDraftCurrencyMismatch pins the server boundary: a draft
// saved under one currency cannot be exported after the company currency has
// moved on. The server would otherwise format, total and sign whole-rupiah
// integers under the new denomination -- a silent 100x reinterpretation, and
// a revision that contradicts the one it follows. The refusal names both
// codes, and it arrives before any revision work, so nothing is stamped and
// the draft survives for a retry.
func TestExportRefusesDraftCurrencyMismatch(t *testing.T) {
	server, token, _, database := draftTestServer(t, "vera_api_draft_export_currency")

	setDraftTestProfileCurrency(t, database, "IDR")
	draftID := saveDraft(t, server, token, draftSaveBody(`{"v":1}`, 1))
	// The company switches to USD; the draft keeps its IDR stamp.
	setDraftTestProfileCurrency(t, database, "USD")

	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input":   quotationInput(),
		"draftId": draftID,
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("export under a switched currency = %d; want 422 (%v)", code, body)
	}
	message, _ := body["error"].(string)
	for _, want := range []string{"IDR", "USD"} {
		if !strings.Contains(message, want) {
			t.Errorf("refusal = %q; want it to name %s", message, want)
		}
	}

	// The refusal is before any revision work: the draft is still there to
	// export once the profile is switched back.
	code, list := doJSON(t, server, http.MethodGet, "/v1/drafts", token, nil)
	if code != http.StatusOK {
		t.Fatalf("drafts after the refusal = %d; want 200", code)
	}
	if drafts, _ := list["drafts"].([]any); len(drafts) != 1 {
		t.Fatalf("%d drafts survived the refused export; want the one kept", len(drafts))
	}
}

// TestExportProceedsUnderMatchingCurrency is the control: currencies that
// match export exactly as they did before the guard existed -- 201, and the
// draft is consumed by the export it became.
func TestExportProceedsUnderMatchingCurrency(t *testing.T) {
	server, token, _, database := draftTestServer(t, "vera_api_draft_export_currency_ok")

	setDraftTestProfileCurrency(t, database, "IDR")
	draftID := saveDraft(t, server, token, draftSaveBody(`{"v":1}`, 1))

	code, exported := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input":   quotationInput(),
		"draftId": draftID,
	})
	if code != http.StatusCreated {
		t.Fatalf("export under a matching currency = %d; want 201 (%v)", code, exported)
	}

	code, list := doJSON(t, server, http.MethodGet, "/v1/drafts", token, nil)
	if code != http.StatusOK {
		t.Fatalf("drafts = %d; want 200", code)
	}
	if drafts, _ := list["drafts"].([]any); len(drafts) != 0 {
		t.Errorf("%d drafts survived the export; want 0", len(drafts))
	}
}
