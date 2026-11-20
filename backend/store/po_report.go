package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"time"

	"vera-app/backend/poreport"
)

// Issue #629: the purchase-order performance report. One organization-scoped
// aggregate answers what the business asks about its saved orders: how many
// arrived in a period, what they were worth, what they cost, and what margin
// they made, split by calendar bucket and business category.
//
// # What the query owns
//
// The report is read-only and nothing is materialized. A single statement
// reads the saved population, resolves each PO line's cost against the exact
// pinned source revision and currency, and returns every actual aggregate the
// response needs: totals, calendar buckets, categories, the category series,
// the projection history and the facet flags. No raw purchase order or line
// row reaches Go, so a report cannot turn into an N+1 read, and the arithmetic
// stays in Postgres numeric where a cost times a scaled quantity cannot wrap.
//
// # The formulas, once
//
//	amount    = subtotal_minor - discount_minor + adjustment_minor  (ex PPN)
//	costed    = SUM(net_minor) of lines whose exact source cost resolved
//	capital   = round_half_away(SUM(cost_unit * quantity_scaled) / 10000)
//	            rounded once per PO, left unrounded across a PO's lines
//	margin    = costed - capital                                   (signed)
//	uncosted  = amount - costed
//
// Capital's division happens after the lines are summed, so two half-unit
// lines round together rather than becoming zeros separately. The per-PO
// rounded capital is then summed into a period or category, which is why the
// table foots. Cost is the pinned revision's cost field only: the line's
// pre-margin unit_price for a matched item, the sub-item's unit_cost for a
// matched sub-item, multiplied by the PO's quantity (never the quotation's).
// A source whose revision currency differs from the PO's is not a cost: the
// report never converts one currency into another.
//
// # Unknown cost is disclosed, never profit
//
// A line whose cost does not resolve contributes revenue to `amount` but no
// money to `costed` or `capital`; the difference is `uncosted`. A deleted or
// unreachable source therefore keeps its revenue and reports zero margin,
// because zero capital and zero costed means zero known margin -- not the
// whole amount as profit. Nothing is allocated proportionally.
//
// # Retired whole-order adjustments
//
// Migration 031's whole-order discount and signed adjustment are retired
// inputs: every current save writes zero (#573), and production was verified
// to hold only zeros. The amount formula still adds them, because a restored
// backup must not silently change what an old order was worth. If a nonzero
// one is present the snapshot reports `LegacyAdjustmentPoCount`, and the API
// attaches an explicit legacy_order_adjustment note; the residual is never
// misdescribed as unknown supplier cost.

// PurchaseOrderReportSnapshot is the complete actual-report snapshot from one
// SQL statement. The domain projection is deliberately absent: the API caller
// derives it from History with the same as-of instant, so the actual figures and
// the forecast share one clock.
type PurchaseOrderReportSnapshot struct {
	// AsOf is the company's date for the report (midnight UTC of the WIB
	// calendar day), not the server's local date. Partial and in-progress
	// buckets and the projection basis are all computed against it.
	AsOf time.Time
	// Totals carries ratios derived from the exact aggregate numerators.
	Totals poreport.Metrics
	// Periods are continuous calendar buckets covering the requested range,
	// including zero rows for gaps.
	Periods []poreport.Period
	// Categories are the fixed code order for All, or the selected category.
	Categories []poreport.Category
	// CategorySeries carries each category's margin percentage aligned to
	// Periods. It is present for an All report only, and an all-null series is
	// omitted.
	CategorySeries []poreport.CategorySeries
	// History is the continuous monthly history before the complete-month
	// cutoff. Zeros are observations, not invented pre-Vera history. It is
	// empty when HistoryOutOfRange is true: a truncated sample must never reach
	// the projection.
	History []poreport.HistoryMonth
	// HistoryOutOfRange reports that a stored history aggregate could not be
	// represented exactly. The actual report is unaffected; the caller renders
	// the forecast unavailable/value_out_of_range from HistoryBasis instead of
	// projecting a corrupted sample.
	HistoryOutOfRange bool
	// HistoryBasis is the complete-month window and sample count behind the
	// history. It is populated even when HistoryOutOfRange is true, so the
	// unavailable forecast can state the sample it would have used.
	HistoryBasis poreport.ProjectionBasis
	// AvailableCurrencies is date-scoped and ignores the selected currency and
	// category. The selected currency is always present.
	AvailableCurrencies []string
	// HasUnassigned is date- and currency-scoped and ignores the category
	// filter, so the All option can state whether an unassigned row exists.
	HasUnassigned bool
	// SourceMissingPoCount is the number of POs in the filtered population
	// whose source quotation was deleted.
	SourceMissingPoCount int64
	// LegacyAdjustmentPoCount is the number of filtered POs carrying a retired
	// nonzero whole-order discount or adjustment.
	LegacyAdjustmentPoCount int64
}

