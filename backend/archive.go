package backend

import (
	"strconv"

	"vera-app/backend/apicontract"
)

type ArchivedQuotationSummary struct {
	ID             int64
	QuoteNumber    string
	RevisionNumber int16
	CustomerName   string
	Subject        string
	CreatedAt      string
	GrandTotal     int64
	DisplayGrand   string
}

type ArchivedQuotationItem struct {
	ItemNo                   int
	Description              string
	Qty                      int64
	UoM                      string
	UnitPrice                int64
	DisplayUnitPrice         string
	MarginEnabled            bool
	MarginPercent            int16
	MarginAmount             int64
	DisplayMarginAmount      string
	OriginalTotal            int64
	DisplayOriginalTotal     string
	MarginalizedUnitPrice    int64
	DisplayMarginalizedUnit  string
	MarginalizedTotal        int64
	DisplayMarginalizedTotal string
}

type ArchivedQuotation struct {
	ArchivedQuotationSummary
	CustomerCode                int16
	CustomerAddress             string
	RecipientName               string
	RecipientAddress            string
	Incoterm                    string
	ValidityDays                int16
	PaymentTermDays             int16
	LeadTimeDays                int16
	DiscountPercent             int16
	TaxZone                     string
	TaxCategory                 string
	TaxType                     string
	ServiceCategory             string
	TaxRateBps                  int16
	OriginalSubtotal            int64
	OriginalDiscount            int64
	OriginalGrand               int64
	MarginalizedSubtotal        int64
	MarginalizedDiscount        int64
	MarginalizedGrand           int64
	PpnAmount                   int64
	CustomerGrandTotal          int64
	TaxAmount                   int64
	MarginalizedAfterTax        int64
	ProfitAmount                int64
	NettProfit                  int64
	DisplayOriginalSubtotal     string
	DisplayOriginalDiscount     string
	DisplayOriginalGrand        string
	DisplayMarginalizedSubtotal string
	DisplayMarginalizedDiscount string
	DisplayMarginalizedGrand    string
	DisplayTax                  string
	DisplayMarginalizedAfterTax string
	DisplayProfit               string
	DisplayNett                 string
	Items                       []ArchivedQuotationItem
	HTMLSnapshot                string
	AvailableRevisions          []int16
}

func (q *Quotation) ListQuotations() ([]ArchivedQuotationSummary, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return nil, err
	}
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Quotations []apicontract.QuotationSummaryRow `json:"quotations"`
	}
	if err := client.Get("/v1/quotations", &response); err != nil {
		return nil, err
	}
	result := make([]ArchivedQuotationSummary, 0, len(response.Quotations))
	for _, row := range response.Quotations {
		result = append(result, ArchivedQuotationSummary{
			ID:             row.ID,
			QuoteNumber:    row.QuoteNumber,
			RevisionNumber: row.RevisionNumber,
			CustomerName:   row.CustomerName,
			Subject:        row.Subject,
			CreatedAt:      row.CreatedAt,
			GrandTotal:     row.GrandTotal,
			DisplayGrand:   row.DisplayGrand,
		})
	}
	return result, nil
}

func (q *Quotation) GetQuotation(id int64) (ArchivedQuotation, error) {
	return q.fetchQuotation("/v1/quotations/" + strconv.FormatInt(id, 10))
}

func (q *Quotation) GetQuotationRevision(id int64, revision int16) (ArchivedQuotation, error) {
	return q.fetchQuotation("/v1/quotations/" + strconv.FormatInt(id, 10) +
		"?revision=" + strconv.Itoa(int(revision)))
}

