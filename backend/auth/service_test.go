package auth

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

	token, user, err := service.Login(ctx, "admin@example.com", "initial-password", false)
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

func TestLoginTrimsEmail(t *testing.T) {
	database := testStore(t, "vera_auth_test_trim")
	ctx := context.Background()
	service := NewService(database)
	if _, err := service.Bootstrap(ctx, "staff@example.com", "Staff", "password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Login(ctx, " staff@example.com ", "password", false); err != nil {
		t.Fatalf("pasted email with surrounding whitespace failed login: %v", err)
	}
}

func TestLoginUpgradesOldPasswordHashOnce(t *testing.T) {
	database := testStore(t, "vera_auth_test_rehash")
	ctx := context.Background()
	service := NewService(database)
	oldHash := oldParameterHash("old password")
	created, err := database.CreateUser(ctx, "old@example.com", "Old", oldHash, "staff")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Login(ctx, created.Email, "old password", false); err != nil {
		t.Fatalf("old hash login failed: %v", err)
	}
	upgraded, err := database.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.PasswordHash == oldHash || NeedsRehash(upgraded.PasswordHash) {
		t.Fatal("first login did not persist the current Argon2 parameters")
	}
	if upgraded.MustChangePassword != created.MustChangePassword {
		t.Fatal("rehash changed the must-change-password flag")
	}
	if _, _, err := service.Login(ctx, created.Email, "old password", false); err != nil {
		t.Fatalf("second login failed: %v", err)
	}
	again, err := database.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.PasswordHash != upgraded.PasswordHash {
		t.Fatal("second login rehashed an already-current password")
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
		if _, _, err := service.Login(ctx, "owner@example.com", password, false); err == nil {
			t.Fatalf("Login with %q succeeded", password)
		} else if err.Error() != "invalid email or password" {
			t.Errorf("error = %q; want generic message", err.Error())
		}
	}
	if _, _, err := service.Login(ctx, "nobody@example.com", "whatever", false); err == nil {
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

// sessionRow is the stored expiry state of the session a token identifies,
// read straight from the sessions row: the values under test are what the
// database keeps, not what the service remembers.
type sessionRow struct {
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
	Remembered        bool
}

func readSessionRow(t *testing.T, schema, token string) sessionRow {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")))
	if err != nil {
		t.Fatalf("connect to read session expiry: %v", err)
	}
	defer pool.Close()
	var row sessionRow
	if err := pool.QueryRow(context.Background(),
		`SELECT expires_at, absolute_expires_at, remembered FROM "`+
			strings.ReplaceAll(schema, `"`, `""`)+`".sessions WHERE token_hash = $1`,
		HashSessionToken(token)).Scan(&row.ExpiresAt, &row.AbsoluteExpiresAt, &row.Remembered); err != nil {
		t.Fatalf("read session expiry: %v", err)
	}
	return row
}

func TestLoginIssuesTheLifetimeTheClientAskedFor(t *testing.T) {
	database := testStore(t, "vera_auth_test_lifetime")
	ctx := context.Background()
	service := NewService(database)
	if _, err := service.Bootstrap(ctx, "lifetime@example.com", "Lifetime", "initial-password"); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	// A fake clock: the expiry is stamped from it, so the assertion is
	// exact rather than a tolerance around time.Now().
	fixed := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }

	for _, tc := range []struct {
		name       string
		remember   bool
		wantIdle   time.Duration
		wantCap    time.Duration
		remembered bool
	}{
		// The values are written out, not the constants: a test that
		// asserts the same symbol the implementation picks proves the
		// wiring but pins nothing about the duration.
		{"unremembered session idles for a working day under a week cap", false, 8 * time.Hour, 7 * 24 * time.Hour, false},
		{"remembered session idles for thirty days under a ninety-day cap", true, 30 * 24 * time.Hour, 90 * 24 * time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, _, err := service.Login(ctx, "lifetime@example.com", "initial-password", tc.remember)
			if err != nil {
				t.Fatalf("Login failed: %v", err)
			}
			row := readSessionRow(t, "vera_auth_test_lifetime", token)
			if want := fixed.Add(tc.wantIdle); !row.ExpiresAt.Equal(want) {
				t.Errorf("expires_at = %s; want %s (issued %s with remember=%v, one of the lifetimes is wrong)",
					row.ExpiresAt.Format(time.RFC3339), want.Format(time.RFC3339), fixed.Format(time.RFC3339), tc.remember)
			}
			if want := fixed.Add(tc.wantCap); !row.AbsoluteExpiresAt.Equal(want) {
				t.Errorf("absolute_expires_at = %s; want %s (the cap is wrong for remember=%v)",
					row.AbsoluteExpiresAt.Format(time.RFC3339), want.Format(time.RFC3339), tc.remember)
			}
			if row.Remembered != tc.remembered {
				t.Errorf("remembered = %v; want %v", row.Remembered, tc.remembered)
			}
		})
	}
}

