package report

import (
	"testing"

	"vera-app/backend/poreport"
)

// Issue #655: an extra line is a pass-through expense. The PDF prints the
// coverage sentence against the non-extra amount and names the Extra Expense
// bucket with the figures the Reports screen shows on the same fixture: a March
// quoted line (5 x 10.000, cost 6.000) beside a freight extra (2 x 500.000),
// and an April record whose only line is an extra.

// extraExpenseMetrics builds one row from its amount split. Every ratio is
// derived with the server's own arithmetic; the extra is never costed and the
// coverage denominator is the non-extra amount.
func extraExpenseMetrics(poCount, amount, uncosted, capital, extra int64) poreport.Metrics {
	costed := amount - uncosted - extra
	margin := costed - capital
	return poreport.Metrics{
		PoCount:           poCount,
		AmountMinor:       amount,
		CostedMinor:       costed,
		UncostedMinor:     uncosted,
		ExtraExpenseMinor: extra,
		CapitalMinor:      capital,
		MarginMinor:       margin,
		MarginBps:         fixtureRatio(margin, capital),
		CoverageBps:       fixtureRatio(costed, amount-extra),
	}
}

func extraExpenseReport(totals poreport.Metrics, periods []poreport.Period) poreport.Report {
	basis := poreport.ProjectionBasis{AsOf: "2026-11-19"}
	return poreport.Report{
		GeneratedAt: "2026-11-19T05:00:00Z",
		AsOf:        "2026-11-19",
		Filter: poreport.Query{
			From: "2026-03-01", To: "2026-05-01", Bucket: poreport.BucketMonth,
			Category: poreport.CategoryAll, Currency: "IDR",
		},
		Company:    poreport.Company{Name: "PT. Extra Report", Address: "Batam"},
		Population: poreport.Population{AvailableCurrencies: []string{"IDR"}},
		Totals:     totals,
		Periods:    periods,
		Categories: []poreport.Category{{
			BusinessCategory: poreport.CategoryFm,
			Label:            "Facility Management",
			Metrics:          totals,
		}},
		Projection: poreport.Projection{
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
		},
	}
}

func mixedAndExtraOnlyReport() poreport.Report {
	march := extraExpenseMetrics(1, 1_050_000, 0, 30_000, 1_000_000)
	april := extraExpenseMetrics(1, 1_000_000, 0, 0, 1_000_000)
	periods := []poreport.Period{
		{PeriodStart: "2026-03-01", Metrics: march},
		{PeriodStart: "2026-04-01", Metrics: april},
	}
	return extraExpenseReport(extraExpenseMetrics(2, 2_050_000, 0, 30_000, 2_000_000), periods)
}

func TestExtraExpenseRangePrintsNonExtraCoverageAndItsOwnBucket(t *testing.T) {
	text := renderPageText(t, mixedAndExtraOnlyReport())
	requireText(t, text,
		// The same figures the Reports screen shows for this fixture.
		"PO AMOUNT",
		"Rp2.050.000",
		"PO MARGIN",
		"Rp20.000",
		// 50.000 / (2.050.000 - 2.000.000), never 50.000 / 2.050.000.
		"Costed Rp50.000 of Rp50.000 (100.00%) of the non-extra amount",
		"Extra expense Rp2.000.000",
		"pass-through",
	)
	requireNoText(t, "non-extra coverage", text,
		"Costed Rp50.000 of Rp2.050.000",
		"(2.44%)",
	)
}

func TestExtraOnlyRangePrintsNoCoveragePercentage(t *testing.T) {
	april := extraExpenseMetrics(1, 1_000_000, 0, 0, 1_000_000)
	report := extraExpenseReport(april, []poreport.Period{{
		PeriodStart: "2026-04-01", Metrics: april,
	}})
	text := renderPageText(t, report)
	requireText(t, text,
		"carries only extra expense",
		"Coverage -",
		"Extra expense Rp1.000.000",
	)
	requireNoText(t, "extra-only coverage", text,
		"Costed Rp0",
		"(0.00%)",
		"(100.00%)",
	)
}

func TestAllQuotationReportKeepsTheOriginalCoverageSentence(t *testing.T) {
	text := renderPageText(t, sampleReport())
	requireText(t, text,
		"Costed Rp910.000.000 of Rp1.234.567.890 (73.71%)  ·  Uncosted Rp324.567.890",
	)
	requireNoText(t, "all-quotation coverage", text,
		"non-extra",
		"Extra expense",
	)
}
