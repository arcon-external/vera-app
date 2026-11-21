package backend

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/auth"
	"vera-app/backend/poreport"
	"vera-app/backend/render/report"
)

func int64Pointer(value int64) *int64 { return &value }

// reportExportFixture is a minimal valid snapshot: one actual period and both
// projections unavailable with their stated reason. Its filter is what the
// save dialog names the file from.
func reportExportFixture() poreport.Report {
	basis := poreport.ProjectionBasis{AsOf: "2026-01-04"}
	return poreport.Report{
		GeneratedAt: "2026-01-05T08:30:00Z",
		AsOf:        "2026-01-04",
		Filter: poreport.Query{
			From: "2025-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
			Category: poreport.CategoryAll, Currency: "IDR",
		},
		Company: poreport.Company{Name: "PT. Arcon Penta Persada", Address: "Batam"},
		Totals: poreport.Metrics{
			PoCount: 1, AmountMinor: 1_000_000, CostedMinor: 900_000, UncostedMinor: 100_000,
			CapitalMinor: 700_000, MarginMinor: 200_000,
			MarginBps: int64Pointer(2857), CoverageBps: int64Pointer(9000),
		},
		Periods: []poreport.Period{{
			PeriodStart: "2025-01-01",
			Metrics: poreport.Metrics{
				PoCount: 1, AmountMinor: 1_000_000, CostedMinor: 900_000, UncostedMinor: 100_000,
				CapitalMinor: 700_000, MarginMinor: 200_000,
				MarginBps: int64Pointer(2857), CoverageBps: int64Pointer(9000),
			},
		}},
		Projection: poreport.Projection{
			Monthly: poreport.ProjectionSeries{
				Status: poreport.ProjectionStatusUnavailable,
				Reason: poreport.ReasonNoDataInRange,
				Method: poreport.MethodMonthly,
				Basis:  basis,
			},
			Yearly: poreport.ProjectionSeries{
				Status: poreport.ProjectionStatusUnavailable,
				Reason: poreport.ReasonNoDataInRange,
				Method: poreport.MethodYearly,
				Basis:  basis,
			},
		},
	}
}

func reportJSON(t *testing.T, snapshot poreport.Report) string {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	return string(raw)
}

func staffQuotation() *Quotation {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	return q
}

func TestExportPurchaseOrderReportWritesTheChosenFile(t *testing.T) {
	q := staffQuotation()
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")

	var gotOptions runtime.SaveDialogOptions
	previousDialog := saveFileDialog
	saveFileDialog = func(_ context.Context, options runtime.SaveDialogOptions) (string, error) {
		gotOptions = options
		return path, nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	// The export must not reach the API: nil is the strongest possible proof.
	previousClient := apiClient
	apiClient = nil
	t.Cleanup(func() { apiClient = previousClient })

	got, err := q.ExportPurchaseOrderReport(reportJSON(t, reportExportFixture()))
	if err != nil {
		t.Fatalf("ExportPurchaseOrderReport: %v", err)
	}
	if got != path {
		t.Errorf("returned path = %q, want %q", got, path)
	}
	if gotOptions.Title != "Save Purchase Order Report" {
		t.Errorf("dialog title = %q", gotOptions.Title)
	}
	if gotOptions.DefaultFilename != "Purchase_Order_Report_2025-01-01_to_2026-01-01.pdf" {
		t.Errorf("dialog filename = %q", gotOptions.DefaultFilename)
	}
	if len(gotOptions.Filters) != 1 || gotOptions.Filters[0].Pattern != "*.pdf" {
		t.Errorf("dialog filters = %+v", gotOptions.Filters)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written report: %v", err)
	}
	if !strings.HasPrefix(string(raw), "%PDF") {
		t.Fatalf("written file is not a PDF (%d bytes)", len(raw))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("report mode = %04o, want 0644", info.Mode().Perm())
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".report-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Errorf("temporary report files left behind: %v", temps)
	}
}

func TestExportPurchaseOrderReportCancelReturnsEmptyPath(t *testing.T) {
	q := staffQuotation()
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		return "", nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	got, err := q.ExportPurchaseOrderReport(reportJSON(t, reportExportFixture()))
	if err != nil {
		t.Errorf("cancel returned an error: %v", err)
	}
	if got != "" {
		t.Errorf("cancel returned %q, want an empty path", got)
	}
}

func TestExportPurchaseOrderReportRefusesACorruptSnapshotBeforeTheDialog(t *testing.T) {
	q := staffQuotation()
	previousDialog := saveFileDialog
	dialogOpened := false
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		dialogOpened = true
		return filepath.Join(t.TempDir(), "never.pdf"), nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	if _, err := q.ExportPurchaseOrderReport("{not json"); err == nil {
		t.Fatalf("malformed snapshot was accepted")
	}
	if dialogOpened {
		t.Errorf("the save dialog opened for a snapshot the renderer refused")
	}

	unknownCurrency := reportExportFixture()
	unknownCurrency.Filter.Currency = "XYZ"
	if _, err := q.ExportPurchaseOrderReport(reportJSON(t, unknownCurrency)); err == nil {
		t.Errorf("an unresolved currency was accepted instead of being refused")
	}
	if dialogOpened {
		t.Errorf("the save dialog opened for an unresolved currency")
	}

	tooLarge := reportExportFixture()
	tooLarge.Periods = make([]poreport.Period, poreport.MaxPeriods+1)
	if _, err := q.ExportPurchaseOrderReport(reportJSON(t, tooLarge)); err == nil {
		t.Errorf("an oversized period array was accepted")
	}
	if dialogOpened {
		t.Errorf("the save dialog opened for an oversized period array")
	}
}

func TestExportPurchaseOrderReportValidatesTheChosenPath(t *testing.T) {
	q := staffQuotation()
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		return filepath.Join(t.TempDir(), "report.txt"), nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	if _, err := q.ExportPurchaseOrderReport(reportJSON(t, reportExportFixture())); err == nil {
		t.Errorf("a non-PDF destination was accepted")
	}
}

func TestExportPurchaseOrderReportRequiresASignedInUser(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	setSessionTokenMemory("")
	q := newTestQuotation()
	previousClient := apiClient
	apiClient = nil
	t.Cleanup(func() { apiClient = previousClient })
	if _, err := q.ExportPurchaseOrderReport(reportJSON(t, reportExportFixture())); err == nil {
		t.Errorf("an unauthenticated export was accepted")
	}
}

// Issue #641: the same binding accepts a versioned view without changing its
// signature, but a view still has to be validated before the save dialog opens.
// The selected month is frozen in the payload; the renderer never fetches.
func reportViewFixture() report.ViewEnvelope {
	return report.ViewEnvelope{
		Version:  report.ViewVersion,
		Sources:  []poreport.Report{reportExportFixture()},
		Overview: 0,
		Periods: report.ViewPeriods{
			Sources: []int{0},
			Months:  []string{"2025-01"},
			Layout:  report.ViewLayoutIndividual,
		},
		Categories: report.ViewCategories{
			Source: 0,
			Codes:  []string{},
			Layout: report.ViewLayoutAggregate,
		},
	}
}

func viewJSON(t *testing.T, view report.ViewEnvelope) string {
	t.Helper()
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	return string(raw)
}

func TestExportPurchaseOrderReportWritesAVersionedView(t *testing.T) {
	q := staffQuotation()
	dir := t.TempDir()
	path := filepath.Join(dir, "selected-month.pdf")
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		return path, nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })
	previousClient := apiClient
	apiClient = nil
	t.Cleanup(func() { apiClient = previousClient })

	got, err := q.ExportPurchaseOrderReport(viewJSON(t, reportViewFixture()))
	if err != nil {
		t.Fatalf("ExportPurchaseOrderReport(view): %v", err)
	}
	if got != path {
		t.Errorf("returned path = %q, want %q", got, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written view: %v", err)
	}
	if !strings.HasPrefix(string(raw), "%PDF") {
		t.Fatalf("written view is not a PDF (%d bytes)", len(raw))
	}
}

