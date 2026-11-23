package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrPOSourceNotFound reports a Quotation that does not exist (or was deleted).
// The selector turns it into a 404; validation reports it as an invalid pair
// rather than an error, because "gone" is an answer a confirm screen renders.
var ErrPOSourceNotFound = errors.New("purchase order source quotation not found")

// ErrPOSourceInvalid reports a selector query that cannot be expressed: today,
// a page whose offset overflows a signed 64-bit integer (issue #483). It is
// malformed input, so the handler answers 400 — not ErrPOSourceNotFound's 404.
var ErrPOSourceInvalid = errors.New("purchase order source query is invalid")

// POSourcePageSize is the selector's fixed page size (issue #467): fifteen
// Quotations per page, scrollable, so a large archive stays usable. It is part
// of the API contract, not a caller preference.
const POSourcePageSize = 15

// POSourceActivePO is the active Purchase Order occupying a Quotation: its id
// and whether it is still a draft or already saved (issue #468). The selector
// offers "Open draft" for a draft and keeps a saved record disabled.
type POSourceActivePO struct {
	ID    int64
	State string
}

// POSourceOccupancy is the archived Quotation detail's narrow question (issue
// #499): does this Quotation already have its one PO, and is that record a
// draft or finalized? It is about the Quotation, never a single revision --
// the one-PO-per-Quotation rule spans every issued revision. A nil ActivePO is
// a free Quotation. The answer is advisory: the create transaction re-checks
// occupancy, so a stale read can only change the label, never bypass the rule.
type POSourceOccupancy struct {
	ActivePO    *POSourceActivePO
	QuotationID int64
	// Bought state (#651): a quotation with remaining leaves may produce
	// another PO even when ActivePO already names a draft or saved record.
	AllBought            bool
	RemainingLeafCount   int
	SavedPOCount         int
	DraftPOCount         int
	LegacyHeaderDiscount bool
	// Closure and expiry (#656). Closed means Create PO must be refused and the
	// remainder is Closed Loss, not Unsold. ExpiredUnclosed is an advisory badge
	// only: expiry alone never becomes a loss.
	Closed          bool
	ClosedAt        *time.Time
	ClosedReason    string
	ClosedByName    string
	ValidUntil      *time.Time
	ExpiredUnclosed bool
}

