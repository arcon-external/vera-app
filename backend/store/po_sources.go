package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrPOSourceNotFound reports a Quotation that does not exist (or was deleted).
// The selector turns it into a 404; validation reports it as an invalid pair
// rather than an error, because "gone" is an answer a confirm screen renders.
var ErrPOSourceNotFound = errors.New("purchase order source quotation not found")

// POSourcePageSize is the selector's fixed page size (issue #467): fifteen
// Quotations per page, scrollable, so a large archive stays usable. It is part
// of the API contract, not a caller preference.
const POSourcePageSize = 15

// POSourceQuotation is one row of the selector's first step. It describes the
// latest issued revision, because that is the revision a PO starts against by
// default; the caller can still choose an older revision in the second step.
type POSourceQuotation struct {
	QuoteNumber         string
	Subject             string
	CustomerName        string
	ID                  int64
	LatestRevision      int16
	IssuedRevisionCount int
	HasActivePO         bool
}

// POSourceRevision is one issued revision of a selected Quotation.
type POSourceRevision struct {
	IssuedAt time.Time
	Revision int16
}

// POSourceQuotationDetail is the second step's data.
type POSourceQuotationDetail struct {
	QuoteNumber  string
	Subject      string
	CustomerName string
	ID           int64
	Revisions    []POSourceRevision
}

// POSourceValidation is the confirm-time answer. Valid is false with a reason
// when the pair no longer exists; an infrastructure failure is returned as an
// error instead, so a broken database is never mistaken for a deleted source.
type POSourceValidation struct {
	Quotation      POSourceQuotation
	Reason         string
	RevisionNumber int16
	Valid          bool
}

// Reasons the selector reports on a failed revalidation. They are stable
// strings because both PO entry paths render them.
const (
	POSourceReasonQuotationNotFound = "quotation_not_found"
	POSourceReasonRevisionNotFound  = "revision_not_found"
)

