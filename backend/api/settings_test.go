package api

import (
	"net/http"
	"strconv"
	"testing"
)

func TestChangePasswordRequiresCurrent(t *testing.T) {
	server, _ := testServer(t, "vera_api_settings_password")
	token := doLogin(t, server, "api-admin@example.com", "test-password")

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/password",
		`{"currentPassword":"wrong-password","newPassword":"brand-new-password"}`); code != http.StatusUnauthorized {
		t.Fatalf("change with wrong current = %d; want 401", code)
	}

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/password",
		`{"currentPassword":"test-password","newPassword":"short"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("change to short password = %d; want 422", code)
	}

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/password",
		`{"currentPassword":"test-password","newPassword":"brand-new-password"}`); code != http.StatusOK {
		t.Fatalf("change password = %d; want 200", code)
	}

	// Old password is dead, new one works.
	if code, _ := doAdminRequest(t, server, "", http.MethodPost, "/v1/auth/login",
		`{"email":"api-admin@example.com","password":"test-password"}`); code != http.StatusUnauthorized {
		t.Fatalf("login with old password = %d; want 401", code)
	}
	fresh := doLogin(t, server, "api-admin@example.com", "brand-new-password")
	if code, _ := doAdminRequest(t, server, fresh, http.MethodGet, "/v1/auth/me", ""); code != http.StatusOK {
		t.Fatalf("login with new password then me = %d; want 200", code)
	}
	// Password change revokes other sessions, including the one that made it.
	if code, _ := doAdminRequest(t, server, token, http.MethodGet, "/v1/auth/me", ""); code != http.StatusUnauthorized {
		t.Fatalf("pre-change session = %d; want 401 after password change", code)
	}
}

func TestLogoutEverywhereRevokesAllSessions(t *testing.T) {
	server, _ := testServer(t, "vera_api_settings_logout")
	first := doLogin(t, server, "api-admin@example.com", "test-password")
	second := doLogin(t, server, "api-admin@example.com", "test-password")

	if code, _ := doAdminRequest(t, server, first, http.MethodPost, "/v1/auth/logout-everywhere", ""); code != http.StatusNoContent {
		t.Fatalf("logout everywhere = %d; want 204", code)
	}
	for name, tok := range map[string]string{"first": first, "second": second} {
		if code, _ := doAdminRequest(t, server, tok, http.MethodGet, "/v1/auth/me", ""); code != http.StatusUnauthorized {
			t.Fatalf("%s session = %d; want 401", name, code)
		}
	}
}

func TestDeletionRequestApproveFlow(t *testing.T) {
	server, _ := testServer(t, "vera_api_settings_delete")
	token := doLogin(t, server, "api-admin@example.com", "test-password")

	// Staff account to delete: create + redeem like the admin lifecycle test.
	code, created := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/users",
		`{"email":"leaver@example.com","displayName":"Leaver","role":"staff"}`)
	if code != http.StatusCreated {
		t.Fatalf("create user = %d; want 201", code)
	}
	staffID := int64(created["id"].(float64))
	staffPath := "/v1/admin/users/" + strconv.Itoa(int(staffID))
	_, reset := doAdminRequest(t, server, token, http.MethodPost, staffPath+"/reset", "")
	invite, _ := reset["token"].(string)
	if code, _ := doAdminRequest(t, server, "", http.MethodPost, "/v1/auth/redeem",
		`{"token":"`+invite+`","newPassword":"leaver-password"}`); code != http.StatusOK {
		t.Fatalf("redeem = %d; want 200", code)
	}
	staffToken := doLogin(t, server, "leaver@example.com", "leaver-password")

	// Request requires explicit confirmation flag.
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPost, "/v1/auth/deletion-request",
		`{"confirm":false}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("unconfirmed request = %d; want 422", code)
	}
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPost, "/v1/auth/deletion-request",
		`{"confirm":true}`); code != http.StatusAccepted {
		t.Fatalf("deletion request = %d; want 202", code)
	}

	// Admin sees the pending request.
	code, pending := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/deletion-requests", "")
	if code != http.StatusOK {
		t.Fatalf("list requests = %d; want 200", code)
	}
	requests, _ := pending["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("pending requests = %d; want 1", len(requests))
	}

	// Approve soft-deletes: record retained, inactive, sessions dead.
	reqID := int64(requests[0].(map[string]any)["id"].(float64))
	if code, _ := doAdminRequest(t, server, token, http.MethodPost,
		"/v1/admin/deletion-requests/"+strconv.Itoa(int(reqID))+"/approve", ""); code != http.StatusOK {
		t.Fatalf("approve = %d; want 200", code)
	}
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/auth/me", ""); code != http.StatusUnauthorized {
		t.Fatalf("deleted user session = %d; want 401", code)
	}
	code, listed := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/users", "")
	if code != http.StatusOK {
		t.Fatalf("list users = %d; want 200", code)
	}
	found, active := false, true
	for _, entry := range listed["users"].([]any) {
		row := entry.(map[string]any)
		if int64(row["id"].(float64)) == staffID {
			found, active = true, row["isActive"].(bool)
		}
	}
	if !found {
		t.Fatal("deleted user record missing; soft-delete must retain it")
	}
	if active {
		t.Error("deleted user still active")
	}

	// The last admin cannot delete themselves: request succeeds, approval fails.
	if code, _ := doAdminRequest(t, server, token, http.MethodPost, "/v1/auth/deletion-request",
		`{"confirm":true}`); code != http.StatusAccepted {
		t.Fatalf("admin self-request = %d; want 202", code)
	}
	code, pending = doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/deletion-requests", "")
	if code != http.StatusOK {
		t.Fatalf("list requests = %d; want 200", code)
	}
	selfReqs, _ := pending["requests"].([]any)
	if len(selfReqs) != 1 {
		t.Fatalf("pending requests = %d; want 1", len(selfReqs))
	}
	selfID := int64(selfReqs[0].(map[string]any)["id"].(float64))
	if code, _ := doAdminRequest(t, server, token, http.MethodPost,
		"/v1/admin/deletion-requests/"+strconv.Itoa(int(selfID))+"/approve", ""); code != http.StatusConflict {
		t.Fatalf("approve self-delete of last admin = %d; want 409", code)
	}
	// And the admin can still work afterwards.
	if code, _ := doAdminRequest(t, server, token, http.MethodGet, "/v1/auth/me", ""); code != http.StatusOK {
		t.Fatalf("admin me after refused delete = %d; want 200", code)
	}
}

func TestApproveDeletionSignsOutEverywhere(t *testing.T) {
	server, database, adminToken := adminTestSetup(t, "vera_api_deletion_signout")
	seedStaff(t, database, "leaving-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "leaving-staff@example.com", "staff-password")

	code, requested := doAdminRequest(t, server, staffToken, http.MethodPost, "/v1/auth/deletion-request", `{"confirm":true}`)
	if code != http.StatusAccepted {
		t.Fatalf("request deletion = %d; want 202", code)
	}
	requestID, _ := requested["id"].(float64)

	code, _ = doAdminRequest(t, server, adminToken, http.MethodPost,
		"/v1/admin/deletion-requests/"+strconv.FormatInt(int64(requestID), 10)+"/approve", "")
	if code != http.StatusOK {
		t.Fatalf("approve deletion = %d; want 200", code)
	}

	// The staff session dies with the approval.
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/auth/me", ""); code != http.StatusUnauthorized {
		t.Errorf("session after approval = %d; want 401", code)
	}
}