// reportCategoryOrder is the fixed wire order of an All category breakdown.
// Unassigned comes last so every real business line reads before the bucket
// that exists for records saved before the category feature.
var reportCategoryOrder = []string{
	poreport.CategoryFm,
	poreport.CategoryFab,
	poreport.CategorySupplyGeneral,
	poreport.CategoryOthers,
	poreport.CategoryUnassigned,
}

// reportCategoryLabels is the workbook vocabulary the screen, the chart legend
// and the later PDF all print. The code stays the stable key; the label is the
// human wording.
var reportCategoryLabels = map[string]string{
	poreport.CategoryFm:            "Facility Management",
	poreport.CategoryFab:           "Fabrication",
	poreport.CategorySupplyGeneral: "Goods Supply & General",
	poreport.CategoryOthers:        "Others",
	poreport.CategoryUnassigned:    "Unassigned",
}

// ReportPurchaseOrders computes the actual purchase-order report snapshot for
// one normalized query. now is the caller's clock; the store resolves the
// company's WIB calendar day from it once, so period progress and the
// projection basis cannot disagree.
//
// The query is normalized again here rather than trusted: the desktop reaches
// this through the API, but the store is also called directly by tests and by
// any future server-side caller. An invalid query is ErrInvalid, which the API
// maps to 400; an aggregate outside the exact JSON range is ErrValueOutOfRange,
// which the API maps to 422.
func (s *Store) ReportPurchaseOrders(ctx context.Context, query poreport.Query, now time.Time) (PurchaseOrderReportSnapshot, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	query, err = poreport.NormalizeQuery(query)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	from, err := time.Parse(time.DateOnly, query.From)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, fmt.Errorf("%w: from %q", poreport.ErrInvalid, query.From)
	}
	to, err := time.Parse(time.DateOnly, query.To)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, fmt.Errorf("%w: to %q", poreport.ErrInvalid, query.To)
	}
	asOf := companyToday(now)
	historyTo := reportHistoryCutoff(to, asOf)

	var totalsRaw, periodsRaw, categoriesRaw, categoryPeriodsRaw, historyRaw, currenciesRaw []byte
	var hasUnassigned bool
	if err := pool.QueryRow(ctx, purchaseOrderReportSQL(s.configSchema()),
		org.ID,
		query.Currency,
		query.From,
		query.To,
		query.Category,
		historyTo.Format(time.DateOnly),
		query.Bucket,
	).Scan(
		&totalsRaw,
		&periodsRaw,
		&categoriesRaw,
		&categoryPeriodsRaw,
		&historyRaw,
		&currenciesRaw,
		&hasUnassigned,
	); err != nil {
		return PurchaseOrderReportSnapshot{}, fmt.Errorf("read purchase order report: %w", err)
	}

	var totalsRow reportTotalsJSON
	if err := json.Unmarshal(totalsRaw, &totalsRow); err != nil {
		return PurchaseOrderReportSnapshot{}, fmt.Errorf("decode purchase order report totals: %w", err)
	}
	amount, err := reportExactNumber(totalsRow.AmountMinor, "total amount")
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	costed, err := reportExactNumber(totalsRow.CostedMinor, "total costed amount")
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	capital, err := reportExactNumber(totalsRow.CapitalMinor, "total capital")
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	totals, err := reportMetrics(totalsRow.PoCount, amount, costed, capital)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, fmt.Errorf("total metrics: %w", err)
	}
	if err := poreport.CheckExact(totalsRow.SourceMissingPoCount); err != nil {
		return PurchaseOrderReportSnapshot{}, fmt.Errorf("source-missing count: %w", err)
	}
	if err := poreport.CheckExact(totalsRow.LegacyAdjustmentPoCount); err != nil {
		return PurchaseOrderReportSnapshot{}, fmt.Errorf("legacy adjustment count: %w", err)
	}

	periods, err := reportPeriods(periodsRaw, from, to, asOf, query.Bucket)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	categories, err := reportCategories(categoriesRaw, query.Category)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	series, err := reportCategorySeries(categoryPeriodsRaw, periods, query.Category, categories)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	history, err := reportHistory(historyRaw, historyTo, asOf)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}
	currencies, err := reportCurrencies(currenciesRaw, query.Currency)
	if err != nil {
		return PurchaseOrderReportSnapshot{}, err
	}

	return PurchaseOrderReportSnapshot{
		AsOf:                    asOf,
		Totals:                  totals,
		Periods:                 periods,
		Categories:              categories,
		CategorySeries:          series,
		History:                 history.months,
		HistoryOutOfRange:       history.outOfRange,
		HistoryBasis:            history.basis,
		AvailableCurrencies:     currencies,
		HasUnassigned:           hasUnassigned,
		SourceMissingPoCount:    totalsRow.SourceMissingPoCount,
		LegacyAdjustmentPoCount: totalsRow.LegacyAdjustmentPoCount,
	}, nil
}

