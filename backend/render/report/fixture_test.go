package report

import (
	"os"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

func testFonts() *faces {
	f, err := loadFaces(os.DirFS("../../assets"))
	if err != nil {
		panic(err)
	}
	return f
}

func testReportRenderer(snapshot poreport.Report) (*reportRenderer, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return nil, err
	}
	loaded := testFonts()
	renderer := &reportRenderer{
		snapshot: snapshot,
		faces:    loaded,
		pal:      defaultPalette(),
		canvas:   newCanvas(loaded),
	}
	renderer.draw()
	return renderer, nil
}

func monthStart(year int, month time.Month) string {
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

func pointer(value int64) *int64 { return &value }

// fixtureCategoryCodes is the report's fixed category order. The same order is
// used by the store and by the rendered table.
var fixtureCategoryCodes = []string{
	poreport.CategoryFm, poreport.CategoryFab, poreport.CategorySupplyGeneral,
	poreport.CategoryOthers, poreport.CategoryUnassigned,
}

var fixtureCategoryLabels = []string{
	"Facility Management", "Fabrication", "Goods Supply & General", "Others", "Unassigned",
}

// The two weight vectors split one period's money across the five categories.
// Amount and uncosted share a vector so each cell's costed amount is the same
// share as the period's costed amount, which keeps uncosted from turning
// negative. Capital uses a different vector so a category's margin percentage
// is its own relationship, not a copy of the period's. Unassigned carries zero
// capital, which is what makes a null percentage a real zero-denominator case
// rather than a hand-placed dash.
var (
	fixtureValueWeights   = []int64{40, 30, 20, 8, 2}
	fixtureCapitalWeights = []int64{36, 31, 21, 12, 0}
)

// fixtureCell is one category's contribution to one period. It carries money
// only: counts are a separate, independent partition of the same population,
// so no fractional purchase order is invented to make a matrix add up.
type fixtureCell struct {
	amount   int64
	costed   int64
	uncosted int64
	capital  int64
	margin   int64
}

// fixtureGrid splits each period's money across the categories. Every row and
// every column of the resulting grid sums to the same totals, and every cell's
// costed, uncosted and margin obey the domain identities.
func fixtureGrid(periods []poreport.Metrics) [][]fixtureCell {
	grid := make([][]fixtureCell, len(periods))
	for periodIndex, period := range periods {
		amount := distribute(period.AmountMinor, fixtureValueWeights)
		uncosted := distribute(period.UncostedMinor, fixtureValueWeights)
		capital := distribute(period.CapitalMinor, fixtureCapitalWeights)
		cells := make([]fixtureCell, len(fixtureCategoryCodes))
		for categoryIndex := range fixtureCategoryCodes {
			costed := amount[categoryIndex] - uncosted[categoryIndex]
			cells[categoryIndex] = fixtureCell{
				amount:   amount[categoryIndex],
				costed:   costed,
				uncosted: uncosted[categoryIndex],
				capital:  capital[categoryIndex],
				margin:   costed - capital[categoryIndex],
			}
		}
		grid[periodIndex] = cells
	}
	return grid
}

func fixtureRatio(numerator, denominator int64) *int64 {
	ratio, err := poreport.RatioBps(numerator, denominator)
	if err != nil {
		panic(err)
	}
	return ratio
}

// fixtureMetrics builds the metrics for one row from its cells. Every ratio is
// computed from the row's own money with the same function the server uses; no
// fixture sets a percentage by hand.
func fixtureMetrics(poCount int64, cells []fixtureCell) poreport.Metrics {
	var amount, costed, uncosted, capital, margin int64
	for _, cell := range cells {
		amount += cell.amount
		costed += cell.costed
		uncosted += cell.uncosted
		capital += cell.capital
		margin += cell.margin
	}
	return poreport.Metrics{
		PoCount:       poCount,
		AmountMinor:   amount,
		CostedMinor:   costed,
		UncostedMinor: uncosted,
		CapitalMinor:  capital,
		MarginMinor:   margin,
		MarginBps:     fixtureRatio(margin, capital),
		CoverageBps:   fixtureRatio(costed, amount),
	}
}

func fixtureCategorySeries(grid [][]fixtureCell) []poreport.CategorySeries {
	series := make([]poreport.CategorySeries, 0, len(fixtureCategoryCodes))
	for categoryIndex, code := range fixtureCategoryCodes {
		points := make([]*int64, 0, len(grid))
		present := false
		for _, row := range grid {
			cell := row[categoryIndex]
			ratio := fixtureRatio(cell.margin, cell.capital)
			if ratio != nil {
				present = true
			}
			points = append(points, ratio)
		}
		// The store omits a series that is null in every period; an all-null
		// axis would claim a category was plotted when nothing was measured.
		if !present {
			continue
		}
		series = append(series, poreport.CategorySeries{BusinessCategory: code, MarginBps: points})
	}
	return series
}

func fixtureCategories(grid [][]fixtureCell, counts []int64) []poreport.Category {
	categories := make([]poreport.Category, 0, len(fixtureCategoryCodes))
	for categoryIndex, code := range fixtureCategoryCodes {
		cells := make([]fixtureCell, 0, len(grid))
		for _, row := range grid {
			cells = append(cells, row[categoryIndex])
		}
		categories = append(categories, poreport.Category{
			BusinessCategory: code,
			Label:            fixtureCategoryLabels[categoryIndex],
			Metrics:          fixtureMetrics(counts[categoryIndex], cells),
		})
	}
	return categories
}

// samplePeriodMetrics is the twelve actual months of the sample report. Nine
// months carry an ordinary 28.57% margin; March is discounted below cost;
// June has no resolved source cost at all, so its capital is zero and its
// margin percentage is genuinely unknown; December is the partial, in-progress
// month that also carries the odd amount and a second loss.
func samplePeriodMetrics() []poreport.Metrics {
	periods := make([]poreport.Metrics, 0, 12)
	for month := 1; month <= 12; month++ {
		metrics := poreport.Metrics{
			PoCount:       2,
			AmountMinor:   100_000_000,
			CostedMinor:   90_000_000,
			UncostedMinor: 10_000_000,
			CapitalMinor:  70_000_000,
			MarginMinor:   20_000_000,
		}
		switch month {
		case 3:
			// A month discounted below cost: capital exceeds the costed amount.
			metrics.CapitalMinor = 120_000_000
			metrics.MarginMinor = -30_000_000
		case 6:
			// Nothing in this month has a resolved source cost: no costed
			// amount and no capital, so the margin percentage is unknown.
			metrics.CostedMinor = 0
			metrics.UncostedMinor = 100_000_000
			metrics.CapitalMinor = 0
			metrics.MarginMinor = 0
		case 12:
			// The partial month carries the odd amount and the second loss.
			metrics.AmountMinor = 134_567_890
			metrics.CostedMinor = 10_000_000
			metrics.UncostedMinor = 124_567_890
			metrics.CapitalMinor = 60_000_000
			metrics.MarginMinor = -50_000_000
		}
		periods = append(periods, metrics)
	}
	return periods
}

// sampleReport is a complete report with every section populated: twelve
// actual months including a negative margin and an unknown percentage, five
// category rows, both projection series, and one disclosure note.
//
// Every period, category and total is built from one money grid, so the rows
// foot and every percentage is derived from its own numerator and denominator.
// The filter and the generated/as-of dates agree: the range ends inside
// December 2025, which is both the clipped (partial) month and the month that
// contains the as-of date, so the "IP" marker is a real fact.
func sampleReport() poreport.Report {
	base := samplePeriodMetrics()
	grid := fixtureGrid(base)

	periods := make([]poreport.Period, 0, len(base))
	for index, metrics := range base {
		periods = append(periods, poreport.Period{
			PeriodStart: monthStart(2025, time.Month(index+1)),
			Partial:     index == len(base)-1,
			InProgress:  index == len(base)-1,
			Metrics:     fixtureMetrics(metrics.PoCount, grid[index]),
		})
	}

	// Counts are their own partition: 8 + 6 + 5 + 3 + 2 = the same 24 POs the
	// twelve months carry.
	categoryCounts := []int64{8, 6, 5, 3, 2}
	categories := fixtureCategories(grid, categoryCounts)

	totalCells := make([]fixtureCell, 0, len(grid)*len(fixtureCategoryCodes))
	periodCount := int64(0)
	for _, metrics := range base {
		periodCount += metrics.PoCount
	}
	for _, row := range grid {
		totalCells = append(totalCells, row...)
	}
	totals := fixtureMetrics(periodCount, totalCells)

	monthly := make([]poreport.ProjectedBucket, 0, 12)
	for month := 1; month <= 12; month++ {
		monthly = append(monthly, poreport.ProjectedBucket{
			PeriodStart:     monthStart(2026, time.Month(month)),
			AmountLowMinor:  8_000_000,
			AmountBaseMinor: 10_000_000,
			AmountHighMinor: 13_000_000,
			CapitalMinor:    7_500_000,
			MarginMinor:     2_500_000,
			MarginBps:       fixtureRatio(2_500_000, 7_500_000),
			SampleSize:      6,
			Source:          poreport.SourceSameCalendarMonth,
		})
	}
	yearly := make([]poreport.ProjectedBucket, 0, 3)
	for year := 2026; year <= 2028; year++ {
		yearly = append(yearly, poreport.ProjectedBucket{
			PeriodStart:     monthStart(year, time.January),
			AmountLowMinor:  100_000_000,
			AmountBaseMinor: 120_000_000,
			AmountHighMinor: 150_000_000,
			CapitalMinor:    90_000_000,
			MarginMinor:     30_000_000,
			MarginBps:       fixtureRatio(30_000_000, 90_000_000),
			SampleSize:      12,
			Source:          poreport.SourceTrailingTwelveCompleteMonths,
		})
	}
	basis := poreport.ProjectionBasis{
		HistoryFrom: "2024-07-01", HistoryTo: "2025-12-01",
		CompleteMonths: 17, PoCount: 34, AsOf: "2025-12-20",
	}

	return poreport.Report{
		GeneratedAt: "2025-12-20T08:30:00Z",
		AsOf:        "2025-12-20",
		Filter: poreport.Query{
			From: "2025-01-01", To: "2025-12-20", Bucket: poreport.BucketMonth,
			Category: poreport.CategoryAll, Currency: "IDR",
		},
		Company: poreport.Company{
			Name:    "PT. Arcon Penta Persada",
			Address: "Komp. Ruko Legenda Gemilang, Blok D. No. 12, Baloi Permai, Batam",
		},
		Population: poreport.Population{
			AvailableCurrencies:  []string{"IDR", "USD"},
			HasUnassigned:        true,
			SourceMissingPoCount: 2,
		},
		Totals:         totals,
		Periods:        periods,
		Categories:     categories,
		CategorySeries: fixtureCategorySeries(grid),
		Projection: poreport.Projection{
			Monthly: poreport.ProjectionSeries{
				Status:  poreport.ProjectionStatusAvailable,
				Method:  poreport.MethodMonthly,
				Basis:   basis,
				Buckets: monthly,
			},
			Yearly: poreport.ProjectionSeries{
				Status:  poreport.ProjectionStatusAvailable,
				Method:  poreport.MethodYearly,
				Basis:   basis,
				Buckets: yearly,
			},
		},
		// The sample is a custom January-December 2025 range, so the
		// current-period projection is correctly none. The retired monthly and
		// yearly series stay populated to prove the PDF no longer prints them.
		CurrentProjection: unavailableCurrentProjection(
			poreport.ProjectionModeNone, poreport.ReasonNotCurrentPeriod,
			poreport.MethodCurrentNone, poreport.CurrentProjectionBasis{AsOf: "2025-12-20"}),
		Notes: []poreport.Note{{
			Code: "legacy_order_adjustment",
			Text: "Some purchase orders in this range carry a retired whole-order discount or adjustment. " +
				"The amount includes it, but it is not supplier cost and no margin is inferred from it.",
		}},
	}
}

func unavailableProjectionReport() poreport.Report {
	snapshot := sampleReport()
	basis := poreport.ProjectionBasis{AsOf: "2025-12-20"}
	snapshot.Projection = poreport.Projection{
		Monthly: poreport.ProjectionSeries{
			Status: poreport.ProjectionStatusUnavailable,
			Reason: poreport.ReasonInsufficientHistory,
			Method: poreport.MethodMonthly,
			Basis:  basis,
		},
		Yearly: poreport.ProjectionSeries{
			Status: poreport.ProjectionStatusUnavailable,
			Reason: poreport.ReasonInsufficientHistory,
			Method: poreport.MethodYearly,
			Basis:  basis,
		},
	}
	return snapshot
}

func unavailableCurrentProjection(mode, reason, method string, basis poreport.CurrentProjectionBasis) poreport.CurrentProjection {
	return poreport.CurrentProjection{
		Mode:    mode,
		Status:  poreport.ProjectionStatusUnavailable,
		Reason:  reason,
		Method:  method,
		Basis:   basis,
		Buckets: []poreport.ProjectedBucket{},
	}
}

// dayReport is the current month bucketed by day: 31 daily actual periods
// (19 elapsed with orders, the rest future zero placeholders), a day-mode
// current projection for the 12 remaining days, and the retired projection
// series still populated so the PDF can be checked for not printing them.
func dayReport() poreport.Report {
	snapshot := sampleReport()
	snapshot.GeneratedAt = "2026-01-19T08:30:00Z"
	snapshot.AsOf = "2026-01-19"
	snapshot.Filter = poreport.Query{
		From: "2026-01-01", To: "2026-02-01", Bucket: poreport.BucketDay,
		Category: poreport.CategoryAll, Currency: "IDR",
	}

	base := make([]poreport.Metrics, 0, 31)
	for day := 1; day <= 31; day++ {
		metrics := poreport.Metrics{}
		if day <= 19 {
			metrics = poreport.Metrics{
				PoCount:       1,
				AmountMinor:   10_000_000,
				CostedMinor:   9_000_000,
				UncostedMinor: 1_000_000,
				CapitalMinor:  8_000_000,
				MarginMinor:   2_000_000,
			}
		}
		base = append(base, metrics)
	}
	grid := fixtureGrid(base)
	periods := make([]poreport.Period, 0, len(base))
	for index, metrics := range base {
		start := time.Date(2026, time.January, index+1, 0, 0, 0, 0, time.UTC)
		periods = append(periods, poreport.Period{
			PeriodStart: start.Format(time.DateOnly),
			InProgress:  index == 18,
			Metrics:     fixtureMetrics(metrics.PoCount, grid[index]),
		})
	}
	snapshot.Periods = periods

	categoryCounts := []int64{8, 5, 3, 2, 1}
	snapshot.Categories = fixtureCategories(grid, categoryCounts)
	snapshot.CategorySeries = fixtureCategorySeries(grid)

	totalCells := make([]fixtureCell, 0, len(grid)*len(fixtureCategoryCodes))
	for _, row := range grid {
		totalCells = append(totalCells, row...)
	}
	snapshot.Totals = fixtureMetrics(19, totalCells)

	future := make([]poreport.ProjectedBucket, 0, 12)
	for day := 20; day <= 31; day++ {
		future = append(future, poreport.ProjectedBucket{
			PeriodStart:     time.Date(2026, time.January, day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly),
			AmountLowMinor:  8_000_000,
			AmountBaseMinor: 10_000_000,
			AmountHighMinor: 12_000_000,
			CapitalMinor:    8_000_000,
			MarginMinor:     2_000_000,
			MarginBps:       fixtureRatio(2_000_000, 8_000_000),
			SampleSize:      19,
			Source:          poreport.SourceElapsedDays,
		})
	}
	snapshot.CurrentProjection = poreport.CurrentProjection{
		Mode:   poreport.ProjectionModeDay,
		Status: poreport.ProjectionStatusAvailable,
		Method: poreport.MethodCurrentDay,
		Basis: poreport.CurrentProjectionBasis{
			From: "2026-01-01", To: "2026-01-20", Units: 19, PoCount: 19, AsOf: "2026-01-19",
		},
		Buckets: future,
	}
	return snapshot
}

// currentYearReport is the current year bucketed by month: eleven actual
// months, a December ghost, and November carrying a stacked month-to-date plus
// remainder forecast.
func currentYearReport() poreport.Report {
	snapshot := sampleReport()
	snapshot.GeneratedAt = "2026-11-19T08:30:00Z"
	snapshot.AsOf = "2026-11-19"
	snapshot.Filter = poreport.Query{
		From: "2026-01-01", To: "2027-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}

	base := make([]poreport.Metrics, 0, 12)
	for month := 1; month <= 12; month++ {
		metrics := poreport.Metrics{}
		if month <= 11 {
			metrics = poreport.Metrics{
				PoCount:       2,
				AmountMinor:   100_000_000,
				CostedMinor:   90_000_000,
				UncostedMinor: 10_000_000,
				CapitalMinor:  70_000_000,
				MarginMinor:   20_000_000,
			}
		}
		base = append(base, metrics)
	}
	grid := fixtureGrid(base)
	periods := make([]poreport.Period, 0, len(base))
	for index, metrics := range base {
		periods = append(periods, poreport.Period{
			PeriodStart: monthStart(2026, time.Month(index+1)),
			InProgress:  index == 10,
			Metrics:     fixtureMetrics(metrics.PoCount, grid[index]),
		})
	}
	snapshot.Periods = periods
	snapshot.Categories = fixtureCategories(grid, []int64{8, 6, 4, 3, 1})
	snapshot.CategorySeries = fixtureCategorySeries(grid)

	totalCells := make([]fixtureCell, 0, len(grid)*len(fixtureCategoryCodes))
	for _, row := range grid {
		totalCells = append(totalCells, row...)
	}
	snapshot.Totals = fixtureMetrics(22, totalCells)

	currentMonth := periods[10].Metrics
	snapshot.CurrentProjection = poreport.CurrentProjection{
		Mode:   poreport.ProjectionModeMonth,
		Status: poreport.ProjectionStatusAvailable,
		Method: poreport.MethodCurrentMonth,
		Basis: poreport.CurrentProjectionBasis{
			From: "2025-12-01", To: "2026-11-01", Units: 11, PoCount: 22, AsOf: "2026-11-19",
		},
		Buckets: []poreport.ProjectedBucket{{
			PeriodStart:     "2026-12-01",
			AmountLowMinor:  80_000_000,
			AmountBaseMinor: 100_000_000,
			AmountHighMinor: 120_000_000,
			CapitalMinor:    70_000_000,
			MarginMinor:     30_000_000,
			MarginBps:       fixtureRatio(30_000_000, 70_000_000),
			SampleSize:      11,
			Source:          poreport.SourceTrailingCompleteMonths,
		}},
		CurrentMonth: &poreport.CurrentMonthProjection{
			Actual: currentMonth,
			Remainder: poreport.CurrentProjectionRemainder{
				LowMinor:     44_000_000,
				BaseMinor:    55_000_000,
				HighMinor:    66_000_000,
				CapitalMinor: 44_000_000,
				MarginMinor:  11_000_000,
				MarginBps:    fixtureRatio(11_000_000, 44_000_000),
				SampleSize:   19,
				Source:       poreport.SourceElapsedDays,
			},
		},
	}
	return snapshot
}

func unavailableCurrentReport() poreport.Report {
	snapshot := dayReport()
	snapshot.CurrentProjection = unavailableCurrentProjection(
		poreport.ProjectionModeDay, poreport.ReasonInsufficientDays, poreport.MethodCurrentDay,
		poreport.CurrentProjectionBasis{
			From: "2026-01-01", To: "2026-01-20", Units: 3, PoCount: 2, AsOf: "2026-01-19",
		})
	return snapshot
}

// distribute splits an exact integer total across fixed weights. The last
// share absorbs the remainder, so the parts always sum back to the total.
func distribute(total int64, weights []int64) []int64 {
	var sum int64
	for _, weight := range weights {
		sum += weight
	}
	out := make([]int64, len(weights))
	var assigned int64
	for index, weight := range weights {
		if index == len(weights)-1 {
			out[index] = total - assigned
			continue
		}
		part := total * weight / sum
		out[index] = part
		assigned += part
	}
	return out
}

// longReport is the 120-period pagination/render fixture. It uses the same
// money grid as the sample, so its category rows and its category series foot
// and derive their percentages for any month count. Its range covers complete
// months only and the as-of date sits after the range, so no period is marked
// partial or in progress; the flags are facts, not decoration.
func longReport(months int) poreport.Report {
	snapshot := sampleReport()
	snapshot.GeneratedAt = "2026-01-05T08:30:00Z"
	snapshot.AsOf = "2026-01-04"
	snapshot.Filter = poreport.Query{
		From: "2016-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}

	base := make([]poreport.Metrics, 0, months)
	for index := 0; index < months; index++ {
		base = append(base, poreport.Metrics{
			PoCount:       1,
			AmountMinor:   10_000_000,
			CostedMinor:   9_000_000,
			UncostedMinor: 1_000_000,
			CapitalMinor:  8_000_000,
			MarginMinor:   1_000_000,
		})
	}
	grid := fixtureGrid(base)

	periods := make([]poreport.Period, 0, months)
	for index, metrics := range base {
		date := time.Date(2016, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, index, 0)
		periods = append(periods, poreport.Period{
			PeriodStart: date.Format(time.DateOnly),
			Metrics:     fixtureMetrics(metrics.PoCount, grid[index]),
		})
	}
	snapshot.Periods = periods

	counts := distribute(int64(months), []int64{40, 30, 25, 15, 10})
	snapshot.Categories = fixtureCategories(grid, counts)
	snapshot.CategorySeries = fixtureCategorySeries(grid)

	totalCells := make([]fixtureCell, 0, len(grid)*len(fixtureCategoryCodes))
	for _, row := range grid {
		totalCells = append(totalCells, row...)
	}
	snapshot.Totals = fixtureMetrics(int64(months), totalCells)

	basis := poreport.ProjectionBasis{
		HistoryFrom: "2016-01-01", HistoryTo: "2026-01-01",
		CompleteMonths: months, PoCount: int64(months), AsOf: "2026-01-04",
	}
	monthly := make([]poreport.ProjectedBucket, 0, 12)
	for month := 1; month <= 12; month++ {
		monthly = append(monthly, poreport.ProjectedBucket{
			PeriodStart:     monthStart(2026, time.Month(month)),
			AmountLowMinor:  8_000_000,
			AmountBaseMinor: 10_000_000,
			AmountHighMinor: 13_000_000,
			CapitalMinor:    7_500_000,
			MarginMinor:     2_500_000,
			MarginBps:       fixtureRatio(2_500_000, 7_500_000),
			SampleSize:      6,
			Source:          poreport.SourceSameCalendarMonth,
		})
	}
	yearly := make([]poreport.ProjectedBucket, 0, 3)
	for year := 2026; year <= 2028; year++ {
		yearly = append(yearly, poreport.ProjectedBucket{
			PeriodStart:     monthStart(year, time.January),
			AmountLowMinor:  100_000_000,
			AmountBaseMinor: 120_000_000,
			AmountHighMinor: 150_000_000,
			CapitalMinor:    90_000_000,
			MarginMinor:     30_000_000,
			MarginBps:       fixtureRatio(30_000_000, 90_000_000),
			SampleSize:      12,
			Source:          poreport.SourceTrailingTwelveCompleteMonths,
		})
	}
	snapshot.Projection = poreport.Projection{
		Monthly: poreport.ProjectionSeries{
			Status:  poreport.ProjectionStatusAvailable,
			Method:  poreport.MethodMonthly,
			Basis:   basis,
			Buckets: monthly,
		},
		Yearly: poreport.ProjectionSeries{
			Status:  poreport.ProjectionStatusAvailable,
			Method:  poreport.MethodYearly,
			Basis:   basis,
			Buckets: yearly,
		},
	}
	snapshot.CurrentProjection = unavailableCurrentProjection(
		poreport.ProjectionModeNone, poreport.ReasonNotCurrentPeriod,
		poreport.MethodCurrentNone, poreport.CurrentProjectionBasis{AsOf: "2026-01-04"})
	return snapshot
}

func TestSampleReportFixtureValidates(t *testing.T) {
	if err := ValidateSnapshot(sampleReport()); err != nil {
		t.Fatalf("sample report does not validate: %v", err)
	}
	if err := ValidateSnapshot(unavailableProjectionReport()); err != nil {
		t.Fatalf("unavailable projection report does not validate: %v", err)
	}
	if err := ValidateSnapshot(dayReport()); err != nil {
		t.Fatalf("day report does not validate: %v", err)
	}
	if err := ValidateSnapshot(currentYearReport()); err != nil {
		t.Fatalf("current-year report does not validate: %v", err)
	}
	if err := ValidateSnapshot(unavailableCurrentReport()); err != nil {
		t.Fatalf("unavailable current projection report does not validate: %v", err)
	}
	if err := ValidateSnapshot(longReport(poreport.MaxPeriods)); err != nil {
		t.Fatalf("120-period report does not validate: %v", err)
	}
}

func sumMetrics(metrics []poreport.Metrics) poreport.Metrics {
	var total poreport.Metrics
	for _, row := range metrics {
		total.PoCount += row.PoCount
		total.AmountMinor += row.AmountMinor
		total.CostedMinor += row.CostedMinor
		total.UncostedMinor += row.UncostedMinor
		total.CapitalMinor += row.CapitalMinor
		total.MarginMinor += row.MarginMinor
	}
	return total
}

func metricsFields(metrics poreport.Metrics) [6]int64 {
	return [6]int64{
		metrics.PoCount, metrics.AmountMinor, metrics.CostedMinor,
		metrics.UncostedMinor, metrics.CapitalMinor, metrics.MarginMinor,
	}
}

func assertMetricSums(t *testing.T, name string, rows []poreport.Metrics, want poreport.Metrics) {
	t.Helper()
	got := sumMetrics(rows)
	if gotFields, wantFields := metricsFields(got), metricsFields(want); gotFields != wantFields {
		t.Errorf("%s rows foot to %v, want %v", name, gotFields, wantFields)
	}
}

func assertDerivedRatio(t *testing.T, name, ratio string, got *int64, numerator, denominator int64) {
	t.Helper()
	want, err := poreport.RatioBps(numerator, denominator)
	if err != nil {
		t.Fatalf("%s %s: RatioBps(%d, %d): %v", name, ratio, numerator, denominator, err)
	}
	if (got == nil) != (want == nil) {
		if want == nil {
			t.Errorf("%s %s = %v; want nil because the denominator %d is not positive", name, ratio, got, denominator)
		} else {
			t.Errorf("%s %s = nil; want %d from %d/%d", name, ratio, *want, numerator, denominator)
		}
		return
	}
	if got != nil && *got != *want {
		t.Errorf("%s %s = %d; want %d from %d/%d", name, ratio, *got, *want, numerator, denominator)
	}
}

func assertDerivedMetrics(t *testing.T, name string, metrics poreport.Metrics) {
	t.Helper()
	if got, want := metrics.AmountMinor, metrics.CostedMinor+metrics.UncostedMinor; got != want {
		t.Errorf("%s amount = %d; want costed+uncosted = %d", name, got, want)
	}
	if got, want := metrics.MarginMinor, metrics.CostedMinor-metrics.CapitalMinor; got != want {
		t.Errorf("%s margin = %d; want costed-capital = %d", name, got, want)
	}
	assertDerivedRatio(t, name, "marginBps", metrics.MarginBps, metrics.MarginMinor, metrics.CapitalMinor)
	assertDerivedRatio(t, name, "coverageBps", metrics.CoverageBps, metrics.CostedMinor, metrics.AmountMinor)
}

// TestFixtureRatiosAreDerivedFromMoney is the guard the rendered percentages
// were missing: a report's margin percentage is margin/capital and its coverage
// is costed/amount. A fixture that prints 11.11% beside a 12.50% relationship
// makes every visual and PDF assertion derived from it false evidence. The
// check covers every period, category, the totals and every projected bucket,
// and pins that a ratio is nil only when its denominator is not positive.
func TestFixtureRatiosAreDerivedFromMoney(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot poreport.Report
	}{
		{"sample", sampleReport()},
		{"day", dayReport()},
		{"currentYear", currentYearReport()},
		{"long120", longReport(poreport.MaxPeriods)},
		{"long1", longReport(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertDerivedMetrics(t, "totals", tc.snapshot.Totals)
			for _, period := range tc.snapshot.Periods {
				assertDerivedMetrics(t, "period "+period.PeriodStart, period.Metrics)
			}
			for _, category := range tc.snapshot.Categories {
				assertDerivedMetrics(t, "category "+category.BusinessCategory, category.Metrics)
			}
			for _, series := range []poreport.ProjectionSeries{
				tc.snapshot.Projection.Monthly,
				tc.snapshot.Projection.Yearly,
			} {
				for _, bucket := range series.Buckets {
					assertDerivedRatio(t, "projection "+bucket.PeriodStart, "marginBps",
						bucket.MarginBps, bucket.MarginMinor, bucket.CapitalMinor)
					if got, want := bucket.MarginMinor, bucket.AmountBaseMinor-bucket.CapitalMinor; got != want {
						t.Errorf("projection %s margin = %d; want base-capital = %d", bucket.PeriodStart, got, want)
					}
				}
			}
			current := tc.snapshot.CurrentProjection
			for _, bucket := range current.Buckets {
				assertDerivedRatio(t, "current projection "+bucket.PeriodStart, "marginBps",
					bucket.MarginBps, bucket.MarginMinor, bucket.CapitalMinor)
				if got, want := bucket.MarginMinor, bucket.AmountBaseMinor-bucket.CapitalMinor; got != want {
					t.Errorf("current projection %s margin = %d; want base-capital = %d",
						bucket.PeriodStart, got, want)
				}
			}
			if current.CurrentMonth != nil {
				remainder := current.CurrentMonth.Remainder
				assertDerivedRatio(t, "current month remainder", "marginBps",
					remainder.MarginBps, remainder.MarginMinor, remainder.CapitalMinor)
				if got, want := remainder.MarginMinor, remainder.BaseMinor-remainder.CapitalMinor; got != want {
					t.Errorf("current month remainder margin = %d; want base-capital = %d", got, want)
				}
			}

			// The category series must be the same money grid's per-period
			// ratio, in period order. A hand-invented series cannot pass this.
			periodMetrics := make([]poreport.Metrics, 0, len(tc.snapshot.Periods))
			for _, period := range tc.snapshot.Periods {
				periodMetrics = append(periodMetrics, period.Metrics)
			}
			grid := fixtureGrid(periodMetrics)
			want := fixtureCategorySeries(grid)
			if len(tc.snapshot.CategorySeries) != len(want) {
				t.Fatalf("category series count = %d; want %d from the money grid",
					len(tc.snapshot.CategorySeries), len(want))
			}
			for index := range want {
				gotSeries, wantSeries := tc.snapshot.CategorySeries[index], want[index]
				if gotSeries.BusinessCategory != wantSeries.BusinessCategory {
					t.Fatalf("category series %d = %s; want %s",
						index, gotSeries.BusinessCategory, wantSeries.BusinessCategory)
				}
				if len(gotSeries.MarginBps) != len(wantSeries.MarginBps) {
					t.Fatalf("%s series has %d points; want %d",
						gotSeries.BusinessCategory, len(gotSeries.MarginBps), len(wantSeries.MarginBps))
				}
				categoryIndex := -1
				for candidate, code := range fixtureCategoryCodes {
					if code == gotSeries.BusinessCategory {
						categoryIndex = candidate
					}
				}
				if categoryIndex < 0 {
					t.Fatalf("series names unknown category %s", gotSeries.BusinessCategory)
				}
				for point := range wantSeries.MarginBps {
					cell := grid[point][categoryIndex]
					assertDerivedRatio(t,
						"series "+gotSeries.BusinessCategory+" "+tc.snapshot.Periods[point].PeriodStart,
						"marginBps", gotSeries.MarginBps[point], cell.margin, cell.capital)
					wantPoint, gotPoint := wantSeries.MarginBps[point], gotSeries.MarginBps[point]
					if (wantPoint == nil) != (gotPoint == nil) ||
						(wantPoint != nil && *wantPoint != *gotPoint) {
						t.Errorf("%s series %d = %v; want %v from the money grid",
							gotSeries.BusinessCategory, point, gotPoint, wantPoint)
					}
				}
			}
		})
	}
}

// TestFixturesFoot is the guard behind every rendered-total assertion. The
// renderer intentionally does no report arithmetic: if the fixture rows did
// not sum to its totals, the PDF would faithfully print a lie and the visual
// evidence would be worthless. Sample and 120-period fixtures are checked
// independently, including counts.
func TestFixturesFoot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot poreport.Report
	}{
		{"sample", sampleReport()},
		{"day", dayReport()},
		{"currentYear", currentYearReport()},
		{"long120", longReport(poreport.MaxPeriods)},
		{"long1", longReport(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			periodMetrics := make([]poreport.Metrics, 0, len(tc.snapshot.Periods))
			for _, period := range tc.snapshot.Periods {
				periodMetrics = append(periodMetrics, period.Metrics)
			}
			categoryMetrics := make([]poreport.Metrics, 0, len(tc.snapshot.Categories))
			for _, category := range tc.snapshot.Categories {
				categoryMetrics = append(categoryMetrics, category.Metrics)
			}
			assertMetricSums(t, "period", periodMetrics, tc.snapshot.Totals)
			assertMetricSums(t, "category", categoryMetrics, tc.snapshot.Totals)
		})
	}
}
