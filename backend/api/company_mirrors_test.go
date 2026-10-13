package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"vera-app/backend/store"
)

// The company profile is one DTO mirrored by hand at four hops, and the save
// is a full replacement where an empty string clears a column:
//
//  1. store.CompanyProfile (backend/store/company.go) — the struct, the only
//     truth anything else is derived from.
//  2. companyDTO (company.go:16) — what GET /v1/admin/company and
//     GET /v1/company render.
//  3. the update bind struct (company.go:56) — what PUT /v1/admin/company
//     accepts.
//  4. the store's SQL column lists (backend/store/company.go) — what is
//     actually persisted.
//
// #345 guarded the desktop map, which sits on the far side of this chain.
// These tests pin hops 2 and 3, the two that were left unguarded and the two
// that wipe. Both failures are silent in the way that matters: a key missing
// from companyDTO is omitted from the response, the desktop decodes the field
// as "", its readers pass "" on, and the next save writes "" over the column —
// nothing is wrong on either side in isolation, and no single save looks
// broken. A field missing from the bind struct is never read, the full
// replacement writes the zero value, and the column is cleared the same way.
//
// The case-insensitivity note, because every guard here leans on it.
// store.CompanyProfile carries NO json tags, and neither does
// backend.CompanyProfile (backend/admin.go), which is what the desktop
// decodes into. Desktop↔server matching there works only through
// encoding/json's case-insensitive key match: "legalName" reaches LegalName
// and "taxId" reaches TaxID because both lowercase to the same thing. That is
// also why every expectation below derives its wire key from the Go field name
// instead of listing it — a wire name that is not a case-variant of its Go
// name decodes to zero on Get, the readers pass "" on, and the next save wipes
// the column. A new field added with such a key fails here, naming the field.
//
// Hop 4 is deliberately not guarded; see the note at the end of this file.

// companyProfileWireKey derives the JSON key a CompanyProfile field travels
// under, mirroring #345's treatment for the desktop map: the struct carries
// no json tags, so there are no tags to compare against, and the wire name is
// derived from the Go name with the one irregular pinned here rather than left
// for a reader to trip over.
//
// TaxID is the exception: its wire key is "taxId", not the mechanical
// "taxID". HasLogo travels, but on the read path only.
func companyProfileWireKey(field string) (string, bool) {
	switch field {
	case "LogoBytes", "LogoMime", "LogoPath":
		// Artwork travels the dedicated logo endpoints
		// (/v1/admin/company/logo), never the profile wire. A DTO that
		// started emitting these would put bytes and file paths in every
		// settings response.
		return "", false
	case "HasLogo":
		return "hasLogo", true
	case "TaxID":
		return "taxId", true
	}
	return strings.ToLower(field[:1]) + field[1:], true
}

// companyProfileIsWritable reports whether PUT /v1/admin/company binds a
// field. The artwork fields and HasLogo are read-only on the write path: the
// store recomputes HasLogo from logo_bytes (len(LogoBytes) > 0), and the
// desktop's update body is a map of strings, so a bool can never ride it.
// They are excluded from the bind expectation, and the test asserts their
// absence so the contract stays visible instead of implied.
func companyProfileIsWritable(field string) bool {
	switch field {
	case "HasLogo", "LogoBytes", "LogoMime", "LogoPath":
		return false
	}
	return true
}

// wireExpectation is what one CompanyProfile field should look like on the
// wire: the Go field it came from, so a failure names the field rather than a
// key, and the sentinel value it should carry.
type wireExpectation struct {
	field string
	value any
}

