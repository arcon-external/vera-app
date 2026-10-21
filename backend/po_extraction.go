package backend

import (
	"encoding/json"
	"fmt"
	"strconv"

	"vera-app/backend/apicontract"
)

// Issue #477: desktop bindings for text-PDF extraction.
//
// The desktop never runs the extractor and never sees a PDF password or a
// server credential. It asks the authenticated API to start a run, then reads
// the latest state. A replaced PDF is reported stale by the server with no
// result; the client cannot apply proposals to a different document.

// PurchaseOrderExtractionBBoxDTO is a source region in PDF points, with the
// origin at the page's top-left.
type PurchaseOrderExtractionBBoxDTO struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// PurchaseOrderExtractionPageDTO is one page's size, used to place a region on
// a relative page map.
type PurchaseOrderExtractionPageDTO struct {
	Number int
	Width  float64
	Height float64
}

// PurchaseOrderExtractionCellDTO is one proposed value and its evidence.
// Confidence stays a pointer so "the engine does not report confidence"
// (null, the Poppler case) is distinct from "confidence is zero".
type PurchaseOrderExtractionCellDTO struct {
	Value      string
	Raw        string
	Status     string
	Page       *int
	BBox       *PurchaseOrderExtractionBBoxDTO
	Confidence *float64
	Note       string
}

// PurchaseOrderExtractionFieldDTO is one header proposal.
type PurchaseOrderExtractionFieldDTO struct {
	Key  string
	Cell PurchaseOrderExtractionCellDTO
}

// PurchaseOrderExtractionRowDTO is one proposed customer line.
type PurchaseOrderExtractionRowDTO struct {
	LineNumber  PurchaseOrderExtractionCellDTO
	Description PurchaseOrderExtractionCellDTO
	Quantity    PurchaseOrderExtractionCellDTO
	UoM         PurchaseOrderExtractionCellDTO
	UnitPrice   PurchaseOrderExtractionCellDTO
	Amount      PurchaseOrderExtractionCellDTO
	Status      string
	Page        int
	BBox        *PurchaseOrderExtractionBBoxDTO
	Note        string
}

// PurchaseOrderExtractionAbstentionsDTO counts what the extractor did not
// assert.
type PurchaseOrderExtractionAbstentionsDTO struct {
	Fields int
	Rows   int
}

// PurchaseOrderExtractionResultDTO is the typed extraction contract.
type PurchaseOrderExtractionResultDTO struct {
	Engine           string
	EngineVersion    string
	PageCount        int
	PagesWithText    int
	PagesWithoutText int
	DurationMs       int64
	SchemaVersion    int
	Pages            []PurchaseOrderExtractionPageDTO
	Fields           []PurchaseOrderExtractionFieldDTO
	Rows             []PurchaseOrderExtractionRowDTO
	Abstentions      PurchaseOrderExtractionAbstentionsDTO
}

// PurchaseOrderExtractionDocumentDTO is the active PDF generation the server
// compared the run against.
type PurchaseOrderExtractionDocumentDTO struct {
	ID      int64
	Version int32
	SHA256  string
}

// PurchaseOrderExtractionDTO is one run. Result is nil for queued, running,
// failed and stale runs; the client renders the reason from ErrorCode.
type PurchaseOrderExtractionDTO struct {
	UUID            string
	Status          string
	Engine          string
	EngineVersion   string
	DocumentSHA256  string
	ErrorCode       *string
	ErrorMessage    *string
	Result          *PurchaseOrderExtractionResultDTO
	ID              int64
	PurchaseOrderID int64
	DocumentID      int64
	DocumentVersion int32
	PageCount       int
	RequestedBy     int64
	CreatedAt       string
	StartedAt       *string
	FinishedAt      *string
	DurationMS      *int64
}

// PurchaseOrderExtractionStateDTO is the latest run plus the active document
// generation. Run is nil before the first extraction.
type PurchaseOrderExtractionStateDTO struct {
	Run      *PurchaseOrderExtractionDTO
	Document *PurchaseOrderExtractionDocumentDTO
}

// StartPurchaseOrderExtraction asks the server to queue a text extraction for
// the current PDF. It is idempotent while a run is active: the server returns
// the run already in flight.
func (q *Quotation) StartPurchaseOrderExtraction(id int64) (PurchaseOrderExtractionStateDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderExtractionStateDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderExtractionStateDTO{}, err
	}
	var response apicontract.PurchaseOrderExtractionStateRow
	if err := client.Post(
		"/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/extractions",
		struct{}{}, &response); err != nil {
		return PurchaseOrderExtractionStateDTO{}, err
	}
	return mapPurchaseOrderExtractionState(response)
}

// GetPurchaseOrderExtraction reads the latest run and the active document
// generation, so a replaced PDF is visibly stale even if the client missed the
// replacement event.
func (q *Quotation) GetPurchaseOrderExtraction(id int64) (PurchaseOrderExtractionStateDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderExtractionStateDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderExtractionStateDTO{}, err
	}
	var response apicontract.PurchaseOrderExtractionStateRow
	if err := client.Get(
		"/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/extractions/latest", &response); err != nil {
		return PurchaseOrderExtractionStateDTO{}, err
	}
	return mapPurchaseOrderExtractionState(response)
}

