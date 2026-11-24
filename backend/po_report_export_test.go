package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/apiclient"
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

// adminQuotation is a signed-in admin binding. Issue #683 makes the report
// exports admin-only, so the successful-path tests must carry the role the
// route and the binding now require. This is the injected-service seam; the
// desktop shape (no auth service, role read from the server) is covered by
// TestExportPurchaseOrderReportAllowsAnAdminInTheDesktopShape.
func adminQuotation() *Quotation {
	q := newTestQuotation()
	q.SetAuthService(auth.AdminTestService())
	return q
}

// staffQuotation is a signed-in staff binding: the role that must be refused.
// The desktop never builds this seam; the shipped path is covered by
// TestExportPurchaseOrderReportRefusesStaffInTheDesktopShape, which attaches no
// auth service and lets the fake server answer the role.
func staffQuotation() *Quotation {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	return q
}

func TestExportPurchaseOrderReportWritesTheChosenFile(t *testing.T) {
	q := adminQuotation()
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
	q := adminQuotation()
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
	q := adminQuotation()
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
	q := adminQuotation()
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

// Issue #683: a staff session may not export the report even though it is
// signed in. The refusal is decided before the payload is decoded and before
// the save dialog opens, so a staff caller can never write a file.
func TestExportPurchaseOrderReportRefusesStaff(t *testing.T) {
	q := staffQuotation()
	dialogOpened := false
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		dialogOpened = true
		return filepath.Join(t.TempDir(), "never.pdf"), nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	if _, err := q.ExportPurchaseOrderReport(reportJSON(t, reportExportFixture())); err == nil {
		t.Fatalf("a staff export was accepted")
	}
	if dialogOpened {
		t.Errorf("the save dialog opened for a staff session")
	}
}

// Issue #683: the Excel export is the same payload and the same save
// discipline as the PDF, with an `.xlsx` destination. It is a separate binding
// so the PDF path stays byte-for-byte what it was; the file it writes is a real
// workbook, not a renamed PDF.
func TestExportPurchaseOrderReportXLSXWritesTheChosenFile(t *testing.T) {
	q := adminQuotation()
	dir := t.TempDir()
	path := filepath.Join(dir, "report.xlsx")

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

	got, err := q.ExportPurchaseOrderReportXLSX(reportJSON(t, reportExportFixture()))
	if err != nil {
		t.Fatalf("ExportPurchaseOrderReportXLSX: %v", err)
	}
	if got != path {
		t.Errorf("returned path = %q, want %q", got, path)
	}
	if gotOptions.Title != "Save Purchase Order Report" {
		t.Errorf("dialog title = %q", gotOptions.Title)
	}
	if gotOptions.DefaultFilename != "Purchase_Order_Report_2025-01-01_to_2026-01-01.xlsx" {
		t.Errorf("dialog filename = %q", gotOptions.DefaultFilename)
	}
	if len(gotOptions.Filters) != 1 || gotOptions.Filters[0].Pattern != "*.xlsx" {
		t.Errorf("dialog filters = %+v", gotOptions.Filters)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written workbook: %v", err)
	}
	if !strings.HasPrefix(string(raw), "PK") {
		t.Fatalf("written file is not a zip workbook (%d bytes)", len(raw))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("workbook mode = %04o, want 0644", info.Mode().Perm())
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".report-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Errorf("temporary workbook files left behind: %v", temps)
	}
}

func TestExportPurchaseOrderReportXLSXCancelReturnsEmptyPath(t *testing.T) {
	q := adminQuotation()
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		return "", nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	got, err := q.ExportPurchaseOrderReportXLSX(reportJSON(t, reportExportFixture()))
	if err != nil {
		t.Errorf("cancel returned an error: %v", err)
	}
	if got != "" {
		t.Errorf("cancel returned %q, want an empty path", got)
	}
}

func TestExportPurchaseOrderReportXLSXRefusesStaff(t *testing.T) {
	q := staffQuotation()
	dialogOpened := false
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		dialogOpened = true
		return filepath.Join(t.TempDir(), "never.xlsx"), nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	if _, err := q.ExportPurchaseOrderReportXLSX(reportJSON(t, reportExportFixture())); err == nil {
		t.Fatalf("a staff workbook export was accepted")
	}
	if dialogOpened {
		t.Errorf("the save dialog opened for a staff session")
	}
}

func TestExportPurchaseOrderReportXLSXRefusesAnInvalidViewBeforeTheDialog(t *testing.T) {
	q := adminQuotation()
	badVersion := reportViewFixture()
	badVersion.Version = report.EnvelopeVersion(2)

	dialogOpened := false
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		dialogOpened = true
		return filepath.Join(t.TempDir(), "never.xlsx"), nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	if _, err := q.ExportPurchaseOrderReportXLSX(viewJSON(t, badVersion)); err == nil {
		t.Fatalf("an invalid view was accepted")
	}
	if dialogOpened {
		t.Errorf("the save dialog opened before the invalid view was refused")
	}
}

func TestExportPurchaseOrderReportXLSXValidatesTheChosenPath(t *testing.T) {
	q := adminQuotation()
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		return filepath.Join(t.TempDir(), "report.pdf"), nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	if _, err := q.ExportPurchaseOrderReportXLSX(reportJSON(t, reportExportFixture())); err == nil {
		t.Errorf("a PDF destination was accepted for a workbook")
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
	q := adminQuotation()
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

	badAggregate := base
	badAggregate.Periods.Layout = report.ViewLayoutAggregate

	emptyOverride := emptyReportViewFixture()
	emptyOverride.Periods.CurrentMonthSource = reportViewIntPointer(0)

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
		{name: "single month as aggregate", payload: viewJSON(t, badAggregate)},
		{name: "empty selection with current-month override", payload: viewJSON(t, emptyOverride)},
		{name: "legacy with selection", payload: string(legacyMutated)},
		{name: "zero version", payload: `{"version":0,"sources":[],"overview":0,"periods":{"sources":[],"months":[],"layout":"individual"},"categories":{"source":0,"codes":[],"layout":"aggregate"}}`},
		{name: "null version", payload: `{"version":null,"sources":[],"overview":0,"periods":{"sources":[],"months":[],"layout":"individual"},"categories":{"source":0,"codes":[],"layout":"aggregate"}}`},
	}

	q := adminQuotation()
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

// Issue #642: the same binding accepts the extended v1 selections without a
// second version or a second reader. The empty selection and a one-calendar-year
// Aggregate are still validated before the save dialog opens.
func reportAggregateFixture() poreport.Report {
	snapshot := reportExportFixture()
	snapshot.Filter = poreport.Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}
	periods := make([]poreport.Period, 0, 12)
	for month := 1; month <= 12; month++ {
		periods = append(periods, poreport.Period{
			PeriodStart: fmt.Sprintf("2025-%02d-01", month),
			Metrics: poreport.Metrics{
				PoCount: 1, AmountMinor: 1_000_000, CostedMinor: 900_000,
				UncostedMinor: 100_000, CapitalMinor: 700_000, MarginMinor: 200_000,
				MarginBps: int64Pointer(2857), CoverageBps: int64Pointer(9000),
			},
		})
	}
	snapshot.Periods = periods
	snapshot.Totals = poreport.Metrics{
		PoCount: 12, AmountMinor: 12_000_000, CostedMinor: 10_800_000,
		UncostedMinor: 1_200_000, CapitalMinor: 8_400_000, MarginMinor: 2_400_000,
		MarginBps: int64Pointer(2857), CoverageBps: int64Pointer(9000),
	}
	return snapshot
}

func reportViewIntPointer(value int) *int { return &value }

func emptyReportViewFixture() report.ViewEnvelope {
	return report.ViewEnvelope{
		Version:  report.ViewVersion,
		Sources:  []poreport.Report{reportExportFixture()},
		Overview: 0,
		Periods: report.ViewPeriods{
			Sources: []int{}, Months: []string{}, Layout: report.ViewLayoutIndividual,
		},
		Categories: report.ViewCategories{
			Source: 0, Codes: []string{}, Layout: report.ViewLayoutAggregate,
		},
	}
}

func aggregateReportViewFixture() report.ViewEnvelope {
	snapshot := reportAggregateFixture()
	months := make([]string, 0, 12)
	for month := 1; month <= 12; month++ {
		months = append(months, fmt.Sprintf("2025-%02d", month))
	}
	return report.ViewEnvelope{
		Version:  report.ViewVersion,
		Sources:  []poreport.Report{snapshot},
		Overview: 0,
		Periods: report.ViewPeriods{
			Sources: []int{0}, Months: months, Layout: report.ViewLayoutAggregate,
		},
		Categories: report.ViewCategories{
			Source: 0, Codes: []string{}, Layout: report.ViewLayoutAggregate,
		},
	}
}

func TestExportPurchaseOrderReportWritesEmptyAndAggregateViews(t *testing.T) {
	cases := []struct {
		name string
		view report.ViewEnvelope
	}{
		{name: "empty selection", view: emptyReportViewFixture()},
		{name: "one-calendar-year aggregate", view: aggregateReportViewFixture()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			q := adminQuotation()
			dir := t.TempDir()
			path := filepath.Join(dir, "extended-view.pdf")
			previousDialog := saveFileDialog
			saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
				return path, nil
			}
			t.Cleanup(func() { saveFileDialog = previousDialog })
			previousClient := apiClient
			apiClient = nil
			t.Cleanup(func() { apiClient = previousClient })

			got, err := q.ExportPurchaseOrderReport(viewJSON(t, test.view))
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
		})
	}
}

