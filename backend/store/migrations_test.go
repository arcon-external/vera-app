package store

import (
	"strings"
	"testing"
)

func TestLoadMigrationsReplacesSchemaPlaceholder(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}
	if len(migrations) != 6 {
		t.Fatalf("migration count = %d; want 6", len(migrations))
	}
	if migrations[0].version != 1 || migrations[1].version != 2 || migrations[2].version != 3 || migrations[3].version != 4 || migrations[4].version != 5 || migrations[5].version != 6 {
		t.Errorf("migration versions = %d, %d, %d, %d, %d, %d; want 1, 2, 3, 4, 5, 6",
			migrations[0].version, migrations[1].version, migrations[2].version, migrations[3].version, migrations[4].version, migrations[5].version)
	}
	for _, migration := range migrations {
		if strings.Contains(migration.sql, "{{schema}}") {
			t.Errorf("migration %d still contains an unresolved schema placeholder", migration.version)
		}
	}
	if !strings.Contains(migrations[0].sql, `"vera_test".quotation_revisions`) {
		t.Error("migration does not qualify quotation revisions with the configured schema")
	}
	if !strings.Contains(migrations[1].sql, `"vera_test".users`) {
		t.Error("auth migration does not qualify users with the configured schema")
	}
}

func TestRemoveGoogleMigrationRepairsLegacySchema(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}
	sql := migrations[2].sql
	for _, expected := range []string{
		"DROP COLUMN IF EXISTS google_sub",
		"password_hash SET NOT NULL",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("repair migration missing %q", expected)
		}
	}
	if strings.Contains(sql, "{{schema}}") {
		t.Error("repair migration still contains an unresolved schema placeholder")
	}
}

func TestInitialMigrationContainsRevisionIntegrityConstraints(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}
	sql := migrations[0].sql
	for _, expected := range []string{
		"UNIQUE (quotation_id, revision_number)",
		"UNIQUE (revision_id, item_no)",
		"marginalized_grand_after_tax BIGINT",
		"quotation_documents",
		"sequence_counters",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("migration missing %q", expected)
		}
	}
}
