package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vera-app/backend"
)

// writeInput puts an input file on disk and returns its path, so tests read the
// file the way the CLI does rather than calling the loader with a string.
func writeInput(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quotation.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const minimalInput = `{
  "customerCode": 2,
  "subject": "Test",
  "items": [{"description": "A line", "uom": "Lot", "qty": 1, "unitCost": 1000000}]
}`

func TestLoadInputDocAcceptsAMinimalFile(t *testing.T) {
	doc, err := LoadInputDoc(writeInput(t, minimalInput))
	if err != nil {
		t.Fatalf("LoadInputDoc: %v", err)
	}
	if doc.CustomerCode != 2 {
		t.Errorf("customerCode = %d; want 2", doc.CustomerCode)
	}
	if len(doc.Items) != 1 {
		t.Fatalf("items = %d; want 1", len(doc.Items))
	}
}

func TestLoadInputDocRejectsBadInput(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"no customer code": {
			`{"subject":"x","items":[{"description":"a","uom":"Lot","qty":1}]}`,
			"customerCode",
		},
		"customer code out of range": {
			`{"customerCode":100,"items":[{"description":"a","uom":"Lot","qty":1}]}`,
			"between 1 and 99",
		},
		"no items": {
			`{"customerCode":2,"items":[]}`,
			"at least one line",
		},
		"item without a description": {
			`{"customerCode":2,"items":[{"uom":"Lot","qty":1}]}`,
			"description",
		},
		"item without a unit": {
			`{"customerCode":2,"items":[{"description":"a","qty":1}]}`,
			"uom",
		},
		"item without a quantity": {
			`{"customerCode":2,"items":[{"description":"a","uom":"Lot"}]}`,
			"qty",
		},
		"sub-item without a description": {
			`{"customerCode":2,"items":[{"description":"a","subItems":[{"uom":"Pc","qty":1}]}]}`,
			"subItems[0]",
		},
		"a misspelled field": {
			// A silently ignored field on a pricing document is a quotation
			// for a number nobody chose.
			`{"customerCode":2,"items":[{"description":"a","uom":"Lot","qty":1,"unitPrice":5}]}`,
			"unitPrice",
		},
		"not JSON at all": {
			`customerCode = 2`,
			"input file",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadInputDoc(writeInput(t, tc.body))
			if err == nil {
				t.Fatal("accepted input it should have refused")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q; want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestMissingInputFileNamesThePath(t *testing.T) {
	_, err := LoadInputDoc(filepath.Join(t.TempDir(), "absent.json"))
	if err == nil {
		t.Fatal("accepted a path that does not exist")
	}
	if !strings.Contains(err.Error(), "absent.json") {
		t.Errorf("error = %q; want the path in it", err)
	}
}

func TestConversionUsesTheServersFieldNames(t *testing.T) {
	doc, err := LoadInputDoc(writeInput(t, `{
	  "customerCode": 6,
	  "subject": "Conversions",
	  "items": [{
	    "description": "A line",
	    "uom": "Pc",
	    "qty": 3,
	    "unitCost": 250000,
	    "marginEnabled": true,
	    "marginPercent": 20,
	    "discountEnabled": true,
	    "discountPercent": 10
	  }]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	input, err := doc.toQuotationInput()
	if err != nil {
		t.Fatal(err)
	}
	if input.CustomerID != 6 {
		t.Errorf("CustomerID = %d; want 6", input.CustomerID)
	}
	item := input.Items[0]
	// unitCost in the file is UnitPrice on the wire: one word for the caller,
	// the server's own name underneath.
	if item.UnitPrice != 250000 {
		t.Errorf("UnitPrice = %d; want 250000", item.UnitPrice)
	}
	if item.Qty != 3 || item.UoM != "Pc" {
		t.Errorf("qty/uom = %d/%q; want 3/Pc", item.Qty, item.UoM)
	}
	if !item.MarginEnabled || item.MarginPercent != 20 {
		t.Errorf("margin = %v/%d; want true/20", item.MarginEnabled, item.MarginPercent)
	}
	if !item.DiscountEnabled || item.DiscountPercent != 10 {
		t.Errorf("discount = %v/%d; want true/10", item.DiscountEnabled, item.DiscountPercent)
	}
}

func TestConversionAppliesTheEditorsDefaults(t *testing.T) {
	doc, err := LoadInputDoc(writeInput(t, minimalInput))
	if err != nil {
		t.Fatal(err)
	}
	input, err := doc.toQuotationInput()
	if err != nil {
		t.Fatal(err)
	}
	// A minimal file has to produce the document a new quotation in the editor
	// would, not a zeroed one.
	if input.Incoterm != "EXW" {
		t.Errorf("Incoterm = %q; want EXW", input.Incoterm)
	}
	if input.Validity != 30 || input.PaymentTerm != 90 || input.LeadTime != 60 {
		t.Errorf("terms = %d/%d/%d; want 30/90/60", input.Validity, input.PaymentTerm, input.LeadTime)
	}
	if input.TaxZone != backend.TaxZoneFTZ {
		t.Errorf("TaxZone = %q; want %q", input.TaxZone, backend.TaxZoneFTZ)
	}
	if input.TaxType != backend.TaxTypeGoods {
		t.Errorf("TaxType = %q; want %q", input.TaxType, backend.TaxTypeGoods)
	}
}

func TestServiceCategoryIsDroppedForGoods(t *testing.T) {
	doc, err := LoadInputDoc(writeInput(t, `{
	  "customerCode": 2,
	  "serviceCategory": "SupplyInstall",
	  "items": [{"description":"a","uom":"Lot","qty":1,"unitCost":1}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	input, err := doc.toQuotationInput()
	if err != nil {
		t.Fatal(err)
	}
	// Sending a service category on a goods quotation is a validation error,
	// not a harmless extra, so the conversion drops it rather than letting the
	// server refuse a file that says Goods.
	if input.ServiceCategory != "" {
		t.Errorf("ServiceCategory = %q; want it dropped for goods", input.ServiceCategory)
	}
}

func TestASplitLineDoesNotPriceItself(t *testing.T) {
	doc, err := LoadInputDoc(writeInput(t, `{
	  "customerCode": 2,
	  "items": [{
	    "description": "A package",
	    "uom": "Pc",
	    "qty": 9,
	    "unitCost": 999999,
	    "marginEnabled": true,
	    "marginPercent": 40,
	    "showSubPrices": true,
	    "subItems": [
	      {"description": "Part A", "uom": "Pc", "qty": 2, "unitCost": 100000},
	      {"description": "Part B", "uom": "LS", "qty": 1, "unitCost": 50000}
	    ]
	  }]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	input, err := doc.toQuotationInput()
	if err != nil {
		t.Fatal(err)
	}
	item := input.Items[0]
	// The parts answer the pricing question; the line's own figures would be a
	// second, contradictory answer.
	if item.UnitPrice != 0 || item.MarginEnabled || item.MarginPercent != 0 {
		t.Errorf("the line kept its own pricing: price=%d margin=%v/%d",
			item.UnitPrice, item.MarginEnabled, item.MarginPercent)
	}
	if item.Qty != 1 || item.UoM != "Lot" {
		t.Errorf("qty/uom = %d/%q; want 1/Lot", item.Qty, item.UoM)
	}
	if len(item.SubItems) != 2 {
		t.Fatalf("subItems = %d; want 2", len(item.SubItems))
	}
	if item.SubItems[0].UnitCost != 100000 || item.SubItems[1].UnitCost != 50000 {
		t.Errorf("sub-item costs = %d/%d; want 100000/50000",
			item.SubItems[0].UnitCost, item.SubItems[1].UnitCost)
	}
	if !item.ShowSubPrices {
		t.Error("ShowSubPrices was lost; the customer presentation would change")
	}
}

func TestSubItemDetailsBecomeSubSubItems(t *testing.T) {
	doc, err := LoadInputDoc(writeInput(t, `{
	  "customerCode": 2,
	  "items": [{
	    "description": "A package",
	    "subItems": [{
	      "description": "Part A", "uom": "Pc", "qty": 1, "unitCost": 100,
	      "details": ["Includes fixing", "  ", "Includes testing"]
	    }]
	  }]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	input, err := doc.toQuotationInput()
	if err != nil {
		t.Fatal(err)
	}
	details := input.Items[0].SubItems[0].SubSubItems
	// Blank lines are dropped rather than printed as empty numbered rows.
	if len(details) != 2 {
		t.Fatalf("details = %d; want 2 (%v)", len(details), details)
	}
	if details[0].Description != "Includes fixing" || details[1].Description != "Includes testing" {
		t.Errorf("details = %v", details)
	}
}

func TestLineDetailsWithoutSubItemsAreRefused(t *testing.T) {
	doc, err := LoadInputDoc(writeInput(t, `{
	  "customerCode": 2,
	  "items": [{"description":"a","uom":"Lot","qty":1,"unitCost":1,"details":["scope"]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	// Scope lines print under a priced part, so a plain line has nowhere to put
	// them. Dropping them would print a document missing text the caller wrote.
	_, err = doc.toQuotationInput()
	if err == nil {
		t.Fatal("accepted line details that cannot be printed")
	}
	if !strings.Contains(err.Error(), "subItems") {
		t.Errorf("error = %q; want it to name the fix", err)
	}
}

func TestExampleInputIsValid(t *testing.T) {
	// `vera schema` hands people this file and tells them it runs. If it stops
	// running, the CLI's own documentation is wrong.
	doc, err := LoadInputDoc(writeInput(t, exampleInput))
	if err != nil {
		t.Fatalf("the worked example does not load: %v", err)
	}
	if _, err := doc.toQuotationInput(); err != nil {
		t.Fatalf("the worked example does not convert: %v", err)
	}
	if len(doc.Items) < 3 {
		t.Errorf("the example has %d lines; it is meant to exercise several shapes", len(doc.Items))
	}
}

func TestBlankInputIsDeliberatelyIncomplete(t *testing.T) {
	// The skeleton is a shape to fill in, not something to run: loading it must
	// fail, and the message must name the first thing to fill in. A skeleton
	// that quietly produced a zero-rupee quotation would be far worse.
	_, err := LoadInputDoc(writeInput(t, blankInput))
	if err == nil {
		t.Fatal("the blank skeleton was accepted as a complete quotation")
	}
	if !strings.Contains(err.Error(), "customerCode") {
		t.Errorf("error = %q; want it to name the first blank", err)
	}
}
