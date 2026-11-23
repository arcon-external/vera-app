package store

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"vera-app/backend/poreport"
)

// Issue #657: the PO Fulfillment drill-down.
//
// Fulfillment is a paginated, quotation-first projection over the same facts the
// report aggregates from: a quotation's latest issued revision supplies the
// quoted leaves and their row-discounted customer baseline, and cross-revision
// saved PO allocations supply the bought side. Unlike the report it is never
// part of the report snapshot: it is paged, so attaching it to every report read
// would inflate a figure staff did not ask for.
//
// # What it covers, and what it cannot
//
// Fulfillment covers source-linked quotations. A source-less purchase order (a
// legacy blank draft) and a purchase order whose quotation was deleted have no
// quotation row to list under, so they are excluded here. Their realized
// variance is not lost: the Actual report still counts it through its own
// aggregate. The difference against Actual is therefore expected, not a bug.
//
// # Drafts reserve nothing
//
// Only saved purchase orders buy quantity. A draft is advisory and contributes
// neither a bought quantity nor a contribution row; extras on a draft are not
// listed either, because a draft's amount is not an actual expense yet.
//
// # Status precedence
//
// A quotation's status is derived, never stored: closed > all_bought >
// expired_unclosed > open. Closure wins because the remainder is already Closed
// Loss; a fully bought quotation is never "expired" because it has no open
// remainder to lapse. Expiry is the latest issued revision's issue date plus its
// validity days, compared with the company's WIB calendar day; the last valid
// day itself is still valid. A newer revision resets the clock.
//
// # Removed leaves
//
// Bought state spans revisions. A leaf bought on an earlier revision and removed
// from the latest one is still listed with Removed set, its frozen realized
// revenue and price variance, and zero quoted, unsold and over-bought quantity.
// It can therefore never be mistaken for a false over-buy.
//
// # Aggregate groups and extras
//
// An aggregate header is presentation only, so fulfilment decomposes its
// children and puts the group's title on each child; the header is never a row
// of its own. An extra expense line is listed separately in ExtraExpenses and
// never enters quoted, bought, unsold or over-bought. An extra inside an
// aggregate carries the same group title.
//
// # Exact money
//
// Every amount crosses JSON, so every figure is bounded by poreport.CheckExact.
// No supplier unit cost is exposed: only realized margin, which the report
// already publishes as an aggregate. Per-leaf capital is rounded once per leaf;
// the report rounds once per purchase order, so a per-leaf breakdown can differ
// from the report's total by a minor unit without either being wrong.

