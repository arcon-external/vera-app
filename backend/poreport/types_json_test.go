package poreport

import (
	"encoding/json"
	"testing"
)

func TestReportJSONUsesContractNamesAndNullRatios(t *testing.T) {
	t.Parallel()

	marginBps := int64(0)
	coverageBps := int64(10000)
	report := Report{
		GeneratedAt: "2026-11-19T10:00:00Z",
		AsOf:        "2026-11-19",
		Filter: Query{
			From: "2026-01-01", To: "2027-01-01", Bucket: "month",
			Category: "all", Currency: "IDR",
		},
		Company: Company{Name: "PT Arcon", Address: "Jakarta"},
		Population: Population{
			AvailableCurrencies:  []string{"IDR"},
			HasUnassigned:        true,
			SourceMissingPoCount: 1,
		},
		Totals: Metrics{
			PoCount:           2,
			AmountMinor:       1300,
			CostedMinor:       300,
			UncostedMinor:     0,
			ExtraExpenseMinor: 1000,
			CapitalMinor:      300,
			MarginMinor:       0,
			MarginBps:         &marginBps,
			CoverageBps:       &coverageBps,
		},
		Pipeline: PipelineSummary{UnsoldAmountMinor: 20_000, UnsoldMarginMinor: 8_000},
		Variance: VarianceSummary{
			QuantityExtraIncomeAmountMinor: 0,
			QuantityExtraIncomeMarginMinor: 0,
			UnsoldAmountMinor:              20_000,
			UnsoldMarginMinor:              8_000,
			ClosedLossAmountMinor:          0,
			ClosedLossMarginMinor:          0,
			PriceExtraIncomeMinor:          0,
			PriceLossMinor:                 3_000,
			NetRealizedVarianceAmountMinor: -3_000,
			NetRealizedVarianceMarginMinor: -3_000,
		},
		Periods: []Period{{
			PeriodStart: "2026-01-01",
			// The range starts on this bucket's first day and the as-of date is
			// later in the same year, so neither flag is true.
			Partial: false,
			Metrics: Metrics{
				PoCount: 2, AmountMinor: 300, CostedMinor: 300, UncostedMinor: 0,
				CapitalMinor: 300, MarginMinor: 0, MarginBps: &marginBps, CoverageBps: &coverageBps,
			},
		}},
		Categories: []Category{{
			BusinessCategory: CategoryFm,
			Label:            "Facility Management",
			Metrics: Metrics{
				PoCount: 1, AmountMinor: 300, CostedMinor: 300, UncostedMinor: 0,
				CapitalMinor: 300, MarginMinor: 0, MarginBps: &marginBps, CoverageBps: &coverageBps,
			},
		}},
		CategorySeries: []CategorySeries{{
			BusinessCategory: CategoryFm,
			MarginBps:        []*int64{nil},
		}},
		Projection: Projection{
			Monthly: ProjectionSeries{
				Status:  ProjectionStatusAvailable,
				Method:  MethodMonthly,
				Basis:   ProjectionBasis{CompleteMonths: 6},
				Buckets: []ProjectedBucket{},
			},
			Yearly: ProjectionSeries{
				Status:  ProjectionStatusUnavailable,
				Reason:  ReasonInsufficientHistory,
				Method:  MethodYearly,
				Buckets: []ProjectedBucket{},
			},
		},
		CurrentProjection: CurrentProjection{
			Mode:   ProjectionModeMonth,
			Status: ProjectionStatusAvailable,
			Method: MethodCurrentMonth,
			Basis: CurrentProjectionBasis{
				From: "2026-01-01", To: "2026-11-01",
				Units: 10, PoCount: 12, AsOf: "2026-11-19",
			},
			Buckets: []ProjectedBucket{},
			CurrentMonth: &CurrentMonthProjection{
				Actual: Metrics{PoCount: 3, AmountMinor: 900, CostedMinor: 900, CapitalMinor: 300, MarginMinor: 600},
				Remainder: CurrentProjectionRemainder{
					LowMinor: 100, BaseMinor: 300, HighMinor: 500,
					CapitalMinor: 100, MarginMinor: 200, MarginBps: &marginBps,
					SampleSize: 19, Source: SourceElapsedDays,
				},
			},
		},
		Notes: []Note{{Code: "ppn_excluded", Text: "PPN is excluded."}},
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(Report) error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Report) error = %v", err)
	}

	for _, key := range []string{
		"generatedAt", "asOf", "filter", "company", "population", "totals",
		"pipeline", "variance",
		"periods", "categories", "categorySeries", "projection",
		"currentProjection", "notes",
	} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("report JSON is missing %q", key)
		}
	}

	totals := decoded["totals"].(map[string]any)
	for _, key := range []string{
		"poCount", "amountMinor", "costedMinor", "uncostedMinor",
		"extraExpenseMinor", "capitalMinor", "marginMinor", "marginBps", "coverageBps",
	} {
		if _, ok := totals[key]; !ok {
			t.Errorf("totals JSON is missing %q", key)
		}
	}
	if totals["extraExpenseMinor"] != float64(1000) {
		t.Errorf("extraExpenseMinor = %v; want the exact 1000", totals["extraExpenseMinor"])
	}
	if totals["amountMinor"] != float64(1300) ||
		totals["costedMinor"] != float64(300) ||
		totals["uncostedMinor"] != float64(0) {
		t.Errorf("totals identity JSON = %v; want amount 1300 = costed 300 + extra 1000", totals)
	}
	if totals["coverageBps"] != float64(10000) {
		t.Errorf("coverageBps = %v; want the exact 10000 pointer", totals["coverageBps"])
	}
	if totals["marginBps"] != float64(0) {
		t.Errorf("marginBps = %v; want the exact zero pointer", totals["marginBps"])
	}
	pipeline := decoded["pipeline"].(map[string]any)
	if pipeline["unsoldAmountMinor"] != float64(20_000) {
		t.Errorf("pipeline unsold = %v; want 20000", pipeline["unsoldAmountMinor"])
	}
	variance := decoded["variance"].(map[string]any)
	if variance["priceLossMinor"] != float64(3_000) ||
		variance["netRealizedVarianceAmountMinor"] != float64(-3_000) ||
		variance["closedLossAmountMinor"] != float64(0) {
		t.Errorf("variance JSON = %v; want price loss 3000, net -3000, closed 0", variance)
	}
	series := decoded["categorySeries"].([]any)[0].(map[string]any)["marginBps"].([]any)
	if series[0] != nil {
		t.Errorf("category series marginBps[0] = %v; want null for a gap", series[0])
	}

	period := decoded["periods"].([]any)[0].(map[string]any)
	for _, key := range []string{
		"periodStart", "partial", "inProgress",
		"poCount", "amountMinor", "costedMinor", "uncostedMinor",
		"extraExpenseMinor", "capitalMinor", "marginMinor", "marginBps", "coverageBps",
	} {
		if _, ok := period[key]; !ok {
			t.Errorf("period JSON is missing promoted %q", key)
		}
	}

	projection := decoded["projection"].(map[string]any)
	monthly := projection["monthly"].(map[string]any)
	if _, ok := monthly["reason"]; ok {
		t.Errorf("available monthly series carries reason %v; want omitted", monthly["reason"])
	}
	yearly := projection["yearly"].(map[string]any)
	if yearly["reason"] != ReasonInsufficientHistory {
		t.Errorf("yearly reason = %v; want %s", yearly["reason"], ReasonInsufficientHistory)
	}

	current := decoded["currentProjection"].(map[string]any)
	for _, key := range []string{"mode", "status", "method", "basis", "buckets", "currentMonth"} {
		if _, ok := current[key]; !ok {
			t.Errorf("currentProjection is missing %q", key)
		}
	}
	if _, ok := current["reason"]; ok {
		t.Errorf("available currentProjection carries reason %v; want omitted", current["reason"])
	}
	basis := current["basis"].(map[string]any)
	for _, key := range []string{"from", "to", "units", "poCount", "asOf"} {
		if _, ok := basis[key]; !ok {
			t.Errorf("currentProjection basis is missing %q", key)
		}
	}
	currentMonth := current["currentMonth"].(map[string]any)
	remainder := currentMonth["remainder"].(map[string]any)
	for _, key := range []string{"low", "base", "high", "capital", "margin", "marginBps", "sampleSize", "source"} {
		if _, ok := remainder[key]; !ok {
			t.Errorf("currentProjection remainder is missing %q", key)
		}
	}
	if remainder["source"] != SourceElapsedDays {
		t.Errorf("remainder source = %v; want %s", remainder["source"], SourceElapsedDays)
	}
}
