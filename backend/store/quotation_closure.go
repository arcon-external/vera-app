package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Issue #656: explicit quotation closure.
//
// Closing a quotation is a commercial decision, not a cleanup: the unbought
// remainder stops being Unsold (a pipeline figure that is never subtracted from
// realized variance) and becomes Closed Loss (a realized loss that is). It takes
// an admin and a reason, it is refused while already closed, and reopening
// restores Unsold.
//
// Closure lives on the quotation row, not on a revision: a revision is an
// immutable document, while "we gave up on the rest of this order" is a fact
// about the whole quotation. Reopening clears the three live columns, so every
// action is also appended to quotation_closure_events; that table is the record
// of who closed it, when and why after the live columns are gone.

// QuotationClosure is the stored closure: who closed the quotation, when, and
// why. CloseQuotation returns the closure it wrote; ReopenQuotation returns the
// closure it removed.
type QuotationClosure struct {
	ClosedAt     time.Time
	ClosedBy     int64
	ClosedByName string
	Reason       string
}

// ErrQuotationClosed refuses work that needs an open quotation. It is a distinct
// sentinel because the remedy is different from "already bought": reopen it.
var ErrQuotationClosed = errors.New("this quotation is closed; reopen it before ordering or revising")

// ErrQuotationAlreadyClosed and ErrQuotationNotClosed are the idempotence
// conflicts. A second close does not rewrite the first one's who/when/why, and
// a reopen of an open quotation is not a silent success.
var (
	ErrQuotationAlreadyClosed = errors.New("this quotation is already closed")
	ErrQuotationNotClosed     = errors.New("this quotation is not closed")
)

// ErrQuotationCloseReasonRequired keeps "why" mandatory: a closure with no
// reason is not an auditable decision.
var ErrQuotationCloseReasonRequired = errors.New("closing a quotation needs a reason")

// MaxQuotationCloseReasonLength bounds the reason at 500 characters, not bytes:
// the sentence is read by people, and a rejection at 500 ideographs must not be
// a rejection at 500/3 of them. The database's CHECK refuses blank, not long.
const MaxQuotationCloseReasonLength = 500

// ErrQuotationCloseReasonTooLong refuses a reason over the bound. It is its own
// sentinel so the wire can say "too long" rather than mislabel it as missing.
var ErrQuotationCloseReasonTooLong = fmt.Errorf(
	"the close reason must be at most %d characters", MaxQuotationCloseReasonLength)

// ErrQuotationCloseForbidden refuses close or reopen from anyone but an enabled
// admin. The route's middleware is the first gate; the store re-checks the
// stored role inside the write transaction, exactly as the PO repair path does.
var ErrQuotationCloseForbidden = errors.New("closing or reopening a quotation is an admin action")

// CloseQuotation closes one quotation for the given actor.
//
// A quotation that does not exist answers ErrPOSourceNotFound, the package's
// quotation-not-found sentinel. The reason is trimmed; an empty result is
// ErrQuotationCloseReasonRequired. Closing an already-closed quotation is
// ErrQuotationAlreadyClosed rather than a rewrite. Closing never touches a
// purchase order, an allocation or a revision.
func (s *Store) CloseQuotation(
	ctx context.Context, quotationID int64, reason string, actorID int64,
) (QuotationClosure, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return QuotationClosure{}, ErrQuotationCloseReasonRequired
	}
	if utf8.RuneCountInString(reason) > MaxQuotationCloseReasonLength {
		return QuotationClosure{}, ErrQuotationCloseReasonTooLong
	}
	pool, err := s.poolOrError()
	if err != nil {
		return QuotationClosure{}, err
	}
	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return QuotationClosure{}, fmt.Errorf("begin quotation close: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	actorName, err := requireQuotationClosureAdmin(ctx, tx, schema, actorID)
	if err != nil {
		return QuotationClosure{}, err
	}
	// The quotation row is the lock: closure is one decision per quotation, so
	// two admins closing at once produce one closure and one conflict.
	current, err := readQuotationClosureForUpdate(ctx, tx, schema, quotationID)
	if err != nil {
		return QuotationClosure{}, err
	}
	if current.ClosedAt != nil {
		return QuotationClosure{}, ErrQuotationAlreadyClosed
	}

	var closedAt time.Time
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.quotations
		SET closed_at = NOW(), closed_by = $2, closed_reason = $3,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING closed_at`, schema),
		quotationID, actorID, reason).Scan(&closedAt); err != nil {
		return QuotationClosure{}, fmt.Errorf("close quotation %d: %w", quotationID, err)
	}
	if err := appendQuotationClosureEvent(ctx, tx, schema, quotationID, "close", reason, actorID); err != nil {
		return QuotationClosure{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return QuotationClosure{}, fmt.Errorf("commit quotation close: %w", err)
	}
	return QuotationClosure{
		ClosedAt:     closedAt,
		ClosedBy:     actorID,
		ClosedByName: actorName,
		Reason:       reason,
	}, nil
}

// ReopenQuotation restores a closed quotation. It returns the closure it
// removed, because that is the record the live columns no longer carry.
//
// Reopening an open quotation is ErrQuotationNotClosed. Like a close, a reopen
// never touches a purchase order, an allocation or a revision: the allocation
// rows that were bought stay exactly where they were.
func (s *Store) ReopenQuotation(
	ctx context.Context, quotationID int64, actorID int64,
) (QuotationClosure, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return QuotationClosure{}, err
	}
	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return QuotationClosure{}, fmt.Errorf("begin quotation reopen: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := requireQuotationClosureAdmin(ctx, tx, schema, actorID); err != nil {
		return QuotationClosure{}, err
	}
	current, err := readQuotationClosureForUpdate(ctx, tx, schema, quotationID)
	if err != nil {
		return QuotationClosure{}, err
	}
	if current.ClosedAt == nil {
		return QuotationClosure{}, ErrQuotationNotClosed
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotations
		SET closed_at = NULL, closed_by = NULL, closed_reason = '',
		    updated_at = NOW()
		WHERE id = $1`, schema), quotationID); err != nil {
		return QuotationClosure{}, fmt.Errorf("reopen quotation %d: %w", quotationID, err)
	}
	if err := appendQuotationClosureEvent(ctx, tx, schema, quotationID, "reopen", "", actorID); err != nil {
		return QuotationClosure{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return QuotationClosure{}, fmt.Errorf("commit quotation reopen: %w", err)
	}
	return QuotationClosure{
		ClosedAt:     *current.ClosedAt,
		ClosedBy:     current.ClosedBy,
		ClosedByName: current.ClosedByName,
		Reason:       current.Reason,
	}, nil
}