func mapPurchaseOrderExtractionState(row apicontract.PurchaseOrderExtractionStateRow) (PurchaseOrderExtractionStateDTO, error) {
	state := PurchaseOrderExtractionStateDTO{}
	if row.Document != nil {
		state.Document = &PurchaseOrderExtractionDocumentDTO{
			ID:      row.Document.ID,
			Version: row.Document.Version,
			SHA256:  row.Document.SHA256,
		}
	}
	if row.Run != nil {
		run, err := mapPurchaseOrderExtraction(*row.Run)
		if err != nil {
			return PurchaseOrderExtractionStateDTO{}, err
		}
		state.Run = &run
	}
	return state, nil
}

func mapPurchaseOrderExtraction(row apicontract.PurchaseOrderExtractionRow) (PurchaseOrderExtractionDTO, error) {
	dto := PurchaseOrderExtractionDTO{
		ID:              row.ID,
		UUID:            row.UUID,
		PurchaseOrderID: row.PurchaseOrderID,
		DocumentID:      row.DocumentID,
		DocumentVersion: row.DocumentVersion,
		DocumentSHA256:  row.DocumentSHA256,
		Status:          row.Status,
		Engine:          row.Engine,
		EngineVersion:   row.EngineVersion,
		PageCount:       row.PageCount,
		ErrorCode:       row.ErrorCode,
		ErrorMessage:    row.ErrorMessage,
		RequestedBy:     row.RequestedBy,
		CreatedAt:       row.CreatedAt,
		StartedAt:       row.StartedAt,
		FinishedAt:      row.FinishedAt,
		DurationMS:      row.DurationMS,
	}
	trimmed := string(row.Result)
	if trimmed != "" && trimmed != "null" {
		var output apicontract.ExtractionOutput
		if err := json.Unmarshal(row.Result, &output); err != nil {
			return PurchaseOrderExtractionDTO{}, fmt.Errorf("read the extraction result: %w", err)
		}
		result := mapPurchaseOrderExtractionResult(output)
		dto.Result = &result
	}
	return dto, nil
}

func mapPurchaseOrderExtractionResult(output apicontract.ExtractionOutput) PurchaseOrderExtractionResultDTO {
	result := PurchaseOrderExtractionResultDTO{
		SchemaVersion:    output.SchemaVersion,
		Engine:           output.Engine,
		EngineVersion:    output.EngineVersion,
		PageCount:        output.PageCount,
		PagesWithText:    output.PagesWithText,
		PagesWithoutText: output.PagesWithoutText,
		DurationMs:       output.DurationMs,
		Pages:            make([]PurchaseOrderExtractionPageDTO, 0, len(output.Pages)),
		Fields:           make([]PurchaseOrderExtractionFieldDTO, 0, len(output.Fields)),
		Rows:             make([]PurchaseOrderExtractionRowDTO, 0, len(output.Rows)),
		Abstentions: PurchaseOrderExtractionAbstentionsDTO{
			Fields: output.Abstentions.Fields,
			Rows:   output.Abstentions.Rows,
		},
	}
	for _, page := range output.Pages {
		result.Pages = append(result.Pages, PurchaseOrderExtractionPageDTO{
			Number: page.Number,
			Width:  page.Width,
			Height: page.Height,
		})
	}
	for _, field := range output.Fields {
		result.Fields = append(result.Fields, PurchaseOrderExtractionFieldDTO{
			Key:  field.Key,
			Cell: mapPurchaseOrderExtractionCell(field.ExtractionCell),
		})
	}
	for _, row := range output.Rows {
		result.Rows = append(result.Rows, PurchaseOrderExtractionRowDTO{
			LineNumber:  mapPurchaseOrderExtractionCell(row.LineNumber),
			Description: mapPurchaseOrderExtractionCell(row.Description),
			Quantity:    mapPurchaseOrderExtractionCell(row.Quantity),
			UoM:         mapPurchaseOrderExtractionCell(row.UoM),
			UnitPrice:   mapPurchaseOrderExtractionCell(row.UnitPrice),
			Amount:      mapPurchaseOrderExtractionCell(row.Amount),
			Status:      row.Status,
			Page:        row.Page,
			BBox:        mapPurchaseOrderExtractionBBox(row.BBox),
			Note:        row.Note,
		})
	}
	return result
}

func mapPurchaseOrderExtractionCell(cell apicontract.ExtractionCell) PurchaseOrderExtractionCellDTO {
	return PurchaseOrderExtractionCellDTO{
		Value:      cell.Value,
		Raw:        cell.Raw,
		Status:     cell.Status,
		Page:       cell.Page,
		BBox:       mapPurchaseOrderExtractionBBox(cell.BBox),
		Confidence: cell.Confidence,
		Note:       cell.Note,
	}
}

func mapPurchaseOrderExtractionBBox(box *apicontract.ExtractionBBox) *PurchaseOrderExtractionBBoxDTO {
	if box == nil {
		return nil
	}
	return &PurchaseOrderExtractionBBoxDTO{
		X:      box.X,
		Y:      box.Y,
		Width:  box.Width,
		Height: box.Height,
	}
}