// Issue #643: the category comparison travels in the same version-1 envelope.
// The binding still validates it before opening the save dialog and still needs
// no API client: every figure is frozen in the payload.
func reportCategoryFixture() poreport.Report {
	snapshot := reportAggregateFixture()
	money := func(poCount, amount, costed, uncosted, capital, margin int64) poreport.Metrics {
		return poreport.Metrics{
			PoCount: poCount, AmountMinor: amount, CostedMinor: costed,
			UncostedMinor: uncosted, CapitalMinor: capital, MarginMinor: margin,
			CoverageBps: int64Pointer(9000),
		}
	}
	snapshot.Categories = []poreport.Category{
		{
			BusinessCategory: poreport.CategoryFm, Label: "Facility Management",
			Metrics: money(3, 3_000_000, 2_700_000, 300_000, 2_100_000, 600_000),
		},
		{
			BusinessCategory: poreport.CategoryFab, Label: "Fabrication",
			Metrics: money(2, 2_000_000, 1_800_000, 200_000, 2_200_000, -400_000),
		},
		{
			BusinessCategory: poreport.CategorySupplyGeneral, Label: "Goods Supply & General",
			Metrics: money(1, 1_000_000, 900_000, 100_000, 700_000, 200_000),
		},
		// An empty supplied label falls back to the workbook name; a zero row is
		// still a supplied row, never dropped or invented.
		{BusinessCategory: poreport.CategoryOthers, Label: "", Metrics: poreport.Metrics{}},
		{BusinessCategory: poreport.CategoryUnassigned, Label: "Unassigned", Metrics: poreport.Metrics{}},
	}
	return snapshot
}

