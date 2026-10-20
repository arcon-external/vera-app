package backend

import (
	"net/url"
	"strconv"

	"vera-app/backend/apicontract"
)

// Issue #467: desktop bindings for the PO source selector. The desktop has no
// HTTP layer of its own -- it calls the same authenticated API as the CLI --
// so these methods are thin, typed wrappers over /v1/po-sources. Both PO entry
// paths call them, which is why the shapes live beside the API contract rather
// than inside the modal.

// POSourceQuery is one page request from the selector's first step.
type POSourceQuery struct {
	Search   string
	Page     int
	HideUsed bool
}

// POSourceQuotationDTO is one selectable Quotation: its latest issued
// revision, how many revisions are issued, and whether a PO already occupies
// it. HasActivePO is false until #468 supplies the real query.
type POSourceQuotationDTO struct {
	QuoteNumber         string
	Subject             string
	CustomerName        string
	ID                  int64
	LatestRevision      int16
	IssuedRevisionCount int
	HasActivePO         bool
}

// POSourcePageDTO is one page of the selector's first step.
type POSourcePageDTO struct {
	Quotations []POSourceQuotationDTO
	Page       int
	PageSize   int
	Total      int
	TotalPages int
}

// POSourceRevisionDTO is one issued revision in the selector's second step.
type POSourceRevisionDTO struct {
	IssuedAt string
	Revision int16
}

// POSourceRevisionsDTO is the second step: issued revisions, newest first.
type POSourceRevisionsDTO struct {
	QuoteNumber  string
	Subject      string
	CustomerName string
	Revisions    []POSourceRevisionDTO
	QuotationID  int64
}

// POSourceSelection is the exact pair the selector returns: one Quotation and
// one issued revision. It is the input to the confirm-time revalidation and,
// in #468, the source link a PO is created against.
type POSourceSelection struct {
	QuotationID    int64
	RevisionNumber int16
}

// POSourceValidationDTO answers the confirm-time revalidation. A false Valid
// carries a stable Reason ("quotation_not_found" or "revision_not_found").
type POSourceValidationDTO struct {
	Reason         string
	QuotationID    int64
	RevisionNumber int16
	Valid          bool
}

// ListPOSourceQuotations fetches one page of issued Quotations for the
// selector. Unsaved drafts never appear; search is case-insensitive across
// number, subject, customer and item/sub-item descriptions.
func (q *Quotation) ListPOSourceQuotations(query POSourceQuery) (POSourcePageDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return POSourcePageDTO{}, err
	}
	client, err := api()
	if err != nil {
		return POSourcePageDTO{}, err
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	params := url.Values{}
	params.Set("page", strconv.Itoa(page))
	if query.Search != "" {
		params.Set("search", query.Search)
	}
	if query.HideUsed {
		params.Set("hideUsed", "true")
	}
	var response apicontract.POSourceListResponse
	if err := client.Get("/v1/po-sources?"+params.Encode(), &response); err != nil {
		return POSourcePageDTO{}, err
	}
	rows := make([]POSourceQuotationDTO, 0, len(response.Quotations))
	for _, row := range response.Quotations {
		rows = append(rows, POSourceQuotationDTO{
			ID:                  row.ID,
			QuoteNumber:         row.QuoteNumber,
			Subject:             row.Subject,
			CustomerName:        row.CustomerName,
			LatestRevision:      row.LatestRevision,
			IssuedRevisionCount: row.IssuedRevisionCount,
			HasActivePO:         row.HasActivePO,
		})
	}
	return POSourcePageDTO{
		Quotations: rows,
		Page:       response.Page,
		PageSize:   response.PageSize,
		Total:      response.Total,
		TotalPages: response.TotalPages,
	}, nil
}

// ListPOSourceRevisions fetches the issued revisions of one Quotation.
func (q *Quotation) ListPOSourceRevisions(quotationID int64) (POSourceRevisionsDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return POSourceRevisionsDTO{}, err
	}
	client, err := api()
	if err != nil {
		return POSourceRevisionsDTO{}, err
	}
	var response apicontract.POSourceRevisionsResponse
	path := "/v1/po-sources/" + strconv.FormatInt(quotationID, 10) + "/revisions"
	if err := client.Get(path, &response); err != nil {
		return POSourceRevisionsDTO{}, err
	}
	revisions := make([]POSourceRevisionDTO, 0, len(response.Revisions))
	for _, revision := range response.Revisions {
		revisions = append(revisions, POSourceRevisionDTO{
			Revision: revision.Revision,
			IssuedAt: revision.IssuedAt,
		})
	}
	return POSourceRevisionsDTO{
		QuotationID:  response.QuotationID,
		QuoteNumber:  response.QuoteNumber,
		Subject:      response.Subject,
		CustomerName: response.CustomerName,
		Revisions:    revisions,
	}, nil
}

// ValidatePOSource rechecks the exact pair at confirm time.
func (q *Quotation) ValidatePOSource(selection POSourceSelection) (POSourceValidationDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return POSourceValidationDTO{}, err
	}
	client, err := api()
	if err != nil {
		return POSourceValidationDTO{}, err
	}
	revisionNumber := selection.RevisionNumber
	var response apicontract.POSourceValidationResponse
	if err := client.Post("/v1/po-sources/validate", apicontract.POSourceValidateRequest{
		QuotationID:    selection.QuotationID,
		RevisionNumber: &revisionNumber,
	}, &response); err != nil {
		return POSourceValidationDTO{}, err
	}
	return POSourceValidationDTO{
		Valid:          response.Valid,
		Reason:         response.Reason,
		QuotationID:    response.QuotationID,
		RevisionNumber: response.RevisionNumber,
	}, nil
}
