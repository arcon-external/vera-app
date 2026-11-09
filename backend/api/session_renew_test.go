package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/auth"
)

// Ticket #536: POST /v1/auth/renew slides the idle deadline of the session
// the bearer token names, never past the absolute cap, and answers the same
// coded 401 the middleware uses when the session has genuinely ended.

// renewTestPool opens a pool on the API test schema so a test can move a
// session's deadlines directly; the renewal rules are what is under test, not
// the clock.
func renewTestPool(t *testing.T, schema string) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test schema: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func setSessionDeadlines(t *testing.T, pool *pgxpool.Pool, schema, token string, idle, absolute time.Time) {
	t.Helper()
	command, err := pool.Exec(context.Background(),
		`UPDATE "`+strings.ReplaceAll(schema, `"`, `""`)+`".sessions
		 SET expires_at = $2, absolute_expires_at = $3
		 WHERE token_hash = $1`,
		auth.HashSessionToken(token), idle, absolute)
	if err != nil {
		t.Fatalf("set session deadlines: %v", err)
	}
	if command.RowsAffected() != 1 {
		t.Fatalf("set session deadlines: %d rows affected; want 1", command.RowsAffected())
	}
}

func parseSessionTime(t *testing.T, body map[string]any, field string) time.Time {
	t.Helper()
	raw, ok := body[field].(string)
	if !ok || raw == "" {
		t.Fatalf("%s = %v; want an RFC3339 string", field, body[field])
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("parse %s %q: %v", field, raw, err)
	}
	return parsed
}

func TestMeCarriesTheSessionExpiry(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_536_me_expiry")

	code, body := doAdminRequest(t, server, token, http.MethodGet, "/v1/auth/me", "")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/auth/me = %d; want 200", code)
	}
	idle := parseSessionTime(t, body, "expiresAt")
	absolute := parseSessionTime(t, body, "absoluteExpiresAt")
	if !absolute.After(idle) {
		t.Errorf("absoluteExpiresAt = %s; want it after expiresAt %s", absolute, idle)
	}
	if canRenew, _ := body["canRenew"].(bool); !canRenew {
		t.Error("canRenew = false; a fresh standard session must be renewable")
	}
	if remembered, _ := body["remembered"].(bool); remembered {
		t.Error("remembered = true; the admin test setup signs in without it")
	}
}

func TestRenewBeforeTheAbsoluteCapExtendsTheIdleExpiry(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_536_renew_before_cap")
	pool := renewTestPool(t, "vera_536_renew_before_cap")

	// Put the session an hour into its life, with its idle deadline an hour
	// out and the cap a week away: renewal must move the idle deadline
	// without touching the cap.
	now := time.Now().UTC().Truncate(time.Second)
	capAt := now.Add(7 * 24 * time.Hour)
	setSessionDeadlines(t, pool, "vera_536_renew_before_cap", token, now.Add(time.Hour), capAt)

	code, body := doAdminRequest(t, server, token, http.MethodPost, "/v1/auth/renew", "")
	if code != http.StatusOK {
		t.Fatalf("POST /v1/auth/renew = %d %v; want 200", code, body)
	}
	renewed := parseSessionTime(t, body, "expiresAt")
	if !renewed.After(now.Add(time.Hour)) {
		t.Errorf("expiresAt = %s; renewal did not extend the idle deadline", renewed)
	}
	if got := parseSessionTime(t, body, "absoluteExpiresAt"); !got.Equal(capAt) {
		t.Errorf("absoluteExpiresAt = %s; renewal moved the cap; want %s", got, capAt)
	}
	if canRenew, _ := body["canRenew"].(bool); !canRenew {
		t.Error("canRenew = false after a renewal below the cap")
	}

	// The renewed token is still the same live session.
	if code, _ := doAdminRequest(t, server, token, http.MethodGet, "/v1/auth/me", ""); code != http.StatusOK {
		t.Errorf("GET /v1/auth/me after renewal = %d; want 200", code)
	}
}