func reportCategoryViewFixture(layout string, codes []string) report.ViewEnvelope {
	overview := reportCategoryFixture()
	overview.Filter.Category = poreport.CategoryFm
	facility := overview.Categories[0]
	overview.Categories = []poreport.Category{facility}
	overview.Totals = facility.Metrics
	source := reportCategoryFixture()
	return report.ViewEnvelope{
		Version:  report.ViewVersion,
		Sources:  []poreport.Report{overview, source},
		Overview: 0,
		Periods: report.ViewPeriods{
			Sources: []int{0}, Months: []string{"2025-01"}, Layout: report.ViewLayoutIndividual,
		},
		Categories: report.ViewCategories{
			Source: 1, Codes: codes, Layout: layout,
		},
	}
}

func TestExportPurchaseOrderReportWritesCategoryComparisonViews(t *testing.T) {
	cases := []struct {
		name string
		view report.ViewEnvelope
	}{
		{
			name: "individual subset under a restricted overview",
			view: reportCategoryViewFixture(report.ViewLayoutIndividual,
				[]string{poreport.CategoryFm, poreport.CategoryFab, poreport.CategoryUnassigned}),
		},
		{
			name: "empty individual selection",
			view: reportCategoryViewFixture(report.ViewLayoutIndividual, []string{}),
		},
		{
			name: "full aggregate under a restricted overview",
			view: reportCategoryViewFixture(report.ViewLayoutAggregate, []string{
				poreport.CategoryFm, poreport.CategoryFab, poreport.CategorySupplyGeneral,
				poreport.CategoryOthers, poreport.CategoryUnassigned,
			}),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			q := adminQuotation()
			dir := t.TempDir()
			path := filepath.Join(dir, "category-view.pdf")
			previousDialog := saveFileDialog
			saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
				return path, nil
			}
			t.Cleanup(func() { saveFileDialog = previousDialog })
			previousClient := apiClient
			apiClient = nil
			t.Cleanup(func() { apiClient = previousClient })

			got, err := q.ExportPurchaseOrderReport(viewJSON(t, test.view))
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
		})
	}
}

