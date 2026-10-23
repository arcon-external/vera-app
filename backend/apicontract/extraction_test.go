package apicontract

import (
	"strings"
	"testing"
)

// Issue #498: the extraction contract is evidence, never Customer identity.
// This pins the closed six-key header set: adding a seventh key (a customer
// name, say) would change the stored-result schema for every revision, so the
// decision to keep identity out is asserted here rather than left to memory.
func TestExtractionContractHasNoCustomerIdentityField(t *testing.T) {
	if len(ExtractionFieldKeys) != 6 {
		t.Fatalf("header keys = %d; want the closed six-key set", len(ExtractionFieldKeys))
	}
	for _, key := range ExtractionFieldKeys {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "customer") || strings.Contains(lower, "client") {
			t.Errorf("extraction key %q describes Customer identity; extraction cannot supply a Customer", key)
		}
	}
}
