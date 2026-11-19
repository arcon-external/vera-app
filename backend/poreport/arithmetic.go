package poreport

import (
	"fmt"
	"math/big"
	"slices"
)

// CheckExact refuses a value that a JavaScript number cannot name exactly.
// Signed values are bounded at both ends because margin can be a loss.
func CheckExact(value int64) error {
	if value > MaxExactMinor || value < -MaxExactMinor {
		return fmt.Errorf(
			"%w: %d is outside -%d..%d", ErrValueOutOfRange, value, MaxExactMinor, MaxExactMinor)
	}
	return nil
}

// RatioBps returns numerator/denominator in basis points, rounded half away
// from zero. It returns nil when the denominator is not positive: a percentage
// with no capital is unknown, not zero. The multiplication happens in big.Int
// so two int64 operands cannot wrap before rounding, and a result outside the
// exact JSON range is refused rather than emitted.
func RatioBps(numerator, denominator int64) (*int64, error) {
	if denominator <= 0 {
		return nil, nil
	}
	scaled := new(big.Int).Mul(big.NewInt(numerator), big.NewInt(10000))
	result := roundQuotientAwayFromZero(scaled, big.NewInt(denominator))
	value, err := exactInt64(result)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// percentileInc is Excel's PERCENTILE.INC over an int64 sample: the p-th
// percentile at rank (n-1)*p, linearly interpolated, then rounded half away
// from zero. It is the one quantile definition the report uses for medians,
// quartiles and yearly rolling-window bands. The sample is sorted in a copy.
func percentileInc(values []int64, numerator, denominator int64) (int64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("%w: a percentile needs at least one value", ErrInvalid)
	}
	if denominator <= 0 || numerator < 0 || numerator > denominator {
		return 0, fmt.Errorf("%w: percentile %d/%d is not a fraction between zero and one",
			ErrInvalid, numerator, denominator)
	}

	ordered := slices.Clone(values)
	slices.Sort(ordered)

	rank := new(big.Int).Mul(big.NewInt(int64(len(ordered)-1)), big.NewInt(numerator))
	index, remainder := new(big.Int).QuoRem(rank, big.NewInt(denominator), new(big.Int))
	if remainder.Sign() == 0 {
		return exactInt64(big.NewInt(ordered[index.Int64()]))
	}

	lower := big.NewInt(ordered[index.Int64()])
	upper := big.NewInt(ordered[index.Int64()+1])
	denominatorBig := big.NewInt(denominator)
	lowerWeight := new(big.Int).Sub(denominatorBig, remainder)
	weighted := new(big.Int).Mul(lower, lowerWeight)
	weighted.Add(weighted, new(big.Int).Mul(upper, remainder))
	return exactInt64(roundQuotientAwayFromZero(weighted, denominatorBig))
}

// roundQuotientAwayFromZero divides two integers and rounds an exact half away
// from zero. The denominator must be positive. QuoRem truncates toward zero, so
// the remainder carries the numerator's sign; adding that sign on a half is
// what makes -2.5 round to -3 rather than -2.
func roundQuotientAwayFromZero(numerator, denominator *big.Int) *big.Int {
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if remainder.Sign() == 0 {
		return quotient
	}
	twice := new(big.Int).Abs(remainder)
	twice.Lsh(twice, 1)
	if twice.Cmp(denominator) >= 0 {
		if numerator.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return quotient
}

// exactInt64 converts a big.Int result to the exact JSON range or refuses it.
func exactInt64(value *big.Int) (int64, error) {
	if !value.IsInt64() {
		return 0, fmt.Errorf("%w: %s does not fit an int64", ErrValueOutOfRange, value.String())
	}
	result := value.Int64()
	if err := CheckExact(result); err != nil {
		return 0, err
	}
	return result, nil
}
