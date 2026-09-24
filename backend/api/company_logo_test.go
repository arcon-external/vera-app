package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"vera-app/backend/store"
)

func TestCompanyLogoAdminRoundTrip(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_company_logo")

	artwork := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', 1, 2, 3})
	code, saved := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/logo",
		`{"logoBase64":"`+artwork+`","logoMime":"image/png"}`)
	if code != http.StatusOK {
		t.Fatalf("put logo = %d; want 200 (%v)", code, saved)
	}

	// The profile advertises the artwork without embedding bytes.
	code, profile := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/company", "")
	if code != http.StatusOK {
		t.Fatalf("get profile = %d; want 200", code)
	}
	if hasLogo, _ := profile["hasLogo"].(bool); !hasLogo {
		t.Error("profile hasLogo = false after upload")
	}
	if _, present := profile["logoBytes"]; present {
		t.Error("profile DTO embeds artwork bytes; they belong on the logo endpoint")
	}

	// Staff may read the artwork (it prints on customer documents) but never write it.
	staffToken := staffLogin(t, server, "vera_api_company_logo")
	code, seen := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/admin/company/logo", "")
	if code != http.StatusOK {
		t.Fatalf("staff get logo = %d; want 200", code)
	}
	if seen["logoBase64"] != artwork || seen["logoMime"] != "image/png" {
		t.Error("staff read back different artwork than stored")
	}
	if builtIn, _ := seen["builtIn"].(bool); builtIn {
		t.Error("uploaded artwork flagged builtIn")
	}
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPut, "/v1/admin/company/logo",
		`{"logoBase64":"`+artwork+`","logoMime":"image/png"}`); code != http.StatusForbidden {
		t.Errorf("staff put logo = %d; want 403", code)
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodGet, "/v1/admin/company/logo", ""); code != http.StatusUnauthorized {
		t.Errorf("anon get logo = %d; want 401", code)
	}

	// Validation rejects before storage.
	if code, _ := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/logo",
		`{"logoBase64":"`+artwork+`","logoMime":"application/pdf"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("bad mime = %d; want 422", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/logo",
		`{"logoBase64":"!!!","logoMime":"image/png"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("bad base64 = %d; want 422", code)
	}

	// Clearing restores the baked asset: the endpoint now serves the
	// built-in bytes with builtIn set instead of a 404.
	code, cleared := doAdminRequest(t, server, token, http.MethodDelete, "/v1/admin/company/logo", "")
	if code != http.StatusOK {
		t.Fatalf("clear logo = %d; want 200 (%v)", code, cleared)
	}
	code, builtin := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/company/logo", "")
	if code != http.StatusOK {
		t.Fatalf("get after clear = %d; want 200", code)
	}
	if builtIn, _ := builtin["builtIn"].(bool); !builtIn {
		t.Error("get after clear misses builtIn flag")
	}
	if builtin["logoBase64"] == "" || builtin["logoMime"] != "image/png" {
		t.Error("get after clear does not serve the baked-in artwork")
	}

	if count, err := database.AuditCount(context.Background(), "company.logo.update"); err != nil || count != 1 {
		t.Errorf("audit company.logo.update = %d, %v; want 1, nil", count, err)
	}
	if count, err := database.AuditCount(context.Background(), "company.logo.clear"); err != nil || count != 1 {
		t.Errorf("audit company.logo.clear = %d, %v; want 1, nil", count, err)
	}
}

func TestExportRendersUploadedLogo(t *testing.T) {
	resetVerifySchema(t, "vera_api_company_logo_render")
	server, token := testQuotationServer(t, "vera_api_company_logo_render")
	database, err := store.Open(context.Background(), store.Config{
		DSN:    strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")),
		Schema: "vera_api_company_logo_render",
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	seedQuotationClient(t, database, 2, "Test Customer")

	artwork := base64.StdEncoding.EncodeToString([]byte("custom-logo-bytes"))
	if code, _ := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/logo",
		`{"logoBase64":"`+artwork+`","logoMime":"image/png"}`); code != http.StatusOK {
		t.Fatalf("upload logo = %d; want 200", code)
	}

	code, exported := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, quotationInput())
	if code != http.StatusCreated {
		t.Fatalf("export = %d; want 201 (%v)", code, exported)
	}
	id, _ := exported["id"].(float64)
	quotePath := "/v1/quotations/" + strconv.Itoa(int(id))
	code, detail := doJSON(t, server, http.MethodGet, quotePath, token, nil)
	if code != http.StatusOK {
		t.Fatalf("detail = %d; want 200", code)
	}
	snapshot, _ := detail["htmlSnapshot"].(string)
	if !strings.Contains(snapshot, artwork) {
		t.Error("exported snapshot does not contain the uploaded artwork")
	}

	// Clearing restores the baked asset: the custom bytes must be gone.
	if code, _ := doAdminRequest(t, server, token, http.MethodDelete, "/v1/admin/company/logo", ""); code != http.StatusOK {
		t.Fatalf("clear logo = %d; want 200", code)
	}
	code, exported = doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, quotationInput())
	if code != http.StatusCreated {
		t.Fatalf("second export = %d; want 201", code)
	}
	id, _ = exported["id"].(float64)
	code, detail = doJSON(t, server, http.MethodGet, "/v1/quotations/"+strconv.Itoa(int(id)), token, nil)
	if code != http.StatusOK {
		t.Fatalf("second detail = %d; want 200", code)
	}
	if snapshot, _ := detail["htmlSnapshot"].(string); strings.Contains(snapshot, artwork) {
		t.Error("snapshot still contains cleared artwork; want the baked asset")
	}
}

func TestCompanyLogoOversizedRejected(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_company_logo_big")
	huge := base64.StdEncoding.EncodeToString(make([]byte, 512*1024+8))
	code, body := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/logo",
		`{"logoBase64":"`+huge+`","logoMime":"image/png"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("oversized logo = %d (%v); want 422", code, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(strings.ToLower(message), "512kb") {
		t.Errorf("oversized error = %v; want the limit message", body["error"])
	}
}
