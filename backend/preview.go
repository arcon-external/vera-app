package backend

import (
	"bytes"
	"fmt"
	"html/template"

	"vera-app/backend/render/quotation"
)

// CustomerPreviewItem is one line of the customer's document, plus the cost
// figures the review screen's "before margin" table shows for it.
//
// Like CustomerPreview, it carries no margin: the amount a line earned is on
// InternalPreviewItem, which is where the panel reads it from.
//
// DescriptionLines are the plain item's "- " bullet lines, numbered X.Y
// after the item's No. They are customer-safe -- number and text only, no
// margin, no PPH22, no profit -- because the review panel reads the same
// lines the document prints.
type CustomerPreviewItem struct {
	Description              string
	DisplayDesc              template.HTML
	UoM                      string
	DisplayOriginalUnitPrice string
	DisplayOriginalTotal     string
	DisplayUnitPrice         string
	DisplayTotal             string
	DescriptionLines         []quotation.DescriptionLine
	OriginalUnitPrice        uint64
	OriginalTotal            uint64
	UnitPrice                uint64
	Total                    uint64
	DiscountEnabled          bool
	DiscountPercent          uint16
	No                       uint32
	Qty                      uint32
}

// CustomerPreview is the quotation as the customer sees it, plus the cost side
// the review screen shows beside it.
//
// The cost side is here on purpose: ReviewPanel renders its "before margin"
// table from Items, and those figures are what staff check the margin against.
// What must never appear here is the internal economics -- what a line earned,
// the PPH22 withheld, the nett profit. Those belong to InternalBreakdown, which
// is where the one caller that wants them reads them from.
//
// This struct has been wrong before. It was written in #28 with none of those
// figures, and #48 added three nett totals to it as collateral while adding the
// cost side. Nothing read them for a month, because a comment describing a
// boundary is not a boundary. TestCustomerPreviewCarriesNoInternalEconomics is.
type CustomerPreview struct {
	Incoterm               string
	Subject                string
	RecipientName          string
	RecipientCompany       string
	RecipientAddress       string
	QuoteNumber            string
	Date                   string
	ValidUntil             string
	DisplayPpnAmount       string
	DisplaySubTotal        string
	DisplayDiscountAmount  string
	DisplayGrandTotal      string
	Items                  []CustomerPreviewItem
	SubTotal               uint64
	OriginalGrandTotal     uint64
	DiscountAmount         uint64
	GrandTotal             uint64
	PpnAmount              uint64
	OriginalSubTotal       uint64
	OriginalDiscountAmount uint64
	RevisionID             uint16
	DiscountPercent        uint8
	// DisplayLeadTime is the composed sentence ("2 Years, 10 Months, 4 Weeks,
	// and 5 Days", "-" when none is stated), so the review screen shows the
	// customer exactly what the document prints. The four numerics beside it
	// are the figures as entered, for the editor to restore.
	DisplayLeadTime string
	LeadTimeYears   uint8
	LeadTimeMonths  uint8
	LeadTimeWeeks   uint8
	LeadTimeDays    uint8
	Validity        uint8
	PaymentTerm     uint8
}

// InternalPreviewItem shows the cost vs sell economics of one line.
//
// SubItems carries the line's priced composition when it is a main item with
// sub-items, and the blended fields describe the margin the composition
// earned overall: total profit over total cost. The blend is not any single
// rate -- it is what the summed money works out to -- so a fractional result
// is rounded half-up and marked approximate the same way the editor shows it.
type InternalPreviewItem struct {
	DisplayOriginalUnitPrice string
	DisplayMarginAmount      string
	DisplayMarginalizedTotal string
	DisplayMarginBlended     string
	DisplayMarginalizedUnit  string
	Description              string
	DisplayOriginalTotal     string
	SubItems                 []InternalPreviewSubItem
	SellTotal                uint64
	MarginalizedUnitPrice    uint64
	MarginalizedTotal        uint64
	OriginalTotal            uint64
	OriginalUnitPrice        uint64
	CostTotal                uint64
	SellUnit                 uint64
	CostUnit                 uint64
	MarginAmount             int64
	MarginBlendedPercent     uint32
	No                       uint32
	Qty                      uint32
	MarginPercent            uint16
	DiscountPercent          uint16
	MarginEnabled            bool
	DiscountEnabled          bool
	MarginBlendedApprox      bool
	// ListUnitPrice is what the row would have been charged without its
	// discount, so staff can see what came off. Internal only.
	ListUnitPrice uint64
}

