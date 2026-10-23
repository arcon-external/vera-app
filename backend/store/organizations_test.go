package store

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Tenancy boundary (issue #465): the fleet is single-tenant today and the PO
// sprint must not create another global singleton. Migration 029 seeds exactly
// one organization from the company profile, and new tenant-owned operations
// require that organization in the context. These tests run at the existing
// store seam against disposable Postgres.

func TestOrganizationsSeedExactlyOneMatchingProfile(t *testing.T) {
	if strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")) == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_orgs")

	profile, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get company profile: %v", err)
	}

	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	if org.ID == "" {
		t.Error("seeded organization has an empty ID")
	}
	if org.Name != profile.LegalName {
		t.Errorf("seeded organization name = %q; want profile legal name %q", org.Name, profile.LegalName)
	}

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+database.configSchema()+".organizations").Scan(&count); err != nil {
		t.Fatalf("count organizations: %v", err)
	}
	if count != 1 {
		t.Errorf("organization count = %d; want exactly 1", count)
	}
}

func TestRequireOrgRefusesMissingContext(t *testing.T) {
	if strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")) == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	database := openTestSchema(t, "vera_test_orgs_noctx")

	if _, err := database.RequireOrg(context.Background()); err == nil {
		t.Error("RequireOrg without organization context succeeded; want refusal")
	}
	if _, err := database.RequireOrg(ContextWithOrg(context.Background(), "  ")); err == nil {
		t.Error("RequireOrg with a blank organization ID succeeded; want refusal")
	}
}

func TestRequireOrgAcceptsSeededOrg(t *testing.T) {
	if strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")) == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_orgs_ctx")

	seeded, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	got, err := database.RequireOrg(ContextWithOrg(ctx, seeded.ID))
	if err != nil {
		t.Fatalf("RequireOrg with seeded org: %v", err)
	}
	if got.ID != seeded.ID || got.Name != seeded.Name {
		t.Errorf("RequireOrg = %+v; want %+v", got, seeded)
	}
	if _, err := database.RequireOrg(ContextWithOrg(ctx, "00000000-0000-0000-0000-000000000000")); err == nil {
		t.Error("RequireOrg with an unknown organization ID succeeded; want refusal")
	}
}

func TestRequireOrgRefusesSuspendedOrg(t *testing.T) {
	if strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")) == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_orgs_susp")

	seeded, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE "+database.configSchema()+".organizations SET suspended_at = NOW() WHERE id = $1", seeded.ID); err != nil {
		t.Fatalf("suspend organization: %v", err)
	}
	if _, err := database.RequireOrg(ContextWithOrg(ctx, seeded.ID)); err == nil {
		t.Error("RequireOrg with a suspended organization succeeded; want refusal")
	}
}

// TestOrganizationSeedFollowsProfileAndFallsBack replays the migration 029
// seed statement against a live schema: a customized profile name wins on
// upgrade, and a missing profile row still yields the sensible default. It
// reads the real migration SQL so the assertion pins the shipped artifact,
// not a copy of it.
func TestOrganizationSeedFollowsProfileAndFallsBack(t *testing.T) {
	if strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")) == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_orgs_seed")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	migrations, err := loadMigrations("vera_test_orgs_seed")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var seed string
	for _, migration := range migrations {
		if migration.version == 29 {
			seed = migration.sql
		}
	}
	if seed == "" {
		t.Fatal("migration 29 not found")
	}

	execSeed := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, seed); err != nil {
			t.Fatalf("replay organization seed: %v", err)
		}
	}
	readOrgName := func() (string, int) {
		t.Helper()
		var name string
		var count int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*), MIN(name) FROM "+schema+".organizations").Scan(&count, &name); err != nil {
			t.Fatalf("read organizations: %v", err)
		}
		return name, count
	}

	// Idempotent: replaying the seed on an already-seeded schema changes nothing.
	execSeed()
	if _, count := readOrgName(); count != 1 {
		t.Fatalf("organization count after replay = %d; want 1", count)
	}

	// Upgrade: a customized profile name is picked up when the seed runs
	// before any organization exists.
	if _, err := database.UpdateCompanyGeneral(ctx, CompanyProfile{LegalName: "Custom Tenant Co"}); err != nil {
		t.Fatalf("customize profile: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM "+schema+".organizations"); err != nil {
		t.Fatalf("clear organizations: %v", err)
	}
	execSeed()
	if name, count := readOrgName(); count != 1 || name != "Custom Tenant Co" {
		t.Errorf("reseeded organization = %q (count %d); want %q (count 1)", name, count, "Custom Tenant Co")
	}

	// Fresh database without a profile row: the seed falls back instead of
	// inserting an empty name.
	if _, err := pool.Exec(ctx, "DELETE FROM "+schema+".organizations"); err != nil {
		t.Fatalf("clear organizations: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM "+schema+".company_profile"); err != nil {
		t.Fatalf("clear profile: %v", err)
	}
	execSeed()
	if name, count := readOrgName(); count != 1 || name != "PT Arcon Penta Persada" {
		t.Errorf("fallback organization = %q (count %d); want %q (count 1)", name, count, "PT Arcon Penta Persada")
	}
}
