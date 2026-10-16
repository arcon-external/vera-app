package api

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/store"
)

func TestEmailChangeRequestRejectsInvalidEmailAndNormalizes(t *testing.T) {
	server, database, adminToken := adminTestSetup(t, "vera_api_email_validate")
	staffID, staffToken := emailStaff(t, server, database, "email-validate@example.com")
	for _, email := range []string{"x", "foo@bar", "a b@c"} {
		for _, tc := range []struct{ token, method, path, key string }{
			{staffToken, http.MethodPost, "/v1/auth/email-change-request", "proposedEmail"},
			{adminToken, http.MethodPatch, "/v1/admin/users/" + strconv.FormatInt(staffID, 10) + "/email", "email"},
		} {
			code, _ := doAdminRequest(t, server, tc.token, tc.method, tc.path,
				`{"`+tc.key+`":`+strconv.Quote(email)+`}`)
			if code != http.StatusUnprocessableEntity {
				t.Errorf("%s %q = %d; want 422", tc.path, email, code)
			}
		}
	}
	code, _ := doAdminRequest(t, server, staffToken, http.MethodPost, "/v1/auth/email-change-request",
		`{"proposedEmail":"  MiXeD@Example.COM  "}`)
	if code != http.StatusAccepted {
		t.Fatalf("mixed-case request = %d; want 202", code)
	}
	code, queue := doAdminRequest(t, server, adminToken, http.MethodGet, "/v1/admin/email-change-requests", "")
	if code != http.StatusOK {
		t.Fatalf("queue = %d; want 200", code)
	}
	row := queue["requests"].([]any)[0].(map[string]any)
	if row["proposedEmail"] != "mixed@example.com" {
		t.Errorf("pending email = %v; want normalized", row["proposedEmail"])
	}
	id := int64(row["id"].(float64))
	if code, _ := doAdminRequest(t, server, adminToken, http.MethodPost,
		"/v1/admin/email-change-requests/"+strconv.FormatInt(id, 10)+"/approve", ""); code != http.StatusOK {
		t.Fatalf("approve = %d; want 200", code)
	}
	user, err := database.GetUserByID(context.Background(), staffID)
	if err != nil || user.Email != "mixed@example.com" {
		t.Fatalf("approved email = %q, %v; want normalized", user.Email, err)
	}
}

