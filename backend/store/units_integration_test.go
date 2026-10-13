package store

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The company's unit list and the canonical unit it feeds are stored state,
// so their contracts live here with a database behind them. Each test opens
// its own throwaway schema (openTestSchema drops it first), so a rerun on a
// warm database behaves exactly like the first run.

// companyUnitsSeeds is today's frontend catalog verbatim, as migration 026
// seeds it: value = label, in display order. The test pins the exact order,
// because the array order IS the order contract the wire and the dropdown
// both lean on.
var companyUnitsSeeds = []string{
	"Lot", "Pc", "EA", "LS", "set", "meter", "m2", "m3",
	"Hour", "Day", "Week", "Month", "Year", "Quarter",
}

func TestCompanyUnitsSeededInOrder(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_units_seeds")

	units, err := database.GetCompanyUnits(ctx)
	if err != nil {
		t.Fatalf("get company units: %v", err)
	}
	if len(units) != len(companyUnitsSeeds) {
		t.Fatalf("got %d seeded units; want %d: %v", len(units), len(companyUnitsSeeds), units)
	}
	for i, want := range companyUnitsSeeds {
		if units[i].Value != want {
			t.Errorf("unit %d value = %q; want %q", i+1, units[i].Value, want)
		}
		if units[i].Label != want {
			t.Errorf("unit %d label = %q; want the seed value %q (value = label at seed time)",
				i+1, units[i].Label, want)
		}
		if units[i].SortOrder != i+1 {
			t.Errorf("unit %d sort_order = %d; want %d", i+1, units[i].SortOrder, i+1)
		}
	}

	profile, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get company profile: %v", err)
	}
	if profile.CanonicalUoM != "Lot" {
		t.Errorf("fresh profile canonical unit = %q; want the migration default \"Lot\"",
			profile.CanonicalUoM)
	}
}

func TestCompanyUnitsRoundTripPreservesOrder(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_units_roundtrip")

	// A label that differs from its value is the whole point of the field:
	// "m2" stores and signs, "m²" prints. SortOrder is the store's assignment
	// from the array position; the deep comparison below expects it filled.
	want := []CompanyUnit{
		{Value: "EA", Label: "each"},
		{Value: "Pc", Label: "piece"},
		{Value: "m2", Label: "m²"},
		{Value: "Lot", Label: "Lot"},
	}
	for i := range want {
		want[i].SortOrder = i + 1
	}
	updated, err := database.UpdateCompanyUnits(ctx, want)
	if err != nil {
		t.Fatalf("update company units: %v", err)
	}
	if !reflect.DeepEqual(updated, want) {
		t.Errorf("update returned %+v; want %+v", updated, want)
	}

	got, err := database.GetCompanyUnits(ctx)
	if err != nil {
		t.Fatalf("get company units: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read-back == %+v; want %+v (order and labels must survive)", got, want)
	}
	for i, unit := range got {
		if unit.SortOrder != i+1 {
			t.Errorf("unit %d sort_order = %d; want array position %d", i+1, unit.SortOrder, i+1)
		}
	}
}

