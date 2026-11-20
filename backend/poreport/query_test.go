package poreport

import (
	"errors"
	"testing"
)

func TestNormalizeQueryDefaultsCategoryAndAcceptsHalfOpenRange(t *testing.T) {
	t.Parallel()

	query, err := NormalizeQuery(Query{
		From:     "2026-01-01",
		To:       "2026-04-01",
		Bucket:   "month",
		Category: "",
		Currency: "IDR",
	})
	if err != nil {
		t.Fatalf("NormalizeQuery() error = %v", err)
	}
	if query.Category != CategoryAll {
		t.Errorf("Category = %q; want %q", query.Category, CategoryAll)
	}
	if query.From != "2026-01-01" || query.To != "2026-04-01" {
		t.Errorf("range = %s..%s; want 2026-01-01..2026-04-01", query.From, query.To)
	}
}

func TestNormalizeQueryTrimsAndCanonicalizesWhitespace(t *testing.T) {
	t.Parallel()

	query, err := NormalizeQuery(Query{
		From:     " 2026-02-01 ",
		To:       " 2026-03-01 ",
		Bucket:   " quarter ",
		Category: " FAB ",
		Currency: " IDR ",
	})
	if err != nil {
		t.Fatalf("NormalizeQuery() error = %v", err)
	}
	if query.From != "2026-02-01" || query.To != "2026-03-01" {
		t.Errorf("range = %q..%q; want trimmed dates", query.From, query.To)
	}
	if query.Bucket != BucketQuarter || query.Category != CategoryFab || query.Currency != "IDR" {
		t.Errorf("normalized query = %+v", query)
	}
}

func TestNormalizeQueryRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query Query
	}{
		{
			name:  "empty from",
			query: Query{To: "2026-02-01", Bucket: "month", Category: "all", Currency: "IDR"},
		},
		{
			name:  "unpadded date",
			query: Query{From: "2026-1-01", To: "2026-02-01", Bucket: "month", Category: "all", Currency: "IDR"},
		},
		{
			name:  "impossible calendar date",
			query: Query{From: "2023-02-29", To: "2023-03-01", Bucket: "month", Category: "all", Currency: "IDR"},
		},
		{
			name:  "same day",
			query: Query{From: "2026-01-01", To: "2026-01-01", Bucket: "month", Category: "all", Currency: "IDR"},
		},
		{
			name:  "reversed range",
			query: Query{From: "2026-02-01", To: "2026-01-01", Bucket: "month", Category: "all", Currency: "IDR"},
		},
		{
			name:  "unknown bucket",
			query: Query{From: "2026-01-01", To: "2026-02-01", Bucket: "week", Category: "all", Currency: "IDR"},
		},
		{
			name:  "uppercase bucket",
			query: Query{From: "2026-01-01", To: "2026-02-01", Bucket: "MONTH", Category: "all", Currency: "IDR"},
		},
		{
			name:  "unknown category",
			query: Query{From: "2026-01-01", To: "2026-02-01", Bucket: "month", Category: "fm", Currency: "IDR"},
		},
		{
			name:  "empty currency",
			query: Query{From: "2026-01-01", To: "2026-02-01", Bucket: "month", Category: "all", Currency: "   "},
		},
		{
			name:  "year zero",
			query: Query{From: "0000-01-01", To: "0000-02-01", Bucket: "month", Category: "all", Currency: "IDR"},
		},
		{
			name:  "day range spans two months",
			query: Query{From: "2026-01-31", To: "2026-02-02", Bucket: "day", Category: "all", Currency: "IDR"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NormalizeQuery(test.query); !errors.Is(err, ErrInvalid) {
				t.Fatalf("NormalizeQuery(%+v) error = %v; want ErrInvalid", test.query, err)
			}
		})
	}
}

func TestNormalizeQueryAcceptsEveryCategoryAndBucket(t *testing.T) {
	t.Parallel()

	for _, category := range []string{
		CategoryAll,
		CategoryFm,
		CategoryFab,
		CategorySupplyGeneral,
		CategoryOthers,
		CategoryUnassigned,
	} {
		query := validQuery()
		query.Category = category
		if _, err := NormalizeQuery(query); err != nil {
			t.Errorf("NormalizeQuery(category=%q) error = %v", category, err)
		}
	}

	for _, bucket := range []string{BucketMonth, BucketQuarter, BucketYear} {
		query := validQuery()
		query.Bucket = bucket
		if _, err := NormalizeQuery(query); err != nil {
			t.Errorf("NormalizeQuery(bucket=%q) error = %v", bucket, err)
		}
	}

	day := Query{From: "2026-01-01", To: "2026-02-01", Bucket: BucketDay, Category: CategoryAll, Currency: "IDR"}
	if _, err := NormalizeQuery(day); err != nil {
		t.Errorf("NormalizeQuery(bucket=day) error = %v", err)
	}
}

func TestNormalizeQueryCountsIntersectedCalendarBuckets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		from    string
		to      string
		bucket  string
		wantErr error
	}{
		{name: "one partial month", from: "2026-01-15", to: "2026-01-16", bucket: "month"},
		{name: "two months to a month start", from: "2026-01-15", to: "2026-03-01", bucket: "month"},
		{name: "three months through a month start", from: "2026-01-15", to: "2026-03-02", bucket: "month"},
		{name: "one day at a month end", from: "2026-01-31", to: "2026-02-01", bucket: "month"},
		{name: "120 month buckets", from: "2016-01-01", to: "2026-01-01", bucket: "month"},
		{name: "121 month buckets", from: "2015-12-01", to: "2026-01-01", bucket: "month", wantErr: ErrRangeTooLarge},
		{name: "120 quarter buckets", from: "1996-01-01", to: "2026-01-01", bucket: "quarter"},
		{name: "121 quarter buckets", from: "1995-10-01", to: "2026-01-01", bucket: "quarter", wantErr: ErrRangeTooLarge},
		{name: "120 year buckets", from: "1906-01-01", to: "2026-01-01", bucket: "year"},
		{name: "121 year buckets", from: "1905-01-01", to: "2026-01-01", bucket: "year", wantErr: ErrRangeTooLarge},
		{name: "one day bucket", from: "2026-01-31", to: "2026-02-01", bucket: "day"},
		{name: "31 day buckets", from: "2026-01-01", to: "2026-02-01", bucket: "day"},
		{name: "day range into the next month", from: "2026-01-31", to: "2026-02-02", bucket: "day", wantErr: ErrInvalid},
		{name: "day range across a year start", from: "2026-12-31", to: "2027-01-02", bucket: "day", wantErr: ErrInvalid},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			query := Query{
				From: test.from, To: test.to, Bucket: test.bucket,
				Category: CategoryAll, Currency: "IDR",
			}
			_, err := NormalizeQuery(query)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("NormalizeQuery(%s..%s, %s) error = %v; want %v", test.from, test.to, test.bucket, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeQuery(%s..%s, %s) error = %v; want nil", test.from, test.to, test.bucket, err)
			}
		})
	}
}

func validQuery() Query {
	return Query{
		From:     "2026-01-01",
		To:       "2027-01-01",
		Bucket:   BucketMonth,
		Category: CategoryAll,
		Currency: "IDR",
	}
}
