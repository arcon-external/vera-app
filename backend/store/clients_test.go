package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// clientTestCleanup removes test-scoped clients so reruns on a warm
// database behave like a fresh one.
func clientTestCleanup(t *testing.T, database *Store, codes ...int16) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	for _, code := range codes {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(
			`DELETE FROM %s.customers WHERE customer_code = $1`, database.configSchema()), code); err != nil {
			t.Fatalf("clean client %d: %v", code, err)
		}
	}
}

func TestClientCRUD(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_clients"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	clientTestCleanup(t, database, 90, 91)

	created, err := database.CreateClient(ctx, 90, "Test Client A", "Test Street 1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Code != 90 || created.Name != "Test Client A" || created.PrimaryAddress != "Test Street 1" {
		t.Errorf("created = %+v; want code 90 with fields", created)
	}

	auto, err := database.CreateClient(ctx, 0, "Test Client B", "Test Street 2")
	if err != nil {
		t.Fatalf("auto-code create: %v", err)
	}
	if auto.Code != 91 {
		t.Errorf("auto code = %d; want max+1 = 91", auto.Code)
	}

	if _, err := database.CreateClient(ctx, 90, "Duplicate Code", "Addr"); err == nil {
		t.Error("duplicate code accepted; want already-in-use")
	} else if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("duplicate code error = %v; want already-in-use", err)
	}
	if _, err := database.CreateClient(ctx, 92, "test client a", "Addr"); err == nil {
		t.Error("duplicate name (any case) accepted; want already-in-use")
	}
	if _, err := database.CreateClient(ctx, 100, "Bad Code", "Addr"); err == nil {
		t.Error("code 100 accepted; want range error")
	}
	if _, err := database.CreateClient(ctx, 92, "  ", "Addr"); err == nil {
		t.Error("blank name accepted; want required error")
	}

	ref, err := database.GetCustomerByCode(ctx, 90)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ref.Name != "Test Client A" || ref.Address != "Test Street 1" {
		t.Errorf("ref = %+v; want stored fields", ref)
	}
	if _, err := database.GetCustomerByCode(ctx, 99); err == nil {
		t.Error("unknown code resolved; want not-found")
	}

	updated, err := database.UpdateClient(ctx, created.ID, "Test Client Renamed", "New Street 9")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Test Client Renamed" || updated.PrimaryAddress != "New Street 9" || updated.Code != 90 {
		t.Errorf("updated = %+v; want renamed with same code", updated)
	}
	if _, err := database.UpdateClient(ctx, created.ID, "Test Client B", "Addr"); err == nil {
		t.Error("rename onto taken name accepted; want already-in-use")
	}
	if _, err := database.UpdateClient(ctx, 999999, "Ghost", "Addr"); err == nil {
		t.Error("unknown id updated; want not-found")
	}

	listed, err := database.ListClients(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, row := range listed {
		if row.Code == 90 && row.QuotationCount != 0 {
			t.Errorf("row = %+v; want zero quotations", row)
		}
		if row.Code == 90 {
			found = true
		}
	}
	if !found {
		t.Error("created client missing from list")
	}

	name, err := database.DeleteClient(ctx, created.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if name != "Test Client Renamed" {
		t.Errorf("deleted name = %q; want renamed value", name)
	}
	if _, err := database.DeleteClient(ctx, created.ID); err == nil {
		t.Error("double delete accepted; want not-found")
	}
	clientTestCleanup(t, database, 91)
}

func TestClientDeleteBlockedByQuotations(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_client_guard"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	clientTestCleanup(t, database, 92)

	client, err := database.CreateClient(ctx, 92, "Guarded Client", "Guard Street")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s.quotations (quote_number, customer_id) VALUES ('1592260001', $1)`,
		database.configSchema()), client.ID); err != nil {
		t.Fatalf("seed quotation: %v", err)
	}

	if _, err := database.DeleteClient(ctx, client.ID); err == nil {
		t.Fatal("referenced delete accepted; want refusal")
	} else if !strings.Contains(err.Error(), "quotations reference it") {
		t.Fatalf("refusal = %v; want quotations-reference message", err)
	}
	rows, err := database.ListClients(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, row := range rows {
		if row.Code == 92 && row.QuotationCount != 1 {
			t.Errorf("count = %d; want 1 referenced quotation", row.QuotationCount)
		}
	}
}

func TestLegacyClientSeedPopulatesDirectory(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database, err := Open(ctx, Config{DSN: dsn, Schema: "vera_test_client_seed"})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	rows, err := database.ListClients(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byCode := make(map[int16]ManagedClient)
	for _, row := range rows {
		byCode[row.Code] = row
	}
	for code, name := range map[int16]string{
		2: "PT. Infineon Technologies Batam",
		3: "PT. H-Tech Oilfield Equipment, NOV XL Systems",
		4: "PT. Datalec Global Solution",
		5: "PT. Panasonic Industrial Devices Batam",
		6: "PT. Surya Teknologi Batam",
	} {
		row, ok := byCode[code]
		if !ok {
			t.Errorf("seeded code %02d missing from directory", code)
			continue
		}
		if row.Name != name {
			t.Errorf("code %02d name = %q; want %q", code, row.Name, name)
		}
		if row.PrimaryAddress == "" {
			t.Errorf("code %02d has no primary address", code)
		}
	}

	// Existing rows are never overwritten: rename one, re-run every
	// migration, and confirm the rename survives.
	seeded, err := database.GetClientByID(ctx, byCode[2].ID)
	if err != nil {
		t.Fatalf("fetch seeded client: %v", err)
	}
	if _, err := database.UpdateClient(ctx, seeded.ID, "Custom Name", seeded.PrimaryAddress); err != nil {
		t.Fatalf("rename seeded client: %v", err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	kept, err := database.GetClientByID(ctx, seeded.ID)
	if err != nil {
		t.Fatalf("fetch renamed client: %v", err)
	}
	if kept.Name != "Custom Name" {
		t.Errorf("name after re-migrate = %q; want rename preserved", kept.Name)
	}
}
