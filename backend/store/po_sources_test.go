package store

import (
	"errors"
	"testing"
)

// Issue #483: the selector's page is caller input and its offset is
// (page-1)*POSourcePageSize. A page the HTTP handler accepted could overflow
// that product into a negative OFFSET, which Postgres rejected as a database
// error and the API surfaced as a 500. The store refuses the page as invalid
// input instead.
func TestListPOSourceQuotationsRejectsAPageWhoseOffsetOverflows(t *testing.T) {
	database := openTestSchema(t, "vera_po_source_page_overflow")
	ctx := poContext(t, database)

	// 614891469123651722 = floor(MaxInt64/15)+2, so (page-1)*15 wraps
	// negative and Postgres rejects the OFFSET. That must be invalid input,
	// never a database error the caller sees as a server failure.
	if _, _, err := database.ListPOSourceQuotations(ctx, 614891469123651722, "", false); !errors.Is(err, ErrPOSourceInvalid) {
		t.Errorf("overflowing page error = %v; want ErrPOSourceInvalid", err)
	}

	// One less is representable: an empty page past the end, not a refusal.
	// This pins that the guard rejects only the unrepresentable offset and
	// never clamps.
	rows, total, err := database.ListPOSourceQuotations(ctx, 614891469123651721, "", false)
	if err != nil {
		t.Fatalf("largest representable page: %v", err)
	}
	if len(rows) != 0 || total != 0 {
		t.Errorf("boundary page = %d rows, total %d; want an empty page past the end",
			len(rows), total)
	}
}

// TestPOSourcePageOverflowErrorIsNotNotFound keeps the new sentinel distinct
// from ErrPOSourceNotFound: a page past the offset limit is malformed input,
// not a missing Quotation, and the handler's 404 branch must not catch it.
func TestPOSourcePageOverflowErrorIsNotNotFound(t *testing.T) {
	database := openTestSchema(t, "vera_po_source_page_overflow_kind")
	ctx := poContext(t, database)

	_, _, err := database.ListPOSourceQuotations(ctx, 614891469123651722, "", false)
	if errors.Is(err, ErrPOSourceNotFound) {
		t.Fatalf("overflowing page error = %v; must not be ErrPOSourceNotFound", err)
	}
}
