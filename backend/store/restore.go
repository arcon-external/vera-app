package store

import (
	"context"
	"fmt"
)

// DSN returns the connection string this store was opened with. It contains
// the database password: callers must pass it to subprocesses only, and
// never log or return it.
func (s *Store) DSN() string {
	return s.config.DSN
}

// ResetSchema drops the application schema. It is used only by the admin
// restore flow, after a safety snapshot has been taken: pg_restore
// recreates the schema from the dump. All sessions die with the schema by
// design.
func (s *Store) ResetSchema(ctx context.Context) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"DROP SCHEMA %s CASCADE;", s.configSchema())); err != nil {
		return fmt.Errorf("reset schema: %w", err)
	}
	return nil
}

// RestoreRowCounts reports post-restore row counts for verification.
func (s *Store) RestoreRowCounts(ctx context.Context) (quotations, revisions int64, err error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, 0, err
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.quotations", s.configSchema())).Scan(&quotations); err != nil {
		return 0, 0, fmt.Errorf("count quotations: %w", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.quotation_revisions", s.configSchema())).Scan(&revisions); err != nil {
		return 0, 0, fmt.Errorf("count revisions: %w", err)
	}
	return quotations, revisions, nil
}

// AuditCount returns how many audit entries exist for an action. It exists
// so tests and operators can verify guardrail events were recorded.
func (s *Store) AuditCount(ctx context.Context, action string) (int, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, err
	}
	var count int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.auth_audit WHERE action = $1", s.configSchema()),
		action).Scan(&count); err != nil {
		return 0, fmt.Errorf("count audit entries: %w", err)
	}
	return count, nil
}
