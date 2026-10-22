package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"vera-app/backend/apicontract"
)

// Issue #474: the optional customer-system confirmation number.
//
// A confirmation number is a reference staff submit manually to the customer's
// system. Vera stores and copies it; it never submits anything. The value is
// null until staff explicitly ask for it, at which point it is allocated from
// an independent customer-year counter in the same transaction that saves the
// Purchase Order. Once issued it is immutable, and a deleted record's number is
// never reissued.
//
// # Format and bound
//
// "23" replaces the quotation's "15"; the customer code, two-digit assignment
// year (WIB, company_date.go) and four-digit sequence follow the approved
// quotation numbering contract. The first number of a customer-year cycle is
// 0001. At 8000 the allocation reports a warning while numbers keep issuing;
// past 9999 the sequence widens to five digits instead of wrapping, and at
// POConfirmationSequenceMax the allocator refuses rather than reissuing.
//
// # Counter independence
//
// The counter key lives in the same sequence_counters table under its own
// "po-confirmation:" namespace. Quotation issuance and PO confirmation numbers
// therefore never consume each other, and each new January starts a fresh
// customer cycle.
const (
	// POConfirmationWarnAt is the customer-year sequence at which allocation
	// starts reporting a warning. It is the quotation threshold: there is one
	// numbering policy, reused rather than re-invented.
	POConfirmationWarnAt = SequenceWarnAt
	// POConfirmationSequenceMax is the defense-only hard cap. Reaching it is
	// implausible; past 9999 the number widens to five digits, and at the cap
	// allocation refuses instead of wrapping.
	POConfirmationSequenceMax = SequenceMax
)

var (
	// ErrPOConfirmationExhausted means one customer-year cycle reached
	// POConfirmationSequenceMax. It is a refusal, never a wrap or a reissue.
	ErrPOConfirmationExhausted = errors.New("purchase order confirmation sequence is exhausted")
	// ErrPurchaseOrderNotSaved refuses a later assignment on a draft. A
	// confirmation number belongs to a finalized record; assigning one to a
	// draft would put an issued identifier on entry that is still changing.
	ErrPurchaseOrderNotSaved = errors.New("purchase order is not saved")
)

// POConfirmationSequenceKey builds the counter key for one customer-year
// confirmation cycle, e.g. po-confirmation:default:02:26. It shares the
// tenancy slot and the two-digit year with quotation numbering but no counter:
// the namespaces are disjoint.
func POConfirmationSequenceKey(customerCode, yy int) string {
	return fmt.Sprintf("po-confirmation:%s:%02d:%02d", sequenceTenant, customerCode, yy)
}

// poAssignmentYY reports the two-digit assignment year in the company's WIB
// date context. It is the assignment date's year, not the received date's:
// the parent specification separates the two (stories 34 and 65).
func poAssignmentYY(now time.Time) int {
	return companyToday(now).Year() % 100
}

func checkPOConfirmationScope(customerCode, yy int) error {
	if customerCode < 1 || customerCode > 99 {
		return fmt.Errorf("invalid customer code %d: want 1-99", customerCode)
	}
	if yy < 0 || yy > 99 {
		return fmt.Errorf("invalid year %d: want 0-99", yy)
	}
	return nil
}