// emailStaff seeds a staff user and returns its ID plus bearer token.
func emailStaff(t *testing.T, server *Server, database *store.Store, email string) (int64, string) {
	t.Helper()
	hash, err := auth.HashPassword("staff-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := database.CreateUser(context.Background(), email, "Email Staff", hash, "staff")
	if err != nil {
		t.Fatalf("seed staff: %v", err)
	}
	service := auth.NewService(database)
	token, _, err := service.Login(context.Background(), email, "staff-password", false)
	if err != nil {
		t.Fatalf("staff login: %v", err)
	}
	return user.ID, token
}

func TestAdminSetsUserEmailDirectly(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_email_admin")
	staffID, _ := emailStaff(t, server, database, "email-target@example.com")

	code, body := doAdminRequest(t, server, token, http.MethodPatch,
		"/v1/admin/users/"+strconv.FormatInt(staffID, 10)+"/email", `{"email":"  email-renamed@example.com "}`)
	if code != http.StatusOK {
		t.Fatalf("set email = %d; want 200 (%v)", code, body)
	}
	if body["email"] != "email-renamed@example.com" {
		t.Errorf("email = %v; want trimmed rename", body["email"])
	}
	// Old address is dead, new address logs in.
	service := auth.NewService(database)
	if _, _, err := service.Login(context.Background(), "email-target@example.com", "staff-password", false); err == nil {
		t.Error("login with old email succeeded; want failure")
	}
	if _, _, err := service.Login(context.Background(), "email-renamed@example.com", "staff-password", false); err != nil {
		t.Errorf("login with new email failed: %v", err)
	}

	// Collision, malformed, and unknown targets are rejected cleanly.
	staffID2, _ := emailStaff(t, server, database, "email-other@example.com")
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch,
		"/v1/admin/users/"+strconv.FormatInt(staffID2, 10)+"/email", `{"email":"EMAIL-RENAMED@example.com"}`); code != http.StatusConflict {
		t.Errorf("collision = %d; want 409", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch,
		"/v1/admin/users/"+strconv.FormatInt(staffID2, 10)+"/email", `{"email":"not-an-email"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("malformed = %d; want 422", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch,
		"/v1/admin/users/999999/email", `{"email":"ghost@example.com"}`); code != http.StatusNotFound {
		t.Errorf("unknown user = %d; want 404", code)
	}
	if count, err := database.AuditCount(context.Background(), "user.email.change"); err != nil || count != 1 {
		t.Errorf("audit user.email.change = %d, %v; want 1, nil", count, err)
	}
}

func TestStaffEmailChangeRequestFlow(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_email_flow")
	staffID, staffToken := emailStaff(t, server, database, "email-staff@example.com")

	// Staff cannot use the admin endpoint.
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPatch,
		"/v1/admin/users/"+strconv.FormatInt(staffID, 10)+"/email", `{"email":"x@example.com"}`); code != http.StatusForbidden {
		t.Fatalf("staff direct set = %d; want 403", code)
	}

	code, body := doAdminRequest(t, server, staffToken, http.MethodPost,
		"/v1/auth/email-change-request", `{"proposedEmail":"email-proposed@example.com"}`)
	if code != http.StatusAccepted {
		t.Fatalf("request = %d; want 202 (%v)", code, body)
	}
	if body["status"] != "pending" {
		t.Errorf("status = %v; want pending", body["status"])
	}
	id := int64(body["id"].(float64))

	// Own-address and taken-address proposals are rejected.
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPost,
		"/v1/auth/email-change-request", `{"proposedEmail":"email-staff@example.com"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("own email = %d; want 422", code)
	}
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPost,
		"/v1/auth/email-change-request", `{"proposedEmail":"api-admin@example.com"}`); code != http.StatusConflict {
		t.Errorf("taken email = %d; want 409", code)
	}

	// Staff sees their pending status; admin sees the queue.
	code, mine := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/auth/email-change-requests/mine", "")
	if code != http.StatusOK {
		t.Fatalf("mine = %d; want 200", code)
	}
	if rows, _ := mine["requests"].([]any); len(rows) != 1 {
		t.Errorf("mine rows = %d; want 1 pending", len(rows))
	}
	code, queue := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/email-change-requests", "")
	if code != http.StatusOK {
		t.Fatalf("queue = %d; want 200", code)
	}
	if rows, _ := queue["requests"].([]any); len(rows) != 1 {
		t.Fatalf("queue rows = %d; want 1", len(rows))
	}

	// Approve flips the email and kills the old login.
	if code, decided := doAdminRequest(t, server, token, http.MethodPost,
		"/v1/admin/email-change-requests/"+strconv.FormatInt(id, 10)+"/approve", ""); code != http.StatusOK {
		t.Fatalf("approve = %d; want 200 (%v)", code, decided)
	} else if decided["status"] != "approved" {
		t.Errorf("status = %v; want approved", decided["status"])
	}
	service := auth.NewService(database)
	if _, _, err := service.Login(context.Background(), "email-staff@example.com", "staff-password", false); err == nil {
		t.Error("login with old email succeeded after approval; want failure")
	}
	if _, _, err := service.Login(context.Background(), "email-proposed@example.com", "staff-password", false); err != nil {
		t.Errorf("login with approved email failed: %v", err)
	}
	// Re-deciding is a 404 (already-decided maps there, like deletions).
	if code, _ := doAdminRequest(t, server, token, http.MethodPost,
		"/v1/admin/email-change-requests/"+strconv.FormatInt(id, 10)+"/reject", ""); code != http.StatusNotFound {
		t.Errorf("re-decide = %d; want 404", code)
	}
}

func TestStaffEmailChangeReject(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_email_reject")
	_, staffToken := emailStaff(t, server, database, "email-reject@example.com")

	code, body := doAdminRequest(t, server, staffToken, http.MethodPost,
		"/v1/auth/email-change-request", `{"proposedEmail":"email-nope@example.com"}`)
	if code != http.StatusAccepted {
		t.Fatalf("request = %d; want 202", code)
	}
	id := int64(body["id"].(float64))
	if code, decided := doAdminRequest(t, server, token, http.MethodPost,
		"/v1/admin/email-change-requests/"+strconv.FormatInt(id, 10)+"/reject", ""); code != http.StatusOK {
		t.Fatalf("reject = %d; want 200", code)
	} else if decided["status"] != "rejected" {
		t.Errorf("status = %v; want rejected", decided["status"])
	}
	service := auth.NewService(database)
	if _, _, err := service.Login(context.Background(), "email-reject@example.com", "staff-password", false); err != nil {
		t.Errorf("original login broken by reject: %v", err)
	}
}