// reportTotalsJSON is the exact totals object Postgres returns. Every money
// field is a json.Number because the SQL sum is numeric: a 10,000-PO report can
// exceed int64's comfortable range only after the fact, and the caller must see
// a typed range refusal rather than a wrapped figure.
type reportTotalsJSON struct {
	PoCount                 int64       `json:"poCount"`
	AmountMinor             json.Number `json:"amountMinor"`
	CostedMinor             json.Number `json:"costedMinor"`
	CapitalMinor            json.Number `json:"capitalMinor"`
	SourceMissingPoCount    int64       `json:"sourceMissingPoCount"`
	LegacyAdjustmentPoCount int64       `json:"legacyAdjustmentPoCount"`
}

type reportBucketJSON struct {
	PeriodStart  string      `json:"periodStart"`
	PoCount      int64       `json:"poCount"`
	AmountMinor  json.Number `json:"amountMinor"`
	CostedMinor  json.Number `json:"costedMinor"`
	CapitalMinor json.Number `json:"capitalMinor"`
}

type reportCategoryJSON struct {
	BusinessCategory string      `json:"businessCategory"`
	PoCount          int64       `json:"poCount"`
	AmountMinor      json.Number `json:"amountMinor"`
	CostedMinor      json.Number `json:"costedMinor"`
	CapitalMinor     json.Number `json:"capitalMinor"`
}

type reportCategoryPeriodJSON struct {
	PeriodStart      string      `json:"periodStart"`
	BusinessCategory string      `json:"businessCategory"`
	MarginMinor      json.Number `json:"marginMinor"`
	CapitalMinor     json.Number `json:"capitalMinor"`
}

type reportHistoryJSON struct {
	MonthStart   string      `json:"monthStart"`
	PoCount      int64       `json:"poCount"`
	AmountMinor  json.Number `json:"amountMinor"`
	CostedMinor  json.Number `json:"costedMinor"`
	CapitalMinor json.Number `json:"capitalMinor"`
}

// reportExactNumber reads one JSON numeric aggregate as an int64 that a
// JavaScript number can name exactly. big.Rat accepts the exact decimal text
// Postgres emits, rejects a fractional result, and keeps the bound check in the
// domain's one CheckExact rule.
func reportExactNumber(value json.Number, field string) (int64, error) {
	if value == "" {
		return 0, fmt.Errorf("%w: %s is missing", poreport.ErrValueOutOfRange, field)
	}
	rational, ok := new(big.Rat).SetString(value.String())
	if !ok || !rational.IsInt() {
		return 0, fmt.Errorf("%w: %s = %s is not an integer",
			poreport.ErrValueOutOfRange, field, value)
	}
	if !rational.Num().IsInt64() {
		return 0, fmt.Errorf("%w: %s = %s does not fit an int64",
			poreport.ErrValueOutOfRange, field, value)
	}
	result := rational.Num().Int64()
	if err := poreport.CheckExact(result); err != nil {
		return 0, fmt.Errorf("%s: %w", field, err)
	}
	return result, nil
}

