package backend

import (
	"strconv"

	"vera-app/backend/apicontract"
)

// Issue #468: desktop bindings for a manual Purchase Order draft. Both PO
// entry paths reach the same authenticated API; these wrappers keep the
// desktop free of HTTP shape knowledge and give Wails one typed surface.

// PurchaseOrderItemDTO is one entered line. QuantityScaled is an exact count
// of 1/10000 units; unit prices are minor units of the PO currency.
type PurchaseOrderItemDTO struct {
	QuotationItemNo *int
	QuotationSubNo  *int
	LineNumber      int
	Description     string
	QuantityScaled  int64
	UoM             string
	UnitPriceMinor  int64
}

// PurchaseOrderDTO is a draft or saved PO with its editable header and lines.
type PurchaseOrderDTO struct {
	PODate                *string
	RequestedDeliveryDate *string
	UUID                  string
	State                 string
	QuoteNumber           string
	CustomerName          string
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	ReceivedDate          string
	Incoterms             string
	Currency              string
	TaxZone               string
	TaxCategory           string
	CreatedAt             string
	UpdatedAt             string
	Items                 []PurchaseOrderItemDTO
	ID                    int64
	QuotationID           int64
	SourceRevision        int16
	CustomerID            int64
	PaymentTerms          string
}

// PurchaseOrderCreateInput is the selector's confirmed pair plus an optional
// received-date override. An empty date asks the server for today.
type PurchaseOrderCreateInput struct {
	ReceivedDate   string
	QuotationID    int64
	RevisionNumber int16
}

// PurchaseOrderItemInput is one replace-lines entry. Line numbers come from
// array order.
type PurchaseOrderItemInput struct {
	QuotationItemNo *int
	QuotationSubNo  *int
	Description     string
	QuantityScaled  int64
	UoM             string
	UnitPriceMinor  int64
}

// PurchaseOrderUpdateInput replaces the editable draft header and every line.
type PurchaseOrderUpdateInput struct {
	ID                    int64
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	PODate                string
	ReceivedDate          string
	RequestedDeliveryDate string
	Incoterms             string
	PaymentTerms          string
	Items                 []PurchaseOrderItemInput
}

// CreatePurchaseOrderDraft creates the one active PO for a Quotation from the
// confirmed issued revision.
func (q *Quotation) CreatePurchaseOrderDraft(input PurchaseOrderCreateInput) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
	}
	revision := input.RevisionNumber
	var response apicontract.PurchaseOrderRow
	if err := client.Post("/v1/purchase-orders", apicontract.PurchaseOrderCreateRequest{
		QuotationID:    input.QuotationID,
		RevisionNumber: &revision,
		ReceivedDate:   input.ReceivedDate,
	}, &response); err != nil {
		return PurchaseOrderDTO{}, err
	}
	return mapPurchaseOrderDTO(response), nil
}

// GetPurchaseOrder reopens one PO by id.
func (q *Quotation) GetPurchaseOrder(id int64) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
	}
	var response apicontract.PurchaseOrderRow
	if err := client.Get("/v1/purchase-orders/"+strconv.FormatInt(id, 10), &response); err != nil {
		return PurchaseOrderDTO{}, err
	}
	return mapPurchaseOrderDTO(response), nil
}

// UpdatePurchaseOrderDraft replaces the editable draft values.
func (q *Quotation) UpdatePurchaseOrderDraft(input PurchaseOrderUpdateInput) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
	}
	items := make([]apicontract.PurchaseOrderItemInput, 0, len(input.Items))
	for _, item := range input.Items {
		items = append(items, apicontract.PurchaseOrderItemInput{
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			Description:     item.Description,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
		})
	}
	var response apicontract.PurchaseOrderRow
	if err := client.Put("/v1/purchase-orders/"+strconv.FormatInt(input.ID, 10),
		apicontract.PurchaseOrderUpdateRequest{
			BillingAddress:        input.BillingAddress,
			DeliveryAddress:       input.DeliveryAddress,
			ShipToCode:            input.ShipToCode,
			ContactEmail:          input.ContactEmail,
			ContactPhone:          input.ContactPhone,
			PONumber:              input.PONumber,
			PODate:                input.PODate,
			ReceivedDate:          input.ReceivedDate,
			RequestedDeliveryDate: input.RequestedDeliveryDate,
			PaymentTerms:          input.PaymentTerms,
			Incoterms:             input.Incoterms,
			Items:                 items,
		}, &response); err != nil {
		return PurchaseOrderDTO{}, err
	}
	return mapPurchaseOrderDTO(response), nil
}

func mapPurchaseOrderDTO(row apicontract.PurchaseOrderRow) PurchaseOrderDTO {
	dto := PurchaseOrderDTO{
		ID:                    row.ID,
		UUID:                  row.UUID,
		State:                 row.State,
		QuotationID:           row.QuotationID,
		SourceRevision:        row.SourceRevision,
		QuoteNumber:           row.QuoteNumber,
		CustomerID:            row.CustomerID,
		CustomerName:          row.CustomerName,
		BillingAddress:        row.BillingAddress,
		DeliveryAddress:       row.DeliveryAddress,
		ShipToCode:            row.ShipToCode,
		ContactEmail:          row.ContactEmail,
		ContactPhone:          row.ContactPhone,
		PONumber:              row.PONumber,
		PODate:                row.PODate,
		ReceivedDate:          row.ReceivedDate,
		RequestedDeliveryDate: row.RequestedDeliveryDate,
		PaymentTerms:          row.PaymentTerms,
		Incoterms:             row.Incoterms,
		Currency:              row.Currency,
		TaxZone:               row.TaxZone,
		TaxCategory:           row.TaxCategory,
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}
	dto.Items = make([]PurchaseOrderItemDTO, 0, len(row.Items))
	for _, item := range row.Items {
		dto.Items = append(dto.Items, PurchaseOrderItemDTO{
			LineNumber:      item.LineNumber,
			Description:     item.Description,
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
		})
	}
	return dto
}
