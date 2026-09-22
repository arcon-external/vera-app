package store

import (
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
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_company"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	seeded, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get seeded profile: %v", err)
	}
	if seeded.LegalName == "" {
		t.Error("seeded legal name is empty")
	}

	updated, err := database.UpdateCompanyProfile(ctx, CompanyProfile{
		LegalName: "  PT Example Persada  ",
		Address:   "Batam, Indonesia",
		Phone:     "+62-778-000000",
		Email:     "info@example.co.id",
		TaxID:     "00.000.000.0-000.000",
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.LegalName != "PT Example Persada" || updated.Email != "info@example.co.id" {
		t.Errorf("updated = %+v; want trimmed fields", updated)
	}
	if updated.LogoPath != "" {
		t.Errorf("logo path = %q; want stub empty", updated.LogoPath)
	}

	stored, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get stored profile: %v", err)
	}
	if stored != updated {
		t.Errorf("stored = %+v; want %+v", stored, updated)
	}

	for name, profile := range map[string]CompanyProfile{
		"empty name":  {LegalName: "  "},
		"bad email":   {LegalName: "PT X", Email: "not-an-email"},
		"long phone":  {LegalName: "PT X", Phone: strings.Repeat("1", 41)},
		"long tax ID": {LegalName: "PT X", TaxID: strings.Repeat("1", 41)},
		"long name":   {LegalName: strings.Repeat("X", 161)},
	} {
		if _, err := database.UpdateCompanyProfile(ctx, profile); err == nil {
			t.Errorf("%s accepted; want validation error", name)
		}
	}
	// Clearing optional fields is allowed.
	cleared, err := database.UpdateCompanyProfile(ctx, CompanyProfile{LegalName: "PT Example Persada"})
	if err != nil {
		t.Fatalf("clear optional fields: %v", err)
	}
	if cleared.Email != "" || cleared.Phone != "" || cleared.TaxID != "" || cleared.Address != "" {
		t.Errorf("cleared = %+v; want empty optionals", cleared)
	}
}
