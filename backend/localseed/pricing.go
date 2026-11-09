package localseed

import (
	"fmt"

	"vera-app/backend/store"
)

// priceQuotationTotals applies the quotation pricing pipeline to fixture data.
//
// It exists because the seed builds immutable revisions directly through the
// store rather than through the desktop's Prepare/Export path, which needs a
// signed-in Wails session. The rules are the documented ones and are pinned by
// a worked-example test:
//
//	margin first, then the row discount; a package is the sum of its parts;
//	the cost side never moves; the whole-quotation discount applies to the
//	summed post-row subtotal; PPN is added on top for Non-FTZ goods and PPH22
//	is internal only.
//
// The seed is fixture data, not a second pricing implementation: no production
// path imports it, and the test below asserts the same arithmetic the backend
// builder uses.
func priceQuotationTotals(
	items []store.RevisionItem,
	discountPercent int16,
	taxZone, taxType, serviceCategory string,
) (store.RevisionTotals, error) {
	var originalSubtotal, marginalizedSubtotal int64
	for _, item := range items {
		originalSubtotal += item.OriginalTotal
		marginalizedSubtotal += item.MarginalizedTotal
	}
	wholeDiscount := int64(discountPercent)
	if wholeDiscount < 0 || wholeDiscount > 100 {
		return store.RevisionTotals{}, fmt.Errorf("fixture discount %d is outside 0..100", wholeDiscount)
	}
	marginalizedDiscount := marginalizedSubtotal * wholeDiscount / 100
	marginalizedGrand := marginalizedSubtotal - marginalizedDiscount

	ppnBps, err := store.PurchaseOrderPPNBps(taxZone, taxType, serviceCategory, "")
	if err != nil {
		return store.RevisionTotals{}, fmt.Errorf("fixture tax basis: %w", err)
	}
	serviceBps := int64(0)
	if taxType == "Services" {
		switch serviceCategory {
		case "NonConstruction":
			serviceBps = 200
		case "Construction":
			serviceBps = 175
		default:
			return store.RevisionTotals{}, fmt.Errorf("fixture services category %q is unknown", serviceCategory)
		}
	}
	ppnAmount := marginalizedGrand * int64(ppnBps) / 10000
	serviceTax := marginalizedGrand * serviceBps / 10000
	return store.RevisionTotals{
		OriginalSubtotal:          originalSubtotal,
		OriginalDiscount:          0,
		OriginalGrand:             originalSubtotal,
		MarginalizedSubtotal:      marginalizedSubtotal,
		MarginalizedDiscount:      marginalizedDiscount,
		MarginalizedGrand:         marginalizedGrand,
		TaxAmount:                 serviceTax,
		MarginalizedGrandAfterTax: marginalizedGrand - serviceTax,
		ProfitAmount:              marginalizedGrand - originalSubtotal,
		NettProfit:                marginalizedGrand - serviceTax - originalSubtotal,
		PpnAmount:                 ppnAmount,
		CustomerGrandTotal:        marginalizedGrand + ppnAmount,
	}, nil
}

func priceItem(spec ItemSpec, itemNo int) (store.RevisionItem, error) {
	if spec.Qty <= 0 {
		return store.RevisionItem{}, fmt.Errorf("fixture item %q has a non-positive quantity", spec.Description)
	}
	if len(spec.SubItems) > 0 {
		return pricePackage(spec, itemNo)
	}
	listUnit, netUnit := listAndNet(spec.UnitCost, spec.MarginEnabled, spec.MarginPercent,
		spec.DiscountEnabled, spec.DiscountPercent)
	costTotal := spec.Qty * spec.UnitCost
	listTotal := spec.Qty * listUnit
	netTotal := spec.Qty * netUnit
	return store.RevisionItem{
		ItemNo:                itemNo,
		Description:           spec.Description,
		UoM:                   spec.UoM,
		Qty:                   spec.Qty,
		UnitPrice:             spec.UnitCost,
		MarginEnabled:         spec.MarginEnabled,
		MarginPercent:         spec.MarginPercent,
		MarginAmount:          netUnit - spec.UnitCost,
		OriginalTotal:         costTotal,
		MarginalizedUnitPrice: netUnit,
		MarginalizedTotal:     netTotal,
		ListUnitPrice:         listUnit,
		DiscountAmount:        listTotal - netTotal,
		DiscountEnabled:       spec.DiscountEnabled,
		DiscountPercent:       spec.DiscountPercent,
	}, nil
}