// InternalPreviewSubItem is one priced sub-item inside the internal
// breakdown: both sides, its margin, and its detail lines.
type InternalPreviewSubItem struct {
	Description              string
	DisplayOriginalUnitPrice string
	DisplayMarginAmount      string
	DisplayMarginalizedTotal string
	DisplayMarginalizedUnit  string
	DisplayOriginalTotal     string
	SubNo                    string
	UoM                      string
	Details                  []string
	Qty                      uint32
	MarginPercent            uint16
	OriginalUnitPrice        uint64
	OriginalTotal            uint64
	MarginAmount             int64
	MarginalizedUnitPrice    uint64
	MarginalizedTotal        uint64
	DiscountAmount           uint64
	ListUnitPrice            uint64
	DiscountPercent          uint16
	MarginEnabled            bool
	DiscountEnabled          bool
}

// blendedMarginPercent is total profit over total cost, in whole percent.
// Exact integer results pass through; fractions round half-up and report
// approximate so the caller can mark them. Mirrored by blendedMargin in the
// frontend's format module.
func blendedMarginPercent(costTotal, sellTotal uint64) (uint32, bool) {
	if costTotal == 0 {
		return 0, false
	}
	cost := int64(costTotal)
	numerator := (int64(sellTotal) - cost) * 100
	approx := numerator%cost != 0
	percent := (2*numerator + cost) / (2 * cost)
	if percent < 0 {
		percent = 0
	}
	return uint32(percent), approx
}

// InternalBreakdown is the internal-only economics view. It must never be
// rendered into the customer-facing PDF.
type InternalBreakdown struct {
	DisplayMarginalizedGrand    string
	DisplayNett                 string
	TaxType                     string
	ServiceCategory             string
	DisplayMarginalizedAfterTax string
	DisplayPpn                  string
	DisplayCost                 string
	DisplaySell                 string
	DisplayCostDisc             string
	DisplaySellDisc             string
	DisplayCostGrd              string
	DisplayGrand                string
	DisplayProfit               string
	DisplayTax                  string
	DisplayMarginalizedDiscount string
	DisplayRowDiscountTotal     string
	DisplayOriginalSubtotal     string
	DisplayOriginalDiscount     string
	DisplayOriginalGrand        string
	TaxLabel                    string
	DisplayMarginalizedSubtotal string
	Items                       []InternalPreviewItem
	NettProfit                  int64
	TaxAmount                   uint64
	SellSubtotal                uint64
	MarginalizedGrandAfterTax   uint64
	PpnAmount                   uint64
	CostGrand                   uint64
	GrandTotal                  uint64
	ProfitAmount                int64
	CostSubtotal                uint64
	DiscountSell                uint64
	DiscountCost                uint64
	MarginalizedGrand           uint64
	OriginalSubtotal            uint64
	OriginalDiscount            uint64
	OriginalGrand               uint64
	MarginalizedSubtotal        uint64
	MarginalizedDiscount        uint64
	RowDiscountTotal            uint64
	TaxRateBps                  uint16
	DiscountPercent             uint8
}

