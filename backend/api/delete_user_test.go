package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func deactivateUser(t *testing.T, server *Server, token string, id int64) {
	t.Helper()
	code, _ := doAdminRequest(t, server, token, http.MethodPatch,
		fmt.Sprintf("/v1/admin/users/%d/active", id), `{"active":false}`)
	if code != http.StatusOK {
		t.Fatalf("deactivate user %d = %d; want 200", id, code)
	}
}

func TestDeleteUserRemovesRowAndSessions(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_delete_happy")
	seedStaff(t, database, "gone-staff@example.com", "staff-password")
	staff, err := database.GetUserByEmail(context.Background(), "gone-staff@example.com")
	if err != nil {
		t.Fatalf("find staff: %v", err)
	}
	staffToken := doLogin(t, server, "gone-staff@example.com", "staff-password")
	deactivateUser(t, server, token, staff.ID)

	code, response := doAdminRequest(t, server, token, http.MethodDelete,
		fmt.Sprintf("/v1/admin/users/%d", staff.ID), "")
	if code != http.StatusOK {
		t.Fatalf("delete user = %d; want 200 (%v)", code, response["error"])
	}
	if email, _ := response["email"].(string); email != "gone-staff@example.com" {
		t.Errorf("response email = %q; want the deleted address", email)
	}

	if _, err := database.GetUserByEmail(context.Background(), "gone-staff@example.com"); err == nil {
		t.Error("deleted user still resolvable by email")
	}
	// Sessions died with the row: the old token no longer authenticates.
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/auth/me", ""); code != http.StatusUnauthorized {
		t.Errorf("old session = %d; want 401 after hard delete", code)
	}
	count, err := database.AuditCount(context.Background(), "user.delete")
	if err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if count != 1 {
		t.Errorf("user.delete audit entries = %d; want 1", count)
	}
}

func TestDeleteUserRejectsActiveTarget(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_delete_active")
	seedStaff(t, database, "busy-staff@example.com", "staff-password")
	staff, err := database.GetUserByEmail(context.Background(), "busy-staff@example.com")
	if err != nil {
		t.Fatalf("find staff: %v", err)
	}

	code, _ := doAdminRequest(t, server, token, http.MethodDelete,
		fmt.Sprintf("/v1/admin/users/%d", staff.ID), "")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("delete active user = %d; want 422", code)
	}
	if _, err := database.GetUserByEmail(context.Background(), "busy-staff@example.com"); err != nil {
		t.Error("active user vanished after rejected delete")
	}
}

func TestDeleteUserRejectsSelf(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_delete_self")
	admin, err := database.GetUserByEmail(context.Background(), "api-admin@example.com")
	if err != nil {
		t.Fatalf("find admin: %v", err)
	}

	code, _ := doAdminRequest(t, server, token, http.MethodDelete,
		fmt.Sprintf("/v1/admin/users/%d", admin.ID), "")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("self-delete = %d; want 422", code)
	}
}

func TestDeleteUserRequiresAdmin(t *testing.T) {
	server, database, adminToken := adminTestSetup(t, "vera_api_delete_auth")
	seedStaff(t, database, "doomed-staff@example.com", "staff-password")
	staff, err := database.GetUserByEmail(context.Background(), "doomed-staff@example.com")
	if err != nil {
		t.Fatalf("find staff: %v", err)
	}
	deactivateUser(t, server, adminToken, staff.ID)
	seedStaff(t, database, "other-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "other-staff@example.com", "staff-password")

	path := fmt.Sprintf("/v1/admin/users/%d", staff.ID)
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodDelete, path, ""); code != http.StatusForbidden {
		t.Fatalf("staff delete = %d; want 403", code)
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodDelete, path, ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous delete = %d; want 401", code)
	}
	if _, err := database.GetUserByEmail(context.Background(), "doomed-staff@example.com"); err != nil {
		t.Error("user vanished after rejected deletes")
	}
}