// ReportPurchaseOrderFulfillment reads one filtered page of the fulfillment
// drill-down. now is the caller's clock: the store resolves the company's WIB
// calendar day from it once so status and ValidUntil cannot disagree with the
// report's as-of date.
//
// Filters run before pagination, and the page rows and the total come from one
// REPEATABLE READ snapshot, so a page can never pair a count with rows from a
// later transaction. Rows are ordered newest issue first, then by quotation id.
func (s *Store) ReportPurchaseOrderFulfillment(
	ctx context.Context, query poreport.FulfillmentQuery, now time.Time,
) (poreport.FulfillmentPage, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return poreport.FulfillmentPage{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return poreport.FulfillmentPage{}, err
	}
	query, err = poreport.NormalizeFulfillmentQuery(query)
	if err != nil {
		return poreport.FulfillmentPage{}, err
	}

	size := int64(query.PageSize)
	if int64(query.Page-1) > math.MaxInt64/size {
		return poreport.FulfillmentPage{}, fmt.Errorf(
			"%w: page %d is past the largest representable offset",
			poreport.ErrInvalid, query.Page)
	}
	offset := int64(query.Page-1) * size
	asOf := companyToday(now)
	asOfText := asOf.Format(time.DateOnly)
	search := escapeLike(query.Search)

	// The count and the page must read the same quotation population, so they
	// share one snapshot. The heavy leaf/bought expansion is evaluated once per
	// statement; the two statements are cheap beside it.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return poreport.FulfillmentPage{}, fmt.Errorf("begin purchase order fulfillment read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	filterSQL := purchaseOrderFulfillmentFilterSQL(s.configSchema())
	filterArgs := []any{org.ID, query.Currency, query.Status, search, asOfText}

	var total int
	if err := tx.QueryRow(ctx, filterSQL+`
SELECT COUNT(*) FROM filtered`, filterArgs...).Scan(&total); err != nil {
		return poreport.FulfillmentPage{}, fmt.Errorf("count purchase order fulfillment: %w", err)
	}

	page := poreport.FulfillmentPage{
		AsOf:       asOfText,
		Filter:     query,
		Quotations: []poreport.FulfillmentQuotation{},
		Page:       query.Page,
		PageSize:   query.PageSize,
		Total:      total,
	}
	page.TotalPages = total / query.PageSize
	if total%query.PageSize != 0 {
		page.TotalPages++
	}
	if page.TotalPages < 1 {
		// One empty page, not "page 1 of 0": the screen keeps working pagination
		// controls while its empty state is shown.
		page.TotalPages = 1
	}
	if total == 0 || offset >= int64(total) {
		return page, nil
	}

	rows, err := tx.Query(ctx, filterSQL+`
SELECT quotation_id, quote_number, customer_name, currency, latest_revision,
       issued_at, valid_until, status
FROM filtered
ORDER BY issued_at DESC, quotation_id DESC
LIMIT $6 OFFSET $7`, append(filterArgs, query.PageSize, offset)...)
	if err != nil {
		return poreport.FulfillmentPage{}, fmt.Errorf("read purchase order fulfillment page: %w", err)
	}
	var quotationIDs []int64
	for rows.Next() {
		var row fulfillmentQuotationRow
		if err := rows.Scan(&row.QuotationID, &row.QuoteNumber, &row.CustomerName,
			&row.Currency, &row.LatestRevision, &row.IssuedAt, &row.ValidUntil,
			&row.Status); err != nil {
			rows.Close()
			return poreport.FulfillmentPage{}, fmt.Errorf("scan purchase order fulfillment quotation: %w", err)
		}
		page.Quotations = append(page.Quotations, poreport.FulfillmentQuotation{
			QuotationID:    row.QuotationID,
			QuoteNumber:    row.QuoteNumber,
			CustomerName:   row.CustomerName,
			Currency:       row.Currency,
			LatestRevision: row.LatestRevision,
			IssuedAt:       row.IssuedAt.Format(time.DateOnly),
			ValidUntil:     row.ValidUntil.Format(time.DateOnly),
			Status:         row.Status,
			Items:          []poreport.FulfillmentItem{},
			ExtraExpenses:  []poreport.FulfillmentExtraExpense{},
		})
		quotationIDs = append(quotationIDs, row.QuotationID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return poreport.FulfillmentPage{}, fmt.Errorf("iterate purchase order fulfillment quotations: %w", err)
	}
	rows.Close()

	if err := s.fillPurchaseOrderFulfillment(ctx, tx, org.ID, query.Currency, quotationIDs, page.Quotations); err != nil {
		return poreport.FulfillmentPage{}, err
	}
	return page, nil
}

// fulfillmentQuotationRow is one page row before its item detail is attached.
type fulfillmentQuotationRow struct {
	QuotationID    int64
	QuoteNumber    string
	CustomerName   string
	Currency       string
	LatestRevision int16
	IssuedAt       time.Time
	ValidUntil     time.Time
	Status         string
}

// fulfillmentLeafKey identifies one priced leaf across revisions.
type fulfillmentLeafKey struct {
	QuotationID int64
	ItemNo      int
	SubNo       int
}

// fulfillmentItemRow is one leaf's quoted and bought aggregate. The quoted side
// is nil exactly for a bought leaf removed by a later revision.
type fulfillmentItemRow struct {
	QuotationID          int64
	ItemNo               int
	SubNo                int
	QuotedQuantityScaled *int64
	QuotedUnitPriceMinor *int64
	CostUnitMinor        *int64
	Description          string
	BoughtQuantityScaled int64
	RevenueMinor         int64
	CostedRevenueMinor   int64
	CapitalScaled        int64
	BoughtAtQuotedMinor  int64
	Removed              bool
}

// fulfillmentContributionRow is one saved PO line's allocation to one leaf. Its
// group title and line description are read here so a removed leaf can still be
// described and a grouped child can name its aggregate.
type fulfillmentContributionRow struct {
	QuotationID             int64
	ItemNo                  int
	SubNo                   int
	ID                      int64
	PONumber                string
	State                   string
	ReceivedDate            time.Time
	AllocatedQuantityScaled int64
	GroupTitle              string
	Description             string
}

// fulfillmentExtraRow is one explicit extra expense line on a saved PO.
type fulfillmentExtraRow struct {
	QuotationID     int64
	PurchaseOrderID int64
	PONumber        string
	LineNumber      int
	Description     string
	QuantityScaled  int64
	UoM             string
	AmountMinor     int64
	GroupTitle      string
}

// fillPurchaseOrderFulfillment attaches contributions, leaves and extras to the
// page rows. All reads share the caller's snapshot.
func (s *Store) fillPurchaseOrderFulfillment(
	ctx context.Context,
	tx pgx.Tx,
	orgID string,
	currency string,
	quotationIDs []int64,
	quotations []poreport.FulfillmentQuotation,
) error {
	schema := s.configSchema()
	index := make(map[int64]int, len(quotations))
	for position := range quotations {
		index[quotations[position].QuotationID] = position
	}

	contributions := make(map[fulfillmentLeafKey][]poreport.FulfillmentContribution)
	groupTitles := make(map[fulfillmentLeafKey]string)
	descriptions := make(map[fulfillmentLeafKey]string)

	rows, err := tx.Query(ctx, purchaseOrderFulfillmentContributionsSQL(schema),
		orgID, currency, quotationIDs)
	if err != nil {
		return fmt.Errorf("read purchase order fulfillment contributions: %w", err)
	}
	for rows.Next() {
		var row fulfillmentContributionRow
		if err := rows.Scan(&row.QuotationID, &row.ItemNo, &row.SubNo,
			&row.ID, &row.PONumber, &row.State, &row.ReceivedDate,
			&row.AllocatedQuantityScaled, &row.GroupTitle,
			&row.Description); err != nil {
			rows.Close()
			return fmt.Errorf("scan purchase order fulfillment contribution: %w", err)
		}
		key := fulfillmentLeafKey{QuotationID: row.QuotationID, ItemNo: row.ItemNo, SubNo: row.SubNo}
		if err := checkFulfillmentExact("contribution quantity", row.AllocatedQuantityScaled); err != nil {
			rows.Close()
			return err
		}
		contributions[key] = append(contributions[key], poreport.FulfillmentContribution{
			ID:                      row.ID,
			PONumber:                row.PONumber,
			State:                   row.State,
			ReceivedDate:            row.ReceivedDate.Format(time.DateOnly),
			AllocatedQuantityScaled: row.AllocatedQuantityScaled,
		})
		if row.GroupTitle != "" && groupTitles[key] == "" {
			groupTitles[key] = row.GroupTitle
		}
		if row.Description != "" && descriptions[key] == "" {
			descriptions[key] = row.Description
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate purchase order fulfillment contributions: %w", err)
	}
	rows.Close()

	itemRows, err := tx.Query(ctx, purchaseOrderFulfillmentItemsSQL(schema),
		orgID, currency, quotationIDs)
	if err != nil {
		return fmt.Errorf("read purchase order fulfillment items: %w", err)
	}
	for itemRows.Next() {
		var row fulfillmentItemRow
		if err := itemRows.Scan(&row.QuotationID, &row.ItemNo, &row.SubNo,
			&row.QuotedQuantityScaled, &row.QuotedUnitPriceMinor, &row.CostUnitMinor,
			&row.Description,
			&row.BoughtQuantityScaled, &row.RevenueMinor, &row.CostedRevenueMinor,
			&row.CapitalScaled, &row.BoughtAtQuotedMinor, &row.Removed); err != nil {
			itemRows.Close()
			return fmt.Errorf("scan purchase order fulfillment item: %w", err)
		}
		position, ok := index[row.QuotationID]
		if !ok {
			continue
		}
		key := fulfillmentLeafKey{QuotationID: row.QuotationID, ItemNo: row.ItemNo, SubNo: row.SubNo}
		item, err := buildFulfillmentItem(row, key, quotations[position].Status,
			contributions[key], groupTitles[key], descriptions[key])
		if err != nil {
			itemRows.Close()
			return err
		}
		quotations[position].Items = append(quotations[position].Items, item)
	}
	if err := itemRows.Err(); err != nil {
		itemRows.Close()
		return fmt.Errorf("iterate purchase order fulfillment items: %w", err)
	}
	itemRows.Close()

	extraRows, err := tx.Query(ctx, purchaseOrderFulfillmentExtrasSQL(schema),
		orgID, currency, quotationIDs)
	if err != nil {
		return fmt.Errorf("read purchase order fulfillment extra expenses: %w", err)
	}
	for extraRows.Next() {
		var row fulfillmentExtraRow
		if err := extraRows.Scan(&row.QuotationID, &row.PurchaseOrderID,
			&row.PONumber, &row.LineNumber, &row.Description,
			&row.QuantityScaled, &row.UoM, &row.AmountMinor, &row.GroupTitle); err != nil {
			extraRows.Close()
			return fmt.Errorf("scan purchase order fulfillment extra expense: %w", err)
		}
		position, ok := index[row.QuotationID]
		if !ok {
			continue
		}
		if err := checkFulfillmentExact("extra expense amount", row.AmountMinor); err != nil {
			extraRows.Close()
			return err
		}
		if err := checkFulfillmentExact("extra expense quantity", row.QuantityScaled); err != nil {
			extraRows.Close()
			return err
		}
		quotations[position].ExtraExpenses = append(quotations[position].ExtraExpenses,
			poreport.FulfillmentExtraExpense{
				PurchaseOrderID: row.PurchaseOrderID,
				PONumber:        row.PONumber,
				LineNumber:      row.LineNumber,
				Description:     row.Description,
				QuantityScaled:  row.QuantityScaled,
				UoM:             row.UoM,
				AmountMinor:     row.AmountMinor,
				GroupTitle:      row.GroupTitle,
			})
	}
	if err := extraRows.Err(); err != nil {
		extraRows.Close()
		return fmt.Errorf("iterate purchase order fulfillment extra expenses: %w", err)
	}
	extraRows.Close()
	return nil
}

// buildFulfillmentItem derives one leaf's figures from its quoted side and its
// cross-revision bought aggregate. A removed leaf keeps only the realized side;
// every remainder is zero by construction.
func buildFulfillmentItem(
	row fulfillmentItemRow,
	key fulfillmentLeafKey,
	status string,
	contributions []poreport.FulfillmentContribution,
	groupTitle, fallbackDescription string,
) (poreport.FulfillmentItem, error) {
	quoted := int64(0)
	quotedUnitPrice := int64(0)
	costUnit := int64(0)
	if row.QuotedQuantityScaled != nil {
		quoted = *row.QuotedQuantityScaled
	}
	if row.QuotedUnitPriceMinor != nil {
		quotedUnitPrice = *row.QuotedUnitPriceMinor
	}
	if row.CostUnitMinor != nil {
		costUnit = *row.CostUnitMinor
	}

	description := row.Description
	if description == "" {
		description = fallbackDescription
	}
	item := poreport.FulfillmentItem{
		QuotationItemNo:       row.ItemNo,
		Label:                 fulfillmentLabel(row.ItemNo, key.SubNo),
		Description:           description,
		GroupTitle:            groupTitle,
		Removed:               row.Removed,
		QuotedQuantityScaled:  quoted,
		BoughtQuantityScaled:  row.BoughtQuantityScaled,
		RealizedAmountMinor:   row.RevenueMinor,
		PriceExtraIncomeMinor: max(row.RevenueMinor-row.BoughtAtQuotedMinor, 0),
		PriceLossMinor:        max(row.BoughtAtQuotedMinor-row.RevenueMinor, 0),
		PurchaseOrders:        contributions,
	}
	if key.SubNo != 0 {
		subNo := key.SubNo
		item.QuotationSubNo = &subNo
	}
	if item.PurchaseOrders == nil {
		item.PurchaseOrders = []poreport.FulfillmentContribution{}
	}

	capital, err := fulfillmentMulDiv(row.CapitalScaled, 1, QuantityScale)
	if err != nil {
		return poreport.FulfillmentItem{}, fmt.Errorf(
			"%w: realized capital of %s: %v", poreport.ErrValueOutOfRange, item.Label, err)
	}
	item.RealizedMarginMinor = row.CostedRevenueMinor - capital

	if !row.Removed {
		switch {
		case row.BoughtQuantityScaled > quoted:
			over := row.BoughtQuantityScaled - quoted
			amount, err := fulfillmentMulDiv(over, quotedUnitPrice, QuantityScale)
			if err != nil {
				return poreport.FulfillmentItem{}, fmt.Errorf(
					"%w: quantity extra amount of %s: %v",
					poreport.ErrValueOutOfRange, item.Label, err)
			}
			cost, err := fulfillmentMulDiv(over, costUnit, QuantityScale)
			if err != nil {
				return poreport.FulfillmentItem{}, fmt.Errorf(
					"%w: quantity extra cost of %s: %v",
					poreport.ErrValueOutOfRange, item.Label, err)
			}
			item.OverBoughtQuantityScaled = over
			item.QuantityExtraIncomeAmountMinor = amount
			item.QuantityExtraIncomeMarginMinor = amount - cost
		case row.BoughtQuantityScaled < quoted:
			unbought := quoted - row.BoughtQuantityScaled
			amount, err := fulfillmentMulDiv(unbought, quotedUnitPrice, QuantityScale)
			if err != nil {
				return poreport.FulfillmentItem{}, fmt.Errorf(
					"%w: remainder amount of %s: %v",
					poreport.ErrValueOutOfRange, item.Label, err)
			}
			cost, err := fulfillmentMulDiv(unbought, costUnit, QuantityScale)
			if err != nil {
				return poreport.FulfillmentItem{}, fmt.Errorf(
					"%w: remainder cost of %s: %v",
					poreport.ErrValueOutOfRange, item.Label, err)
			}
			if status == string(poreport.FulfillmentStatusClosed) {
				item.ClosedLossAmountMinor = amount
				item.ClosedLossMarginMinor = amount - cost
			} else {
				item.UnsoldQuantityScaled = unbought
				item.UnsoldAmountMinor = amount
				item.UnsoldMarginMinor = amount - cost
			}
		}
	}

	for name, value := range map[string]int64{
		"quoted quantity":              item.QuotedQuantityScaled,
		"bought quantity":              item.BoughtQuantityScaled,
		"unsold quantity":              item.UnsoldQuantityScaled,
		"over-bought quantity":         item.OverBoughtQuantityScaled,
		"realized amount":              item.RealizedAmountMinor,
		"realized margin":              item.RealizedMarginMinor,
		"unsold amount":                item.UnsoldAmountMinor,
		"unsold margin":                item.UnsoldMarginMinor,
		"quantity extra income amount": item.QuantityExtraIncomeAmountMinor,
		"quantity extra income margin": item.QuantityExtraIncomeMarginMinor,
		"price extra income":           item.PriceExtraIncomeMinor,
		"price loss":                   item.PriceLossMinor,
		"closed loss amount":           item.ClosedLossAmountMinor,
		"closed loss margin":           item.ClosedLossMarginMinor,
	} {
		if err := checkFulfillmentExact(name+" of "+item.Label, value); err != nil {
			return poreport.FulfillmentItem{}, err
		}
	}
	return item, nil
}

func fulfillmentLabel(itemNo, subNo int) string {
	if subNo == 0 {
		return fmt.Sprintf("%d", itemNo)
	}
	return fmt.Sprintf("%d.%d", itemNo, subNo)
}

// fulfillmentMulDiv rounds a scaled quantity times a unit price to minor units,
// half away from zero, exactly as the report's SQL ROUND does.
func fulfillmentMulDiv(left, right, denominator int64) (int64, error) {
	return poMulDiv(left, right, denominator, true)
}

func checkFulfillmentExact(field string, value int64) error {
	if err := poreport.CheckExact(value); err != nil {
		return fmt.Errorf("purchase order fulfillment %s: %w", field, err)
	}
	return nil
}

// purchaseOrderFulfillmentFilterSQL is the shared status/filter CTE. The same
// text feeds the count and the page, so filtering before pagination is
// structural rather than a promise. Parameters are:
//
//	$1 org id, $2 currency, $3 status ('all' or one state), $4 escaped search,
//	$5 as-of company date.
func purchaseOrderFulfillmentFilterSQL(schema string) string {
	return fmt.Sprintf(`
WITH latest_revision AS (
    -- The true latest issued revision, selected before any currency or search
    -- filter: a quotation whose latest revision is USD must not answer an IDR
    -- page from an older IDR revision.
    SELECT DISTINCT ON (r.quotation_id)
           r.id AS revision_id, r.quotation_id, r.revision_number,
           r.currency, r.created_at, r.validity_days,
           q.quote_number, q.closed_at,
           COALESCE(NULLIF(r.recipient_company, ''), c.legal_name) AS customer_name
    FROM %[1]s.quotation_revisions r
    JOIN %[1]s.quotations q ON q.id = r.quotation_id
    JOIN %[1]s.customers c ON c.id = q.customer_id
    ORDER BY r.quotation_id, r.revision_number DESC
),
candidates AS (
    SELECT * FROM latest_revision
    WHERE currency = $2
      AND (
          $4 = ''
          OR quote_number ILIKE '%%' || $4 || '%%'
          OR customer_name ILIKE '%%' || $4 || '%%'
      )
),
leaves AS (
    -- A plain line is one leaf; a package whose parts print with prices is its
    -- parts. Exactly the report pipeline's leaf expansion, kept local so the
    -- report and fulfillment may evolve independently.
    SELECT c.quotation_id, i.item_no, NULL::integer AS sub_no,
           i.qty::numeric * 10000 AS quoted_qty_scaled
    FROM candidates c
    JOIN %[1]s.quotation_revision_items i ON i.revision_id = c.revision_id
    WHERE NOT (i.show_sub_prices AND EXISTS (
        SELECT 1 FROM %[1]s.quotation_revision_sub_items s
        WHERE s.revision_item_id = i.id
    ))
    UNION ALL
    SELECT c.quotation_id, i.item_no, s.sub_no,
           s.qty::numeric * 10000
    FROM candidates c
    JOIN %[1]s.quotation_revision_items i ON i.revision_id = c.revision_id
    JOIN %[1]s.quotation_revision_sub_items s ON s.revision_item_id = i.id
    WHERE i.show_sub_prices
),
bought AS (
    -- Bought state is cross-revision and saved-only. Drafts reserve nothing.
    SELECT a.quotation_id, a.quotation_item_no AS item_no,
           COALESCE(a.quotation_sub_no, 0) AS sub_no,
           SUM(a.quantity_scaled::numeric) AS bought_qty_scaled
    FROM %[1]s.purchase_order_allocations a
    JOIN %[1]s.purchase_orders po ON po.id = a.purchase_order_id
    JOIN candidates c ON c.quotation_id = a.quotation_id
    WHERE po.org_id = $1 AND po.state = 'saved' AND po.currency = $2
    GROUP BY a.quotation_id, a.quotation_item_no, COALESCE(a.quotation_sub_no, 0)
),
leaf_state AS (
    SELECT c.quotation_id,
           COUNT(l.quotation_id) AS leaf_count,
           COUNT(*) FILTER (
               WHERE COALESCE(b.bought_qty_scaled, 0) < l.quoted_qty_scaled
           ) AS unbought_count
    FROM candidates c
    LEFT JOIN leaves l ON l.quotation_id = c.quotation_id
    LEFT JOIN bought b
           ON b.quotation_id = l.quotation_id
          AND b.item_no = l.item_no
          AND b.sub_no = COALESCE(l.sub_no, 0)
    GROUP BY c.quotation_id
),
statused AS (
    SELECT c.quotation_id, c.quote_number, c.customer_name, c.currency,
           c.revision_number AS latest_revision,
           (c.created_at AT TIME ZONE 'Asia/Jakarta')::date AS issued_at,
           (c.created_at AT TIME ZONE 'Asia/Jakarta')::date + c.validity_days
               AS valid_until,
           CASE
               WHEN c.closed_at IS NOT NULL THEN 'closed'
               WHEN COALESCE(s.leaf_count, 0) > 0
                    AND COALESCE(s.unbought_count, 0) = 0 THEN 'all_bought'
               WHEN (c.created_at AT TIME ZONE 'Asia/Jakarta')::date
                    + c.validity_days < $5::date THEN 'expired_unclosed'
               ELSE 'open'
           END AS status
    FROM candidates c
    LEFT JOIN leaf_state s ON s.quotation_id = c.quotation_id
),
filtered AS (
    SELECT * FROM statused WHERE $3 = 'all' OR status = $3
)
`, schema)
}

// purchaseOrderFulfillmentItemsSQL reads each leaf's quoted side and its
// cross-revision bought aggregate. A bought leaf absent from the latest revision
// survives the FULL OUTER JOIN with nil quoted columns and Removed set.
func purchaseOrderFulfillmentItemsSQL(schema string) string {
	return fmt.Sprintf(`
WITH latest_revision AS (
    SELECT DISTINCT ON (r.quotation_id)
           r.id AS revision_id, r.quotation_id, r.currency
    FROM %[1]s.quotation_revisions r
    WHERE r.quotation_id = ANY($3)
    ORDER BY r.quotation_id, r.revision_number DESC
),
leaves AS (
    SELECT lr.quotation_id, i.item_no, NULL::integer AS sub_no,
           i.qty::numeric * 10000 AS quoted_qty_scaled,
           i.marginalized_unit_price::numeric AS quoted_unit_price_minor,
           i.unit_price::numeric AS cost_unit_minor,
           i.description
    FROM latest_revision lr
    JOIN %[1]s.quotation_revision_items i ON i.revision_id = lr.revision_id
    WHERE lr.currency = $2
      AND NOT (i.show_sub_prices AND EXISTS (
          SELECT 1 FROM %[1]s.quotation_revision_sub_items s
          WHERE s.revision_item_id = i.id
      ))
    UNION ALL
    SELECT lr.quotation_id, i.item_no, s.sub_no,
           s.qty::numeric * 10000,
           s.unit_price::numeric,
           s.unit_cost::numeric,
           s.description
    FROM latest_revision lr
    JOIN %[1]s.quotation_revision_items i ON i.revision_id = lr.revision_id
    JOIN %[1]s.quotation_revision_sub_items s ON s.revision_item_id = i.id
    WHERE lr.currency = $2 AND i.show_sub_prices
),
bought AS (
    SELECT a.quotation_id, a.quotation_item_no AS item_no,
           COALESCE(a.quotation_sub_no, 0) AS sub_no,
           SUM(a.quantity_scaled::numeric) AS bought_qty_scaled,
           SUM(a.revenue_minor::numeric) AS revenue_minor,
           COALESCE(SUM(a.revenue_minor::numeric)
                    FILTER (WHERE a.cost_unit_minor IS NOT NULL), 0)
               AS costed_revenue_minor,
           COALESCE(SUM(a.quantity_scaled::numeric * a.cost_unit_minor), 0)
               AS capital_scaled,
           COALESCE(SUM(ROUND(a.quantity_scaled::numeric
                              * a.quoted_unit_price_minor / 10000)), 0)
               AS bought_at_quoted_minor
    FROM %[1]s.purchase_order_allocations a
    JOIN %[1]s.purchase_orders po ON po.id = a.purchase_order_id
    WHERE po.org_id = $1 AND po.state = 'saved' AND po.currency = $2
      AND a.quotation_id = ANY($3)
    GROUP BY a.quotation_id, a.quotation_item_no, COALESCE(a.quotation_sub_no, 0)
)
SELECT COALESCE(l.quotation_id, b.quotation_id) AS quotation_id,
       COALESCE(l.item_no, b.item_no) AS item_no,
       COALESCE(l.sub_no, b.sub_no, 0) AS sub_no,
       l.quoted_qty_scaled, l.quoted_unit_price_minor, l.cost_unit_minor,
       COALESCE(l.description, ''),
       COALESCE(b.bought_qty_scaled, 0)::bigint,
       COALESCE(b.revenue_minor, 0)::bigint,
       COALESCE(b.costed_revenue_minor, 0)::bigint,
       COALESCE(b.capital_scaled, 0)::bigint,
       COALESCE(b.bought_at_quoted_minor, 0)::bigint,
       (l.quoted_qty_scaled IS NULL) AS removed
FROM leaves l
FULL OUTER JOIN bought b
  ON b.quotation_id = l.quotation_id
 AND b.item_no = l.item_no
 AND b.sub_no = COALESCE(l.sub_no, 0)
ORDER BY quotation_id, item_no, sub_no
`, schema)
}

// purchaseOrderFulfillmentContributionsSQL lists the saved PO lines behind each
// leaf. The group title and the line description ride along because a removed
// leaf has no latest quotation row to read them from.
func purchaseOrderFulfillmentContributionsSQL(schema string) string {
	return fmt.Sprintf(`
SELECT a.quotation_id, a.quotation_item_no,
       COALESCE(a.quotation_sub_no, 0),
       po.id, po.po_number, po.state, po.received_date,
       a.quantity_scaled,
       COALESCE(g.title, ''),
       poi.description
FROM %[1]s.purchase_order_allocations a
JOIN %[1]s.purchase_orders po ON po.id = a.purchase_order_id
JOIN %[1]s.purchase_order_items poi ON poi.id = a.purchase_order_item_id
LEFT JOIN %[1]s.purchase_order_item_groups g ON g.id = poi.aggregate_group_id
WHERE po.org_id = $1 AND po.state = 'saved' AND po.currency = $2
  AND a.quotation_id = ANY($3)
ORDER BY po.received_date, po.id, poi.line_number
`, schema)
}

// purchaseOrderFulfillmentExtrasSQL lists explicit extra expense lines
// separately from every quantity. A draft extra is not an actual expense yet, so
// only saved records are read.
func purchaseOrderFulfillmentExtrasSQL(schema string) string {
	return fmt.Sprintf(`
SELECT po.quotation_id, po.id, po.po_number, poi.line_number,
       poi.description, poi.quantity_scaled, poi.uom, poi.net_minor,
       COALESCE(g.title, '')
FROM %[1]s.purchase_order_items poi
JOIN %[1]s.purchase_orders po ON po.id = poi.purchase_order_id
LEFT JOIN %[1]s.purchase_order_item_groups g ON g.id = poi.aggregate_group_id
WHERE po.org_id = $1 AND po.state = 'saved' AND po.currency = $2
  AND poi.line_kind = 'extra'
  AND po.quotation_id = ANY($3)
ORDER BY po.received_date, po.id, poi.line_number
`, schema)
}
