package api

// Guards for the label endpoints (issue #339).
//
// The routes follow the same split as the units: every signed-in role reads
// the wording, because it prints on customer documents and the preview that
// renders it has to read it; only an admin replaces it. The save is a full
// replacement validated server-side, so a client cannot smuggle markup into a
// document.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"vera-app/backend/labels"
)

func TestCompanyLabelsRoundTrip(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_company_labels")

	code, got := doAdminRequest(t, server, token, http.MethodGet, "/v1/company/labels", "")
	if code != http.StatusOK {
		t.Fatalf("get = %d; want 200 (%v)", code, got)
	}
	entries, ok := got["labels"].([]any)
	if !ok || len(entries) != len(labels.LabelDefaults()) {
		t.Fatalf("get carried %v; want the whole label set", got["labels"])
	}
	// Every entry carries the effective value, the built-in default, and the
	// server's own rules: the frontend never hard-codes the document's wording
	// or a second copy of the validation caps.
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("entry %v is not an object", raw)
		}
		for _, key := range []string{"key", "value", "default", "maxLength", "terms"} {
			if _, present := entry[key]; !present {
				t.Errorf("entry %v carries no %q", entry, key)
			}
		}
		if entry["value"] != entry["default"] {
			t.Errorf("entry %v value != default on a fresh schema; want the built-in wording", entry)
		}
	}

	code, updated := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/labels",
		`{"labels":[{"key":"docTitle","value":"PROPOSAL"},{"key":"closing","value":"Thank you."}]}`)
	if code != http.StatusOK {
		t.Fatalf("put = %d; want 200 (%v)", code, updated)
	}
	if value := labelValue(t, updated, "docTitle"); value != "PROPOSAL" {
		t.Errorf("docTitle = %v; want PROPOSAL", value)
	}
	// The default rides along, so the editor can show what a cleared field
	// falls back to.
	if def := labelDefault(t, updated, "docTitle"); def != "QUOTATION" {
		t.Errorf("docTitle default = %v; want QUOTATION", def)
	}
	if value := labelValue(t, updated, "closing"); value != "Thank you." {
		t.Errorf("closing = %v; want the stored override", value)
	}
	if value := labelValue(t, updated, "subject"); value != labels.DefaultLabels().Subject {
		t.Errorf("subject = %v; want the built-in default", value)
	}

	// A full replacement is a replacement: a label left out loses its override.
	code, replaced := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/labels",
		`{"labels":[{"key":"subject","value":"Scope"}]}`)
	if code != http.StatusOK {
		t.Fatalf("put replace = %d; want 200 (%v)", code, replaced)
	}
	if value := labelValue(t, replaced, "docTitle"); value != "QUOTATION" {
		t.Errorf("docTitle = %v after a replacement that did not mention it; want the built-in", value)
	}
	if value := labelValue(t, replaced, "subject"); value != "Scope" {
		t.Errorf("subject = %v; want Scope", value)
	}

	// An empty value clears the label back to the built-in.
	code, cleared := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/labels",
		`{"labels":[{"key":"subject","value":""}]}`)
	if code != http.StatusOK {
		t.Fatalf("put clear = %d; want 200 (%v)", code, cleared)
	}
	if value := labelValue(t, cleared, "subject"); value != labels.DefaultLabels().Subject {
		t.Errorf("subject = %v after clearing; want the built-in default", value)
	}

	// The stored wording is what the quotation renders from: the store hands
	// the effective set to Prepare, and the HTML template and the native
	// renderer read it off the seam.
	if code, _ := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/labels",
		`{"labels":[{"key":"docTitle","value":"ESTIMATE"}]}`); code != http.StatusOK {
		t.Fatalf("put estimate = %d; want 200", code)
	}
	effective, err := database.GetCompanyLabels(context.Background())
	if err != nil {
		t.Fatalf("store labels: %v", err)
	}
	if effective.DocTitle != "ESTIMATE" {
		t.Errorf("stored docTitle = %q; want ESTIMATE", effective.DocTitle)
	}

	if count, err := database.AuditCount(context.Background(), "company.labels.update"); err != nil || count != 4 {
		t.Errorf("audit company.labels.update = %d, %v; want 4, nil", count, err)
	}
}

