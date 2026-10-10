package main

import (
	"strings"
	"testing"

	"vera-app/backend"
)

// The documentation names the values a caller may use, and it got them wrong
// once: it listed the retired TaxCategory vocabulary, so a services quotation
// built from `vera schema` was rejected by the server with
// "services require a category: \"SupplyOnly\"" and no hint of what to use
// instead.
//
// These tests read the constants rather than repeating the strings, so the
// documentation cannot drift from the server again -- if a category is renamed
// in backend/tax.go, the docs stop matching and this fails.

// TestSchemaNamesTheServiceCategoriesTheServerAccepts is the guard.
func TestSchemaNamesTheServiceCategoriesTheServerAccepts(t *testing.T) {
	var out, errOut strings.Builder
	if code := Run([]string{"schema"}, &out, &errOut); code != exitOK {
		t.Fatalf("vera schema exited %d: %s", code, errOut.String())
	}
	reference := out.String()

	for _, category := range []string{
		backend.ServiceCategoryNonConstruction,
		backend.ServiceCategoryConstruction,
	} {
		if !strings.Contains(reference, category) {
			t.Errorf("`vera schema` never mentions %q, which is a category the "+
				"server accepts. Anyone quoting services is left guessing.", category)
		}
	}
}

// TestSchemaDoesNotOfferTheRetiredCategories is the other half, and the half
// that actually broke.
//
// The TaxCategory values still exist in backend/tax.go because old revisions
// carry them, and they are read through a different field. Offering them as
// serviceCategory values sends the caller somewhere the server refuses.
func TestSchemaDoesNotOfferTheRetiredCategories(t *testing.T) {
	var out, errOut strings.Builder
	if code := Run([]string{"schema"}, &out, &errOut); code != exitOK {
		t.Fatalf("vera schema exited %d: %s", code, errOut.String())
	}
	reference := out.String()

	for _, retired := range []string{
		backend.TaxCategorySupplyOnly,
		backend.TaxCategorySupplyInstall,
		backend.TaxCategorySupplyInstallConstruction,
	} {
		if strings.Contains(reference, retired) {
			t.Errorf("`vera schema` offers %q as a serviceCategory value. It is the "+
				"retired taxCategory vocabulary, read through a different field, and "+
				"the server rejects it here.", retired)
		}
	}
}
