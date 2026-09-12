package backend

import (
	"fmt"
	"time"

	"vera-app/backend/store"
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
	TaxRateBps                  int16
	OriginalSubtotal            int64
	OriginalDiscount            int64
	OriginalGrand               int64
	MarginalizedSubtotal        int64
	MarginalizedDiscount        int64
	MarginalizedGrand           int64
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
	if q.store == nil {
		return nil, fmt.Errorf("Postgres store is not configured")
	}
	rows, err := q.store.ListQuotations(q.context())
	if err != nil {
		return nil, err
	}
	result := make([]ArchivedQuotationSummary, 0, len(rows))
	for _, row := range rows {
		result = append(result, ArchivedQuotationSummary{
			ID:             row.ID,
			QuoteNumber:    row.QuoteNumber,
			RevisionNumber: row.RevisionNumber,
			CustomerName:   row.CustomerName,
			Subject:        row.Subject,
			CreatedAt:      row.CreatedAt.Format(time.RFC3339),
			GrandTotal:     row.GrandTotal,
			DisplayGrand:   formatSignedRupiah(row.GrandTotal),
		})
	}
	return result, nil
}

func (q *Quotation) GetQuotation(id int64) (ArchivedQuotation, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return ArchivedQuotation{}, err
	}
	if q.store == nil {
		return ArchivedQuotation{}, fmt.Errorf("Postgres store is not configured")
	}
	record, err := q.store.GetQuotation(q.context(), id, nil)
	if err != nil {
		return ArchivedQuotation{}, err
	}
	return mapArchivedQuotation(record), nil
}

func (q *Quotation) GetQuotationRevision(id int64, revision int16) (ArchivedQuotation, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return ArchivedQuotation{}, err
	}
	if q.store == nil {
		return ArchivedQuotation{}, fmt.Errorf("Postgres store is not configured")
	}
	record, err := q.store.GetQuotation(q.context(), id, &revision)
	if err != nil {
		return ArchivedQuotation{}, err
	}
	return mapArchivedQuotation(record), nil
}

func (q *Quotation) DeleteQuotation(id int64) error {
	if err := q.requireAdmin(q.ctx); err != nil {
		return err
	}
	if q.store == nil {
		return fmt.Errorf("Postgres store is not configured")
	}
	return q.store.DeleteQuotation(q.context(), id)
}

func mapArchivedQuotation(record store.QuotationRecord) ArchivedQuotation {
	items := make([]ArchivedQuotationItem, 0, len(record.Items))
	for _, item := range record.Items {
		items = append(items, ArchivedQuotationItem{
			ItemNo:                   item.ItemNo,
			Description:              item.Description,
			Qty:                      item.Qty,
			UoM:                      item.UoM,
			UnitPrice:                item.UnitPrice,
			DisplayUnitPrice:         formatRupiah(uint64(item.UnitPrice)),
			MarginEnabled:            item.MarginEnabled,
			MarginPercent:            item.MarginPercent,
			MarginAmount:             item.MarginAmount,
			DisplayMarginAmount:      formatSignedRupiah(item.MarginAmount),
			OriginalTotal:            item.OriginalTotal,
			DisplayOriginalTotal:     formatSignedRupiah(item.OriginalTotal),
			MarginalizedUnitPrice:    item.MarginalizedUnitPrice,
			DisplayMarginalizedUnit:  formatSignedRupiah(item.MarginalizedUnitPrice),
			MarginalizedTotal:        item.MarginalizedTotal,
			DisplayMarginalizedTotal: formatSignedRupiah(item.MarginalizedTotal),
		})
	}

	return ArchivedQuotation{
		ArchivedQuotationSummary: ArchivedQuotationSummary{
			ID:             record.ID,
			QuoteNumber:    record.QuoteNumber,
			RevisionNumber: record.RevisionNumber,
			CustomerName:   record.CustomerName,
			Subject:        record.Subject,
			CreatedAt:      record.CreatedAt.Format(time.RFC3339),
			GrandTotal:     record.GrandTotal,
			DisplayGrand:   formatSignedRupiah(record.GrandTotal),
		},
		CustomerCode:                record.CustomerCode,
		CustomerAddress:             record.CustomerAddress,
		RecipientName:               record.RecipientName,
		RecipientAddress:            record.RecipientAddress,
		Incoterm:                    record.Incoterm,
		ValidityDays:                record.ValidityDays,
		PaymentTermDays:             record.PaymentTermDays,
		LeadTimeDays:                record.LeadTimeDays,
		DiscountPercent:             record.DiscountPercent,
		TaxZone:                     record.TaxZone,
		TaxCategory:                 record.TaxCategory,
		TaxRateBps:                  record.TaxRateBps,
		OriginalSubtotal:            record.Totals.OriginalSubtotal,
		OriginalDiscount:            record.Totals.OriginalDiscount,
		OriginalGrand:               record.Totals.OriginalGrand,
		MarginalizedSubtotal:        record.Totals.MarginalizedSubtotal,
		MarginalizedDiscount:        record.Totals.MarginalizedDiscount,
		MarginalizedGrand:           record.Totals.MarginalizedGrand,
		TaxAmount:                   record.Totals.TaxAmount,
		MarginalizedAfterTax:        record.Totals.MarginalizedGrandAfterTax,
		ProfitAmount:                record.Totals.ProfitAmount,
		NettProfit:                  record.Totals.NettProfit,
		DisplayOriginalSubtotal:     formatSignedRupiah(record.Totals.OriginalSubtotal),
		DisplayOriginalDiscount:     formatSignedRupiah(record.Totals.OriginalDiscount),
		DisplayOriginalGrand:        formatSignedRupiah(record.Totals.OriginalGrand),
		DisplayMarginalizedSubtotal: formatSignedRupiah(record.Totals.MarginalizedSubtotal),
		DisplayMarginalizedDiscount: formatSignedRupiah(record.Totals.MarginalizedDiscount),
		DisplayMarginalizedGrand:    formatSignedRupiah(record.Totals.MarginalizedGrand),
		DisplayTax:                  formatSignedRupiah(record.Totals.TaxAmount),
		DisplayMarginalizedAfterTax: formatSignedRupiah(record.Totals.MarginalizedGrandAfterTax),
		DisplayProfit:               formatSignedRupiah(record.Totals.ProfitAmount),
		DisplayNett:                 formatSignedRupiah(record.Totals.NettProfit),
		Items:                       items,
		HTMLSnapshot:                record.HTMLSnapshot,
		AvailableRevisions:          record.AvailableRevisions,
	}
}