func TestRenewClampsToTheAbsoluteCap(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_536_renew_clamp")
	pool := renewTestPool(t, "vera_536_renew_clamp")

	now := time.Now().UTC().Truncate(time.Second)
	capAt := now.Add(30 * time.Minute)
	setSessionDeadlines(t, pool, "vera_536_renew_clamp", token, now.Add(5*time.Minute), capAt)

	code, body := doAdminRequest(t, server, token, http.MethodPost, "/v1/auth/renew", "")
	if code != http.StatusOK {
		t.Fatalf("POST /v1/auth/renew = %d %v; want 200", code, body)
	}
	if got := parseSessionTime(t, body, "expiresAt"); !got.Equal(capAt) {
		t.Errorf("expiresAt = %s; want the cap %s, not now + 8h", got, capAt)
	}
	if canRenew, _ := body["canRenew"].(bool); canRenew {
		t.Error("canRenew = true at the cap; there is nothing left to extend")
	}
}

func TestRenewAfterTheAbsoluteCapIsACoded401(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_536_renew_after_cap")
	pool := renewTestPool(t, "vera_536_renew_after_cap")

	now := time.Now().UTC().Truncate(time.Second)
	setSessionDeadlines(t, pool, "vera_536_renew_after_cap", token,
		now.Add(24*time.Hour), now.Add(-time.Minute))

	code, body := doAdminRequest(t, server, token, http.MethodPost, "/v1/auth/renew", "")
	if code != http.StatusUnauthorized || body["code"] != "session_expired" {
		t.Fatalf("renew past the cap = %d %v; want coded 401 session_expired", code, body)
	}
	// The middleware agrees, so no other route treats the session as live.
	code, body = doAdminRequest(t, server, token, http.MethodGet, "/v1/auth/me", "")
	if code != http.StatusUnauthorized || body["code"] != "session_expired" {
		t.Fatalf("me past the cap = %d %v; want coded 401 session_expired", code, body)
	}
}

func TestRenewEndedSessionsAreCoded401s(t *testing.T) {
	server, database, _ := adminTestSetup(t, "vera_536_renew_ended")

	revoked := doLogin(t, server, "api-admin@example.com", "test-password")
	if code, _ := doAdminRequest(t, server, revoked, http.MethodPost, "/v1/auth/logout", ""); code != http.StatusNoContent {
		t.Fatalf("logout = %d; want 204", code)
	}
	code, body := doAdminRequest(t, server, revoked, http.MethodPost, "/v1/auth/renew", "")
	if code != http.StatusUnauthorized || body["code"] != "session_revoked" {
		t.Fatalf("renew a revoked session = %d %v; want coded 401 session_revoked", code, body)
	}

	code, body = doAdminRequest(t, server, "", http.MethodPost, "/v1/auth/renew", "")
	if code != http.StatusUnauthorized || body["code"] != "session_missing" {
		t.Fatalf("renew without a token = %d %v; want coded 401 session_missing", code, body)
	}

	code, body = doAdminRequest(t, server, "not-a-session", http.MethodPost, "/v1/auth/renew", "")
	if code != http.StatusUnauthorized || body["code"] != "session_invalid" {
		t.Fatalf("renew an unknown token = %d %v; want coded 401 session_invalid", code, body)
	}

	// An idle-expired session is the same typed answer, not a 503.
	seedStaff(t, database, "idle-expired@example.com", "staff-password")
	expiredUser, err := database.GetUserByEmail(context.Background(), "idle-expired@example.com")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := database.CreateSession(context.Background(), expiredUser.ID,
		auth.HashSessionToken("idle-expired-token"), now.Add(-time.Minute), now.Add(24*time.Hour), false); err != nil {
		t.Fatal(err)
	}
	code, body = doAdminRequest(t, server, "idle-expired-token", http.MethodPost, "/v1/auth/renew", "")
	if code != http.StatusUnauthorized || body["code"] != "session_expired" {
		t.Fatalf("renew an idle-expired session = %d %v; want coded 401 session_expired", code, body)
	}
}

// TestRenewOperationalFailureIs503 keeps a database outage out of the 401
// bucket: the desktop must not treat it as a session end (#531).
func TestRenewOperationalFailureIs503(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_536_renew_operational")
	database.Close()

	code, body := doAdminRequest(t, server, token, http.MethodPost, "/v1/auth/renew", "")
	if code != http.StatusServiceUnavailable || body["code"] != "session_check_failed" {
		t.Fatalf("renew with a closed database = %d %v; want coded 503", code, body)
	}
}