// nextPOConfirmationSequenceTx allocates one sequence atomically. The INSERT
// seeds a new cycle at 2 and returns 1; the conflict arm increments under the
// row lock Postgres takes for the update. A refused allocation (the cycle is
// at POConfirmationSequenceMax) returns ErrPOConfirmationExhausted and writes
// nothing.
//
// Because this runs inside the caller's transaction, a save that fails after
// allocation rolls the increment back: a refused attempt leaves no gap.
func nextPOConfirmationSequenceTx(ctx context.Context, tx pgx.Tx, schema string, customerCode, yy int) (sequence int64, warn bool, err error) {
	if err := checkPOConfirmationScope(customerCode, yy); err != nil {
		return 0, false, err
	}
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.sequence_counters (name, next_value)
		VALUES ($1, 2)
		ON CONFLICT (name) DO UPDATE
			SET next_value = sequence_counters.next_value + 1
			WHERE sequence_counters.next_value <= $2
		RETURNING next_value - 1`, schema),
		POConfirmationSequenceKey(customerCode, yy), POConfirmationSequenceMax).Scan(&sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, true, fmt.Errorf("%w: customer %d year %d reached %d",
			ErrPOConfirmationExhausted, customerCode, yy, POConfirmationSequenceMax)
	}
	if err != nil {
		return 0, false, fmt.Errorf("allocate purchase order confirmation sequence: %w", err)
	}
	return sequence, sequence >= POConfirmationWarnAt, nil
}

// allocatePurchaseOrderConfirmationTx assigns the number inside the caller's
// transaction. It locks the record first, returns without consuming anything
// when a number already exists (idempotence), refuses a draft, and writes the
// identifier and its provenance together.
//
// The caller owns the transaction: the allocation, the final save and the
// number's row update commit or roll back as one unit.
func allocatePurchaseOrderConfirmationTx(ctx context.Context, tx pgx.Tx, schema, orgID string,
	orderID, assignedBy int64, now time.Time) error {
	var state string
	var existing pgtype.Text
	var customerID pgtype.Int8
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT state, confirmation_number, customer_id
		FROM %s.purchase_orders
		WHERE id = $1 AND org_id = $2
		FOR UPDATE`, schema), orderID, orgID).Scan(&state, &existing, &customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPurchaseOrderNotFound
	}
	if err != nil {
		return fmt.Errorf("lock purchase order %d for confirmation: %w", orderID, err)
	}
	if existing.Valid {
		// Already issued. Returning the stored value unchanged is the
		// idempotent answer; nothing new is allocated.
		return nil
	}
	if state != PurchaseOrderStateSaved {
		return ErrPurchaseOrderNotSaved
	}
	if !customerID.Valid {
		return fmt.Errorf(
			"%w: a saved purchase order needs its customer for a confirmation number",
			ErrPurchaseOrderIncomplete)
	}

	var customerCode int16
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT customer_code FROM %s.customers WHERE id = $1`, schema),
		customerID.Int64).Scan(&customerCode); err != nil {
		return fmt.Errorf("read customer %d for confirmation: %w", customerID.Int64, err)
	}
	yy := poAssignmentYY(now)
	sequence, _, err := nextPOConfirmationSequenceTx(ctx, tx, schema, int(customerCode), yy)
	if err != nil {
		return err
	}
	number := apicontract.FormatPOConfirmationNumber(int(customerCode), yy, sequence)

	tag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET
			confirmation_number = $3,
			confirmation_customer_code = $4,
			confirmation_year = $5,
			confirmation_sequence = $6,
			confirmation_assigned_at = $7,
			confirmation_assigned_by = $8,
			updated_at = NOW()
		WHERE id = $1 AND org_id = $2 AND confirmation_number IS NULL`, schema),
		orderID, orgID, number, customerCode, yy, sequence, now, assignedBy)
	if err != nil {
		return fmt.Errorf("assign purchase order %d confirmation number: %w", orderID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("assign purchase order %d confirmation number: %w", orderID, ErrPurchaseOrderNotSaved)
	}
	return nil
}

// AssignPurchaseOrderConfirmationNumber issues the optional reference on an
// already-saved record. It is idempotent: an already-assigned record returns
// its existing number and consumes nothing. A draft is refused.
func (s *Store) AssignPurchaseOrderConfirmationNumber(ctx context.Context, id, assignedBy int64) (PurchaseOrder, error) {
	return s.assignPurchaseOrderConfirmation(ctx, id, assignedBy, time.Now())
}

// assignPurchaseOrderConfirmation is the clock-explicit form used by tests to
// prove the customer-year separation. Production callers go through
// AssignPurchaseOrderConfirmationNumber.
func (s *Store) assignPurchaseOrderConfirmation(ctx context.Context, id, assignedBy int64, now time.Time) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if id < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	if assignedBy < 1 {
		return PurchaseOrder{}, fmt.Errorf("%w: the assigning user is required", ErrPurchaseOrderInvalid)
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrder{}, err
	}
	schema := s.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("begin confirmation assignment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := allocatePurchaseOrderConfirmationTx(
		ctx, tx, schema, org.ID, id, assignedBy, now); err != nil {
		return PurchaseOrder{}, err
	}
	order, err := getPurchaseOrder(ctx, tx, schema, org.ID, id)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, fmt.Errorf("commit confirmation assignment: %w", err)
	}
	return order, nil
}
