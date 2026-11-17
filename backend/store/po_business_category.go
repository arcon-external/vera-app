package store

import (
	"fmt"
	"strings"
)

// Issue #628: the business category every purchase order is reported under.
//
// The four codes are the client's own reporting vocabulary (FM, FAB,
// SUPPLY_GEN, OTHERS). The empty string is a real value only while a draft:
// the final save refuses it, and every record saved before migration 050 keeps
// reading empty rather than having a category inferred for it. The store, the
// API contract, the archive filter and the editor select all read this one
// definition so a code cannot drift between them.
const (
	PurchaseOrderBusinessCategoryUnassigned    = ""
	PurchaseOrderBusinessCategoryFm            = "FM"
	PurchaseOrderBusinessCategoryFab           = "FAB"
	PurchaseOrderBusinessCategorySupplyGeneral = "SUPPLY_GEN"
	PurchaseOrderBusinessCategoryOthers        = "OTHERS"
)

// PurchaseOrderBusinessCategoryOption is one selectable category and the label
// staff read. Labels are product vocabulary, not company data, so they live
// with the codes rather than in a company-held label table.
type PurchaseOrderBusinessCategoryOption struct {
	Code  string
	Label string
}

// PurchaseOrderBusinessCategories is the ordered option list: the four codes in
// the client's workbook order. It never includes the empty draft value; a
// caller that needs Unassigned names it separately.
func PurchaseOrderBusinessCategories() []PurchaseOrderBusinessCategoryOption {
	return []PurchaseOrderBusinessCategoryOption{
		{Code: PurchaseOrderBusinessCategoryFm, Label: "Facility Management"},
		{Code: PurchaseOrderBusinessCategoryFab, Label: "Fabrication"},
		{Code: PurchaseOrderBusinessCategorySupplyGeneral, Label: "Goods Supply & General"},
		{Code: PurchaseOrderBusinessCategoryOthers, Label: "Others"},
	}
}

// IsPurchaseOrderBusinessCategory reports whether a non-empty value is one of
// the four codes. Empty is deliberately not a category here: callers that allow
// a draft's Unassigned value say so explicitly.
func IsPurchaseOrderBusinessCategory(value string) bool {
	candidate := strings.TrimSpace(value)
	if candidate == PurchaseOrderBusinessCategoryUnassigned {
		return false
	}
	for _, option := range PurchaseOrderBusinessCategories() {
		if candidate == option.Code {
			return true
		}
	}
	return false
}

// NormalizePurchaseOrderBusinessCategory validates a submitted value. An empty
// value stays empty (a draft may be Unassigned); anything else must be exactly
// one of the four codes. The error names the field, because the editor shows it
// beside the select and the API returns it inside the refusal.
func NormalizePurchaseOrderBusinessCategory(value string) (string, error) {
	candidate := strings.TrimSpace(value)
	if candidate == PurchaseOrderBusinessCategoryUnassigned {
		return PurchaseOrderBusinessCategoryUnassigned, nil
	}
	if IsPurchaseOrderBusinessCategory(candidate) {
		return candidate, nil
	}
	return PurchaseOrderBusinessCategoryUnassigned, fmt.Errorf(
		"%w: business category %q must be one of FM, FAB, SUPPLY_GEN or OTHERS",
		ErrPurchaseOrderInvalid, value)
}

// PurchaseOrderBusinessCategoryLabel is the label for a stored value. An empty
// value is Unassigned, the honest bucket for every record saved before the
// category existed; an unknown value is returned as-is rather than recast.
func PurchaseOrderBusinessCategoryLabel(value string) string {
	candidate := strings.TrimSpace(value)
	if candidate == PurchaseOrderBusinessCategoryUnassigned {
		return "Unassigned"
	}
	for _, option := range PurchaseOrderBusinessCategories() {
		if candidate == option.Code {
			return option.Label
		}
	}
	return candidate
}
