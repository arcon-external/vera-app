package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Issue #654: the aggregate PO line is the fourth, PO-only presentation layer.
// One group header groups one or more existing priced lines of any quotation
// depth. The header owns a title, a display quantity, a display unit and a
// customer print toggle; its total is derived from the children and never
// stored. The children remain the priced truth: they keep their own
// description, quantity, unit, price, quotation depth and allocation.
//
// One aggregate level is structural: the header table has no parent pointer and
// the child key points at purchase_order_items, so an aggregate cannot contain
// an aggregate. A composite foreign key (aggregate_group_id, purchase_order_id)
// refuses a child from another purchase order even through raw SQL.
//
// Group ids are not stable across saves: a save replaces every group of the
// record. They are internal handles, never external keys, which is why the
// correction snapshot stores the derived display_order instead.
const (
	// maxAggregateGroupTitleLength bounds the group's customer-facing title to
	// the same 500 Unicode characters the schema CHECK enforces.
	maxAggregateGroupTitleLength = 500
	// maxAggregateGroupUoMLength bounds the display unit label. It is a label
	// only: no unit conversion happens anywhere (AGENTS.md section 10).
	maxAggregateGroupUoMLength = 80
)

var (
	// ErrPurchaseOrderAggregateInvalid refuses a malformed or impossible
	// group: an empty title, unit or quantity, no children, non-contiguous
	// children, a bad group index, or a split display group spread across
	// aggregates. It is a 422 at the API boundary.
	ErrPurchaseOrderAggregateInvalid = errors.New("purchase order aggregate line is invalid")
	// ErrPurchaseOrderClientOutdated refuses a legacy save that would silently
	// rewrite a record it cannot represent: an aggregate group it cannot
	// express (the compatibility contract is the nil-versus-empty distinction on
	// the submitted group slice), or an extra line submitted without a line kind,
	// which would move the pass-through amount to uncosted. The API maps it to a
	// 409 the client answers with the named update prompt.
	ErrPurchaseOrderClientOutdated = errors.New("purchase order client cannot represent newer purchase order lines")
)

// PurchaseOrderAggregateGroup is one PO-only presentation group. TotalMinor and
// DisplayUnitPriceMinor are derived on read; neither is stored.
type PurchaseOrderAggregateGroup struct {
	ID                     int64
	DisplayOrder           int
	Title                  string
	OrderedQuantityScaled  int64
	OrderedUoM             string
	ShowChildrenToCustomer bool
	TotalMinor             int64
	// DisplayUnitPriceMinor is round_half_away(TotalMinor*10000/qty).
	// Nil when the exact value exceeds the safe integer range.
	DisplayUnitPriceMinor *int64
}

// PurchaseOrderAggregateGroupInput is one submitted group. Its position in the
// request slice is the handle items reference; display_order is derived.
type PurchaseOrderAggregateGroupInput struct {
	Title                  string
	OrderedQuantityScaled  int64
	OrderedUoM             string
	ShowChildrenToCustomer bool
}

// normalize trims the two free-text fields a staff member typed, so the stored
// value is the one the bounds were checked against and no invisible padding
// reaches the customer document.
func (input PurchaseOrderAggregateGroupInput) normalize() PurchaseOrderAggregateGroupInput {
	input.Title = strings.TrimSpace(input.Title)
	input.OrderedUoM = strings.TrimSpace(input.OrderedUoM)
	return input
}

// purchaseOrderDisplayGroupKey is the display-group key of one line, mirrored
// from the client's displayGroupKeyOf (frontend/src/poAggregate.ts): a stored
// group key wins; a pre-migration row that still names a quotation sub-item
// belongs to its item under the `q:<item>` key migration 048 backfills; a plain
// line has no key. The server has to read the same key, or the client's
// split-package rule cannot be enforced on a record whose rows carry no stored
// key.
func purchaseOrderDisplayGroupKey(item PurchaseOrderItem) string {
	if item.GroupKey != "" {
		return item.GroupKey
	}
	if item.QuotationItemNo != nil && item.QuotationSubNo != nil {
		return fmt.Sprintf("q:%d", *item.QuotationItemNo)
	}
	return ""
}

