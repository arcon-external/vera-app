package store

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

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
