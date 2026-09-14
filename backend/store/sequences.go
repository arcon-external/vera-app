package store

import (
	"context"
	"fmt"
)

// PeekQuoteSequence reports the next allocatable quote sequence without
// consuming it, so UIs can preview the forthcoming quote number (e.g. in a
// save dialog). Allocation remains authoritative: concurrent exporters may
// observe the same preview, and the export response carries the final number.
func (s *Store) PeekQuoteSequence(ctx context.Context) (uint16, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, err
	}
	var next int64 = 1
	err = pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COALESCE((SELECT next_value FROM %s.sequence_counters WHERE name = 'quotation'), 1)",
		s.configSchema())).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("peek quote sequence: %w", err)
	}
	if next < 1 || next > 9999 {
		return 0, fmt.Errorf("quote sequence exhausted")
	}
	return uint16(next), nil
}

// NextQuoteSequence allocates a quote sequence atomically in Postgres. The
// first allocated value is 1 and the existing four-digit quote format limit is
// enforced without consuming a value beyond 9999.
func (s *Store) NextQuoteSequence(ctx context.Context) (uint16, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, err
	}

	schema := s.configSchema()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin quote sequence transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.sequence_counters (name, next_value)
		VALUES ('quotation', 1)
		ON CONFLICT (name) DO NOTHING`, schema)); err != nil {
		return 0, fmt.Errorf("initialize quote sequence: %w", err)
	}

	var sequence int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.sequence_counters
		SET next_value = next_value + 1
		WHERE name = 'quotation' AND next_value <= 9999
		RETURNING next_value - 1`, schema)).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("allocate quote sequence: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit quote sequence: %w", err)
	}
	return uint16(sequence), nil
}
