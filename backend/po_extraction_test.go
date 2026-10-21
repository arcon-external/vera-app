package backend

import (
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #478: the desktop binding must carry the per-page OCR evidence, not
// just the rows. Without it the review panel cannot say which pages were OCRed,
// at what resolution, or with which engine version - and a low-confidence value
// would look like a fact.
func TestExtractionResultDTOCarriesOCRProvenance(t *testing.T) {
	confidence := 87.5
	cellConfidence := 55.0
	page := 2
	output := apicontract.ExtractionOutput{
		SchemaVersion:    apicontract.ExtractionSchemaVersion,
		Engine:           "poppler",
		EngineVersion:    "25.03.0",
		OCREngine:        "tesseract",
		OCREngineVersion: "5.5.0",
		PageCount:        2,
		PagesWithText:    1,
		PagesWithoutText: 1,
		PagesWithOCR:     1,
		Pages: []apicontract.ExtractionPageInfo{
			{Number: 1, Width: 595, Height: 842},
			{Number: 2, Width: 595, Height: 842},
		},
		PageModes: []apicontract.ExtractionPageMode{
			{Number: 1, Mode: apicontract.ExtractionPageModeText, TextWords: 20, TextChars: 180, Reason: "text floor met"},
			{Number: 2, Mode: apicontract.ExtractionPageModeOCR, TextWords: 2, TextChars: 12, OCRWords: 30, DPI: 300, Confidence: &confidence, Reason: "below the text floor"},
		},
		Rows: []apicontract.ExtractionRow{{
			Quantity: apicontract.ExtractionCell{
				Value: "2", Raw: "2", Status: apicontract.ExtractionStatusNeedsReview,
				Page: &page, Confidence: &cellConfidence, Note: "low confidence",
			},
			Status: apicontract.ExtractionStatusNeedsReview,
			Page:   2,
		}},
	}
	dto := mapPurchaseOrderExtractionResult(output)
	if dto.OCREngine != "tesseract" || dto.OCREngineVersion != "5.5.0" {
		t.Errorf("OCR engine = %q/%q; want the pinned engine", dto.OCREngine, dto.OCREngineVersion)
	}
	if dto.PagesWithOCR != 1 || len(dto.PageModes) != 2 {
		t.Fatalf("page evidence = %d OCR pages, %d modes; want 1 and 2", dto.PagesWithOCR, len(dto.PageModes))
	}
	ocrPage := dto.PageModes[1]
	if ocrPage.Mode != apicontract.ExtractionPageModeOCR || ocrPage.DPI != 300 || ocrPage.OCRWords != 30 ||
		ocrPage.Confidence == nil || *ocrPage.Confidence != 87.5 || ocrPage.Reason == "" {
		t.Errorf("OCR page evidence = %+v", ocrPage)
	}
	if dto.Rows[0].Quantity.Confidence == nil || *dto.Rows[0].Quantity.Confidence != 55 {
		t.Errorf("row confidence = %v; want the measured 55", dto.Rows[0].Quantity.Confidence)
	}
}