// reportExactBigInt is reportExactNumber for a derived value that was computed
// exactly in big.Int first. It refuses a value whose int64 or exact-JSON bound
// would lose a digit rather than letting a later projection see a truncated one.
func reportExactBigInt(value *big.Int, field string) (int64, error) {
	if !value.IsInt64() {
		return 0, fmt.Errorf("%w: %s = %s does not fit an int64",
			poreport.ErrValueOutOfRange, field, value)
	}
	result := value.Int64()
	if err := poreport.CheckExact(result); err != nil {
		return 0, fmt.Errorf("%s: %w", field, err)
	}
	return result, nil
}

// reportMetrics derives the full metrics shape from exact aggregate numerators.
// Ratios are computed from the aggregate pair (weighted), never averaged from
// row percentages, and are nil rather than zero when the denominator is not
// positive.
// reportOptionalExact is reportExactNumber for a grouped row that legitimately
// does not exist: a gap month or a category with no orders is an actual zero,
// not a missing aggregate. The totals object never uses this helper, so a broken
// totals shape still fails rather than silently reading zero.
func reportOptionalExact(value json.Number, field string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return reportExactNumber(value, field)
}

func reportMetrics(poCount, amount, costed, capital int64) (poreport.Metrics, error) {
	for name, value := range map[string]int64{
		"po count": poCount,
		"amount":   amount,
		"costed":   costed,
		"capital":  capital,
	} {
		if err := poreport.CheckExact(value); err != nil {
			return poreport.Metrics{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	margin := costed - capital
	uncosted := amount - costed
	if err := poreport.CheckExact(margin); err != nil {
		return poreport.Metrics{}, fmt.Errorf("margin: %w", err)
	}
	if err := poreport.CheckExact(uncosted); err != nil {
		return poreport.Metrics{}, fmt.Errorf("uncosted: %w", err)
	}
	marginBps, err := poreport.RatioBps(margin, capital)
	if err != nil {
		return poreport.Metrics{}, fmt.Errorf("margin percentage: %w", err)
	}
	coverageBps, err := poreport.RatioBps(costed, amount)
	if err != nil {
		return poreport.Metrics{}, fmt.Errorf("coverage percentage: %w", err)
	}
	return poreport.Metrics{
		PoCount:       poCount,
		AmountMinor:   amount,
		CostedMinor:   costed,
		UncostedMinor: uncosted,
		CapitalMinor:  capital,
		MarginMinor:   margin,
		MarginBps:     marginBps,
		CoverageBps:   coverageBps,
	}, nil
}

// reportPeriods turns the grouped rows into continuous ordered calendar
// buckets. A gap in the data is an actual zero period, not a missing one: the
// table and chart must foot across the whole requested range. Partial marks a
// bucket clipped by the report's half-open range; InProgress marks the bucket
// containing the as-of date, which is the one comparison a reader must not make
// against a complete period.
func reportPeriods(raw []byte, from, to, asOf time.Time, bucket string) ([]poreport.Period, error) {
	var rows []reportBucketJSON
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("decode purchase order report periods: %w", err)
	}
	byStart := make(map[string]reportBucketJSON, len(rows))
	for _, row := range rows {
		byStart[row.PeriodStart] = row
	}

	starts := reportPeriodStarts(from, to, bucket)
	periods := make([]poreport.Period, 0, len(starts))
	for _, start := range starts {
		end := reportNextPeriod(start, bucket)
		row := byStart[start.Format(time.DateOnly)]
		amount, err := reportOptionalExact(row.AmountMinor, "period "+start.Format(time.DateOnly)+" amount")
		if err != nil {
			return nil, err
		}
		costed, err := reportOptionalExact(row.CostedMinor, "period "+start.Format(time.DateOnly)+" costed amount")
		if err != nil {
			return nil, err
		}
		capital, err := reportOptionalExact(row.CapitalMinor, "period "+start.Format(time.DateOnly)+" capital")
		if err != nil {
			return nil, err
		}
		metrics, err := reportMetrics(row.PoCount, amount, costed, capital)
		if err != nil {
			return nil, fmt.Errorf("period %s: %w", start.Format(time.DateOnly), err)
		}
		periods = append(periods, poreport.Period{
			PeriodStart: start.Format(time.DateOnly),
			Partial:     start.Before(from) || to.Before(end),
			InProgress:  !asOf.Before(start) && asOf.Before(end),
			Metrics:     metrics,
		})
	}
	return periods, nil
}

// reportCategories materializes the breakdown in its fixed wire order. All
// carries the four business codes and then Unassigned, including zero rows, so
// a category that had no orders in the range still occupies its axis slot; a
// filtered report carries exactly the selected row.
func reportCategories(raw []byte, filter string) ([]poreport.Category, error) {
	var rows []reportCategoryJSON
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("decode purchase order report categories: %w", err)
	}
	byCode := make(map[string]reportCategoryJSON, len(rows))
	for _, row := range rows {
		byCode[row.BusinessCategory] = row
	}

	order := reportCategoryOrder
	if filter != poreport.CategoryAll {
		order = []string{filter}
	}
	categories := make([]poreport.Category, 0, len(order))
	for _, code := range order {
		row := byCode[code]
		amount, err := reportOptionalExact(row.AmountMinor, "category "+code+" amount")
		if err != nil {
			return nil, err
		}
		costed, err := reportOptionalExact(row.CostedMinor, "category "+code+" costed amount")
		if err != nil {
			return nil, err
		}
		capital, err := reportOptionalExact(row.CapitalMinor, "category "+code+" capital")
		if err != nil {
			return nil, err
		}
		metrics, err := reportMetrics(row.PoCount, amount, costed, capital)
		if err != nil {
			return nil, fmt.Errorf("category %s: %w", code, err)
		}
		categories = append(categories, poreport.Category{
			BusinessCategory: code,
			Label:            reportCategoryLabels[code],
			Metrics:          metrics,
		})
	}
	return categories, nil
}

// reportCategorySeries aligns each category's margin percentage to the actual
// periods. It only exists for All: a filtered report is already one line, and
// repeating it as a series would draw the same value twice. A series whose
// every period has zero capital is omitted entirely; a period with zero capital
// inside a live series stays null, which is "unknown", not "0%".
func reportCategorySeries(raw []byte, periods []poreport.Period, filter string, categories []poreport.Category) ([]poreport.CategorySeries, error) {
	if filter != poreport.CategoryAll {
		return []poreport.CategorySeries{}, nil
	}
	var rows []reportCategoryPeriodJSON
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("decode purchase order report category series: %w", err)
	}
	type pair struct {
		margin  int64
		capital int64
	}
	byCategory := make(map[string]map[string]pair, len(categories))
	for _, row := range rows {
		margin, err := reportExactNumber(row.MarginMinor, "category series "+row.BusinessCategory+" margin")
		if err != nil {
			return nil, err
		}
		capital, err := reportExactNumber(row.CapitalMinor, "category series "+row.BusinessCategory+" capital")
		if err != nil {
			return nil, err
		}
		if byCategory[row.BusinessCategory] == nil {
			byCategory[row.BusinessCategory] = make(map[string]pair)
		}
		byCategory[row.BusinessCategory][row.PeriodStart] = pair{margin: margin, capital: capital}
	}

	periodStarts := make([]string, 0, len(periods))
	for _, period := range periods {
		periodStarts = append(periodStarts, period.PeriodStart)
	}
	series := make([]poreport.CategorySeries, 0, len(categories))
	for _, category := range categories {
		values := make([]*int64, 0, len(periodStarts))
		present := false
		for _, start := range periodStarts {
			entry := byCategory[category.BusinessCategory][start]
			bps, err := poreport.RatioBps(entry.margin, entry.capital)
			if err != nil {
				return nil, fmt.Errorf("category series %s at %s: %w", category.BusinessCategory, start, err)
			}
			if bps != nil {
				present = true
			}
			values = append(values, bps)
		}
		if !present {
			continue
		}
		series = append(series, poreport.CategorySeries{
			BusinessCategory: category.BusinessCategory,
			MarginBps:        values,
		})
	}
	return series, nil
}

