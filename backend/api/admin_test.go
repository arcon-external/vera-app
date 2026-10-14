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

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// adminTestSetup opens an isolated schema, migrates, and bootstraps one admin.
// It returns the server, DB handle (for seeding), and an admin bearer token.
func adminTestSetup(t *testing.T, schema string) (*Server, *store.Store, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	ctx := context.Background()
	// Warm-database reruns must behave like a fresh one: this setup
	// bootstraps an admin, which fails if a previous run left one behind.
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
	server := NewServer(database, service, nil)
	token := doLogin(t, server, "api-admin@example.com", "test-password")
	return server, database, token
}

func seedStaff(t *testing.T, database *store.Store, email, password string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := database.CreateUser(context.Background(), email, "Staff", hash, "staff"); err != nil {
		t.Fatalf("seed staff: %v", err)
	}
}

func doAdminRequest(t *testing.T, server *Server, token, method, path, body string) (int, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	var decoded map[string]any
	if len(recorder.Body.Bytes()) > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s: decode response: %v", method, path, err)
		}
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	return recorder.Code, decoded
}

func TestStaffDeniedAdminEndpoints(t *testing.T) {
	server, database, adminToken := adminTestSetup(t, "vera_api_admin_gates")
	seedStaff(t, database, "api-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "api-staff@example.com", "staff-password")
	_ = adminToken

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/v1/admin/users", ""},
		{http.MethodPost, "/v1/admin/users", `{"email":"x@example.com","displayName":"X","role":"staff"}`},
		{http.MethodPatch, "/v1/admin/users/1/role", `{"role":"staff"}`},
		{http.MethodPatch, "/v1/admin/users/1/active", `{"active":false}`},
		{http.MethodPost, "/v1/admin/users/1/reset", ""},
		// The company writes (issue #432): the whole-record PUT and the three
		// narrow per-tab routes. A route missing from this list is a route whose
		// admin gate nothing checks, which is how the units route stayed
		// ungated until these landed.
		{http.MethodPut, "/v1/admin/company", `{"legalName":"PT Hijack"}`},
		{http.MethodPut, "/v1/admin/company/general", `{"legalName":"PT Hijack"}`},
		{http.MethodPut, "/v1/admin/company/theme/quotation", `{"deep":"#000000"}`},
		{http.MethodPut, "/v1/admin/company/theme/portal", `{"deep":"#000000"}`},
		{http.MethodPut, "/v1/admin/company/units", `{"units":[{"value":"Lot","label":"Lot"}]}`},
	}
	for _, tc := range cases {
		if code, _ := doAdminRequest(t, server, staffToken, tc.method, tc.path, tc.body); code != http.StatusForbidden {
			t.Errorf("%s %s as staff = %d; want 403", tc.method, tc.path, code)
		}
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodGet, "/v1/admin/users", ""); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/admin/users without token = %d; want 401", code)
	}
}

func TestAdminUserLifecycle(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_admin_lifecycle")

	code, created := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/users",
		`{"email":"staff@example.com","displayName":"Staff","role":"staff"}`)
	if code != http.StatusCreated {
		t.Fatalf("create user = %d (%v); want 201", code, created)
	}
	if created["mustChangePassword"] != true {
		t.Error("new user must be flagged to change password on first use")
	}
	userID := int64(created["id"].(float64))
	userPath := "/v1/admin/users/" + strconv.FormatInt(userID, 10)

	code, listed := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/users", "")
	if code != http.StatusOK {
		t.Fatalf("list users = %d; want 200", code)
	}
	users, _ := listed["users"].([]any)
	if len(users) != 2 {
		t.Fatalf("users = %d; want bootstrap admin + new staff", len(users))
	}

	// Redeem the invite, log in as staff, then deactivate → session dies.
	_, reset := doAdminRequest(t, server, token, http.MethodPost, userPath+"/reset", "")
	invite, _ := reset["token"].(string)
	if invite == "" {
		t.Fatal("no invite token issued")
	}
	code, _ = doAdminRequest(t, server, "", http.MethodPost, "/v1/auth/redeem",
		`{"token":"`+invite+`","newPassword":"staff-password"}`)
	if code != http.StatusOK {
		t.Fatalf("redeem invite = %d; want 200", code)
	}
	staffToken := doLogin(t, server, "staff@example.com", "staff-password")

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch,
		userPath+"/active", `{"active":false}`); code != http.StatusOK {
		t.Fatalf("deactivate = %d; want 200", code)
	}
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/auth/me", ""); code != http.StatusUnauthorized {
		t.Fatalf("deactivated session = %d; want 401", code)
	}
}

func TestCannotDemoteLastActiveAdmin(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_admin_last")

	_, listed := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/users", "")
	users, _ := listed["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("users = %d; want exactly the bootstrap admin", len(users))
	}
	adminID := int64(users[0].(map[string]any)["id"].(float64))
	adminPath := "/v1/admin/users/" + strconv.FormatInt(adminID, 10)

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch,
		adminPath+"/role", `{"role":"staff"}`); code != http.StatusConflict {
		t.Fatalf("demote last admin = %d; want 409", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch,
		adminPath+"/active", `{"active":false}`); code != http.StatusConflict {
		t.Fatalf("deactivate last admin = %d; want 409", code)
	}
}

func TestInviteTokenSingleUse(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_admin_token")

	_, created := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/users",
		`{"email":"invite@example.com","displayName":"Invite","role":"staff"}`)
	userID := int64(created["id"].(float64))
	resetPath := "/v1/admin/users/" + strconv.FormatInt(userID, 10) + "/reset"

	_, first := doAdminRequest(t, server, token, http.MethodPost, resetPath, "")
	firstToken, _ := first["token"].(string)
	if firstToken == "" {
		t.Fatal("no invite token issued")
	}

	// Redeeming consumes the token.
	code, redeemed := doAdminRequest(t, server, "", http.MethodPost, "/v1/auth/redeem",
		`{"token":"`+firstToken+`","newPassword":"invite-password"}`)
	if code != http.StatusOK {
		t.Fatalf("first redeem = %d; want 200", code)
	}
	if email, _ := redeemed["email"].(string); email != "invite@example.com" {
		t.Errorf("redeem email = %q; want invite@example.com", email)
	}
	// Reuse is rejected.
	if code, _ := doAdminRequest(t, server, "", http.MethodPost, "/v1/auth/redeem",
		`{"token":"`+firstToken+`","newPassword":"other-password"}`); code != http.StatusUnauthorized {
		t.Fatalf("second redeem = %d; want 401", code)
	}
	// A fresh issue invalidates nothing outstanding except by replacement:
	// the newest token works.
	_, second := doAdminRequest(t, server, token, http.MethodPost, resetPath, "")
	secondToken, _ := second["token"].(string)
	if secondToken == "" || secondToken == firstToken {
		t.Fatal("second reset must issue a fresh, distinct token")
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodPost, "/v1/auth/redeem",
		`{"token":"`+secondToken+`","newPassword":"invite-password-2"}`); code != http.StatusOK {
		t.Fatalf("redeem of fresh token = %d; want 200", code)
	}
}

func TestInviteTokenExpiry(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_admin_expiry")

	_, created := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/users",
		`{"email":"expiry@example.com","displayName":"Expiry","role":"staff"}`)
	userID := int64(created["id"].(float64))

	expired, err := database.IssueInviteToken(context.Background(), userID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("issue expired token: %v", err)
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodPost, "/v1/auth/redeem",
		`{"token":"`+expired+`","newPassword":"long-enough-password"}`); code != http.StatusUnauthorized {
		t.Fatalf("redeem of expired token = %d; want 401", code)
	}
}
