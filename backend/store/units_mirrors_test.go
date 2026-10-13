package store

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// unitColumnList is the single declaration of the company_uom columns the
// store reads and writes (backend/store/units.go). A column list written
// twice is a column list that drifts, and a CompanyUnit field with no column
// anywhere is silently never persisted: loss, not a wipe, and nobody notices
// until someone reads the field back and finds it empty. This guard pins the
// const to the struct by reflection — one direction ("every field has a
// column") catches a field added without its column; the other ("every column
// has a field") catches a renamed field leaving an orphan column behind.

// unitFieldColumn is the mechanical snake_case a Go field name maps to: the
// column names in the migration are lower_snake_case of the struct fields
// (Value -> value, Label -> label, SortOrder -> sort_order), declared once in
// 026_company_uom.sql and written down here so the guard can derive without
// guessing.
func unitFieldColumn(field string) string {
	var b strings.Builder
	for i, r := range field {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestUnitColumnListCoversEveryField(t *testing.T) {
	declared := map[string]bool{}
	for _, column := range strings.Split(unitColumnList, ",") {
		declared[strings.TrimSpace(column)] = true
	}

	var missing []string
	typ := reflect.TypeOf(CompanyUnit{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		column := unitFieldColumn(field.Name)
		if !declared[column] {
			missing = append(missing, fmt.Sprintf("CompanyUnit.%s (column %q)", field.Name, column))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("unitColumnList is missing [%s].\n\n"+
			"A struct field with no column in the list is never persisted — loss, "+
			"not a wipe, and nobody notices until the field reads back empty. Add "+
			"the column to the const (and to migration SQL if the table needs it).",
			strings.Join(missing, ", "))
	}

	var extra []string
	for column := range declared {
		found := false
		for i := 0; i < typ.NumField(); i++ {
			if unitFieldColumn(typ.Field(i).Name) == column {
				found = true
				break
			}
		}
		if !found {
			extra = append(extra, column)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("unitColumnList declares [%s], which no CompanyUnit field maps to.\n\n"+
			"A column that names no field persists data nobody reads — check for a "+
			"renamed field whose column the const still keeps.", strings.Join(extra, ", "))
	}
}