// reportHistorySample is the projection input plus the facts that survive an
// unrepresentable aggregate. The basis is built from the month labels and
// counts, never from money, so it stays truthful when a stored total is outside
// the exact JSON range.
type reportHistorySample struct {
	months     []poreport.HistoryMonth
	basis      poreport.ProjectionBasis
	outOfRange bool
}

// reportHistory turns grouped history months into the continuous sequence the
// domain projection consumes: from the first observed month (an observed month
// counts even if Vera's data began mid-month) through the last complete month
// before the cutoff. Months with no orders stay present as zeros. Nothing
// earlier than the first observed month is invented, because pre-Vera history
// is not evidence of zero sales.
//
// Only the fields Project reads are populated. History is not a wire object, so
// its margin and percentages are deliberately not computed: a ratio overflow on
// an unused value must never fail an actual report. When a money value itself is
// outside the exact range, the sample is withheld and the basis is returned with
// outOfRange set, so the caller can mark only the forecast unavailable. Values
// are never clamped or zeroed and then projected.
func reportHistory(raw []byte, historyTo, asOf time.Time) (reportHistorySample, error) {
	var rows []reportHistoryJSON
	if err := json.Unmarshal(raw, &rows); err != nil {
		return reportHistorySample{}, fmt.Errorf("decode purchase order report history: %w", err)
	}
	if len(rows) == 0 {
		return reportHistorySample{months: []poreport.HistoryMonth{}}, nil
	}
	byMonth := make(map[string]reportHistoryJSON, len(rows))
	first := rows[0].MonthStart
	count := new(big.Int)
	for _, row := range rows {
		byMonth[row.MonthStart] = row
		if row.MonthStart < first {
			first = row.MonthStart
		}
		count.Add(count, big.NewInt(row.PoCount))
	}
	start, err := time.Parse(time.DateOnly, first)
	if err != nil {
		return reportHistorySample{}, fmt.Errorf("%w: history month %q", poreport.ErrValueOutOfRange, first)
	}
	last := historyTo.AddDate(0, -1, 0)
	if start.After(last) {
		return reportHistorySample{months: []poreport.HistoryMonth{}}, nil
	}

	basis := poreport.ProjectionBasis{
		HistoryFrom: first,
		HistoryTo:   historyTo.Format(time.DateOnly),
		AsOf:        asOf.Format(time.DateOnly),
	}
	for month := start; !month.After(last); month = month.AddDate(0, 1, 0) {
		basis.CompleteMonths++
	}
	if count.IsInt64() {
		if err := poreport.CheckExact(count.Int64()); err == nil {
			basis.PoCount = count.Int64()
		} else {
			return reportHistorySample{basis: basis, outOfRange: true}, nil
		}
	} else {
		return reportHistorySample{basis: basis, outOfRange: true}, nil
	}

	history := make([]poreport.HistoryMonth, 0, basis.CompleteMonths)
	for month := start; !month.After(last); month = month.AddDate(0, 1, 0) {
		key := month.Format(time.DateOnly)
		row := byMonth[key]
		if err := poreport.CheckExact(row.PoCount); err != nil {
			return reportHistorySample{basis: basis, outOfRange: true}, nil
		}
		amount, err := reportOptionalExact(row.AmountMinor, "history "+key+" amount")
		if err != nil {
			return reportHistorySample{basis: basis, outOfRange: true}, nil
		}
		costed, err := reportOptionalExact(row.CostedMinor, "history "+key+" costed amount")
		if err != nil {
			return reportHistorySample{basis: basis, outOfRange: true}, nil
		}
		capital, err := reportOptionalExact(row.CapitalMinor, "history "+key+" capital")
		if err != nil {
			return reportHistorySample{basis: basis, outOfRange: true}, nil
		}
		uncosted, err := reportExactBigInt(
			new(big.Int).Sub(big.NewInt(amount), big.NewInt(costed)),
			"history "+key+" uncosted amount")
		if err != nil {
			return reportHistorySample{basis: basis, outOfRange: true}, nil
		}
		history = append(history, poreport.HistoryMonth{
			MonthStart: key,
			Metrics: poreport.Metrics{
				PoCount:       row.PoCount,
				AmountMinor:   amount,
				CostedMinor:   costed,
				UncostedMinor: uncosted,
				CapitalMinor:  capital,
			},
		})
	}
	return reportHistorySample{months: history, basis: basis}, nil
}