// ListPOSourceQuotations lists issued Quotations for the PO source selector,
// newest first, one row per Quotation carrying its latest issued revision and
// its issued revision count. Unsaved drafts are absent by construction: they
// live in quotation_drafts and never become a quotation until export.
//
// Search is case-insensitive and covers the Quotation number, the customer
// name, and the subject plus item and sub-item descriptions of every issued
// revision. Every revision is selectable in step two, so every revision's text
// must be findable: an item that only revision 0 ever carried still locates
// its Quotation. The row itself keeps showing the latest issued revision. The
// total is the unpaged match count, computed from the same predicate as the
// page.
//
// hasActivePO is a placeholder for #468: no PO table exists yet, so every row
// is eligible. hideUsed therefore filters nothing today, but it is honoured on
// the same predicate the real query will fill in, so the caller's contract
// does not change when persistence arrives.
func (s *Store) ListPOSourceQuotations(ctx context.Context, page int, search string, hideUsed bool) ([]POSourceQuotation, int, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	schema := s.configSchema()

	// The latest revision supplies the row's subject and revision number. The
	// search is not tied to it: the EXISTS clauses below reach every revision
	// of the Quotation, matching step two's full revision list.
	from := fmt.Sprintf(`
		FROM %s.quotations q
		JOIN LATERAL (
			SELECT r.id, r.revision_number, r.subject, r.created_at
			FROM %s.quotation_revisions r
			WHERE r.quotation_id = q.id
			ORDER BY r.revision_number DESC
			LIMIT 1
		) l ON TRUE
		JOIN %s.customers c ON c.id = q.customer_id
		JOIN (
			SELECT quotation_id, COUNT(*) AS issued_revision_count
			FROM %s.quotation_revisions
			GROUP BY quotation_id
		) counts ON counts.quotation_id = q.id`, schema, schema, schema, schema)

	where := " WHERE (NOT $1 OR NOT FALSE)" // hideUsed; FALSE stands in for hasActivePO until #468.
	args := []any{hideUsed}
	if term := strings.TrimSpace(search); term != "" {
		pattern := "%" + escapeLike(term) + "%"
		where += fmt.Sprintf(`
			AND (
				q.quote_number ILIKE $2 ESCAPE '\'
				OR EXISTS (
					SELECT 1 FROM %s.quotation_revisions sr
					WHERE sr.quotation_id = q.id AND sr.subject ILIKE $2 ESCAPE '\'
				)
				OR c.legal_name ILIKE $2 ESCAPE '\'
				OR EXISTS (
					SELECT 1 FROM %s.quotation_revision_items i
					JOIN %s.quotation_revisions sr ON sr.id = i.revision_id
					WHERE sr.quotation_id = q.id AND i.description ILIKE $2 ESCAPE '\'
				)
				OR EXISTS (
					SELECT 1 FROM %s.quotation_revision_sub_items si
					JOIN %s.quotation_revision_items i ON i.id = si.revision_item_id
					JOIN %s.quotation_revisions sr ON sr.id = i.revision_id
					WHERE sr.quotation_id = q.id AND si.description ILIKE $2 ESCAPE '\'
				)
			)`, schema, schema, schema, schema, schema, schema)
		args = append(args, pattern)
	}

	var total int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) "+from+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count po source quotations: %w", err)
	}

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT q.id, q.quote_number, c.legal_name, l.subject,
		       l.revision_number, counts.issued_revision_count, FALSE
		%s%s
		ORDER BY q.id DESC
		LIMIT $%d OFFSET $%d`, from, where, len(args)+1, len(args)+2),
		append(args, POSourcePageSize, (page-1)*POSourcePageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list po source quotations: %w", err)
	}
	defer rows.Close()

	quotations := make([]POSourceQuotation, 0, POSourcePageSize)
	for rows.Next() {
		var quotation POSourceQuotation
		if err := rows.Scan(
			&quotation.ID,
			&quotation.QuoteNumber,
			&quotation.CustomerName,
			&quotation.Subject,
			&quotation.LatestRevision,
			&quotation.IssuedRevisionCount,
			&quotation.HasActivePO,
		); err != nil {
			return nil, 0, fmt.Errorf("scan po source quotation: %w", err)
		}
		quotations = append(quotations, quotation)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate po source quotations: %w", err)
	}
	return quotations, total, nil
}

// escapeLike neutralises the LIKE wildcards in a user's search term, so a
// subject containing "%" is searched literally rather than matching everything.
func escapeLike(term string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(term)
}

// ListPOSourceRevisions returns the issued revisions of one Quotation, newest
// first. Every stored revision is issued: drafts never reach this table.
func (s *Store) ListPOSourceRevisions(ctx context.Context, quotationID int64) (POSourceQuotationDetail, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return POSourceQuotationDetail{}, err
	}
	schema := s.configSchema()

	var detail POSourceQuotationDetail
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT q.id, q.quote_number, c.legal_name, l.subject
		FROM %s.quotations q
		JOIN LATERAL (
			SELECT subject FROM %s.quotation_revisions r
			WHERE r.quotation_id = q.id
			ORDER BY r.revision_number DESC
			LIMIT 1
		) l ON TRUE
		JOIN %s.customers c ON c.id = q.customer_id
		WHERE q.id = $1`, schema, schema, schema), quotationID).Scan(
		&detail.ID, &detail.QuoteNumber, &detail.CustomerName, &detail.Subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return POSourceQuotationDetail{}, ErrPOSourceNotFound
	}
	if err != nil {
		return POSourceQuotationDetail{}, fmt.Errorf("load po source quotation %d: %w", quotationID, err)
	}

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT revision_number, created_at
		FROM %s.quotation_revisions
		WHERE quotation_id = $1
		ORDER BY revision_number DESC`, schema), quotationID)
	if err != nil {
		return POSourceQuotationDetail{}, fmt.Errorf("list po source revisions: %w", err)
	}
	defer rows.Close()
	detail.Revisions = make([]POSourceRevision, 0, 4)
	for rows.Next() {
		var revision POSourceRevision
		if err := rows.Scan(&revision.Revision, &revision.IssuedAt); err != nil {
			return POSourceQuotationDetail{}, fmt.Errorf("scan po source revision: %w", err)
		}
		detail.Revisions = append(detail.Revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return POSourceQuotationDetail{}, fmt.Errorf("iterate po source revisions: %w", err)
	}
	return detail, nil
}

// ValidatePOSource answers whether an exact Quotation + revision pair still
// exists and is issued. It is the selector's confirm gate: the pair is checked
// again after the user has spent time choosing, so a deletion or a stale list
// cannot create a link to a source that is gone.
func (s *Store) ValidatePOSource(ctx context.Context, quotationID int64, revision int16) (POSourceValidation, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return POSourceValidation{}, err
	}
	schema := s.configSchema()

	var validation POSourceValidation
	validation.RevisionNumber = revision
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT q.id, q.quote_number, c.legal_name, l.subject,
		       l.revision_number, counts.issued_revision_count
		FROM %s.quotations q
		JOIN LATERAL (
			SELECT revision_number, subject FROM %s.quotation_revisions r
			WHERE r.quotation_id = q.id
			ORDER BY r.revision_number DESC
			LIMIT 1
		) l ON TRUE
		JOIN %s.customers c ON c.id = q.customer_id
		JOIN (
			SELECT quotation_id, COUNT(*) AS issued_revision_count
			FROM %s.quotation_revisions
			GROUP BY quotation_id
		) counts ON counts.quotation_id = q.id
		WHERE q.id = $1`, schema, schema, schema, schema), quotationID).Scan(
		&validation.Quotation.ID,
		&validation.Quotation.QuoteNumber,
		&validation.Quotation.CustomerName,
		&validation.Quotation.Subject,
		&validation.Quotation.LatestRevision,
		&validation.Quotation.IssuedRevisionCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		validation.Reason = POSourceReasonQuotationNotFound
		return validation, nil
	}
	if err != nil {
		return POSourceValidation{}, fmt.Errorf("validate po source quotation %d: %w", quotationID, err)
	}

	var issued bool
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT TRUE FROM %s.quotation_revisions
		WHERE quotation_id = $1 AND revision_number = $2`, schema),
		quotationID, revision).Scan(&issued); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			validation.Reason = POSourceReasonRevisionNotFound
			return validation, nil
		}
		return POSourceValidation{}, fmt.Errorf("validate po source revision: %w", err)
	}
	validation.Valid = issued
	return validation, nil
}
