package backend

import (
	"net/url"
	"strings"

	"vera-app/backend/apicontract"
	"vera-app/backend/poreport"
)

// Issue #629: the desktop's read of the purchase-order performance report. The
// desktop has no report arithmetic of its own: it calls the same authenticated
// endpoint the screen and the PDF share, sends the validated filter, and returns
// the server's snapshot unchanged. Keeping one payload is what makes the screen
// and the PDF agree by construction rather than by reconciliation.
//
// The query fields travel as the contract's own wire names; an empty category
// or currency is omitted so the server applies its documented defaults (All,
// and the company currency). An invalid value is the server's typed 400/422,
// surfaced by apiclient exactly as every other binding surfaces it.

// GetPurchaseOrderReport fetches the complete report snapshot for one filter.
// From is inclusive and To exclusive; Bucket is month, quarter or year;
// Category is all, FM, FAB, SUPPLY_GEN, OTHERS or UNASSIGNED; Currency is one
// ISO code and is never converted.
func (q *Quotation) GetPurchaseOrderReport(query poreport.Query) (poreport.Report, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return poreport.Report{}, err
	}
	client, err := api()
	if err != nil {
		return poreport.Report{}, err
	}
	params := url.Values{}
	params.Set("from", strings.TrimSpace(query.From))
	params.Set("to", strings.TrimSpace(query.To))
	params.Set("bucket", strings.TrimSpace(query.Bucket))
	if category := strings.TrimSpace(query.Category); category != "" {
		params.Set("category", category)
	}
	if currency := strings.TrimSpace(query.Currency); currency != "" {
		params.Set("currency", currency)
	}

	var report poreport.Report
	if err := client.Get(apicontract.PurchaseOrderReportPath+"?"+params.Encode(), &report); err != nil {
		return poreport.Report{}, err
	}
	return report, nil
}