// markCompanyProfile fills target with a distinct sentinel per field and
// returns what the wire should carry for every field that has a wire key.
// Each sentinel is the field's own name, so a value that lands under the wrong
// key — a swapped field or a mis-tagged one — reads as "taxId carried Email"
// instead of a bare mismatch.
func markCompanyProfile(t *testing.T, target *store.CompanyProfile) map[string]wireExpectation {
	t.Helper()
	sentinels := map[string]wireExpectation{}
	typ := reflect.TypeOf(store.CompanyProfile{})
	value := reflect.ValueOf(target).Elem()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		wire, onWire := companyProfileWireKey(field.Name)
		if !onWire {
			continue
		}
		switch field.Type.Kind() {
		case reflect.String:
			value.Field(i).SetString(field.Name)
		case reflect.Bool:
			value.Field(i).SetBool(true)
		default:
			// Fail loudly rather than silently skipping the field: a new
			// non-string, non-bool field needs a sentinel case decided by a
			// person, not defaulted by the guard.
			t.Fatalf("CompanyProfile.%s is a %s, and this guard carries string "+
				"and bool sentinels only. Give the field a sentinel before adding "+
				"it to the struct.", field.Name, field.Type)
		}
		sentinels[wire] = wireExpectation{field: field.Name, value: value.Field(i).Interface()}
	}
	return sentinels
}

