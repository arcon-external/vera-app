package store

import (
	"context"
	"fmt"
	"time"
)

const (
	// SequenceWarnAt surfaces a warn flag once a customer-year cycle reaches
	// this sequence, giving months of runway before overflow digits appear.
	SequenceWarnAt = 8000
	// SequenceMax is the defense-only hard cap per customer-year cycle.
	// Reaching it is implausible (9999/year already needs ~27 quotes a day
	// to one customer, every day); past 9999 the number widens to 5 digits
	// instead of wrapping.
	SequenceMax = 99999
	// sequenceTenant reserves the tenancy slot in counter keys from day one,
	// so the SaaS retrofit never needs a second sequence migration.
	sequenceTenant = "default"
)

// SequenceKey builds the counter key for one customer-year cycle, e.g.
// quotation:default:01:26. Each January is a fresh key starting at 1, so
// there is no wrap path to test or race.
func SequenceKey(customerCode, yy int) string {
	return fmt.Sprintf("quotation:%s:%02d:%02d", sequenceTenant, customerCode, yy)
}

// CurrentYY reports the two-digit issue year used for numbering. Numbers
// are always issued in the creation year; there is no backdating into
// closed years.
func CurrentYY() int {
	return time.Now().Year() % 100
}

func checkSequenceScope(customerCode, yy int) error {
	if customerCode < 1 || customerCode > 99 {
		return fmt.Errorf("invalid customer code %d: want 1-99", customerCode)
	}
	if yy < 0 || yy > 99 {
		return fmt.Errorf("invalid year %d: want 0-99", yy)
	}
	return nil
}

// PeekQuoteSequence reports the next allocatable sequence for a
// customer-year cycle without consuming it, so UIs can preview the
// forthcoming quote number (e.g. in a save dialog). Allocation remains
// authoritative: concurrent exporters may observe the same preview, and the
// export response carries the final number.
func (s *Store) PeekQuoteSequence(ctx context.Context, customerCode, yy int) (seq int64, warn bool, err error) {
	if err := checkSequenceScope(customerCode, yy); err != nil {
		return 0, false, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return 0, false, err
	}
	var next int64 = 1
	err = pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COALESCE((SELECT next_value FROM %s.sequence_counters WHERE name = $1), 1)",
		s.configSchema()), SequenceKey(customerCode, yy)).Scan(&next)
	if err != nil {
		return 0, false, fmt.Errorf("peek quote sequence: %w", err)
	}
	if next < 1 || next > SequenceMax {
		return 0, true, fmt.Errorf("quote sequence exhausted")
	}
	return next, next >= SequenceWarnAt, nil
}

// NextQuoteSequence allocates a sequence for a customer-year cycle
// atomically in Postgres. The first allocated value per cycle is 1.
func (s *Store) NextQuoteSequence(ctx context.Context, customerCode, yy int) (seq int64, warn bool, err error) {
	if err := checkSequenceScope(customerCode, yy); err != nil {
		return 0, false, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return 0, false, err
	}

	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin quote sequence transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sequence, err := nextQuoteSequenceTx(ctx, tx, schema, customerCode, yy)
	if err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit quote sequence: %w", err)
	}
	return sequence, sequence >= SequenceWarnAt, nil
}
