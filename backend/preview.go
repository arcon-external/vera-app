package backend

import (
	"bytes"
	"fmt"
	"html/template"
)

// CustomerPreviewItem mirrors QuotationItem but only carries the
// customer-facing sell prices that also appear in the exported PDF.
type CustomerPreviewItem struct {
	Description              string
	DisplayDesc              template.HTML
	UoM                      string
	DisplayOriginalUnitPrice string
	DisplayOriginalTotal     string
	DisplayUnitPrice         string
	DisplayTotal             string
	OriginalUnitPrice        uint64
	OriginalTotal            uint64
	UnitPrice                uint64
	Total                    uint64
	MarginEnabled            bool
	MarginPercent            uint8
	MarginAmount             uint64
	No                       uint32
	Qty                      uint32
}

// CustomerPreview is the exact customer-facing quotation: sell prices only.
// It must never contain margin, tax, or nett figures.
type CustomerPreview struct {
	Items                     []CustomerPreviewItem
	Subject                   string
	RecipientName             string
	RecipientCompany          string
	RecipientAddress          string
	QuoteNumber               string
	RevisionID                uint16
	Date                      string
	ValidUntil                string
	Incoterm                  string
	DisplaySubTotal           string
	DisplayDiscountAmount     string
	DisplayGrandTotal         string
	SubTotal                  uint64
	DiscountAmount            uint64
	GrandTotal                uint64
	OriginalSubTotal          uint64
	OriginalDiscountAmount    uint64
	OriginalGrandTotal        uint64
	MarginalizedGrandAfterTax uint64
	ProfitAmount              int64
	NettProfit                int64
	DiscountPercent           uint8
	Validity                  uint8
	PaymentTerm               uint8
	LeadTime                  uint8
}

// InternalPreviewItem shows the cost vs sell economics of one line.
type InternalPreviewItem struct {
	Description              string
	CostUnit                 uint64
	SellUnit                 uint64
	CostTotal                uint64
	SellTotal                uint64
	OriginalUnitPrice        uint64
	OriginalTotal            uint64
	MarginalizedUnitPrice    uint64
	MarginalizedTotal        uint64
	DisplayOriginalUnitPrice string
	DisplayOriginalTotal     string
	DisplayMarginalizedUnit  string
	DisplayMarginalizedTotal string
	DisplayMarginAmount      string
	MarginEnabled            bool
	MarginPercent            uint8
	MarginAmount             uint64
	No                       uint32
	Qty                      uint32
}

// InternalBreakdown is the internal-only economics view. It must never be
// rendered into the customer-facing PDF.
type InternalBreakdown struct {
	Items                       []InternalPreviewItem
	TaxLabel                    string
	DisplayCost                 string
	DisplaySell                 string
	DisplayCostDisc             string
	DisplaySellDisc             string
	DisplayCostGrd              string
	DisplayGrand                string
	DisplayProfit               string
	DisplayTax                  string
	DisplayNett                 string
	DisplayOriginalSubtotal     string
	DisplayOriginalDiscount     string
	DisplayOriginalGrand        string
	DisplayMarginalizedSubtotal string
	DisplayMarginalizedDiscount string
	DisplayMarginalizedGrand    string
	DisplayMarginalizedAfterTax string
	CostSubtotal                uint64
	SellSubtotal                uint64
	DiscountCost                uint64
	DiscountSell                uint64
	CostGrand                   uint64
	GrandTotal                  uint64
	ProfitAmount                int64
	TaxAmount                   uint64
	NettProfit                  int64
	TaxRateBps                  uint16
	DiscountPercent             uint8
	OriginalSubtotal            uint64
	OriginalDiscount            uint64
	OriginalGrand               uint64
	MarginalizedSubtotal        uint64
	MarginalizedDiscount        uint64
	MarginalizedGrand           uint64
	MarginalizedGrandAfterTax   uint64
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
			Description:              it.Description,
			DisplayDesc:              it.DisplayDesc,
			UoM:                      it.UoM,
			DisplayOriginalUnitPrice: it.DisplayOriginalUnitPrice,
			DisplayOriginalTotal:     it.DisplayOriginalTotal,
			DisplayUnitPrice:         it.DisplayUnitPrice,
			DisplayTotal:             it.DisplayTotal,
			OriginalUnitPrice:        it.OriginalUnitPrice,
			OriginalTotal:            it.OriginalTotal,
			UnitPrice:                it.UnitPrice,
			Total:                    it.Total,
			MarginEnabled:            it.MarginEnabled,
			MarginPercent:            it.MarginPercent,
			MarginAmount:             it.MarginAmount,
			No:                       it.No,
			Qty:                      it.Qty,
		})
	}

	return CustomerPreview{
		Items:                     items,
		Subject:                   q.Data.Subject,
		RecipientName:             q.Data.RecipientName,
		RecipientCompany:          q.Data.RecipientCompany,
		RecipientAddress:          q.Data.RecipientAddress,
		QuoteNumber:               q.Data.QuoteNumber,
		RevisionID:                q.Data.RevisionID,
		Date:                      q.Data.Date,
		ValidUntil:                q.Data.ValidUntil,
		Incoterm:                  q.Data.Incoterm,
		DisplaySubTotal:           q.Data.DisplaySubTotal,
		DisplayDiscountAmount:     q.Data.DisplayDiscountAmount,
		DisplayGrandTotal:         q.Data.DisplayGrandTotal,
		SubTotal:                  q.Data.SubTotal,
		DiscountAmount:            q.Data.DiscountAmount,
		GrandTotal:                q.Data.GrandTotal,
		OriginalSubTotal:          q.Data.OriginalSubTotal,
		OriginalDiscountAmount:    q.Data.OriginalDiscountAmount,
		OriginalGrandTotal:        q.Data.OriginalGrandTotal,
		MarginalizedGrandAfterTax: q.Data.MarginalizedGrandAfterTax,
		ProfitAmount:              q.Data.ProfitAmount,
		NettProfit:                q.Data.NettProfit,
		DiscountPercent:           q.Data.DiscountPercent,
		Validity:                  q.Data.Validity,
		PaymentTerm:               q.Data.PaymentTerm,
		LeadTime:                  q.Data.LeadTime,
	}, nil
}

