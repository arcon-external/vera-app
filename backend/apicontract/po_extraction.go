package apicontract

import "encoding/json"

// Issue #477: the authenticated extraction surface.
//
// The run row is metadata plus the result JSON verbatim. Keeping Result as
// json.RawMessage means the API does not re-encode (and therefore cannot
// silently alter) the extractor's evidence contract; the desktop decodes it
// once into the typed shape.
type PurchaseOrderExtractionRow struct {
	UUID            string          `json:"uuid"`
	Status          string          `json:"status"`
	Engine          string          `json:"engine"`
	EngineVersion   string          `json:"engineVersion"`
	DocumentSHA256  string          `json:"documentSha256"`
	ErrorCode       *string         `json:"errorCode"`
	ErrorMessage    *string         `json:"errorMessage"`
	Result          json.RawMessage `json:"result"`
	ID              int64           `json:"id"`
	PurchaseOrderID int64           `json:"purchaseOrderId"`
	DocumentID      int64           `json:"documentId"`
	DocumentVersion int32           `json:"documentVersion"`
	PageCount       int             `json:"pageCount"`
	RequestedBy     int64           `json:"requestedBy"`
	CreatedAt       string          `json:"createdAt"`
	StartedAt       *string         `json:"startedAt"`
	FinishedAt      *string         `json:"finishedAt"`
	DurationMS      *int64          `json:"durationMs"`
}

// PurchaseOrderExtractionDocumentRow is the active document's generation, so
// the client can compare it with the run even if it missed the run's own
// generation fields.
type PurchaseOrderExtractionDocumentRow struct {
	SHA256  string `json:"sha256"`
	ID      int64  `json:"id"`
	Version int32  `json:"version"`
}

// PurchaseOrderExtractionStateRow is the latest run and the active document.
// Run is null when nothing has been extracted yet; the client renders the
// manual path rather than a fake empty result.
type PurchaseOrderExtractionStateRow struct {
	Run      *PurchaseOrderExtractionRow         `json:"run"`
	Document *PurchaseOrderExtractionDocumentRow `json:"document"`
}
