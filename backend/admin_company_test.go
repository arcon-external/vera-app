package backend

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// companyProfileWireKey maps a CompanyProfile struct field name to the JSON
// key UpdateCompanyProfile sends for it on PUT /v1/admin/company.
//
// CompanyProfile carries no json tags, so there are no tags to compare
// against: the map's keys are lowerCamel wire names while the struct fields
// are PascalCase Go names. Every field lowercases to its wire key by dropping
// the first letter's case — except TaxID, whose wire name is "taxId" rather
// than the mechanical "taxID". That exception is pinned here rather than
// derived, so a reader meets it instead of guessing.
//
// HasLogo has no entry: it is a bool and the update map is map[string]string,
// so it can never travel on this wire. Neither update path writes it — the
// desktop forwards fourteen string columns and the server binds fourteen —
// and the store recomputes it from logo_bytes (len(LogoBytes) > 0). The test
// below asserts its absence to pin that contract.
func companyProfileWireKey(field string) (string, bool) {
	if field == "HasLogo" {
		return "", false
	}
	if field == "TaxID" {
		return "taxId", true
	}
	return strings.ToLower(field[:1]) + field[1:], true
}

// TestUpdateCompanyProfileSendsEveryField pins the hand-written update map in
// UpdateCompanyProfile against the CompanyProfile struct by reflection. The
// endpoint full-replaces: a struct field missing from the map never reaches
// the server, and the column is silently cleared on every save. A field added
// to the struct without adding its wire key fails here naming the field.
func TestUpdateCompanyProfileSendsEveryField(t *testing.T) {
	var gotBody map[string]any
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/admin/company" {
			t.Errorf("path = %s; want /v1/admin/company", request.URL.Path)
		}
		if request.Method != http.MethodPut {
			t.Errorf("method = %s; want PUT", request.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode PUT body: %v", err)
		}
		gotBody = body
		// Answer shaped like the real companyDTO: lowerCamel keys plus the
		// recomputed hasLogo, so the response decode path runs as in prod.
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"legalName": "PT Arcon Penta Persada", "address": "Jl. Merdeka 1, Jakarta",
			"phone": "+62-21-5550192", "email": "admin@arconpentapersada.co.id",
			"taxId": "01.234.567.8-901.234", "hasLogo": true, "currency": "USD",
			"themeQDeep": "#111111", "themeQAccent": "#222222",
			"themeQSoft": "#333333", "themeQInk": "#444444",
			"themePDeep": "#555555", "themePAccent": "#666666",
			"themePSoft": "#777777", "themePInk": "#888888",
		})
	})

	// Every string field carries a distinct sentinel so a swapped value fails
	// naming the field, not just a missing key.
	input := CompanyProfile{
		LegalName: "sentinel:LegalName", Address: "sentinel:Address",
		Phone: "sentinel:Phone", Email: "sentinel:Email", TaxID: "sentinel:TaxID",
		Currency:   "sentinel:Currency",
		ThemeQDeep: "sentinel:ThemeQDeep", ThemeQAccent: "sentinel:ThemeQAccent",
		ThemeQSoft: "sentinel:ThemeQSoft", ThemeQInk: "sentinel:ThemeQInk",
		ThemePDeep: "sentinel:ThemePDeep", ThemePAccent: "sentinel:ThemePAccent",
		ThemePSoft: "sentinel:ThemePSoft", ThemePInk: "sentinel:ThemePInk",
		HasLogo: true,
	}
	api := NewAdminAPI()
	updated, err := api.UpdateCompanyProfile(input)
	if err != nil {
		t.Fatalf("UpdateCompanyProfile failed: %v", err)
	}
	if updated.LegalName != "PT Arcon Penta Persada" {
		t.Errorf("updated.LegalName = %q; want the stubbed echo (response decode broken?)", updated.LegalName)
	}
	if gotBody == nil {
		t.Fatal("stub captured no PUT body")
	}

	// The expected wire set is derived from the struct, never listed by hand:
	// a new struct field lands here automatically.
	expected := map[string]string{}
	typ := reflect.TypeOf(CompanyProfile{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i).Name
		wire, ok := companyProfileWireKey(field)
		if !ok {
			continue
		}
		expected[wire] = field
	}

	var missing []string
	for wire, field := range expected {
		if _, present := gotBody[wire]; !present {
			missing = append(missing, "CompanyProfile."+field+" (wire "+quote(wire)+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("UpdateCompanyProfile dropped [%s] from the PUT body. "+
			"The endpoint full-replaces, so each missing field's column is cleared on every save.",
			strings.Join(missing, ", "))
	}

	var extra []string
	for wire := range gotBody {
		if _, want := expected[wire]; !want {
			extra = append(extra, quote(wire))
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("UpdateCompanyProfile sent unexpected wire keys [%s]; "+
			"the map drifted from CompanyProfile — check for a renamed field.",
			strings.Join(extra, ", "))
	}

	// Values must ride on the right keys: a swapped line fails naming it.
	for wire, field := range expected {
		raw, present := gotBody[wire]
		if !present {
			continue
		}
		if raw != "sentinel:"+field {
			t.Errorf("wire key %s carried %v; want the CompanyProfile.%s value",
				quote(wire), raw, field)
		}
	}

	// HasLogo is the documented exception: a bool that neither update path
	// writes. If it ever appears on the wire, the contract changed and the
	// comment on companyProfileWireKey must change with it.
	if _, present := gotBody["hasLogo"]; present {
		t.Error(`wire carries "hasLogo": UpdateCompanyProfile must not send it — ` +
			`the store recomputes HasLogo from logo_bytes`)
	}
}

func quote(s string) string {
	return `"` + s + `"`
}
