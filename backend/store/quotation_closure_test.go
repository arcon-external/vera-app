package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// Issue #656: a quotation is closed explicitly, by an admin, with a reason.
// Closing is what turns the unbought remainder from Unsold (pipeline, not a
// loss) into Closed Loss; reopening restores it. These tests drive the store
// seam: who/when/why is stored, a close is idempotent rather than rewritten,
// and the audit survives the reopen that clears the live columns.

// quotationClosureAdmin creates the admin actor closure requires.
func quotationClosureAdmin(t *testing.T, database *Store) AuthUser {
	t.Helper()
	user, err := database.CreateUser(context.Background(),
		fmt.Sprintf("closure-admin-%d@example.com", time.Now().UnixNano()),
		"Closure Admin", "hash", "admin")
	if err != nil {
		t.Fatalf("create closure admin: %v", err)
	}
	return user
}

type closureEventRow struct {
	Action string
	Reason string
	Actor  int64
}

func closureEvents(t *testing.T, database *Store, quotationID int64) []closureEventRow {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	rows, err := pool.Query(context.Background(), fmt.Sprintf(`
		SELECT action, reason, actor_id
		FROM %s.quotation_closure_events
		WHERE quotation_id = $1
		ORDER BY id`, database.configSchema()), quotationID)
	if err != nil {
		t.Fatalf("read closure events: %v", err)
	}
	defer rows.Close()
	events := make([]closureEventRow, 0, 2)
	for rows.Next() {
		var event closureEventRow
		if err := rows.Scan(&event.Action, &event.Reason, &event.Actor); err != nil {
			t.Fatalf("scan closure event: %v", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate closure events: %v", err)
	}
	return events
}

func closureSeedQuotation(t *testing.T, database *Store) QuotationRecord {
	t.Helper()
	return seedReportQuotation(t, database, 2, "PT. Closure", "Closure source", "IDR", "FTZ",
		[]reportQuotationLine{{
			Description: "WIP rack", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000,
		}})
}

// TestCloseQuotationStoresWhoWhenWhy pins the happy path and the admin gate:
// staff are refused, an admin close records the actor's name, the instant and
// the trimmed reason, and both the detail and the archive can see it.
func TestCloseQuotationStoresWhoWhenWhy(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_store")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := closureSeedQuotation(t, database)

	if _, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", staff.ID); !errors.Is(err, ErrQuotationCloseForbidden) {
		t.Fatalf("staff close error = %v; want ErrQuotationCloseForbidden", err)
	}
	if events := closureEvents(t, database, quotation.ID); len(events) != 0 {
		t.Fatalf("a refused close wrote %d audit events; want none", len(events))
	}

	before := time.Now().Add(-time.Second)
	closed, err := database.CloseQuotation(ctx, quotation.ID, "  customer cancelled the remainder  ", admin.ID)
	if err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	if closed.ClosedBy != admin.ID {
		t.Errorf("closedBy = %d; want %d", closed.ClosedBy, admin.ID)
	}
	if closed.ClosedByName != admin.DisplayName {
		t.Errorf("closedByName = %q; want %q", closed.ClosedByName, admin.DisplayName)
	}
	if closed.Reason != "customer cancelled the remainder" {
		t.Errorf("reason = %q; want the trimmed sentence", closed.Reason)
	}
	if closed.ClosedAt.Before(before) || closed.ClosedAt.After(time.Now().Add(time.Second)) {
		t.Errorf("closedAt = %s; want the instant of the close", closed.ClosedAt)
	}

	record, err := database.GetQuotation(ctx, quotation.ID, nil)
	if err != nil {
		t.Fatalf("GetQuotation: %v", err)
	}
	if record.ClosedAt == nil || record.ClosedByName != admin.DisplayName ||
		record.ClosedReason != "customer cancelled the remainder" {
		t.Errorf("quotation closure = %v/%q/%q; want the stored close",
			record.ClosedAt, record.ClosedByName, record.ClosedReason)
	}

	other := closureSeedQuotation(t, database)
	summaries, err := database.ListQuotations(ctx)
	if err != nil {
		t.Fatalf("ListQuotations: %v", err)
	}
	closedFlags := map[int64]bool{}
	for _, summary := range summaries {
		closedFlags[summary.ID] = summary.Closed
	}
	if !closedFlags[quotation.ID] {
		t.Error("closed quotation is not marked closed in the archive list")
	}
	if closedFlags[other.ID] {
		t.Error("an open quotation is marked closed in the archive list")
	}

	events := closureEvents(t, database, quotation.ID)
	if len(events) != 1 || events[0].Action != "close" ||
		events[0].Reason != "customer cancelled the remainder" || events[0].Actor != admin.ID {
		t.Errorf("closure events = %+v; want one close with the actor and reason", events)
	}
}

// TestCloseQuotationRefusesASecondClose pins idempotence as a conflict: the
// second close must not rewrite the first one's who/when/why or append a
// second audit event.
func TestCloseQuotationRefusesASecondClose(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_twice")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := closureSeedQuotation(t, database)

	first, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", admin.ID)
	if err != nil {
		t.Fatalf("first close: %v", err)
	}
	if _, err := database.CloseQuotation(ctx, quotation.ID, "a different reason", admin.ID); !errors.Is(err, ErrQuotationAlreadyClosed) {
		t.Fatalf("second close error = %v; want ErrQuotationAlreadyClosed", err)
	}

	record, err := database.GetQuotation(ctx, quotation.ID, nil)
	if err != nil {
		t.Fatalf("GetQuotation: %v", err)
	}
	if record.ClosedAt == nil || !record.ClosedAt.Equal(first.ClosedAt) ||
		record.ClosedReason != "customer cancelled" {
		t.Errorf("stored closure = %v/%q; want the first close untouched",
			record.ClosedAt, record.ClosedReason)
	}
	if events := closureEvents(t, database, quotation.ID); len(events) != 1 {
		t.Errorf("audit events = %+v; want exactly the first close", events)
	}
}

// TestCloseQuotationRequiresAReason keeps "why" mandatory: the empty and the
// whitespace-only reason are both refused before anything is written.
func TestCloseQuotationRequiresAReason(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_reason")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := closureSeedQuotation(t, database)

	for _, reason := range []string{"", "   ", "\n\t "} {
		if _, err := database.CloseQuotation(ctx, quotation.ID, reason, admin.ID); !errors.Is(err, ErrQuotationCloseReasonRequired) {
			t.Errorf("reason %q error = %v; want ErrQuotationCloseReasonRequired", reason, err)
		}
	}
	record, err := database.GetQuotation(ctx, quotation.ID, nil)
	if err != nil {
		t.Fatalf("GetQuotation: %v", err)
	}
	if record.ClosedAt != nil {
		t.Errorf("a reason-less close left the quotation closed at %v", record.ClosedAt)
	}
	if events := closureEvents(t, database, quotation.ID); len(events) != 0 {
		t.Errorf("a refused close wrote %d audit events; want none", len(events))
	}
}

// TestCloseQuotationRefusesAnInactiveAdmin proves the store's own re-check:
// the route's middleware is not the only gate.
func TestCloseQuotationRefusesAnInactiveAdmin(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_inactive")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := closureSeedQuotation(t, database)

	if err := database.SetUserActive(ctx, admin.ID, false); err != nil {
		t.Fatalf("deactivate admin: %v", err)
	}
	if _, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", admin.ID); !errors.Is(err, ErrQuotationCloseForbidden) {
		t.Fatalf("inactive admin close error = %v; want ErrQuotationCloseForbidden", err)
	}
}

