package backend

import "fmt"

// Tax zones for quotation pricing.
const (
	TaxZoneFTZ    = "FTZ"
	TaxZoneNonFTZ = "NonFTZ"
)

// Tax types: a quotation is either goods-only or services (services alone
// or combined with goods inside).
const (
	TaxTypeGoods    = "Goods"
	TaxTypeServices = "Services"
)

// Service categories for Services quotations, in any zone.
const (
	ServiceCategoryNonConstruction = "NonConstruction"
	ServiceCategoryConstruction    = "Construction"
)

// Legacy per-quotation categories, superseded by TaxType + ServiceCategory.
// They remain for reading old revisions and for mixed-version rollouts.
const (
	TaxCategorySupplyOnly                = "SupplyOnly"
	TaxCategorySupplyInstall             = "SupplyInstall"
	TaxCategorySupplyInstallConstruction = "SupplyInstallConstruction"
)

// Rate constants in basis points.
const (
	ppnRateBps               = 1100 // 11% PPN on non-FTZ goods (customer-facing)
	servicesNonConstrRateBps = 200  // 2% withholding on services (internal only)
	servicesConstrRateBps    = 175  // 1.75% final tax on construction (internal only)
)

// taxLabels maps tax selections to human-readable labels.
var taxLabels = map[string]string{
	"GoodsFTZ":                "Goods - FTZ (0%)",
	"GoodsNonFTZ":             "Goods - PPN 11%",
	"ServicesNonConstruction": "Services - Non-construction 2% withholding",
	"ServicesConstruction":    "Services - Construction 1.75% final",
}

// resolveTax resolves a zone/type/category triple into two rates: ppnBps
// (added to the customer document; only non-FTZ goods) and servicesBps
// (internal only; never rendered for customers).
func resolveTax(zone, taxType, serviceCategory string) (ppnBps, servicesBps uint16, label string, err error) {
	switch zone {
	case TaxZoneFTZ, TaxZoneNonFTZ:
		// Valid zone; continue below.
	default:
		return 0, 0, "", fmt.Errorf("invalid tax zone: %q", zone)
	}
	switch taxType {
	case TaxTypeGoods:
		if serviceCategory != "" {
			return 0, 0, "", fmt.Errorf("goods must not have a service category")
		}
		if zone == TaxZoneNonFTZ {
			return ppnRateBps, 0, taxLabels["GoodsNonFTZ"], nil
		}
		return 0, 0, taxLabels["GoodsFTZ"], nil
	case TaxTypeServices:
		switch serviceCategory {
		case ServiceCategoryNonConstruction:
			return 0, servicesNonConstrRateBps, taxLabels["ServicesNonConstruction"], nil
		case ServiceCategoryConstruction:
			return 0, servicesConstrRateBps, taxLabels["ServicesConstruction"], nil
		default:
			return 0, 0, "", fmt.Errorf("services require a category: %q", serviceCategory)
		}
	default:
		return 0, 0, "", fmt.Errorf("invalid tax type: %q", taxType)
	}
}

// legacyTaxCategoryToNew maps pre-type-model categories forward so old
// desktop builds and stored revisions keep working during rollouts.
func legacyTaxCategoryToNew(category string) (taxType, serviceCategory string, ok bool) {
	switch category {
	case TaxCategorySupplyOnly:
		return TaxTypeGoods, "", true
	case TaxCategorySupplyInstall:
		return TaxTypeServices, ServiceCategoryNonConstruction, true
	case TaxCategorySupplyInstallConstruction:
		return TaxTypeServices, ServiceCategoryConstruction, true
	default:
		return "", "", false
	}
}

// taxRateFor resolves the legacy single-rate API. It is retained for
// existing callers; new code uses resolveTax. Exactly one of the PPN and
// services rates is ever nonzero, so their sum preserves the old meaning
// of "the rate".
func taxRateFor(zone, category string) (uint16, string, error) {
	if zone == TaxZoneFTZ {
		if category == "" {
			return 0, "FTZ (0%)", nil
		}
		return 0, "", fmt.Errorf("FTZ must not have a tax category")
	}
	taxType, serviceCategory, ok := legacyTaxCategoryToNew(category)
	if !ok {
		return 0, "", fmt.Errorf("invalid tax category for Non-FTZ: %q", category)
	}
	ppnBps, servicesBps, label, err := resolveTax(zone, taxType, serviceCategory)
	if err != nil {
		return 0, "", err
	}
	return ppnBps + servicesBps, label, nil
}

// calcTax computes a tax amount on a grand total using integer math. bps is
// in basis points (e.g. 200 = 2%). Fractions truncate.
func calcTax(grandTotal uint64, bps uint16) uint64 {
	return grandTotal * uint64(bps) / 10000
}
