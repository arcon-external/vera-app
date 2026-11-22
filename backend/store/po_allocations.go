package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Issue #650: the allocation is the canonical link between one quotation-backed
// PO line and the exact quotation revision leaf it fulfils. The PO line keeps
// the customer-facing values; the allocation keeps the source quantity, the
// frozen quotation price baseline, the actual PO revenue and the frozen
// supplier cost.
//
// A draft created from a quotation writes one allocation per priced source
// line. Finalize and correction rewrite them from the final lines; correction
// snapshots the old allocations beside the old PO lines. A line with no
// quotation reference is legacy and writes no allocation: the report never
// invents a source from a description or position.

func allocationSourceKey(itemNo int, subNo *int) string {
	if subNo == nil {
		return fmt.Sprintf("%d.", itemNo)
	}
	return fmt.Sprintf("%d.%d", itemNo, *subNo)
}

// replacePurchaseOrderAllocations replaces every allocation of one PO from the
// persisted line set. The caller owns the transaction, so a failed insert
// restores the previous allocations. source must carry the exact revision and
// its frozen lines.
func replacePurchaseOrderAllocations(
	ctx context.Context,
	tx pgx.Tx,
	schema, orgID string,
	orderID int64,
	source PurchaseOrderSource,
	lines []PurchaseOrderItem,
) error {
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_allocations WHERE purchase_order_id = $1`, schema),
		orderID); err != nil {
		return fmt.Errorf("clear purchase order %d allocations: %w", orderID, err)
	}
	if source.QuotationID < 1 || len(lines) == 0 {
		return nil
	}
	bySource := make(map[string]PurchaseOrderSourceLine, len(source.Lines))
	for _, line := range source.Lines {
		bySource[allocationSourceKey(line.QuotationItemNo, line.QuotationSubNo)] = line
	}
	for index, item := range lines {
		if item.QuotationItemNo == nil {
			// A legacy or free line has no cost identity. The new workspace
			// writes extra lines explicitly in a later ticket; this ticket
			// must not invent an allocation for either.
			continue
		}
		if item.ID < 1 {
			return fmt.Errorf("%w: line %d has no persisted id for its allocation",
				ErrPurchaseOrderInvalid, index+1)
		}
		sourceLine, ok := bySource[allocationSourceKey(*item.QuotationItemNo, item.QuotationSubNo)]
		if !ok {
			return fmt.Errorf("%w: line %d names a source leaf missing from revision %d",
				ErrPurchaseOrderInvalid, index+1, source.RevisionNumber)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_allocations (
				org_id, purchase_order_id, purchase_order_item_id,
				quotation_id, source_revision, source_revision_id,
				quotation_item_no, quotation_sub_no,
				quantity_scaled, source_uom, ordered_uom,
				quoted_unit_price_minor, revenue_minor, cost_unit_minor
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, schema),
			orgID, orderID, item.ID,
			source.QuotationID, source.RevisionNumber, source.RevisionID,
			item.QuotationItemNo, item.QuotationSubNo,
			item.QuantityScaled, sourceLine.UoM, item.UoM,
			sourceLine.quotedUnitPriceMinor, item.NetMinor, sourceLine.costUnitMinor,
		); err != nil {
			return fmt.Errorf("store purchase order %d line %d allocation: %w",
				orderID, index+1, err)
		}
	}
	return nil
}

// snapshotPurchaseOrderCorrectionAllocations keeps the allocation picture of a
// saved PO beside the immutable correction snapshot. The live allocations may
// be replaced immediately afterwards; the audit copy stays readable.
func snapshotPurchaseOrderCorrectionAllocations(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema string,
	correctionID, orderID int64,
) error {
	if _, err := q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_correction_allocations (
			correction_id, line_number, quotation_id, source_revision,
			source_revision_id, quotation_item_no, quotation_sub_no,
			quantity_scaled, source_uom, ordered_uom,
			quoted_unit_price_minor, revenue_minor, cost_unit_minor
		)
		SELECT $1, poi.line_number, a.quotation_id, a.source_revision,
		       a.source_revision_id, a.quotation_item_no, a.quotation_sub_no,
		       a.quantity_scaled, a.source_uom, a.ordered_uom,
		       a.quoted_unit_price_minor, a.revenue_minor, a.cost_unit_minor
		FROM %s.purchase_order_allocations a
		JOIN %s.purchase_order_items poi
		  ON poi.id = a.purchase_order_item_id
		 AND poi.purchase_order_id = a.purchase_order_id
		WHERE a.purchase_order_id = $2
		ORDER BY poi.line_number`, schema, schema, schema),
		correctionID, orderID); err != nil {
		return fmt.Errorf("snapshot purchase order %d allocations: %w", orderID, err)
	}
	return nil
}

// peekPurchaseOrderQuotationID reads the source link without locking the PO, so
// the caller can acquire the quotation lock first and preserve the global lock
// order (quotation, then PO).
func peekPurchaseOrderQuotationID(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	orderID int64,
) (int64, error) {
	var quotationID int64
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(quotation_id, 0)
		FROM %s.purchase_orders
		WHERE id = $1 AND org_id = $2`, schema), orderID, orgID).Scan(&quotationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrPurchaseOrderNotFound
		}
		return 0, fmt.Errorf("read purchase order %d source: %w", orderID, err)
	}
	return quotationID, nil
}

// lockQuotationForAllocation takes the parent quotation row before the PO row.
// A source-missing PO has no live quotation and locks nothing here; the
// finalize/correction boundary refuses it separately.
func lockQuotationForAllocation(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema string,
	quotationID int64,
) error {
	if quotationID < 1 {
		return nil
	}
	var id int64
	if err := q.QueryRow(ctx, fmt.Sprintf(
		`SELECT id FROM %s.quotations WHERE id = $1 FOR UPDATE`, schema),
		quotationID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPurchaseOrderSourceNotFound
		}
		return fmt.Errorf("lock quotation %d for allocation: %w", quotationID, err)
	}
	return nil
}
