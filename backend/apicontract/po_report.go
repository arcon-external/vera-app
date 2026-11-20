package apicontract

import "vera-app/backend/poreport"

// Issue #629: the purchase-order report's HTTP contract. The domain package is
// the one declaration of the response shape, because the desktop binding
// returns poreport.Report directly and Wails generates its model from that Go
// type. These aliases let the server name the wire type without copying it; a
// JSON tag can never drift between client and server because there is only one.
//
// PurchaseOrderReportPath is shared so the server route and the desktop wrapper
// cannot disagree about the URL. The query parameters are the Query struct's
// own json tags (from, to, bucket, category, currency), which the wrapper sends
// verbatim.
const PurchaseOrderReportPath = "/v1/reports/purchase-orders"

// PurchaseOrderReport is the complete report snapshot: actual totals, period
// and category rows, aligned category series, facet population, the monthly
// and yearly projections, the current-period projection and notes. Money is
// integer minor units and percentages are basis points.
type PurchaseOrderReport = poreport.Report

// PurchaseOrderReportQuery is the validated report filter. From is inclusive
// and To exclusive; Bucket is day, month, quarter or year; Category is one of
// all, FM, FAB, SUPPLY_GEN, OTHERS or UNASSIGNED; the currency is a partition,
// never a conversion.
type PurchaseOrderReportQuery = poreport.Query
