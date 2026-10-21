package apicontract

// Issue #477: the text-PDF extraction result contract.
//
// The extractor is evidence, never a fact. Every field and every row cell
// carries its own status, its source page and (where the engine supplies one)
// its source region. Poppler supplies word boxes, so `bbox` is populated for
// every proposal; `confidence` is always null because it does not report one.
// A value the extractor cannot normalise is returned as `needs_review` with
// the printed text preserved, never guessed into a different value.
//
// The same schema is the extractor's stdout contract (cmd/vextract), the
// stored result, the API response and the desktop binding. One definition is
// what keeps those four from drifting.
const ExtractionSchemaVersion = 1

// Cell and row statuses. `absent` is a deliberate answer, not a missing
// object: a field the document does not print is reported so the reviewer can
// tell "not found" from "not processed".
const (
	ExtractionStatusExtracted   = "extracted"
	ExtractionStatusNeedsReview = "needs_review"
	ExtractionStatusAbsent      = "absent"
)

// Header field keys, in the order the review panel shows them. Currency is
// evidence only: the PO's currency comes from the Quotation and is protected,
// so the reviewer compares this value but never applies it.
const (
	ExtractionFieldPONumber              = "poNumber"
	ExtractionFieldPODate                = "poDate"
	ExtractionFieldRequestedDeliveryDate = "requestedDeliveryDate"
	ExtractionFieldCurrency              = "currency"
	ExtractionFieldPaymentTerms          = "paymentTerms"
	ExtractionFieldIncoterms             = "incoterms"
)

// ExtractionFieldKeys is the complete header set, in display order. A result
// always carries every key so the UI never has to distinguish "omitted" from
// "the engine found nothing".
var ExtractionFieldKeys = []string{
	ExtractionFieldPONumber,
	ExtractionFieldPODate,
	ExtractionFieldRequestedDeliveryDate,
	ExtractionFieldCurrency,
	ExtractionFieldPaymentTerms,
	ExtractionFieldIncoterms,
}

// ExtractionBBox is one region in PDF points, with the origin at the page's
// top-left exactly as pdftotext prints it. Width and Height are derived from
// the engine's min/max values so a UI does not have to subtract them.
type ExtractionBBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ExtractionPageInfo is the page's size, needed to turn a bbox into a relative
// region for display. It is evidence about the document, not content.
type ExtractionPageInfo struct {
	Number int     `json:"number"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ExtractionCell is one proposed value and its provenance.
//
// Value is normalised for the cell's kind (an ISO date, a decimal with a dot,
// an uppercase currency code). Raw is the printed text the extractor read, so
// a reviewer can see what was on the page. Status is one of the three
// constants above. Page is null only when the status is absent.
type ExtractionCell struct {
	Value      string          `json:"value"`
	Raw        string          `json:"raw"`
	Status     string          `json:"status"`
	Page       *int            `json:"page"`
	BBox       *ExtractionBBox `json:"bbox"`
	Confidence *float64        `json:"confidence"`
	Note       string          `json:"note,omitempty"`
}

// ExtractionField is one header proposal. Key is stable across engines; the
// client owns the human label. The cell's value, status and provenance are
// promoted into the same JSON object as the key.
type ExtractionField struct {
	Key string `json:"key"`
	ExtractionCell
}

// ExtractionRow is one proposed customer line.
//
// LineNumber is the number as printed on the customer's document, independent
// of any Quotation item match: matching is review logic, not the extractor's
// job. Status is the row's aggregate: `extracted` only when every required
// cell (description, quantity, UoM, unit price) is extracted; otherwise
// `needs_review`. A row is never `absent` -- an absent row is a row the table
// does not print, so it is not emitted at all.
type ExtractionRow struct {
	LineNumber  ExtractionCell  `json:"lineNumber"`
	Description ExtractionCell  `json:"description"`
	Quantity    ExtractionCell  `json:"quantity"`
	UoM         ExtractionCell  `json:"uom"`
	UnitPrice   ExtractionCell  `json:"unitPrice"`
	Amount      ExtractionCell  `json:"amount"`
	Status      string          `json:"status"`
	Page        int             `json:"page"`
	BBox        *ExtractionBBox `json:"bbox"`
	Note        string          `json:"note,omitempty"`
}

// ExtractionAbstentions counts what the extractor did not assert. It is a
// summary for honesty, not a score: an abstention is neither correct nor
// incorrect.
type ExtractionAbstentions struct {
	Fields int `json:"fields"`
	Rows   int `json:"rows"`
}

// ExtractionOutput is the complete extractor answer. Engine and EngineVersion
// are part of the stored result so repeat runs are comparable and a
// regression is attributable to a version.
type ExtractionOutput struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	Engine           string                `json:"engine"`
	EngineVersion    string                `json:"engineVersion"`
	PageCount        int                   `json:"pageCount"`
	PagesWithText    int                   `json:"pagesWithText"`
	PagesWithoutText int                   `json:"pagesWithoutText"`
	DurationMs       int64                 `json:"durationMs"`
	Pages            []ExtractionPageInfo  `json:"pages"`
	Fields           []ExtractionField     `json:"fields"`
	Rows             []ExtractionRow       `json:"rows"`
	Abstentions      ExtractionAbstentions `json:"abstentions"`
}
