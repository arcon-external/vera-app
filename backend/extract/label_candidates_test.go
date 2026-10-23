package extract

import (
	"testing"

	"vera-app/backend/apicontract"
)

// Review regression (#476): a legal-text line containing "issued to" and an
// Incoterm section's "Location" line are label lookalikes, not field values.
// A valid value printed later must win over the lookalike; the lookalike must
// not turn the field into a conflict.
func TestProposePrefersARealFieldValueOverALabelLookalike(t *testing.T) {
	words := []testWord{
		word("Purchase", 50, 60, 60), word("Order", 114, 60, 40),
		word("No:", 154, 60, 18), word("900123", 174, 60, 40),
		word("This", 50, 90, 20), word("purchase", 72, 90, 50),
		word("order", 124, 90, 30), word("shall", 156, 90, 30),
		word("be", 188, 90, 14), word("governed", 204, 90, 48),
		word("by", 254, 90, 14), word("the", 270, 90, 18),
		word("terms", 290, 90, 30), word("issued", 322, 90, 34),
		word("to", 358, 90, 14), word("the", 374, 90, 18),
		word("supplier.", 394, 90, 50),
		word("Order", 50, 120, 38), word("submitted", 92, 120, 60),
		word("on:", 154, 120, 20), word("Tuesday", 176, 120, 45),
		word("7", 224, 120, 6), word("Apr", 234, 120, 20),
		word("2026", 258, 120, 26),
		word("Incoterm", 50, 150, 48), word("Code:", 100, 150, 32),
		word("DDP", 136, 150, 24),
		word("Incoterm", 50, 180, 48), word("Location:", 100, 180, 48),
		word("Ship-to", 152, 180, 42),
	}
	result := parseFixture(t, words)
	assertField(t, result, apicontract.ExtractionFieldPONumber, "900123", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPODate, "2026-04-07", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "DDP", apicontract.ExtractionStatusExtracted)
}
