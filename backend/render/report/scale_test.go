package report

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// chartScaleFixture is the contract the TypeScript chart reads as well. A
// change to NiceScale that the fixture does not record is a change the screen
// will not make, and the two documents would disagree about the same data.
type chartScaleFixture struct {
	Algorithm   string           `json:"algorithm"`
	Description string           `json:"description"`
	Tolerance   float64          `json:"tolerance"`
	Cases       []chartScaleCase `json:"cases"`
}

type chartScaleCase struct {
	Name     string          `json:"name"`
	Input    chartScaleInput `json:"input"`
	Expected Scale           `json:"expected"`
}

type chartScaleInput struct {
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	TargetTicks int     `json:"targetTicks"`
}

func loadChartScaleFixture(t *testing.T) chartScaleFixture {
	t.Helper()
	path := filepath.Join("testdata", "chart_scales.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture chartScaleFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatalf("%s has no cases", path)
	}
	if fixture.Tolerance <= 0 {
		fixture.Tolerance = 1e-9
	}
	return fixture
}

// TestNiceScaleMatchesTheSharedFixture is the Go half of the shared
// scale/axis contract with the TypeScript chart (#631).
func TestNiceScaleMatchesTheSharedFixture(t *testing.T) {
	fixture := loadChartScaleFixture(t)
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := NiceScale(tc.Input.Min, tc.Input.Max, tc.Input.TargetTicks)
			assertNear(t, "domain min", got.Min, tc.Expected.Min, fixture.Tolerance)
			assertNear(t, "domain max", got.Max, tc.Expected.Max, fixture.Tolerance)
			assertNear(t, "step", got.Step, tc.Expected.Step, fixture.Tolerance)
			if len(got.Ticks) != len(tc.Expected.Ticks) {
				t.Fatalf("ticks = %v (%d), want %v (%d)",
					got.Ticks, len(got.Ticks), tc.Expected.Ticks, len(tc.Expected.Ticks))
			}
			for i := range tc.Expected.Ticks {
				assertNear(t, "tick", got.Ticks[i], tc.Expected.Ticks[i], fixture.Tolerance)
			}
		})
	}
}

// TestNiceScaleProductionTickCounts pins the two target counts the renderer
// actually uses: 5 for a money axis and 4 for the margin percentage axis.
// DefaultTargetTicks is the money default and must be 5, not the old 6.
func TestNiceScaleProductionTickCounts(t *testing.T) {
	if DefaultTargetTicks != 5 {
		t.Fatalf("DefaultTargetTicks = %d, want 5 (the money chart's production target)", DefaultTargetTicks)
	}
	for _, target := range []int{4, 5} {
		scale := NiceScale(-1_250_000, 3_250_000, target)
		if len(scale.Ticks) < 2 {
			t.Errorf("target %d returned %d ticks", target, len(scale.Ticks))
		}
	}
}

// TestNiceScaleAlwaysIncludesZero makes the fixture's "zero-inclusive" rule a
// property rather than a coincidence of the cases: a chart column has to grow
// from zero, and an axis with no zero cannot show a loss.
func TestNiceScaleAlwaysIncludesZero(t *testing.T) {
	for _, tc := range []struct{ min, max float64 }{
		{1, 10}, {-10, -1}, {-3, 7}, {0, 0}, {1000000, 2000000},
	} {
		scale := NiceScale(tc.min, tc.max, DefaultTargetTicks)
		if scale.Min > 0 || scale.Max < 0 {
			t.Errorf("NiceScale(%v, %v) domain [%v, %v] excludes zero", tc.min, tc.max, scale.Min, scale.Max)
		}
		if len(scale.Ticks) < 2 {
			t.Errorf("NiceScale(%v, %v) returned %d ticks", tc.min, tc.max, len(scale.Ticks))
		}
	}
}

func assertNear(t *testing.T, what string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %v, want %v (tolerance %v)", what, got, want, tolerance)
	}
}