// quotationClosureRow is the locked quotation's live closure columns.
type quotationClosureRow struct {
	ClosedAt     *time.Time
	ClosedBy     int64
	ClosedByName string
	Reason       string
}

// readQuotationClosureForUpdate locks the quotation row and reads its current
// closure. FOR UPDATE OF q is required because the actor name is a left join:
// locking the users side would serialise admin profile writes for no reason.
func readQuotationClosureForUpdate(
	ctx context.Context, q purchaseOrderQuerier, schema string, quotationID int64,
) (quotationClosureRow, error) {
	var row quotationClosureRow
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT q.closed_at, COALESCE(q.closed_by, 0),
		       COALESCE(cb.display_name, ''), q.closed_reason
		FROM %s.quotations q
		LEFT JOIN %s.users cb ON cb.id = q.closed_by
		WHERE q.id = $1
		FOR UPDATE OF q`, schema, schema), quotationID).Scan(
		&row.ClosedAt, &row.ClosedBy, &row.ClosedByName, &row.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return quotationClosureRow{}, ErrPOSourceNotFound
	}
	if err != nil {
		return quotationClosureRow{}, fmt.Errorf(
			"read quotation %d closure: %w", quotationID, err)
	}
	return row, nil
}

// appendQuotationClosureEvent records one close or reopen. The event is
// append-only: nothing updates or deletes it, so the history survives the
// reopen that clears the live columns.
func appendQuotationClosureEvent(
	ctx context.Context, q purchaseOrderQuerier, schema string,
	quotationID int64, action, reason string, actorID int64,
) error {
	if _, err := q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_closure_events (quotation_id, action, reason, actor_id)
		VALUES ($1, $2, $3, $4)`, schema),
		quotationID, action, reason, actorID); err != nil {
		return fmt.Errorf("record quotation %d %s event: %w", quotationID, action, err)
	}
	return nil
}

// requireQuotationClosureAdmin re-reads the actor's stored role inside the
// write transaction and returns the display name the closure records. The
// route's middleware is the first gate; this is the one a direct store caller
// cannot skip.
func requireQuotationClosureAdmin(
	ctx context.Context, q purchaseOrderQuerier, schema string, userID int64,
) (string, error) {
	if userID < 1 {
		return "", ErrQuotationCloseForbidden
	}
	var displayName, role string
	var active bool
	err := q.QueryRow(ctx, fmt.Sprintf(
		`SELECT display_name, role, is_active FROM %s.users WHERE id = $1`, schema),
		userID).Scan(&displayName, &role, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrQuotationCloseForbidden
	}
	if err != nil {
		return "", fmt.Errorf("read the actor of quotation closure: %w", err)
	}
	if role != "admin" || !active {
		return "", ErrQuotationCloseForbidden
	}
	return displayName, nil
}
