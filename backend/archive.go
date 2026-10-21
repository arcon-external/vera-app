package backend

import (
	"strconv"

	"vera-app/backend/apicontract"
	"vera-app/backend/currency"
)

type ArchivedQuotationSummary struct {
	QuoteNumber  string
	CustomerName string
	Subject      string
	// Currency is the code the latest revision was issued in. The archive
	// page resolves per-row entries from it so rows render in their
	// revision currency, not the live profile one (issue #336 Wave B).
	Currency       string
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
	DiscountAmount           int64
	ListUnitPrice            int64
	ItemNo                   int
	OriginalTotal            int64
	MarginalizedUnitPrice    int64
	UnitPrice                int64
	MarginalizedTotal        int64
	Qty                      int64
	MarginPercent            int16
	DiscountPercent          int16
	MarginEnabled            bool
	DiscountEnabled          bool
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
	DiscountAmount           int64
	ListUnitPrice            int64
	SubNo                    int
	OriginalUnitPrice        int64
	OriginalTotal            int64
	MarginalizedUnitPrice    int64
	MarginalizedTotal        int64
	Qty                      int64
	MarginPercent            int16
	DiscountPercent          int16
	MarginEnabled            bool
	DiscountEnabled          bool
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
	// Currency lives on the embedded summary (one field, not two: the
	// bindings generator flattens the embedding, so a second declaration
	// here would emit a duplicate identifier in models.ts).
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
	// DisplayLeadTime is the composed sentence the revision printed
	// ("2 Years, 10 Months, 4 Weeks, and 5 Days", "-" when none was stated).
	// The four numerics beside it are the figures as entered, so reopening a
	// revision in the editor restores the units and not a back-solved total.
	DisplayLeadTime string
	LeadTimeYears   uint8
	LeadTimeMonths  uint8
	LeadTimeWeeks   uint8
	LeadTimeDays    uint8
	// PaymentTermDays and the neighbour terms are the stored revision's own
	// figures: history, not the live profile's defaults.
	PaymentTermDays int16
	DiscountPercent int16
	ValidityDays    int16
	TaxRateBps      int16
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
			Currency:       row.Currency,
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
				Details:                  append(make([]string, 0, len(sub.Details)), sub.Details...),
				Qty:                      sub.Qty,
				UoM:                      sub.UoM,
				OriginalUnitPrice:        sub.OriginalUnitPrice,
				MarginEnabled:            sub.MarginEnabled,
				MarginPercent:            sub.MarginPercent,
				DiscountEnabled:          sub.DiscountEnabled,
				DiscountPercent:          sub.DiscountPercent,
				ListUnitPrice:            sub.ListUnitPrice,
				DiscountAmount:           sub.DiscountAmount,
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
			DiscountEnabled:          item.DiscountEnabled,
			DiscountPercent:          item.DiscountPercent,
			ListUnitPrice:            item.ListUnitPrice,
			DiscountAmount:           item.DiscountAmount,
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
			// Normalised, so a revision that predates the column reads as
			// the default it was signed as rather than an empty string.
			// The editor blocks loading this under a live profile with a
			// different code, rather than reinterpreting the integers.
			Currency:     currency.Normalize(detail.Currency),
			CreatedAt:    "",
			GrandTotal:   detail.GrandTotal,
			DisplayGrand: detail.DisplayMarginalizedGrand,
		},
		RecipientName:               detail.RecipientName,
		RecipientAddress:            detail.RecipientAddress,
		CustomerCode:                detail.CustomerCode,
		CustomerAddress:             detail.CustomerAddress,
		ValidityDays:                detail.ValidityDays,
		PaymentTermDays:             detail.PaymentTermDays,
		Incoterm:                    detail.Incoterm,
		DiscountPercent:             int16(detail.DiscountPercent),
		DisplayLeadTime:             detail.DisplayLeadTime,
		LeadTimeYears:               detail.LeadTimeYears,
		LeadTimeMonths:              detail.LeadTimeMonths,
		LeadTimeWeeks:               detail.LeadTimeWeeks,
		LeadTimeDays:                detail.LeadTimeDays,
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
		// Normalized for the same reason as SubItems above: a nil slice
		// marshals to null, and the detail page reads .length on it.
		AvailableRevisions: append(make([]int16, 0, len(detail.AvailableRevisions)),
			detail.AvailableRevisions...),
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
