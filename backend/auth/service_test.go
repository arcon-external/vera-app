package auth

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/store"
)

// resetAuthSchema drops a test schema so a rerun on a warm database behaves
// exactly like a fresh one. The tests below bootstrap a user, which refuses
// once one exists, so without this the second run of the suite fails with
// "bootstrap is disabled: users already exist".
func resetAuthSchema(t *testing.T, schema string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")))
	if err != nil {
		t.Fatalf("connect for schema reset: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		`DROP SCHEMA IF EXISTS "`+strings.ReplaceAll(schema, `"`, `""`)+`" CASCADE`); err != nil {
		t.Fatalf("reset schema %s: %v", schema, err)
	}
}

func testStore(t *testing.T, schema string) *store.Store {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run auth integration tests")
	}
	resetAuthSchema(t, schema)
	database, err := store.Open(context.Background(), store.Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	return database
}

func TestAuthLoginLogoutFlow(t *testing.T) {
	database := testStore(t, "vera_auth_test_flow")
	ctx := context.Background()
	service := NewService(database)

	admin, err := service.Bootstrap(ctx, "admin@example.com", "Admin", "initial-password")
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if admin.Role != "admin" {
		t.Errorf("bootstrap role = %q; want admin", admin.Role)
	}
	if _, err := service.Bootstrap(ctx, "second@example.com", "Second", "password"); err == nil {
		t.Fatal("second Bootstrap succeeded; must refuse once users exist")
	}

	token, user, err := service.Login(ctx, "admin@example.com", "initial-password")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if token == "" {
		t.Fatal("empty session token")
	}

	resolved, err := service.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if resolved.ID != user.ID {
		t.Errorf("validated user %d; want %d", resolved.ID, user.ID)
	}

	if err := service.Logout(ctx, token); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
	if _, err := service.Validate(ctx, token); err == nil {
		t.Fatal("revoked session still validates")
	}
}

func TestAuthRejectsBadCredentialsGenerically(t *testing.T) {
	database := testStore(t, "vera_auth_test_reject")
	ctx := context.Background()
	service := NewService(database)

	if _, err := service.Bootstrap(ctx, "owner@example.com", "Owner", "right-password"); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	for _, password := range []string{"wrong-password", ""} {
		if _, _, err := service.Login(ctx, "owner@example.com", password); err == nil {
			t.Fatalf("Login with %q succeeded", password)
		} else if err.Error() != "invalid email or password" {
			t.Errorf("error = %q; want generic message", err.Error())
		}
	}
	if _, _, err := service.Login(ctx, "nobody@example.com", "whatever"); err == nil {
		t.Fatal("Login with unknown email succeeded")
	} else if err.Error() != "invalid email or password" {
		t.Errorf("unknown-email error = %q; want generic message", err.Error())
	}
}

func TestAuthRoleGate(t *testing.T) {
	staff := struct{ Role string }{Role: "staff"}
	if err := RequireRole(store.AuthUser{Role: staff.Role}, "admin"); err == nil {
		t.Fatal("staff passed the admin gate")
	}
	if err := RequireRole(store.AuthUser{Role: "admin"}, "admin"); err != nil {
		t.Errorf("admin denied by the admin gate: %v", err)
	}
}

func TestCreateUserRequiresPassword(t *testing.T) {
	database := testStore(t, "vera_auth_test_password")
	ctx := context.Background()
	if _, err := database.CreateUser(ctx, "nopass@example.com", "No Pass", "", "staff"); err == nil {
		t.Fatal("CreateUser without a password hash succeeded; email/password is mandatory")
	}
}
