package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"vera-app/backend/store"
)

// The company's unit list is mirrored by hand at three hops, and unlike the
// profile's four (which company_mirrors_test.go guards), the store is the
// only side named "CompanyUnit" more than once:
//
//  1. store.CompanyUnit (backend/store/units.go) — the struct, the only truth
//     anything else is derived from.
//  2. companyUnitsDTO (company.go:162) — what GET /v1/company/units and
//     PUT /v1/admin/company/units render.
//  3. the store's unitColumnList (backend/store/units.go) — what is actually
//     persisted.
//
// The failure modes mirror the profile's. A key missing from the DTO row is
// omitted from the response, the desktop decodes it as "", the readers pass ""
// on, and the next save writes "" over the column — a wipe by round trip with
// nothing wrong on either side in isolation. A field missing from the column
// list is never persisted: loss, not a wipe, and nobody notices until someone
// reads the field back and finds it empty.
//
// The one deliberate absence is SortOrder. The wire contract is the ordered
// array itself ("units": [{value, label}, ...]) and a redundant number on the
// wire could drift from the array; the API never emits it and the desktop
// CompanyUnit (backend/admin.go) models only Value and Label for that reason.
// The DTO test asserts its absence so the contract stays visible instead of
// implied, and the column test pins the store side by reflection.

// companyUnitWireKey maps a CompanyUnit struct field to the JSON key the
// units wire carries it under. Value and Label travel as "value" and "label"
// (both mechanical lowercasing); SortOrder never travels — the ordered array
// IS the order, so a redundant number on the wire could drift from it.
func companyUnitWireKey(field string) (string, bool) {
	switch field {
	case "SortOrder":
		return "", false
	}
	return strings.ToLower(field), true
}

// companyUnitsDTOIsWired reports whether every CompanyUnit field appears in
// the DTO rows with its sentinel, and no row carries a key outside the wire
// set. It is the same reflection shape as markCompanyProfile: a new field
// lands here automatically because the expected set is derived from the
// struct, never listed by hand.
func TestCompanyUnitsDTOCarriesEveryField(t *testing.T) {
	// Two rows with distinct sentinels per row, so a row that copies its
	// neighbor's value — or a swapped key — fails naming the row and field.
	var units []store.CompanyUnit
	var want []map[string]string // wire -> Go field name, per row
	for i := 0; i < 2; i++ {
		var unit store.CompanyUnit
		unit.SortOrder = i + 1
		rowWant := map[string]string{}
		typ := reflect.TypeOf(store.CompanyUnit{})
		value := reflect.ValueOf(&unit).Elem()
		for f := 0; f < typ.NumField(); f++ {
			field := typ.Field(f)
			wire, onWire := companyUnitWireKey(field.Name)
			if !onWire {
				continue
			}
			// The row sentinel is the row index plus the field name, so the
			// first row's "Lot" cannot be confused with the second's "Lot".
			sentinel := fmt.Sprintf("row%d:%s", i, field.Name)
			switch field.Type.Kind() {
			case reflect.String:
				value.Field(f).SetString(sentinel)
			default:
				t.Fatalf("store.CompanyUnit.%s is a %s; this guard carries string "+
					"sentinels only. Give the field a sentinel before adding it "+
					"to the struct.", field.Name, field.Type)
			}
			rowWant[wire] = field.Name
		}
		units = append(units, unit)
		want = append(want, rowWant)
	}

	dto := companyUnitsDTO(units)
	if len(dto) != 1 {
		t.Errorf("companyUnitsDTO returned %d top-level keys; want exactly 1 (\"units\")", len(dto))
	}
	raw, present := dto["units"]
	if !present {
		t.Fatal(`companyUnitsDTO returned no "units" key; this guard proves nothing`)
	}
	rows, ok := raw.([]map[string]any)
	if !ok {
		t.Fatalf(`dto["units"] is %T; want []map[string]any`, raw)
	}
	if len(rows) != len(units) {
		t.Fatalf("dto carried %d rows; want %d", len(rows), len(units))
	}

	var missing, wrong, extra []string
	for rowIndex, row := range rows {
		for wire, field := range want[rowIndex] {
			sentinel := fmt.Sprintf("row%d:%s", rowIndex, field)
			got, present := row[wire]
			if !present {
				missing = append(missing, fmt.Sprintf("row %d: store.CompanyUnit.%s (wire %q)", rowIndex, field, wire))
				continue
			}
			if !reflect.DeepEqual(got, sentinel) {
				wrong = append(wrong, fmt.Sprintf("row %d: wire %q carried %v; want the "+
					"store.CompanyUnit.%s value %q", rowIndex, wire, got, field, sentinel))
			}
		}
		for wire := range row {
			if _, expected := want[rowIndex][wire]; !expected {
				extra = append(extra, fmt.Sprintf("row %d: %q", rowIndex, wire))
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("companyUnitsDTO rows are missing [%s].\n\n"+
			"The desktop decodes a missing key as the zero value, its readers pass \"\" "+
			"on, and the next save writes \"\" over the column. A key missing here is a "+
			"wipe by round trip, not a formatting bug.", strings.Join(missing, ", "))
	}
	sort.Strings(wrong)
	if len(wrong) > 0 {
		t.Errorf("companyUnitsDTO carried a value under the wrong key:\n  %s\n\n"+
			"The wire keys are derived from the Go field names, so this is a "+
			"copy-paste swap in companyUnitsDTO.", strings.Join(wrong, "\n  "))
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("companyUnitsDTO rows carry [%s], which no CompanyUnit field maps to.\n\n"+
			"SortOrder is the deliberate missing member: the ordered array IS the "+
			"order contract, and a redundant number on the wire could drift from it. "+
			"Any other extra key means the DTO drifted from the struct.",
			strings.Join(extra, ", "))
	}
}

// TestCompanyUnitsDTORoundTripsThroughJSON pins the actual served bytes, not
// just the map: the DTO's rows are []map[string]any, and encoding/json renders
// exactly the keys they hold. This is the shape Wave 2 of #337 consumes, and
// it doubles as the wire documentation (see the report).
func TestCompanyUnitsDTORoundTripsThroughJSON(t *testing.T) {
	units := []store.CompanyUnit{
		{Value: "Lot", Label: "Lot", SortOrder: 1},
		{Value: "m2", Label: "m²", SortOrder: 7},
		{Value: "Quarter", Label: "Quarter", SortOrder: 14},
	}
	body, err := json.Marshal(companyUnitsDTO(units))
	if err != nil {
		t.Fatalf("marshal companyUnitsDTO: %v", err)
	}
	var decoded struct {
		Units []struct {
			Value string `json:"value"`
			Label string `json:"label"`
		} `json:"units"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal companyUnitsDTO output: %v", err)
	}
	if len(decoded.Units) != len(units) {
		t.Fatalf("decoded %d units; want %d", len(decoded.Units), len(units))
	}
	for i, unit := range units {
		if decoded.Units[i].Value != unit.Value || decoded.Units[i].Label != unit.Label {
			t.Errorf("unit %d round-tripped as %+v; want value %q label %q",
				i, decoded.Units[i], unit.Value, unit.Label)
		}
	}
}
