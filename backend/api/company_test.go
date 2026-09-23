package api

import (
	"context"
	"net/http"
	"testing"
)

func TestCompanyProfileAdminRoundTrip(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_company")

	code, body := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/company", "")
	if code != http.StatusOK {
		t.Fatalf("get = %d; want 200 (%v)", code, body)
	}
	if body["legalName"] == "" {
		t.Error("seeded legal name is empty")
	}

	code, updated := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company",
		`{"legalName":"  PT Example Persada ","address":"Batam","phone":"+62-778-1","email":"info@example.co.id","taxId":"00.000.000.0-000.000","themeQDeep":"#0A1A5C","themePInk":"#111827"}`)
	if code != http.StatusOK {
		t.Fatalf("put = %d; want 200 (%v)", code, updated)
	}
	if updated["legalName"] != "PT Example Persada" || updated["email"] != "info@example.co.id" {
		t.Errorf("updated = %v; want trimmed fields", updated)
	}
	if updated["themeQDeep"] != "#0A1A5C" || updated["themePInk"] != "#111827" {
		t.Errorf("updated theme = %v; want stored hex values", updated)
	}
	if _, leaked := updated["logoPath"]; leaked {
		t.Error("logo path leaks into the DTO")
	}
	for _, bad := range []string{
		`{"legalName":"PT X","themeQDeep":"deep-blue"}`,
		`{"legalName":"PT X","themePAccent":"#FFF"}`,
		`{"legalName":"PT X","themeQSoft":"D6E4FF"}`,
	} {
		if code, body := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company", bad); code != http.StatusUnprocessableEntity {
			t.Errorf("put %s = %d (%v); want 422", bad, code, body)
		}
	}

	if code, _ := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company",
		`{"legalName":"  "}`); code != http.StatusUnprocessableEntity {
		t.Errorf("empty name = %d; want 422", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company",
		`{"legalName":"PT X","email":"not-an-email"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("bad email = %d; want 422", code)
	}

	staffToken := staffLogin(t, server, "vera_api_company")
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/admin/company", ""); code != http.StatusForbidden {
		t.Errorf("staff get = %d; want 403", code)
	}
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPut, "/v1/admin/company",
		`{"legalName":"PT Hijack"}`); code != http.StatusForbidden {
		t.Errorf("staff put = %d; want 403", code)
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodGet, "/v1/admin/company", ""); code != http.StatusUnauthorized {
		t.Errorf("anon get = %d; want 401", code)
	}

	if count, err := database.AuditCount(context.Background(), "company.update"); err != nil || count != 1 {
		t.Errorf("audit company.update = %d, %v; want 1, nil", count, err)
	}
}

func TestCompanyIdentityServesAllRoles(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_company_identity")

	code, _ := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company",
		`{"legalName":"PT Live Identity","themeQDeep":"#0A1A5C"}`)
	if code != http.StatusOK {
		t.Fatalf("rename = %d; want 200", code)
	}

	staffToken := staffLogin(t, server, "vera_api_company_identity")
	code, body := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/company", "")
	if code != http.StatusOK {
		t.Fatalf("staff identity = %d; want 200", code)
	}
	if body["legalName"] != "PT Live Identity" {
		t.Errorf("legalName = %v; want the stored name", body["legalName"])
	}
	if body["themeQDeep"] != "#0A1A5C" {
		t.Errorf("themeQDeep = %v; want the stored theme for desktop caches", body["themeQDeep"])
	}
	if _, leaked := body["logoPath"]; leaked {
		t.Error("logo path leaks into the identity DTO")
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodGet, "/v1/company", ""); code != http.StatusUnauthorized {
		t.Errorf("anon identity = %d; want 401", code)
	}
}
