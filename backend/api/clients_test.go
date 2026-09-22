package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/store"
)

func clientRowByCode(t *testing.T, rows []any, code float64) map[string]any {
	t.Helper()
	for _, item := range rows {
		row, _ := item.(map[string]any)
		if row["code"] == code {
			return row
		}
	}
	t.Fatalf("no client with code %v in %v", code, rows)
	return nil
}

func TestClientsListServesAllRoles(t *testing.T) {
	server, _, _ := adminTestSetup(t, "vera_api_clients_list")
	staffToken := staffLogin(t, server, "vera_api_clients_list")

	code, body := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/clients", "")
	if code != http.StatusOK {
		t.Fatalf("staff list = %d; want 200", code)
	}
	if _, ok := body["clients"]; !ok {
		t.Fatalf("list has no clients envelope: %v", body)
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodGet, "/v1/clients", ""); code != http.StatusUnauthorized {
		t.Errorf("anon list = %d; want 401", code)
	}
}

func TestClientsAdminCRUD(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_clients_crud")
	staffToken := staffLogin(t, server, "vera_api_clients_crud")

	// Staff mutations are forbidden.
	if code, _ := doAdminRequest(t, server, staffToken, http.MethodPost, "/v1/admin/clients",
		`{"code":80,"name":"Staff Client","address":"Addr"}`); code != http.StatusForbidden {
		t.Fatalf("staff create = %d; want 403", code)
	}

	code, created := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/clients",
		`{"code":80,"name":"API Client A","address":"Street 1"}`)
	if code != http.StatusCreated {
		t.Fatalf("create = %d; want 201 (%v)", code, created)
	}
	if created["code"] != 80.0 || created["name"] != "API Client A" {
		t.Errorf("created = %v; want code 80", created)
	}
	id := int64(created["id"].(float64))
	path := "/v1/admin/clients/" + strconv.FormatInt(id, 10)

	// Auto-code allocates max+1.
	code, auto := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/clients",
		`{"code":0,"name":"API Client B","address":"Street 2"}`)
	if code != http.StatusCreated {
		t.Fatalf("auto create = %d; want 201 (%v)", code, auto)
	}
	if auto["code"] != 81.0 {
		t.Errorf("auto code = %v; want 81", auto["code"])
	}
	autoID := int64(auto["id"].(float64))

	// Duplicates and validation.
	if code, _ := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/clients",
		`{"code":80,"name":"Clash","address":"Addr"}`); code != http.StatusConflict {
		t.Errorf("duplicate code = %d; want 409", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/clients",
		`{"code":82,"name":"api client a","address":"Addr"}`); code != http.StatusConflict {
		t.Errorf("duplicate name = %d; want 409", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPost, "/v1/admin/clients",
		`{"code":82,"name":"","address":"Addr"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("blank name = %d; want 422", code)
	}

	// Update renames; code change is refused.
	code, updated := doAdminRequest(t, server, token, http.MethodPatch, path,
		`{"name":"API Client Renamed","address":"Street 9"}`)
	if code != http.StatusOK {
		t.Fatalf("update = %d; want 200 (%v)", code, updated)
	}
	if updated["name"] != "API Client Renamed" {
		t.Errorf("updated = %v; want rename", updated)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, path,
		`{"code":99,"name":"API Client Renamed","address":"Street 9"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("code change = %d; want 422", code)
	}
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/admin/clients/999999",
		`{"name":"Ghost","address":"Addr"}`); code != http.StatusNotFound {
		t.Errorf("unknown update = %d; want 404", code)
	}

	// Unreferenced delete succeeds.
	if code, deleted := doAdminRequest(t, server, token, http.MethodDelete,
		"/v1/admin/clients/"+strconv.FormatInt(autoID, 10), ""); code != http.StatusOK {
		t.Fatalf("delete = %d; want 200 (%v)", code, deleted)
	} else if deleted["name"] != "API Client B" {
		t.Errorf("deleted = %v; want client B", deleted)
	}

	// Reference the remaining client, then delete: 409 naming the count.
	customerID := int64(clientRowByCode(t, listClientsHelper(t, server, token), 80.0)["id"].(float64))
	pool, err := pgxpool.New(context.Background(), database.DSN())
	if err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO vera_api_clients_crud.quotations (quote_number, customer_id) VALUES ('1580260001', $1)`,
		customerID); err != nil {
		t.Fatalf("seed quotation: %v", err)
	}
	if code, body := doAdminRequest(t, server, token, http.MethodDelete, path, ""); code != http.StatusConflict {
		t.Fatalf("referenced delete = %d; want 409 (%v)", code, body)
	} else if message, _ := body["error"].(string); !strings.Contains(message, "1 quotations reference it") {
		t.Errorf("error = %q; want count-naming refusal", message)
	}

	if count, err := database.AuditCount(context.Background(), "client.create"); err != nil || count != 2 {
		t.Errorf("audit client.create = %d, %v; want 2, nil", count, err)
	}
}

func listClientsHelper(t *testing.T, server *Server, token string) []any {
	t.Helper()
	code, body := doAdminRequest(t, server, token, http.MethodGet, "/v1/clients", "")
	if code != http.StatusOK {
		t.Fatalf("list = %d; want 200", code)
	}
	rows, _ := body["clients"].([]any)
	return rows
}

// seedQuotationClient creates a client for quotation tests, tolerating
// reruns on a warm database where the row already exists.
func seedQuotationClient(t *testing.T, database *store.Store, code int, name string) {
	t.Helper()
	if _, err := database.CreateClient(context.Background(), code, name, "Test Address"); err != nil {
		if !strings.Contains(err.Error(), "already in use") {
			t.Fatalf("seed client %d: %v", code, err)
		}
	}
}
