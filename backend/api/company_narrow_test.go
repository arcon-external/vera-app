package api

// Guards for the narrow company writes (issue #432), the per-tab saves the
// settings section makes: General, Quotation colours, Portal colours, and the
// Units list with its canonical unit.
//
// What these pin is the scoping. The whole-record PUT /v1/admin/company
// writes fourteen columns and therefore requires its caller to resend all of
// them; each narrow route writes only its own, which is what lets a save from
// one settings tab leave the other tabs' columns exactly as they were. A
// regressed handler here is a colour or a canonical unit that silently moved
// because someone saved a name.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"vera-app/backend/store"
)

// storedProfile reads the singleton row, so a narrow save can be checked
// against what the migration seeded rather than against literals that would
// drift with the palette.
func storedProfile(t *testing.T, database *store.Store) store.CompanyProfile {
	t.Helper()
	profile, err := database.GetCompanyProfile(context.Background())
	if err != nil {
		t.Fatalf("get company profile: %v", err)
	}
	return profile
}

func databaseCompanyUnits(t *testing.T, database *store.Store) []store.CompanyUnit {
	t.Helper()
	units, err := database.GetCompanyUnits(context.Background())
	if err != nil {
		t.Fatalf("get company units: %v", err)
	}
	return units
}

func TestCompanyGeneralEndpointRoundTrip(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_company_general")

	// Read the seeded row before the save, not after it: comparing the profile
	// to itself proves nothing, and a handler that wrote a colour column would
	// still look clean. (Caught by the red proof, which is why this line is
	// where it is.)
	seeded := storedProfile(t, database)
	if seeded.ThemeQDeep == "" || seeded.CanonicalUoM == "" {
		t.Fatalf("seeded profile carries no theme or canonical: %+v", seeded)
	}

	code, updated := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/general",
		`{"legalName":"  PT Narrow Save  ","address":"Batam","phone":"+62-778-1","email":"narrow@example.co.id","taxId":"00.000.000.0-000.000","currency":"usd"}`)
	if code != http.StatusOK {
		t.Fatalf("put general = %d; want 200 (%v)", code, updated)
	}
	if updated["legalName"] != "PT Narrow Save" || updated["address"] != "Batam" {
		t.Errorf("updated = %v; want the trimmed six General fields", updated)
	}
	if updated["currency"] != "USD" {
		t.Errorf("currency = %v; want the store's upper-cased catalogue code", updated["currency"])
	}

	// The columns General does not own are untouched by the save above.
	after := storedProfile(t, database)
	if after.ThemeQDeep != seeded.ThemeQDeep || after.ThemePDeep != seeded.ThemePDeep ||
		after.CanonicalUoM != seeded.CanonicalUoM {
		t.Errorf("a General save moved the brand: %s/%s -> %s/%s, canonical %q -> %q",
			seeded.ThemeQDeep, seeded.ThemePDeep, after.ThemeQDeep, after.ThemePDeep,
			seeded.CanonicalUoM, after.CanonicalUoM)
	}

	// Validation is the store's, mapped to 422 the way the settings form shows it.
	for _, bad := range []string{
		`{"legalName":"  "}`,
		`{"legalName":"PT X","email":"not-an-email"}`,
		`{"legalName":"PT X","currency":"Rupiah"}`,
		`{"legalName":"PT X","phone":"` + strings.Repeat("1", 41) + `"}`,
	} {
		if code, body := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/general", bad); code != http.StatusUnprocessableEntity {
			t.Errorf("put %s = %d (%v); want 422", bad, code, body)
		}
	}

	staffToken := staffLogin(t, server, "vera_api_company_general")
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPut, "/v1/admin/company/general",
		`{"legalName":"PT Hijack"}`); code != http.StatusForbidden {
		t.Errorf("staff put general = %d; want 403", code)
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodPut, "/v1/admin/company/general",
		`{"legalName":"PT Anon"}`); code != http.StatusUnauthorized {
		t.Errorf("anon put general = %d; want 401", code)
	}

	if count, err := database.AuditCount(context.Background(), "company.update"); err != nil || count != 1 {
		t.Errorf("audit company.update = %d, %v; want 1, nil", count, err)
	}
}

