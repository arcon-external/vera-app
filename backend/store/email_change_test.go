package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// emailTestUsers creates two staff users with test-scoped emails, cleaning
// any leftovers first so reruns on a warm database behave like a fresh one.
func emailTestUsers(t *testing.T, database *Store) (userA, userB AuthUser) {
	t.Helper()
	ctx := context.Background()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	for _, email := range []string{"email-a@example.com", "email-b@example.com", "email-new@example.com"} {
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			`DELETE FROM %s.users WHERE LOWER(email) = LOWER($1)`, database.configSchema()), email); err != nil {
			t.Fatalf("clean user %s: %v", email, err)
		}
	}
	// CreateUser only requires a non-empty hash; these rows never log in.
	const hash = "test-hash-not-for-login"
	if userA, err = database.CreateUser(ctx, "email-a@example.com", "Email A", hash, "staff"); err != nil {
		t.Fatalf("create user A: %v", err)
	}
	if userB, err = database.CreateUser(ctx, "email-b@example.com", "Email B", hash, "staff"); err != nil {
		t.Fatalf("create user B: %v", err)
	}
	return userA, userB
}

func TestSetUserEmailFlipsLoginIdentity(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_email_set")
	userA, userB := emailTestUsers(t, database)

	old, fresh, err := database.SetUserEmail(ctx, userA.ID, " email-new@example.com ")
	if err != nil {
		t.Fatalf("set email: %v", err)
	}
	if old != "email-a@example.com" || fresh != "email-new@example.com" {
		t.Errorf("set = %q -> %q; want trimmed flip", old, fresh)
	}
	if _, err := database.GetUserByEmail(ctx, "EMAIL-NEW@EXAMPLE.COM"); err != nil {
		t.Errorf("lookup by new email (any case) failed: %v", err)
	}
	if _, err := database.GetUserByEmail(ctx, "email-a@example.com"); err == nil {
		t.Error("old email still resolves; want it dead")
	}

	if _, _, err := database.SetUserEmail(ctx, userB.ID, "EMAIL-NEW@example.com"); err == nil {
		t.Error("collision with another user accepted; want already-in-use")
	} else if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("collision error = %v; want already-in-use", err)
	}
	if _, _, err := database.SetUserEmail(ctx, userB.ID, "not-an-email"); err == nil {
		t.Error("malformed email accepted; want invalid")
	}
	if _, _, err := database.SetUserEmail(ctx, 999999, "ghost@example.com"); err == nil {
		t.Error("unknown user accepted; want not-found")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown-user error = %v; want not-found", err)
	}
}

func TestEmailChangeRequestFlow(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_email_flow")
	userA, userB := emailTestUsers(t, database)

	first, err := database.RequestEmailChange(ctx, userA.ID, "email-new@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if first.Status != "pending" || first.ProposedEmail != "email-new@example.com" {
		t.Errorf("request = %+v; want pending email-new", first)
	}
	// Second request while pending returns the existing row, not a new one.
	again, err := database.RequestEmailChange(ctx, userA.ID, "email-other@example.com")
	if err != nil {
		t.Fatalf("duplicate request: %v", err)
	}
	if again.ID != first.ID || again.ProposedEmail != "email-new@example.com" {
		t.Errorf("duplicate = %+v; want existing row %d", again, first.ID)
	}
	if _, err := database.RequestEmailChange(ctx, userB.ID, "email-b@example.com"); err == nil {
		t.Error("request for current email accepted; want rejection")
	}
	if _, err := database.RequestEmailChange(ctx, userB.ID, "email-a@example.com"); err == nil {
		t.Error("request for taken email accepted; want already-in-use")
	}

	pending, err := database.ListEmailChangeRequests(ctx)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != first.ID {
		t.Fatalf("pending = %+v; want exactly the one request", pending)
	}

	decided, err := database.DecideEmailChange(ctx, first.ID, userB.ID, true)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if decided.Status != "approved" {
		t.Errorf("status = %q; want approved", decided.Status)
	}
	if user, err := database.GetUserByID(ctx, userA.ID); err != nil {
		t.Fatalf("fetch user: %v", err)
	} else if user.Email != "email-new@example.com" {
		t.Errorf("email = %q; want flipped", user.Email)
	}
	if _, err := database.DecideEmailChange(ctx, first.ID, userB.ID, false); err == nil {
		t.Error("re-decide accepted; want already-decided")
	}

	mine, err := database.ListUserEmailChangeRequests(ctx, userA.ID)
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine) != 1 || mine[0].Status != "approved" {
		t.Errorf("mine = %+v; want the approved outcome visible", mine)
	}
	if _, err := database.DecideEmailChange(ctx, 999999, userB.ID, true); err == nil {
		t.Error("unknown request decided; want not-found")
	}
}

func TestEmailChangeRejectLeavesIdentity(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_email_reject")
	userA, userB := emailTestUsers(t, database)

	request, err := database.RequestEmailChange(ctx, userA.ID, "email-new@example.com")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	decided, err := database.DecideEmailChange(ctx, request.ID, userB.ID, false)
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if decided.Status != "rejected" {
		t.Errorf("status = %q; want rejected", decided.Status)
	}
	if user, err := database.GetUserByID(ctx, userA.ID); err != nil {
		t.Fatalf("fetch user: %v", err)
	} else if user.Email != "email-a@example.com" {
		t.Errorf("email = %q; want unchanged", user.Email)
	}
	if pending, err := database.ListEmailChangeRequests(ctx); err != nil {
		t.Fatalf("list pending: %v", err)
	} else if len(pending) != 0 {
		t.Errorf("pending = %d; want empty after reject", len(pending))
	}
}

func TestEmailWritesNormalizeAndLegacyMigration(t *testing.T) {
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_email_normalize")
	created, err := database.CreateUser(ctx, "  MiXeD@Example.COM  ", "Mixed", "test-hash", "staff")
	if err != nil || created.Email != "mixed@example.com" {
		t.Fatalf("create email = %q, %v; want normalized", created.Email, err)
	}
	_, changed, err := database.SetUserEmail(ctx, created.ID, "  ReNaMeD@Example.COM ")
	if err != nil || changed != "renamed@example.com" {
		t.Fatalf("direct change = %q, %v; want normalized", changed, err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatal(err)
	}
	// Mimic a row written before migration 028; then execute the embedded
	// migration twice to prove normalization and idempotence.
	if _, err := pool.Exec(ctx, fmt.Sprintf("UPDATE %s.users SET email = $1 WHERE id = $2", database.configSchema()),
		"  LeGaCy@Example.COM  ", created.ID); err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(database.config.Schema)
	if err != nil {
		t.Fatal(err)
	}
	var normalizeSQL string
	for _, migration := range migrations {
		if migration.version == 28 {
			normalizeSQL = migration.sql
			break
		}
	}
	if normalizeSQL == "" {
		t.Fatal("migration 028 missing")
	}
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, normalizeSQL); err != nil {
			t.Fatalf("migration 028 run %d: %v", i+1, err)
		}
	}
	user, err := database.GetUserByID(ctx, created.ID)
	if err != nil || user.Email != "legacy@example.com" {
		t.Fatalf("migrated email = %q, %v; want normalized", user.Email, err)
	}
}