func TestExportPurchaseOrderReportRefusesInvalidCategoryViewsBeforeTheDialog(t *testing.T) {
	subsetAggregate := reportCategoryViewFixture(report.ViewLayoutAggregate,
		[]string{poreport.CategoryFm, poreport.CategoryFab})
	outOfOrder := reportCategoryViewFixture(report.ViewLayoutIndividual,
		[]string{poreport.CategoryFab, poreport.CategoryFm})
	duplicate := reportCategoryViewFixture(report.ViewLayoutIndividual,
		[]string{poreport.CategoryFm, poreport.CategoryFm})
	unknown := reportCategoryViewFixture(report.ViewLayoutIndividual,
		[]string{poreport.CategoryFm, "NOPE"})

	foreignCategory := reportCategoryViewFixture(report.ViewLayoutIndividual,
		[]string{poreport.CategoryFm})
	foreignCategory.Sources = append([]poreport.Report{}, foreignCategory.Sources...)
	foreign := foreignCategory.Sources[1]
	foreign.Filter.Category = poreport.CategoryFab
	foreignCategory.Sources[1] = foreign

	wrongRange := reportCategoryViewFixture(report.ViewLayoutIndividual,
		[]string{poreport.CategoryFm})
	wrongRange.Sources = append([]poreport.Report{}, wrongRange.Sources...)
	other := wrongRange.Sources[1]
	other.Filter.From = "2024-01-01"
	wrongRange.Sources[1] = other

	absentRow := reportCategoryViewFixture(report.ViewLayoutIndividual,
		[]string{poreport.CategoryOthers})
	absentRow.Sources = append([]poreport.Report{}, absentRow.Sources...)
	absentSource := absentRow.Sources[1]
	kept := make([]poreport.Category, 0, len(absentSource.Categories))
	for _, category := range absentSource.Categories {
		if category.BusinessCategory == poreport.CategoryOthers {
			continue
		}
		kept = append(kept, category)
	}
	absentSource.Categories = kept
	absentRow.Sources[1] = absentSource

	cases := []struct {
		name string
		view report.ViewEnvelope
	}{
		{name: "subset aggregate", view: subsetAggregate},
		{name: "out-of-order codes", view: outOfOrder},
		{name: "duplicate code", view: duplicate},
		{name: "unknown code", view: unknown},
		{name: "foreign restricted category source", view: foreignCategory},
		{name: "category source range mismatch", view: wrongRange},
		{name: "code absent from the source", view: absentRow},
	}

	q := adminQuotation()
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
			if _, err := q.ExportPurchaseOrderReport(viewJSON(t, test.view)); err == nil {
				t.Fatal("invalid category view was accepted")
			}
			if dialogOpened {
				t.Fatal("the save dialog opened before the invalid category view was refused")
			}
		})
	}
}

// Issue #683, F1. The desktop builds &backend.Quotation{} with no auth service
// (main.go:23), so the only role source it has is the server. These tests use
// that exact shape: no SetAuthService, a stored session token, and a real HTTP
// server answering /v1/auth/me. The injected auth.StaffTestService seam the
// other staff tests use is deliberately absent, because a green test against a
// seam the desktop never builds proves nothing about the shipped binding.
type desktopSessionProbe struct {
	quotation    *Quotation
	dialogOpened *bool
	roleChecks   *int32
}

// newDesktopSessionProbe wires the desktop shape at a fake veraserver. role is
// the role /v1/auth/me reports; a request without the stored bearer token is
// refused with the server's coded 401.
func newDesktopSessionProbe(t *testing.T, role string) desktopSessionProbe {
	t.Helper()
	t.Setenv("VERA_NO_KEYRING", "1")
	setSessionTokenMemory("desktop-session-token")
	t.Cleanup(func() { setSessionTokenMemory("") })

	var roleChecks int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/me" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer desktop-session-token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"missing session","code":"session_missing"}`)
			return
		}
		atomic.AddInt32(&roleChecks, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":7,"email":"user@example.com","displayName":"User","role":"`+role+`"}`)
	}))
	t.Cleanup(server.Close)

	previousClient := apiClient
	ConfigureAPI(apiclient.New(server.URL, SessionToken))
	t.Cleanup(func() { apiClient = previousClient })

	dialogOpened := false
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		dialogOpened = true
		return "", nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	return desktopSessionProbe{
		quotation:    &Quotation{},
		dialogOpened: &dialogOpened,
		roleChecks:   &roleChecks,
	}
}

