package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"vera-app/backend/poreport"
)

type reportFormatFixture struct {
	Money []struct {
		Name     string `json:"name"`
		Code     string `json:"code"`
		Minor    int64  `json:"minor"`
		Expected string `json:"expected"`
	} `json:"money"`
	Bps []struct {
		Name     string `json:"name"`
		Value    *int64 `json:"value"`
		Expected string `json:"expected"`
	} `json:"bps"`
	Count []struct {
		Name     string `json:"name"`
		Value    int64  `json:"value"`
		Expected string `json:"expected"`
	} `json:"count"`
}

func loadReportFormatFixture(t *testing.T) reportFormatFixture {
	t.Helper()
	path := filepath.Join("testdata", "report_formats.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture reportFormatFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(fixture.Money) == 0 || len(fixture.Bps) == 0 || len(fixture.Count) == 0 {
		t.Fatalf("%s is missing one of its sections", path)
	}
	return fixture
}

func TestCategoryAndFilterLabelsUseTheWorkbookVocabulary(t *testing.T) {
	cases := map[string]string{
		poreport.CategoryAll:           "All categories",
		poreport.CategoryFm:            "Facility Management",
		poreport.CategoryFab:           "Fabrication",
		poreport.CategorySupplyGeneral: "Goods Supply & General",
		poreport.CategoryOthers:        "Others",
		poreport.CategoryUnassigned:    "Unassigned",
	}
	for code, want := range cases {
		if got := categoryLabel(code); got != want {
			t.Errorf("categoryLabel(%q) = %q, want %q", code, got, want)
		}
	}
	if got := categoryRowLabel(poreport.CategoryUnassigned, ""); got != "Unassigned (UNASSIGNED)" {
		t.Errorf("unassigned row label = %q", got)
	}
	if got := categoryRowLabel(poreport.CategorySupplyGeneral, "Goods Supply & General"); got != "Goods Supply & General (SUPPLY_GEN)" {
		t.Errorf("supply row label = %q", got)
	}
	if got := categoryFilterLabel(poreport.CategoryAll); got != "All categories (all)" {
		t.Errorf("all filter label = %q", got)
	}
}

func TestMoneyFormattingFollowsTheFilterCurrency(t *testing.T) {
	if got := formatMoney("IDR", 1_234_567_890); got != "Rp1.234.567.890" {
		t.Errorf("IDR money = %q", got)
	}
	if got := formatSignedMoney("IDR", -20_000_000); got != "Rp-20.000.000" {
		t.Errorf("negative IDR money = %q", got)
	}
	if got := formatMoney("USD", 123_456); got != "$1,234.56" {
		t.Errorf("USD money = %q", got)
	}
	if got := formatMoney("EUR", 123_456); got != "1.234,56 €" {
		t.Errorf("EUR money = %q", got)
	}
	if got := compactMoney("IDR", 150_000_000); got != "Rp150M" {
		t.Errorf("compact money = %q", got)
	}
	if got := compactMoney("IDR", -150_000_000); got != "Rp-150M" {
		t.Errorf("negative compact money = %q", got)
	}
	// A two-decimal currency arrives in minor units, exactly like the tables.
	// Compacting before applying the exponent prints "$10K" for $100: a 100x
	// error between the axis and the same page's table.
	if got := compactMoney("USD", 10_000); got != "$100" {
		t.Errorf("USD compact money = %q; want $100 (10000 minor)", got)
	}
	if got := compactMoney("USD", 12_345_600); got != "$123.5K" {
		t.Errorf("USD compact money = %q; want $123.5K", got)
	}
	if got := compactMoney("USD", -12_345_600); got != "$-123.5K" {
		t.Errorf("negative USD compact money = %q; want $-123.5K", got)
	}
	if got := compactMoney("EUR", 10_000); got != "100 €" {
		t.Errorf("EUR compact money = %q; want 100 €", got)
	}
	if got := compactMoney("IDR", 10_000); got != "Rp10K" {
		t.Errorf("IDR compact money = %q; want Rp10K", got)
	}
}