// GetCustomerPreview returns the customer-facing quotation derived from the
// last successful Prepare call. It fails if Prepare has not been called.
func (q *Quotation) GetCustomerPreview() (CustomerPreview, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return CustomerPreview{}, err
	}
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
			DescriptionLines:         it.DescriptionLines,
			OriginalUnitPrice:        it.OriginalUnitPrice,
			OriginalTotal:            it.OriginalTotal,
			UnitPrice:                it.UnitPrice,
			Total:                    it.Total,
			DiscountEnabled:          it.DiscountEnabled,
			DiscountPercent:          it.DiscountPercent,
			No:                       it.No,
			Qty:                      it.Qty,
		})
	}

	return CustomerPreview{
		Items:                  items,
		Subject:                q.Data.Subject,
		RecipientName:          q.Data.RecipientName,
		RecipientCompany:       q.Data.RecipientCompany,
		RecipientAddress:       q.Data.RecipientAddress,
		QuoteNumber:            q.Data.QuoteNumber,
		RevisionID:             q.Data.RevisionID,
		Date:                   q.Data.Date,
		ValidUntil:             q.Data.ValidUntil,
		Incoterm:               q.Data.Incoterm,
		DisplaySubTotal:        q.Data.DisplaySubTotal,
		DisplayDiscountAmount:  q.Data.DisplayDiscountAmount,
		DisplayGrandTotal:      q.Data.DisplayGrandTotal,
		DisplayPpnAmount:       q.Data.DisplayPpnAmount,
		SubTotal:               q.Data.SubTotal,
		DiscountAmount:         q.Data.DiscountAmount,
		GrandTotal:             q.Data.GrandTotal,
		PpnAmount:              q.Data.PpnAmount,
		OriginalSubTotal:       q.Data.OriginalSubTotal,
		OriginalDiscountAmount: q.Data.OriginalDiscountAmount,
		OriginalGrandTotal:     q.Data.OriginalGrandTotal,
		DiscountPercent:        q.Data.DiscountPercent,
		DisplayLeadTime:        q.Data.LeadTime,
		LeadTimeYears:          q.Data.LeadTimeYears,
		LeadTimeMonths:         q.Data.LeadTimeMonths,
		LeadTimeWeeks:          q.Data.LeadTimeWeeks,
		LeadTimeDays:           q.Data.LeadTimeDays,
		Validity:               q.Data.Validity,
		PaymentTerm:            q.Data.PaymentTerm,
	}, nil
}

