package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"vera-app/backend/labels"
)

// GetCompanyLabels returns the document's effective wording: the built-in
// defaults with the company's overrides applied.
//
// It resolves here rather than at the callers because every reader wants the
// same thing -- the quotation's header, the server's render, the settings
// editor -- and a resolution done per caller is a resolution done differently
// per caller. The overrides themselves are read as a map and handed to
// labels.ResolveOverrides, which treats an unknown key or a blank value as
// "print the default": a row for a label a later release retired must not be
// able to blank anything on the document.
//
// A store with no database configured fails like every other store method: the
// caller -- resolveCompanyIdentity -- already has an offline fallback for the
// whole identity, and a repository that silently answered with defaults would
// make a misconfigured server print a document nobody chose.
func (s *Store) GetCompanyLabels(ctx context.Context) (labels.DocumentLabels, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return labels.DocumentLabels{}, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(
		`SELECT key, value FROM %s.company_labels`, s.configSchema()))
	if err != nil {
		return labels.DocumentLabels{}, fmt.Errorf("get company labels: %w", err)
	}
	defer rows.Close()
	overrides := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return labels.DocumentLabels{}, fmt.Errorf("scan company label: %w", err)
		}
		overrides[key] = value
	}
	if err := rows.Err(); err != nil {
		return labels.DocumentLabels{}, fmt.Errorf("get company labels: %w", err)
	}
	return labels.ResolveOverrides(overrides), nil
}

// UpdateCompanyLabels full-replaces the override table with the given rows.
//
// Like the unit list it replaces (units.go), and unlike the profile, because
// the labels are one vocabulary rather than one row: a partial update would
// have no meaning for a set where every key has a default. A row with an empty
// value clears that label back to the built-in default, so the caller never
// needs a delete request per label.
//
// The replacement runs in one transaction so the table is never observable
// half-replaced. The store validates before touching the database -- the key
// must name a real label, no key twice, and every value plain text -- so a
// refused save names the label rather than surfacing a constraint violation.
func (s *Store) UpdateCompanyLabels(ctx context.Context, rows []labels.CompanyLabel) (labels.DocumentLabels, error) {
	if err := labels.CheckOverrides(rows); err != nil {
		return labels.DocumentLabels{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return labels.DocumentLabels{}, err
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return labels.DocumentLabels{}, fmt.Errorf("begin company labels update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.company_labels`, s.configSchema())); err != nil {
		return labels.DocumentLabels{}, fmt.Errorf("replace company labels: %w", err)
	}
	for _, row := range rows {
		value := strings.TrimSpace(row.Value)
		// An empty value is a clear, not a row: storing it would make the
		// override table hold a row that resolves to the default anyway, and
		// the wire contract is that empty means "print the built-in".
		if value == "" {
			continue
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s.company_labels (key, value) VALUES ($1, $2)`,
			s.configSchema()), strings.TrimSpace(row.Key), value); err != nil {
			return labels.DocumentLabels{}, fmt.Errorf("replace company labels: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return labels.DocumentLabels{}, fmt.Errorf("replace company labels: %w", err)
	}
	return s.GetCompanyLabels(ctx)
}