func TestCompanyUnitsRejectsInvalidInput(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_units_rejects")

	cases := []struct {
		name  string
		units []CompanyUnit
		want  string // a fragment of the error, so the message names the problem
	}{
		{"empty list", nil, "at least one unit"},
		{"empty value", []CompanyUnit{{Value: "", Label: "Lot"}}, `unit 1: value is required`},
		{"value too long", []CompanyUnit{{Value: "verylongunitname", Label: "x"}}, `unit 1: value "verylongunitname" is too long`},
		{"unshaped value", []CompanyUnit{{Value: "m²", Label: "m²"}}, `unit 1: value "m²" is not shaped like a unit label`},
		{"value with a space", []CompanyUnit{{Value: "two words", Label: "two words"}}, `unit 1: value "two words" is not shaped like a unit label`},
		{"empty label", []CompanyUnit{{Value: "Lot", Label: " "}}, `unit 1: label is required`},
		{"label too long", []CompanyUnit{{Value: "Lot", Label: strings.Repeat("x", 21)}}, `unit 1: label "xxxxxxxxxxxxxxxxxxxxx" is too long`},
		{"case-insensitive duplicate", []CompanyUnit{{Value: "Pc", Label: "Pc"}, {Value: "PC", Label: "PC"}}, `unit 2: value "PC" duplicates an earlier unit`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := database.UpdateCompanyUnits(ctx, tc.units)
			if err == nil {
				t.Fatal("update company units succeeded; want a validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q; want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestCanonicalUnitMustSurviveListReplacement(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_units_canonical_survives")

	if _, err := database.UpdateCompanyProfile(ctx, CompanyProfile{
		LegalName:    "PT Arcon Penta Persada",
		CanonicalUoM: "Pc",
	}); err != nil {
		t.Fatalf("set canonical unit to Pc: %v", err)
	}

	_, err := database.UpdateCompanyUnits(ctx, []CompanyUnit{
		{Value: "Lot", Label: "Lot"},
		{Value: "EA", Label: "EA"},
	})
	if err == nil {
		t.Fatal("replacing the list without the canonical unit succeeded; want a refusal")
	}
	if !strings.Contains(err.Error(), `the canonical unit "Pc" must stay on the list`) {
		t.Errorf("error = %q; want it to name the canonical unit", err.Error())
	}
}

// TestCanonicalUnitUpdateAndEmptyKeepsStored covers UpdateCompanyProfile's
// canonical contract: a non-empty value must name a live list member exactly
// (case-sensitive, like the FK compares), and an empty value keeps whatever is
// stored so an old desktop client that does not know the field cannot move it.
func TestCanonicalUnitUpdateAndEmptyKeepsStored(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_units_canonical_update")

	// A canonical must name the list exactly: "lot" matches only
	// case-insensitively, and the FK compares byte for byte.
	if _, err := database.UpdateCompanyProfile(ctx, CompanyProfile{
		LegalName:    "PT Arcon Penta Persada",
		CanonicalUoM: "lot",
	}); err == nil || !strings.Contains(err.Error(),
		`company canonical unit "lot" is not on the company's unit list`) {
		t.Fatalf("lowercase canonical: err = %v; want a refusal naming the unit", err)
	}

	if _, err := database.UpdateCompanyProfile(ctx, CompanyProfile{
		LegalName:    "PT Arcon Penta Persada",
		CanonicalUoM: "m2",
	}); err != nil {
		t.Fatalf("set canonical unit to m2: %v", err)
	}

	// Empty canonical keeps the stored value, even though the profile save
	// otherwise goes through.
	updated, err := database.UpdateCompanyProfile(ctx, CompanyProfile{
		LegalName: "PT Arcon Penta Persada (renamed)",
	})
	if err != nil {
		t.Fatalf("update profile with empty canonical: %v", err)
	}
	if updated.CanonicalUoM != "m2" {
		t.Errorf("empty canonical moved the stored value to %q; want it kept \"m2\"",
			updated.CanonicalUoM)
	}
}

// TestCompanyUnitsForeignKeyRefusesCanonicalRemoval proves the DEFERRABLE FK
// from migration 026 actually fires: bypassing the store check and deleting
// the canonical row directly is refused at commit, so the DB-level existence
// guarantee is real and not just a comment.
func TestCompanyUnitsForeignKeyRefusesCanonicalRemoval(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_units_fk")

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	// The seeded canonical is the migration default "Lot" — guaranteed
	// present because 026_company_uom.sql seeds the list before it adds the
	// canonical_uom column and the FK.
	profile, err := database.GetCompanyProfile(ctx)
	if err != nil {
		t.Fatalf("get company profile: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.company_uom WHERE value = $1`, database.configSchema()), profile.CanonicalUoM); err == nil {
		t.Fatalf("deleting the canonical unit %q directly succeeded; the DEFERRABLE "+
			"FK did not fire at commit", profile.CanonicalUoM)
	}

	// Neither side of the constraint may be left missing: the row is still
	// there, and the profile still names it.
	units, err := database.GetCompanyUnits(ctx)
	if err != nil {
		t.Fatalf("get company units: %v", err)
	}
	found := false
	for _, unit := range units {
		if unit.Value == profile.CanonicalUoM {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("canonical unit %q vanished from the list despite the refused delete",
			profile.CanonicalUoM)
	}
}

// TestRemovedUnitRevisionStillReadsBack is the read-compatibility promise:
// revisions self-snapshot their row's UoM (migration 016/017), so removing a
// unit from the company list — which blocks nothing new being written — never
// breaks reading a quotation that carries it.
func TestRemovedUnitRevisionStillReadsBack(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	ctx := context.Background()
	database := openTestSchema(t, "vera_test_units_removed_revision")
	resetTestSequences(t, database)

	draft := leadTimeDraft("", 0, 0, 0, 0)
	draft.Items[0].UoM = "Pc"
	saved, err := database.SaveRevision(ctx, draft)
	if err != nil {
		t.Fatalf("save revision with UoM Pc: %v", err)
	}

	// Drop Pc from the list, keeping the canonical default.
	if _, err := database.UpdateCompanyUnits(ctx, []CompanyUnit{
		{Value: "Lot", Label: "Lot"},
		{Value: "EA", Label: "EA"},
		{Value: "m2", Label: "m²"},
	}); err != nil {
		t.Fatalf("replace list without Pc: %v", err)
	}

	record, err := database.GetQuotation(ctx, saved.ID, nil)
	if err != nil {
		t.Fatalf("get quotation after list shrink: %v", err)
	}
	if len(record.Items) == 0 {
		t.Fatal("quotation read back with no items")
	}
	if got := record.Items[0].UoM; got != "Pc" {
		t.Errorf("stored item UoM read back as %q; want the snapshot value \"Pc\" "+
			"even though Pc left the company list", got)
	}
}