func TestExportPurchaseOrderReportRefusesAnInvalidViewBeforeTheDialog(t *testing.T) {
	base := reportViewFixture()
	badVersion := base
	badVersion.Version = report.EnvelopeVersion(2)

	badFields := json.RawMessage(viewJSON(t, base))
	var object map[string]json.RawMessage
	if err := json.Unmarshal(badFields, &object); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	object["selection"] = json.RawMessage(`{"layout":"individual"}`)
	withUnknown, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal unknown-field view: %v", err)
	}

	badRef := base
	badRef.Periods.Sources = []int{4}

	badMonth := base
	badMonth.Periods.Months = []string{"2025-02"}

	badCurrency := base
	other := reportExportFixture()
	other.Filter.Currency = "USD"
	badCurrency.Sources = []poreport.Report{base.Sources[0], other}

	asOfMismatch := base
	other = reportExportFixture()
	other.AsOf = "2026-01-05"
	other.Filter.From = "2024-01-01"
	other.Filter.To = "2025-01-01"
	other.Projection.Monthly.Basis.AsOf = "2026-01-05"
	other.Projection.Yearly.Basis.AsOf = "2026-01-05"
	asOfMismatch.Sources = []poreport.Report{base.Sources[0], other}

	legacyWithSelection := map[string]any{}
	legacySource := reportExportFixture()
	legacyRaw, err := json.Marshal(legacySource)
	if err != nil {
		t.Fatalf("marshal legacy source: %v", err)
	}
	if err := json.Unmarshal(legacyRaw, &legacyWithSelection); err != nil {
		t.Fatalf("unmarshal legacy source: %v", err)
	}
	legacyWithSelection["selection"] = map[string]any{"months": []string{"2025-01"}}
	legacyMutated, err := json.Marshal(legacyWithSelection)
	if err != nil {
		t.Fatalf("marshal legacy mutation: %v", err)
	}

	cases := []struct {
		name    string
		payload string
	}{
		{name: "unknown version", payload: viewJSON(t, badVersion)},
		{name: "unknown field", payload: string(withUnknown)},
		{name: "missing annual source", payload: viewJSON(t, badRef)},
		{name: "month not covered", payload: viewJSON(t, badMonth)},
		{name: "foreign currency source", payload: viewJSON(t, badCurrency)},
		{name: "as-of mismatch", payload: viewJSON(t, asOfMismatch)},
		{name: "legacy with selection", payload: string(legacyMutated)},
		{name: "zero version", payload: `{"version":0,"sources":[],"overview":0,"periods":{"sources":[],"months":[],"layout":"individual"},"categories":{"source":0,"codes":[],"layout":"aggregate"}}`},
		{name: "null version", payload: `{"version":null,"sources":[],"overview":0,"periods":{"sources":[],"months":[],"layout":"individual"},"categories":{"source":0,"codes":[],"layout":"aggregate"}}`},
	}

	q := staffQuotation()
	dialogOpened := false
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		dialogOpened = true
		return filepath.Join(t.TempDir(), "never.pdf"), nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dialogOpened = false
			if _, err := q.ExportPurchaseOrderReport(test.payload); err == nil {
				t.Fatal("invalid view was accepted")
			}
			if dialogOpened {
				t.Fatal("the save dialog opened before the invalid view was refused")
			}
		})
	}
}