func TestCompanyLabelsValidation(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_company_labels_validation")

	for name, body := range map[string]string{
		"unknown key":     `{"labels":[{"key":"notALabel","value":"x"}]}`,
		"markup":          `{"labels":[{"key":"docTitle","value":"<b>Q</b>"}]}`,
		"template action": `{"labels":[{"key":"subject","value":"{{.Margin}}"}]}`,
		"newline":         `{"labels":[{"key":"subject","value":"a\nb"}]}`,
		"over the cap":    `{"labels":[{"key":"docTitle","value":"` + strings.Repeat("X", 25) + `"}]}`,
		"bad token":       `{"labels":[{"key":"termsPayment","value":"{amount}"}]}`,
	} {
		code, got := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/labels", body)
		if code != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d (%v); want 422", name, code, got)
			continue
		}
		message, _ := got["error"].(string)
		if !strings.Contains(message, "label") {
			t.Errorf("%s error = %q; want it to name the label", name, message)
		}
	}

	// An entry with no value is a clear: the wire treats empty and absent the
	// same way, which is what lets the editor restore a built-in by blanking a
	// field.
	if code, cleared := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/labels",
		`{"labels":[{"key":"docTitle"}]}`); code != http.StatusOK {
		t.Errorf("a valueless entry = %d (%v); want 200", code, cleared)
	} else if value := labelValue(t, cleared, "docTitle"); value != "QUOTATION" {
		t.Errorf("docTitle = %v after a valueless entry; want the built-in", value)
	}
}

func TestCompanyLabelsAreAdminOnly(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_company_labels_auth")

	staffToken := staffLogin(t, server, "vera_api_company_labels_auth")
	// Every signed-in role reads the wording: it prints on customer documents,
	// so hiding it would break the previews the editors render.
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/company/labels", ""); code != http.StatusOK {
		t.Errorf("staff get = %d; want 200", code)
	}
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPut, "/v1/admin/company/labels",
		`{"labels":[{"key":"docTitle","value":"HIJACK"}]}`); code != http.StatusForbidden {
		t.Errorf("staff put = %d; want 403", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodGet, "/v1/company/labels", ""); code != http.StatusOK {
		t.Errorf("admin get = %d; want 200", code)
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodGet, "/v1/company/labels", ""); code != http.StatusUnauthorized {
		t.Errorf("anon get = %d; want 401", code)
	}
	// The refused write must have changed nothing.
	code, got := doAdminRequest(t, server, token, http.MethodGet, "/v1/company/labels", "")
	if code != http.StatusOK {
		t.Fatalf("get = %d; want 200", code)
	}
	if value := labelValue(t, got, "docTitle"); value != "QUOTATION" {
		t.Errorf("docTitle = %v after a refused staff write; want the built-in", value)
	}
}

// labelValue and labelDefault read one entry out of the served label array, so
// a failure names the label rather than the whole response.
func labelValue(t *testing.T, response map[string]any, key string) string {
	t.Helper()
	entry := findLabel(t, response, key)
	value, _ := entry["value"].(string)
	return value
}

func labelDefault(t *testing.T, response map[string]any, key string) string {
	t.Helper()
	entry := findLabel(t, response, key)
	value, _ := entry["default"].(string)
	return value
}

func findLabel(t *testing.T, response map[string]any, key string) map[string]any {
	t.Helper()
	entries, ok := response["labels"].([]any)
	if !ok {
		t.Fatalf("response carries no labels array: %v", response)
	}
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if ok && entry["key"] == key {
			return entry
		}
	}
	t.Fatalf("the label set carries no %q: %v", key, response)
	return nil
}