// TestCompanyDTOCarriesEveryField pins companyDTO against CompanyProfile by
// reflection, with the expected set derived from the struct so a new field
// lands here automatically.
//
// A key missing from the DTO is omitted from the response, the desktop decodes
// it as "", the readers pass "" on, and the next save writes "" over the
// column: this is the wipe-by-round-trip, and it is invisible on both sides.
func TestCompanyDTOCarriesEveryField(t *testing.T) {
	var profile store.CompanyProfile
	want := markCompanyProfile(t, &profile)

	dto := companyDTO(profile)
	// A DTO that rendered nothing would make every assertion below vacuous.
	if len(dto) == 0 {
		t.Fatal("companyDTO returned no keys; this guard proves nothing")
	}

	var missing, wrong []string
	for wire, expected := range want {
		got, present := dto[wire]
		if !present {
			missing = append(missing, fmt.Sprintf("CompanyProfile.%s (wire %q)", expected.field, wire))
			continue
		}
		// DeepEqual so a future non-comparable value fails the comparison
		// instead of panicking the test.
		if !reflect.DeepEqual(got, expected.value) {
			wrong = append(wrong, valueMismatch(wire, expected.field, got, expected.value))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("companyDTO is missing [%s].\n\n"+
			"The desktop decodes a missing key as the zero value, its readers pass \"\" "+
			"on, and the next save writes \"\" over the column. A key missing here is a "+
			"wipe by round trip, not a formatting bug.",
			strings.Join(missing, ", "))
	}
	sort.Strings(wrong)
	if len(wrong) > 0 {
		t.Errorf("companyDTO carried a field's value under another field's key:\n  %s\n\n"+
			"The wire keys are derived from the Go field names, so this is a copy-paste "+
			"swap in the DTO.", strings.Join(wrong, "\n  "))
	}

	var extra []string
	for wire := range dto {
		if _, expected := want[wire]; !expected {
			extra = append(extra, wire)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("companyDTO carries [%s], which no CompanyProfile field maps to.\n\n"+
			"The DTO drifted from the struct — check for a renamed field.",
			strings.Join(extra, ", "))
	}
}

// TestUpdateCompanyProfileBindsEveryField pins the request struct the update
// handler binds against CompanyProfile by reflection.
//
// The struct is declared inline in updateCompanyProfile, so it has no name to
// reflect on, and a test cannot name it either. The handler hands it to
// context.Bind as an any, and echo's Context.Bind calls the instance's Binder
// — so a recorder installed as the binder is the one place a test can see the
// real struct, tags and value-placement alike. The inner binder still does the
// real decode, so what the recorder reads is what the request actually
// produced rather than a re-declared copy of it.
//
// A field missing from that struct is never bound, the handler's full
// replacement writes the zero value, and the column is cleared on every save.
func TestUpdateCompanyProfileBindsEveryField(t *testing.T) {
	var profile store.CompanyProfile
	want := markCompanyProfile(t, &profile)
	body, err := json.Marshal(wireBody(want))
	if err != nil {
		t.Fatalf("marshal sentinel body: %v", err)
	}

	recorder := &bindRecorder{inner: &echo.DefaultBinder{}}
	instance := echo.New()
	instance.Binder = recorder
	request := httptest.NewRequest(http.MethodPut, "/v1/admin/company", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	context := instance.NewContext(request, httptest.NewRecorder())
	// The handler audits only after a successful store write, so the user is
	// not strictly needed here; set it anyway so a future success path does
	// not panic on an empty context.
	context.Set("user", store.AuthUser{Role: "admin"})

	// The store is deliberately unconfigured. This guard is about what the
	// handler accepts off the wire, and a real database would only add a DSN
	// to a test that must run everywhere: UpdateCompanyProfile binds first and
	// validates second, so the recorder has seen everything by the time the
	// store answers "Postgres store is not initialized".
	server := NewServer(&store.Store{}, nil, nil)
	if err := server.updateCompanyProfile(context); err != nil {
		t.Fatalf("updateCompanyProfile returned an error: %v", err)
	}

	if recorder.notAStruct {
		t.Fatal("the handler bound a value that was not a struct pointer. This guard " +
			"reads the bound struct through echo's binder, and until it sees a struct " +
			"again it proves nothing.")
	}
	if !recorder.sawStruct {
		t.Fatal("the handler bound nothing this recorder could see. Either it stopped " +
			"binding a request struct or echo's binder changed shape, and until this " +
			"guard reads the real struct it proves nothing.")
	}

	var missing, wrong []string
	for wire, expected := range want {
		if !companyProfileIsWritable(expected.field) {
			continue
		}
		bound, present := recorder.fields[wire]
		if !present {
			missing = append(missing, fmt.Sprintf("CompanyProfile.%s (wire %q)", expected.field, wire))
			continue
		}
		if !reflect.DeepEqual(bound.value, expected.value) {
			wrong = append(wrong, valueMismatch(wire, expected.field, bound.value, expected.value))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("the PUT /v1/admin/company bind struct is missing [%s].\n\n"+
			"The endpoint full-replaces: a field it never binds is written as the zero "+
			"value, so the column is cleared on every save by an admin who touched "+
			"nothing in that section.", strings.Join(missing, ", "))
	}
	sort.Strings(wrong)
	if len(wrong) > 0 {
		t.Errorf("the bind struct read a value into the wrong field:\n  %s\n\n"+
			"The wire keys are derived from the Go field names, so this is a copy-paste "+
			"swap between the bind struct and companyDTO.", strings.Join(wrong, "\n  "))
	}

	var extra []string
	for wire, bound := range recorder.fields {
		if _, expected := want[wire]; !expected {
			extra = append(extra, fmt.Sprintf("%q (CompanyProfile.%s)", wire, bound.name))
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("the bind struct declares [%s], which no CompanyProfile field maps "+
			"to.\n\nThe bind struct drifted from the struct — check for a renamed field.",
			strings.Join(extra, ", "))
	}

	// The read-only exceptions must stay read-only. If the bind struct starts
	// accepting one, the write path changed and the exception needs revisiting
	// rather than quietly passing.
	for wire, expected := range want {
		if companyProfileIsWritable(expected.field) {
			continue
		}
		if _, bound := recorder.fields[wire]; !bound {
			continue
		}
		t.Errorf("the bind struct now carries %q (CompanyProfile.%s), which is "+
			"read-only on the write path: the store recomputes HasLogo from "+
			"logo_bytes and the artwork rides the logo endpoints. Delete the "+
			"field or move the exception in companyProfileIsWritable.",
			wire, expected.field)
	}
}

// wireBody flattens the derived expectations into the JSON body the sentinel
// probe sends.
func wireBody(want map[string]wireExpectation) map[string]any {
	body := make(map[string]any, len(want))
	for wire, expected := range want {
		body[wire] = expected.value
	}
	return body
}

// valueMismatch renders one wrong-value failure. An empty string where a
// sentinel was expected is the signature of two struct fields claiming the
// same json tag — encoding/json drops both rather than picking one — so the
// failure says that instead of leaving the reader hunting a bug that is not
// there.
func valueMismatch(wire, field string, got, want any) string {
	detail := fmt.Sprintf("wire %q carried %v; want the CompanyProfile.%s value %v",
		wire, got, field, want)
	if text, isText := got.(string); isText && text == "" {
		detail += "\n    (an empty value means the field was never bound: two struct fields " +
			"claiming the same json tag make encoding/json drop both)"
	}
	return detail
}

// boundField is one field of the struct a handler bound: its Go name and the
// value the decoder actually left in it.
type boundField struct {
	name  string
	value any
}

// bindRecorder sits in front of echo's binder so a test can see the request
// struct a handler binds. updateCompanyProfile declares that struct inline —
// there is no exported type to reflect on — and the handler passes it to
// context.Bind as an any, which echo forwards to the instance's Binder. The
// recorder delegates the real decode to the inner binder and then reads what
// the decode produced, so the guard observes the handler's own struct rather
// than a copy of it.
type bindRecorder struct {
	inner echo.Binder

	// sawStruct reports that a struct was observed at all, so a binder that
	// stops being called fails the guard instead of quietly proving nothing.
	sawStruct bool
	// notAStruct reports a bound value that was not a struct pointer.
	notAStruct bool
	fields     map[string]boundField
}

func (r *bindRecorder) Bind(target any, context echo.Context) error {
	if err := r.inner.Bind(target, context); err != nil {
		return err
	}
	r.record(target)
	return nil
}

func (r *bindRecorder) record(target any) {
	if r.fields == nil {
		r.fields = map[string]boundField{}
	}
	typ := reflect.TypeOf(target)
	if typ == nil || typ.Kind() != reflect.Pointer || typ.Elem().Kind() != reflect.Struct {
		r.notAStruct = true
		return
	}
	r.sawStruct = true
	value := reflect.ValueOf(target).Elem()
	for i := 0; i < typ.Elem().NumField(); i++ {
		field := typ.Elem().Field(i)
		tag := jsonFieldKey(field)
		if tag == "" || tag == "-" {
			continue
		}
		r.fields[tag] = boundField{name: field.Name, value: value.Field(i).Interface()}
	}
}

// jsonFieldKey is the JSON key a struct field decodes under: its json tag's
// name when it carries one, its Go field name when it does not (which is what
// encoding/json matches on, case-insensitively — see the file's note on that).
func jsonFieldKey(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	if comma := strings.IndexByte(tag, ','); comma >= 0 {
		tag = tag[:comma]
	}
	return tag
}

// A note on the fourth mirror, recorded here because it is the one hop with no
// guard, and the next reader deserves the decision rather than the silence.
//
// The store's SQL uses explicit column lists — the GetCompanyProfile SELECT,
// the UpdateCompanyProfile SET/RETURNING, and the SetCompanyLogo
// SET/RETURNING, all in backend/store/company.go. A struct field with no
// column in a list is silently never persisted: loss, not a wipe, and nobody
// notices until someone reads the field back and finds it empty. A guard for
// it would have to scrape SQL text and map snake_case column names to Go field
// names by convention, which re-introduces by hand the very mapping it claims
// to protect and pins the SQL's whitespace rather than its meaning.
//
// The property that actually matters — every field written is read back —
// needs a database, so its home is backend/store/company_test.go's
// TestCompanyProfileRoundTrip, which writes a profile and reads it back. That
// test skips without VERA_POSTGRES_TEST_DSN, so it is not something a bare
// `go test ./...` can lean on, and the two guards in this file are pure for
// exactly that reason. Until someone wants the database-side guard, the
// honest control on this mirror is the migration set: 010, 013, 014 and 022
// add the columns, and the schema is the only place the column list is
// declared for the database to see.
//
// What that round trip does not cover today, so it is written down rather than
// assumed: the profile the update returns is checked, and the read-back
// comparison covers the identity and logo fields, but it compares neither
// stored.Currency nor any of the eight theme colours. A column missing from a
// list is therefore still quiet there. Closing that gap is a store-test change
// and belongs to its own issue.

// The fourth mirror, deliberately unguarded: company.go's request-to-profile
// copy literal (the store.CompanyProfile{...} built from the bound request).
// A field present in the bind struct, the DTO, the store struct and the SQL,
// but omitted from that literal, still wipes on save with both guards above
// green -- a third wipe vector in the same function. It cannot be seen from
// outside the process (the store is a concrete *store.Store, so the literal's
// output cannot be intercepted), and the cheap reliable fix -- naming the
// anonymous request struct so the literal can be compared against it by
// reflection -- is a company.go edit. Recorded here so the next reader meets
// it: if you add a profile field, touch the literal too, or name the struct
// and guard it.