// pricePackage rolls a modular line up from its priced parts. The parent is
// always one lot: its cost, list and net totals are the sums of the parts, and
// its discount percentage is the parts' summed discount over their summed list.
func pricePackage(spec ItemSpec, itemNo int) (store.RevisionItem, error) {
	subs := make([]store.RevisionSubItem, 0, len(spec.SubItems))
	var costTotal, listTotal, netTotal int64
	for index, subSpec := range spec.SubItems {
		if subSpec.Qty <= 0 {
			return store.RevisionItem{}, fmt.Errorf("fixture sub-item %q has a non-positive quantity", subSpec.Description)
		}
		listUnit, netUnit := listAndNet(subSpec.UnitCost, subSpec.MarginEnabled, subSpec.MarginPercent,
			subSpec.DiscountEnabled, subSpec.DiscountPercent)
		subCost := subSpec.Qty * subSpec.UnitCost
		subList := subSpec.Qty * listUnit
		subNet := subSpec.Qty * netUnit
		costTotal += subCost
		listTotal += subList
		netTotal += subNet
		details := make([]store.RevisionSubSubItem, 0, len(subSpec.Details))
		for detailIndex, detail := range subSpec.Details {
			details = append(details, store.RevisionSubSubItem{SubSubNo: detailIndex + 1, Description: detail})
		}
		subs = append(subs, store.RevisionSubItem{
			SubNo:           index + 1,
			Description:     subSpec.Description,
			UoM:             subSpec.UoM,
			Qty:             subSpec.Qty,
			UnitCost:        subSpec.UnitCost,
			UnitPrice:       netUnit,
			MarginAmount:    netUnit - subSpec.UnitCost,
			Total:           subNet,
			ListUnitPrice:   listUnit,
			DiscountAmount:  subList - subNet,
			MarginEnabled:   subSpec.MarginEnabled,
			MarginPercent:   subSpec.MarginPercent,
			DiscountEnabled: subSpec.DiscountEnabled,
			DiscountPercent: subSpec.DiscountPercent,
			SubSubItems:     details,
		})
	}
	discountPercent := int16(0)
	if listTotal > 0 {
		discountPercent = int16((listTotal - netTotal) * 100 / listTotal)
	}
	return store.RevisionItem{
		ItemNo:                itemNo,
		Description:           spec.Description,
		UoM:                   spec.UoM,
		Qty:                   1,
		UnitPrice:             costTotal,
		OriginalTotal:         costTotal,
		MarginalizedUnitPrice: netTotal,
		MarginalizedTotal:     netTotal,
		ListUnitPrice:         listTotal,
		DiscountAmount:        listTotal - netTotal,
		DiscountEnabled:       listTotal > netTotal,
		DiscountPercent:       discountPercent,
		ShowSubPrices:         spec.ShowSubPrices,
		SubItems:              subs,
	}, nil
}

// listAndNet applies the documented order: margin first, then discount. Both
// truncate, exactly as backend/builder.go does.
func listAndNet(cost int64, marginEnabled bool, marginPercent int16, discountEnabled bool, discountPercent int16) (listUnit, netUnit int64) {
	listUnit = cost
	if marginEnabled {
		listUnit = cost + cost*int64(marginPercent)/100
	}
	netUnit = listUnit
	if discountEnabled {
		netUnit = listUnit - listUnit*int64(discountPercent)/100
	}
	return listUnit, netUnit
}
