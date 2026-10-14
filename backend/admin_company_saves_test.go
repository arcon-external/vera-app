package backend

// Guards for the narrow company writes (issue #432).
//
// The Company settings section saves one tab at a time: General, Units,
// Quotation (wording + colours) and Portal. Each has its own endpoint that
// writes only its own columns, which is what lets the whole-record
// UpdateCompanyProfile keep working untouched for older clients — a
// whole-record save that a single tab used would clear the columns the other
// tabs own.
//
// These guards pin what each narrow endpoint puts on the wire. A key that
// drifts in is a column the save did not mean to write; a value that lands on
// the wrong key is a colour saved into another colour's column, which nobody
// notices until a document prints it.

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// companyStub answers every company route with the shape companyDTO serves,
// so the response-decode half of each binding runs as it does in production.
func companyStub(t *testing.T, wantPath string, gotBody *map[string]any) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != wantPath {
			t.Errorf("path = %s; want %s", request.URL.Path, wantPath)
		}
		if request.Method != http.MethodPut {
			t.Errorf("method = %s; want PUT", request.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode PUT body: %v", err)
		}
		*gotBody = body
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"legalName": "PT Arcon Penta Persada", "address": "Jl. Merdeka 1, Jakarta",
			"phone": "+62-21-5550192", "email": "admin@arconpentapersada.co.id",
			"taxId": "01.234.567.8-901.234", "hasLogo": true, "currency": "USD",
			"canonicalUoM": "Lot",
			"themeQDeep":   "#111111", "themeQAccent": "#222222",
			"themeQSoft": "#333333", "themeQInk": "#444444",
			"themePDeep": "#555555", "themePAccent": "#666666",
			"themePSoft": "#777777", "themePInk": "#888888",
		})
	}
}

// assertWireKeys checks a captured body's key set and the value on each key.
// A missing key is a field the save drops; an extra key is a column the save
// writes that it does not own.
func assertWireKeys(t *testing.T, got map[string]any, want map[string]string) {
	t.Helper()
	var missing []string
	for key, value := range want {
		raw, present := got[key]
		if !present {
			missing = append(missing, quote(key))
			continue
		}
		if raw != value {
			t.Errorf("wire key %s carried %v; want %q", quote(key), raw, value)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("the PUT body is missing [%s]", strings.Join(missing, ", "))
	}

	var extra []string
	for key := range got {
		if _, want := want[key]; !want {
			extra = append(extra, quote(key))
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("the PUT body carries [%s], which this save does not own",
			strings.Join(extra, ", "))
	}
}

// TestUpdateCompanyGeneralSendsOnlyItsOwnFields pins the narrow General save:
// exactly the six General-tab keys travel, and no theme colour and no
// canonical unit. A payload carrying them would let a rename clear the brand.
func TestUpdateCompanyGeneralSendsOnlyItsOwnFields(t *testing.T) {
	var gotBody map[string]any
	stubAPIClient(t, companyStub(t, "/v1/admin/company/general", &gotBody))

	api := NewAdminAPI()
	updated, err := api.UpdateCompanyGeneral(
		"PT Arcon Penta Persada", "Jl. Merdeka 1, Jakarta", "+62-21-5550192",
		"admin@arconpentapersada.co.id", "01.234.567.8-901.234", "USD")
	if err != nil {
		t.Fatalf("UpdateCompanyGeneral failed: %v", err)
	}
	if gotBody == nil {
		t.Fatal("stub captured no PUT body")
	}
	assertWireKeys(t, gotBody, map[string]string{
		"legalName": "PT Arcon Penta Persada",
		"address":   "Jl. Merdeka 1, Jakarta",
		"phone":     "+62-21-5550192",
		"email":     "admin@arconpentapersada.co.id",
		"taxId":     "01.234.567.8-901.234",
		"currency":  "USD",
	})

	// The echoed profile decodes, so the caller can refresh from the answer.
	if updated.LegalName != "PT Arcon Penta Persada" || updated.Currency != "USD" {
		t.Errorf("updated = %+v; want the stubbed echo (response decode broken?)", updated)
	}
	if !reflect.DeepEqual(updated.ThemeQDeep, "#111111") {
		t.Errorf("updated.ThemeQDeep = %q; want the stubbed echo", updated.ThemeQDeep)
	}
}

// TestUpdateQuotationThemeSendsOnlyTheQuotationColours pins the quotation
// colour save: four keys, the four bases, and none of the portal's. Rebranding
// the document must not repaint the verification page.
func TestUpdateQuotationThemeSendsOnlyTheQuotationColours(t *testing.T) {
	var gotBody map[string]any
	stubAPIClient(t, companyStub(t, "/v1/admin/company/theme/quotation", &gotBody))

	api := NewAdminAPI()
	if _, err := api.UpdateQuotationTheme("#1A237E", "#284AF3", "#D6E4FF", "#1a1a1a"); err != nil {
		t.Fatalf("UpdateQuotationTheme failed: %v", err)
	}
	if gotBody == nil {
		t.Fatal("stub captured no PUT body")
	}
	assertWireKeys(t, gotBody, map[string]string{
		"deep": "#1A237E", "accent": "#284AF3", "soft": "#D6E4FF", "ink": "#1a1a1a",
	})
}

// TestUpdatePortalThemeSendsOnlyThePortalColours is the other half of the
// brand, on its own path with the same four keys.
func TestUpdatePortalThemeSendsOnlyThePortalColours(t *testing.T) {
	var gotBody map[string]any
	stubAPIClient(t, companyStub(t, "/v1/admin/company/theme/portal", &gotBody))

	api := NewAdminAPI()
	if _, err := api.UpdatePortalTheme("#0A1A5C", "#3B82F6", "#DBEAFE", "#111827"); err != nil {
		t.Fatalf("UpdatePortalTheme failed: %v", err)
	}
	if gotBody == nil {
		t.Fatal("stub captured no PUT body")
	}
	assertWireKeys(t, gotBody, map[string]string{
		"deep": "#0A1A5C", "accent": "#3B82F6", "soft": "#DBEAFE", "ink": "#111827",
	})
}

// TestNarrowThemeWritesSurfaceARefusal proves a refused save is an error the
// caller sees rather than a success it acts on: the store refuses a colour
// that is not a strict #RRGGBB with 422, and the binding must hand that
// message back so the editor can show it beside the field.
func TestNarrowThemeWritesSurfaceARefusal(t *testing.T) {
	for _, path := range []string{"/v1/admin/company/theme/quotation", "/v1/admin/company/theme/portal"} {
		t.Run(path, func(t *testing.T) {
			stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"error": "company theme quotation deep must be a #RRGGBB color",
				})
			})
			api := NewAdminAPI()
			var err error
			if strings.HasSuffix(path, "quotation") {
				_, err = api.UpdateQuotationTheme("deep-blue", "", "", "")
			} else {
				_, err = api.UpdatePortalTheme("deep-blue", "", "", "")
			}
			if err == nil {
				t.Fatal("a 422 came back as a nil error: the editor would report a save that did not happen")
			}
			if !strings.Contains(err.Error(), "must be a #RRGGBB") {
				t.Errorf("error = %q; want the store's refusal message", err)
			}
		})
	}
}