// TestRenewSessionSlidesTheIdleDeadlineByTheSessionsOwnLifetime pins the
// service half of renewal: a standard session slides by a working day and a
// remembered one by thirty days, and neither moves the absolute cap.
func TestRenewSessionSlidesTheIdleDeadlineByTheSessionsOwnLifetime(t *testing.T) {
	database := testStore(t, "vera_auth_test_renew")
	ctx := context.Background()
	service := NewService(database)
	if _, err := service.Bootstrap(ctx, "renew@example.com", "Renew", "initial-password"); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	issued := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return issued }

	for _, tc := range []struct {
		name     string
		remember bool
		idle     time.Duration
	}{
		{"standard", false, 8 * time.Hour},
		{"remembered", true, 30 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, _, err := service.Login(ctx, "renew@example.com", "initial-password", tc.remember)
			if err != nil {
				t.Fatalf("Login failed: %v", err)
			}
			before := readSessionRow(t, "vera_auth_test_renew", token)

			later := issued.Add(2 * time.Hour)
			service.now = func() time.Time { return later }
			info, err := service.RenewSession(ctx, token)
			if err != nil {
				t.Fatalf("RenewSession: %v", err)
			}
			if want := later.Add(tc.idle); !info.ExpiresAt.Equal(want) {
				t.Errorf("renewed expires_at = %s; want %s", info.ExpiresAt, want)
			}
			if !info.AbsoluteExpiresAt.Equal(before.AbsoluteExpiresAt) {
				t.Errorf("renewal moved the absolute cap: %s -> %s",
					before.AbsoluteExpiresAt, info.AbsoluteExpiresAt)
			}
			if info.Remembered != tc.remember {
				t.Errorf("renewed remembered = %v; want %v", info.Remembered, tc.remember)
			}
		})
	}
}

// TestRedeemRevokesTheSessionsTheOldPasswordAuthorized is the revocation
// gap: a redeemed invite or reset used to leave every session the old
// password had issued alive. Now that a remembered session lives a month,
// that is a month of access with a password the holder has already rotated.
func TestRedeemRevokesTheSessionsTheOldPasswordAuthorized(t *testing.T) {
	database := testStore(t, "vera_auth_test_redeem_revoke")
	ctx := context.Background()
	service := NewService(database)
	admin, err := service.Bootstrap(ctx, "redeem@example.com", "Redeem", "initial-password")
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// A session the holder is actively using when the password is reset.
	token, _, err := service.Login(ctx, "redeem@example.com", "initial-password", false)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	invite, err := database.IssueInviteToken(ctx, admin.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("IssueInviteToken failed: %v", err)
	}
	hash, err := HashPassword("rotated-password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if _, err := database.RedeemInviteToken(ctx, invite, hash, time.Now()); err != nil {
		t.Fatalf("RedeemInviteToken failed: %v", err)
	}

	if _, err := service.Validate(ctx, token); err == nil {
		t.Fatal("a session issued before the password reset still validates; want it revoked with the reset")
	}
	// The redemption really took effect: the new password signs in, and
	// the old one no longer does.
	if _, _, err := service.Login(ctx, "redeem@example.com", "rotated-password", true); err != nil {
		t.Fatalf("Login with the redeemed password failed: %v", err)
	}
	if _, _, err := service.Login(ctx, "redeem@example.com", "initial-password", true); err == nil {
		t.Error("Login with the rotated-away password succeeded; want failure")
	}
}
