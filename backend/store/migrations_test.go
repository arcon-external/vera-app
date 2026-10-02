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
	if len(migrations) == 0 {
		t.Fatal("no migrations loaded")
	}
	// Contiguity rather than a hand-written list of every version. The list was
	// 1..14 spelled out across two statements, so every migration added a line to
	// this test and a missing edit failed here rather than in the database.
	for i, migration := range migrations {
		if want := i + 1; migration.version != want {
			t.Fatalf("migration at position %d is version %d; want %d (versions must be contiguous from 1)", i, migration.version, want)
		}
	}
	if migrations[0].version != 1 || migrations[1].version != 2 || migrations[2].version != 3 || migrations[3].version != 4 || migrations[4].version != 5 || migrations[5].version != 6 || migrations[6].version != 7 || migrations[7].version != 8 || migrations[8].version != 9 || migrations[9].version != 10 || migrations[10].version != 11 || migrations[11].version != 12 || migrations[12].version != 13 || migrations[13].version != 14 {
		t.Errorf("migration versions = %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d; want 1..14",
			migrations[0].version, migrations[1].version, migrations[2].version, migrations[3].version, migrations[4].version, migrations[5].version, migrations[6].version, migrations[7].version, migrations[8].version, migrations[9].version, migrations[10].version, migrations[11].version, migrations[12].version, migrations[13].version)
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
