package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Issue #469: the Purchase Order archive. A PO is found by its customer PO
// number, its source Quotation number, or its customer snapshot; the list is
// paged 15 rows at a time by the server and never leaves the caller's
// organization.

// PurchaseOrderListPageSize is the archive's fixed page size. It is part of the
// API contract, not a caller preference.
const PurchaseOrderListPageSize = 15

// The source-missing filter values. "any" is the default; "only" and "exclude"
// make the source-missing state an independent predicate. Until #480/#481 ship
// deletion and repair, purchase_orders.quotation_id is NOT NULL with a live
// foreign key, so no row can match "only".
const (
	PurchaseOrderSourceMissingAny     = "any"
	PurchaseOrderSourceMissingOnly    = "only"
	PurchaseOrderSourceMissingExclude = "exclude"
)

// PurchaseOrderListQuery is one archive page request.
type PurchaseOrderListQuery struct {
	Search        string
	State         string
	CustomerIDs   []int64
	ReceivedFrom  *time.Time
	ReceivedTo    *time.Time
	SourceMissing string
	Page          int
}

// PurchaseOrderListRow is one archive row. The customer name is the PO's own
// snapshot, not the live client directory, so a rename never rewrites history.
type PurchaseOrderListRow struct {
	UUID           string
	State          string
	PONumber       string
	QuoteNumber    string
	CustomerName   string
	ReceivedDate   time.Time
	Currency       string
	ID             int64
	CustomerID     int64
	SourceRevision int16
	SourceMissing  bool
}

// PurchaseOrderListCustomer is one option of the customer multi-select. The
// facet is org-wide: filtering by another field must not hide a customer from
// the picker.
type PurchaseOrderListCustomer struct {
	Name string
	ID   int64
}

// PurchaseOrderListPage is one page plus the unpaged match count and the
// org-wide customer facet.
type PurchaseOrderListPage struct {
	Rows       []PurchaseOrderListRow
	Customers  []PurchaseOrderListCustomer
	Page       int
	PageSize   int
	Total      int
	TotalPages int
}