// TestUpdateCompanyUnitsSendsTheCanonicalUoM pins that the Units tab owns the
// canonical unit now: it rides the same PUT as the list, under its own key,
// and an empty value still travels (the store reads it as "keep the stored
// one", which is what a list-only save means).
func TestUpdateCompanyUnitsSendsTheCanonicalUoM(t *testing.T) {
	var gotBody map[string]any
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/admin/company/units" {
			t.Errorf("path = %s; want /v1/admin/company/units", request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode PUT body: %v", err)
		}
		gotBody = body
		_ = json.NewEncoder(writer).Encode(map[string]any{"units": []map[string]string{
			{"value": "Lot", "label": "Lot"},
			{"value": "m2", "label": "m²"},
		}})
	})

	api := NewAdminAPI()
	units, err := api.UpdateCompanyUnits([]CompanyUnit{
		{Value: "Lot", Label: "Lot"}, {Value: "m2", Label: "m²"},
	}, "m2")
	if err != nil {
		t.Fatalf("UpdateCompanyUnits failed: %v", err)
	}
	if gotBody == nil {
		t.Fatal("stub captured no PUT body")
	}
	if gotBody["canonicalUoM"] != "m2" {
		t.Errorf("canonicalUoM = %v; want \"m2\" beside the list it was validated against", gotBody["canonicalUoM"])
	}
	rows, ok := gotBody["units"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("units = %v; want the two rows under the \"units\" key", gotBody["units"])
	}

	// An empty canonical keeps the stored value, and it still travels: the
	// request struct binds it whether or not the tab moved it.
	gotBody = nil
	if _, err := api.UpdateCompanyUnits([]CompanyUnit{{Value: "Lot", Label: "Lot"}}, ""); err != nil {
		t.Fatalf("list-only save failed: %v", err)
	}
	if raw, present := gotBody["canonicalUoM"]; !present || raw != "" {
		t.Errorf("canonicalUoM = %v (present %v); want an empty value kept on the wire", raw, present)
	}
	if len(units) != 2 || units[1].Label != "m²" {
		t.Errorf("units = %+v; want the two rows the stub echoed", units)
	}
}
