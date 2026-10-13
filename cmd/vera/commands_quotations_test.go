package main

import (
	"bytes"
	"strings"
	"testing"

	"vera-app/backend"
)

// TestPrintInternalTotalsLabelsTheDiscountLinesCorrectly is the guard for the
// bug this whole area turned on: the internal totals table used to print the
// whole-quotation figure under "Row discounts" and the always-zero original
// under "Whole-quotation discount", so the genuine per-row total never reached
// the operator.
//
// The three figures are set to distinct, unlikely-to-collide values so a
// swapped label is caught rather than passed by a coincidental match.
func TestPrintInternalTotalsLabelsTheDiscountLinesCorrectly(t *testing.T) {
	var out, errOut bytes.Buffer
	a := newApp(&out, &errOut)

	internal := backend.InternalBreakdown{
		DisplayOriginalSubtotal:     "7.580.000",
		DisplayMarginalizedSubtotal: "7.580.000",
		// The two lines under test. The whole-quotation figure is deliberately
		// non-zero so it differs from the retired original shown below.
		DisplayRowDiscountTotal:     "620.000",
		DisplayMarginalizedDiscount: "31.500",
		// The retired original-side discount: always zero since #270, kept only
		// so nothing else reads a missing field.
		DisplayOriginalDiscount:     "0",
		DisplayGrand:                "7.548.500",
		DisplayMarginalizedAfterTax: "7.548.500",
		DisplayProfit:               "500.000",
	}

	a.printInternalTotals(internal)

	got := out.String()
	// The two lines must be present in the table; without this a blank table
	// would "pass" the per-line checks below.
	if !strings.Contains(got, "Row discounts") || !strings.Contains(got, "Whole-quotation discount") {
		t.Fatalf("the internal totals table is missing the discount lines:\n%s", got)
	}

	// "Row discounts" now states the genuine per-row total.
	assertLabelEndsIn(t, got, "Row discounts", "620.000")
	// "Whole-quotation discount" now states the whole-quotation figure, not the
	// always-zero original.
	assertLabelEndsIn(t, got, "Whole-quotation discount", "31.500")
}

// assertLabelEndsIn finds the printed table line beginning with label and
// requires its value column to be value. tabwriter keeps one row per line and
// pads between the columns with spaces, so the value is the trimmed line's
// suffix.
func assertLabelEndsIn(t *testing.T, table, label, value string) {
	t.Helper()
	for _, line := range strings.Split(table, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, label) {
			continue
		}
		if strings.HasSuffix(trimmed, value) {
			return
		}
		t.Errorf("%q prints %q; want its value column to be %q", label, trimmed, value)
		return
	}
	t.Errorf("the internal totals table has no line beginning with %q", label)
}
