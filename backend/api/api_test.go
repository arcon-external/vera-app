package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

func testServer(t *testing.T, schema string) (*Server, context.Context) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	ctx := context.Background()
	// See adminTestSetup: bootstrapping needs a schema with no users yet.
	resetTestSchema(t, schema)
	database, err := store.Open(ctx, store.Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	service := auth.NewService(database)
	if _, err := service.Bootstrap(ctx, "api-admin@example.com", "API Admin", "test-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return NewServer(database, service, nil), ctx
}

func TestHealthzNeedsNoAuth(t *testing.T) {
	server := NewServer(nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d; want 200", recorder.Code)
	}
}

// Monitors (e.g. UptimeRobot) poll with HEAD; both health paths must
// answer it like their GET twins.
func TestHealthHead(t *testing.T) {
	server := NewServer(nil, nil, nil)
	for _, path := range []string{"/healthz", "/health"} {
		request := httptest.NewRequest(http.MethodHead, path, nil)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("HEAD %s = %d; want 200", path, recorder.Code)
		}
	}
}

func TestLoginLogoutMeFlow(t *testing.T) {
	server, _ := testServer(t, "vera_api_test_flow")

	token := doLogin(t, server, "api-admin@example.com", "test-password")

	request := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/auth/me = %d; want 200", recorder.Code)
	}
	var me struct {
		Email string
		Role  string
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me.Email != "api-admin@example.com" || me.Role != "admin" {
		t.Errorf("me = %+v; want admin user", me)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("POST /v1/auth/logout = %d; want 204", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("GET /v1/auth/me after logout = %d; want 401", recorder.Code)
	}
}

func TestLoginRejectsBadCredentialsGenerically(t *testing.T) {
	server, _ := testServer(t, "vera_api_test_reject")

	body := `{"email":"api-admin@example.com","password":"wrong"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d; want 401", recorder.Code)
	}
	var problem struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if problem.Error != "invalid email or password" {
		t.Errorf("error = %q; want generic message", problem.Error)
	}
}

func TestProtectedRoutesRejectMissingToken(t *testing.T) {
	server, _ := testServer(t, "vera_api_test_guard")

	for _, target := range []struct{ method, path string }{
		{http.MethodGet, "/v1/auth/me"},
		{http.MethodPost, "/v1/auth/logout"},
	} {
		request := httptest.NewRequest(target.method, target.path, nil)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token = %d; want 401", target.method, target.path, recorder.Code)
		}
	}
}

// sessionExpiryAfterLogin signs in with the given wire body and reads the
// expiry the server stamped on the session it issued, straight from the
// sessions row: the value under test is the one the database keeps.
func sessionExpiryAfterLogin(t *testing.T, server *Server, schema, body string) time.Time {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login = %d; want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Token string
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	pool, err := pgxpool.New(context.Background(),
		strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")))
	if err != nil {
		t.Fatalf("connect to read session expiry: %v", err)
	}
	defer pool.Close()
	var expiresAt time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT expires_at FROM "`+strings.ReplaceAll(schema, `"`, `""`)+`".sessions WHERE token_hash = $1`,
		auth.HashSessionToken(result.Token)).Scan(&expiresAt); err != nil {
		t.Fatalf("read session expiry: %v", err)
	}
	return expiresAt
}

// TestLoginRememberSelectsTheSessionLifetime pins the wire: the remember
// field picks the remembered lifetime, and its absence — the payload every
// older client sends — keeps the standard one.
func TestLoginRememberSelectsTheSessionLifetime(t *testing.T) {
	server, _, _ := adminTestSetup(t, "vera_api_login_remember")
	const email = "api-admin@example.com"

	for _, tc := range []struct {
		name string
		body string
		want time.Duration
	}{
		// The values are written out, not the auth package's constants:
		// the assertion is about what a client can expect on the wire.
		{"absent remember field keeps the standard lifetime",
			`{"email":"` + email + `","password":"test-password"}`, 8 * time.Hour},
		{"remember false keeps the standard lifetime",
			`{"email":"` + email + `","password":"test-password","remember":false}`, 8 * time.Hour},
		{"remember true buys the remembered lifetime",
			`{"email":"` + email + `","password":"test-password","remember":true}`, 30 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expiresAt := sessionExpiryAfterLogin(t, server, "vera_api_login_remember", tc.body)
			// The server signs it at issuance with the real clock, so the
			// lifetime is checked as the span between now and the expiry.
			got := expiresAt.Sub(time.Now())
			const tolerance = 2 * time.Minute
			if got < tc.want-tolerance || got > tc.want+tolerance {
				t.Errorf("session lives %s; want %s (±%s): expires_at %s",
					got.Round(time.Second), tc.want, tolerance, expiresAt.Format(time.RFC3339))
			}
		})
	}
}

func doLogin(t *testing.T, server interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, email, password string) string {
	t.Helper()
	body := `{"email":` + strconv.Quote(email) + `,"password":` + strconv.Quote(password) + `}`
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login = %d; want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Token string
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if result.Token == "" {
		t.Fatal("empty bearer token")
	}
	return result.Token
}
