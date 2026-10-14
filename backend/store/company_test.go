package store

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

// The narrow company writes (issue #432) are the per-tab saves: General,
// Quotation colours, Portal colours. Each writes only its own columns, so a
// save from one tab cannot clear what another tab owns — the property the
// whole-record UpdateCompanyProfile cannot promise, because it must resend
// everything. These tests pin the column scoping and the shared validation.

func TestUpdateCompanyGeneralWritesOnlyItsOwnColumns(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_company_general")

	// Seed the whole record first, so a save that writes more than it should
	// shows up as a changed column rather than as a default.
	if _, err := database.UpdateCompanyProfile(ctx, CompanyProfile{
		LegalName:    "PT Arcon Penta Persada",
		Address:      "Batam, Indonesia",
		Phone:        "+62-778-000000",
		Email:        "info@example.co.id",
		TaxID:        "00.000.000.0-000.000",
		Currency:     "USD",
		ThemeQDeep:   "#0A1A5C",
		ThemeQAccent: "#3B82F6",
		ThemeQSoft:   "#DBEAFE",
		ThemeQInk:    "#111827",
		ThemePDeep:   "#123456",
		ThemePAccent: "#234567",
		ThemePSoft:   "#345678",
		ThemePInk:    "#456789",
		CanonicalUoM: "m2",
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	updated, err := database.UpdateCompanyGeneral(ctx, CompanyProfile{
		LegalName: "  PT Example Persada  ",
		Address:   "Jakarta, Indonesia",
		Phone:     "+62-21-5550192",
		Email:     "admin@example.co.id",
		TaxID:     "01.234.567.8-901.234",
		Currency:  "eur",
	})
	if err != nil {
		t.Fatalf("update general: %v", err)
	}
	// Trimmed and normalized exactly as the whole-record path does: one rule,
	// one place.
	if updated.LegalName != "PT Example Persada" || updated.Address != "Jakarta, Indonesia" {
		t.Errorf("updated = %+v; want trimmed fields", updated)
	}
	if updated.Currency != "EUR" {
		t.Errorf("currency = %q; want a catalogue code upper-cased", updated.Currency)
	}
	if updated.Email != "admin@example.co.id" || updated.TaxID != "01.234.567.8-901.234" ||
		updated.Phone != "+62-21-5550192" {
		t.Errorf("updated = %+v; want the six General fields written", updated)
	}

	// The columns General does not own are untouched: the eight theme colours
	// and the canonical unit still hold what the whole-record save stored.
	stored, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get stored profile: %v", err)
	}
	for name, got := range map[string]string{
		"ThemeQDeep": stored.ThemeQDeep, "ThemeQAccent": stored.ThemeQAccent,
		"ThemeQSoft": stored.ThemeQSoft, "ThemeQInk": stored.ThemeQInk,
		"ThemePDeep": stored.ThemePDeep, "ThemePAccent": stored.ThemePAccent,
		"ThemePSoft": stored.ThemePSoft, "ThemePInk": stored.ThemePInk,
		"CanonicalUoM": stored.CanonicalUoM,
	} {
		if got == "" {
			t.Errorf("stored %s is empty: the General save cleared a column it does not own", name)
		}
	}
	if stored.ThemeQDeep != "#0A1A5C" || stored.ThemePDeep != "#123456" {
		t.Errorf("stored theme = %s/%s; want the seeds #0A1A5C/#123456", stored.ThemeQDeep, stored.ThemePDeep)
	}
	if stored.CanonicalUoM != "m2" {
		t.Errorf("stored canonical = %q; want the seeded \"m2\"", stored.CanonicalUoM)
	}

	// The same validation the whole-record path applies, because both run
	// normalizeCompanyGeneral: a name is required, an email must parse.
	for name, profile := range map[string]CompanyProfile{
		"empty name": {LegalName: "  "},
		"bad email":  {LegalName: "PT X", Email: "not-an-email"},
		"long phone": {LegalName: "PT X", Phone: strings.Repeat("1", 41)},
		"long name":  {LegalName: strings.Repeat("X", 161)},
		"bad code":   {LegalName: "PT X", Currency: "Rupiah"},
	} {
		if _, err := database.UpdateCompanyGeneral(ctx, profile); err == nil {
			t.Errorf("%s accepted; want the same refusal the whole-record save gives", name)
		}
	}
}

func TestUpdateQuotationThemeWritesOnlyTheQuotationColumns(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_company_q_theme")

	if _, err := database.UpdateCompanyProfile(ctx, CompanyProfile{
		LegalName:  "PT Arcon Penta Persada",
		ThemePDeep: "#123456",
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	// Empty is a legal value: it means "use the stylesheet default", so a
	// colour cleared from the picker saves as clear rather than being refused.
	updated, err := database.UpdateQuotationTheme(ctx, "#0A1A5C", "", "#DBEAFE", "#111827")
	if err != nil {
		t.Fatalf("update quotation theme: %v", err)
	}
	if updated.ThemeQDeep != "#0A1A5C" || updated.ThemeQAccent != "" ||
		updated.ThemeQSoft != "#DBEAFE" || updated.ThemeQInk != "#111827" {
		t.Errorf("updated quotation theme = %+v; want the four values written, blank kept blank", updated)
	}
	if updated.ThemePDeep != "#123456" {
		t.Errorf("portal deep = %q; want the portal half untouched by a quotation save", updated.ThemePDeep)
	}

	stored, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get stored profile: %v", err)
	}
	if stored.ThemeQDeep != "#0A1A5C" || stored.ThemePDeep != "#123456" {
		t.Errorf("stored theme = %s/%s; want the quotation half moved and the portal half kept",
			stored.ThemeQDeep, stored.ThemePDeep)
	}

	// Strict hex, the same rule checkTheme applies to the whole-record save.
	for _, bad := range []string{"deep-blue", "#FFF", "D6E4FF", "#12345G"} {
		if _, err := database.UpdateQuotationTheme(ctx, bad, "", "", ""); err == nil ||
			!strings.Contains(err.Error(), "company theme quotation deep must be a #RRGGBB color") {
			t.Errorf("quotation deep %q: err = %v; want a #RRGGBB refusal naming the field", bad, err)
		}
	}
}

func TestUpdatePortalThemeWritesOnlyThePortalColumns(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_company_p_theme")

	if _, err := database.UpdateQuotationTheme(ctx, "#0A1A5C", "#3B82F6", "#DBEAFE", "#111827"); err != nil {
		t.Fatalf("seed quotation theme: %v", err)
	}

	updated, err := database.UpdatePortalTheme(ctx, "#123456", "#234567", "#345678", "#456789")
	if err != nil {
		t.Fatalf("update portal theme: %v", err)
	}
	if updated.ThemePDeep != "#123456" || updated.ThemePInk != "#456789" {
		t.Errorf("updated portal theme = %+v; want the four portal values written", updated)
	}
	if updated.ThemeQDeep != "#0A1A5C" || updated.ThemeQInk != "#111827" {
		t.Errorf("updated quotation theme = %+v; want the quotation half untouched by a portal save", updated)
	}

	for _, bad := range []string{"#FFF", "228B22", "#gggggg"} {
		if _, err := database.UpdatePortalTheme(ctx, "", bad, "", ""); err == nil ||
			!strings.Contains(err.Error(), "company theme portal accent must be a #RRGGBB color") {
			t.Errorf("portal accent %q: err = %v; want a #RRGGBB refusal naming the field", bad, err)
		}
	}
}

func TestCompanyProfileRoundTrip(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_company_theme")

	seeded, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get seeded profile: %v", err)
	}
	if seeded.LegalName == "" {
		t.Error("seeded legal name is empty")
	}
	if seeded.Currency != "IDR" {
		t.Errorf("seeded currency = %q; migration defaults must carry IDR", seeded.Currency)
	}
	for _, field := range [][2]string{
		{"themeQDeep", seeded.ThemeQDeep},
		{"themeQAccent", seeded.ThemeQAccent},
		{"themeQSoft", seeded.ThemeQSoft},
		{"themeQInk", seeded.ThemeQInk},
		{"themePDeep", seeded.ThemePDeep},
		{"themePAccent", seeded.ThemePAccent},
		{"themePSoft", seeded.ThemePSoft},
		{"themePInk", seeded.ThemePInk},
	} {
		if field[1] == "" {
			t.Errorf("seeded %s is empty; migration defaults must carry today's palette", field[0])
		}
	}

	updated, err := database.UpdateCompanyProfile(ctx, CompanyProfile{
		LegalName:    "  PT Example Persada  ",
		Address:      "Batam, Indonesia",
		Phone:        "+62-778-000000",
		Email:        "info@example.co.id",
		TaxID:        "00.000.000.0-000.000",
		Currency:     "usd",
		ThemeQDeep:   "#0A1A5C",
		ThemeQAccent: "#3B82F6",
		ThemeQSoft:   "#DBEAFE",
		ThemeQInk:    "#111827",
		ThemePDeep:   "#0A1A5C",
		ThemePAccent: "#3B82F6",
		ThemePSoft:   "#DBEAFE",
		ThemePInk:    "#111827",
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.LegalName != "PT Example Persada" || updated.Email != "info@example.co.id" {
		t.Errorf("updated = %+v; want trimmed fields", updated)
	}
	// Lower-case codes store upper-cased: the column carries catalogue codes.
	if updated.Currency != "USD" {
		t.Errorf("updated currency = %q; want USD", updated.Currency)
	}
	if updated.ThemeQDeep != "#0A1A5C" || updated.ThemePInk != "#111827" {
		t.Errorf("updated theme = %+v; want stored hex values", updated)
	}
	if updated.LogoPath != "" {
		t.Errorf("logo path = %q; want stub empty", updated.LogoPath)
	}

	stored, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get stored profile: %v", err)
	}
	// CompanyProfile holds a []byte artwork field, so compare field by field.
	if stored.LegalName != updated.LegalName || stored.Address != updated.Address ||
		stored.Phone != updated.Phone || stored.Email != updated.Email ||
		stored.TaxID != updated.TaxID || stored.LogoPath != updated.LogoPath ||
		stored.LogoMime != updated.LogoMime || stored.HasLogo != updated.HasLogo ||
		!bytes.Equal(stored.LogoBytes, updated.LogoBytes) {
		t.Errorf("stored = %+v; want %+v", stored, updated)
	}

	for name, profile := range map[string]CompanyProfile{
		"empty name":   {LegalName: "  "},
		"bad email":    {LegalName: "PT X", Email: "not-an-email"},
		"long phone":   {LegalName: "PT X", Phone: strings.Repeat("1", 41)},
		"long tax ID":  {LegalName: "PT X", TaxID: strings.Repeat("1", 41)},
		"long name":    {LegalName: strings.Repeat("X", 161)},
		"bad theme":    {LegalName: "PT X", ThemeQDeep: "deep-blue"},
		"short theme":  {LegalName: "PT X", ThemePAccent: "#FFF"},
		"hashless":     {LegalName: "PT X", ThemeQSoft: "D6E4FF"},
		"bad currency": {LegalName: "PT X", Currency: "Rupiah"},
		"short code":   {LegalName: "PT X", Currency: "US"},
		"padded code":  {LegalName: "PT X", Currency: " USD "},
	} {
		_, err := database.UpdateCompanyProfile(ctx, profile)
		switch name {
		case "padded code":
			// Surrounding spaces trim before lookup: a pasted " USD " stores.
			if err != nil {
				t.Errorf("%s rejected; want normalization, got %v", name, err)
			}
		default:
			if err == nil {
				t.Errorf("%s accepted; want validation error", name)
			} else if name == "bad currency" || name == "short code" {
				if !strings.Contains(err.Error(), "company currency") {
					t.Errorf("%s error = %q; want it to name the company currency field", name, err)
				}
			}
		}
	}
	// Clearing optional fields is allowed. The currency is not clearable:
	// empty writes as the default.
	cleared, err := database.UpdateCompanyProfile(ctx, CompanyProfile{LegalName: "PT Example Persada"})
	if err != nil {
		t.Fatalf("clear optional fields: %v", err)
	}
	if cleared.Email != "" || cleared.Phone != "" || cleared.TaxID != "" || cleared.Address != "" {
		t.Errorf("cleared = %+v; want empty optionals", cleared)
	}
	if cleared.Currency != "IDR" {
		t.Errorf("cleared currency = %q; want the IDR default", cleared.Currency)
	}
}
