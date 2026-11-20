package poreport

import (
	"fmt"
	"strings"
	"time"
)

const reportDateLayout = "2006-01-02"

// NormalizeQuery validates the report filter and returns its canonical form.
// It requires half-open YYYY-MM-DD dates (from inclusive, to exclusive), one of
// the four buckets, one of the six categories (empty means all), a nonempty
// currency, and a range intersecting at most MaxPeriods actual calendar
// buckets. A day bucket is additionally confined to one calendar month and at
// most MaxDayPeriods days.
//
// Currency catalogue membership is deliberately not checked here: the API and
// the store own the currency list. Empty category is normalized to "all" so a
// caller that omits the filter gets the documented default.
func NormalizeQuery(query Query) (Query, error) {
	query.From = strings.TrimSpace(query.From)
	query.To = strings.TrimSpace(query.To)
	query.Bucket = strings.TrimSpace(query.Bucket)
	query.Category = strings.TrimSpace(query.Category)
	query.Currency = strings.TrimSpace(query.Currency)

	from, err := parseReportDate(query.From)
	if err != nil {
		return Query{}, fmt.Errorf("%w: from %q: %v", ErrInvalid, query.From, err)
	}
	to, err := parseReportDate(query.To)
	if err != nil {
		return Query{}, fmt.Errorf("%w: to %q: %v", ErrInvalid, query.To, err)
	}
	if !from.Before(to) {
		return Query{}, fmt.Errorf("%w: from %s must be before to %s", ErrInvalid, query.From, query.To)
	}

	switch query.Bucket {
	case BucketDay, BucketMonth, BucketQuarter, BucketYear:
	default:
		return Query{}, fmt.Errorf("%w: bucket %q must be day, month, quarter or year", ErrInvalid, query.Bucket)
	}

	if query.Category == "" {
		query.Category = CategoryAll
	}
	switch query.Category {
	case CategoryAll, CategoryFm, CategoryFab, CategorySupplyGeneral, CategoryOthers, CategoryUnassigned:
	default:
		return Query{}, fmt.Errorf(
			"%w: category %q must be all, FM, FAB, SUPPLY_GEN, OTHERS or UNASSIGNED",
			ErrInvalid, query.Category)
	}

	if query.Currency == "" {
		return Query{}, fmt.Errorf("%w: currency is required", ErrInvalid)
	}

	if query.Bucket == BucketDay {
		lastIncluded := to.AddDate(0, 0, -1)
		if from.Year() != lastIncluded.Year() || from.Month() != lastIncluded.Month() {
			return Query{}, fmt.Errorf(
				"%w: a day range must stay within one calendar month; %s..%s spans %s..%s",
				ErrInvalid, query.From, query.To,
				from.Format(reportDateLayout), lastIncluded.Format(reportDateLayout))
		}
		periods := intersectedPeriods(from, to, query.Bucket)
		if periods > MaxDayPeriods {
			return Query{}, fmt.Errorf(
				"%w: %s..%s by day intersects %d days; the cap is %d",
				ErrRangeTooLarge, query.From, query.To, periods, MaxDayPeriods)
		}
		return query, nil
	}

	periods := intersectedPeriods(from, to, query.Bucket)
	if periods > MaxPeriods {
		return Query{}, fmt.Errorf(
			"%w: %s..%s by %s intersects %d periods; the cap is %d",
			ErrRangeTooLarge, query.From, query.To, query.Bucket, periods, MaxPeriods)
	}

	return query, nil
}

// parseReportDate accepts only a zero-padded, real calendar date. time.Parse
// alone would also accept a value that formats differently, so the round trip
// is what pins the wire spelling.
func parseReportDate(value string) (time.Time, error) {
	date, err := time.Parse(reportDateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("not a valid calendar date")
	}
	if date.Format(reportDateLayout) != value {
		return time.Time{}, fmt.Errorf("not a zero-padded YYYY-MM-DD date")
	}
	if date.Year() < 1 {
		return time.Time{}, fmt.Errorf("year must be at least 1")
	}
	return date, nil
}

// intersectedPeriods counts the calendar buckets a half-open range touches.
// The day before To is the last included day, so a range ending exactly on a
// month, quarter or year start does not count the empty bucket.
func intersectedPeriods(from, to time.Time, bucket string) int64 {
	lastIncluded := to.AddDate(0, 0, -1)
	switch bucket {
	case BucketDay:
		return int64(lastIncluded.Sub(from)/(24*time.Hour)) + 1
	case BucketMonth:
		return monthIndex(lastIncluded) - monthIndex(from) + 1
	case BucketQuarter:
		return monthIndex(lastIncluded)/3 - monthIndex(from)/3 + 1
	case BucketYear:
		return int64(lastIncluded.Year()) - int64(from.Year()) + 1
	default:
		return 0
	}
}

// monthIndex gives a month a single increasing integer. Years are positive and
// bounded by the parsed four-digit date, so the value cannot overflow int64.
func monthIndex(date time.Time) int64 {
	return int64(date.Year())*12 + int64(date.Month()) - 1
}

func firstOfMonth(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func firstOfNextMonth(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}
