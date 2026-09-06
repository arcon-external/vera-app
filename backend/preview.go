package backend

import (
	"fmt"
	"html/template"
)

// CustomerPreviewItem mirrors QuotationItem but only carries the
// customer-facing sell prices that also appear in the exported PDF.
type CustomerPreviewItem struct {
	Description      string
	DisplayDesc      template.HTML
	UoM              string
	DisplayUnitPrice string
	DisplayTotal     string
	UnitPrice        uint64
	Total            uint64
	No               uint32
	Qty              uint32
}

// CustomerPreview is the exact customer-facing quotation: sell prices only.
// It must never contain margin, tax, or nett figures.
type CustomerPreview struct {
	Items                 []CustomerPreviewItem
	Subject               string
	RecipientName         string
	RecipientCompany      string
	RecipientAddress      string
	Date                  string
	ValidUntil            string
	Incoterm              string
	DisplaySubTotal       string
	DisplayDiscountAmount string
	DisplayGrandTotal     string
	SubTotal              uint64
	DiscountAmount        uint64
	GrandTotal            uint64
	DiscountPercent       uint8
	Validity              uint8
	PaymentTerm           uint8
	LeadTime              uint8
}

// InternalPreviewItem shows the cost vs sell economics of one line.
type InternalPreviewItem struct {
	Description string
	CostUnit    uint64
	SellUnit    uint64
	CostTotal   uint64
	SellTotal   uint64
	No          uint32
	Qty         uint32
}

// InternalBreakdown is the internal-only economics view. It must never be
// rendered into the customer-facing PDF.
type InternalBreakdown struct {
	Items           []InternalPreviewItem
	TaxLabel        string
	DisplayCost     string
	DisplaySell     string
	DisplayCostDisc string
	DisplaySellDisc string
	DisplayCostGrd  string
	DisplayGrand    string
	DisplayProfit   string
	DisplayTax      string
	DisplayNett     string
	CostSubtotal    uint64
	SellSubtotal    uint64
	DiscountCost    uint64
	DiscountSell    uint64
	CostGrand       uint64
	GrandTotal      uint64
	ProfitAmount    uint64
	TaxAmount       uint64
	NettProfit      uint64
	TaxRateBps      uint16
	MarginPercent   uint8
	DiscountPercent uint8
}

// GetCustomerPreview returns the customer-facing quotation derived from the
// last successful Prepare call. It fails if Prepare has not been called.
func (q *Quotation) GetCustomerPreview() (CustomerPreview, error) {
	if !q.prepared {
		return CustomerPreview{}, fmt.Errorf("no prepared quotation: call Prepare first")
	}

	items := make([]CustomerPreviewItem, 0, len(q.Data.Items))
	for _, it := range q.Data.Items {
		items = append(items, CustomerPreviewItem{
			Description:      it.Description,
			DisplayDesc:      it.DisplayDesc,
			UoM:              it.UoM,
			DisplayUnitPrice: it.DisplayUnitPrice,
			DisplayTotal:     it.DisplayTotal,
			UnitPrice:        it.UnitPrice,
			Total:            it.Total,
			No:               it.No,
			Qty:              it.Qty,
		})
	}

	return CustomerPreview{
		Items:                 items,
		Subject:               q.Data.Subject,
		RecipientName:         q.Data.RecipientName,
		RecipientCompany:      q.Data.RecipientCompany,
		RecipientAddress:      q.Data.RecipientAddress,
		Date:                  q.Data.Date,
		ValidUntil:            q.Data.ValidUntil,
		Incoterm:              q.Data.Incoterm,
		DisplaySubTotal:       q.Data.DisplaySubTotal,
		DisplayDiscountAmount: q.Data.DisplayDiscountAmount,
		DisplayGrandTotal:     q.Data.DisplayGrandTotal,
		SubTotal:              q.Data.SubTotal,
		DiscountAmount:        q.Data.DiscountAmount,
		GrandTotal:            q.Data.GrandTotal,
		DiscountPercent:       q.Data.DiscountPercent,
		Validity:              q.Data.Validity,
		PaymentTerm:           q.Data.PaymentTerm,
		LeadTime:              q.Data.LeadTime,
	}, nil
}

// GetInternalBreakdown returns the internal-only cost/margin/tax/nett view
// derived from the last successful Prepare call. It fails if Prepare has
// not been called.
func (q *Quotation) GetInternalBreakdown() (InternalBreakdown, error) {
	if !q.prepared {
		return InternalBreakdown{}, fmt.Errorf("no prepared quotation: call Prepare first")
	}

	costItems := buildItems(q.lastInput.Items, 0)

	items := make([]InternalPreviewItem, 0, len(costItems))
	var costSubtotal uint64
	for i, ci := range costItems {
		var sellTotal uint64
		var sellUnit uint64
		if i < len(q.Data.Items) {
			sellTotal = q.Data.Items[i].Total
			sellUnit = q.Data.Items[i].UnitPrice
		}
		costSubtotal += ci.Total
		items = append(items, InternalPreviewItem{
			Description: ci.Description,
			CostUnit:    ci.UnitPrice,
			SellUnit:    sellUnit,
			CostTotal:   ci.Total,
			SellTotal:   sellTotal,
			No:          ci.No,
			Qty:         ci.Qty,
		})
	}

	discountCost := costSubtotal * uint64(q.Data.DiscountPercent) / 100
	costGrand := costSubtotal - discountCost
	profit := q.Data.GrandTotal - costGrand

	return InternalBreakdown{
		Items:           items,
		TaxLabel:        q.Data.TaxLabel,
		DisplayCost:     formatRupiah(costSubtotal),
		DisplaySell:     q.Data.DisplaySubTotal,
		DisplayCostDisc: formatRupiah(discountCost),
		DisplaySellDisc: q.Data.DisplayDiscountAmount,
		DisplayCostGrd:  formatRupiah(costGrand),
		DisplayGrand:    q.Data.DisplayGrandTotal,
		DisplayProfit:   formatRupiah(profit),
		DisplayTax:      q.Data.DisplayTaxAmount,
		DisplayNett:     q.Data.DisplayNettProfit,
		CostSubtotal:    costSubtotal,
		SellSubtotal:    q.Data.SubTotal,
		DiscountCost:    discountCost,
		DiscountSell:    q.Data.DiscountAmount,
		CostGrand:       costGrand,
		GrandTotal:      q.Data.GrandTotal,
		ProfitAmount:    profit,
		TaxAmount:       q.Data.TaxAmount,
		NettProfit:      q.Data.NettProfit,
		TaxRateBps:      q.Data.TaxRateBps,
		MarginPercent:   q.Data.ProfitMargin,
		DiscountPercent: q.Data.DiscountPercent,
	}, nil
}
