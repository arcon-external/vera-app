package backend

import "fmt"

// Tax zones for quotation pricing.
const (
	TaxZoneFTZ    = "FTZ"
	TaxZoneNonFTZ = "NonFTZ"
)

// Tax categories for Non-FTZ quotations.
const (
	TaxCategorySupplyOnly                = "SupplyOnly"
	TaxCategorySupplyInstall             = "SupplyInstall"
	TaxCategorySupplyInstallConstruction = "SupplyInstallConstruction"
)

// taxRateBps maps Non-FTZ categories to withholding rates in basis points.
var taxRateBps = map[string]uint16{
	TaxCategorySupplyOnly:                1100, // 11%
	TaxCategorySupplyInstall:             200,  // 2%
	TaxCategorySupplyInstallConstruction: 175,  // 1.75%
}

// taxLabels maps categories and zones to human-readable labels.
var taxLabels = map[string]string{
	TaxZoneFTZ:                           "FTZ (0%)",
	TaxCategorySupplyOnly:                "Supply Only (11%)",
	TaxCategorySupplyInstall:             "Supply and Install Services (2%)",
	TaxCategorySupplyInstallConstruction: "Supply and Install Services - Construction (1.75%)",
}

// taxRateFor resolves the withholding rate in basis points for a zone/category
// pair. FTZ always yields 0 and must not carry a category; Non-FTZ requires
// a known category.
func taxRateFor(zone, category string) (uint16, string, error) {
	switch zone {
	case TaxZoneFTZ:
		if category != "" {
			return 0, "", fmt.Errorf("FTZ must not have a tax category")
		}
		return 0, taxLabels[TaxZoneFTZ], nil
	case TaxZoneNonFTZ:
		bps, ok := taxRateBps[category]
		if !ok {
			return 0, "", fmt.Errorf("invalid tax category for Non-FTZ: %q", category)
		}
		return bps, taxLabels[category], nil
	default:
		return 0, "", fmt.Errorf("invalid tax zone: %q", zone)
	}
}

// calcTax computes the withholding amount on a grand total using integer
// math. bps is in basis points (e.g. 200 = 2%). Fractions truncate.
func calcTax(grandTotal uint64, bps uint16) uint64 {
	return grandTotal * uint64(bps) / 10000
}