// POSourceQuotation is one row of the selector's first step. It describes the
// latest issued revision, because that is the revision a PO starts against by
// default; the caller can still choose an older revision in the second step.
type POSourceQuotation struct {
	QuoteNumber         string
	Subject             string
	CustomerName        string
	ActivePO            *POSourceActivePO
	ID                  int64
	LatestRevision      int16
	IssuedRevisionCount int
	// Bought-state badges (#651). LeafCount and RemainingLeafCount describe the
	// latest revision; SavedPOCount/DraftPOCount are advisory. AllBought is the
	// selector's disable rule; LegacyHeaderDiscount warns that the revision
	// must be reissued before a new PO may follow it.
	LeafCount            int
	RemainingLeafCount   int
	SavedPOCount         int
	DraftPOCount         int
	AllBought            bool
	LegacyHeaderDiscount bool
	// Closure and expiry (#656), from the quotation row and its latest issued
	// revision. A closed row stays visible with a badge; it is never hidden by
	// hideUsed, which is about "nothing left to buy".
	Closed          bool
	ClosedAt        *time.Time
	ClosedReason    string
	ValidUntil      *time.Time
	ExpiredUnclosed bool
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
// Occupancy is answered by the real purchase_orders table since #468, scoped
// to the caller's organization. A draft and a saved record both occupy the
// Quotation, so hideUsed filters on the joined row in the same predicate the
// page uses.
//
// excludePOID is the repair mode #475 adds: the one record being repaired is
// not "another active PO", so its own Quotation stays selectable while every
// other occupied Quotation remains disabled. Zero is the ordinary entry path.
// A non-zero id that is not an active PO of this organization is refused,
// never silently treated as "nothing to exclude".
func (s *Store) ListPOSourceQuotations(ctx context.Context, page int, search string, hideUsed bool, excludePOID int64) ([]POSourceQuotation, int, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return nil, 0, err
	}
	if excludePOID < 0 {
		return nil, 0, fmt.Errorf("%w: the excluded purchase order id must not be negative", ErrPOSourceInvalid)
	}
	pool, err := s.poolOrError()
	if err != nil {
		return nil, 0, err
	}
	schema := s.configSchema()
	if excludePOID > 0 {
		var exists bool
		if err := pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT EXISTS (
				SELECT 1 FROM %s.purchase_orders WHERE id = $1 AND org_id = $2
			)`, schema), excludePOID, org.ID).Scan(&exists); err != nil {
			return nil, 0, fmt.Errorf("resolve the purchase order %d to exclude: %w", excludePOID, err)
		}
		if !exists {
			return nil, 0, ErrPurchaseOrderNotFound
		}
	}
	if page < 1 {
		page = 1
	}
	// PostgreSQL's OFFSET is a signed 64-bit integer and the offset is
	// (page-1)*POSourcePageSize. Validate before multiplying: the product
	// wraps for a page this function otherwise accepts, and the negative
	// offset that reaches Postgres is a database error, not a malformed query
	// (issue #483). This mirrors the PO archive's guard (#469), adapted to the
	// selector's own sentinel: the HTTP handler maps ErrPOSourceInvalid to 400,
	// and the desktop binding sends its page through that same endpoint.
	if int64(page-1) > math.MaxInt64/POSourcePageSize {
		return nil, 0, fmt.Errorf(
			"%w: page %d is past the largest representable offset", ErrPOSourceInvalid, page)
	}
	offset := int64(page-1) * POSourcePageSize

	// The latest revision supplies the row's subject and badges. Bought state is
	// derived from allocations across every revision, exactly as the
	// availability read does, so the selector's Open / Partial / All bought
	// label cannot disagree with the workspace.
	availability := fmt.Sprintf(`
		WITH latest AS (
			SELECT DISTINCT ON (quotation_id)
			       quotation_id, id AS revision_id, revision_number, discount_percent
			FROM %[1]s.quotation_revisions
			ORDER BY quotation_id, revision_number DESC
		),
		leaves AS (
			SELECT l.quotation_id, i.item_no, NULL::integer AS sub_no,
			       i.qty * %[2]d AS quoted_qty
			FROM latest l
			JOIN %[1]s.quotation_revision_items i ON i.revision_id = l.revision_id
			WHERE NOT i.show_sub_prices
			   OR NOT EXISTS (
			       SELECT 1 FROM %[1]s.quotation_revision_sub_items s
			       WHERE s.revision_item_id = i.id)
			UNION ALL
			SELECT l.quotation_id, i.item_no, s.sub_no, s.qty * %[2]d
			FROM latest l
			JOIN %[1]s.quotation_revision_items i ON i.revision_id = l.revision_id
			JOIN %[1]s.quotation_revision_sub_items s ON s.revision_item_id = i.id
			WHERE i.show_sub_prices
		),
		bought AS (
			SELECT a.quotation_id, a.quotation_item_no,
			       COALESCE(a.quotation_sub_no, 0) AS sub_no,
			       SUM(a.quantity_scaled) AS bought_qty
			FROM %[1]s.purchase_order_allocations a
			JOIN %[1]s.purchase_orders po ON po.id = a.purchase_order_id
			WHERE a.org_id = $2 AND po.state = 'saved'
			  -- Repair mode (#651): the record being repaired must not make its
			  -- own quotation look All bought (or hide it under hideUsed). Its
			  -- other POs still count.
			  AND ($3 = 0 OR po.id <> $3)
			GROUP BY 1, 2, 3
		),
		source_state AS (
			SELECT l.quotation_id,
			       COUNT(*) AS leaf_count,
			       COUNT(*) FILTER (
			           WHERE COALESCE(b.bought_qty, 0) < l.quoted_qty
			       ) AS remaining_leaf_count,
			       BOOL_AND(COALESCE(b.bought_qty, 0) >= l.quoted_qty) AS all_bought
			FROM leaves l
			LEFT JOIN bought b
			  ON b.quotation_id = l.quotation_id
			 AND b.quotation_item_no = l.item_no
			 AND b.sub_no = COALESCE(l.sub_no, 0)
			GROUP BY l.quotation_id
		)`, schema, QuantityScale)

	from := fmt.Sprintf(`
		FROM %[1]s.quotations q
		JOIN LATERAL (
			SELECT r.revision_number, r.subject, r.discount_percent,
			       r.created_at, r.validity_days
			FROM %[1]s.quotation_revisions r
			WHERE r.quotation_id = q.id
			ORDER BY r.revision_number DESC
			LIMIT 1
		) l ON TRUE
		JOIN %[1]s.customers c ON c.id = q.customer_id
		JOIN (
			SELECT quotation_id, COUNT(*) AS issued_revision_count
			FROM %[1]s.quotation_revisions
			GROUP BY quotation_id
		) counts ON counts.quotation_id = q.id
		LEFT JOIN source_state ss ON ss.quotation_id = q.id
		LEFT JOIN LATERAL (
			SELECT id, state
			FROM %[1]s.purchase_orders
			WHERE quotation_id = q.id AND org_id = $2
			ORDER BY (state = 'draft') DESC, id DESC
			LIMIT 1
		) active ON TRUE
		LEFT JOIN (
			SELECT quotation_id,
			       COUNT(*) FILTER (WHERE state = 'saved') AS saved_count,
			       COUNT(*) FILTER (WHERE state = 'draft') AS draft_count
			FROM %[1]s.purchase_orders
			WHERE org_id = $2 AND quotation_id IS NOT NULL
			GROUP BY quotation_id
		) po_counts ON po_counts.quotation_id = q.id`, schema)

	// hideUsed now means "hide a quotation that is fully bought"; a partial
	// quotation stays selectable. The repaired record's own id no longer
	// changes occupancy: a target may already carry other POs.
	where := " WHERE (NOT $1 OR NOT COALESCE(ss.all_bought, FALSE))"
	args := []any{hideUsed, org.ID, excludePOID}
	if term := strings.TrimSpace(search); term != "" {
		pattern := "%" + escapeLike(term) + "%"
		where += fmt.Sprintf(`
			AND (
				q.quote_number ILIKE $4 ESCAPE '\'
				OR EXISTS (
					SELECT 1 FROM %s.quotation_revisions sr
					WHERE sr.quotation_id = q.id AND sr.subject ILIKE $4 ESCAPE '\'
				)
				OR c.legal_name ILIKE $4 ESCAPE '\'
				OR EXISTS (
					SELECT 1 FROM %s.quotation_revision_items i
					JOIN %s.quotation_revisions sr ON sr.id = i.revision_id
					WHERE sr.quotation_id = q.id AND i.description ILIKE $4 ESCAPE '\'
				)
				OR EXISTS (
					SELECT 1 FROM %s.quotation_revision_sub_items si
					JOIN %s.quotation_revision_items i ON i.id = si.revision_item_id
					JOIN %s.quotation_revisions sr ON sr.id = i.revision_id
					WHERE sr.quotation_id = q.id AND si.description ILIKE $4 ESCAPE '\'
				)
			)`, schema, schema, schema, schema, schema, schema)
		args = append(args, pattern)
	}

	var total int
	if err := pool.QueryRow(ctx, availability+" SELECT COUNT(*) "+from+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count po source quotations: %w", err)
	}

	rows, err := pool.Query(ctx, availability+fmt.Sprintf(`
		SELECT q.id, q.quote_number, c.legal_name, l.subject,
		       l.revision_number, counts.issued_revision_count,
		       active.id, active.state,
		       COALESCE(ss.leaf_count, 0),
		       COALESCE(ss.remaining_leaf_count, 0),
		       COALESCE(po_counts.saved_count, 0),
		       COALESCE(po_counts.draft_count, 0),
		       COALESCE(ss.all_bought, FALSE),
		       (l.discount_percent > 0),
		       q.closed_at, q.closed_reason, l.created_at, l.validity_days
		%s%s
		ORDER BY q.id DESC
		LIMIT $%d OFFSET $%d`, from, where, len(args)+1, len(args)+2),
		append(args, POSourcePageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list po source quotations: %w", err)
	}
	defer rows.Close()

	quotations := make([]POSourceQuotation, 0, POSourcePageSize)
	now := time.Now()
	for rows.Next() {
		var quotation POSourceQuotation
		var activeID pgtype.Int8
		var activeState pgtype.Text
		var closedAt *time.Time
		var closedReason string
		var issuedAt time.Time
		var validityDays int16
		if err := rows.Scan(
			&quotation.ID,
			&quotation.QuoteNumber,
			&quotation.CustomerName,
			&quotation.Subject,
			&quotation.LatestRevision,
			&quotation.IssuedRevisionCount,
			&activeID,
			&activeState,
			&quotation.LeafCount,
			&quotation.RemainingLeafCount,
			&quotation.SavedPOCount,
			&quotation.DraftPOCount,
			&quotation.AllBought,
			&quotation.LegacyHeaderDiscount,
			&closedAt,
			&closedReason,
			&issuedAt,
			&validityDays,
		); err != nil {
			return nil, 0, fmt.Errorf("scan po source quotation: %w", err)
		}
		quotation.Closed = closedAt != nil
		quotation.ClosedAt = closedAt
		quotation.ClosedReason = closedReason
		validUntil, expired := quotationExpiryAt(issuedAt, validityDays, now)
		quotation.ValidUntil = &validUntil
		quotation.ExpiredUnclosed = expired && closedAt == nil
		if activeID.Valid {
			quotation.ActivePO = &POSourceActivePO{
				ID:    activeID.Int64,
				State: activeState.String,
			}
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

// GetPOSourceOccupancy answers the archived detail page's bought-state
// question for one Quotation (#499, #651): how much is still orderable, how
// many POs exist, and the newest record the page may offer to reopen. It is a
// read; creation and finalization re-check the remaining quantity under the
// quotation lock, so a stale answer can only change the label.
func (s *Store) GetPOSourceOccupancy(ctx context.Context, quotationID int64) (POSourceOccupancy, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return POSourceOccupancy{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return POSourceOccupancy{}, err
	}
	schema := s.configSchema()
	availability, err := getQuotationAvailability(ctx, pool, schema, org.ID, quotationID, -1)
	if err != nil {
		if errors.Is(err, ErrPurchaseOrderSourceNotFound) {
			return POSourceOccupancy{}, ErrPOSourceNotFound
		}
		return POSourceOccupancy{}, err
	}
	occupancy := POSourceOccupancy{
		QuotationID:          quotationID,
		AllBought:            availability.AllBought,
		RemainingLeafCount:   availabilityRemainingLeaves(availability),
		SavedPOCount:         availability.SavedPOCount,
		DraftPOCount:         availability.DraftPOCount,
		LegacyHeaderDiscount: availability.LegacyHeaderDiscount,
		Closed:               availability.Closed,
		ClosedAt:             availability.ClosedAt,
		ClosedReason:         availability.ClosedReason,
		ClosedByName:         availability.ClosedByName,
		ValidUntil:           availability.ValidUntil,
		ExpiredUnclosed:      availability.ExpiredUnclosed,
	}
	var activeID int64
	var activeState string
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, state FROM %s.purchase_orders
		WHERE org_id = $1 AND quotation_id = $2
		ORDER BY (state = 'draft') DESC, id DESC
		LIMIT 1`, schema), org.ID, quotationID).Scan(&activeID, &activeState)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return POSourceOccupancy{}, fmt.Errorf(
			"read po source occupancy for quotation %d: %w", quotationID, err)
	}
	if err == nil {
		occupancy.ActivePO = &POSourceActivePO{ID: activeID, State: activeState}
	}
	return occupancy, nil
}

func availabilityRemainingLeaves(availability QuotationAvailability) int {
	remaining := 0
	for _, item := range availability.Items {
		if item.RemainingQuantityScaled > 0 {
			remaining++
		}
	}
	return remaining
}
