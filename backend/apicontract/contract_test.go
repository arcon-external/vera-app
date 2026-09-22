package apicontract

import "testing"

func TestFormatQuoteNumber(t *testing.T) {
	cases := []struct {
		customerCode int
		yy           int
		seq          int64
		want         string
	}{
		{2, 26, 1, "1502260001"},
		{1, 26, 112, "1501260112"},
		{2, 26, 777, "1502260777"},
		{6, 26, 42, "1506260042"},
		{2, 27, 1, "1502270001"},
		{2, 26, 9999, "1502269999"},
		// Overflow widens to 5 digits instead of wrapping.
		{2, 26, 10000, "15022610000"},
		{2, 26, 10001, "15022610001"},
	}
	for _, tc := range cases {
		if got := FormatQuoteNumber(tc.customerCode, tc.yy, tc.seq); got != tc.want {
			t.Errorf("FormatQuoteNumber(%d, %d, %d) = %q; want %q",
				tc.customerCode, tc.yy, tc.seq, got, tc.want)
		}
	}
}
