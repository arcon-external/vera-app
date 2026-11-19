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
func sampleReport() poreport.Report {
	periods := make([]poreport.Period, 0, 12)
	for month := 1; month <= 12; month++ {
		period := poreport.Period{
			PeriodStart: monthStart(2025, time.Month(month)),
			Partial:     month == 12,
			InProgress:  month == 12,
			Metrics: poreport.Metrics{
				PoCount:       int64(month % 3),
				AmountMinor:   int64(month * 10_000_000),
				CostedMinor:   int64(month * 9_000_000),
				UncostedMinor: int64(month * 1_000_000),
				CapitalMinor:  int64(month * 8_000_000),
				MarginMinor:   int64(month * 1_000_000),
				MarginBps:     pointer(int64(1250)),
				CoverageBps:   pointer(int64(9000)),
			},
		}
		switch month {
		case 3:
			period.MarginMinor = -20_000_000
			period.MarginBps = pointer(-2857)
		case 6:
			period.CapitalMinor = 0
			period.MarginMinor = 10_000_000
			period.MarginBps = nil
		}
		periods = append(periods, period)
	}

	categories := []poreport.Category{
		{BusinessCategory: poreport.CategoryFm, Label: "Facility Management", Metrics: poreport.Metrics{
			PoCount: 8, AmountMinor: 400_000_000, CostedMinor: 350_000_000, UncostedMinor: 50_000_000,
			CapitalMinor: 300_000_000, MarginMinor: 50_000_000, MarginBps: pointer(1666), CoverageBps: pointer(8750),
		}},
		{BusinessCategory: poreport.CategoryFab, Label: "Fabrication", Metrics: poreport.Metrics{
			PoCount: 6, AmountMinor: 350_000_000, CostedMinor: 300_000_000, UncostedMinor: 50_000_000,
			CapitalMinor: 250_000_000, MarginMinor: 50_000_000, MarginBps: pointer(2000), CoverageBps: pointer(8571),
		}},
		{BusinessCategory: poreport.CategorySupplyGeneral, Label: "Goods Supply & General", Metrics: poreport.Metrics{
			PoCount: 5, AmountMinor: 300_000_000, CostedMinor: 250_000_000, UncostedMinor: 50_000_000,
			CapitalMinor: 200_000_000, MarginMinor: 50_000_000, MarginBps: pointer(2500), CoverageBps: pointer(8333),
		}},
		{BusinessCategory: poreport.CategoryOthers, Label: "Others", Metrics: poreport.Metrics{
			PoCount: 3, AmountMinor: 120_000_000, CostedMinor: 100_000_000, UncostedMinor: 20_000_000,
			CapitalMinor: 80_000_000, MarginMinor: 20_000_000, MarginBps: pointer(2500), CoverageBps: pointer(8333),
		}},
		{BusinessCategory: poreport.CategoryUnassigned, Label: "Unassigned", Metrics: poreport.Metrics{
			PoCount: 2, AmountMinor: 64_567_890, CostedMinor: 0, UncostedMinor: 64_567_890,
			CapitalMinor: 0, MarginMinor: 0, MarginBps: nil, CoverageBps: pointer(0),
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

func longReport(months int) poreport.Report {
	snapshot := sampleReport()
	snapshot.Filter = poreport.Query{
		From: "2016-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}
	periods := make([]poreport.Period, 0, months)
	for index := 0; index < months; index++ {
		date := time.Date(2016, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, index, 0)
		periods = append(periods, poreport.Period{
			PeriodStart: date.Format(time.DateOnly),
			Metrics: poreport.Metrics{
				PoCount:       int64(index%5 + 1),
				AmountMinor:   int64(1_000_000 + index*250_000),
				CostedMinor:   int64(900_000 + index*200_000),
				UncostedMinor: int64(100_000 + index*50_000),
				CapitalMinor:  int64(700_000 + index*150_000),
				MarginMinor:   int64(200_000 + index*50_000),
				MarginBps:     pointer(int64(1500 + index)),
				CoverageBps:   pointer(int64(9000)),
			},
		})
	}
	snapshot.Periods = periods
	// The aligned category series must follow the longer period list.
	for index := range snapshot.CategorySeries {
		points := make([]*int64, len(periods))
		for month := range periods {
			points[month] = pointer(int64(1000 + index*100 + month))
		}
		snapshot.CategorySeries[index].MarginBps = points
	}
	snapshot.Totals = poreport.Metrics{
		PoCount: int64(months), AmountMinor: int64(months) * 2_000_000,
		CostedMinor: int64(months) * 1_800_000, UncostedMinor: int64(months) * 200_000,
		CapitalMinor: int64(months) * 1_500_000, MarginMinor: int64(months) * 300_000,
		MarginBps: pointer(2000), CoverageBps: pointer(9000),
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
