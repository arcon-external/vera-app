package api

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"vera-app/backend/store"
)

// TestDraftCurrencyStampedFromLiveProfile pins the server side of the B4
// contract: creation stamps the live company currency (never anything the
// client sent), the load returns it, and a later save under a new profile
// keeps the original stamp rather than relabelling old integers.
func TestDraftCurrencyStampedFromLiveProfile(t *testing.T) {
	server, token, _, database := draftTestServer(t, "vera_api_drafts_currency")

	setProfileCurrency := func(code string) {
		t.Helper()
		if _, err := database.UpdateCompanyProfile(context.Background(), store.CompanyProfile{
			LegalName: "PT Arcon Penta Persada",
			Currency:  code,
		}); err != nil {
			t.Fatalf("set company currency %s: %v", code, err)
		}
	}
	getCurrency := func(id int64) string {
		t.Helper()
		code, body := doJSON(t, server, http.MethodGet,
			"/v1/drafts/"+strconv.FormatInt(id, 10), token, nil)
		if code != http.StatusOK {
			t.Fatalf("get = %d; want 200 (%v)", code, body)
		}
		currencyCode, _ := body["currency"].(string)
		return currencyCode
	}

	setProfileCurrency("USD")
	draftID := saveDraft(t, server, token, draftSaveBody(`{"v":1}`, 1))
	if got := getCurrency(draftID); got != "USD" {
		t.Errorf("draft currency = %q; want the live profile's USD", got)
	}

	// The profile moves on; the stored stamp must not follow it.
	setProfileCurrency("IDR")
	body := draftSaveBody(`{"v":2}`, 1)
	body["id"] = draftID
	code, _ := doJSON(t, server, http.MethodPost, "/v1/drafts", token, body)
	if code != http.StatusOK {
		t.Fatalf("second save = %d; want 200", code)
	}
	if got := getCurrency(draftID); got != "USD" {
		t.Errorf("draft currency after resave = %q; want the original USD", got)
	}

	// A fresh draft under the new profile stamps the new code.
	freshID := saveDraft(t, server, token, draftSaveBody(`{"v":1}`, 1))
	if got := getCurrency(freshID); got != "IDR" {
		t.Errorf("fresh draft currency = %q; want the live profile's IDR", got)
	}
}