// validatePurchaseOrderAggregatePlacement checks the submitted group set and
// the lines' membership before any row is written. Every refusal wraps
// ErrPurchaseOrderAggregateInvalid and names the group or line it refused.
func validatePurchaseOrderAggregatePlacement(
	groups []PurchaseOrderAggregateGroupInput,
	items []PurchaseOrderItem,
) error {
	if groups == nil {
		// A nil slice means "this client does not know about aggregates". It is
		// allowed only when no line claims a group; the store refuses a nil
		// submission on a grouped record separately (refuseLegacyAggregateOverwrite).
		for index, item := range items {
			if item.AggregateGroupIndex != 0 {
				return fmt.Errorf(
					"%w: line %d names aggregate group %d but the request carries no groups",
					ErrPurchaseOrderAggregateInvalid, index+1, item.AggregateGroupIndex)
			}
		}
		return nil
	}

	for index, group := range groups {
		position := index + 1
		title := strings.TrimSpace(group.Title)
		if count := utf8.RuneCountInString(title); count < 1 || count > maxAggregateGroupTitleLength {
			return fmt.Errorf("%w: group %d needs a title of 1 to %d characters",
				ErrPurchaseOrderAggregateInvalid, position, maxAggregateGroupTitleLength)
		}
		if group.OrderedQuantityScaled < 1 || group.OrderedQuantityScaled > MaxExactMinor {
			return fmt.Errorf("%w: group %d display quantity must be between 1 and %d (scaled by %d)",
				ErrPurchaseOrderAggregateInvalid, position, MaxExactMinor, QuantityScale)
		}
		uom := strings.TrimSpace(group.OrderedUoM)
		if count := utf8.RuneCountInString(uom); count < 1 || count > maxAggregateGroupUoMLength {
			return fmt.Errorf("%w: group %d needs a unit label of 1 to %d characters",
				ErrPurchaseOrderAggregateInvalid, position, maxAggregateGroupUoMLength)
		}
	}

	// firstPosition[g] is the submitted position of the group's first child;
	// lastPosition[g] is its most recent one. A group's children must occupy one
	// contiguous run, which keeps display_order (the first child's line number)
	// unique and the read order unambiguous.
	firstPosition := make([]int, len(groups)+1)
	lastPosition := make([]int, len(groups)+1)
	displayGroupIndex := make(map[string]int, len(items))
	for index, item := range items {
		position := index + 1
		groupIndex := item.AggregateGroupIndex
		if groupKey := purchaseOrderDisplayGroupKey(item); groupKey != "" {
			// A split display group (#573) is one package: every row of it must
			// sit in the same aggregate, or the customer would see half a
			// package inside a group and half outside.
			if first, seen := displayGroupIndex[groupKey]; seen && first != groupIndex {
				return fmt.Errorf(
					"%w: line %d splits display group %q between aggregate group %d and %d",
					ErrPurchaseOrderAggregateInvalid, position, groupKey, first, groupIndex)
			}
			displayGroupIndex[groupKey] = groupIndex
		}
		if groupIndex < 0 || groupIndex > len(groups) {
			return fmt.Errorf("%w: line %d names aggregate group %d; the request carries %d groups",
				ErrPurchaseOrderAggregateInvalid, position, groupIndex, len(groups))
		}
		if groupIndex == 0 {
			continue
		}
		if firstPosition[groupIndex] == 0 {
			firstPosition[groupIndex] = position
			lastPosition[groupIndex] = position
			continue
		}
		if position != lastPosition[groupIndex]+1 {
			return fmt.Errorf("%w: group %d is not contiguous (line %d follows line %d)",
				ErrPurchaseOrderAggregateInvalid, groupIndex, position, lastPosition[groupIndex])
		}
		lastPosition[groupIndex] = position
	}
	for groupIndex := 1; groupIndex <= len(groups); groupIndex++ {
		if firstPosition[groupIndex] == 0 {
			return fmt.Errorf("%w: group %d has no child lines",
				ErrPurchaseOrderAggregateInvalid, groupIndex)
		}
	}
	return nil
}