func TestFormattingMatchesTheSharedCrossSurfaceFixture(t *testing.T) {
	fixture := loadReportFormatFixture(t)
	for _, tc := range fixture.Money {
		t.Run("money/"+tc.Name, func(t *testing.T) {
			if got := formatMoney(tc.Code, tc.Minor); got != tc.Expected {
				t.Errorf("formatMoney(%q, %d) = %q, want %q", tc.Code, tc.Minor, got, tc.Expected)
			}
			if got := formatSignedMoney(tc.Code, tc.Minor); got != tc.Expected {
				t.Errorf("formatSignedMoney(%q, %d) = %q, want %q", tc.Code, tc.Minor, got, tc.Expected)
			}
		})
	}
	for _, tc := range fixture.Bps {
		t.Run("bps/"+tc.Name, func(t *testing.T) {
			if got := formatPct(tc.Value); got != tc.Expected {
				t.Errorf("formatPct(%v) = %q, want %q", tc.Value, got, tc.Expected)
			}
		})
	}
	for _, tc := range fixture.Count {
		t.Run("count/"+tc.Name, func(t *testing.T) {
			if got := formatCount(tc.Value); got != tc.Expected {
				t.Errorf("formatCount(%d) = %q, want %q", tc.Value, got, tc.Expected)
			}
		})
	}
}

func TestUnknownPercentPrintsAPlaceholderNeverZero(t *testing.T) {
	if got := formatPct(nil); got != "-" {
		t.Errorf("nil percent = %q, want a placeholder hyphen", got)
	}
	if got := formatPct(pointer(0)); got != "0.00%" {
		t.Errorf("measured zero percent = %q, want 0.00%%", got)
	}
	if got := formatPct(pointer(-2857)); got != "-28.57%" {
		t.Errorf("negative percent = %q", got)
	}
}

func TestChartPercentTicksStayInsideTheAxis(t *testing.T) {
	for _, tc := range []struct {
		value float64
		want  string
	}{
		{0, "0%"},
		{12.5, "12.5%"},
		{-28.57, "-28.57%"},
		{-100000000000000, "-100T%"},
		{90071992547409.91, "90.1T%"},
	} {
		if got := percentTick(tc.value); got != tc.want {
			t.Errorf("percentTick(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestPeriodLabelsFollowTheBucket(t *testing.T) {
	for _, tc := range []struct {
		start, bucket, chart, table string
	}{
		{"2025-01-01", poreport.BucketMonth, "Jan 25", "Jan 2025"},
		{"2025-04-01", poreport.BucketQuarter, "Q2 25", "Q2 2025"},
		{"2025-01-01", poreport.BucketYear, "2025", "2025"},
		{"2026-01-01", poreport.BucketDay, "1 Jan", "1 Jan 2026"},
		{"2026-01-31", poreport.BucketDay, "31 Jan", "31 Jan 2026"},
		{"2026-12-09", poreport.BucketDay, "9 Dec", "9 Dec 2026"},
	} {
		if got := periodLabel(tc.start, tc.bucket); got != tc.chart {
			t.Errorf("periodLabel(%q, %q) = %q, want %q", tc.start, tc.bucket, got, tc.chart)
		}
		if got := periodTableLabel(tc.start, tc.bucket); got != tc.table {
			t.Errorf("periodTableLabel(%q, %q) = %q, want %q", tc.start, tc.bucket, got, tc.table)
		}
	}
}

func TestBucketLabelNamesTheDayBucket(t *testing.T) {
	for bucket, want := range map[string]string{
		poreport.BucketDay:     "Daily",
		poreport.BucketMonth:   "Monthly",
		poreport.BucketQuarter: "Quarterly",
		poreport.BucketYear:    "Yearly",
	} {
		if got := bucketLabel(bucket); got != want {
			t.Errorf("bucketLabel(%q) = %q, want %q", bucket, got, want)
		}
	}
}

func TestCurrentProjectionReasonLabelsCoverEveryStableCode(t *testing.T) {
	for _, reason := range []string{
		poreport.ReasonNotCurrentPeriod, poreport.ReasonNoData,
		poreport.ReasonInsufficientDays, poreport.ReasonInsufficientMonths,
		poreport.ReasonIncompleteCostBasis, poreport.ReasonValueOutOfRange,
		poreport.ReasonInvalidQuery,
	} {
		if got := projectionReasonLabel(reason); got == reason || got == "" {
			t.Errorf("projectionReasonLabel(%q) = %q; every stable code needs prose", reason, got)
		}
	}
}

func TestGeneratedTimestampKeepsTheServerSecond(t *testing.T) {
	if got := generatedLabel("2026-01-05T08:30:07Z"); got != "05 Jan 2026 08:30:07 UTC" {
		t.Errorf("generated label = %q", got)
	}
}
