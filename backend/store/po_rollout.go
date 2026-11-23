package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// Issue #658: the strict cutover has three server gates, all behind one switch
// (Config.StrictPOAllocations). Compatibility mode runs none of them, so the
// code path an older desktop uses is unchanged.
//
// The gates are deliberately "new or changed", not "any occurrence":
//   - a source-less blank draft is creation-only, so the whole entry is gone;
//   - a quotation-kind line must name a leaf unless it is the already-stored
//     nil-reference line a pre-cutover record carries. That keeps a historical
//     purchase order correctable, which is the anti-data-loss requirement;
//   - a row discount is refused unless it is exactly the stored historical
//     pair; a header discount or adjustment is refused outright because the
//     write path already stores both as zero (#573).

// strictStoredLine is the pre-write picture of one stored line the gates
// compare against. Only the fields a gate reads are loaded.
type strictStoredLine struct {
	QuotationItemNo *int
	QuotationSubNo  *int
	DiscountEnabled bool
	DiscountPercent int16
}

// enforceStrictPurchaseOrderSave applies the cutover rules to one save that
// replaces the record's lines. Callers run it inside the purchase order's
// locked transaction, before any snapshot or row write, so a refusal leaves the
// stored record untouched.
func (s *Store) enforceStrictPurchaseOrderSave(
	ctx context.Context,
	q purchaseOrderQuerier,
	orgID string,
	orderID int64,
	input PurchaseOrderUpdate,
) error {
	if !s.config.StrictPOAllocations {
		return nil
	}
	if input.DiscountPercent != 0 || input.AdjustmentMinor != 0 {
		return ErrPurchaseOrderDiscountForbidden
	}
	stored, err := loadStrictStoredLines(ctx, q, s.configSchema(), orgID, orderID)
	if err != nil {
		return err
	}
	for index, item := range input.Items {
		kind, err := normalizePurchaseOrderLineKind(item.LineKind)
		if err != nil {
			return fmt.Errorf("%w: line %d", err, index+1)
		}
		if kind == PurchaseOrderLineKindQuotation &&
			item.QuotationItemNo == nil && item.QuotationSubNo == nil {
			line, found := stored[item.ID]
			if item.ID < 1 || !found ||
				line.QuotationItemNo != nil || line.QuotationSubNo != nil {
				return fmt.Errorf("%w: line %d", ErrPurchaseOrderFreeLineRetired, index+1)
			}
		}
		// A disabled row has no monetary discount whatever percent it carries,
		// so only an effective discount is the retired shape.
		if !item.DiscountEnabled || item.DiscountPercent == 0 {
			continue
		}
		line, found := stored[item.ID]
		if item.ID < 1 || !found ||
			line.DiscountEnabled != item.DiscountEnabled ||
			line.DiscountPercent != item.DiscountPercent {
			return fmt.Errorf("%w: line %d", ErrPurchaseOrderDiscountForbidden, index+1)
		}
	}
	return nil
}

// loadStrictStoredLines reads the fields the cutover gates compare. The caller
// has already established the purchase order's organization and holds its
// write lock; the items table carries no organization column of its own.
func loadStrictStoredLines(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	orderID int64,
) (map[int64]strictStoredLine, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT i.id, i.quotation_item_no, i.quotation_sub_no,
		       i.discount_enabled, i.discount_percent
		FROM %s.purchase_order_items i
		JOIN %s.purchase_orders po ON po.id = i.purchase_order_id
		WHERE i.purchase_order_id = $1 AND po.org_id = $2`, schema, schema),
		orderID, orgID)
	if err != nil {
		return nil, fmt.Errorf("load purchase order %d stored lines: %w", orderID, err)
	}
	defer rows.Close()
	stored := make(map[int64]strictStoredLine)
	for rows.Next() {
		var id int64
		var itemNo, subNo pgtype.Int4
		var line strictStoredLine
		if err := rows.Scan(&id, &itemNo, &subNo,
			&line.DiscountEnabled, &line.DiscountPercent); err != nil {
			return nil, fmt.Errorf("scan purchase order %d stored line: %w", orderID, err)
		}
		if itemNo.Valid {
			value := int(itemNo.Int32)
			line.QuotationItemNo = &value
		}
		if subNo.Valid {
			value := int(subNo.Int32)
			line.QuotationSubNo = &value
		}
		stored[id] = line
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order %d stored lines: %w", orderID, err)
	}
	return stored, nil
}
