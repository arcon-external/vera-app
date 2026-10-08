package backend

import (
	"strconv"

	"vera-app/backend/apicontract"
)

type ArchivedQuotationSummary struct {
	QuoteNumber    string
	CustomerName   string
	Subject        string
	CreatedAt      string
	DisplayGrand   string
	ID             int64
	GrandTotal     int64
	RevisionNumber int16
}

type ArchivedQuotationItem struct {
	DisplayMarginAmount      string
	Description              string
	DisplayMarginalizedTotal string
	UoM                      string
	DisplayMarginalizedUnit  string
	DisplayUnitPrice         string
	DisplayOriginalTotal     string
	Subs                     []ArchivedQuotationSubItem
	MarginAmount             int64
	ItemNo                   int
	OriginalTotal            int64
	MarginalizedUnitPrice    int64
	UnitPrice                int64
	MarginalizedTotal        int64
	Qty                      int64
	MarginPercent            int16
	MarginEnabled            bool
	ShowSubPrices            bool
}

// ArchivedQuotationSubItem is one sub-item of an itemized line as the
// archive returns it: both sides, so reopening a quotation restores the
// composition exactly as it was issued.
type ArchivedQuotationSubItem struct {
	DisplayMarginAmount      string
	Description              string
	Details                  []string
	DisplayMarginalizedTotal string
	UoM                      string
	DisplayMarginalizedUnit  string
	DisplayOriginalUnit      string
	DisplayOriginalTotal     string
	MarginAmount             int64
	SubNo                    int
	OriginalUnitPrice        int64
	OriginalTotal            int64
	MarginalizedUnitPrice    int64
	MarginalizedTotal        int64
	Qty                      int64
	MarginPercent            int16
	MarginEnabled            bool
}

type ArchivedQuotation struct {
	DisplayPpn                  string
	DisplayOriginalGrand        string
	DisplayOriginalDiscount     string
	RecipientName               string
	RecipientAddress            string
	Incoterm                    string
	HTMLSnapshot                string
	DisplayNett                 string
	DisplayProfit               string
	DisplayMarginalizedAfterTax string
	TaxZone                     string
	TaxCategory                 string
	TaxType                     string
	ServiceCategory             string
	DisplayTax                  string
	DisplayMarginalizedGrand    string
	DisplayMarginalizedDiscount string
	DisplayMarginalizedSubtotal string
	DisplayOriginalSubtotal     string
	CustomerAddress             string
	Items                       []ArchivedQuotationItem
	AvailableRevisions          []int16
	ArchivedQuotationSummary
	OriginalGrand        int64
	OriginalSubtotal     int64
	MarginalizedAfterTax int64
	PpnAmount            int64
	NettProfit           int64
	MarginalizedSubtotal int64
	MarginalizedDiscount int64
	ProfitAmount         int64
	OriginalDiscount     int64
	TaxAmount            int64
	MarginalizedGrand    int64
	CustomerGrandTotal   int64
	CustomerCode         int16
	LeadTimeDays         int16
	PaymentTermDays      int16
	DiscountPercent      int16
	ValidityDays         int16
	TaxRateBps           int16
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
		subs := make([]ArchivedQuotationSubItem, 0, len(item.Subs))
		for _, sub := range item.Subs {
			subs = append(subs, ArchivedQuotationSubItem{
				SubNo:                    sub.SubNo,
				Description:              sub.Description,
				Details:                  append([]string(nil), sub.Details...),
				Qty:                      sub.Qty,
				UoM:                      sub.UoM,
				OriginalUnitPrice:        sub.OriginalUnitPrice,
				MarginEnabled:            sub.MarginEnabled,
				MarginPercent:            sub.MarginPercent,
				MarginAmount:             sub.MarginAmount,
				OriginalTotal:            sub.OriginalTotal,
				MarginalizedUnitPrice:    sub.MarginalizedUnitPrice,
				MarginalizedTotal:        sub.MarginalizedTotal,
				DisplayOriginalUnit:      sub.DisplayOriginalUnit,
				DisplayOriginalTotal:     sub.DisplayOriginalTotal,
				DisplayMarginalizedUnit:  sub.DisplayMarginalizedUnit,
				DisplayMarginalizedTotal: sub.DisplayMarginalizedTotal,
				DisplayMarginAmount:      sub.DisplayMarginAmount,
			})
		}
		items = append(items, ArchivedQuotationItem{
			ShowSubPrices:            item.ShowSubPrices,
			Subs:                     subs,
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
		CustomerGrandTotal:          detail.GrandTotal,
		DisplayPpn:                  detail.DisplayPpn,
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
