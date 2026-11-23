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
	// skipDuplicateLeaves lets a draft carry two lines that claim one leaf
	// while the reviewer is resolving the duplicate. The first line keeps the
	// allocation; the duplicate stays unallocated and finalization refuses it
	// through the unresolved-difference gate. Every committed path passes
	// false, so the typed refusal is still the rule there.
	skipDuplicateLeaves bool,
) error {
	// One quotation leaf may appear at most once in a purchase order. The
	// database enforces it with po_allocations_po_source_uidx (NULLS NOT
	// DISTINCT, so a null sub-item is one leaf, not many); checking here turns
	// the raw 23505 into the typed refusal the editor can explain, before any
	// allocation is cleared.
	claimed := make(map[string]int, len(lines))
	skipLine := make(map[int]bool)
	for index, item := range lines {
		if item.QuotationItemNo == nil {
			continue
		}
		lineNumber := item.LineNumber
		if lineNumber < 1 {
			lineNumber = index + 1
		}
		key := allocationSourceKey(*item.QuotationItemNo, item.QuotationSubNo)
		if first, taken := claimed[key]; taken {
			if skipDuplicateLeaves {
				skipLine[lineNumber] = true
				continue
			}
			leaf := fmt.Sprintf("quotation item %d", *item.QuotationItemNo)
			if item.QuotationSubNo != nil {
				leaf = fmt.Sprintf("quotation item %d sub-item %d",
					*item.QuotationItemNo, *item.QuotationSubNo)
			}
			return fmt.Errorf(
				"%w: line %d names the same %s as line %d; one quotation leaf may appear once per purchase order",
				ErrPurchaseOrderInvalid, lineNumber, leaf, first)
		}
		claimed[key] = lineNumber
	}
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
		lineNumber := item.LineNumber
		if lineNumber < 1 {
			lineNumber = index + 1
		}
		if skipLine[lineNumber] {
			continue
		}
		if item.ID < 1 {
			return fmt.Errorf("%w: line %d has no persisted id for its allocation",
				ErrPurchaseOrderInvalid, lineNumber)
		}
		sourceLine, ok := bySource[allocationSourceKey(*item.QuotationItemNo, item.QuotationSubNo)]
		if !ok {
			return fmt.Errorf("%w: line %d names a source leaf missing from revision %d",
				ErrPurchaseOrderInvalid, lineNumber, source.RevisionNumber)
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
			sourceLine.QuotedUnitPriceMinor, item.NetMinor, sourceLine.costUnitMinor,
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