// GetQuotationPreviewHTML renders the same customer-facing HTML used by PDF
// export. It intentionally exposes no internal pricing fields to the frontend.
func (q *Quotation) GetQuotationPreviewHTML() (string, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return "", err
	}
	if !q.prepared {
		return "", fmt.Errorf("no prepared quotation: call Prepare first")
	}

	var body bytes.Buffer
	// The preview must match the PDF byte for byte, and the PDF executes the
	// display copy (UoMs resolved to labels, see renderInput), so this path
	// executes the same copy: a stale preview that still printed raw values
	// would show the editor one document and the customer another.
	data := q.displayData()
	if err := q.Data.HtmlTemplate.Execute(&body, data); err != nil {
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
	if err := q.requireUser(q.ctx); err != nil {
		return InternalBreakdown{}, err
	}
	if !q.prepared {
		return InternalBreakdown{}, fmt.Errorf("no prepared quotation: call Prepare first")
	}

	items := make([]InternalPreviewItem, 0, len(q.Data.Items))
	for _, item := range q.Data.Items {
		view := InternalPreviewItem{
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
			DiscountEnabled:          item.DiscountEnabled,
			DiscountPercent:          item.DiscountPercent,
			ListUnitPrice:            item.ListUnitPrice,
			No:                       item.No,
			Qty:                      item.Qty,
		}
		// SubItems is set before the conditional below so a plain line marshals
		// as [] and never as null. A nil slice becomes JSON null, and the
		// review panel reads .length on this field -- null there threw and
		// unmounted the whole app (white screen after Prepare).
		view.SubItems = make([]InternalPreviewSubItem, 0)
		if len(item.SubItems) > 0 {
			subs := make([]InternalPreviewSubItem, 0, len(item.SubItems))
			var costTotal, sellTotal uint64
			for _, sub := range item.SubItems {
				details := make([]string, 0, len(sub.SubSubItems))
				for _, detail := range sub.SubSubItems {
					details = append(details, detail.Description)
				}
				originalTotal := sub.OriginalUnitPrice * uint64(sub.Qty)
				subs = append(subs, InternalPreviewSubItem{
					SubNo:                    sub.SubNo,
					Description:              sub.Description,
					Details:                  details,
					Qty:                      sub.Qty,
					UoM:                      sub.UoM,
					OriginalUnitPrice:        sub.OriginalUnitPrice,
					OriginalTotal:            originalTotal,
					MarginEnabled:            sub.MarginEnabled,
					MarginPercent:            sub.MarginPercent,
					MarginAmount:             sub.MarginAmount,
					MarginalizedUnitPrice:    sub.UnitPrice,
					MarginalizedTotal:        sub.Total,
					ListUnitPrice:            sub.ListUnitPrice,
					DiscountAmount:           sub.DiscountAmount,
					DiscountPercent:          sub.DiscountPercent,
					DiscountEnabled:          sub.DiscountEnabled,
					DisplayOriginalUnitPrice: formatMoney(q.Data.Currency, sub.OriginalUnitPrice),
					DisplayOriginalTotal:     formatMoney(q.Data.Currency, originalTotal),
					DisplayMarginalizedUnit:  formatMoney(q.Data.Currency, sub.UnitPrice),
					DisplayMarginalizedTotal: formatMoney(q.Data.Currency, sub.Total),
					DisplayMarginAmount:      formatSignedMoney(q.Data.Currency, sub.MarginAmount),
				})
				costTotal += originalTotal
				sellTotal += sub.Total
			}
			percent, approx := blendedMarginPercent(costTotal, sellTotal)
			view.MarginBlendedPercent = percent
			view.MarginBlendedApprox = approx
			if approx {
				view.DisplayMarginBlended = fmt.Sprintf("≈%d%%", percent)
			} else {
				view.DisplayMarginBlended = fmt.Sprintf("%d%%", percent)
			}
			view.SubItems = subs
		}
		items = append(items, view)
	}

	return InternalBreakdown{
		Items:                       items,
		TaxLabel:                    q.Data.TaxLabel,
		TaxType:                     q.Data.TaxType,
		ServiceCategory:             q.Data.ServiceCategory,
		PpnAmount:                   q.Data.PpnAmount,
		DisplayPpn:                  q.Data.DisplayPpnAmount,
		DisplayCost:                 formatMoney(q.Data.Currency, q.Data.OriginalSubTotal),
		DisplaySell:                 formatMoney(q.Data.Currency, q.Data.SubTotal),
		DisplayCostDisc:             formatMoney(q.Data.Currency, q.Data.OriginalDiscountAmount),
		DisplaySellDisc:             q.Data.DisplayDiscountAmount,
		DisplayCostGrd:              formatMoney(q.Data.Currency, q.Data.OriginalGrandTotal),
		DisplayGrand:                q.Data.DisplayGrandTotal,
		DisplayProfit:               formatSignedMoney(q.Data.Currency, q.Data.ProfitAmount),
		DisplayTax:                  q.Data.DisplayTaxAmount,
		DisplayNett:                 q.Data.DisplayNettProfit,
		DisplayOriginalSubtotal:     formatMoney(q.Data.Currency, q.Data.OriginalSubTotal),
		DisplayOriginalDiscount:     formatMoney(q.Data.Currency, q.Data.OriginalDiscountAmount),
		DisplayOriginalGrand:        formatMoney(q.Data.Currency, q.Data.OriginalGrandTotal),
		DisplayMarginalizedSubtotal: formatMoney(q.Data.Currency, q.Data.SubTotal),
		DisplayMarginalizedDiscount: formatMoney(q.Data.Currency, q.Data.DiscountAmount),
		DisplayRowDiscountTotal:     formatMoney(q.Data.Currency, q.Data.RowDiscountTotal),
		DisplayMarginalizedGrand:    formatMoney(q.Data.Currency, q.Data.SellGrandTotal),
		DisplayMarginalizedAfterTax: formatMoney(q.Data.Currency, q.Data.MarginalizedGrandAfterTax),
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
		RowDiscountTotal:            q.Data.RowDiscountTotal,
		MarginalizedGrand:           q.Data.SellGrandTotal,
		MarginalizedGrandAfterTax:   q.Data.MarginalizedGrandAfterTax,
	}, nil
}
