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

// sampleReport is a complete report with every section populated: twelve
// actual months including a negative margin and an unknown percentage, five
// category rows, both projection series, and one disclosure note.
//
// Every period and category row sums to Totals. The figures are constructed
// together, not copied from two unrelated examples: a document whose rows do
// not foot cannot prove that the renderer prints a trustworthy total.
func sampleReport() poreport.Report {
	periods := make([]poreport.Period, 0, 12)
	for month := 1; month <= 12; month++ {
		metrics := poreport.Metrics{
			PoCount:       2,
			AmountMinor:   100_000_000,
			CostedMinor:   90_000_000,
			UncostedMinor: 10_000_000,
			CapitalMinor:  70_000_000,
			MarginMinor:   20_000_000,
			MarginBps:     pointer(2222),
			CoverageBps:   pointer(9000),
		}
		switch month {
		case 3:
			// A month discounted below cost: capital exceeds costed amount.
			metrics.CapitalMinor = 120_000_000
			metrics.MarginMinor = -30_000_000
			metrics.MarginBps = pointer(-3333)
		case 4:
			// Balances March's extra capital so the twelve months sum to 830m.
			metrics.CapitalMinor = 20_000_000
			metrics.MarginMinor = 70_000_000
			metrics.MarginBps = pointer(7778)
		case 6:
			metrics.MarginBps = nil
		case 12:
			// The partial month carries the odd amount and the second loss.
			metrics.AmountMinor = 134_567_890
			metrics.CostedMinor = 10_000_000
			metrics.UncostedMinor = 124_567_890
			metrics.CapitalMinor = 60_000_000
			metrics.MarginMinor = -50_000_000
			metrics.MarginBps = pointer(-50000)
			metrics.CoverageBps = pointer(743)
		}
		periods = append(periods, poreport.Period{
			PeriodStart: monthStart(2025, time.Month(month)),
			Partial:     month == 12,
			InProgress:  month == 12,
			Metrics:     metrics,
		})
	}

	categories := []poreport.Category{
		{BusinessCategory: poreport.CategoryFm, Label: "Facility Management", Metrics: poreport.Metrics{
			PoCount: 8, AmountMinor: 400_000_000, CostedMinor: 330_000_000, UncostedMinor: 70_000_000,
			CapitalMinor: 260_000_000, MarginMinor: 70_000_000, MarginBps: pointer(2121), CoverageBps: pointer(8250),
		}},
		{BusinessCategory: poreport.CategoryFab, Label: "Fabrication", Metrics: poreport.Metrics{
			PoCount: 6, AmountMinor: 350_000_000, CostedMinor: 300_000_000, UncostedMinor: 50_000_000,
			CapitalMinor: 320_000_000, MarginMinor: -20_000_000, MarginBps: pointer(-667), CoverageBps: pointer(8571),
		}},
		{BusinessCategory: poreport.CategorySupplyGeneral, Label: "Goods Supply & General", Metrics: poreport.Metrics{
			PoCount: 5, AmountMinor: 300_000_000, CostedMinor: 240_000_000, UncostedMinor: 60_000_000,
			CapitalMinor: 190_000_000, MarginMinor: 50_000_000, MarginBps: pointer(2083), CoverageBps: pointer(8000),
		}},
		{BusinessCategory: poreport.CategoryOthers, Label: "Others", Metrics: poreport.Metrics{
			PoCount: 3, AmountMinor: 120_000_000, CostedMinor: 90_000_000, UncostedMinor: 30_000_000,
			CapitalMinor: 60_000_000, MarginMinor: 30_000_000, MarginBps: pointer(3333), CoverageBps: pointer(7500),
		}},
		{BusinessCategory: poreport.CategoryUnassigned, Label: "Unassigned", Metrics: poreport.Metrics{
			PoCount: 2, AmountMinor: 64_567_890, CostedMinor: 40_000_000, UncostedMinor: 24_567_890,
			CapitalMinor: 0, MarginMinor: 40_000_000, MarginBps: nil, CoverageBps: pointer(6195),
		}},
	}

	series := make([]poreport.CategorySeries, 0, 5)
	codes := []string{
		poreport.CategoryFm, poreport.CategoryFab, poreport.CategorySupplyGeneral,
		poreport.CategoryOthers, poreport.CategoryUnassigned,
	}
	for index, code := range codes {
		points := make([]*int64, 0, len(periods))
		for month := range periods {
			if index == 4 && month%2 == 0 {
				points = append(points, nil)
			} else {
				points = append(points, pointer(int64(1000+index*250+month)))
			}
		}
		series = append(series, poreport.CategorySeries{BusinessCategory: code, MarginBps: points})
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
			MarginBps:       pointer(3333),
			SampleSize:      6,
			Source:          poreport.SourceSameCalendarMonth,
		})
	}
	yearly := make([]poreport.ProjectedBucket, 0, 3)
	for year := 2027; year <= 2029; year++ {
		yearly = append(yearly, poreport.ProjectedBucket{
			PeriodStart:     monthStart(year, time.January),
			AmountLowMinor:  100_000_000,
			AmountBaseMinor: 120_000_000,
			AmountHighMinor: 150_000_000,
			CapitalMinor:    90_000_000,
			MarginMinor:     30_000_000,
			MarginBps:       pointer(3333),
			SampleSize:      12,
			Source:          poreport.SourceTrailingTwelveCompleteMonths,
		})
	}

	return poreport.Report{
		GeneratedAt: "2026-01-05T08:30:00Z",
		AsOf:        "2026-01-04",
		Filter: poreport.Query{
			From: "2025-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
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
		Totals: poreport.Metrics{
			PoCount: 24, AmountMinor: 1_234_567_890, CostedMinor: 1_000_000_000,
			UncostedMinor: 234_567_890, CapitalMinor: 830_000_000, MarginMinor: 170_000_000,
			MarginBps: pointer(2048), CoverageBps: pointer(8100),
		},
		Periods:        periods,
		Categories:     categories,
		CategorySeries: series,
		Projection: poreport.Projection{
			Monthly: poreport.ProjectionSeries{
				Status: poreport.ProjectionStatusAvailable,
				Method: poreport.MethodMonthly,
				Basis: poreport.ProjectionBasis{
					HistoryFrom: "2024-07-01", HistoryTo: "2026-01-01",
					CompleteMonths: 18, PoCount: 24, AsOf: "2026-01-04",
				},
				Buckets: monthly,
			},
			Yearly: poreport.ProjectionSeries{
				Status: poreport.ProjectionStatusAvailable,
				Method: poreport.MethodYearly,
				Basis: poreport.ProjectionBasis{
					HistoryFrom: "2024-07-01", HistoryTo: "2026-01-01",
					CompleteMonths: 18, PoCount: 24, AsOf: "2026-01-04",
				},
				Buckets: yearly,
			},
		},
		Notes: []poreport.Note{{
			Code: "legacy_order_adjustment",
			Text: "Some purchase orders in this range carry a retired whole-order discount or adjustment. " +
				"The amount includes it, but it is not supplier cost and no margin is inferred from it.",
		}},
	}
}

