package backend

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/poreport"
	"vera-app/backend/render/report"
)

// ExportPurchaseOrderReport writes the supplied report snapshot or versioned
// view through the native save dialog and returns the chosen path. An empty
// path means the user cancelled; that is not an error.
//
// Issue #683: the report workspace is an admin surface, so this binding requires
// an admin rather than merely a signed-in user. The role check runs before the
// payload is decoded, which means a staff caller cannot reach the renderer or the
// dialog at all.
//
// The payload is the only source of the document. This binding does not call
// the report endpoint, recalculate a figure, store an archive or touch the
// signing path: the screen already holds the report it showed, and the PDF has
// to be the same document. A payload without a version is the strict legacy
// report-only snapshot; a versioned view is decoded and validated as its own
// document. Malformed, oversized or out-of-range payloads are refused before
// the dialog opens.
func (q *Quotation) ExportPurchaseOrderReport(reportJSON string) (string, error) {
	if err := q.requireAdmin(q.ctx); err != nil {
		return "", err
	}
	document, err := report.DecodeExport([]byte(reportJSON))
	if err != nil {
		return "", err
	}

	// The report renderer reads the same embedded Inter faces the quotation
	// uses. fs.Sub keeps the renderer's font names independent of the
	// application's embed layout.
	fonts, err := fs.Sub(backAssets, "assets")
	if err != nil {
		return "", fmt.Errorf("load report fonts: %w", err)
	}
	pdf, err := report.RenderExport(document, fonts)
	if err != nil {
		return "", fmt.Errorf("render purchase order report: %w", err)
	}
	overview := document.Overview()

	filePath, err := saveFileDialog(q.ctx, runtime.SaveDialogOptions{
		Title:           "Save Purchase Order Report",
		DefaultFilename: reportFilename(overview),
		Filters:         []runtime.FileFilter{{DisplayName: "PDF Files (*.pdf)", Pattern: "*.pdf"}},
	})
	if err != nil {
		return "", fmt.Errorf("open save dialog: %w", err)
	}
	if filePath == "" {
		return "", nil
	}
	validatedPath, err := validateSavePath(filePath)
	if err != nil {
		return "", fmt.Errorf("validate purchase order report path: %w", err)
	}
	if err := writeReportFile(validatedPath, pdf); err != nil {
		return "", err
	}
	return validatedPath, nil
}

// ExportPurchaseOrderReportXLSX writes the supplied report snapshot or
// versioned view through the native save dialog as an Excel workbook and
// returns the chosen path. An empty path means the user cancelled; that is not
// an error.
//
// Issue #683: this binding requires an admin exactly like the PDF export, and
// for the same reason: the report workspace is an admin surface. The payload is
// the only source of the workbook; nothing is fetched or recalculated, so the
// spreadsheet and the PDF are the same document.
func (q *Quotation) ExportPurchaseOrderReportXLSX(reportJSON string) (string, error) {
	if err := q.requireAdmin(q.ctx); err != nil {
		return "", err
	}
	document, err := report.DecodeExport([]byte(reportJSON))
	if err != nil {
		return "", err
	}
	workbook, err := report.RenderXLSX(document)
	if err != nil {
		return "", fmt.Errorf("render purchase order report workbook: %w", err)
	}
	overview := document.Overview()

	filePath, err := saveFileDialog(q.ctx, runtime.SaveDialogOptions{
		Title:           "Save Purchase Order Report",
		DefaultFilename: reportFilenameWithExtension(overview, ".xlsx"),
		Filters:         []runtime.FileFilter{{DisplayName: "Excel Files (*.xlsx)", Pattern: "*.xlsx"}},
	})
	if err != nil {
		return "", fmt.Errorf("open save dialog: %w", err)
	}
	if filePath == "" {
		return "", nil
	}
	validatedPath, err := validateSavePathForExtension(filePath, ".xlsx")
	if err != nil {
		return "", fmt.Errorf("validate purchase order report workbook path: %w", err)
	}
	if err := writeReportFile(validatedPath, workbook); err != nil {
		return "", err
	}
	return validatedPath, nil
}

// reportFilename previews the PDF save name. The filter's dates are validated
// ISO dates, so they carry no path separator and cannot escape the chosen
// folder.
func reportFilename(snapshot poreport.Report) string {
	return reportFilenameWithExtension(snapshot, ".pdf")
}

// reportFilenameWithExtension previews a save name for one report format.
func reportFilenameWithExtension(snapshot poreport.Report, extension string) string {
	from := strings.TrimSpace(snapshot.Filter.From)
	to := strings.TrimSpace(snapshot.Filter.To)
	if from == "" || to == "" {
		return "purchase-order-report" + extension
	}
	return fmt.Sprintf("Purchase_Order_Report_%s_to_%s%s", from, to, extension)
}

// writeReportFile writes the report through a temporary file in the
// destination directory and renames it into place. On Unix the rename replaces
// the destination atomically; Windows does not guarantee that, so the comment
// claims no universal atomicity. In every case a failed write or rename is
// returned after the temporary file is removed, and the previous destination
// is not replaced with a truncated file.
func writeReportFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".report-*.tmp")
	if err != nil {
		return fmt.Errorf("create purchase order report: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("set purchase order report permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write purchase order report: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync purchase order report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close purchase order report: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("replace purchase order report: %w", err)
	}
	return nil
}
