package poreport

import (
	"errors"
	"math"
	"testing"
)

func TestCheckExactBoundaries(t *testing.T) {
	t.Parallel()

	for _, value := range []int64{-MaxExactMinor, 0, MaxExactMinor} {
		if err := CheckExact(value); err != nil {
			t.Errorf("CheckExact(%d) error = %v; want nil", value, err)
		}
	}
	for _, value := range []int64{MaxExactMinor + 1, -MaxExactMinor - 1, math.MaxInt64, math.MinInt64} {
		if err := CheckExact(value); !errors.Is(err, ErrValueOutOfRange) {
			t.Errorf("CheckExact(%d) error = %v; want ErrValueOutOfRange", value, err)
		}
	}
}

func TestRatioBpsRoundsHalfAwayFromZeroExactly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		numerator   int64
		denominator int64
		want        int64
	}{
		{name: "exact", numerator: 1, denominator: 4, want: 2500},
		{name: "truncates below a half", numerator: 1, denominator: 3, want: 3333},
		{name: "rounds above a half", numerator: 2, denominator: 3, want: 6667},
		{name: "exactly a half rounds away", numerator: 1, denominator: 20000, want: 1},
		{name: "below a half rounds to zero", numerator: 1, denominator: 40000, want: 0},
		{name: "three halves round away", numerator: 3, denominator: 20000, want: 2},
		{name: "negative exactly a half rounds away", numerator: -1, denominator: 20000, want: -1},
		{name: "negative below a half truncates", numerator: -1, denominator: 40000, want: 0},
		{name: "negative thirds", numerator: -2, denominator: 3, want: -6667},
		{name: "zero numerator", numerator: 0, denominator: 7, want: 0},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := RatioBps(test.numerator, test.denominator)
			if err != nil {
				t.Fatalf("RatioBps(%d, %d) error = %v", test.numerator, test.denominator, err)
			}
			if got == nil {
				t.Fatalf("RatioBps(%d, %d) = nil; want %d", test.numerator, test.denominator, test.want)
			}
			if *got != test.want {
				t.Errorf("RatioBps(%d, %d) = %d; want %d", test.numerator, test.denominator, *got, test.want)
			}
		})
	}
}

func TestRatioBpsReturnsNilWithoutAPositiveDenominator(t *testing.T) {
	t.Parallel()

	for _, denominator := range []int64{0, -1, math.MinInt64} {
		got, err := RatioBps(123, denominator)
		if err != nil {
			t.Errorf("RatioBps(123, %d) error = %v; want nil", denominator, err)
		}
		if got != nil {
			t.Errorf("RatioBps(123, %d) = %d; want nil", denominator, *got)
		}
	}
}

func TestRatioBpsUsesWideIntermediatesAndRefusesAnInexactResult(t *testing.T) {
	t.Parallel()

	got, err := RatioBps(MaxExactMinor, MaxExactMinor)
	if err != nil {
		t.Fatalf("RatioBps(max, max) error = %v", err)
	}
	if got == nil || *got != 10000 {
		t.Fatalf("RatioBps(max, max) = %v; want 10000", got)
	}

	got, err = RatioBps(math.MaxInt64, math.MaxInt64)
	if err != nil {
		t.Fatalf("RatioBps(MaxInt64, MaxInt64) error = %v", err)
	}
	if got == nil || *got != 10000 {
		t.Fatalf("RatioBps(MaxInt64, MaxInt64) = %v; want 10000", got)
	}

	if _, err := RatioBps(MaxExactMinor, 1); !errors.Is(err, ErrValueOutOfRange) {
		t.Errorf("RatioBps(max, 1) error = %v; want ErrValueOutOfRange", err)
	}
	if _, err := RatioBps(math.MinInt64, 1); !errors.Is(err, ErrValueOutOfRange) {
		t.Errorf("RatioBps(MinInt64, 1) error = %v; want ErrValueOutOfRange", err)
	}
}

func TestPercentileIncInterpolatesLinearlyAndRoundsHalfAwayFromZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		values      []int64
		numerator   int64
		denominator int64
		want        int64
	}{
		{name: "single value", values: []int64{7}, numerator: 1, denominator: 4, want: 7},
		{name: "median even averages", values: []int64{1, 2, 3, 4}, numerator: 1, denominator: 2, want: 3},
		{name: "p25", values: []int64{1, 2, 3, 4}, numerator: 1, denominator: 4, want: 2},
		{name: "p75", values: []int64{1, 2, 3, 4}, numerator: 3, denominator: 4, want: 3},
		{name: "median odd", values: []int64{1, 2, 3, 4, 5}, numerator: 1, denominator: 2, want: 3},
		{name: "p25 odd", values: []int64{1, 2, 3, 4, 5}, numerator: 1, denominator: 4, want: 2},
		{name: "p75 odd", values: []int64{1, 2, 3, 4, 5}, numerator: 3, denominator: 4, want: 4},
		{name: "negative median half rounds away", values: []int64{-4, -3, -2, -1}, numerator: 1, denominator: 2, want: -3},
		{name: "negative interpolation", values: []int64{-4, -3, -2, -1}, numerator: 1, denominator: 4, want: -3},
		{
			name:        "interpolation at the exact ceiling rounds to the ceiling",
			values:      []int64{MaxExactMinor - 1, MaxExactMinor},
			numerator:   1,
			denominator: 2,
			want:        MaxExactMinor,
		},
		{
			name:        "negative interpolation at the exact floor",
			values:      []int64{-MaxExactMinor, -MaxExactMinor + 1},
			numerator:   1,
			denominator: 2,
			want:        -MaxExactMinor,
		},
		{
			name:        "unsorted input is ordered first",
			values:      []int64{4, 1, 3, 2},
			numerator:   1,
			denominator: 2,
			want:        3,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := percentileInc(test.values, test.numerator, test.denominator)
			if err != nil {
				t.Fatalf("percentileInc(%v, %d/%d) error = %v", test.values, test.numerator, test.denominator, err)
			}
			if got != test.want {
				t.Errorf("percentileInc(%v, %d/%d) = %d; want %d", test.values, test.numerator, test.denominator, got, test.want)
			}
		})
	}
}

func TestPercentileIncRejectsEmptyAndInexactResults(t *testing.T) {
	t.Parallel()

	if _, err := percentileInc(nil, 1, 2); !errors.Is(err, ErrInvalid) {
		t.Errorf("percentileInc(nil) error = %v; want ErrInvalid", err)
	}
	if _, err := percentileInc([]int64{1}, 1, 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("percentileInc(denominator=0) error = %v; want ErrInvalid", err)
	}
	if _, err := percentileInc([]int64{math.MaxInt64, math.MaxInt64}, 1, 2); !errors.Is(err, ErrValueOutOfRange) {
		t.Errorf("percentileInc(MaxInt64) error = %v; want ErrValueOutOfRange", err)
	}
}
