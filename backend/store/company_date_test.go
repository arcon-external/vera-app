package store

import (
	"testing"
	"time"
)

// Issue #468: the received date defaults to "today" in the company's date
// context, not the server's. The office works in WIB (UTC+7); a UTC build
// running on Cloud Run must still date an order on the Jakarta calendar.
func TestCompanyTodayUsesWIB(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		// The Judge's boundary: 18:30 UTC is 01:30 the next day in Jakarta.
		{"midnight boundary", time.Date(2026, 10, 20, 18, 30, 0, 0, time.UTC), "2026-10-21"},
		{"just before WIB midnight", time.Date(2026, 10, 20, 16, 59, 59, 0, time.UTC), "2026-10-20"},
		{"exactly WIB midnight", time.Date(2026, 10, 20, 17, 0, 0, 0, time.UTC), "2026-10-21"},
		{"already WIB morning", time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC), "2026-10-20"},
		// The instant is what matters, not the time.Time's location: a machine
		// in another zone still dates in Jakarta.
		{"west-coast input", time.Date(2026, 10, 20, 11, 30, 0, 0,
			time.FixedZone("PDT", -7*60*60)), "2026-10-21"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := companyToday(testCase.now).Format("2006-01-02")
			if got != testCase.want {
				t.Errorf("companyToday(%s) = %s; want %s",
					testCase.now.Format(time.RFC3339), got, testCase.want)
			}
			if hour, minute, second := companyToday(testCase.now).Clock(); hour != 0 || minute != 0 || second != 0 {
				t.Errorf("companyToday(%s) carries a time of day", testCase.now)
			}
		})
	}
}

// The seed that lands in the column must stay a day count in words, because a
// Quotation revision stores only payment_term_days. This is the only stable
// wording the source can supply; the PO field is free text from there.
func TestQuotationPaymentTermsSeedsTheIssuedDayCount(t *testing.T) {
	if got := quotationPaymentTerms(45); got != "45 days" {
		t.Errorf("quotationPaymentTerms(45) = %q; want %q", got, "45 days")
	}
	if got := quotationPaymentTerms(1); got != "1 day" {
		t.Errorf("quotationPaymentTerms(1) = %q; want %q", got, "1 day")
	}
	if got := quotationPaymentTerms(0); got != "" {
		t.Errorf("quotationPaymentTerms(0) = %q; want empty", got)
	}
}