// TestCloseQuotationRefusesAnUnknownQuotation keeps "gone" distinct from a
// database failure; the closure endpoints answer not-found, not 500.
func TestCloseQuotationRefusesAnUnknownQuotation(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_missing")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)

	if _, err := database.CloseQuotation(ctx, 9_999_999, "customer cancelled", admin.ID); !errors.Is(err, ErrPOSourceNotFound) {
		t.Fatalf("unknown quotation close error = %v; want ErrPOSourceNotFound", err)
	}
	if _, err := database.ReopenQuotation(ctx, 9_999_999, admin.ID); !errors.Is(err, ErrPOSourceNotFound) {
		t.Fatalf("unknown quotation reopen error = %v; want ErrPOSourceNotFound", err)
	}
}

// TestReopenQuotationRestoresOpenAndKeepsTheAudit pins the asymmetry that
// motivated the event table: the live columns are cleared, but who closed it,
// when and why survives the reopen.
func TestReopenQuotationRestoresOpenAndKeepsTheAudit(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_reopen")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := closureSeedQuotation(t, database)

	closed, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", admin.ID)
	if err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	reopened, err := database.ReopenQuotation(ctx, quotation.ID, admin.ID)
	if err != nil {
		t.Fatalf("ReopenQuotation: %v", err)
	}
	if reopened.ClosedAt.IsZero() || reopened.Reason != "customer cancelled" ||
		reopened.ClosedByName != admin.DisplayName {
		t.Errorf("reopen returned %+v; want the closure it removed", reopened)
	}
	if !reopened.ClosedAt.Equal(closed.ClosedAt) {
		t.Errorf("reopen returned closedAt %s; want the original %s",
			reopened.ClosedAt, closed.ClosedAt)
	}

	record, err := database.GetQuotation(ctx, quotation.ID, nil)
	if err != nil {
		t.Fatalf("GetQuotation: %v", err)
	}
	if record.ClosedAt != nil || record.ClosedByName != "" || record.ClosedReason != "" {
		t.Errorf("reopened record closure = %v/%q/%q; want open",
			record.ClosedAt, record.ClosedByName, record.ClosedReason)
	}
	if _, err := database.ReopenQuotation(ctx, quotation.ID, admin.ID); !errors.Is(err, ErrQuotationNotClosed) {
		t.Fatalf("second reopen error = %v; want ErrQuotationNotClosed", err)
	}

	events := closureEvents(t, database, quotation.ID)
	if len(events) != 2 {
		t.Fatalf("audit events = %+v; want a close and a reopen", events)
	}
	if events[0].Action != "close" || events[0].Reason != "customer cancelled" ||
		events[0].Actor != admin.ID {
		t.Errorf("close event = %+v; want the original close", events[0])
	}
	if events[1].Action != "reopen" || events[1].Reason != "" || events[1].Actor != admin.ID {
		t.Errorf("reopen event = %+v; want an actor-only reopen", events[1])
	}
}

