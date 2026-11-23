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
	// ExcludePOID is the repair-mode exemption (#475): the one saved record
	// being repaired keeps its own Quotation selectable. Zero is the ordinary
	// PO-entry path; a non-zero value is an admin operation server-side.
	ExcludePOID int64
}

// POSourceActivePODTO is the active PO occupying a Quotation (issue #468).
// A draft offers "Open draft"; a saved record stays disabled.
type POSourceActivePODTO struct {
	ID    int64
	State string
}

// POSourceQuotationDTO is one selectable Quotation: its latest issued
// revision, how many revisions are issued, and the active PO that occupies it
// (if any).
type POSourceQuotationDTO struct {
	ActivePO            *POSourceActivePODTO
	QuoteNumber         string
	Subject             string
	CustomerName        string
	ID                  int64
	LatestRevision      int16
	IssuedRevisionCount int
	// Bought-state badges (#651). AllBought disables a new PO; a partial
	// quotation stays selectable and shows its remaining leaf count.
	AllBought            bool
	LeafCount            int
	RemainingLeafCount   int
	SavedPOCount         int
	DraftPOCount         int
	LegacyHeaderDiscount bool
	// Closure and expiry (#656). A closed row is visible but cannot start a
	// new purchase order; an expired-unclosed row keeps its Unsold remainder.
	Closed          bool
	ClosedAt        *string
	ClosedReason    string
	ValidUntil      *string
	ExpiredUnclosed bool
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

// POSourceOccupancyDTO is the archived Quotation detail's occupancy answer
// (issue #499). ActivePO is the one record occupying the Quotation -- a draft
// or a finalized record -- or nil when the Quotation is free. Occupancy
// belongs to the Quotation, so the displayed revision is deliberately not an
// input.
type POSourceOccupancyDTO struct {
	ActivePO             *POSourceActivePODTO
	QuotationID          int64
	AllBought            bool
	RemainingLeafCount   int
	SavedPOCount         int
	DraftPOCount         int
	LegacyHeaderDiscount bool
	// Closure and expiry (#656). ClosedByName is the detail page's "who".
	Closed          bool
	ClosedAt        *string
	ClosedReason    string
	ClosedByName    string
	ValidUntil      *string
	ExpiredUnclosed bool
}

// POSourceAvailabilityItemDTO is one remaining quotation leaf. The price is
// the row-discounted customer price; supplier cost is never exposed (Story 42).
type POSourceAvailabilityItemDTO struct {
	QuotationItemNo          int
	QuotationSubNo           *int
	Label                    string
	Description              string
	ParentDescription        string
	GroupKey                 string
	DescriptionLines         []string
	UoM                      string
	QuotedQuantityScaled     int64
	BoughtQuantityScaled     int64
	DraftQuantityScaled      int64
	RemainingQuantityScaled  int64
	OverBoughtQuantityScaled int64
	QuotedUnitPriceMinor     int64
}

// POSourceAvailabilityDTO is the server-side bought state for one quotation
// revision. A draft quantity is advisory and never reduces remaining.
type POSourceAvailabilityDTO struct {
	QuotationID          int64
	RevisionNumber       int16
	LatestRevision       int16
	AllBought            bool
	LegacyHeaderDiscount bool
	SavedPOCount         int
	DraftPOCount         int
	// Closure and expiry (#656). A closed revision's remaining leaves are
	// Closed Loss; the workspace disables ordering rather than hiding them.
	Closed          bool
	ClosedAt        *string
	ClosedReason    string
	ValidUntil      *string
	ExpiredUnclosed bool
	Items           []POSourceAvailabilityItemDTO
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
	if query.ExcludePOID > 0 {
		params.Set("excludePOId", strconv.FormatInt(query.ExcludePOID, 10))
	}
	var response apicontract.POSourceListResponse
	if err := client.Get("/v1/po-sources?"+params.Encode(), &response); err != nil {
		return POSourcePageDTO{}, err
	}
	rows := make([]POSourceQuotationDTO, 0, len(response.Quotations))
	for _, row := range response.Quotations {
		dto := POSourceQuotationDTO{
			ID:                   row.ID,
			QuoteNumber:          row.QuoteNumber,
			Subject:              row.Subject,
			CustomerName:         row.CustomerName,
			LatestRevision:       row.LatestRevision,
			IssuedRevisionCount:  row.IssuedRevisionCount,
			AllBought:            row.AllBought,
			LeafCount:            row.LeafCount,
			RemainingLeafCount:   row.RemainingLeafCount,
			SavedPOCount:         row.SavedPOCount,
			DraftPOCount:         row.DraftPOCount,
			LegacyHeaderDiscount: row.LegacyHeaderDiscount,
			Closed:               row.Closed,
			ClosedAt:             row.ClosedAt,
			ClosedReason:         row.ClosedReason,
			ValidUntil:           row.ValidUntil,
			ExpiredUnclosed:      row.ExpiredUnclosed,
		}
		if row.ActivePO != nil {
			dto.ActivePO = &POSourceActivePODTO{
				ID:    row.ActivePO.ID,
				State: row.ActivePO.State,
			}
		}
		rows = append(rows, dto)
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

// GetPOSourceOccupancy asks whether a Quotation already has its one PO, and
// whether that record is a draft or finalized (issue #499). The archived detail
// page uses it to offer Open PO draft / View PO instead of a second creation
// attempt; the create path re-checks occupancy inside the server transaction,
// so this read may be stale without ever becoming a bypass.
func (q *Quotation) GetPOSourceOccupancy(quotationID int64) (POSourceOccupancyDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return POSourceOccupancyDTO{}, err
	}
	client, err := api()
	if err != nil {
		return POSourceOccupancyDTO{}, err
	}
	var response apicontract.POSourceOccupancyResponse
	path := "/v1/po-sources/" + strconv.FormatInt(quotationID, 10) + "/occupancy"
	if err := client.Get(path, &response); err != nil {
		return POSourceOccupancyDTO{}, err
	}
	dto := POSourceOccupancyDTO{
		QuotationID:          response.QuotationID,
		AllBought:            response.AllBought,
		RemainingLeafCount:   response.RemainingLeafCount,
		SavedPOCount:         response.SavedPOCount,
		DraftPOCount:         response.DraftPOCount,
		LegacyHeaderDiscount: response.LegacyHeaderDiscount,
		Closed:               response.Closed,
		ClosedAt:             response.ClosedAt,
		ClosedReason:         response.ClosedReason,
		ClosedByName:         response.ClosedByName,
		ValidUntil:           response.ValidUntil,
		ExpiredUnclosed:      response.ExpiredUnclosed,
	}
	if response.ActivePO != nil {
		dto.ActivePO = &POSourceActivePODTO{
			ID:    response.ActivePO.ID,
			State: response.ActivePO.State,
		}
	}
	return dto, nil
}

// GetQuotationAvailability reads the latest revision's bought state: quoted,
// saved-bought, draft, remaining and over-bought quantities per leaf, plus the
// All bought and legacy-header flags. The workspace uses it to show only
// remaining leaves and to disable Create PO when nothing is left.
func (q *Quotation) GetQuotationAvailability(quotationID int64, revision int16) (POSourceAvailabilityDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return POSourceAvailabilityDTO{}, err
	}
	client, err := api()
	if err != nil {
		return POSourceAvailabilityDTO{}, err
	}
	var response apicontract.POSourceAvailabilityResponse
	path := "/v1/po-sources/" + strconv.FormatInt(quotationID, 10) + "/availability"
	if revision >= 0 {
		path += "?revision=" + strconv.Itoa(int(revision))
	}
	if err := client.Get(path, &response); err != nil {
		return POSourceAvailabilityDTO{}, err
	}
	items := make([]POSourceAvailabilityItemDTO, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, POSourceAvailabilityItemDTO{
			QuotationItemNo:          item.QuotationItemNo,
			QuotationSubNo:           item.QuotationSubNo,
			Label:                    item.Label,
			Description:              item.Description,
			ParentDescription:        item.ParentDescription,
			GroupKey:                 item.GroupKey,
			DescriptionLines:         item.DescriptionLines,
			UoM:                      item.UoM,
			QuotedQuantityScaled:     item.QuotedQuantityScaled,
			BoughtQuantityScaled:     item.BoughtQuantityScaled,
			DraftQuantityScaled:      item.DraftQuantityScaled,
			RemainingQuantityScaled:  item.RemainingQuantityScaled,
			OverBoughtQuantityScaled: item.OverBoughtQuantityScaled,
			QuotedUnitPriceMinor:     item.QuotedUnitPriceMinor,
		})
	}
	return POSourceAvailabilityDTO{
		QuotationID:          response.QuotationID,
		RevisionNumber:       response.RevisionNumber,
		LatestRevision:       response.LatestRevision,
		AllBought:            response.AllBought,
		LegacyHeaderDiscount: response.LegacyHeaderDiscount,
		SavedPOCount:         response.SavedPOCount,
		DraftPOCount:         response.DraftPOCount,
		Closed:               response.Closed,
		ClosedAt:             response.ClosedAt,
		ClosedReason:         response.ClosedReason,
		ValidUntil:           response.ValidUntil,
		ExpiredUnclosed:      response.ExpiredUnclosed,
		Items:                items,
	}, nil
}
