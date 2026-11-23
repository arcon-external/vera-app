package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Issue #651: bought-state availability. Each quotation leaf reports the
// quantity that saved purchase-order allocations have consumed, the quantity
// sitting in drafts (advisory only), and what remains orderable. Supplier cost
// never appears here: this shape feeds the selector and the workspace, which
// are customer-facing (Story 42).
//
// Bought state is keyed by (quotation_id, quotation_item_no, quotation_sub_no)
// across every revision. A revision that adds an item starts at zero bought; a
// revision that lowers the quantity below the bought quantity clamps remaining
// to zero and reports the difference as over-bought; a removed item simply has
// no leaf to show, while its saved allocations stay in history.

// QuotationAvailabilityItem is one priced leaf of the revision the caller asked
// about. QuotedUnitPriceMinor is the row-discounted quotation baseline, never a
// supplier cost.
type QuotationAvailabilityItem struct {
	QuotationItemNo          int
	QuotationSubNo           *int
	Label                    string
	Description              string
	ParentDescription        string
	GroupKey                 string
	DescriptionLines         []string
	UoM                      string
	QuotedQuantityScaled     int64
	BoughtQuantityScaled     int64
	DraftQuantityScaled      int64
	RemainingQuantityScaled  int64
	OverBoughtQuantityScaled int64
	QuotedUnitPriceMinor     int64
}

// QuotationAvailability is one quotation's bought state for one revision.
// RevisionNumber is the revision whose leaves are listed (normally the latest);
// AllBought is derived from those leaves, so it is false when even one leaf has
// room. LegacyHeaderDiscount reports a stored whole-order quotation discount:
// the plan blocks new allocation-backed POs on such a revision until it is
// reissued with row-only pricing.
type QuotationAvailability struct {
	QuotationID          int64
	RevisionNumber       int16
	LatestRevision       int16
	AllBought            bool
	LegacyHeaderDiscount bool
	SavedPOCount         int
	DraftPOCount         int
	Items                []QuotationAvailabilityItem
}

// GetQuotationAvailability answers the selector and workspace for a quotation's
// latest issued revision.
func (s *Store) GetQuotationAvailability(ctx context.Context, quotationID int64) (QuotationAvailability, error) {
	return s.quotationAvailability(ctx, quotationID, -1)
}

// GetQuotationAvailabilityForRevision answers the same read for the exact
// revision the selector is showing. Bought quantities still span every revision;
// only the listed leaves and the quoted baseline come from the chosen revision.
func (s *Store) GetQuotationAvailabilityForRevision(
	ctx context.Context, quotationID int64, revision int16,
) (QuotationAvailability, error) {
	if revision < 0 {
		return QuotationAvailability{}, fmt.Errorf("%w: the revision must not be negative", ErrPOSourceInvalid)
	}
	return s.quotationAvailability(ctx, quotationID, revision)
}

func (s *Store) quotationAvailability(
	ctx context.Context, quotationID int64, revision int16,
) (QuotationAvailability, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return QuotationAvailability{}, err
	}
	if quotationID < 1 {
		return QuotationAvailability{}, fmt.Errorf("%w: the quotation id is required", ErrPOSourceInvalid)
	}
	pool, err := s.poolOrError()
	if err != nil {
		return QuotationAvailability{}, err
	}
	return getQuotationAvailability(ctx, pool, s.configSchema(), org.ID, quotationID, revision)
}

