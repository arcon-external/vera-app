package report

import (
	"testing"

	"vera-app/backend/poreport"
)

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
	if got := formatSignedMoney("IDR", -20_000_000); got != "-Rp20.000.000" {
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
}

func TestUnknownPercentPrintsAnEmDashNeverZero(t *testing.T) {
	if got := formatPct(nil); got != "—" {
		t.Errorf("nil percent = %q, want an em dash", got)
	}
	if got := formatPct(pointer(0)); got != "0.0%" {
		t.Errorf("measured zero percent = %q, want 0.0%%", got)
	}
	if got := formatPct(pointer(-2857)); got != "-28.6%" {
		t.Errorf("negative percent = %q", got)
	}
}

func TestPeriodLabelsFollowTheBucket(t *testing.T) {
	for _, tc := range []struct {
		start, bucket, chart, table string
	}{
		{"2025-01-01", poreport.BucketMonth, "Jan 25", "Jan 2025"},
		{"2025-04-01", poreport.BucketQuarter, "Q2 25", "Q2 2025"},
		{"2025-01-01", poreport.BucketYear, "2025", "2025"},
	} {
		if got := periodLabel(tc.start, tc.bucket); got != tc.chart {
			t.Errorf("periodLabel(%q, %q) = %q, want %q", tc.start, tc.bucket, got, tc.chart)
		}
		if got := periodTableLabel(tc.start, tc.bucket); got != tc.table {
			t.Errorf("periodTableLabel(%q, %q) = %q, want %q", tc.start, tc.bucket, got, tc.table)
		}
	}
}

func TestGeneratedTimestampKeepsTheServerSecond(t *testing.T) {
	if got := generatedLabel("2026-01-05T08:30:07Z"); got != "05 Jan 2026 08:30:07 UTC" {
		t.Errorf("generated label = %q", got)
	}
}