// GetQuotationPreviewHTML renders the same customer-facing HTML used by PDF
// export. It intentionally exposes no internal pricing fields to the frontend.
func (q *Quotation) GetQuotationPreviewHTML() (string, error) {
	if !q.prepared {
		return "", fmt.Errorf("no prepared quotation: call Prepare first")
	}

	var body bytes.Buffer
	if err := q.Data.HtmlTemplate.Execute(&body, q.Data); err != nil {
		return "", fmt.Errorf("execute quotation preview template: %w", err)
	}
	if body.Len() == 0 {
		return "", fmt.Errorf("quotation preview template produced empty output")
	}

	return body.String(), nil
}

// GetInternalBreakdown returns the internal-only cost/margin/tax/nett view
// derived from the last successful Prepare call. It fails if Prepare has
// not been called.
func (q *Quotation) GetInternalBreakdown() (InternalBreakdown, error) {
	if !q.prepared {
		return InternalBreakdown{}, fmt.Errorf("no prepared quotation: call Prepare first")
	}

	items := make([]InternalPreviewItem, 0, len(q.Data.Items))
	for _, item := range q.Data.Items {
		items = append(items, InternalPreviewItem{
			Description:              item.Description,
			CostUnit:                 item.OriginalUnitPrice,
			SellUnit:                 item.UnitPrice,
			CostTotal:                item.OriginalTotal,
			SellTotal:                item.Total,
			OriginalUnitPrice:        item.OriginalUnitPrice,
			OriginalTotal:            item.OriginalTotal,
			MarginalizedUnitPrice:    item.UnitPrice,
			MarginalizedTotal:        item.Total,
			DisplayOriginalUnitPrice: item.DisplayOriginalUnitPrice,
			DisplayOriginalTotal:     item.DisplayOriginalTotal,
			DisplayMarginalizedUnit:  item.DisplayUnitPrice,
			DisplayMarginalizedTotal: item.DisplayTotal,
			DisplayMarginAmount:      item.DisplayMarginAmount,
			MarginEnabled:            item.MarginEnabled,
			MarginPercent:            item.MarginPercent,
			MarginAmount:             item.MarginAmount,
			No:                       item.No,
			Qty:                      item.Qty,
		})
	}

	return InternalBreakdown{
		Items:                       items,
		TaxLabel:                    q.Data.TaxLabel,
		DisplayCost:                 formatRupiah(q.Data.OriginalSubTotal),
		DisplaySell:                 formatRupiah(q.Data.SubTotal),
		DisplayCostDisc:             formatRupiah(q.Data.OriginalDiscountAmount),
		DisplaySellDisc:             q.Data.DisplayDiscountAmount,
		DisplayCostGrd:              formatRupiah(q.Data.OriginalGrandTotal),
		DisplayGrand:                q.Data.DisplayGrandTotal,
		DisplayProfit:               formatSignedRupiah(q.Data.ProfitAmount),
		DisplayTax:                  q.Data.DisplayTaxAmount,
		DisplayNett:                 q.Data.DisplayNettProfit,
		DisplayOriginalSubtotal:     formatRupiah(q.Data.OriginalSubTotal),
		DisplayOriginalDiscount:     formatRupiah(q.Data.OriginalDiscountAmount),
		DisplayOriginalGrand:        formatRupiah(q.Data.OriginalGrandTotal),
		DisplayMarginalizedSubtotal: formatRupiah(q.Data.SubTotal),
		DisplayMarginalizedDiscount: formatRupiah(q.Data.DiscountAmount),
		DisplayMarginalizedGrand:    formatRupiah(q.Data.GrandTotal),
		DisplayMarginalizedAfterTax: formatRupiah(q.Data.MarginalizedGrandAfterTax),
		CostSubtotal:                q.Data.OriginalSubTotal,
		SellSubtotal:                q.Data.SubTotal,
		DiscountCost:                q.Data.OriginalDiscountAmount,
		DiscountSell:                q.Data.DiscountAmount,
		CostGrand:                   q.Data.OriginalGrandTotal,
		GrandTotal:                  q.Data.GrandTotal,
		ProfitAmount:                q.Data.ProfitAmount,
		TaxAmount:                   q.Data.TaxAmount,
		NettProfit:                  q.Data.NettProfit,
		TaxRateBps:                  q.Data.TaxRateBps,
		DiscountPercent:             q.Data.DiscountPercent,
		OriginalSubtotal:            q.Data.OriginalSubTotal,
		OriginalDiscount:            q.Data.OriginalDiscountAmount,
		OriginalGrand:               q.Data.OriginalGrandTotal,
		MarginalizedSubtotal:        q.Data.SubTotal,
		MarginalizedDiscount:        q.Data.DiscountAmount,
		MarginalizedGrand:           q.Data.GrandTotal,
		MarginalizedGrandAfterTax:   q.Data.MarginalizedGrandAfterTax,
	}, nil
}
