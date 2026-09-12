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
	database, err := store.Open(ctx, store.Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	service := auth.NewService(database)
	if _, err := service.Bootstrap(ctx, "api-admin@example.com", "API Admin", "test-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return NewServer(database, service), ctx
}

func TestHealthzNeedsNoAuth(t *testing.T) {
	server := NewServer(nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d; want 200", recorder.Code)
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