// ListPurchaseOrders returns one page of the archive, newest received first.
//
// Search is case-insensitive over the customer PO number, the source Quotation
// number and the customer snapshot; LIKE wildcards in the term are escaped, so
// "50%" is a literal and not "everything". The customer filter takes one or
// more ids, the received-date range is inclusive, and Draft/Saved is the stored
// entry state -- never an invented fulfilment or payment status.
//
// SourceMissing is derived from the Quotation join, not from a column: when
// deletion ships, the retained PO must keep reading without its source. Today
// the foreign key makes a missing source impossible, so "only" answers zero
// rows and the predicate stays inert rather than pretending the feature exists.
func (s *Store) ListPurchaseOrders(ctx context.Context, query PurchaseOrderListQuery) (PurchaseOrderListPage, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderListPage{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderListPage{}, err
	}

	page := query.Page
	if page < 1 {
		page = 1
	}
	state := strings.TrimSpace(query.State)
	switch state {
	case "", PurchaseOrderStateDraft, PurchaseOrderStateSaved:
	default:
		return PurchaseOrderListPage{}, fmt.Errorf("%w: unknown state filter %q", ErrPurchaseOrderInvalid, query.State)
	}
	sourceMissing := strings.TrimSpace(query.SourceMissing)
	if sourceMissing == "" {
		sourceMissing = PurchaseOrderSourceMissingAny
	}
	switch sourceMissing {
	case PurchaseOrderSourceMissingAny, PurchaseOrderSourceMissingOnly, PurchaseOrderSourceMissingExclude:
	default:
		return PurchaseOrderListPage{}, fmt.Errorf("%w: unknown source-missing filter %q",
			ErrPurchaseOrderInvalid, query.SourceMissing)
	}
	from := normalizeDate(query.ReceivedFrom)
	to := normalizeDate(query.ReceivedTo)
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return PurchaseOrderListPage{}, fmt.Errorf("%w: received-from must not be after received-to", ErrPurchaseOrderInvalid)
	}

	schema := s.configSchema()
	fromClause := fmt.Sprintf(`
		FROM %s.purchase_orders po
		LEFT JOIN %s.quotations q ON q.id = po.quotation_id`, schema, schema)

	where := " WHERE po.org_id = $1"
	args := []any{org.ID}
	if term := strings.TrimSpace(query.Search); term != "" {
		pattern := "%" + escapeLike(term) + "%"
		args = append(args, pattern)
		placeholder := fmt.Sprintf("$%d", len(args))
		where += fmt.Sprintf(`
			AND (
				COALESCE(po.po_number, '') ILIKE %s ESCAPE '\'
				OR COALESCE(q.quote_number, '') ILIKE %s ESCAPE '\'
				OR po.customer_name ILIKE %s ESCAPE '\'
			)`, placeholder, placeholder, placeholder)
	}
	if len(query.CustomerIDs) > 0 {
		args = append(args, query.CustomerIDs)
		where += fmt.Sprintf(" AND po.customer_id = ANY($%d)", len(args))
	}
	if state != "" {
		args = append(args, state)
		where += fmt.Sprintf(" AND po.state = $%d", len(args))
	}
	if !from.IsZero() {
		args = append(args, from.Format("2006-01-02"))
		where += fmt.Sprintf(" AND po.received_date >= $%d::date", len(args))
	}
	if !to.IsZero() {
		args = append(args, to.Format("2006-01-02"))
		where += fmt.Sprintf(" AND po.received_date <= $%d::date", len(args))
	}
	switch sourceMissing {
	case PurchaseOrderSourceMissingOnly:
		where += " AND q.id IS NULL"
	case PurchaseOrderSourceMissingExclude:
		where += " AND q.id IS NOT NULL"
	}

	var total int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) "+fromClause+where, args...).Scan(&total); err != nil {
		return PurchaseOrderListPage{}, fmt.Errorf("count purchase orders: %w", err)
	}

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT po.id, po.uuid::text, po.state, COALESCE(po.po_number, ''),
		       COALESCE(q.quote_number, ''), po.source_revision, po.customer_id,
		       po.customer_name, po.received_date, po.currency, (q.id IS NULL)
		%s%s
		ORDER BY po.received_date DESC, po.id DESC
		LIMIT $%d OFFSET $%d`, fromClause, where, len(args)+1, len(args)+2),
		append(args, PurchaseOrderListPageSize, (page-1)*PurchaseOrderListPageSize)...)
	if err != nil {
		return PurchaseOrderListPage{}, fmt.Errorf("list purchase orders: %w", err)
	}
	defer rows.Close()

	result := make([]PurchaseOrderListRow, 0, PurchaseOrderListPageSize)
	for rows.Next() {
		var row PurchaseOrderListRow
		if err := rows.Scan(
			&row.ID,
			&row.UUID,
			&row.State,
			&row.PONumber,
			&row.QuoteNumber,
			&row.SourceRevision,
			&row.CustomerID,
			&row.CustomerName,
			&row.ReceivedDate,
			&row.Currency,
			&row.SourceMissing,
		); err != nil {
			return PurchaseOrderListPage{}, fmt.Errorf("scan purchase order row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return PurchaseOrderListPage{}, fmt.Errorf("iterate purchase orders: %w", err)
	}

	customers, err := listPurchaseOrderCustomers(ctx, pool, schema, org.ID)
	if err != nil {
		return PurchaseOrderListPage{}, err
	}

	totalPages := (total + PurchaseOrderListPageSize - 1) / PurchaseOrderListPageSize
	if totalPages < 1 {
		// One empty page, not "page 1 of 0": the archive still shows its
		// empty state with working pagination controls.
		totalPages = 1
	}
	return PurchaseOrderListPage{
		Rows:       result,
		Customers:  customers,
		Page:       page,
		PageSize:   PurchaseOrderListPageSize,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

// listPurchaseOrderCustomers returns the customers that actually own POs in
// this organization, newest snapshot name per customer, alphabetical. It runs
// without the page's filters on purpose: choosing customer B must stay possible
// while the current result happens to contain only customer A.
func listPurchaseOrderCustomers(ctx context.Context, q purchaseOrderQuerier, schema, orgID string) ([]PurchaseOrderListCustomer, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT customer_id, customer_name FROM (
			SELECT DISTINCT ON (customer_id) customer_id, customer_name
			FROM %s.purchase_orders
			WHERE org_id = $1
			ORDER BY customer_id, received_date DESC, id DESC
		) latest
		ORDER BY customer_name, customer_id`, schema), orgID)
	if err != nil {
		return nil, fmt.Errorf("list purchase order customers: %w", err)
	}
	defer rows.Close()

	customers := make([]PurchaseOrderListCustomer, 0, 8)
	for rows.Next() {
		var customer PurchaseOrderListCustomer
		if err := rows.Scan(&customer.ID, &customer.Name); err != nil {
			return nil, fmt.Errorf("scan purchase order customer: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order customers: %w", err)
	}
	return customers, nil
}
