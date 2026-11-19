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

// isPurchaseOrderBusinessCategory reports whether a non-empty value is one of
// the four codes. Empty is deliberately not a category here: callers that allow
// a draft's Unassigned value say so explicitly.
func isPurchaseOrderBusinessCategory(value string) bool {
	switch value {
	case PurchaseOrderBusinessCategoryFm,
		PurchaseOrderBusinessCategoryFab,
		PurchaseOrderBusinessCategorySupplyGeneral,
		PurchaseOrderBusinessCategoryOthers:
		return true
	default:
		return false
	}
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
	if isPurchaseOrderBusinessCategory(candidate) {
		return candidate, nil
	}
	return PurchaseOrderBusinessCategoryUnassigned, fmt.Errorf(
		"%w: business category %q must be one of FM, FAB, SUPPLY_GEN or OTHERS",
		ErrPurchaseOrderInvalid, value)
}
