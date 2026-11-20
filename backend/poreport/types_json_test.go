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
			PoCount:       2,
			AmountMinor:   300,
			CostedMinor:   300,
			UncostedMinor: 0,
			CapitalMinor:  300,
			MarginMinor:   0,
			MarginBps:     &marginBps,
			CoverageBps:   &coverageBps,
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
		"periods", "categories", "categorySeries", "projection", "notes",
	} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("report JSON is missing %q", key)
		}
	}

	totals := decoded["totals"].(map[string]any)
	for _, key := range []string{
		"poCount", "amountMinor", "costedMinor", "uncostedMinor",
		"capitalMinor", "marginMinor", "marginBps", "coverageBps",
	} {
		if _, ok := totals[key]; !ok {
			t.Errorf("totals JSON is missing %q", key)
		}
	}
	if totals["coverageBps"] != float64(10000) {
		t.Errorf("coverageBps = %v; want the exact 10000 pointer", totals["coverageBps"])
	}
	if totals["marginBps"] != float64(0) {
		t.Errorf("marginBps = %v; want the exact zero pointer", totals["marginBps"])
	}
	series := decoded["categorySeries"].([]any)[0].(map[string]any)["marginBps"].([]any)
	if series[0] != nil {
		t.Errorf("category series marginBps[0] = %v; want null for a gap", series[0])
	}

	period := decoded["periods"].([]any)[0].(map[string]any)
	for _, key := range []string{
		"periodStart", "partial", "inProgress",
		"poCount", "amountMinor", "costedMinor", "uncostedMinor",
		"capitalMinor", "marginMinor", "marginBps", "coverageBps",
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
}