// TestCloseAndReopenDoNotTouchPurchaseOrdersOrRevisions pins the boundary:
// closure is quotation state, never a rewrite of what was already bought or
// what was already issued.
func TestCloseAndReopenDoNotTouchPurchaseOrdersOrRevisions(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_untouched")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := closureSeedQuotation(t, database)
	saved := finalizeAvailabilityQuantity(t, database, ctx, staff, quotation, "PO-CLOSURE-3", 3*QuantityScale)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	allocationSnapshot := func() string {
		var snapshot string
		if err := pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT COALESCE(string_agg(
				id::text || ':' || purchase_order_id::text || ':' ||
				quotation_item_no::text || ':' || quantity_scaled::text || ':' ||
				revenue_minor::text, ',' ORDER BY id), '')
			FROM %s.purchase_order_allocations
			WHERE quotation_id = $1`, schema), quotation.ID).Scan(&snapshot); err != nil {
			t.Fatalf("read allocation snapshot: %v", err)
		}
		return snapshot
	}
	revisionSnapshot := func() string {
		var snapshot string
		if err := pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT COALESCE(string_agg(
				id::text || ':' || revision_number::text || ':' || created_at::text,
				',' ORDER BY revision_number), '')
			FROM %s.quotation_revisions WHERE quotation_id = $1`, schema),
			quotation.ID).Scan(&snapshot); err != nil {
			t.Fatalf("read revision snapshot: %v", err)
		}
		return snapshot
	}
	allocationsBefore, revisionsBefore := allocationSnapshot(), revisionSnapshot()
	poBefore, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("GetPurchaseOrder before close: %v", err)
	}

	if _, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	if _, err := database.ReopenQuotation(ctx, quotation.ID, admin.ID); err != nil {
		t.Fatalf("ReopenQuotation: %v", err)
	}

	if after := allocationSnapshot(); after != allocationsBefore {
		t.Errorf("allocations changed across close/reopen:\nbefore %s\nafter  %s",
			allocationsBefore, after)
	}
	if after := revisionSnapshot(); after != revisionsBefore {
		t.Errorf("revisions changed across close/reopen:\nbefore %s\nafter  %s",
			revisionsBefore, after)
	}
	poAfter, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("GetPurchaseOrder after reopen: %v", err)
	}
	if poAfter.State != poBefore.State || poAfter.Version != poBefore.Version {
		t.Errorf("purchase order = %s/v%d; want %s/v%d untouched",
			poAfter.State, poAfter.Version, poBefore.State, poBefore.Version)
	}
}