// TestExportPurchaseOrderReportRefusesStaffInTheDesktopShape is the criterion
// the first pass missed: a signed-in staff token, no auth service, both
// report bindings must refuse before the payload is decoded and before the save
// dialog opens. Delete the server role check and this test goes green while a
// staff desktop writes both files.
func TestExportPurchaseOrderReportRefusesStaffInTheDesktopShape(t *testing.T) {
	probe := newDesktopSessionProbe(t, "staff")

	if _, err := probe.quotation.ExportPurchaseOrderReport(reportJSON(t, reportExportFixture())); err == nil {
		t.Fatalf("desktop staff session exported a PDF")
	}
	if *probe.dialogOpened {
		t.Errorf("the PDF save dialog opened for a desktop staff session")
	}

	if _, err := probe.quotation.ExportPurchaseOrderReportXLSX(reportJSON(t, reportExportFixture())); err == nil {
		t.Fatalf("desktop staff session exported a workbook")
	}
	if *probe.dialogOpened {
		t.Errorf("the Excel save dialog opened for a desktop staff session")
	}

	if got := atomic.LoadInt32(probe.roleChecks); got != 2 {
		t.Errorf("role checks = %d; want the server asked once per binding", got)
	}
}

// TestExportPurchaseOrderReportAllowsAnAdminInTheDesktopShape keeps the real
// path from becoming a blanket refusal: an admin session the server confirms is
// allowed to reach the dialog. Cancelling the dialog is enough proof that the
// export passed the role gate, because the dialog is opened after it.
func TestExportPurchaseOrderReportAllowsAnAdminInTheDesktopShape(t *testing.T) {
	probe := newDesktopSessionProbe(t, "admin")

	got, err := probe.quotation.ExportPurchaseOrderReport(reportJSON(t, reportExportFixture()))
	if err != nil {
		t.Fatalf("desktop admin PDF export: %v", err)
	}
	if got != "" {
		t.Errorf("cancelled PDF export returned %q, want an empty path", got)
	}

	got, err = probe.quotation.ExportPurchaseOrderReportXLSX(reportJSON(t, reportExportFixture()))
	if err != nil {
		t.Fatalf("desktop admin Excel export: %v", err)
	}
	if got != "" {
		t.Errorf("cancelled Excel export returned %q, want an empty path", got)
	}

	if !*probe.dialogOpened {
		t.Errorf("the save dialog never opened for an admin session")
	}
	if got := atomic.LoadInt32(probe.roleChecks); got != 2 {
		t.Errorf("role checks = %d; want the server asked once per binding", got)
	}
}

// A refused or unreachable role lookup is a refusal, never a fallback to "a
// token exists". The fake server answers 401 for a token it never issued.
func TestExportPurchaseOrderReportRefusesWhenTheRoleCannotBeConfirmed(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	setSessionTokenMemory("a-token-the-server-does-not-know")
	t.Cleanup(func() { setSessionTokenMemory("") })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"missing session","code":"session_missing"}`)
	}))
	t.Cleanup(server.Close)

	previousClient := apiClient
	ConfigureAPI(apiclient.New(server.URL, SessionToken))
	t.Cleanup(func() { apiClient = previousClient })

	dialogOpened := false
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		dialogOpened = true
		return "", nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	q := &Quotation{}
	if _, err := q.ExportPurchaseOrderReport(reportJSON(t, reportExportFixture())); err == nil {
		t.Fatalf("an unconfirmed session exported a PDF")
	}
	if _, err := q.ExportPurchaseOrderReportXLSX(reportJSON(t, reportExportFixture())); err == nil {
		t.Fatalf("an unconfirmed session exported a workbook")
	}
	if dialogOpened {
		t.Errorf("the save dialog opened without a confirmed admin role")
	}
}