// replacePurchaseOrderItemGroups replaces every aggregate header of one purchase
// order and rewrites the children's membership. The caller owns the transaction
// and calls this after replacePurchaseOrderItems and before the allocations, so
// a failed write restores the previous groups together with the previous lines.
//
// items are the kept lines, so they already carry their persisted id and final
// line number; display_order is derived from the first child's line number and
// never submitted. No money is written here: the PO totals are already computed
// from the item set.
//
// A nil groups slice means the caller's client cannot express aggregates at all.
// It never deletes: silent ungrouping would change the customer-facing document
// without telling anyone, which is what the legacy-client refusal exists to
// prevent. An empty non-nil slice is the ungroup and removes every group.
func replacePurchaseOrderItemGroups(
	ctx context.Context,
	tx pgx.Tx,
	schema, orgID string,
	orderID int64,
	groups []PurchaseOrderAggregateGroupInput,
	items []PurchaseOrderItem,
) error {
	if err := validatePurchaseOrderAggregatePlacement(groups, items); err != nil {
		return err
	}
	if groups == nil {
		return nil
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_item_groups
		 WHERE purchase_order_id = $1 AND org_id = $2`, schema),
		orderID, orgID); err != nil {
		return fmt.Errorf("clear purchase order %d aggregate groups: %w", orderID, err)
	}
	if len(groups) == 0 {
		return nil
	}

	firstLine := make([]int, len(groups)+1)
	for index, item := range items {
		groupIndex := item.AggregateGroupIndex
		if groupIndex == 0 || firstLine[groupIndex] != 0 {
			continue
		}
		lineNumber := item.LineNumber
		if lineNumber < 1 {
			lineNumber = index + 1
		}
		firstLine[groupIndex] = lineNumber
	}
	groupIDs := make([]int64, len(groups)+1)
	for index, group := range groups {
		position := index + 1
		normalized := group.normalize()
		if err := tx.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_item_groups (
				org_id, purchase_order_id, display_order, title,
				ordered_quantity_scaled, ordered_uom, show_children_to_customer
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
			RETURNING id`, schema),
			orgID, orderID, firstLine[position], normalized.Title,
			normalized.OrderedQuantityScaled, normalized.OrderedUoM,
			group.ShowChildrenToCustomer,
		).Scan(&groupIDs[position]); err != nil {
			return fmt.Errorf("store purchase order %d aggregate group %d: %w",
				orderID, position, err)
		}
	}

	// One set-based update attaches every child. Line numbers are unique, so the
	// derived display_order values are unique by construction.
	itemIDs := make([]int64, 0, len(items))
	childGroupIDs := make([]int64, 0, len(items))
	for index, item := range items {
		if item.AggregateGroupIndex == 0 {
			continue
		}
		if item.ID < 1 {
			return fmt.Errorf("%w: line %d has no persisted id for its aggregate group",
				ErrPurchaseOrderAggregateInvalid, index+1)
		}
		itemIDs = append(itemIDs, item.ID)
		childGroupIDs = append(childGroupIDs, groupIDs[item.AggregateGroupIndex])
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_items poi
		SET aggregate_group_id = v.group_id
		FROM unnest($2::bigint[], $3::bigint[]) AS v(item_id, group_id)
		WHERE poi.purchase_order_id = $1 AND poi.id = v.item_id`, schema),
		orderID, itemIDs, childGroupIDs); err != nil {
		return fmt.Errorf("attach purchase order %d aggregate children: %w", orderID, err)
	}

	// Cheap post-check: every header must have at least one child. Validation
	// already guarantees it, so a miss here means a future caller skipped it;
	// failing rolls the whole transaction back rather than storing a fake line.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT g.id, count(poi.id)
		FROM %s.purchase_order_item_groups g
		LEFT JOIN %s.purchase_order_items poi
		       ON poi.purchase_order_id = g.purchase_order_id
		      AND poi.aggregate_group_id = g.id
		WHERE g.purchase_order_id = $1 AND g.org_id = $2
		GROUP BY g.id`, schema, schema), orderID, orgID)
	if err != nil {
		return fmt.Errorf("verify purchase order %d aggregate groups: %w", orderID, err)
	}
	defer rows.Close()
	verified := 0
	for rows.Next() {
		var id, children int64
		if err := rows.Scan(&id, &children); err != nil {
			return fmt.Errorf("scan purchase order %d aggregate group child count: %w", orderID, err)
		}
		if children < 1 {
			return fmt.Errorf("purchase order %d aggregate group %d has no children after the write", orderID, id)
		}
		verified++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate purchase order %d aggregate group child counts: %w", orderID, err)
	}
	if verified != len(groups) {
		return fmt.Errorf("purchase order %d stored %d of %d aggregate groups", orderID, verified, len(groups))
	}
	return nil
}

// loadPurchaseOrderItemGroups reads one purchase order's aggregate headers in
// display order with their derived totals. An empty group (a priced line removed
// out of band) reads back with a total of 0 rather than disappearing, so staff
// can still ungroup it.
func loadPurchaseOrderItemGroups(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	orderID int64,
) ([]PurchaseOrderAggregateGroup, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT g.id, g.display_order, g.title, g.ordered_quantity_scaled,
		       g.ordered_uom, g.show_children_to_customer,
		       COALESCE(SUM(poi.net_minor), 0)::bigint AS total_minor
		FROM %s.purchase_order_item_groups g
		LEFT JOIN %s.purchase_order_items poi
		       ON poi.purchase_order_id = g.purchase_order_id
		      AND poi.aggregate_group_id = g.id
		WHERE g.purchase_order_id = $1 AND g.org_id = $2
		GROUP BY g.id
		ORDER BY g.display_order`, schema, schema), orderID, orgID)
	if err != nil {
		return nil, fmt.Errorf("load purchase order %d aggregate groups: %w", orderID, err)
	}
	defer rows.Close()
	var groups []PurchaseOrderAggregateGroup
	for rows.Next() {
		var group PurchaseOrderAggregateGroup
		if err := rows.Scan(&group.ID, &group.DisplayOrder, &group.Title,
			&group.OrderedQuantityScaled, &group.OrderedUoM,
			&group.ShowChildrenToCustomer, &group.TotalMinor); err != nil {
			return nil, fmt.Errorf("scan purchase order %d aggregate group: %w", orderID, err)
		}
		group.DisplayUnitPriceMinor = aggregateDisplayUnitPriceMinor(
			group.TotalMinor, group.OrderedQuantityScaled)
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order %d aggregate groups: %w", orderID, err)
	}
	return groups, nil
}

// aggregateDisplayUnitPriceMinor derives the customer-facing unit price of an
// aggregate line: round_half_away(total * 10000 / display quantity). It is a
// display figure only -- the total is authoritative -- and it is nil when the
// value cannot cross the exact transport range in either direction, so a tiny
// display quantity shows "---" instead of a wrapped number.
func aggregateDisplayUnitPriceMinor(totalMinor, quantityScaled int64) *int64 {
	value, err := poMulDiv(totalMinor, QuantityScale, quantityScaled, true)
	if err != nil {
		return nil
	}
	return &value
}