// TestCloseQuotationSerialisesConcurrentCloses proves the FOR UPDATE lock the
// hard way: two admins closing at once produce one closure and one conflict,
// never two different reasons overwriting each other.
func TestCloseQuotationSerialisesConcurrentCloses(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_race")
	admin := quotationClosureAdmin(t, database)
	quotation := closureSeedQuotation(t, database)

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		reason := fmt.Sprintf("concurrent close %d", i)
		go func() {
			_, err := database.CloseQuotation(context.Background(), quotation.ID, reason, admin.ID)
			results <- err
		}()
	}
	succeeded, conflicted := 0, 0
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrQuotationAlreadyClosed):
			conflicted++
		default:
			t.Fatalf("concurrent close error = %v; want nil or ErrQuotationAlreadyClosed", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent closes = %d succeeded / %d conflicted; want 1/1",
			succeeded, conflicted)
	}
	if events := closureEvents(t, database, quotation.ID); len(events) != 1 {
		t.Errorf("audit events = %+v; want exactly one close", events)
	}
}

// TestQuotationClosureCheckRefusesAnIncompleteClose proves migration 056's
// CHECK is a real guard, not documentation: a closed row must carry an actor
// and a non-blank reason, and an open row must carry neither.
func TestQuotationClosureCheckRefusesAnIncompleteClose(t *testing.T) {
	database := openTestSchema(t, "vera_quotation_closure_check")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := closureSeedQuotation(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	incomplete := []struct {
		name      string
		updates   string
		usesActor bool
	}{
		{name: "closed at with no actor", updates: "closed_at = NOW()"},
		{name: "closed at and actor with no reason", updates: "closed_at = NOW(), closed_by = $2", usesActor: true},
		{name: "blank reason", updates: "closed_at = NOW(), closed_by = $2, closed_reason = '   '", usesActor: true},
		{name: "actor while open", updates: "closed_by = $2", usesActor: true},
		{name: "reason while open", updates: "closed_reason = 'cancelled'"},
	}
	for _, test := range incomplete {
		args := []any{quotation.ID}
		if test.usesActor {
			args = append(args, admin.ID)
		}
		_, err := pool.Exec(ctx, fmt.Sprintf(
			`UPDATE %s.quotations SET %s WHERE id = $1`, schema, test.updates), args...)
		if err == nil {
			t.Errorf("%s was accepted; the closure CHECK must refuse it", test.name)
			continue
		}
		if !isSQLState(t, err, "23514") {
			t.Errorf("%s error = %v; want SQLSTATE 23514", test.name, err)
		}
	}

	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotations
		SET closed_at = NOW(), closed_by = $2, closed_reason = 'cancelled'
		WHERE id = $1`, schema), quotation.ID, admin.ID); err != nil {
		t.Fatalf("complete closure was refused: %v", err)
	}

	// The audit table is append-only in practice, but the action vocabulary is
	// still the database's: an unknown action is refused, not stored.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_closure_events (quotation_id, action, actor_id)
		VALUES ($1, 'archive', $2)`, schema), quotation.ID, admin.ID); err == nil {
		t.Error("the event table accepted an unknown action")
	} else if !isSQLState(t, err, "23514") {
		t.Errorf("unknown event action error = %v; want SQLSTATE 23514", err)
	}
}
