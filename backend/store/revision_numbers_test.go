package store

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// recordingTracer captures the SQL every statement runs.
//
// Without it the property below is invisible: the difference between a query
// that reads a revision number and one that also drags a whole PDF out of
// Postgres is a few words in a SELECT, and the endpoint's response looks
// identical either way. That is how the revisions endpoint came to fetch the
// largest column in the database to return a list of int16s.
type recordingTracer struct {
	mu      sync.Mutex
	queries []string
}

func (t *recordingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queries = append(t.queries, data.SQL)
	return ctx
}

func (t *recordingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (t *recordingTracer) recorded() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.queries...)
}

func (t *recordingTracer) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queries = nil
}

// mentions reports whether any recorded statement contains the fragment.
func (t *recordingTracer) mentions(fragment string) bool {
	for _, query := range t.recorded() {
		if strings.Contains(query, fragment) {
			return true
		}
	}
	return false
}

// openTracedSchema is openTestSchema with a tracer attached.
func openTracedSchema(t *testing.T, schema string) (*Store, *recordingTracer) {
	t.Helper()
	resetTestSchema(t, testDSN(t), schema)
	tracer := &recordingTracer{}
	database, err := Open(context.Background(), Config{DSN: testDSN(t), Schema: schema, Tracer: tracer})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	return database, tracer
}

// documentColumns are the columns that carry a rendered document. Reaching for
// any of them to answer a question about revision numbers is the mistake this
// guards.
var documentColumns = []string{"pdf_data", "html_snapshot", "quotation_documents", "pdf_path"}

// TestListRevisionNumbersDoesNotFetchTheDocument is the guard.
//
// The revisions endpoint used to call GetQuotation, which selects html_snapshot
// and pdf_data -- so every click moved the whole document out of Postgres to
// return a handful of integers. Nothing about the response revealed it, so the
// only way to hold the line is to assert on what is actually sent.
func TestListRevisionNumbersDoesNotFetchTheDocument(t *testing.T) {
	database, tracer := openTracedSchema(t, "vera_test_revision_numbers")

	tracer.reset()
	if _, err := database.ListRevisionNumbers(context.Background(), 1); err != nil {
		t.Fatalf("ListRevisionNumbers: %v", err)
	}

	// A tracer that recorded nothing would make every assertion below pass
	// without proving anything.
	if len(tracer.recorded()) == 0 {
		t.Fatal("the tracer recorded no statements, so this test proves nothing")
	}
	for _, column := range documentColumns {
		if tracer.mentions(column) {
			t.Errorf("listing revision numbers reached for %q:\n%s",
				column, strings.Join(tracer.recorded(), "\n"))
		}
	}
}

// TestTheTracerSeesDocumentReads proves the guard above can fail.
//
// GetQuotation legitimately selects those columns. If the tracer did not see
// them here, it would not see them in ListRevisionNumbers either, and the test
// above would be a green tick over nothing.
func TestTheTracerSeesDocumentReads(t *testing.T) {
	database, tracer := openTracedSchema(t, "vera_test_tracer_probe")

	tracer.reset()
	// A missing quotation is fine: the statement still runs and is still traced.
	_, _ = database.GetQuotation(context.Background(), 1, nil)

	for _, column := range []string{"pdf_data", "html_snapshot"} {
		if !tracer.mentions(column) {
			t.Errorf("the tracer did not see %q in GetQuotation, so it cannot be "+
				"trusted to see it in ListRevisionNumbers either", column)
		}
	}
}

// TestListRevisionNumbersIsNeverNil pins the shape the API serialises.
//
// A nil slice marshals to JSON null, and the desktop read .length on one of
// those once and unmounted the whole window (issue #282). The endpoint relies on
// this store method to return an empty list rather than nothing.
func TestListRevisionNumbersIsNeverNil(t *testing.T) {
	database, _ := openTracedSchema(t, "vera_test_revision_numbers_nil")

	revisions, err := database.ListRevisionNumbers(context.Background(), 424242)
	if err != nil {
		t.Fatalf("ListRevisionNumbers: %v", err)
	}
	if revisions == nil {
		t.Fatal("returned nil; it would serialise as JSON null on the wire")
	}
	if len(revisions) != 0 {
		t.Fatalf("revisions = %v; want empty for an unknown quotation", revisions)
	}
}
