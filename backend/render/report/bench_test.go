package report

import (
	"os"
	"testing"
)

// Benchmarks cover the two document shapes the export is judged on: a typical
// year (12 actual periods) and the maximum 120-period history. They load the
// fonts on every call, exactly as a fresh export does, so the number includes
// the work a staff member waits for rather than a cached inner loop.
func BenchmarkRenderTypicalReport(b *testing.B) {
	snapshot := sampleReport()
	fonts := os.DirFS("../../assets")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Render(snapshot, fonts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRender120PeriodReport(b *testing.B) {
	snapshot := longReport(120)
	fonts := os.DirFS("../../assets")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Render(snapshot, fonts); err != nil {
			b.Fatal(err)
		}
	}
}

// The two current-period shapes are the reports staff export most often: a
// 31-day month and a 12-month year with the current month stacked.
func BenchmarkRenderDayReport(b *testing.B) {
	snapshot := dayReport()
	fonts := os.DirFS("../../assets")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Render(snapshot, fonts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderCurrentYearReport(b *testing.B) {
	snapshot := currentYearReport()
	fonts := os.DirFS("../../assets")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Render(snapshot, fonts); err != nil {
			b.Fatal(err)
		}
	}
}