func (q *Quotation) fetchQuotation(path string) (ArchivedQuotation, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return ArchivedQuotation{}, err
	}
	client, err := api()
	if err != nil {
		return ArchivedQuotation{}, err
	}
	var detail apicontract.QuotationDetail
	if err := client.Get(path, &detail); err != nil {
		return ArchivedQuotation{}, err
	}
	items := make([]ArchivedQuotationItem, 0, len(detail.Items))
	for _, item := range detail.Items {
		items = append(items, ArchivedQuotationItem{
			ItemNo:                   item.ItemNo,
			Description:              item.Description,
			Qty:                      item.Qty,
			UoM:                      item.UoM,
			UnitPrice:                item.OriginalUnitPrice,
			DisplayUnitPrice:         item.DisplayOriginalUnit,
			MarginEnabled:            item.MarginEnabled,
			MarginPercent:            item.MarginPercent,
			MarginAmount:             item.MarginAmount,
			DisplayMarginAmount:      item.DisplayMarginAmount,
			OriginalTotal:            item.OriginalTotal,
			DisplayOriginalTotal:     item.DisplayOriginalTotal,
			MarginalizedUnitPrice:    item.MarginalizedUnitPrice,
			DisplayMarginalizedUnit:  item.DisplayMarginalizedUnit,
			MarginalizedTotal:        item.MarginalizedTotal,
			DisplayMarginalizedTotal: item.DisplayMarginalizedTotal,
		})
	}
	return ArchivedQuotation{
		ArchivedQuotationSummary: ArchivedQuotationSummary{
			ID:             detail.ID,
			QuoteNumber:    detail.QuoteNumber,
			RevisionNumber: detail.Revision,
			CustomerName:   detail.CustomerName,
			Subject:        detail.Subject,
			CreatedAt:      "",
			GrandTotal:     detail.GrandTotal,
			DisplayGrand:   detail.DisplayMarginalizedGrand,
		},
		RecipientName:               detail.RecipientName,
		Incoterm:                    detail.Incoterm,
		DiscountPercent:             int16(detail.DiscountPercent),
		TaxZone:                     detail.TaxZone,
		TaxCategory:                 detail.TaxCategory,
		TaxType:                     detail.TaxType,
		ServiceCategory:             detail.ServiceCategory,
		PpnAmount:                   detail.PpnAmount,
		OriginalSubtotal:            detail.OriginalSubtotal,
		OriginalDiscount:            detail.OriginalDiscount,
		OriginalGrand:               detail.OriginalGrand,
		MarginalizedSubtotal:        detail.MarginalizedSubtotal,
		MarginalizedDiscount:        detail.MarginalizedDiscount,
		MarginalizedGrand:           detail.MarginalizedGrand,
		TaxAmount:                   detail.TaxAmount,
		MarginalizedAfterTax:        detail.MarginalizedGrandAfterTax,
		ProfitAmount:                detail.ProfitAmount,
		NettProfit:                  detail.NettProfit,
		DisplayOriginalSubtotal:     detail.DisplayOriginalSubtotal,
		DisplayOriginalDiscount:     detail.DisplayOriginalDiscount,
		DisplayOriginalGrand:        detail.DisplayOriginalGrand,
		DisplayMarginalizedSubtotal: detail.DisplayMarginalizedSubtotal,
		DisplayMarginalizedDiscount: detail.DisplayMarginalizedDiscount,
		DisplayMarginalizedGrand:    detail.DisplayMarginalizedGrand,
		DisplayTax:                  detail.DisplayTax,
		DisplayMarginalizedAfterTax: detail.DisplayMarginalizedAfterTax,
		DisplayProfit:               detail.DisplayProfit,
		DisplayNett:                 detail.DisplayNett,
		Items:                       items,
		HTMLSnapshot:                detail.HTMLSnapshot,
		AvailableRevisions:          detail.AvailableRevisions,
	}, nil
}

func (q *Quotation) DeleteQuotation(id int64) error {
	if err := q.requireAdmin(q.ctx); err != nil {
		return err
	}
	client, err := api()
	if err != nil {
		return err
	}
	return client.Delete("/v1/quotations/"+strconv.FormatInt(id, 10), nil)
}