func TestCompanyThemeEndpointsWriteOnlyTheirColumns(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_company_theme")
	before := storedProfile(t, database)

	code, updated := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/theme/quotation",
		`{"deep":"#0A1A5C","accent":"#3B82F6","soft":"#DBEAFE","ink":"#111827"}`)
	if code != http.StatusOK {
		t.Fatalf("put quotation theme = %d; want 200 (%v)", code, updated)
	}
	if updated["themeQDeep"] != "#0A1A5C" || updated["themeQInk"] != "#111827" {
		t.Errorf("updated = %v; want the four quotation colours written", updated)
	}
	if updated["themePDeep"] != before.ThemePDeep || updated["themePInk"] != before.ThemePInk {
		t.Errorf("portal theme moved to %s/%s; want the seeds %s/%s",
			updated["themePDeep"], updated["themePInk"], before.ThemePDeep, before.ThemePInk)
	}

	code, updated = doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/theme/portal",
		`{"deep":"#123456","accent":"#234567","soft":"#345678","ink":"#456789"}`)
	if code != http.StatusOK {
		t.Fatalf("put portal theme = %d; want 200 (%v)", code, updated)
	}
	if updated["themePDeep"] != "#123456" || updated["themePInk"] != "#456789" {
		t.Errorf("updated = %v; want the four portal colours written", updated)
	}
	// The quotation half keeps what the save above wrote: the two halves are
	// independent columns, and a portal save must not repaint every quotation.
	if updated["themeQDeep"] != "#0A1A5C" || updated["themeQInk"] != "#111827" {
		t.Errorf("quotation theme moved to %s/%s; want #0A1A5C/#111827",
			updated["themeQDeep"], updated["themeQInk"])
	}

	// Strict hex, refused naming the field, on both halves.
	for _, path := range []string{"quotation", "portal"} {
		for _, bad := range []string{
			`{"deep":"deep-blue"}`,
			`{"accent":"#FFF"}`,
			`{"soft":"D6E4FF"}`,
			`{"ink":"#12345G"}`,
		} {
			if code, body := doAdminRequest(t, server, token, http.MethodPut,
				"/v1/admin/company/theme/"+path, bad); code != http.StatusUnprocessableEntity {
				t.Errorf("put %s theme %s = %d (%v); want 422", path, bad, code, body)
			}
		}
		// An empty field is legal: it means "use the stylesheet default".
		if code, _ := doAdminRequest(t, server, token, http.MethodPut,
			"/v1/admin/company/theme/"+path, `{"deep":""}`); code != http.StatusOK {
			t.Errorf("clearing the %s deep colour = %d; want 200", path, code)
		}
	}

	// The staff gate is the same on both halves. The account is the same
	// either way, so it logs in once.
	staffToken := staffLogin(t, server, "vera_api_company_theme")
	for _, path := range []string{"quotation", "portal"} {
		if code, _ := doAdminRequest(t, server, staffToken, http.MethodPut,
			"/v1/admin/company/theme/"+path, `{"deep":"#000000"}`); code != http.StatusForbidden {
			t.Errorf("staff put %s theme = %d; want 403", path, code)
		}
	}
	// Two audits per half: the write above and the clear. The four bad-hex
	// refusals are deliberately absent -- a refused save writes nothing, so
	// there is nothing to record.
	for _, action := range []string{"company.quotation-theme.update", "company.portal-theme.update"} {
		if count, err := database.AuditCount(context.Background(), action); err != nil || count != 2 {
			t.Errorf("audit %s = %d, %v; want 2, nil", action, count, err)
		}
	}
}

func TestCompanyUnitsEndpointMovesTheCanonical(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_company_units")

	// The canonical rides the same PUT as the list and is validated against
	// the list being written, not the one it replaces.
	code, body := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/units",
		`{"units":[{"value":"Pc","label":"piece"},{"value":"EA","label":"each"}],"canonicalUoM":"Pc"}`)
	if code != http.StatusOK {
		t.Fatalf("put units = %d; want 200 (%v)", code, body)
	}
	profile := storedProfile(t, database)
	if profile.CanonicalUoM != "Pc" {
		t.Errorf("canonical unit = %q; want the %q the save named", profile.CanonicalUoM, "Pc")
	}
	if len(databaseCompanyUnits(t, database)) != 2 {
		t.Errorf("unit list did not take the new rows")
	}

	// A canonical the new list does not carry is a 422 naming the unit.
	if code, body := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/units",
		`{"units":[{"value":"Lot","label":"Lot"},{"value":"EA","label":"each"}],"canonicalUoM":"m2"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("off-list canonical = %d (%v); want 422", code, body)
	} else if message, _ := body["error"].(string); message == "" ||
		!namesBoth(message, "canonical unit", "m2") {
		t.Errorf("error = %q; want it to name the unit it refused", message)
	}

	// An absent canonical keeps the stored one: an old client that does not
	// know the field must not be able to move it, and must not be able to
	// break a save it can still make.
	if code, body := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/units",
		`{"units":[{"value":"Pc","label":"piece"},{"value":"EA","label":"each"}]}`); code != http.StatusOK {
		t.Fatalf("list-only put = %d; want 200 (%v)", code, body)
	}
	if got := storedProfile(t, database).CanonicalUoM; got != "Pc" {
		t.Errorf("canonical unit = %q after a list-only save; want it kept \"Pc\"", got)
	}

	// A list that drops the canonical is still refused, with the message that
	// names the unit that must stay.
	if code, body := doAdminRequest(t, server, token, http.MethodPut, "/v1/admin/company/units",
		`{"units":[{"value":"Lot","label":"Lot"}]}`); code != http.StatusUnprocessableEntity {
		t.Errorf("dropping the canonical = %d (%v); want 422", code, body)
	} else if message, _ := body["error"].(string); message == "" ||
		!namesBoth(message, "canonical unit", "Pc") {
		t.Errorf("error = %q; want it to name the canonical unit that must stay", message)
	}
	if got := storedProfile(t, database).CanonicalUoM; got != "Pc" {
		t.Errorf("canonical unit = %q after a refused save; want it kept \"Pc\"", got)
	}
}

// namesBoth reports whether a refusal message carries both parts: the kind of
// refusal and the value it objected to. A message that names only one leaves
// the admin guessing which field to fix.
func namesBoth(message, kind, value string) bool {
	return strings.Contains(message, kind) && strings.Contains(message, value)
}