// getQuotationAvailability reads one revision's leaves and the cross-revision
// allocation sums beside them. A revision below zero selects the latest issued
// revision. The caller owns any transaction; the read is repeatable within it.
func getQuotationAvailability(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	quotationID int64,
	revision int16,
) (QuotationAvailability, error) {
	var latest pgtype.Int2
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT MAX(revision_number)
		FROM %s.quotation_revisions
		WHERE quotation_id = $1`, schema), quotationID).Scan(&latest); err != nil {
		return QuotationAvailability{}, fmt.Errorf(
			"read quotation %d latest revision: %w", quotationID, err)
	}
	if !latest.Valid {
		return QuotationAvailability{}, ErrPOSourceNotFound
	}
	latestRevision := latest.Int16
	if revision < 0 {
		revision = latestRevision
	}

	var revisionID int64
	var revisionNumber, discountPercent int16
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT r.id, r.revision_number, r.discount_percent
		FROM %s.quotation_revisions r
		WHERE r.quotation_id = $1 AND r.revision_number = $2`, schema),
		quotationID, revision).Scan(
		&revisionID, &revisionNumber, &discountPercent)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuotationAvailability{}, ErrPurchaseOrderSourceNotFound
	}
	if err != nil {
		return QuotationAvailability{}, fmt.Errorf(
			"read quotation %d availability revision: %w", quotationID, err)
	}

	lines, err := loadPurchaseOrderSourceLines(ctx, q, schema, revisionID)
	if err != nil {
		return QuotationAvailability{}, err
	}

	type leafKey struct {
		itemNo int
		subNo  int
	}
	type quantityPair struct {
		bought int64
		draft  int64
	}
	quantities := map[leafKey]quantityPair{}
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT a.quotation_item_no, COALESCE(a.quotation_sub_no, 0),
		       COALESCE(SUM(a.quantity_scaled) FILTER (WHERE po.state = 'saved'), 0),
		       COALESCE(SUM(a.quantity_scaled) FILTER (WHERE po.state = 'draft'), 0)
		FROM %s.purchase_order_allocations a
		JOIN %s.purchase_orders po ON po.id = a.purchase_order_id
		WHERE a.org_id = $1 AND a.quotation_id = $2
		GROUP BY a.quotation_item_no, COALESCE(a.quotation_sub_no, 0)`, schema, schema),
		orgID, quotationID)
	if err != nil {
		return QuotationAvailability{}, fmt.Errorf(
			"read quotation %d bought quantities: %w", quotationID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var key leafKey
		var pair quantityPair
		if err := rows.Scan(&key.itemNo, &key.subNo, &pair.bought, &pair.draft); err != nil {
			return QuotationAvailability{}, fmt.Errorf("scan quotation %d bought quantity: %w", quotationID, err)
		}
		quantities[key] = pair
	}
	if err := rows.Err(); err != nil {
		return QuotationAvailability{}, fmt.Errorf(
			"iterate quotation %d bought quantities: %w", quotationID, err)
	}

	availability := QuotationAvailability{
		QuotationID:          quotationID,
		RevisionNumber:       revisionNumber,
		LatestRevision:       latestRevision,
		LegacyHeaderDiscount: discountPercent > 0,
		Items:                make([]QuotationAvailabilityItem, 0, len(lines)),
	}
	availability.AllBought = len(lines) > 0
	for _, line := range lines {
		subNo := 0
		if line.QuotationSubNo != nil {
			subNo = *line.QuotationSubNo
		}
		pair := quantities[leafKey{itemNo: line.QuotationItemNo, subNo: subNo}]
		remaining := line.QuantityScaled - pair.bought
		overBought := int64(0)
		if remaining < 0 {
			overBought = -remaining
			remaining = 0
		}
		availability.Items = append(availability.Items, QuotationAvailabilityItem{
			QuotationItemNo:          line.QuotationItemNo,
			QuotationSubNo:           line.QuotationSubNo,
			Label:                    line.Label(),
			Description:              line.Description,
			ParentDescription:        line.ParentDescription,
			GroupKey:                 line.GroupKey,
			DescriptionLines:         append([]string(nil), line.DescriptionLines...),
			UoM:                      line.UoM,
			QuotedQuantityScaled:     line.QuantityScaled,
			BoughtQuantityScaled:     pair.bought,
			DraftQuantityScaled:      pair.draft,
			RemainingQuantityScaled:  remaining,
			OverBoughtQuantityScaled: overBought,
			QuotedUnitPriceMinor:     line.quotedUnitPriceMinor,
		})
		if remaining > 0 {
			availability.AllBought = false
		}
	}

	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FILTER (WHERE state = 'saved'),
		       COUNT(*) FILTER (WHERE state = 'draft')
		FROM %s.purchase_orders
		WHERE org_id = $1 AND quotation_id = $2`, schema),
		orgID, quotationID).Scan(
		&availability.SavedPOCount, &availability.DraftPOCount); err != nil {
		return QuotationAvailability{}, fmt.Errorf(
			"count quotation %d purchase orders: %w", quotationID, err)
	}
	return availability, nil
}

// availabilityLeaf finds one requested item/sub pair in the availability rows.
func availabilityLeaf(availability QuotationAvailability, itemNo int, subNo *int) (QuotationAvailabilityItem, bool) {
	wantSub := 0
	if subNo != nil {
		wantSub = *subNo
	}
	for _, item := range availability.Items {
		haveSub := 0
		if item.QuotationSubNo != nil {
			haveSub = *item.QuotationSubNo
		}
		if item.QuotationItemNo == itemNo && haveSub == wantSub {
			return item, true
		}
	}
	return QuotationAvailabilityItem{}, false
}