// reportCurrencies decodes the date-scoped facet and guarantees the selected
// currency is offered even when no saved order in the range used it. The
// selector must never lose the currency currently on screen.
func reportCurrencies(raw []byte, selected string) ([]string, error) {
	var currencies []string
	if err := json.Unmarshal(raw, &currencies); err != nil {
		return nil, fmt.Errorf("decode purchase order report currencies: %w", err)
	}
	present := false
	for _, code := range currencies {
		if code == selected {
			present = true
			break
		}
	}
	if !present {
		currencies = append(currencies, selected)
	}
	sort.Strings(currencies)
	if currencies == nil {
		currencies = []string{}
	}
	return currencies, nil
}

// reportHistoryCutoff is the exclusive first day of the first month that is not
// complete history: the earlier of the report's final month and the as-of
// month. Everything before it is a complete month for projection purposes; the
// current month never is, even when the report range ends later.
func reportHistoryCutoff(to, asOf time.Time) time.Time {
	reportMonth := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	asOfMonth := time.Date(asOf.Year(), asOf.Month(), 1, 0, 0, 0, 0, time.UTC)
	if asOfMonth.Before(reportMonth) {
		return asOfMonth
	}
	return reportMonth
}

// reportPeriodStarts lists every calendar bucket start the half-open range
// touches, in order. The final included day is To-1, so a range ending exactly
// on a bucket boundary does not add the empty bucket after it.
func reportPeriodStarts(from, to time.Time, bucket string) []time.Time {
	lastIncluded := to.AddDate(0, 0, -1)
	start := reportBucketStart(from, bucket)
	last := reportBucketStart(lastIncluded, bucket)
	starts := make([]time.Time, 0, 12)
	for current := start; !current.After(last); current = reportNextPeriod(current, bucket) {
		starts = append(starts, current)
	}
	return starts
}

