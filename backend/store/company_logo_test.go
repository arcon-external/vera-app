package store

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestCompanyLogoRoundTrip(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_company_logo"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	// A previous failed run may have left artwork behind; start clean.
	if _, err := database.SetCompanyLogo(ctx, nil, ""); err != nil {
		t.Fatalf("clean logo: %v", err)
	}

	seeded, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if seeded.HasLogo {
		t.Fatal("fresh profile reports a logo")
	}

	artwork := bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, 256)
	updated, err := database.SetCompanyLogo(ctx, artwork, "image/png")
	if err != nil {
		t.Fatalf("set logo: %v", err)
	}
	if !updated.HasLogo || updated.LogoMime != "image/png" {
		t.Errorf("updated = hasLogo %v mime %q; want true image/png", updated.HasLogo, updated.LogoMime)
	}
	if !bytes.Equal(updated.LogoBytes, artwork) {
		t.Error("stored bytes differ from uploaded artwork")
	}

	stored, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if !stored.HasLogo || !bytes.Equal(stored.LogoBytes, artwork) {
		t.Error("reloaded profile lost the artwork")
	}

	cleared, err := database.SetCompanyLogo(ctx, nil, "")
	if err != nil {
		t.Fatalf("clear logo: %v", err)
	}
	if cleared.HasLogo || len(cleared.LogoBytes) != 0 {
		t.Error("cleared profile still reports artwork")
	}
}

func TestCompanyLogoValidation(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_company_logo_invalid"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	small := []byte{1, 2, 3}
	if _, err := database.SetCompanyLogo(ctx, small, "application/pdf"); err == nil {
		t.Error("PDF artwork accepted; want a type rejection")
	} else if !strings.Contains(err.Error(), "PNG") {
		t.Errorf("type error = %v; want the allowlist message", err)
	}
	huge := bytes.Repeat([]byte{0}, 512*1024+1)
	if _, err := database.SetCompanyLogo(ctx, huge, "image/png"); err == nil {
		t.Error("oversized artwork accepted; want a size rejection")
	} else if !strings.Contains(err.Error(), "512KB") {
		t.Errorf("size error = %v; want the limit message", err)
	}
	if _, err := database.SetCompanyLogo(ctx, small, ""); err == nil {
		t.Error("empty MIME accepted; want a type rejection")
	}
}