func unavailableProjectionReport() poreport.Report {
	snapshot := sampleReport()
	basis := poreport.ProjectionBasis{AsOf: "2026-01-04"}
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

func longReport(months int) poreport.Report {
	snapshot := sampleReport()
	snapshot.Filter = poreport.Query{
		From: "2016-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}
	periods := make([]poreport.Period, 0, months)
	for index := 0; index < months; index++ {
		date := time.Date(2016, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, index, 0)
		metrics := poreport.Metrics{
			PoCount:       1,
			AmountMinor:   10_000_000,
			CostedMinor:   9_000_000,
			UncostedMinor: 1_000_000,
			CapitalMinor:  8_000_000,
			MarginMinor:   1_000_000,
			MarginBps:     pointer(1111),
			CoverageBps:   pointer(9000),
		}
		periods = append(periods, poreport.Period{
			PeriodStart: date.Format(time.DateOnly),
			Partial:     index == months-1,
			InProgress:  index == months-1,
			Metrics:     metrics,
		})
	}
	snapshot.Periods = periods
	snapshot.Totals = poreport.Metrics{
		PoCount: int64(months), AmountMinor: int64(months) * 10_000_000,
		CostedMinor: int64(months) * 9_000_000, UncostedMinor: int64(months) * 1_000_000,
		CapitalMinor: int64(months) * 8_000_000, MarginMinor: int64(months) * 1_000_000,
		MarginBps: pointer(1111), CoverageBps: pointer(9000),
	}

	// The category rows are the same five categories, split from the same
	// totals. Each row's amount is costed plus uncosted and each margin is
	// costed minus capital, so the category table foots for any month count.
	weights := []int64{40, 30, 25, 15, 10}
	counts := distribute(snapshot.Totals.PoCount, weights)
	costed := distribute(snapshot.Totals.CostedMinor, weights)
	uncosted := distribute(snapshot.Totals.UncostedMinor, weights)
	capital := distribute(snapshot.Totals.CapitalMinor, weights)
	codes := []string{
		poreport.CategoryFm, poreport.CategoryFab, poreport.CategorySupplyGeneral,
		poreport.CategoryOthers, poreport.CategoryUnassigned,
	}
	labels := []string{"Facility Management", "Fabrication", "Goods Supply & General", "Others", "Unassigned"}
	categories := make([]poreport.Category, 0, len(codes))
	for index, code := range codes {
		categories = append(categories, poreport.Category{
			BusinessCategory: code,
			Label:            labels[index],
			Metrics: poreport.Metrics{
				PoCount:       counts[index],
				AmountMinor:   costed[index] + uncosted[index],
				CostedMinor:   costed[index],
				UncostedMinor: uncosted[index],
				CapitalMinor:  capital[index],
				MarginMinor:   costed[index] - capital[index],
				MarginBps:     pointer(1111),
				CoverageBps:   pointer(9000),
			},
		})
	}
	snapshot.Categories = categories
	// The aligned category series must follow the longer period list.
	for index := range snapshot.CategorySeries {
		points := make([]*int64, len(periods))
		for month := range periods {
			points[month] = pointer(int64(1000 + index*100 + month))
		}
		snapshot.CategorySeries[index].MarginBps = points
	}
	return snapshot
}

func TestSampleReportFixtureValidates(t *testing.T) {
	if err := ValidateSnapshot(sampleReport()); err != nil {
		t.Fatalf("sample report does not validate: %v", err)
	}
	if err := ValidateSnapshot(unavailableProjectionReport()); err != nil {
		t.Fatalf("unavailable projection report does not validate: %v", err)
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