func reportBucketStart(date time.Time, bucket string) time.Time {
	year, month := date.Year(), date.Month()
	switch bucket {
	case poreport.BucketYear:
		month = time.January
	case poreport.BucketQuarter:
		month = time.Month((int(month)-1)/3*3 + 1)
	}
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}

func reportNextPeriod(start time.Time, bucket string) time.Time {
	switch bucket {
	case poreport.BucketQuarter:
		return start.AddDate(0, 3, 0)
	case poreport.BucketYear:
		return start.AddDate(1, 0, 0)
	default:
		return start.AddDate(0, 1, 0)
	}
}

// purchaseOrderReportSQL is the one aggregate statement behind the report. It
// returns seven values in one row: the exact totals object, the period,
// category, category-period and history arrays, the date-scoped currency facet,
// and the unassigned flag. Both the actual population and the projection
// history share the same cost resolution, so the two can never disagree about
// what a PO cost.
//
// The actual figures come from one CUBE pass: the grand total, the period rows,
// the category rows and the category-period series are four grouping sets over
// the same scanned rows, so four aggregates no longer rescan a materialized
// population. The currency facet and the unassigned flag share one grouped scan
// as well; the flag is category-independent by contract, which is why it can
// ride with the currency rollup.
func purchaseOrderReportSQL(schema string) string {
	return fmt.Sprintf(`
WITH base_population AS (
    SELECT po.id, po.received_date, po.business_category,
           po.subtotal_minor, po.discount_minor, po.adjustment_minor,
           po.quotation_id, po.source_revision, po.currency,
           (po.received_date >= $3::date AND po.received_date < $4::date) AS is_actual,
           (po.received_date < $6::date) AS is_history
    FROM %[1]s.purchase_orders po
    WHERE po.org_id = $1
      AND po.state = 'saved'
      AND po.currency = $2
      AND po.received_date < $4::date
      AND (
          $5 = 'all'
          OR ($5 = 'UNASSIGNED' AND po.business_category = '')
          OR po.business_category = $5
      )
),
po_metrics AS (
    SELECT po.id,
           po.received_date,
           po.business_category,
           po.subtotal_minor::numeric - po.discount_minor + po.adjustment_minor AS amount,
           COALESCE(SUM(poi.net_minor::numeric)
                    FILTER (WHERE resolved.cost_unit IS NOT NULL), 0) AS costed,
           ROUND(COALESCE(SUM(poi.quantity_scaled::numeric * resolved.cost_unit), 0) / 10000) AS capital,
           (po.discount_minor <> 0 OR po.adjustment_minor <> 0) AS legacy_adjustment,
           (po.quotation_id IS NULL) AS source_missing,
           po.is_actual,
           po.is_history
    FROM base_population po
    LEFT JOIN %[1]s.purchase_order_items poi ON poi.purchase_order_id = po.id
    LEFT JOIN %[1]s.quotation_revisions revision
           ON revision.quotation_id = po.quotation_id
          AND revision.revision_number = po.source_revision
          AND revision.currency = po.currency
    LEFT JOIN %[1]s.quotation_revision_items item
           ON item.revision_id = revision.id
          AND item.item_no = poi.quotation_item_no
    LEFT JOIN %[1]s.quotation_revision_sub_items sub
           ON sub.revision_item_id = item.id
          AND sub.sub_no = poi.quotation_sub_no
    CROSS JOIN LATERAL (
        SELECT CASE
            WHEN poi.quotation_item_no IS NULL THEN NULL
            WHEN poi.quotation_sub_no IS NULL THEN item.unit_price
            ELSE sub.unit_cost
        END AS cost_unit
    ) resolved
    GROUP BY po.id, po.received_date, po.business_category,
             po.subtotal_minor, po.discount_minor, po.adjustment_minor,
             po.quotation_id, po.source_revision, po.currency,
             po.is_actual, po.is_history
),
actual_rows AS (
    SELECT date_trunc(
               CASE $7
                   WHEN 'month' THEN 'month'
                   WHEN 'quarter' THEN 'quarter'
                   ELSE 'year'
               END,
               po.received_date
           )::date AS bucket_start,
           CASE WHEN po.business_category = '' THEN 'UNASSIGNED' ELSE po.business_category END AS category,
           po.amount, po.costed, po.capital, po.source_missing, po.legacy_adjustment
    FROM po_metrics po
    WHERE po.is_actual
),
actual_cube AS (
    -- GROUPING(bucket_start, category): 0 = both keys, 1 = period only,
    -- 2 = category only, 3 = the grand total. One CUBE pass replaces the four
    -- separate aggregates, so totals, periods, categories and the series all
    -- come from the same scanned rows.
    SELECT bucket_start,
           category,
           COUNT(*) AS po_count,
           SUM(amount) AS amount,
           SUM(costed) AS costed,
           SUM(capital) AS capital,
           COUNT(*) FILTER (WHERE source_missing) AS source_missing_count,
           COUNT(*) FILTER (WHERE legacy_adjustment) AS legacy_adjustment_count,
           GROUPING(bucket_start, category) AS grouping
    FROM actual_rows
    GROUP BY CUBE(bucket_start, category)
),
history_metrics AS (
    SELECT date_trunc('month', received_date)::date AS month_start,
           COUNT(*) AS po_count,
           SUM(amount) AS amount,
           SUM(costed) AS costed,
           SUM(capital) AS capital
    FROM po_metrics
    WHERE received_date < $6::date
    GROUP BY 1
),
facet_rollup AS (
    SELECT po.currency,
           bool_or(po.business_category = '') AS has_unassigned
    FROM %[1]s.purchase_orders po
    WHERE po.org_id = $1
      AND po.state = 'saved'
      AND po.received_date >= $3::date
      AND po.received_date < $4::date
    GROUP BY po.currency
)
SELECT
    (SELECT jsonb_build_object(
        'poCount', po_count,
        'amountMinor', COALESCE(amount, 0),
        'costedMinor', COALESCE(costed, 0),
        'capitalMinor', COALESCE(capital, 0),
        'sourceMissingPoCount', source_missing_count,
        'legacyAdjustmentPoCount', legacy_adjustment_count
     ) FROM actual_cube WHERE grouping = 3),
    (SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'periodStart', to_char(bucket_start, 'YYYY-MM-DD'),
        'poCount', po_count,
        'amountMinor', amount,
        'costedMinor', costed,
        'capitalMinor', capital
     ) ORDER BY bucket_start), '[]'::jsonb) FROM actual_cube WHERE grouping = 1),
    (SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'businessCategory', category,
        'poCount', po_count,
        'amountMinor', amount,
        'costedMinor', costed,
        'capitalMinor', capital
     ) ORDER BY category), '[]'::jsonb) FROM actual_cube WHERE grouping = 2),
    (SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'periodStart', to_char(bucket_start, 'YYYY-MM-DD'),
        'businessCategory', category,
        'marginMinor', costed - capital,
        'capitalMinor', capital
     ) ORDER BY bucket_start, category), '[]'::jsonb) FROM actual_cube WHERE grouping = 0),
    (SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'monthStart', to_char(month_start, 'YYYY-MM-DD'),
        'poCount', po_count,
        'amountMinor', amount,
        'costedMinor', costed,
        'capitalMinor', capital
     ) ORDER BY month_start), '[]'::jsonb) FROM history_metrics),
    (SELECT COALESCE(jsonb_agg(currency ORDER BY currency), '[]'::jsonb) FROM facet_rollup),
    (SELECT COALESCE((SELECT has_unassigned FROM facet_rollup WHERE currency = $2), false))
`, schema)
}
